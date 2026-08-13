// Package fsutil holds small filesystem helpers shared across MASS apps.
package fsutil

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

// WriteFileAtomic writes data to path via a temp file in the same directory then
// renames it into place, so a concurrent reader (or a crash) never observes a
// half-written file. The temp file gets perm before the rename. The parent
// directory must already exist.
func WriteFileAtomic(path string, data []byte, perm os.FileMode) error {
	_, err := WriteAtomic(path, bytes.NewReader(data), perm, nil)
	return err
}

// WriteAtomic streams src into path the same way, returning the number of bytes
// written. Replacing an existing file goes through the rename rather than a
// truncating open, which is what makes it safe over a live install: truncating a
// running binary is ETXTBSY on Linux, corrupts the running image on macOS, and
// is refused outright on Windows. The rename itself still fails on Windows while
// another process holds the destination open — callers poll until it doesn't.
//
// verify, when non-nil, runs on the written byte count after the copy but before
// the rename; its error aborts the write, leaving path untouched. That is where
// a caller checks a stream's integrity (a checksum, a declared size) so a
// corrupt source never lands under the real name.
func WriteAtomic(path string, src io.Reader, perm os.FileMode, verify func(written int64) error) (written int64, err error) {
	tmp, err := os.CreateTemp(filepath.Dir(path), "."+filepath.Base(path)+".*.tmp")
	if err != nil {
		return 0, fmt.Errorf("creating temp file: %w", err)
	}
	tmpName := tmp.Name()
	defer func() {
		if err != nil {
			_ = tmp.Close()       // best-effort: the failure is already being reported
			_ = os.Remove(tmpName) // best-effort cleanup on any failure.
		}
	}()

	if written, err = io.Copy(tmp, src); err != nil {
		return written, fmt.Errorf("writing temp file: %w", err)
	}
	if verify != nil {
		if err = verify(written); err != nil {
			return written, err
		}
	}
	if err = tmp.Chmod(perm); err != nil {
		return written, fmt.Errorf("chmod temp file: %w", err)
	}
	if err = tmp.Close(); err != nil {
		return written, fmt.Errorf("closing temp file: %w", err)
	}
	if err = os.Rename(tmpName, path); err != nil {
		return written, fmt.Errorf("renaming into place: %w", err)
	}
	return written, nil
}
