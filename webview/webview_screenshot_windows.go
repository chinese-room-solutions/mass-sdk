//go:build windows

package webview

import (
	"bytes"
	"fmt"
	"image"
	"image/png"
	"unsafe"

	"golang.org/x/sys/windows"
)

// pwRenderFullContent makes PrintWindow render the full content of the window,
// including child windows drawn via DirectComposition (the WebView2/Chromium
// surface). Without it PrintWindow captures only the host window, leaving the
// web content blank.
const pwRenderFullContent = 0x00000002

// Screenshot captures the window's client area (the web content, without the
// title bar or frame) and returns it PNG-encoded. It asks the window to render
// itself into an off-screen bitmap via PrintWindow(PW_RENDERFULLCONTENT), so it
// works even when the WebView2 surface isn't on the visible desktop.
func (w *nativeWindow) Screenshot() ([]byte, error) {
	hwnd := uintptr(w.hwnd)
	width, height, err := clientSize(hwnd)
	if err != nil {
		return nil, err
	}

	img, err := captureClient(hwnd, width, height)
	if err != nil {
		return nil, err
	}

	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		return nil, fmt.Errorf("encoding screenshot: %w", err)
	}
	return buf.Bytes(), nil
}

// clientSize returns the window's client-area dimensions in pixels.
func clientSize(hwnd uintptr) (width, height int, err error) {
	type rect struct{ Left, Top, Right, Bottom int32 }
	var rc rect
	getClientRect := user32.NewProc("GetClientRect")
	if ret, _, callErr := getClientRect.Call(hwnd, uintptr(unsafe.Pointer(&rc))); ret == 0 {
		return 0, 0, fmt.Errorf("GetClientRect: %w", callErr)
	}
	width, height = int(rc.Right-rc.Left), int(rc.Bottom-rc.Top)
	if width <= 0 || height <= 0 {
		return 0, 0, fmt.Errorf("window has no client area (%dx%d)", width, height)
	}
	return width, height, nil
}

// captureClient renders the window's client area into a top-down 32-bit DIB via
// PrintWindow and copies the pixels into an RGBA image. All GDI resources are
// released before returning.
func captureClient(hwnd uintptr, width, height int) (*image.RGBA, error) {
	gdi32 := windows.NewLazySystemDLL("gdi32.dll")
	getDC := user32.NewProc("GetDC")
	releaseDC := user32.NewProc("ReleaseDC")
	printWindow := user32.NewProc("PrintWindow")
	createCompatibleDC := gdi32.NewProc("CreateCompatibleDC")
	createDIBSection := gdi32.NewProc("CreateDIBSection")
	selectObject := gdi32.NewProc("SelectObject")
	deleteObject := gdi32.NewProc("DeleteObject")
	deleteDC := gdi32.NewProc("DeleteDC")

	screenDC, _, _ := getDC.Call(0)
	if screenDC == 0 {
		return nil, fmt.Errorf("GetDC failed")
	}
	defer func() { _, _, _ = releaseDC.Call(0, screenDC) }()

	memDC, _, _ := createCompatibleDC.Call(screenDC)
	if memDC == 0 {
		return nil, fmt.Errorf("CreateCompatibleDC failed")
	}
	defer func() { _, _, _ = deleteDC.Call(memDC) }()

	// BITMAPINFOHEADER for a top-down (negative height) 32-bit BGRA DIB, so the
	// memory layout matches image rows top-to-bottom.
	var bih [40]byte
	putU32(bih[0:], 40)
	putU32(bih[4:], uint32(width))
	putU32(bih[8:], uint32(-int32(height)))
	putU16(bih[12:], 1)  // planes
	putU16(bih[14:], 32) // bits per pixel

	var bits uintptr
	dib, _, _ := createDIBSection.Call(memDC, uintptr(unsafe.Pointer(&bih[0])), 0, uintptr(unsafe.Pointer(&bits)), 0, 0)
	if dib == 0 || bits == 0 {
		return nil, fmt.Errorf("CreateDIBSection failed")
	}
	defer func() { _, _, _ = deleteObject.Call(dib) }()

	prev, _, _ := selectObject.Call(memDC, dib)
	defer func() { _, _, _ = selectObject.Call(memDC, prev) }()

	if ret, _, callErr := printWindow.Call(hwnd, memDC, pwRenderFullContent); ret == 0 {
		return nil, fmt.Errorf("PrintWindow: %w", callErr)
	}

	// The DIB holds BGRA bytes top-down; convert to RGBA.
	src := unsafe.Slice((*byte)(unsafe.Pointer(bits)), width*height*4) //nolint:govet // bits is a valid pointer from CreateDIBSection.
	img := image.NewRGBA(image.Rect(0, 0, width, height))
	for i := 0; i < width*height; i++ {
		s := i * 4
		img.Pix[s+0] = src[s+2] // R ← B
		img.Pix[s+1] = src[s+1] // G
		img.Pix[s+2] = src[s+0] // B ← R
		img.Pix[s+3] = 0xff     // opaque: WebView2 content has no meaningful alpha.
	}
	return img, nil
}

func putU16(b []byte, v uint16) { b[0], b[1] = byte(v), byte(v>>8) }
func putU32(b []byte, v uint32) { b[0], b[1], b[2], b[3] = byte(v), byte(v>>8), byte(v>>16), byte(v>>24) }
