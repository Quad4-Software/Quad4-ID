package cmds

import (
	"archive/zip"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strconv"
	"text/tabwriter"
	"time"

	"github.com/spf13/cobra"

	"github.com/pocket-id/pocket-id/backend/internal/backup"
	"github.com/pocket-id/pocket-id/backend/internal/bootstrap"
	"github.com/pocket-id/pocket-id/backend/internal/common"
	"github.com/pocket-id/pocket-id/backend/internal/service"
	"github.com/pocket-id/pocket-id/backend/internal/storage"
)

// backupTargetFlags describes where backup archives live: a local directory or an S3 bucket
// Every flag falls back to the BACKUP_* environment variables the application configuration also uses
type backupTargetFlags struct {
	Destination       string
	Path              string
	S3Bucket          string
	S3Region          string
	S3Endpoint        string
	S3Prefix          string
	S3AccessKeyID     string
	S3SecretAccessKey string
	S3ForcePathStyle  bool
}

type backupKeyFlags struct {
	EncryptionKey     string
	EncryptionKeyFile string
}

func init() {
	backupCmd := &cobra.Command{
		Use:   "backup",
		Short: "Creates, lists, downloads, deletes and restores Quad4 ID backups",
	}

	createFlags := &struct {
		backupTargetFlags
		backupKeyFlags
		Keep int
	}{}
	createCmd := &cobra.Command{
		Use:   "create",
		Short: "Creates a backup archive at the configured destination",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runBackupCreate(cmd.Context(), createFlags)
		},
	}
	addBackupTargetFlags(createCmd, &createFlags.backupTargetFlags)
	addBackupKeyFlags(createCmd, &createFlags.backupKeyFlags)
	createCmd.Flags().IntVar(&createFlags.Keep, "keep", -1, "Delete the oldest backups so at most this many remain (0 keeps all, -1 uses BACKUP_RETENTION_COUNT or keeps all)")
	backupCmd.AddCommand(createCmd)

	var listFlags backupTargetFlags
	listCmd := &cobra.Command{
		Use:   "list",
		Short: "Lists the backup archives at the configured destination",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runBackupList(cmd.Context(), &listFlags)
		},
	}
	addBackupTargetFlags(listCmd, &listFlags)
	backupCmd.AddCommand(listCmd)

	deleteFlags := &struct {
		backupTargetFlags
		Name string
	}{}
	deleteCmd := &cobra.Command{
		Use:   "delete",
		Short: "Deletes a backup archive from the configured destination",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runBackupDelete(cmd.Context(), deleteFlags)
		},
	}
	addBackupTargetFlags(deleteCmd, &deleteFlags.backupTargetFlags)
	deleteCmd.Flags().StringVarP(&deleteFlags.Name, "name", "n", "", "Name of the backup archive to delete")
	_ = deleteCmd.MarkFlagRequired("name")
	backupCmd.AddCommand(deleteCmd)

	downloadFlags := &struct {
		backupTargetFlags
		Name string
		Out  string
	}{}
	downloadCmd := &cobra.Command{
		Use:   "download",
		Short: "Downloads a backup archive from the configured destination",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runBackupDownload(cmd.Context(), downloadFlags)
		},
	}
	addBackupTargetFlags(downloadCmd, &downloadFlags.backupTargetFlags)
	downloadCmd.Flags().StringVarP(&downloadFlags.Name, "name", "n", "", "Name of the backup archive to download")
	_ = downloadCmd.MarkFlagRequired("name")
	downloadCmd.Flags().StringVarP(&downloadFlags.Out, "out", "o", "-", "Path to write the archive to, or '-' for stdout")
	backupCmd.AddCommand(downloadCmd)

	restoreFlags := &struct {
		backupTargetFlags
		backupKeyFlags
		File                  string
		Name                  string
		Yes                   bool
		ForcefullyAcquireLock bool
	}{}
	restoreCmd := &cobra.Command{
		Use:   "restore",
		Short: "Restores Quad4 ID from a backup archive; Quad4 ID must be stopped first",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runBackupRestore(cmd.Context(), restoreFlags)
		},
	}
	addBackupTargetFlags(restoreCmd, &restoreFlags.backupTargetFlags)
	addBackupKeyFlags(restoreCmd, &restoreFlags.backupKeyFlags)
	restoreCmd.Flags().StringVarP(&restoreFlags.File, "file", "f", "", "Path to a local backup archive")
	restoreCmd.Flags().StringVarP(&restoreFlags.Name, "name", "n", "", "Name of a backup archive at the configured destination")
	restoreCmd.Flags().BoolVarP(&restoreFlags.Yes, "yes", "y", false, "Skip confirmation prompts")
	restoreCmd.Flags().BoolVarP(&restoreFlags.ForcefullyAcquireLock, "forcefully-acquire-lock", "", false, "Forcefully acquire exclusive access by terminating any running Quad4 ID instance")
	backupCmd.AddCommand(restoreCmd)

	rootCmd.AddCommand(backupCmd)
}

