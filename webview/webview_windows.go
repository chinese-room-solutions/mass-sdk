//go:build windows

package webview

import (
	"bytes"
	"encoding/binary"
	"image"
	"image/png"
	"sync"
	"unsafe"

	webview2 "github.com/jchv/go-webview2"
	"golang.org/x/sys/windows"
)

var (
	dwmapi = windows.NewLazySystemDLL("dwmapi.dll")
	user32 = windows.NewLazySystemDLL("user32.dll")
)

type nativeWindow struct {
	wv         webview2.WebView
	hwnd       unsafe.Pointer
	origProc   uintptr // the webview's window proc, chained to from ours
	onMinimize func()  // fired on WM_SIZE/SIZE_MINIMIZED when set

	visMu   sync.Mutex
	visible bool // tracks Show/Hide/minimize so Toggle is consistent
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

	w := &nativeWindow{wv: wv, hwnd: wv.Window(), visible: true}

	// Title bar follows the app theme (defaults to dark). Live changes come
	// through SetTheme, which the app wires to its settings handler.
	applyTitleBarTheme(w.hwnd, opts.Theme != "light")

	// Paint the window backdrop the theme's base colour so the brief gap before
	// WebView2 first paints isn't a blinding white flash on a dark theme. The
	// page's own html background (theme.css) covers the rest once it loads.
	applyWindowBackground(w.hwnd, opts.Theme != "light")

	// Set window and taskbar icon.
	if len(opts.IconPNG) > 0 {
		setWindowIcon(w.hwnd, opts.IconPNG)
	}

	wv.Navigate(opts.URL)
	return w
}

func (w *nativeWindow) Run() { w.wv.Run() }

// Terminate stops the loop from any goroutine. webview2's Terminate calls
// PostQuitMessage on the *calling* thread, so when invoked from a background
// goroutine (e.g. a tray click) it would queue WM_QUIT on the wrong thread and
// Run would never return. Dispatch onto the UI thread so the quit lands where
// the message loop actually reads it.
func (w *nativeWindow) Terminate() { w.wv.Dispatch(w.wv.Terminate) }

func (w *nativeWindow) Destroy() { w.wv.Destroy() }

const (
	gwlpWndProc   = -4
	wmSize        = 0x0005
	sizeMinimized = 1
	swHide        = 0
	swShow        = 5
	swRestore     = 9
)

// procRegistry maps a window handle to its nativeWindow so the shared wndproc
// callback (which only gets the HWND) can find the Go-side state. Guarded by a
// mutex since Open and the message loop run on different goroutines.
var (
	procMu       sync.Mutex
	procRegistry = map[uintptr]*nativeWindow{}
)

// installMinimizeHook subclasses the window proc so a minimize (the title-bar
// fold button) routes to onMinimize instead of the default iconify. Without a
// hook installed (onMinimize nil) the original proc handles everything, so the
// OS behavior is unchanged. Close (WM_CLOSE) is never intercepted — it still
// destroys the window and quits.
func (w *nativeWindow) installMinimizeHook() {
	if w.origProc != 0 {
		return // already installed
	}
	procMu.Lock()
	procRegistry[uintptr(w.hwnd)] = w
	procMu.Unlock()

	setWindowLongPtr := user32.NewProc("SetWindowLongPtrW")
	prev, _, _ := setWindowLongPtr.Call(uintptr(w.hwnd), uintptrNeg(gwlpWndProc), wndProcCallback)
	w.origProc = prev
}

// wndProcCallback is the subclassed window procedure. It folds a minimize into
// the registered onMinimize action; everything else chains to the original proc.
var wndProcCallback = windows.NewCallback(func(hwnd, msg, wParam, lParam uintptr) uintptr {
	procMu.Lock()
	w := procRegistry[hwnd]
	procMu.Unlock()
	if w == nil {
		return defWindowProc(hwnd, msg, wParam, lParam)
	}
	if msg == wmSize && wParam == sizeMinimized && w.onMinimize != nil {
		w.onMinimize() // app-supplied; e.g. Hide, which records visibility
		return 0       // swallow: don't let the window actually iconify
	}
	return callWindowProc(w.origProc, hwnd, msg, wParam, lParam)
})

func callWindowProc(proc, hwnd, msg, wParam, lParam uintptr) uintptr {
	p := user32.NewProc("CallWindowProcW")
	r, _, _ := p.Call(proc, hwnd, msg, wParam, lParam)
	return r
}

func defWindowProc(hwnd, msg, wParam, lParam uintptr) uintptr {
	p := user32.NewProc("DefWindowProcW")
	r, _, _ := p.Call(hwnd, msg, wParam, lParam)
	return r
}

// uintptrNeg converts a signed window-long index to the uintptr the syscall
// expects (e.g. GWLP_WNDPROC = -4).
func uintptrNeg(i int) uintptr { return uintptr(int32(i)) }

