//go:build windows

package tray

import (
	"bytes"
	"encoding/binary"
	"image/png"
)

// trayIcon wraps the PNG in a minimal ICO container, which is what the Windows
// tray (Shell_NotifyIcon via fyne/systray) expects. Vista+ accepts a PNG-encoded
// image embedded directly in an ICO entry, so no re-encoding of the pixels is
// needed — just the 6-byte ICONDIR header + one 16-byte ICONDIRENTRY pointing at
// the PNG bytes. Returns nil if the PNG can't be decoded (dimensions are read
// from the header for the entry).
func trayIcon(pngBytes []byte) []byte {
	if len(pngBytes) == 0 {
		return nil
	}
	cfg, err := png.DecodeConfig(bytes.NewReader(pngBytes))
	if err != nil {
		return nil
	}

	var buf bytes.Buffer
	// ICONDIR: reserved(0), type(1 = icon), count(1).
	_ = binary.Write(&buf, binary.LittleEndian, uint16(0))
	_ = binary.Write(&buf, binary.LittleEndian, uint16(1))
	_ = binary.Write(&buf, binary.LittleEndian, uint16(1))

	// ICONDIRENTRY. Width/height of 0 mean 256; clamp larger images to that.
	dim := func(v int) byte {
		if v >= 256 {
			return 0
		}
		return byte(v)
	}
	buf.WriteByte(dim(cfg.Width))                           // width
	buf.WriteByte(dim(cfg.Height))                          // height
	buf.WriteByte(0)                                        // color count (0 = ≥8bpp)
	buf.WriteByte(0)                                        // reserved
	_ = binary.Write(&buf, binary.LittleEndian, uint16(1))  // color planes
	_ = binary.Write(&buf, binary.LittleEndian, uint16(32)) // bits per pixel
	_ = binary.Write(&buf, binary.LittleEndian, uint32(len(pngBytes)))
	// Image offset: header (6) + this entry (16) = 22.
	_ = binary.Write(&buf, binary.LittleEndian, uint32(22))

	buf.Write(pngBytes)
	return buf.Bytes()
}
