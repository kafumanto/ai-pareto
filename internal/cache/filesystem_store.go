//go:build !js

package cache

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"io"
	"os"
	"path/filepath"
	"time"
)

const (
	// fileMagic identifies the native cache entry format.
	fileMagic = "AIPC"
	// fileVersion identifies the current native cache entry format version.
	fileVersion byte = 1
	// fileHeaderSize is the fixed byte count before one encoded response body.
	fileHeaderSize = len(fileMagic) + 1 + 8 + 8 + sha256.Size
)

// FilesystemStore persists cache entries as private files in one native directory.
//
// Keys remain opaque to the store. The store hashes each key for a safe fixed
// filename and treats missing or malformed files as cache misses.
type FilesystemStore struct {
	// directory is the native cache directory selected by the caller.
	directory string
}

// NewFilesystemStore returns a native filesystem store rooted at directory.
//
// The directory is created lazily by Put. Get does not create directories or
// files, so a missing cache starts as an ordinary miss.
func NewFilesystemStore(directory string) *FilesystemStore {
	return &FilesystemStore{directory: directory}
}

// Get retrieves one complete entry from the hashed key path.
//
// A missing or malformed file returns found=false with no error. Other
// filesystem failures are returned so cache policy can produce a warning.
func (store *FilesystemStore) Get(ctx context.Context, key string) (Entry, bool, error) {
	// Stop before filesystem work when the request is already cancelled.
	if err := ctx.Err(); err != nil {
		return Entry{}, false, err
	}
	data, err := os.ReadFile(store.pathForKey(key))
	if err != nil {
		// A missing destination is the normal empty-cache result.
		if os.IsNotExist(err) {
			return Entry{}, false, nil
		}
		return Entry{}, false, err
	}
	// Preserve cancellation that occurs while the operating system reads the file.
	if err := ctx.Err(); err != nil {
		return Entry{}, false, err
	}
	entry, valid := decodeEntry(data)
	// Treat every format failure as a silent miss so policy can fetch a fresh body.
	if !valid {
		return Entry{}, false, nil
	}
	return entry, true, nil
}

// Put atomically replaces the entry for key with a complete private file.
//
// The temporary file is created beside the destination, closed before rename,
// and removed after every failed write path so readers never observe partial data.
func (store *FilesystemStore) Put(ctx context.Context, key string, entry Entry) error {
	// Stop before creating directories or files when the request is already cancelled.
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := os.MkdirAll(store.directory, 0700); err != nil {
		return err
	}
	// Preserve cancellation after directory creation and before opening a temporary file.
	if err := ctx.Err(); err != nil {
		return err
	}

	temporary, err := os.CreateTemp(store.directory, ".ai-pareto-cache-*")
	if err != nil {
		return err
	}
	temporaryPath := temporary.Name()
	removeTemporary := true
	defer func() {
		// Remove the same-directory temporary file after every path that did not rename it.
		if removeTemporary {
			_ = os.Remove(temporaryPath)
		}
	}()

	// Serialize the complete entry before writing so the destination remains untouched on encoding failure.
	data := encodeEntry(entry)
	written, err := temporary.Write(data)
	if err != nil {
		// Close the temporary descriptor before the deferred cleanup removes its path.
		_ = temporary.Close()
		return err
	}
	// Reject a short write so a partial temporary file cannot be renamed.
	if written != len(data) {
		_ = temporary.Close()
		return io.ErrShortWrite
	}
	// Close before rename so readers can open only a complete file.
	if err := temporary.Close(); err != nil {
		return err
	}
	// Do not publish a completed entry after cancellation between close and rename.
	if err := ctx.Err(); err != nil {
		return err
	}
	// Replace the destination only after the temporary file is complete and closed.
	if err := os.Rename(temporaryPath, store.pathForKey(key)); err != nil {
		return err
	}
	removeTemporary = false
	// Report cancellation that occurs immediately after publication.
	if err := ctx.Err(); err != nil {
		return err
	}
	return nil
}

// pathForKey maps an opaque key to a safe fixed-width native filename.
func (store *FilesystemStore) pathForKey(key string) string {
	// Hash the opaque key so path separators and other key content cannot escape the directory.
	digest := sha256.Sum256([]byte(key))
	return filepath.Join(store.directory, hex.EncodeToString(digest[:])+".entry")
}

// encodeEntry serializes one entry with its timestamp, length, checksum, and body.
func encodeEntry(entry Entry) []byte {
	bodyDigest := sha256.Sum256(entry.Body)
	// Allocate one fixed header followed by the exact body length.
	encoded := make([]byte, fileHeaderSize+len(entry.Body))
	copy(encoded, fileMagic)
	encoded[len(fileMagic)] = fileVersion
	offset := len(fileMagic) + 1
	// Store timestamps as UTC Unix nanoseconds and lengths as fixed-width integers.
	binary.BigEndian.PutUint64(encoded[offset:offset+8], uint64(entry.CreatedAt.UTC().UnixNano()))
	offset += 8
	binary.BigEndian.PutUint64(encoded[offset:offset+8], uint64(len(entry.Body)))
	offset += 8
	copy(encoded[offset:offset+sha256.Size], bodyDigest[:])
	offset += sha256.Size
	// Append the unmodified body after its integrity metadata.
	copy(encoded[offset:], entry.Body)
	return encoded
}

// decodeEntry validates and decodes one complete native cache file.
func decodeEntry(data []byte) (Entry, bool) {
	// Reject data that cannot contain the complete fixed header.
	if len(data) < fileHeaderSize {
		return Entry{}, false
	}
	// Validate the magic and version before interpreting numeric fields.
	if !bytes.Equal(data[:len(fileMagic)], []byte(fileMagic)) {
		return Entry{}, false
	}
	if data[len(fileMagic)] != fileVersion {
		return Entry{}, false
	}
	// Decode the timestamp, declared body length, and stored digest in header order.
	offset := len(fileMagic) + 1
	createdAt := int64(binary.BigEndian.Uint64(data[offset : offset+8]))
	offset += 8
	bodyLength := binary.BigEndian.Uint64(data[offset : offset+8])
	offset += 8
	storedDigest := data[offset : offset+sha256.Size]
	offset += sha256.Size
	// Require the declared body length to consume the file exactly, with no trailing bytes.
	if bodyLength != uint64(len(data)-offset) {
		return Entry{}, false
	}
	body := data[offset:]
	actualDigest := sha256.Sum256(body)
	// Reject any body whose bytes differ from the stored checksum.
	if !bytes.Equal(storedDigest, actualDigest[:]) {
		return Entry{}, false
	}
	return Entry{
		CreatedAt: time.Unix(0, createdAt).UTC(),
		Body:      append([]byte(nil), body...),
	}, true
}

// ensureFilesystemStoreContract keeps the native implementation tied to the portable Store interface.
var _ Store = (*FilesystemStore)(nil)
