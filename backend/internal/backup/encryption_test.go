package backup

import (
	"bytes"
	"crypto/rand"
	"io"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func encrypt(t *testing.T, data, passphrase []byte) []byte {
	t.Helper()
	r, err := NewEncryptingReader(bytes.NewReader(data), passphrase)
	require.NoError(t, err)
	out, err := io.ReadAll(r)
	require.NoError(t, err)
	return out
}

func decrypt(t *testing.T, data, passphrase []byte) ([]byte, error) {
	t.Helper()
	r, err := NewDecryptingReader(bytes.NewReader(data), passphrase)
	if err != nil {
		return nil, err
	}
	return io.ReadAll(r)
}

func TestEncryptDecryptRoundTrip(t *testing.T) {
	passphrase := []byte("correct horse battery staple")

	t.Run("small payload", func(t *testing.T) {
		plain := []byte("PK\x03\x04 the whole archive")
		ct := encrypt(t, plain, passphrase)

		require.True(t, bytes.HasPrefix(ct, []byte(magicHeader)))

		out, err := decrypt(t, ct, passphrase)
		require.NoError(t, err)
		require.Equal(t, plain, out)
	})

	t.Run("payload spanning multiple chunks", func(t *testing.T) {
		plain := make([]byte, chunkPlainSize*2+12345)
		_, err := rand.Read(plain)
		require.NoError(t, err)

		ct := encrypt(t, plain, passphrase)
		out, err := decrypt(t, ct, passphrase)
		require.NoError(t, err)
		require.Equal(t, plain, out)
	})

	t.Run("empty payload", func(t *testing.T) {
		ct := encrypt(t, nil, passphrase)
		out, err := decrypt(t, ct, passphrase)
		require.NoError(t, err)
		require.Empty(t, out)
	})

	t.Run("payload exactly one chunk", func(t *testing.T) {
		plain := bytes.Repeat([]byte{0xAB}, chunkPlainSize)
		ct := encrypt(t, plain, passphrase)
		out, err := decrypt(t, ct, passphrase)
		require.NoError(t, err)
		require.Equal(t, plain, out)
	})
}

func TestDecryptRejectsWrongPassphrase(t *testing.T) {
	ct := encrypt(t, []byte("PK\x03\x04 secret"), []byte("right"))

	_, err := decrypt(t, ct, []byte("wrong"))
	require.Error(t, err)
}

func TestDecryptRejectsTruncatedStream(t *testing.T) {
	plain := make([]byte, chunkPlainSize+10)
	ct := encrypt(t, plain, []byte("pw"))

	// Cutting after the first chunk keeps a valid prefix, so the failure must come from the missing final frame
	_, err := decrypt(t, ct[:len(ct)-20], []byte("pw"))
	require.ErrorIs(t, err, ErrTruncated)
}

func TestDecryptRejectsTamperedCiphertext(t *testing.T) {
	ct := encrypt(t, []byte("PK\x03\x04 data data data"), []byte("pw"))

	// Flip a byte inside the first chunk's ciphertext
	headerLen := len(magicHeader) + saltSize + noncePrefixSize
	ct[headerLen+frameHeaderSize+4] ^= 0xFF

	_, err := decrypt(t, ct, []byte("pw"))
	require.Error(t, err)
}

func TestDecryptRejectsNonBackupStream(t *testing.T) {
	_, err := NewDecryptingReader(bytes.NewReader([]byte("PK\x03\x04 a plain zip")), []byte("pw"))
	require.ErrorIs(t, err, ErrNotEncrypted)
}

func TestPeekEncrypted(t *testing.T) {
	ct := encrypt(t, []byte("data"), []byte("pw"))

	r, encrypted, err := PeekEncrypted(bytes.NewReader(ct))
	require.NoError(t, err)
	require.True(t, encrypted)

	// The returned reader still yields the full stream
	all, err := io.ReadAll(r)
	require.NoError(t, err)
	require.Equal(t, ct, all)

	r, encrypted, err = PeekEncrypted(bytes.NewReader([]byte("PK\x03\x04 zip")))
	require.NoError(t, err)
	require.False(t, encrypted)
	all, err = io.ReadAll(r)
	require.NoError(t, err)
	require.Equal(t, []byte("PK\x03\x04 zip"), all)
}

func TestIsEncryptedName(t *testing.T) {
	require.True(t, IsEncryptedName("quad4-id-backup-20261026-120000-a1b2c3.zip.enc"))
	require.False(t, IsEncryptedName("quad4-id-backup-20261026-120000-a1b2c3.zip"))
}

func TestNewName(t *testing.T) {
	name := NewName(time.Now(), false)
	require.True(t, IsBackupName(name))
	require.False(t, IsEncryptedName(name))

	encName := NewName(time.Now(), true)
	require.True(t, IsBackupName(encName))
	require.True(t, IsEncryptedName(encName))

	// Consecutive names are unique
	n1, n2 := NewName(time.Now(), false), NewName(time.Now(), false)
	require.NotEqual(t, n1, n2)
}

func TestIsBackupNameRejectsOtherFiles(t *testing.T) {
	require.False(t, IsBackupName("random.zip"))
	require.False(t, IsBackupName("quad4-id-backup-20261026.zip"))
	require.False(t, IsBackupName("../../etc/passwd.zip"))
	require.True(t, IsBackupName("quad4-id-backup-20261026-120000-a1b2c3.zip"))
}
