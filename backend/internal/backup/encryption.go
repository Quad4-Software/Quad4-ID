// Package backup provides the encrypted archive format and naming used by Quad4 ID backups
package backup

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/binary"
	"errors"
	"fmt"
	"io"

	"golang.org/x/crypto/scrypt"
)

const (
	// magicHeader identifies a Quad4 ID encrypted backup stream
	magicHeader = "Q4IDENC1"

	saltSize        = 16
	noncePrefixSize = 4
	nonceSize       = 12
	frameHeaderSize = 5
	chunkPlainSize  = 256 * 1024
	// The largest frame a chunk can produce bounds the allocation a malformed length field can cause
	maxFrameSize = chunkPlainSize + 16

	scryptN      = 1 << 15
	scryptR      = 8
	scryptP      = 1
	scryptKeyLen = 32
)

// ErrNotEncrypted is returned when a stream does not start with the encrypted backup magic header
var ErrNotEncrypted = errors.New("the archive is not an encrypted Quad4 ID backup")

// ErrTruncated is returned when an encrypted stream ends before its final chunk
var ErrTruncated = errors.New("the encrypted archive is truncated")

// IsEncryptedName reports whether a backup name carries the encrypted extension
func IsEncryptedName(name string) bool {
	return len(name) > len(EncryptedExtension) && name[len(name)-len(EncryptedExtension):] == EncryptedExtension
}

// PeekEncrypted reports whether the stream starts with the encrypted backup magic header
// The returned reader yields the full stream including the bytes that were peeked
func PeekEncrypted(r io.Reader) (io.Reader, bool, error) {
	magic := make([]byte, len(magicHeader))
	n, err := io.ReadFull(r, magic)
	if err != nil && !errors.Is(err, io.EOF) && !errors.Is(err, io.ErrUnexpectedEOF) {
		return nil, false, err
	}

	reader := io.MultiReader(newBytesReader(magic[:n]), r)
	return reader, n == len(magicHeader) && string(magic) == magicHeader, nil
}

// newBytesReader returns a reader over b without copying it
func newBytesReader(b []byte) io.Reader {
	return &sliceReader{b: b}
}

type sliceReader struct {
	b   []byte
	off int
}

func (r *sliceReader) Read(p []byte) (int, error) {
	if r.off >= len(r.b) {
		return 0, io.EOF
	}
	n := copy(p, r.b[r.off:])
	r.off += n
	return n, nil
}

// deriveKey stretches the passphrase into the AEAD key for the stream identified by salt
func deriveKey(passphrase []byte, salt []byte) ([]byte, error) {
	return scrypt.Key(passphrase, salt, scryptN, scryptR, scryptP, scryptKeyLen)
}

func newAEAD(key []byte) (cipher.AEAD, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	return cipher.NewGCM(block)
}

// chunkNonce derives the per-chunk nonce from the stream's random prefix and the chunk index
func chunkNonce(noncePrefix []byte, index uint64) []byte {
	var nonce [nonceSize]byte
	copy(nonce[:], noncePrefix)
	binary.BigEndian.PutUint64(nonce[noncePrefixSize:], index)
	return nonce[:]
}

// chunkAAD authenticates the chunk index and final flag so frames cannot be reordered, replayed, or dropped
func chunkAAD(index uint64, last byte) []byte {
	aad := make([]byte, 9)
	binary.BigEndian.PutUint64(aad, index)
	aad[8] = last
	return aad
}

// NewEncryptingReader wraps src so the bytes read from it are the encrypted backup stream
func NewEncryptingReader(src io.Reader, passphrase []byte) (io.Reader, error) {
	if len(passphrase) == 0 {
		return nil, errors.New("encryption passphrase is empty")
	}

	header := make([]byte, len(magicHeader)+saltSize+noncePrefixSize)
	copy(header, magicHeader)
	if _, err := rand.Read(header[len(magicHeader):]); err != nil {
		return nil, fmt.Errorf("failed to generate encryption header: %w", err)
	}

	salt := header[len(magicHeader) : len(magicHeader)+saltSize]
	key, err := deriveKey(passphrase, salt)
	if err != nil {
		return nil, fmt.Errorf("failed to derive encryption key: %w", err)
	}

	aead, err := newAEAD(key)
	if err != nil {
		return nil, err
	}

	noncePrefix := header[len(magicHeader)+saltSize:]
	return &encryptingReader{
		src:         src,
		aead:        aead,
		noncePrefix: noncePrefix,
		pending:     newBytesReader(header),
		buf:         make([]byte, chunkPlainSize),
	}, nil
}

// encryptingReader encrypts src chunk by chunk on every Read
type encryptingReader struct {
	src         io.Reader
	aead        cipher.AEAD
	noncePrefix []byte
	pending     io.Reader
	buf         []byte
	index       uint64
	srcDone     bool
}