// SetOnMinimize installs the fold-to-tray hook. A nil callback restores default
// minimize behavior (the hook stays but becomes a passthrough).
func (w *nativeWindow) SetOnMinimize(f func()) {
	w.onMinimize = f
	if f != nil {
		w.wv.Dispatch(w.installMinimizeHook)
	}
}

// SetOnFileDrop is a no-op: WebView2 delivers external file drops to the
// page's DOM handlers, so the in-page dropzone works without native help.
func (w *nativeWindow) SetOnFileDrop(func(paths []string)) {}

// Eval runs JavaScript in the page. Dispatched onto the UI loop, so it is safe
// from any goroutine.
func (w *nativeWindow) Eval(js string) {
	w.wv.Dispatch(func() { w.wv.Eval(js) })
}

// setVisible records the window's shown/hidden state for Toggle.
func (w *nativeWindow) setVisible(v bool) {
	w.visMu.Lock()
	w.visible = v
	w.visMu.Unlock()
}

func (w *nativeWindow) isVisible() bool {
	w.visMu.Lock()
	defer w.visMu.Unlock()
	return w.visible
}

// Hide removes the window from the screen and taskbar without destroying it.
func (w *nativeWindow) Hide() {
	w.setVisible(false)
	w.wv.Dispatch(func() {
		showWindow := user32.NewProc("ShowWindow")
		_, _, _ = showWindow.Call(uintptr(w.hwnd), swHide)
	})
}

// Show re-displays the window and brings it to the foreground.
func (w *nativeWindow) Show() {
	w.setVisible(true)
	w.wv.Dispatch(func() {
		showWindow := user32.NewProc("ShowWindow")
		setForeground := user32.NewProc("SetForegroundWindow")
		_, _, _ = showWindow.Call(uintptr(w.hwnd), swShow)
		_, _, _ = showWindow.Call(uintptr(w.hwnd), swRestore)
		_, _, _ = setForeground.Call(uintptr(w.hwnd))
	})
}

// Toggle hides the window if shown, shows it if hidden.
func (w *nativeWindow) Toggle() {
	if w.isVisible() {
		w.Hide()
	} else {
		w.Show()
	}
}

// SetTheme repaints the title bar for "dark"/"light". Dispatched onto the UI
// thread since it may be called from a JS-bound callback.
func (w *nativeWindow) SetTheme(theme string) {
	dark := theme != "light"
	w.wv.Dispatch(func() { applyTitleBarTheme(w.hwnd, dark) })
}

// PickFolder shows the Vista-era IFileOpenDialog in folder-picker mode, parented
// to the app window, and returns the selected filesystem path. ok is false when
// the user cancels.
func (w *nativeWindow) PickFolder(title string) (string, bool, error) {
	return pickFolder(w.hwnd, title)
}

// applyTitleBarTheme toggles DWMWA_USE_IMMERSIVE_DARK_MODE on the window.
func applyTitleBarTheme(hwnd unsafe.Pointer, dark bool) {
	setWindowAttribute := dwmapi.NewProc("DwmSetWindowAttribute")
	val := int32(0)
	if dark {
		val = 1
	}
	_, _, _ = setWindowAttribute.Call(
		uintptr(hwnd),
		20, // DWMWA_USE_IMMERSIVE_DARK_MODE
		uintptr(unsafe.Pointer(&val)),
		unsafe.Sizeof(val),
	)
}

// applyWindowBackground sets the window-class background brush to the theme's
// base colour so the system erases the window with it (instead of white) in the
// frames before the embedded WebView2 paints. COLORREF is 0x00BBGGRR; the theme
// base greys are near-neutral so byte order barely matters. Idempotent: the old
// brush is freed before the new one is installed.
func applyWindowBackground(hwnd unsafe.Pointer, dark bool) {
	gdi32 := windows.NewLazySystemDLL("gdi32.dll")
	createSolidBrush := gdi32.NewProc("CreateSolidBrush")
	deleteObject := gdi32.NewProc("DeleteObject")
	setClassLongPtr := user32.NewProc("SetClassLongPtrW")
	invalidateRect := user32.NewProc("InvalidateRect")

	var color uintptr = 0x00161616 // dark  #171616
	if !dark {
		color = 0x00f2f5f7 // light #f7f5f2
	}
	brush, _, _ := createSolidBrush.Call(color)
	if brush == 0 {
		return
	}
	// GCLP_HBRBACKGROUND = -10. Returns the previous brush, which we own and free.
	prev, _, _ := setClassLongPtr.Call(uintptr(hwnd), ^uintptr(9), brush)
	if prev != 0 {
		_, _, _ = deleteObject.Call(prev)
	}
	_, _, _ = invalidateRect.Call(uintptr(hwnd), 0, 1)
}

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
