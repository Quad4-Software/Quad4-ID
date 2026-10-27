package service

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/pocket-id/pocket-id/backend/internal/backup"
	"github.com/pocket-id/pocket-id/backend/internal/common"
	"github.com/pocket-id/pocket-id/backend/internal/utils/crypto"
)

const (
	// PendingRestoreArchiveName holds the staged archive that the next boot restores
	PendingRestoreArchiveName = "pending-restore.bin"
	// PendingRestoreMetaName describes the staged archive
	PendingRestoreMetaName = "pending-restore.json"
	// RestoreResultName records the outcome of the most recent restore attempt so the UI can report it
	RestoreResultName = "restore-result.json"
)

// PendingRestore describes a backup archive staged for restore on the next restart
type PendingRestore struct {
	Name      string    `json:"name"`
	Encrypted bool      `json:"encrypted"`
	Size      int64     `json:"size"`
	StagedAt  time.Time `json:"stagedAt"`
	// SealedKey carries the archive's encryption passphrase, sealed with the instance encryption key so it is only readable at boot with ENCRYPTION_KEY
	SealedKey string `json:"sealedKey,omitempty"`
}

// RestoreResult records the outcome of a boot-time restore
type RestoreResult struct {
	Name       string    `json:"name"`
	Success    bool      `json:"success"`
	Error      string    `json:"error,omitempty"`
	FinishedAt time.Time `json:"finishedAt"`
}

// PendingRestoreDir is the directory that holds the staged restore files
// It lives next to the SQLite database, or under "data" for other providers where no database path exists
func PendingRestoreDir() string {
	if common.EnvConfig.DbProvider == common.DbProviderSqlite {
		path, _, _ := strings.Cut(common.EnvConfig.DbConnectionString, "?")
		if dir := filepath.Dir(path); dir != "" {
			return dir
		}
	}
	return "data"
}

// ReadPendingRestore returns the staged restore, or nil when none is staged
func ReadPendingRestore() (*PendingRestore, error) {
	data, err := os.ReadFile(filepath.Join(PendingRestoreDir(), PendingRestoreMetaName))
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	var meta PendingRestore
	if err := json.Unmarshal(data, &meta); err != nil {
		return nil, fmt.Errorf("failed to parse pending restore metadata: %w", err)
	}
	return &meta, nil
}

// PendingRestoreArchivePath returns the path of the staged archive
func PendingRestoreArchivePath() string {
	return filepath.Join(PendingRestoreDir(), PendingRestoreArchiveName)
}

// ClearPendingRestore removes the staged restore files
func ClearPendingRestore() error {
	dir := PendingRestoreDir()
	err1 := os.Remove(filepath.Join(dir, PendingRestoreArchiveName))
	err2 := os.Remove(filepath.Join(dir, PendingRestoreMetaName))
	return errors.Join(ignoreNotExist(err1), ignoreNotExist(err2))
}

// MarkPendingRestoreFailed moves the staged files aside so a restore that failed once is not retried on every boot
func MarkPendingRestoreFailed() error {
	dir := PendingRestoreDir()
	suffix := ".failed-" + time.Now().UTC().Format("20060102-150405")
	err1 := os.Rename(filepath.Join(dir, PendingRestoreArchiveName), filepath.Join(dir, PendingRestoreArchiveName+suffix))
	err2 := os.Rename(filepath.Join(dir, PendingRestoreMetaName), filepath.Join(dir, PendingRestoreMetaName+suffix))
	return errors.Join(ignoreNotExist(err1), ignoreNotExist(err2))
}

// WriteRestoreResult records the outcome of a boot-time restore
func WriteRestoreResult(res RestoreResult) error {
	data, err := json.Marshal(res)
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(PendingRestoreDir(), RestoreResultName), data, 0o600)
}

// ReadRestoreResult returns the outcome of the most recent boot-time restore, or nil when none ran
func ReadRestoreResult() (*RestoreResult, error) {
	data, err := os.ReadFile(filepath.Join(PendingRestoreDir(), RestoreResultName))
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	var res RestoreResult
	if err := json.Unmarshal(data, &res); err != nil {
		return nil, fmt.Errorf("failed to parse restore result: %w", err)
	}
	return &res, nil
}

