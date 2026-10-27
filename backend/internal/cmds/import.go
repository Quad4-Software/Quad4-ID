package cmds

import (
	"archive/zip"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/italypaleale/francis/components"
	"github.com/spf13/cobra"

	"github.com/pocket-id/pocket-id/backend/internal/backup"
	"github.com/pocket-id/pocket-id/backend/internal/bootstrap"
	"github.com/pocket-id/pocket-id/backend/internal/common"
	"github.com/pocket-id/pocket-id/backend/internal/service"
	"github.com/pocket-id/pocket-id/backend/internal/utils"
)

type importFlags struct {
	Path                  string
	Yes                   bool
	ForcefullyAcquireLock bool
}

func init() {
	var flags importFlags

	importCmd := &cobra.Command{
		Use:   "import",
		Short: "Imports all data of Quad4 ID from a ZIP file",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runImport(cmd.Context(), flags)
		},
	}

	importCmd.Flags().StringVarP(&flags.Path, "path", "p", "quad4-id-export.zip", "Path to the ZIP file to import the data from, or '-' to read from stdin")
	importCmd.Flags().BoolVarP(&flags.Yes, "yes", "y", false, "Skip confirmation prompts")
	importCmd.Flags().BoolVarP(&flags.ForcefullyAcquireLock, "forcefully-acquire-lock", "", false, "Forcefully acquire exclusive access by terminating any running Quad4 ID instance")

	rootCmd.AddCommand(importCmd)
}

// runImport handles the high-level orchestration of the import process
func runImport(ctx context.Context, flags importFlags) error {
	// A standalone Francis runtime owns the actor data, so this import only covers what lives in Quad4 ID's own database
	// Nothing here can limit the replicas either, since they are hosts of the runtime's cluster rather than of a cluster in this database
	if !common.EnvConfig.HasEmbeddedFrancisRuntime() {
		printRemoteActorDataNotice(
			"The actor data will NOT be restored, and Quad4 ID replicas will NOT be stopped for you",
			"stop every replica first, then restore the runtime with: francis runtime restore -f actors.bin",
		)
	}

	if !flags.Yes {
		ok, err := askForConfirmation()
		if err != nil {
			return fmt.Errorf("failed to get confirmation: %w", err)
		}
		if !ok {
			fmt.Println("Aborted")
			os.Exit(1)
		}
	}

	var (
		zipReader *zip.ReadCloser
		cleanup   func()
		err       error
	)

	if flags.Path == "-" {
		zipReader, cleanup, err = readZipFromStdin()
		defer cleanup()
	} else {
		err = rejectEncryptedBackupFile(flags.Path)
		if err == nil {
			zipReader, err = zip.OpenReader(flags.Path)
		}
	}
	if err != nil {
		return fmt.Errorf("failed to open zip: %w", err)
	}
	defer zipReader.Close()

	return runImportReader(ctx, &zipReader.Reader, flags)
}

// runImportReader performs the import from an already-open ZIP archive
func runImportReader(ctx context.Context, zipReader *zip.Reader, flags importFlags) error {
	embeddedRuntime := common.EnvConfig.HasEmbeddedFrancisRuntime()

	// An archive carrying the actor data was taken from a deployment with an embedded runtime, and there is nowhere to put that data here
	// Restoring only the Quad4 ID half of it would leave the runtime holding actor state from a different deployment, so refuse rather than half-restore
	if !embeddedRuntime {
		err := bootstrap.EnsureNoActorsBackup(zipReader)
		if err != nil {
			return err
		}
	}

	// Connect to the database without running migrations: the import re-creates the Quad4 ID schema itself
	db, pg, err := bootstrap.ConnectDatabase(ctx)
	if err != nil {
		return err
	}

	importCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	// Take exclusive access to the cluster so no Quad4 ID replica is running while we overwrite the database
	// The lease lives in the actor host's own tables, so it only exists when the runtime is embedded: with a standalone runtime the operator was told to stop the replicas instead
	var providerOpts components.ProviderOptions
	if embeddedRuntime {
		// The cluster admin talks to the same database as the actor host, so build its provider options the same way the host does
		providerOpts, err = bootstrap.ActorsProviderOptions(db, pg)
		if err != nil {
			return err
		}

		release, lost, err := acquireExclusiveAccess(ctx, providerOpts, flags.ForcefullyAcquireLock)
		if err != nil {
			return err
		}
		defer release()

		// Abort the import if exclusive access is lost partway through (for example if the lease can no longer be renewed)
		go func() {
			select {
			case <-lost:
				cancel()
			case <-importCtx.Done():
			}
		}()
	}

	// Init the storage provider
	storage, err := bootstrap.InitStorage(importCtx, db)
	if err != nil {
		return fmt.Errorf("failed to initialize storage: %w", err)
	}

	// Close filesystem storage handles before the command exits
	defer func() {
		_ = storage.Close()
	}()

	// The actor data lives outside of the Quad4 ID schema, so it's restored through Francis
	// Restoring requires exclusive access to the cluster, which was acquired above, and the import service skips the actor data entirely when no provider is passed
	var actorsProvider service.ActorsBackupProvider
	if embeddedRuntime {
		provider, err := bootstrap.NewActorsBackupProvider(importCtx, providerOpts)
		if err != nil {
			return fmt.Errorf("failed to initialize the actor host's data provider: %w", err)
		}
		defer func() {
			_ = provider.Close()
		}()

		actorsProvider = provider
	}

	// Create the import service
	importService := service.NewImportService(db, storage, actorsProvider)

	// Load from ZIP
	err = importService.ImportFromZip(importCtx, zipReader)
	if err != nil {
		return fmt.Errorf("failed to import data from zip: %w", err)
	}

	fmt.Println("Import completed successfully.")
	return nil
}

