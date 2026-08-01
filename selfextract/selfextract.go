// Package selfextract is the self-extracting installer format shared by the
// packer (build time) and the setup binary (install time). `make package`
// appends a payload — the app binary + its sibling assets — to a copy of the
// setup exe, followed by a fixed-size trailer. The result is one file that
// carries everything it needs; at install time the setup exe reads its OWN file,
// finds the trailer, and writes the bundled files out.
//
// The format descends from the worker's MWPLOAD family (payload.hpp /
// tools/pack.cpp) so a Go setup binary (MASS, Grimoire) gets the same
// single-file installer the worker has. Reader and writer share the constants
// here so the two can't drift.
//
// Payload layout, appended after the host exe's normal end-of-file:
//
//	[ record 0 ][ record 1 ] ... [ record N-1 ][ trailer ]
//
// record (little-endian):
//
//	uint32 name_len | name bytes (utf-8 leaf, no NUL)
//	uint64 raw_size | uint64 comp_size | comp_size bytes (one gzip stream)
//
// trailer (TrailerSize bytes, little-endian, at the very end of the file):
//
//	uint64 payload_size  (bytes from the first record up to but excluding trailer)
//	uint64 file_count
//	byte   magic[8]
//
// Records hold gzip streams because these installers are fetched over the
// network: deflate at level 9 more than halves a Go binary (MASS 47.8 MB ->
// 20.8 MB, Grimoire 39.4 MB -> 14.9 MB). zstd compresses those ~7% smaller, but
// its decoder costs ~800 KB more in the setup stub than compress/gzip does, so
// the shipped installer only shrinks ~2% — not enough to earn a dependency. One
// stream per record rather than one over the whole payload keeps records
// seekable and errors attributable to a single file. Both sides stream through
// fixed buffers, so peak memory does not scale with the payload.
package selfextract