// StageRestore validates a backup archive and stages it for restore on the next restart
// open must return a fresh stream over the archive each time it is called, since validating an encrypted archive consumes one stream
// passphrase is required when the archive is encrypted, and is stored sealed with the instance encryption key
func (s *BackupService) StageRestore(name string, open func() (io.ReadCloser, error), passphrase []byte) (err error) {
	rc, err := open()
	if err != nil {
		return fmt.Errorf("failed to open archive: %w", err)
	}

	wrapped, encrypted, err := backup.PeekEncrypted(rc)
	if err != nil {
		rc.Close()
		return fmt.Errorf("failed to read archive: %w", err)
	}

	var sealedKey string
	if encrypted {
		if len(passphrase) == 0 {
			rc.Close()
			return errors.New("this backup is encrypted and requires its encryption passphrase")
		}
		if err = validateEncryptedZip(wrapped, passphrase); err != nil {
			rc.Close()
			return err
		}
		sealedKey, err = SealRestorePassphrase(passphrase)
		if err != nil {
			rc.Close()
			return fmt.Errorf("failed to protect the encryption passphrase: %w", err)
		}
	} else {
		if err = validateZipHeader(wrapped); err != nil {
			rc.Close()
			return err
		}
	}
	rc.Close()

	// The validating reads consumed the first stream, so the archive is copied from a fresh one
	rc, err = open()
	if err != nil {
		return fmt.Errorf("failed to reopen archive: %w", err)
	}
	defer rc.Close()

	dir := PendingRestoreDir()
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("failed to create restore staging directory: %w", err)
	}

	stagedPath := filepath.Join(dir, PendingRestoreArchiveName)
	tmp, err := os.CreateTemp(dir, ".pending-restore-*")
	if err != nil {
		return fmt.Errorf("failed to create staging file: %w", err)
	}
	size, err := io.Copy(tmp, rc)
	if err != nil {
		tmp.Close()
		os.Remove(tmp.Name())
		return fmt.Errorf("failed to stage archive: %w", err)
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmp.Name())
		return fmt.Errorf("failed to stage archive: %w", err)
	}
	if err := os.Rename(tmp.Name(), stagedPath); err != nil {
		os.Remove(tmp.Name())
		return fmt.Errorf("failed to stage archive: %w", err)
	}

	meta := PendingRestore{
		Name:      name,
		Encrypted: encrypted,
		Size:      size,
		StagedAt:  time.Now(),
		SealedKey: sealedKey,
	}
	data, err := json.Marshal(meta)
	if err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(dir, PendingRestoreMetaName), data, 0o600); err != nil {
		return fmt.Errorf("failed to write restore metadata: %w", err)
	}

	return nil
}

// validateZipHeader checks that the stream starts with the ZIP magic
func validateZipHeader(r io.Reader) error {
	head := make([]byte, 2)
	if _, err := io.ReadFull(r, head); err != nil {
		return errors.New("failed to read archive header")
	}
	if head[0] != 'P' || head[1] != 'K' {
		return errors.New("the file is not a Quad4 ID backup archive")
	}
	return nil
}

// validateEncryptedZip checks that the passphrase decrypts a stream that starts with the ZIP magic
// Only the first bytes are decrypted, so a wrong passphrase fails before the archive is staged
func validateEncryptedZip(r io.Reader, passphrase []byte) error {
	dr, err := backup.NewDecryptingReader(r, passphrase)
	if err != nil {
		return err
	}
	return validateZipHeader(dr)
}

// SealRestorePassphrase encrypts the passphrase under a key derived from the instance encryption key so the staged restore can decrypt the archive at boot
func SealRestorePassphrase(passphrase []byte) (string, error) {
	key, err := crypto.DeriveKey(common.EnvConfig.EncryptionKey, "quad4-id/pending-restore-key")
	if err != nil {
		return "", err
	}
	aead, err := newGCM(key)
	if err != nil {
		return "", err
	}

	nonce := make([]byte, aead.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return "", err
	}

	sealed := aead.Seal(nil, nonce, passphrase, nil)
	out := make([]byte, 0, len(nonce)+len(sealed))
	out = append(out, nonce...)
	out = append(out, sealed...)
	return base64.StdEncoding.EncodeToString(out), nil
}

// UnsealRestorePassphrase reverses SealRestorePassphrase
func UnsealRestorePassphrase(sealed string) ([]byte, error) {
	raw, err := base64.StdEncoding.DecodeString(sealed)
	if err != nil {
		return nil, fmt.Errorf("invalid sealed passphrase: %w", err)
	}

	key, err := crypto.DeriveKey(common.EnvConfig.EncryptionKey, "quad4-id/pending-restore-key")
	if err != nil {
		return nil, err
	}
	aead, err := newGCM(key)
	if err != nil {
		return nil, err
	}
	if len(raw) < aead.NonceSize() {
		return nil, errors.New("invalid sealed passphrase")
	}

	return aead.Open(nil, raw[:aead.NonceSize()], raw[aead.NonceSize():], nil)
}

func ignoreNotExist(err error) error {
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	return err
}

func newGCM(key []byte) (cipher.AEAD, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	return cipher.NewGCM(block)
}
