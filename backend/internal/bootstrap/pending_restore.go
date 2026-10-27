package bootstrap

import (
	"archive/zip"
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"gorm.io/gorm"

	"github.com/pocket-id/pocket-id/backend/internal/backup"
	"github.com/pocket-id/pocket-id/backend/internal/common"
	"github.com/pocket-id/pocket-id/backend/internal/service"
	"github.com/pocket-id/pocket-id/backend/internal/storage"
)

// applyPendingRestore applies a backup archive that an admin staged through the UI, then clears the staging files
// Restore failures are recorded and the staged files moved aside so a bad archive cannot break every boot; the instance then starts on the existing data
func applyPendingRestore(ctx context.Context, db *gorm.DB, pg *pgxpool.Pool, fileStorage storage.FileStorage) {
	meta, err := service.ReadPendingRestore()
	if err != nil {
		slog.ErrorContext(ctx, "Failed to read the staged restore", slog.Any("error", err))
		return
	}
	if meta == nil {
		return
	}

	slog.InfoContext(ctx, "Applying the staged restore", slog.String("backup", meta.Name), slog.Bool("encrypted", meta.Encrypted))

	err = doPendingRestore(ctx, meta, db, pg, fileStorage)
	res := service.RestoreResult{
		Name:       meta.Name,
		Success:    err == nil,
		FinishedAt: time.Now(),
	}
	if err != nil {
		res.Error = err.Error()
	}
	if wErr := service.WriteRestoreResult(res); wErr != nil {
		slog.WarnContext(ctx, "Failed to record the restore result", slog.Any("error", wErr))
	}

	if err != nil {
		slog.ErrorContext(ctx, "The staged restore failed; the instance starts with the existing data", slog.String("backup", meta.Name), slog.Any("error", err))
		if mErr := service.MarkPendingRestoreFailed(); mErr != nil {
			slog.WarnContext(ctx, "Failed to move the failed restore aside", slog.Any("error", mErr))
		}
		return
	}

	slog.InfoContext(ctx, "Staged restore applied", slog.String("backup", meta.Name))
	if err := service.ClearPendingRestore(); err != nil {
		slog.WarnContext(ctx, "Failed to remove the staged restore files", slog.Any("error", err))
	}
}

// doPendingRestore streams the staged archive through decryption into a temporary ZIP and imports it
func doPendingRestore(ctx context.Context, meta *service.PendingRestore, db *gorm.DB, pg *pgxpool.Pool, fileStorage storage.FileStorage) (err error) {
	f, err := os.Open(service.PendingRestoreArchivePath())
	if err != nil {
		return fmt.Errorf("failed to open the staged archive: %w", err)
	}
	defer f.Close()

	var r io.Reader = f
	if meta.Encrypted {
		passphrase, err := service.UnsealRestorePassphrase(meta.SealedKey)
		if err != nil {
			return fmt.Errorf("failed to unseal the backup passphrase: %w", err)
		}
		r, err = backup.NewDecryptingReader(f, passphrase)
		if err != nil {
			return err
		}
	}

	// The ZIP reader needs random access, so the archive lands in a temporary file first
	tmp, err := os.CreateTemp("", "quad4-id-restore-*.zip")
	if err != nil {
		return fmt.Errorf("failed to create a temporary file for the restore: %w", err)
	}
	defer func() {
		tmp.Close()
		_ = os.Remove(tmp.Name())
	}()

	if _, err := io.Copy(tmp, r); err != nil {
		return fmt.Errorf("failed to read the staged archive: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("failed to read the staged archive: %w", err)
	}

	zipReader, err := zip.OpenReader(tmp.Name())
	if err != nil {
		return fmt.Errorf("the staged archive is not a valid backup: %w", err)
	}
	defer zipReader.Close()

	embeddedRuntime := common.EnvConfig.HasEmbeddedFrancisRuntime()
	if !embeddedRuntime {
		// The actor data belongs to the standalone runtime, and half-restoring an archive that carries it would corrupt actor state
		if err := EnsureNoActorsBackup(&zipReader.Reader); err != nil {
			return err
		}
	}

	importCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	var actorsProvider service.ActorsBackupProvider
	if embeddedRuntime {
		providerOpts, err := ActorsProviderOptions(db, pg)
		if err != nil {
			return err
		}

		release, lost, err := AcquireExclusiveAccess(ctx, providerOpts, false)
		if err != nil {
			return fmt.Errorf("the staged restore requires exclusive access to the cluster: %w", err)
		}
		defer release()

		go func() {
			select {
			case <-lost:
				cancel()
			case <-importCtx.Done():
			}
		}()

		provider, err := NewActorsBackupProvider(importCtx, providerOpts)
		if err != nil {
			return fmt.Errorf("failed to initialize the actor host's data provider: %w", err)
		}
		defer provider.Close()

		actorsProvider = provider
	}

	importService := service.NewImportService(db, fileStorage, actorsProvider)
	if err := importService.ImportFromZip(importCtx, &zipReader.Reader); err != nil {
		return fmt.Errorf("failed to restore the backup: %w", err)
	}

	return nil
}
