package backup

import (
	"crypto/rand"
	"encoding/hex"
	"regexp"
	"time"
)

const (
	// NamePrefix starts every backup archive Quad4 ID creates, so listings can tell backups apart from other files at the destination
	NamePrefix = "quad4-id-backup-"

	// ZipExtension ends unencrypted backup archives
	ZipExtension = ".zip"

	// EncryptedExtension ends encrypted backup archives, on top of the zip extension
	EncryptedExtension = ".zip.enc"

	// nameTimestampFormat keeps generated names sortable by creation time
	nameTimestampFormat = "20060102-150405"
)

// backupNameRe matches the names NewName produces: prefix, timestamp, and a random suffix, then .zip or .zip.enc
var backupNameRe = regexp.MustCompile(`^` + regexp.QuoteMeta(NamePrefix) + `\d{8}-\d{6}-[0-9a-f]{6}` + regexp.QuoteMeta(ZipExtension) + `(\.enc)?$`)

// NewName generates a unique, time-sortable backup archive name
func NewName(t time.Time, encrypted bool) string {
	var randSuffix [3]byte
	_, _ = rand.Read(randSuffix[:])

	name := NamePrefix + t.UTC().Format(nameTimestampFormat) + "-" + hex.EncodeToString(randSuffix[:]) + ZipExtension
	if encrypted {
		name += ".enc"
	}

	return name
}

// IsBackupName reports whether name looks like an archive Quad4 ID created
func IsBackupName(name string) bool {
	return backupNameRe.MatchString(name)
}