func addBackupTargetFlags(cmd *cobra.Command, flags *backupTargetFlags) {
	cmd.Flags().StringVarP(&flags.Destination, "dest", "d", "", "Backup destination: 'local' or 's3' (default BACKUP_DESTINATION or 'local')")
	cmd.Flags().StringVarP(&flags.Path, "path", "p", "", "Directory for local backups (default BACKUP_LOCAL_PATH or 'data/backups')")
	cmd.Flags().StringVar(&flags.S3Bucket, "s3-bucket", "", "S3 bucket for backups (default BACKUP_S3_BUCKET)")
	cmd.Flags().StringVar(&flags.S3Region, "s3-region", "", "S3 region (default BACKUP_S3_REGION)")
	cmd.Flags().StringVar(&flags.S3Endpoint, "s3-endpoint", "", "S3 endpoint for S3-compatible services (default BACKUP_S3_ENDPOINT)")
	cmd.Flags().StringVar(&flags.S3Prefix, "s3-prefix", "", "Key prefix inside the S3 bucket (default BACKUP_S3_PREFIX)")
	cmd.Flags().StringVar(&flags.S3AccessKeyID, "s3-access-key-id", "", "S3 access key ID (default BACKUP_S3_ACCESS_KEY_ID)")
	cmd.Flags().StringVar(&flags.S3SecretAccessKey, "s3-secret-access-key", "", "S3 secret access key (default BACKUP_S3_SECRET_ACCESS_KEY)")
	cmd.Flags().BoolVar(&flags.S3ForcePathStyle, "s3-force-path-style", false, "Use path-style S3 addressing (default BACKUP_S3_FORCE_PATH_STYLE)")
}

func addBackupKeyFlags(cmd *cobra.Command, flags *backupKeyFlags) {
	cmd.Flags().StringVar(&flags.EncryptionKey, "encryption-key", "", "Passphrase used to encrypt or decrypt the archive (default BACKUP_ENCRYPTION_KEY)")
	cmd.Flags().StringVar(&flags.EncryptionKeyFile, "encryption-key-file", "", "Path to a file containing the encryption passphrase (default BACKUP_ENCRYPTION_KEY_FILE)")
}

// open resolves the destination and returns a storage over it
func (f *backupTargetFlags) open(ctx context.Context) (storage.FileStorage, error) {
	f.Destination = envFallback(f.Destination, "BACKUP_DESTINATION", "local")
	f.Path = envFallback(f.Path, "BACKUP_LOCAL_PATH", "data/backups")
	f.S3Bucket = envFallback(f.S3Bucket, "BACKUP_S3_BUCKET", "")
	f.S3Region = envFallback(f.S3Region, "BACKUP_S3_REGION", "")
	f.S3Endpoint = envFallback(f.S3Endpoint, "BACKUP_S3_ENDPOINT", "")
	f.S3Prefix = envFallback(f.S3Prefix, "BACKUP_S3_PREFIX", "")
	f.S3AccessKeyID = envFallback(f.S3AccessKeyID, "BACKUP_S3_ACCESS_KEY_ID", "")
	f.S3SecretAccessKey = envFallback(f.S3SecretAccessKey, "BACKUP_S3_SECRET_ACCESS_KEY", "")
	if !f.S3ForcePathStyle {
		f.S3ForcePathStyle = os.Getenv("BACKUP_S3_FORCE_PATH_STYLE") == "true"
	}

	switch f.Destination {
	case "local":
		return storage.NewFilesystemStorage(f.Path)
	case "s3":
		if f.S3Bucket == "" {
			return nil, errors.New("an S3 bucket is required: pass --s3-bucket or set BACKUP_S3_BUCKET")
		}
		return storage.NewS3Storage(ctx, storage.S3Config{
			Bucket:          f.S3Bucket,
			Region:          f.S3Region,
			Endpoint:        f.S3Endpoint,
			AccessKeyID:     f.S3AccessKeyID,
			SecretAccessKey: f.S3SecretAccessKey,
			ForcePathStyle:  f.S3ForcePathStyle,
			Root:            f.S3Prefix,
		})
	default:
		return nil, fmt.Errorf("invalid backup destination %q: must be 'local' or 's3'", f.Destination)
	}
}

