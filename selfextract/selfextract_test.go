package selfextract

import (
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"io"
	"math/rand"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestPackExtractRoundTrip(t *testing.T) {
	// Payloads that exercise what the format has to survive: a compressible blob
	// the size of a real binary's order of magnitude, incompressible bytes, and an
	// empty file.
	tests := []struct {
		name  string
		files map[string][]byte
	}{
		{
			name:  "single binary",
			files: map[string][]byte{"app.bin": repeatingBytes(3 << 20)},
		},
		{
			name: "binary plus sibling libraries",
			files: map[string][]byte{
				"app.bin": repeatingBytes(1 << 20),
				"dep.lib": []byte("a shared library payload"),
				"other":   randomBytes(1 << 16), // incompressible: deflate must still round-trip
			},
		},
		{
			name:  "empty file",
			files: map[string][]byte{"app.bin": {}},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			host := filepath.Join(dir, "setup")
			require.NoError(t, os.WriteFile(host, []byte("HOST-STUB-BYTES"), 0o755))

			var payload []string
			for name, data := range tc.files {
				p := filepath.Join(dir, name)
				require.NoError(t, os.WriteFile(p, data, 0o644))
				payload = append(payload, p)
			}

			out := filepath.Join(dir, "packaged")
			require.NoError(t, Pack(host, out, payload))

			// The packaged file starts with the host bytes (host stub preserved).
			packed, err := os.Open(out)
			require.NoError(t, err)
			head := make([]byte, len("HOST-STUB-BYTES"))
			_, err = io.ReadFull(packed, head)
			require.NoError(t, err)
			require.NoError(t, packed.Close())
			require.Equal(t, "HOST-STUB-BYTES", string(head))

			require.True(t, HasPayload(out))
			require.False(t, HasPayload(host)) // plain stub has no trailer

			dst := filepath.Join(dir, "extracted")
			var progressCalls int
			n, err := ExtractFrom(out, dst, func(done, total int) {
				progressCalls++
				require.Equal(t, len(tc.files), total)
				require.Equal(t, progressCalls, done)
			})
			require.NoError(t, err)
			require.Equal(t, len(tc.files), n)
			require.Equal(t, len(tc.files), progressCalls)

			entries, err := os.ReadDir(dst)
			require.NoError(t, err)
			require.Len(t, entries, len(tc.files), "no temp files left beside the extracted payload")

			for name, want := range tc.files {
				got := filepath.Join(dst, name)
				require.Equal(t, digest(t, want), fileDigest(t, got), "%s round-tripped byte-for-byte", name)
				info, err := os.Stat(got)
				require.NoError(t, err)
				require.Equal(t, int64(len(want)), info.Size())
				if runtime.GOOS != "windows" {
					require.NotZero(t, info.Mode()&0o111, "%s keeps the execute bit", name)
				}
			}
		})
	}
}

func TestPackCompresses(t *testing.T) {
	dir := t.TempDir()
	host := filepath.Join(dir, "setup")
	require.NoError(t, os.WriteFile(host, []byte("HOST"), 0o755))
	bin := filepath.Join(dir, "app.bin")
	data := repeatingBytes(4 << 20)
	require.NoError(t, os.WriteFile(bin, data, 0o644))

	out := filepath.Join(dir, "packaged")
	require.NoError(t, Pack(host, out, []string{bin}))

	info, err := os.Stat(out)
	require.NoError(t, err)
	require.Less(t, info.Size(), int64(len(data)),
		"the packaged installer must be smaller than the raw payload it carries")
}

func TestExtractNoPayload(t *testing.T) {
	dir := t.TempDir()
	plain := filepath.Join(dir, "plain")
	require.NoError(t, os.WriteFile(plain, []byte("not a package"), 0o644))

	_, err := ExtractFrom(plain, filepath.Join(dir, "out"), nil)
	require.ErrorIs(t, err, ErrNoPayload)
	require.False(t, HasPayload(plain))
}

