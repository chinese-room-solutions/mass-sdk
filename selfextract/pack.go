package selfextract

import (
	"bufio"
	"compress/gzip"
	"encoding/binary"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
)

// Pack builds a self-extracting installer at outPath: a byte-for-byte copy of
// hostPath (the setup exe) followed by each payload file's record and the
// trailer. The payload files extract as flat leaves by basename at install time,
// so their basenames must be unique — a collision is rejected (it's a packaging
// mistake, e.g. globbing two dirs, and the cryptic half-extracted install it
// produces is far harder to diagnose later). The output is given the host's mode
// so a Unix installer is executable.
func Pack(hostPath, outPath string, payload []string) error {
	if len(payload) == 0 {
		return fmt.Errorf("selfextract: no payload files")
	}
	if err := checkUniqueLeaves(payload); err != nil {
		return err
	}

	if err := os.MkdirAll(filepath.Dir(outPath), 0o755); err != nil {
		return fmt.Errorf("selfextract: creating output dir: %w", err)
	}

	hostInfo, err := os.Stat(hostPath)
	if err != nil {
		return fmt.Errorf("selfextract: stat host %s: %w", hostPath, err)
	}

	out, err := os.OpenFile(outPath, os.O_RDWR|os.O_CREATE|os.O_TRUNC, hostInfo.Mode())
	if err != nil {
		return fmt.Errorf("selfextract: opening output %s: %w", outPath, err)
	}
	defer out.Close() //nolint:errcheck // the final explicit Close reports errors

	// Start the installer as a byte-for-byte copy of the host exe.
	if err := appendFile(out, hostPath); err != nil {
		return fmt.Errorf("selfextract: copying host exe: %w", err)
	}

	var payloadSize uint64
	for _, file := range payload {
		n, err := writeEntry(out, filepath.Base(file), file)
		if err != nil {
			return err
		}
		payloadSize += n
	}

	// Trailer: payload_size, file_count, magic.
	if err := writeTrailer(out, payloadSize, uint64(len(payload))); err != nil {
		return err
	}
	if err := out.Close(); err != nil {
		return fmt.Errorf("selfextract: closing output: %w", err)
	}
	return nil
}

// writeEntry appends one [name_len][name][raw_size][comp_size][gzip stream]
// record to out, returning the record's total byte length (for the payload_size
// accumulator). comp_size isn't known until the stream is written, so its slot is
// reserved and patched in place afterwards — that keeps the source streaming
// straight into the output with no temp file and no whole-payload buffer.
func writeEntry(out *os.File, name, srcPath string) (uint64, error) {
	in, err := os.Open(srcPath)
	if err != nil {
		return 0, fmt.Errorf("selfextract: opening %s: %w", srcPath, err)
	}
	defer in.Close() //nolint:errcheck // read-only source
	info, err := in.Stat()
	if err != nil {
		return 0, fmt.Errorf("selfextract: stat %s: %w", srcPath, err)
	}

	var hdr [4]byte
	binary.LittleEndian.PutUint32(hdr[:], uint32(len(name)))
	if _, err := out.Write(hdr[:]); err != nil {
		return 0, fmt.Errorf("selfextract: writing name length: %w", err)
	}
	if _, err := io.WriteString(out, name); err != nil {
		return 0, fmt.Errorf("selfextract: writing name: %w", err)
	}

	sizesOff, err := out.Seek(0, io.SeekCurrent)
	if err != nil {
		return 0, fmt.Errorf("selfextract: locating %s record: %w", name, err)
	}
	var sizes [16]byte
	binary.LittleEndian.PutUint64(sizes[0:8], uint64(info.Size()))
	if _, err := out.Write(sizes[:]); err != nil {
		return 0, fmt.Errorf("selfextract: writing sizes: %w", err)
	}

	compSize, err := writeStream(out, in)
	if err != nil {
		return 0, fmt.Errorf("selfextract: compressing %s: %w", srcPath, err)
	}

	binary.LittleEndian.PutUint64(sizes[8:16], compSize)
	if _, err := out.WriteAt(sizes[8:16], sizesOff+8); err != nil {
		return 0, fmt.Errorf("selfextract: writing compressed size: %w", err)
	}
	// WriteAt is not guaranteed to leave the write offset alone on every platform;
	// re-seek so the next record still appends.
	if _, err := out.Seek(0, io.SeekEnd); err != nil {
		return 0, fmt.Errorf("selfextract: restoring append offset: %w", err)
	}
	return 4 + uint64(len(name)) + 16 + compSize, nil
}

// writeStream gzips in onto the end of out, returning the compressed byte count.
// Both directions move through fixed buffers, so a multi-GB payload file costs no
// more memory than a small one.
func writeStream(out *os.File, in io.Reader) (uint64, error) {
	start, err := out.Seek(0, io.SeekCurrent)
	if err != nil {
		return 0, err
	}
	bw := bufio.NewWriterSize(out, bufSize)
	// BestCompression over the default: packing runs once per release, and the few
	// extra seconds buy ~0.2% on a Go binary.
	zw, err := gzip.NewWriterLevel(bw, gzip.BestCompression)
	if err != nil {
		return 0, err
	}
	if _, err := io.Copy(zw, in); err != nil {
		return 0, err
	}
	if err := zw.Close(); err != nil {
		return 0, err
	}
	if err := bw.Flush(); err != nil {
		return 0, err
	}
	end, err := out.Seek(0, io.SeekCurrent)
	if err != nil {
		return 0, err
	}
	return uint64(end - start), nil
}

func writeTrailer(out io.Writer, payloadSize, fileCount uint64) error {
	var buf [TrailerSize]byte
	binary.LittleEndian.PutUint64(buf[0:8], payloadSize)
	binary.LittleEndian.PutUint64(buf[8:16], fileCount)
	copy(buf[16:24], magic)
	if _, err := out.Write(buf[:]); err != nil {
		return fmt.Errorf("selfextract: writing trailer: %w", err)
	}
	return nil
}

// appendFile streams src's raw bytes into out.
func appendFile(out io.Writer, src string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close() //nolint:errcheck // read-only source
	_, err = io.Copy(out, in)
	return err
}

// checkUniqueLeaves rejects payload files whose basenames collide.
func checkUniqueLeaves(payload []string) error {
	leaves := make([]string, len(payload))
	for i, f := range payload {
		leaves[i] = filepath.Base(f)
	}
	sort.Strings(leaves)
	for i := 1; i < len(leaves); i++ {
		if leaves[i] == leaves[i-1] {
			return fmt.Errorf("selfextract: duplicate payload entry name %q — entries must have unique filenames", leaves[i])
		}
	}
	return nil
}
