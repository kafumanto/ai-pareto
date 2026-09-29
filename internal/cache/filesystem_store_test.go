//go:build !js

package cache

import (
	"bytes"
	"context"
	"encoding/binary"
	"os"
	"runtime"
	"strings"
	"testing"
	"time"
)

// testStoreConformance exercises the portable Store behavior shared by native and memory stores.
// Expected: missing keys miss, arbitrary bytes and creation times survive storage, and replacement returns the newest entry.
// [SPEC] openspec/changes/implement-raw-http-cache/specs/raw-http-cache/spec.md, heading "Requirement: Native filesystem store".
func testStoreConformance(t *testing.T, newStore func(*testing.T) Store) {
	t.Helper()
	store := newStore(t)
	ctx := context.Background()
	key := "opaque/key/with?delimiters"

	// A new store must report a missing key without exposing an entry or error.
	missing, found, err := store.Get(ctx, key)
	if err != nil {
		t.Fatalf("initial Get() error = %v", err)
	}
	if found || !missing.CreatedAt.IsZero() || missing.Body != nil {
		t.Fatalf("initial Get() = %#v, %t, want zero entry and miss", missing, found)
	}

	// The first write must preserve both the exact body bytes and creation timestamp.
	first := Entry{
		CreatedAt: time.Date(2026, time.September, 29, 12, 34, 56, 789, time.FixedZone("test", 3600)),
		Body:      []byte{0, 1, 2, 255},
	}
	if err := store.Put(ctx, key, first); err != nil {
		t.Fatalf("first Put() error = %v", err)
	}
	got, found, err := store.Get(ctx, key)
	if err != nil {
		t.Fatalf("first Get() error = %v", err)
	}
	if !found || !got.CreatedAt.Equal(first.CreatedAt) || !bytes.Equal(got.Body, first.Body) {
		t.Fatalf("first Get() = %#v, %t, want %#v, true", got, found, first)
	}

	// A second write must replace the prior complete value for the same opaque key.
	replacement := Entry{
		CreatedAt: first.CreatedAt.Add(time.Hour),
		Body:      []byte("replacement"),
	}
	if err := store.Put(ctx, key, replacement); err != nil {
		t.Fatalf("replacement Put() error = %v", err)
	}
	got, found, err = store.Get(ctx, key)
	if err != nil {
		t.Fatalf("replacement Get() error = %v", err)
	}
	if !found || !got.CreatedAt.Equal(replacement.CreatedAt) || !bytes.Equal(got.Body, replacement.Body) {
		t.Fatalf("replacement Get() = %#v, %t, want %#v, true", got, found, replacement)
	}
}

// TestFilesystemStoreConforms verifies the native store against the reusable Store contract.
// Expected: the filesystem store preserves missing, arbitrary-byte, timestamp, and replacement behavior.
// [SPEC] openspec/changes/implement-raw-http-cache/specs/raw-http-cache/spec.md, heading "Requirement: Native filesystem store".
func TestFilesystemStoreConforms(t *testing.T) {
	testStoreConformance(t, func(t *testing.T) Store {
		return NewFilesystemStore(t.TempDir())
	})
}

// TestMemoryStoreConforms verifies the same conformance helper against the portable test store.
// Expected: the helper remains reusable for another Store implementation without changing its assertions.
// [NO_SPEC] Protects the reusable storage test seam for the deferred memory-store implementation.
func TestMemoryStoreConforms(t *testing.T) {
	testStoreConformance(t, func(*testing.T) Store {
		return newMemoryStore()
	})
}