// acquireExclusiveAccess wraps bootstrap.AcquireExclusiveAccess with CLI-facing error messages
func acquireExclusiveAccess(ctx context.Context, providerOpts components.ProviderOptions, force bool) (release func(), lost <-chan struct{}, err error) {
	release, lost, err = bootstrap.AcquireExclusiveAccess(ctx, providerOpts, force)
	if err == nil {
		return release, lost, nil
	}

	switch {
	case errors.Is(err, components.ErrHostsConnected):
		//nolint:staticcheck
		return nil, nil, errors.New("Quad4 ID must be stopped before importing data - please stop the running instance or run with --forcefully-acquire-lock to terminate the other instance")
	case errors.Is(err, components.ErrExclusiveHeld):
		return nil, nil, errors.New("another exclusive operation, such as another import, is already in progress; please wait for it to complete and try again")
	default:
		return nil, nil, fmt.Errorf("failed to acquire exclusive access: %w", err)
	}
}

// rejectEncryptedBackupFile points the user at 'backup restore' when the file is an encrypted backup archive, which the plain import cannot read
func rejectEncryptedBackupFile(path string) error {
	f, err := os.Open(path)
	if err != nil {
		return nil
	}
	defer f.Close()

	_, encrypted, err := backup.PeekEncrypted(f)
	if err != nil || !encrypted {
		return nil
	}

	return errors.New("this backup archive is encrypted: restore it with 'quad4-id backup restore --file " + path + " --encryption-key <key>' instead")
}

func askForConfirmation() (bool, error) {
	fmt.Println("WARNING: This feature is experimental and may not work correctly. Please create a backup before proceeding and report any issues you encounter.")
	fmt.Println()
	fmt.Println("WARNING: Import will erase all existing data at the following locations:")
	fmt.Printf("Database:      %s\n", absolutePathOrOriginal(common.EnvConfig.DbConnectionString))
	fmt.Printf("Uploads Path:  %s\n", absolutePathOrOriginal(common.EnvConfig.UploadPath))

	ok, err := utils.PromptForConfirmation("Do you want to continue?")
	if err != nil {
		return false, err
	}

	return ok, nil
}

// absolutePathOrOriginal returns the absolute path of the given path, or the original if it fails
func absolutePathOrOriginal(path string) string {
	abs, err := filepath.Abs(path)
	if err != nil {
		return path
	}
	return abs
}

func readZipFromStdin() (*zip.ReadCloser, func(), error) {
	tmpFile, err := os.CreateTemp("", "pocket-id-import-*.zip")
	if err != nil {
		return nil, nil, fmt.Errorf("failed to create temporary file: %w", err)
	}

	cleanup := func() {
		_ = os.Remove(tmpFile.Name())
	}

	_, err = io.Copy(tmpFile, os.Stdin)
	if err != nil {
		tmpFile.Close()
		cleanup()
		return nil, nil, fmt.Errorf("failed to read data from stdin: %w", err)
	}

	err = tmpFile.Close()
	if err != nil {
		cleanup()
		return nil, nil, fmt.Errorf("failed to close temporary file: %w", err)
	}

	r, err := zip.OpenReader(tmpFile.Name())
	if err != nil {
		cleanup()
		return nil, nil, err
	}

	return r, cleanup, nil
}