func (r *encryptingReader) Read(p []byte) (int, error) {
	n, err := r.pending.Read(p)
	if !errors.Is(err, io.EOF) {
		return n, err
	}
	if r.srcDone {
		return n, io.EOF
	}

	// Pull the next plaintext chunk out of src
	plainLen, rerr := io.ReadFull(r.src, r.buf)
	last := false
	switch {
	case errors.Is(rerr, io.EOF):
		// src ended on a chunk boundary, so emit an empty final frame
		last = true
		plainLen = 0
	case errors.Is(rerr, io.ErrUnexpectedEOF):
		last = true
	case rerr != nil:
		return n, rerr
	}

	frame, err := r.sealChunk(r.buf[:plainLen], last)
	if err != nil {
		return n, err
	}
	if last {
		r.srcDone = true
	}
	r.index++
	r.pending = newBytesReader(frame)

	m, err := r.pending.Read(p[n:])
	return n + m, err
}

// sealChunk frames and seals one plaintext chunk as u32be(length) || flag || ciphertext, with the index and flag authenticated
func (r *encryptingReader) sealChunk(plain []byte, last bool) ([]byte, error) {
	var flag byte
	if last {
		flag = 1
	}

	ct := r.aead.Seal(nil, chunkNonce(r.noncePrefix, r.index), plain, chunkAAD(r.index, flag))

	frame := make([]byte, frameHeaderSize+len(ct))
	binary.BigEndian.PutUint32(frame, uint32(len(ct))) //nolint:gosec // len(ct) is bounded by chunkPlainSize plus the tag
	frame[4] = flag
	copy(frame[frameHeaderSize:], ct)
	return frame, nil
}

// NewDecryptingReader wraps src so the bytes read from it are the decrypted contents of an encrypted backup stream
// The stream header is read eagerly so a wrong magic or an unreadable header fails here rather than mid-stream
func NewDecryptingReader(src io.Reader, passphrase []byte) (io.Reader, error) {
	if len(passphrase) == 0 {
		return nil, errors.New("decryption passphrase is empty")
	}

	header := make([]byte, len(magicHeader)+saltSize+noncePrefixSize)
	if _, err := io.ReadFull(src, header); err != nil {
		if errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF) {
			return nil, ErrNotEncrypted
		}
		return nil, err
	}
	if string(header[:len(magicHeader)]) != magicHeader {
		return nil, ErrNotEncrypted
	}

	salt := header[len(magicHeader) : len(magicHeader)+saltSize]
	key, err := deriveKey(passphrase, salt)
	if err != nil {
		return nil, fmt.Errorf("failed to derive decryption key: %w", err)
	}

	aead, err := newAEAD(key)
	if err != nil {
		return nil, err
	}

	return &decryptingReader{
		src:         src,
		aead:        aead,
		noncePrefix: header[len(magicHeader)+saltSize:],
	}, nil
}

// decryptingReader decrypts the framed chunks of src on every Read
type decryptingReader struct {
	src         io.Reader
	aead        cipher.AEAD
	noncePrefix []byte
	pending     io.Reader
	index       uint64
	done        bool
}

func (r *decryptingReader) Read(p []byte) (int, error) {
	if r.pending != nil {
		n, err := r.pending.Read(p)
		if !errors.Is(err, io.EOF) {
			return n, err
		}
		r.pending = nil
		if n > 0 {
			return n, nil
		}
	}
	if r.done {
		return 0, io.EOF
	}

	plain, err := r.openChunk()
	if err != nil {
		return 0, err
	}
	r.index++
	r.pending = newBytesReader(plain)

	return r.pending.Read(p)
}

// openChunk reads and unseals the next frame, requiring the final flag before the stream may end
func (r *decryptingReader) openChunk() ([]byte, error) {
	frameHeader := make([]byte, frameHeaderSize)
	_, err := io.ReadFull(r.src, frameHeader)
	if errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF) {
		return nil, ErrTruncated
	}
	if err != nil {
		return nil, err
	}

	ctLen := binary.BigEndian.Uint32(frameHeader)
	if ctLen == 0 || ctLen > maxFrameSize {
		return nil, fmt.Errorf("invalid chunk length %d in encrypted archive", ctLen)
	}
	flag := frameHeader[4]
	if flag > 1 {
		return nil, fmt.Errorf("invalid chunk flag %d in encrypted archive", flag)
	}

	ct := make([]byte, ctLen)
	if _, err := io.ReadFull(r.src, ct); err != nil {
		if errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF) {
			return nil, ErrTruncated
		}
		return nil, err
	}

	plain, err := r.aead.Open(nil, chunkNonce(r.noncePrefix, r.index), ct, chunkAAD(r.index, flag))
	if err != nil {
		return nil, errors.New("failed to decrypt archive: the passphrase is wrong or the data is corrupted")
	}

	if flag == 1 {
		r.done = true
	}
	return plain, nil
}
