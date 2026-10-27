package service

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"slices"
	"strconv"
	"time"

	"gorm.io/gorm"

	"github.com/pocket-id/pocket-id/backend/internal/appconfig"
	"github.com/pocket-id/pocket-id/backend/internal/backup"
	"github.com/pocket-id/pocket-id/backend/internal/storage"
)

// BackupInfo describes one backup archive stored at the configured destination
type BackupInfo struct {
	Name      string    `json:"name"`
	Size      int64     `json:"size"`
	CreatedAt time.Time `json:"createdAt"`
	Encrypted bool      `json:"encrypted"`
}

// ActorsBackupHandle is a provider for the actor host's data that also owns resources the caller must release
type ActorsBackupHandle interface {
	ActorsBackupProvider
	Close() error
}

// ActorsBackupProviderFactory builds a provider for the actor host's data on demand, or returns nil when the actor data is not reachable from this process
type ActorsBackupProviderFactory func(ctx context.Context) (ActorsBackupHandle, error)

// BackupService creates, lists, and deletes backup archives at the configured destination, and stages restores that are applied on restart
type BackupService struct {
	db            *gorm.DB
	uploadStorage storage.FileStorage
	appConfig     appconfig.AppConfigResolver
	actorsFactory ActorsBackupProviderFactory
}

// NewBackupService creates a new BackupService
//
// actorsFactory may be nil or return nil providers, in which case backups do not include the actor host's data
func NewBackupService(db *gorm.DB, uploadStorage storage.FileStorage, cfgResolver appconfig.AppConfigResolver, actorsFactory ActorsBackupProviderFactory) *BackupService {
	return &BackupService{
		db:            db,
		uploadStorage: uploadStorage,
		appConfig:     cfgResolver,
		actorsFactory: actorsFactory,
	}
}

// BackupDestinationStorage builds the file storage that holds backups for the given configuration
func BackupDestinationStorage(ctx context.Context, cfg *appconfig.AppConfigModel) (storage.FileStorage, error) {
	switch cfg.BackupDestination.String() {
	case "s3":
		if cfg.BackupS3Bucket.String() == "" {
			return nil, errors.New("BACKUP_S3_BUCKET must be set when the backup destination is s3")
		}
		return storage.NewS3Storage(ctx, storage.S3Config{
			Bucket:          cfg.BackupS3Bucket.String(),
			Region:          cfg.BackupS3Region.String(),
			Endpoint:        cfg.BackupS3Endpoint.String(),
			AccessKeyID:     cfg.BackupS3AccessKeyID.String(),
			SecretAccessKey: cfg.BackupS3SecretAccessKey.String(),
			ForcePathStyle:  cfg.BackupS3ForcePathStyle.IsTrue(),
			Root:            cfg.BackupS3Prefix.String(),
		})
	case "local", "":
		return storage.NewFilesystemStorage(cfg.BackupLocalPath.String())
	default:
		return nil, fmt.Errorf("invalid backup destination: %s", cfg.BackupDestination)
	}
}

// ListBackups returns the backup archives at the configured destination, newest first
func (s *BackupService) ListBackups(ctx context.Context) ([]BackupInfo, error) {
	cfg, err := s.appConfig.GetConfig(ctx)
	if err != nil {
		return nil, err
	}

	dest, err := BackupDestinationStorage(ctx, cfg)
	if err != nil {
		return nil, err
	}
	defer dest.Close()

	return ListBackupsAt(ctx, dest)
}

// ListBackupsAt lists the backup archives in dest, newest first
// Only files with a name Quad4 ID generates are returned, so other objects at the destination are ignored
func ListBackupsAt(ctx context.Context, dest storage.FileStorage) ([]BackupInfo, error) {
	objects, err := dest.List(ctx, ".")
	if err != nil {
		return nil, fmt.Errorf("failed to list backups: %w", err)
	}

	backups := make([]BackupInfo, 0, len(objects))
	for _, obj := range objects {
		if !backup.IsBackupName(obj.Path) {
			continue
		}
		backups = append(backups, BackupInfo{
			Name:      obj.Path,
			Size:      obj.Size,
			CreatedAt: obj.ModTime,
			Encrypted: backup.IsEncryptedName(obj.Path),
		})
	}

	slices.SortFunc(backups, func(a, b BackupInfo) int {
		return cmp.Compare(b.Name, a.Name)
	})

	return backups, nil
}

// CreateBackup exports all data and writes a new archive to the configured destination
func (s *BackupService) CreateBackup(ctx context.Context) (*BackupInfo, error) {
	cfg, err := s.appConfig.GetConfig(ctx)
	if err != nil {
		return nil, err
	}

	dest, err := BackupDestinationStorage(ctx, cfg)
	if err != nil {
		return nil, err
	}
	defer dest.Close()

	info, err := s.CreateBackupTo(ctx, dest, []byte(cfg.BackupEncryptionKey))
	if err != nil {
		return nil, err
	}

	err = ApplyRetention(ctx, dest, intConfigValue(cfg.BackupRetentionCount, 0))
	if err != nil {
		return info, fmt.Errorf("backup was created but pruning old backups failed: %w", err)
	}

	return info, nil
}

