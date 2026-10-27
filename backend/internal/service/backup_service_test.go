package service

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/pocket-id/pocket-id/backend/internal/backup"
	"github.com/pocket-id/pocket-id/backend/internal/common"
	"github.com/pocket-id/pocket-id/backend/internal/storage"
	"github.com/pocket-id/pocket-id/backend/internal/utils"
)

// newBackupTestService returns a BackupService over a migrated SQLite database with filesystem upload storage
func newBackupTestService(t *testing.T) (*BackupService, func()) {
	t.Helper()

	dir := t.TempDir()
	db := openImportTestDB(t, filepath.Join(dir, "pocket-id.db"), nil)
	sqlDB, _ := db.DB()
	require.NoError(t, utils.MigrateDatabase(t.Context(), sqlDB))

	uploads, err := storage.NewFilesystemStorage(filepath.Join(dir, "uploads"))
	require.NoError(t, err)

	svc := NewBackupService(db, uploads, nil, nil)
	cleanup := func() {
		uploads.Close()
		closeImportTestDB(db)
	}
	return svc, cleanup
}

func TestCreateBackupToAndList(t *testing.T) {
	svc, cleanup := newBackupTestService(t)
	defer cleanup()

	dest, err := storage.NewFilesystemStorage(filepath.Join(t.TempDir(), "backups"))
	require.NoError(t, err)
	defer dest.Close()

	info, err := svc.CreateBackupTo(t.Context(), dest, nil)
	require.NoError(t, err)
	require.False(t, info.Encrypted)
	require.NotEmpty(t, info.Name)
	require.Positive(t, info.Size)

	backups, err := ListBackupsAt(t.Context(), dest)
	require.NoError(t, err)
	require.Len(t, backups, 1)
	require.Equal(t, info.Name, backups[0].Name)

	// The archive is a valid ZIP containing the database dump
	rc, _, err := dest.Open(t.Context(), info.Name)
	require.NoError(t, err)
	defer rc.Close()
	head := make([]byte, 2)
	_, err = io.ReadFull(rc, head)
	require.NoError(t, err)
	require.Equal(t, []byte("PK"), head)
}

func TestCreateEncryptedBackup(t *testing.T) {
	svc, cleanup := newBackupTestService(t)
	defer cleanup()

	dest, err := storage.NewFilesystemStorage(filepath.Join(t.TempDir(), "backups"))
	require.NoError(t, err)
	defer dest.Close()

	info, err := svc.CreateBackupTo(t.Context(), dest, []byte("backup-passphrase"))
	require.NoError(t, err)
	require.True(t, info.Encrypted)

	backups, err := ListBackupsAt(t.Context(), dest)
	require.NoError(t, err)
	require.Len(t, backups, 1)
	require.True(t, backups[0].Encrypted)
}

func TestApplyRetention(t *testing.T) {
	dest, err := storage.NewFilesystemStorage(t.TempDir())
	require.NoError(t, err)
	defer dest.Close()

	// Seed archives in name order, plus a foreign file that retention must leave alone
	names := []string{
		"quad4-id-backup-20260101-000000-aaaaaa.zip",
		"quad4-id-backup-20260102-000000-bbbbbb.zip",
		"quad4-id-backup-20260103-000000-cccccc.zip",
	}
	for _, name := range names {
		require.NoError(t, dest.Save(t.Context(), name, strings.NewReader("x")))
	}
	require.NoError(t, dest.Save(t.Context(), "unrelated.txt", strings.NewReader("y")))

	require.NoError(t, ApplyRetention(t.Context(), dest, 2))

	backups, err := ListBackupsAt(t.Context(), dest)
	require.NoError(t, err)
	require.Len(t, backups, 2)
	require.Equal(t, names[2], backups[0].Name)
	require.Equal(t, names[1], backups[1].Name)

	rc, _, err := dest.Open(t.Context(), "unrelated.txt")
	require.NoError(t, err)
	rc.Close()
}

func TestSealUnsealRestorePassphrase(t *testing.T) {
	old := common.EnvConfig.EncryptionKey
	common.EnvConfig.EncryptionKey = []byte("test-encryption-key-0123456789")
	t.Cleanup(func() { common.EnvConfig.EncryptionKey = old })

	sealed, err := SealRestorePassphrase([]byte("archive-passphrase"))
	require.NoError(t, err)
	require.NotContains(t, sealed, "archive-passphrase")

	pass, err := UnsealRestorePassphrase(sealed)
	require.NoError(t, err)
	require.Equal(t, []byte("archive-passphrase"), pass)

	_, err = UnsealRestorePassphrase("not-valid-base64!!!")
	require.Error(t, err)
}

func TestStageRestoreValidation(t *testing.T) {
	svc, cleanup := newBackupTestService(t)
	defer cleanup()

	dir := t.TempDir()
	oldProvider, oldConn := common.EnvConfig.DbProvider, common.EnvConfig.DbConnectionString
	common.EnvConfig.DbProvider = common.DbProviderSqlite
	common.EnvConfig.DbConnectionString = filepath.Join(dir, "pocket-id.db")
	t.Cleanup(func() {
		common.EnvConfig.DbProvider = oldProvider
		common.EnvConfig.DbConnectionString = oldConn
	})

	openFile := func(p string) func() (io.ReadCloser, error) {
		return func() (io.ReadCloser, error) {
			return os.Open(p)
		}
	}

	t.Run("rejects a non-zip file", func(t *testing.T) {
		p := filepath.Join(t.TempDir(), "not-a-backup.bin")
		require.NoError(t, os.WriteFile(p, []byte("definitely not a zip"), 0o600))
		err := svc.StageRestore("bad.bin", openFile(p), nil)
		require.ErrorContains(t, err, "not a Quad4 ID backup archive")
	})

	t.Run("rejects an encrypted archive without a passphrase", func(t *testing.T) {
		p := filepath.Join(t.TempDir(), "enc.bin")
		require.NoError(t, os.WriteFile(p, encryptTestBackup(t, []byte("PK\x03\x04payload"), []byte("pw")), 0o600))
		err := svc.StageRestore("enc.bin", openFile(p), nil)
		require.ErrorContains(t, err, "passphrase")
	})

	t.Run("rejects an encrypted archive with the wrong passphrase", func(t *testing.T) {
		p := filepath.Join(t.TempDir(), "enc.bin")
		require.NoError(t, os.WriteFile(p, encryptTestBackup(t, []byte("PK\x03\x04payload"), []byte("pw")), 0o600))
		err := svc.StageRestore("enc.bin", openFile(p), []byte("wrong"))
		require.Error(t, err)
	})

	t.Run("stages a valid archive", func(t *testing.T) {
		p := filepath.Join(t.TempDir(), "good.zip")
		require.NoError(t, os.WriteFile(p, []byte("PK\x03\x04fake-zip-payload"), 0o600))
		require.NoError(t, svc.StageRestore("good.zip", openFile(p), nil))

		meta, err := ReadPendingRestore()
		require.NoError(t, err)
		require.NotNil(t, meta)
		require.Equal(t, "good.zip", meta.Name)
		require.False(t, meta.Encrypted)

		require.NoError(t, ClearPendingRestore())
		meta, err = ReadPendingRestore()
		require.NoError(t, err)
		require.Nil(t, meta)
	})
}

func encryptTestBackup(t *testing.T, plain, passphrase []byte) []byte {
	t.Helper()
	r, err := backup.NewEncryptingReader(bytes.NewReader(plain), passphrase)
	require.NoError(t, err)
	out, err := io.ReadAll(r)
	require.NoError(t, err)
	return out
}