// A package written by another format generation must fail loudly rather than
// read as "no payload" (which would silently fall back to copying siblings).
func TestExtractRejectsForeignGeneration(t *testing.T) {
	dir := t.TempDir()
	out := packOne(t, dir, "app.bin", []byte("payload"))

	// Rewrite the trailer's generation byte in place; the family prefix stays.
	f, err := os.OpenFile(out, os.O_RDWR, 0)
	require.NoError(t, err)
	info, err := f.Stat()
	require.NoError(t, err)
	_, err = f.WriteAt([]byte("1"), info.Size()-1)
	require.NoError(t, err)
	require.NoError(t, f.Close())

	require.True(t, HasPayload(out), "a payload this build can't read is still a payload")

	dst := filepath.Join(dir, "dst")
	_, err = ExtractFrom(out, dst, nil)
	require.ErrorIs(t, err, ErrFormatVersion)
	require.Contains(t, err.Error(), "MWPLOAD1")
	require.NoDirExists(t, dst)
}

// A payload whose compressed stream is corrupt must error and leave nothing
// behind — a half-written app binary would install and then fail to launch.
func TestExtractCorruptStreamWritesNothing(t *testing.T) {
	for _, tc := range []struct {
		name    string
		corrupt func(t *testing.T, path string, size int64)
	}{
		{
			name: "flipped byte mid-stream",
			corrupt: func(t *testing.T, path string, size int64) {
				f, err := os.OpenFile(path, os.O_RDWR, 0)
				require.NoError(t, err)
				var b [1]byte
				off := size - TrailerSize - 32
				_, err = f.ReadAt(b[:], off)
				require.NoError(t, err)
				b[0] ^= 0xff
				_, err = f.WriteAt(b[:], off)
				require.NoError(t, err)
				require.NoError(t, f.Close())
			},
		},
		{
			name: "stream truncated",
			corrupt: func(t *testing.T, path string, size int64) {
				// Lop the tail off the gzip stream and shrink payload_size to match, so
				// the trailer stays self-consistent and only the record's declared
				// comp_size is left overhanging the end of the payload.
				raw, err := os.ReadFile(path)
				require.NoError(t, err)
				tr := append([]byte(nil), raw[len(raw)-TrailerSize:]...)
				const drop = 64
				binary.LittleEndian.PutUint64(tr[0:8], binary.LittleEndian.Uint64(tr[0:8])-drop)
				body := append([]byte(nil), raw[:len(raw)-TrailerSize-drop]...)
				require.NoError(t, os.WriteFile(path, append(body, tr...), 0o755))
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			out := packOne(t, dir, "app.bin", repeatingBytes(256<<10))
			info, err := os.Stat(out)
			require.NoError(t, err)
			tc.corrupt(t, out, info.Size())

			dst := filepath.Join(dir, "dst")
			n, err := ExtractFrom(out, dst, nil)
			require.Error(t, err)
			require.Zero(t, n)
			entries, err := os.ReadDir(dst)
			require.NoError(t, err)
			require.Empty(t, entries, "a corrupt payload must write nothing, not even a partial file")
		})
	}
}

func TestPackDuplicateLeafRejected(t *testing.T) {
	dir := t.TempDir()
	host := filepath.Join(dir, "setup")
	require.NoError(t, os.WriteFile(host, []byte("h"), 0o755))

	// Two different paths with the same basename.
	sub := filepath.Join(dir, "a")
	require.NoError(t, os.MkdirAll(sub, 0o755))
	f1 := filepath.Join(dir, "lib.so")
	f2 := filepath.Join(sub, "lib.so")
	require.NoError(t, os.WriteFile(f1, []byte("1"), 0o644))
	require.NoError(t, os.WriteFile(f2, []byte("2"), 0o644))

	err := Pack(host, filepath.Join(dir, "out"), []string{f1, f2})
	require.Error(t, err)
	require.Contains(t, err.Error(), "duplicate")
}

func TestPackEmptyPayloadRejected(t *testing.T) {
	dir := t.TempDir()
	host := filepath.Join(dir, "setup")
	require.NoError(t, os.WriteFile(host, []byte("h"), 0o755))
	err := Pack(host, filepath.Join(dir, "out"), nil)
	require.Error(t, err)
}