// TestFilesystemStoreTreatsMalformedEntriesAsMisses verifies strict native-format validation.
// Expected: truncated, versioned, length, checksum, and trailing-data failures return silent misses without partial entries.
// [SPEC] openspec/changes/implement-raw-http-cache/specs/raw-http-cache/spec.md, heading "Requirement: Native filesystem store".
func TestFilesystemStoreTreatsMalformedEntriesAsMisses(t *testing.T) {
	store := NewFilesystemStore(t.TempDir())
	if err := os.MkdirAll(store.directory, 0700); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	valid := encodeEntry(Entry{CreatedAt: time.Unix(100, 200).UTC(), Body: []byte("body")})
	cases := []struct {
		name string
		data []byte
	}{
		{name: "truncated", data: []byte("short")},
		{name: "wrong-magic", data: append([]byte("NOPE"), valid[len(fileMagic):]...)},
		{name: "wrong-version", data: func() []byte {
			data := append([]byte(nil), valid...)
			data[len(fileMagic)] = fileVersion + 1
			return data
		}()},
		{name: "wrong-length", data: func() []byte {
			data := append([]byte(nil), valid...)
			offset := len(fileMagic) + 1 + 8
			binary.BigEndian.PutUint64(data[offset:offset+8], 0)
			return data
		}()},
		{name: "wrong-checksum", data: func() []byte { data := append([]byte(nil), valid...); data[len(data)-1] ^= 1; return data }()},
		{name: "trailing-data", data: append(append([]byte(nil), valid...), 0)},
	}

	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			key := "corrupt-" + testCase.name
			if err := os.WriteFile(store.pathForKey(key), testCase.data, 0600); err != nil {
				t.Fatalf("WriteFile() error = %v", err)
			}
			entry, found, err := store.Get(context.Background(), key)
			if err != nil || found || !entry.CreatedAt.IsZero() || entry.Body != nil {
				t.Fatalf("Get() = %#v, %t, %v, want zero entry, miss, nil", entry, found, err)
			}
		})
	}
}

// TestFilesystemStoreRemovesTemporaryFileAfterRenameFailure verifies failed replacement cleanup.
// Expected: a failed destination rename leaves no temporary cache file in the destination directory.
// [SPEC] openspec/changes/implement-raw-http-cache/specs/raw-http-cache/spec.md, heading "Requirement: Native filesystem store".
func TestFilesystemStoreRemovesTemporaryFileAfterRenameFailure(t *testing.T) {
	store := NewFilesystemStore(t.TempDir())
	key := "rename-failure"
	if err := os.Mkdir(store.pathForKey(key), 0700); err != nil {
		t.Fatalf("Mkdir() error = %v", err)
	}
	if err := store.Put(context.Background(), key, Entry{Body: []byte("body")}); err == nil {
		t.Fatal("Put() error = nil for directory destination")
	}
	entries, err := os.ReadDir(store.directory)
	if err != nil {
		t.Fatalf("ReadDir() error = %v", err)
	}
	for _, entry := range entries {
		if strings.HasPrefix(entry.Name(), ".ai-pareto-cache-") {
			t.Fatalf("temporary file %q remains after failed Put()", entry.Name())
		}
	}
}

// TestFilesystemStoreUsesPrivatePermissions verifies native private modes where the platform exposes permission bits.
// Expected: created directories and entry files do not grant group or other permissions.
// [SPEC] openspec/changes/implement-raw-http-cache/specs/raw-http-cache/spec.md, heading "Requirement: Native filesystem store".
func TestFilesystemStoreUsesPrivatePermissions(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Windows does not expose Unix permission semantics")
	}
	store := NewFilesystemStore(t.TempDir() + "/nested")
	if err := store.Put(context.Background(), "private", Entry{Body: []byte("body")}); err != nil {
		t.Fatalf("Put() error = %v", err)
	}
	directoryInfo, err := os.Stat(store.directory)
	if err != nil {
		t.Fatalf("Stat(directory) error = %v", err)
	}
	if directoryInfo.Mode().Perm()&0077 != 0 {
		t.Fatalf("directory mode = %o, want no group or other permissions", directoryInfo.Mode().Perm())
	}
	fileInfo, err := os.Stat(store.pathForKey("private"))
	if err != nil {
		t.Fatalf("Stat(entry) error = %v", err)
	}
	if fileInfo.Mode().Perm()&0077 != 0 {
		t.Fatalf("entry mode = %o, want no group or other permissions", fileInfo.Mode().Perm())
	}
}