func envFallback(value, envName, fallback string) string {
	if value != "" {
		return value
	}
	if v := os.Getenv(envName); v != "" {
		return v
	}
	return fallback
}

// resolveEncryptionKey finds the archive passphrase: flag, flag file, then the environment
func resolveEncryptionKey(flags *backupKeyFlags) ([]byte, error) {
	if flags.EncryptionKey != "" {
		return []byte(flags.EncryptionKey), nil
	}
	if flags.EncryptionKeyFile != "" {
		data, err := os.ReadFile(flags.EncryptionKeyFile)
		if err != nil {
			return nil, fmt.Errorf("failed to read the encryption key file: %w", err)
		}
		return []byte(trimSpaceBytes(data)), nil
	}
	if v := os.Getenv("BACKUP_ENCRYPTION_KEY"); v != "" {
		return []byte(v), nil
	}
	v, ok, err := common.LoadStringEnvVarFromFile("BACKUP_ENCRYPTION_KEY")
	if err != nil {
		return nil, err
	}
	if ok {
		return []byte(v), nil
	}
	return nil, nil
}

func trimSpaceBytes(b []byte) []byte {
	for len(b) > 0 && (b[len(b)-1] == '\n' || b[len(b)-1] == '\r' || b[len(b)-1] == ' ' || b[len(b)-1] == '\t') {
		b = b[:len(b)-1]
	}
	return b
}

func runBackupCreate(ctx context.Context, flags *struct {
	backupTargetFlags
	backupKeyFlags
	Keep int
}) error {
	if !common.EnvConfig.HasEmbeddedFrancisRuntime() {
		printRemoteActorDataNotice("The actor data is NOT included in this backup", "back it up separately with: francis runtime backup -f actors.bin")
	}

	db, pg, err := bootstrap.NewDatabase(ctx)
	if err != nil {
		return fmt.Errorf("failed to connect to database: %w", err)
	}

	uploadStorage, err := bootstrap.InitStorage(ctx, db)
	if err != nil {
		return fmt.Errorf("failed to initialize storage: %w", err)
	}
	defer uploadStorage.Close()

	dest, err := flags.open(ctx)
	if err != nil {
		return err
	}
	defer dest.Close()

	key, err := resolveEncryptionKey(&flags.backupKeyFlags)
	if err != nil {
		return err
	}

	backupService := service.NewBackupService(db, uploadStorage, nil, bootstrap.NewActorsBackupProviderFactory(db, pg))
	info, err := backupService.CreateBackupTo(ctx, dest, key)
	if err != nil {
		return err
	}

	keep := flags.Keep
	if keep < 0 {
		keep, _ = strconv.Atoi(os.Getenv("BACKUP_RETENTION_COUNT"))
	}
	if err := service.ApplyRetention(ctx, dest, keep); err != nil {
		return err
	}

	fmt.Printf("Created backup %s\n", info.Name)
	return nil
}

func runBackupList(ctx context.Context, flags *backupTargetFlags) error {
	dest, err := flags.open(ctx)
	if err != nil {
		return err
	}
	defer dest.Close()

	backups, err := service.ListBackupsAt(ctx, dest)
	if err != nil {
		return err
	}
	if len(backups) == 0 {
		fmt.Println("No backups found")
		return nil
	}

	w := tabwriter.NewWriter(os.Stdout, 0, 4, 3, ' ', 0)
	fmt.Fprintln(w, "NAME\tSIZE\tCREATED\tENCRYPTED")
	for _, b := range backups {
		fmt.Fprintf(w, "%s\t%s\t%s\t%t\n", b.Name, formatBytes(b.Size), b.CreatedAt.Format(time.DateTime), b.Encrypted)
	}
	return w.Flush()
}

func runBackupDelete(ctx context.Context, flags *struct {
	backupTargetFlags
	Name string
}) error {
	if !backup.IsBackupName(flags.Name) {
		return fmt.Errorf("invalid backup name: %s", flags.Name)
	}

	dest, err := flags.open(ctx)
	if err != nil {
		return err
	}
	defer dest.Close()

	if err := dest.Delete(ctx, flags.Name); err != nil {
		return fmt.Errorf("failed to delete backup %q: %w", flags.Name, err)
	}

	fmt.Printf("Deleted backup %s\n", flags.Name)
	return nil
}