// CreateBackupTo writes a new backup archive to dest, encrypting it when encryptionKey is not empty
func (s *BackupService) CreateBackupTo(ctx context.Context, dest storage.FileStorage, encryptionKey []byte) (*BackupInfo, error) {
	var actors ActorsBackupProvider
	if s.actorsFactory != nil {
		handle, err := s.actorsFactory(ctx)
		if err != nil {
			return nil, fmt.Errorf("failed to initialize the actor host's data provider: %w", err)
		}
		if handle != nil {
			defer handle.Close()
			actors = handle
		}
	}

	exportService := NewExportService(s.db, s.uploadStorage, actors)

	// Stream the export through the pipe so large archives never sit in memory
	pr, pw := io.Pipe()
	go func() {
		pw.CloseWithError(exportService.ExportToZip(ctx, pw))
	}()

	var r io.Reader = pr
	encrypted := len(encryptionKey) > 0
	if encrypted {
		var err error
		r, err = backup.NewEncryptingReader(pr, encryptionKey)
		if err != nil {
			_ = pr.CloseWithError(err)
			return nil, err
		}
	}

	name := backup.NewName(time.Now(), encrypted)
	err := dest.Save(ctx, name, r)
	if err != nil {
		// Unblock the export goroutine, which is still writing to the pipe
		_ = pr.CloseWithError(err)
		return nil, fmt.Errorf("failed to store backup %q: %w", name, err)
	}

	rc, size, err := dest.Open(ctx, name)
	if err != nil {
		return &BackupInfo{Name: name, CreatedAt: time.Now(), Encrypted: encrypted}, nil
	}
	_ = rc.Close()

	return &BackupInfo{Name: name, Size: size, CreatedAt: time.Now(), Encrypted: encrypted}, nil
}

// ApplyRetention deletes the oldest backup archives in dest, keeping at most keep
// A keep value of zero or less disables pruning
func ApplyRetention(ctx context.Context, dest storage.FileStorage, keep int) error {
	if keep <= 0 {
		return nil
	}

	backups, err := ListBackupsAt(ctx, dest)
	if err != nil {
		return err
	}

	for _, old := range backups[min(keep, len(backups)):] {
		if err := dest.Delete(ctx, old.Name); err != nil {
			return fmt.Errorf("failed to delete old backup %q: %w", old.Name, err)
		}
	}

	return nil
}

// OpenBackup opens a backup archive at the configured destination for reading
func (s *BackupService) OpenBackup(ctx context.Context, name string) (io.ReadCloser, int64, error) {
	if !backup.IsBackupName(name) {
		return nil, 0, fmt.Errorf("invalid backup name: %s", name)
	}

	cfg, err := s.appConfig.GetConfig(ctx)
	if err != nil {
		return nil, 0, err
	}

	dest, err := BackupDestinationStorage(ctx, cfg)
	if err != nil {
		return nil, 0, err
	}

	rc, size, err := dest.Open(ctx, name)
	if err != nil {
		dest.Close()
		return nil, 0, err
	}

	return &closeOnCloseReader{ReadCloser: rc, onClose: dest.Close}, size, nil
}

// closeOnCloseReader closes the destination storage after the archive stream is closed
type closeOnCloseReader struct {
	io.ReadCloser
	onClose func() error
}

func (r *closeOnCloseReader) Close() error {
	err := r.ReadCloser.Close()
	_ = r.onClose()
	return err
}

// DeleteBackup removes a backup archive from the configured destination
func (s *BackupService) DeleteBackup(ctx context.Context, name string) error {
	if !backup.IsBackupName(name) {
		return fmt.Errorf("invalid backup name: %s", name)
	}

	cfg, err := s.appConfig.GetConfig(ctx)
	if err != nil {
		return err
	}

	dest, err := BackupDestinationStorage(ctx, cfg)
	if err != nil {
		return err
	}
	defer dest.Close()

	return dest.Delete(ctx, name)
}

// RunScheduledBackup creates a backup when the newest one is older than the configured interval
// It is called periodically by the backup job and is a no-op when backups are disabled or not yet due
func (s *BackupService) RunScheduledBackup(ctx context.Context) error {
	cfg, err := s.appConfig.GetConfig(ctx)
	if err != nil {
		return fmt.Errorf("failed to load app config for scheduled backup: %w", err)
	}
	if !cfg.BackupEnabled.IsTrue() {
		return nil
	}

	dest, err := BackupDestinationStorage(ctx, cfg)
	if err != nil {
		return fmt.Errorf("failed to open backup destination: %w", err)
	}
	defer dest.Close()

	interval := time.Duration(intConfigValue(cfg.BackupIntervalHours, 24)) * time.Hour
	if interval < time.Hour {
		interval = time.Hour
	}

	backups, err := ListBackupsAt(ctx, dest)
	if err != nil {
		return err
	}
	if len(backups) > 0 && time.Since(backups[0].CreatedAt) < interval {
		return nil
	}

	slog.InfoContext(ctx, "Running scheduled backup", slog.String("destination", cfg.BackupDestination.String()))
	info, err := s.CreateBackupTo(ctx, dest, []byte(cfg.BackupEncryptionKey))
	if err != nil {
		return fmt.Errorf("scheduled backup failed: %w", err)
	}
	slog.InfoContext(ctx, "Scheduled backup created", slog.String("name", info.Name), slog.Int64("size", info.Size))

	return ApplyRetention(ctx, dest, intConfigValue(cfg.BackupRetentionCount, 0))
}

// intConfigValue parses an integer app config value, falling back to fallback on malformed input
func intConfigValue(v appconfig.AppConfigValue, fallback int) int {
	n, err := strconv.Atoi(v.String())
	if err != nil {
		return fallback
	}
	return n
}