import (
	"bufio"
	"compress/gzip"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

// magic = a 7-byte family prefix + a one-byte format generation. The prefix
// answers "does this file carry a payload at all"; the generation says which
// layout, so a stale packer/reader pair fails with a versioned error instead of
// "no payload" (a silent fall back to copying siblings) or garbage records.
// Generation 1 was the uncompressed layout; 2 belongs to the worker's C++
// compressed layout, whose extra record kinds a single-binary Go payload has no
// use for — skipped so no magic ever names two different layouts.
const (
	magicPrefix = "MWPLOAD"
	magic       = magicPrefix + "3"
)

// TrailerSize is the fixed trailer length at the very end of a packaged file:
// payload_size(8) + file_count(8) + magic(8).
const TrailerSize = 8 + 8 + 8

// bufSize is the fixed I/O buffer both halves stream through.
const bufSize = 128 << 10

// ErrNoPayload is returned (wrapped) when the file carries no appended payload —
// a plain setup exe rather than a package. Callers treat this as "not packaged,
// fall back to copying siblings".
var ErrNoPayload = errors.New("selfextract: no appended payload")

// ErrFormatVersion is returned (wrapped) when the file carries a payload written
// by a different format generation. Distinct from ErrNoPayload: there IS a
// payload, this build just can't read it, so the caller must fail rather than
// quietly provision some other way.
var ErrFormatVersion = errors.New("selfextract: unsupported payload format")

// trailer is the parsed tail of a packaged file.
type trailer struct {
	payloadSize uint64
	fileCount   uint64
	fileSize    int64
}

// readTrailer parses the trailer at the end of the open file f (whose total size
// is fileSize). Returns ErrNoPayload when the magic is absent or the file is too
// small / inconsistent, ErrFormatVersion on a generation mismatch.
func readTrailer(f io.ReaderAt, fileSize int64) (trailer, error) {
	if fileSize < TrailerSize {
		return trailer{}, ErrNoPayload
	}
	buf := make([]byte, TrailerSize)
	if _, err := f.ReadAt(buf, fileSize-TrailerSize); err != nil {
		return trailer{}, ErrNoPayload
	}
	got := string(buf[16:24])
	if !strings.HasPrefix(got, magicPrefix) {
		return trailer{}, ErrNoPayload // a plain exe, not a package
	}
	if got != magic {
		return trailer{}, fmt.Errorf("%w: packed as %q, this build reads %q", ErrFormatVersion, got, magic)
	}
	t := trailer{
		payloadSize: binary.LittleEndian.Uint64(buf[0:8]),
		fileCount:   binary.LittleEndian.Uint64(buf[8:16]),
		fileSize:    fileSize,
	}
	// Sanity: the payload + trailer must fit inside the file.
	if t.payloadSize+TrailerSize > uint64(fileSize) {
		return trailer{}, ErrNoPayload
	}
	return t, nil
}

// HasPayload reports whether the file at path carries an appended payload. A
// payload this build can't read (wrong generation) still counts as one, so the
// caller takes the extraction path and gets that error instead of silently
// provisioning another way.
func HasPayload(path string) bool {
	f, err := os.Open(path)
	if err != nil {
		return false
	}
	defer f.Close() //nolint:errcheck // read-only probe
	info, err := f.Stat()
	if err != nil {
		return false
	}
	_, err = readTrailer(f, info.Size())
	return err == nil || errors.Is(err, ErrFormatVersion)
}

// SelfHasPayload reports whether THIS running executable is a packaged
// installer.
func SelfHasPayload() bool {
	exe, err := os.Executable()
	if err != nil {
		return false
	}
	return HasPayload(exe)
}

// ProgressFn is an optional extraction progress sink, called after each file is
// written with (done, total); done runs 1..total.
type ProgressFn func(done, total int)

// ExtractSelf extracts this running executable's appended payload into dstDir
// (created if needed; files overwrite), returning the number of files written.
// Returns ErrNoPayload (wrapped) when this binary isn't a package — the caller
// then provisions some other way (e.g. copying siblings).
func ExtractSelf(dstDir string, progress ProgressFn) (int, error) {
	exe, err := os.Executable()
	if err != nil {
		return 0, fmt.Errorf("selfextract: locating own path: %w", err)
	}
	return ExtractFrom(exe, dstDir, progress)
}

// ExtractFrom extracts the payload appended to the file at srcPath into dstDir.
// Mostly used via ExtractSelf; exposed for testing.
func ExtractFrom(srcPath, dstDir string, progress ProgressFn) (int, error) {
	f, err := os.Open(srcPath)
	if err != nil {
		return 0, fmt.Errorf("selfextract: opening %s: %w", srcPath, err)
	}
	defer f.Close() //nolint:errcheck // read-only source
	info, err := f.Stat()
	if err != nil {
		return 0, fmt.Errorf("selfextract: stat %s: %w", srcPath, err)
	}
	t, err := readTrailer(f, info.Size())
	if err != nil {
		return 0, err // ErrNoPayload / ErrFormatVersion, or a wrap
	}

	if err := os.MkdirAll(dstDir, 0o755); err != nil {
		return 0, fmt.Errorf("selfextract: creating %s: %w", dstDir, err)
	}

	// The payload begins payloadSize+trailer bytes before EOF.
	off := t.fileSize - TrailerSize - int64(t.payloadSize)
	end := t.fileSize - TrailerSize

	written := 0
	for i := uint64(0); i < t.fileCount; i++ {
		e, err := readEntryHeader(f, off, end)
		if err != nil {
			return written, err
		}
		if err := extractEntry(f, e, dstDir); err != nil {
			return written, err
		}
		off = e.dataOff + int64(e.compSize)
		written++
		if progress != nil {
			progress(written, int(t.fileCount))
		}
	}
	return written, nil
}

// entry is one parsed payload record header.
type entry struct {
	name     string
	rawSize  uint64
	compSize uint64
	dataOff  int64 // offset of the gzip stream in the packaged file
}

// readEntryHeader reads one record's [name_len][name][raw_size][comp_size]
// starting at off. end is the exclusive end of the payload region (start of the
// trailer).
func readEntryHeader(f io.ReaderAt, off, end int64) (entry, error) {
	var u32 [4]byte
	if off+4 > end {
		return entry{}, errTruncated("file name length")
	}
	if _, err := f.ReadAt(u32[:], off); err != nil {
		return entry{}, errTruncated("file name length")
	}
	nameLen := int64(binary.LittleEndian.Uint32(u32[:]))
	off += 4

	if off+nameLen > end {
		return entry{}, errTruncated("file name")
	}
	nameBuf := make([]byte, nameLen)
	if _, err := f.ReadAt(nameBuf, off); err != nil {
		return entry{}, errTruncated("file name")
	}
	off += nameLen

	var sizes [16]byte
	if off+16 > end {
		return entry{}, errTruncated("file sizes")
	}
	if _, err := f.ReadAt(sizes[:], off); err != nil {
		return entry{}, errTruncated("file sizes")
	}
	e := entry{
		name:     string(nameBuf),
		rawSize:  binary.LittleEndian.Uint64(sizes[0:8]),
		compSize: binary.LittleEndian.Uint64(sizes[8:16]),
		dataOff:  off + 16,
	}
	// Compared as uint64 against the remaining span so a corrupt (huge) comp_size
	// can't overflow the int64 sum into a passing check.
	if e.compSize > uint64(end-e.dataOff) {
		return entry{}, errTruncated("file data")
	}
	return e, nil
}

// extractEntry decompresses e's gzip stream out to dstDir/name. The name must be
// a flat leaf — a separator or ':' (Windows ADS) is rejected. The bytes land in a
// sibling temp file that is renamed into place only once the stream decodes
// whole, so a corrupt payload never leaves a truncated binary behind under the
// real name.
func extractEntry(f io.ReaderAt, e entry, dstDir string) (err error) {
	if strings.ContainsAny(e.name, `/\:`) {
		return fmt.Errorf("selfextract: unsafe payload entry name: %q", e.name)
	}
	dst := filepath.Join(dstDir, e.name)
	tmp := filepath.Join(dstDir, "."+e.name+".part")

	// 0755: the payload is the app executable + shared libraries, both of which
	// need the execute bit (a non-executable binary fails to launch). On Windows
	// perms are ACL-governed so the mode is a best-effort no-op.
	out, err := os.OpenFile(tmp, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o755)
	if err != nil {
		return fmt.Errorf("selfextract: opening %s: %w", tmp, err)
	}
	defer func() {
		if err != nil {
			_ = out.Close()
			_ = os.Remove(tmp) // best-effort: leave nothing half-written behind
		}
	}()

	src := bufio.NewReaderSize(io.NewSectionReader(f, e.dataOff, int64(e.compSize)), bufSize)
	zr, err := gzip.NewReader(src)
	if err != nil {
		return fmt.Errorf("selfextract: reading %s: %w", e.name, err)
	}
	zr.Multistream(false) // exactly one stream per record
	n, err := io.Copy(out, zr)
	if err != nil {
		return fmt.Errorf("selfextract: extracting %s: %w", e.name, err)
	}
	if err = zr.Close(); err != nil {
		return fmt.Errorf("selfextract: extracting %s: %w", e.name, err)
	}
	if uint64(n) != e.rawSize {
		return fmt.Errorf("selfextract: %s: expected %d bytes, decompressed %d", e.name, e.rawSize, n)
	}
	// Re-assert the mode: O_CREATE's 0755 is filtered by the process umask (no-op
	// on Windows).
	if runtime.GOOS != "windows" {
		if err = os.Chmod(tmp, 0o755); err != nil {
			return fmt.Errorf("selfextract: setting mode on %s: %w", e.name, err)
		}
	}
	if err = out.Close(); err != nil {
		return fmt.Errorf("selfextract: closing %s: %w", tmp, err)
	}
	if err = os.Rename(tmp, dst); err != nil {
		return fmt.Errorf("selfextract: writing %s: %w", dst, err)
	}
	return nil
}

func errTruncated(what string) error {
	return fmt.Errorf("selfextract: truncated payload (%s)", what)
}