func TestExtractRejectsUnsafeName(t *testing.T) {
	// Hand-craft a package whose single entry name contains a path separator, to
	// confirm extraction refuses it (path-traversal / ADS guard).
	dir := t.TempDir()
	host := filepath.Join(dir, "setup")
	require.NoError(t, os.WriteFile(host, []byte("HOST"), 0o755))

	out := filepath.Join(dir, "evil")
	writeEvilPackage(t, host, out, "../escape", []byte("x"))

	require.True(t, HasPayload(out))
	_, err := ExtractFrom(out, filepath.Join(dir, "dst"), nil)
	require.Error(t, err)
	require.Contains(t, err.Error(), "unsafe payload entry name")
}

// A trailer claiming more records than the payload holds must be reported, not
// read past.
func TestExtractRejectsTruncatedRecord(t *testing.T) {
	dir := t.TempDir()
	out := packOne(t, dir, "app.bin", []byte("payload"))

	f, err := os.OpenFile(out, os.O_RDWR, 0)
	require.NoError(t, err)
	info, err := f.Stat()
	require.NoError(t, err)
	var count [8]byte
	binary.LittleEndian.PutUint64(count[:], 2) // one more record than exists
	_, err = f.WriteAt(count[:], info.Size()-TrailerSize+8)
	require.NoError(t, err)
	require.NoError(t, f.Close())

	dst := filepath.Join(dir, "dst")
	n, err := ExtractFrom(out, dst, nil)
	require.Error(t, err)
	require.Contains(t, err.Error(), "truncated payload")
	require.Equal(t, 1, n, "the intact record is reported as written")
}

// packOne builds a one-file package under dir and returns its path.
func packOne(t *testing.T, dir, name string, data []byte) string {
	t.Helper()
	host := filepath.Join(dir, "setup")
	require.NoError(t, os.WriteFile(host, []byte("HOST-STUB-BYTES"), 0o755))
	src := filepath.Join(dir, "src", name)
	require.NoError(t, os.MkdirAll(filepath.Dir(src), 0o755))
	require.NoError(t, os.WriteFile(src, data, 0o644))

	out := filepath.Join(dir, "packaged")
	require.NoError(t, Pack(host, out, []string{src}))
	return out
}

// writeEvilPackage builds a one-entry package with an arbitrary (possibly
// unsafe) entry name, bypassing Pack's basename-only behaviour, to exercise the
// reader's guard.
func writeEvilPackage(t *testing.T, host, out, name string, data []byte) {
	t.Helper()
	hostBytes, err := os.ReadFile(host)
	require.NoError(t, err)

	f, err := os.OpenFile(out, os.O_RDWR|os.O_CREATE|os.O_TRUNC, 0o755)
	require.NoError(t, err)
	defer f.Close() //nolint:errcheck // test helper

	_, err = f.Write(hostBytes)
	require.NoError(t, err)

	n, err := writeEntry(f, name, mustTempFile(t, data))
	require.NoError(t, err)
	require.NoError(t, writeTrailer(f, n, 1))
}

func mustTempFile(t *testing.T, data []byte) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "data")
	require.NoError(t, os.WriteFile(p, data, 0o644))
	return p
}

// repeatingBytes returns compressible filler resembling a binary: long runs plus
// a repeating pattern.
func repeatingBytes(n int) []byte {
	b := make([]byte, n)
	for i := range b {
		b[i] = byte(i / 97 % 251)
	}
	return b
}

// randomBytes returns incompressible filler from a fixed seed (deterministic).
func randomBytes(n int) []byte {
	b := make([]byte, n)
	r := rand.New(rand.NewSource(1)) //nolint:gosec // test fixture, not cryptography
	_, _ = r.Read(b)
	return b
}

func digest(t *testing.T, b []byte) string {
	t.Helper()
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}

func fileDigest(t *testing.T, path string) string {
	t.Helper()
	f, err := os.Open(path)
	require.NoError(t, err)
	defer f.Close() //nolint:errcheck // read-only
	h := sha256.New()
	_, err = io.Copy(h, f)
	require.NoError(t, err)
	return hex.EncodeToString(h.Sum(nil))
}