func runBackupDownload(ctx context.Context, flags *struct {
	backupTargetFlags
	Name string
	Out  string
}) error {
	if !backup.IsBackupName(flags.Name) {
		return fmt.Errorf("invalid backup name: %s", flags.Name)
	}

	dest, err := flags.open(ctx)
	if err != nil {
		return err
	}
	defer dest.Close()

	rc, _, err := dest.Open(ctx, flags.Name)
	if err != nil {
		return fmt.Errorf("failed to open backup %q: %w", flags.Name, err)
	}
	defer rc.Close()

	var w io.Writer
	if flags.Out == "-" {
		w = os.Stdout
	} else {
		f, err := os.Create(flags.Out)
		if err != nil {
			return fmt.Errorf("failed to create output file: %w", err)
		}
		defer f.Close()
		w = f
	}

	if _, err := io.Copy(w, rc); err != nil {
		return fmt.Errorf("failed to download backup: %w", err)
	}

	if flags.Out != "-" {
		fmt.Printf("Downloaded backup %s to %s\n", flags.Name, flags.Out)
	}
	return nil
}

func runBackupRestore(ctx context.Context, flags *struct {
	backupTargetFlags
	backupKeyFlags
	File                  string
	Name                  string
	Yes                   bool
	ForcefullyAcquireLock bool
}) error {
	if (flags.File == "") == (flags.Name == "") {
		return errors.New("exactly one of --file or --name is required")
	}

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

	key, err := resolveEncryptionKey(&flags.backupKeyFlags)
	if err != nil {
		return err
	}

	var src io.ReadCloser
	if flags.File != "" {
		src, err = os.Open(flags.File)
		if err != nil {
			return fmt.Errorf("failed to open archive: %w", err)
		}
	} else {
		if !backup.IsBackupName(flags.Name) {
			return fmt.Errorf("invalid backup name: %s", flags.Name)
		}
		dest, err := flags.open(ctx)
		if err != nil {
			return err
		}
		defer dest.Close()

		rc, _, err := dest.Open(ctx, flags.Name)
		if err != nil {
			return fmt.Errorf("failed to open backup %q: %w", flags.Name, err)
		}
		src = rc
	}
	defer src.Close()

	zipReader, cleanup, err := openBackupZip(src, key)
	if err != nil {
		return err
	}
	defer cleanup()

	return runImportReader(ctx, &zipReader.Reader, importFlags{
		Yes:                   true,
		ForcefullyAcquireLock: flags.ForcefullyAcquireLock,
	})
}

// openBackupZip materializes a (possibly encrypted) backup stream into a temporary file and opens it as a ZIP archive
func openBackupZip(src io.Reader, key []byte) (*zip.ReadCloser, func(), error) {
	wrapped, encrypted, err := backup.PeekEncrypted(src)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to read archive: %w", err)
	}

	r := wrapped
	if encrypted {
		if len(key) == 0 {
			return nil, nil, errors.New("this backup is encrypted: pass --encryption-key or set BACKUP_ENCRYPTION_KEY")
		}
		r, err = backup.NewDecryptingReader(wrapped, key)
		if err != nil {
			return nil, nil, err
		}
	}

	tmp, err := os.CreateTemp("", "quad4-id-restore-*.zip")
	if err != nil {
		return nil, nil, fmt.Errorf("failed to create temporary file: %w", err)
	}
	cleanup := func() {
		_ = os.Remove(tmp.Name())
	}

	if _, err := io.Copy(tmp, r); err != nil {
		tmp.Close()
		cleanup()
		return nil, nil, fmt.Errorf("failed to read archive: %w", err)
	}
	if err := tmp.Close(); err != nil {
		cleanup()
		return nil, nil, fmt.Errorf("failed to read archive: %w", err)
	}

	zipReader, err := zip.OpenReader(tmp.Name())
	if err != nil {
		cleanup()
		return nil, nil, fmt.Errorf("the archive is not a valid backup: %w", err)
	}

	return zipReader, func() {
		zipReader.Close()
		cleanup()
	}, nil
}

func formatBytes(size int64) string {
	const unit = 1024
	if size < unit {
		return fmt.Sprintf("%d B", size)
	}
	div, exp := int64(unit), 0
	for n := size / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(size)/float64(div), "KMGTPE"[exp])
}
