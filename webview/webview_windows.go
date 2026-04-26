//go:build windows

package webview

import (
	"bytes"
	"encoding/binary"
	"image"
	"image/png"
	"unsafe"

	webview2 "github.com/jchv/go-webview2"
	"golang.org/x/sys/windows"
)

var (
	dwmapi = windows.NewLazySystemDLL("dwmapi.dll")
	user32 = windows.NewLazySystemDLL("user32.dll")
)

type nativeWindow struct {
	wv   webview2.WebView
	hwnd unsafe.Pointer
}

// Open creates a native webview window. Returns nil if WebView2 is unavailable.
func Open(opts Options) WindowInterface {
	wv := webview2.NewWithOptions(webview2.WebViewOptions{
		AutoFocus: true,
		WindowOptions: webview2.WindowOptions{
			Title:  opts.Title,
			Width:  uint(opts.Width),
			Height: uint(opts.Height),
			Center: true,
		},
	})
	if wv == nil {
		return nil
	}

	w := &nativeWindow{wv: wv, hwnd: wv.Window()}

	// Dark title bar.
	setWindowAttribute := dwmapi.NewProc("DwmSetWindowAttribute")
	val := int32(1)
	_, _, _ = setWindowAttribute.Call(
		uintptr(w.hwnd),
		20, // DWMWA_USE_IMMERSIVE_DARK_MODE
		uintptr(unsafe.Pointer(&val)),
		unsafe.Sizeof(val),
	)

	// Set window and taskbar icon.
	if len(opts.IconPNG) > 0 {
		setWindowIcon(w.hwnd, opts.IconPNG)
	}

	wv.Navigate(opts.URL)
	return w
}

func (w *nativeWindow) Run()     { w.wv.Run() }
func (w *nativeWindow) Destroy() { w.wv.Destroy() }

const wmSetIcon = 0x0080

// setWindowIcon sets the taskbar icon (ICON_BIG) and a blank title bar icon (ICON_SMALL).
func setWindowIcon(hwnd unsafe.Pointer, iconPNG []byte) {
	bigIcon := createIconFromPNG(iconPNG, 256)
	if bigIcon == 0 {
		return
	}
	blankIcon := createBlankIcon()

	sendMessage := user32.NewProc("SendMessageW")
	h := uintptr(hwnd)
	if blankIcon != 0 {
		_, _, _ = sendMessage.Call(h, wmSetIcon, 0, blankIcon)
	}
	_, _, _ = sendMessage.Call(h, wmSetIcon, 1, bigIcon)
}

func createBlankIcon() uintptr {
	gdi32 := windows.NewLazySystemDLL("gdi32.dll")
	createBitmap := gdi32.NewProc("CreateBitmap")
	deleteObject := gdi32.NewProc("DeleteObject")
	createIconIndirect := user32.NewProc("CreateIconIndirect")

	andBits := [4]byte{0xFF, 0, 0, 0}
	xorBits := [4]byte{0, 0, 0, 0}

	hbmMask, _, _ := createBitmap.Call(1, 1, 1, 1, uintptr(unsafe.Pointer(&andBits[0])))
	hbmColor, _, _ := createBitmap.Call(1, 1, 1, 1, uintptr(unsafe.Pointer(&xorBits[0])))
	if hbmMask == 0 || hbmColor == 0 {
		return 0
	}
	defer func() {
		_, _, _ = deleteObject.Call(hbmMask)
		_, _, _ = deleteObject.Call(hbmColor)
	}()

	type blankIconInfo struct {
		FIcon    int32
		XHotspot int32
		YHotspot int32
		HbmMask  uintptr
		HbmColor uintptr
	}
	ii := blankIconInfo{FIcon: 1, HbmMask: hbmMask, HbmColor: hbmColor}
	icon, _, _ := createIconIndirect.Call(uintptr(unsafe.Pointer(&ii)))
	return icon
}

func createIconFromPNG(data []byte, size int) uintptr {
	src, err := png.Decode(bytes.NewReader(data))
	if err != nil {
		return 0
	}

	dst := image.NewNRGBA(image.Rect(0, 0, size, size))
	srcB := src.Bounds()
	srcW, srcH := srcB.Dx(), srcB.Dy()
	scale := float64(size) / float64(srcW)
	if s := float64(size) / float64(srcH); s < scale {
		scale = s
	}
	dstW := int(float64(srcW) * scale)
	dstH := int(float64(srcH) * scale)
	offX := (size - dstW) / 2
	offY := (size - dstH) / 2

	for y := 0; y < dstH; y++ {
		srcY := srcB.Min.Y + y*srcH/dstH
		for x := 0; x < dstW; x++ {
			srcX := srcB.Min.X + x*srcW/dstW
			dst.Set(offX+x, offY+y, src.At(srcX, srcY))
		}
	}

	return nrgbaToHICON(dst)
}

func nrgbaToHICON(img *image.NRGBA) uintptr {
	w := img.Bounds().Dx()
	h := img.Bounds().Dy()

	pixels := make([]byte, w*h*4)
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			si := y*img.Stride + x*4
			di := (h-1-y)*w*4 + x*4
			pixels[di+0] = img.Pix[si+2] // B
			pixels[di+1] = img.Pix[si+1] // G
			pixels[di+2] = img.Pix[si+0] // R
			pixels[di+3] = img.Pix[si+3] // A
		}
	}

	gdi32 := windows.NewLazySystemDLL("gdi32.dll")
	createBitmap := gdi32.NewProc("CreateBitmap")
	createDIBSection := gdi32.NewProc("CreateDIBSection")
	deleteObject := gdi32.NewProc("DeleteObject")
	createIconIndirect := user32.NewProc("CreateIconIndirect")

	var bihDIB [40]byte
	binary.LittleEndian.PutUint32(bihDIB[0:], 40)
	binary.LittleEndian.PutUint32(bihDIB[4:], uint32(w))
	binary.LittleEndian.PutUint32(bihDIB[8:], uint32(h))
	binary.LittleEndian.PutUint16(bihDIB[12:], 1)
	binary.LittleEndian.PutUint16(bihDIB[14:], 32)

	var ppvBits uintptr
	hbmColor, _, _ := createDIBSection.Call(0, uintptr(unsafe.Pointer(&bihDIB[0])), 0, uintptr(unsafe.Pointer(&ppvBits)), 0, 0)
	if hbmColor == 0 || ppvBits == 0 {
		return 0
	}

	copy(unsafe.Slice((*byte)(unsafe.Pointer(ppvBits)), len(pixels)), pixels) //nolint:govet // ppvBits is a valid pointer from CreateDIBSection syscall out-param

	maskSize := ((w + 31) / 32) * 4 * h
	andBits := make([]byte, maskSize)
	hbmMask, _, _ := createBitmap.Call(uintptr(w), uintptr(h), 1, 1, uintptr(unsafe.Pointer(&andBits[0])))
	if hbmMask == 0 {
		_, _, _ = deleteObject.Call(hbmColor)
		return 0
	}

	type iconInfo struct {
		FIcon    int32
		XHotspot int32
		YHotspot int32
		HbmMask  uintptr
		HbmColor uintptr
	}

	ii := iconInfo{FIcon: 1, HbmMask: hbmMask, HbmColor: hbmColor}
	icon, _, _ := createIconIndirect.Call(uintptr(unsafe.Pointer(&ii)))

	_, _, _ = deleteObject.Call(hbmMask)
	_, _, _ = deleteObject.Call(hbmColor)

	return icon
}
