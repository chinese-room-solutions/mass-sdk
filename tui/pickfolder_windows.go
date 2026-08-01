//go:build windows

package tui

import (
	"fmt"
	"runtime"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
)

// pickFolder shows the modern IFileOpenDialog (Vista+) in folder-picker mode,
// with no parent window (the installer has no HWND). It drives the COM object
// through its vtable by syscall — pure Go, no cgo, so the CGO_ENABLED=0 setup
// binary keeps a native dialog. This mirrors the webview package's picker but is
// self-contained here so tui doesn't depend on webview (which is CGO on Unix).
//
// The goroutine is pinned to its OS thread for the whole call: COM apartment
// state is per-thread, so without LockOSThread the Go scheduler could move the
// goroutine to a different thread mid-dialog (where COM isn't initialised),
// which surfaces as bogus HRESULTs like "Incorrect function". The form calls
// this synchronously from its key loop, so a modal dialog on this thread is fine.
func pickFolder(title string) (string, bool, error) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()

	// Initialise this thread's COM apartment as STA (the shell dialog wants STA).
	// CoInitializeEx returns S_OK (first init — we own it, so uninit on the way
	// out), or S_FALSE / RPC_E_CHANGED_MODE when COM is already up on the thread
	// (usable as-is; don't uninit someone else's apartment). x/sys reports the
	// non-S_OK HRESULTs as a non-nil error, so branch on the code, not nil-ness.
	switch hr := coInitSTA(); hr {
	case 0: // S_OK — we initialised it
		defer windows.CoUninitialize()
	case sFalse, rpcEChangedMode: // already initialised — proceed, don't uninit
	default:
		return "", false, fmt.Errorf("CoInitializeEx: hr=0x%x", uint32(hr))
	}

	var dialog *iFileOpenDialog
	hr, _, _ := procCoCreateInstance.Call(
		uintptr(unsafe.Pointer(&clsidFileOpenDialog)),
		0,
		uintptr(clsctxInprocServer),
		uintptr(unsafe.Pointer(&iidIFileOpenDialog)),
		uintptr(unsafe.Pointer(&dialog)),
	)
	if hr != 0 || dialog == nil {
		return "", false, fmt.Errorf("CoCreateInstance(FileOpenDialog): hr=0x%x", hr)
	}
	defer dialog.Release()

	var opts uint32
	if hr := dialog.GetOptions(&opts); hr == 0 {
		dialog.SetOptions(opts | fosPickFolders | fosForceFilesystem | fosPathMustExist)
	}
	if title != "" {
		if p, err := windows.UTF16PtrFromString(title); err == nil {
			dialog.SetTitle(p)
		}
	}

	if hr := dialog.Show(nil); hr != 0 {
		if uint32(hr) == hresultCancelled {
			return "", false, nil // user cancelled → fall back to inline edit
		}
		return "", false, fmt.Errorf("dialog.Show: hr=0x%x", uint32(hr))
	}

	var item *iShellItem
	if hr := dialog.GetResult(&item); hr != 0 || item == nil {
		return "", false, fmt.Errorf("dialog.GetResult: hr=0x%x", uint32(hr))
	}
	defer item.Release()

	var pathPtr *uint16
	if hr := item.GetDisplayName(sigdnFileSysPath, &pathPtr); hr != 0 || pathPtr == nil {
		return "", false, fmt.Errorf("item.GetDisplayName: hr=0x%x", uint32(hr))
	}
	defer func() { _, _, _ = procCoTaskMemFree.Call(uintptr(unsafe.Pointer(pathPtr))) }()

	return windows.UTF16PtrToString(pathPtr), true, nil
}

// ── COM plumbing (windowless folder picker) ──────────────────────────
//
// Mirrors the webview package's COM bindings; kept here so tui has no
// dependency on webview. If a third caller appears, hoist this into a shared
// internal package.

const (
	clsctxInprocServer = 0x1
	sFalse             = 0x00000001 // CoInitializeEx: COM already initialised on this thread
	rpcEChangedMode    = 0x80010106 // CoInitializeEx: already initialised in a different mode
	hresultCancelled   = 0x800704C7 // HRESULT_FROM_WIN32(ERROR_CANCELLED)

	coinitApartmentThreaded = 0x2 // COINIT_APARTMENTTHREADED (STA)

	fosPickFolders     = 0x00000020
	fosForceFilesystem = 0x00000040
	fosPathMustExist   = 0x00000800

	sigdnFileSysPath = 0x80058000
)

// coInitSTA calls CoInitializeEx for an STA apartment and returns the raw
// HRESULT. We bind the proc directly rather than use windows.CoInitializeEx,
// which folds the non-S_OK success codes (S_FALSE) and RPC_E_CHANGED_MODE into a
// Go error — losing the distinction we need between "we initialised it" (uninit
// later) and "already initialised" (leave it alone).
func coInitSTA() uintptr {
	hr, _, _ := procCoInitializeEx.Call(0, coinitApartmentThreaded)
	return hr
}

var (
	ole32                = windows.NewLazySystemDLL("ole32.dll")
	procCoInitializeEx   = ole32.NewProc("CoInitializeEx")
	procCoCreateInstance = ole32.NewProc("CoCreateInstance")
	procCoTaskMemFree    = ole32.NewProc("CoTaskMemFree")

	// CLSID_FileOpenDialog {DC1C5A9C-E88A-4DDE-A5A1-60F82A20AEF7}
	clsidFileOpenDialog = windows.GUID{
		Data1: 0xDC1C5A9C, Data2: 0xE88A, Data3: 0x4DDE,
		Data4: [8]byte{0xA5, 0xA1, 0x60, 0xF8, 0x2A, 0x20, 0xAE, 0xF7},
	}
	// IID_IFileOpenDialog {D57C7288-D4AD-4768-BE02-9D969532D960}
	iidIFileOpenDialog = windows.GUID{
		Data1: 0xD57C7288, Data2: 0xD4AD, Data3: 0x4768,
		Data4: [8]byte{0xBE, 0x02, 0x9D, 0x96, 0x95, 0x32, 0xD9, 0x60},
	}
)

// iFileOpenDialog mirrors the COM vtable up to the methods we call.
// IFileOpenDialog : IFileDialog : IModalWindow : IUnknown.
type iFileOpenDialog struct {
	vtbl *iFileOpenDialogVtbl
}

type iFileOpenDialogVtbl struct {
	// IUnknown
	QueryInterface uintptr
	AddRef         uintptr
	Release        uintptr
	// IModalWindow
	Show uintptr
	// IFileDialog
	SetFileTypes        uintptr
	SetFileTypeIndex    uintptr
	GetFileTypeIndex    uintptr
	Advise              uintptr
	Unadvise            uintptr
	SetOptions          uintptr
	GetOptions          uintptr
	SetDefaultFolder    uintptr
	SetFolder           uintptr
	GetFolder           uintptr
	GetCurrentSelection uintptr
	SetFileName         uintptr
	GetFileName         uintptr
	SetTitle            uintptr
	SetOkButtonLabel    uintptr
	SetFileNameLabel    uintptr
	GetResult           uintptr
	// ... remaining IFileDialog/IFileOpenDialog methods unused.
}

func (d *iFileOpenDialog) Release() {
	_, _, _ = syscall.SyscallN(d.vtbl.Release, uintptr(unsafe.Pointer(d)))
}

func (d *iFileOpenDialog) Show(parent unsafe.Pointer) uintptr {
	hr, _, _ := syscall.SyscallN(d.vtbl.Show, uintptr(unsafe.Pointer(d)), uintptr(parent))
	return hr
}

func (d *iFileOpenDialog) SetOptions(opts uint32) uintptr {
	hr, _, _ := syscall.SyscallN(d.vtbl.SetOptions, uintptr(unsafe.Pointer(d)), uintptr(opts))
	return hr
}

func (d *iFileOpenDialog) GetOptions(opts *uint32) uintptr {
	hr, _, _ := syscall.SyscallN(d.vtbl.GetOptions, uintptr(unsafe.Pointer(d)), uintptr(unsafe.Pointer(opts)))
	return hr
}

func (d *iFileOpenDialog) SetTitle(title *uint16) uintptr {
	hr, _, _ := syscall.SyscallN(d.vtbl.SetTitle, uintptr(unsafe.Pointer(d)), uintptr(unsafe.Pointer(title)))
	return hr
}

func (d *iFileOpenDialog) GetResult(item **iShellItem) uintptr {
	hr, _, _ := syscall.SyscallN(d.vtbl.GetResult, uintptr(unsafe.Pointer(d)), uintptr(unsafe.Pointer(item)))
	return hr
}

// iShellItem mirrors the IShellItem vtable up to GetDisplayName.
type iShellItem struct {
	vtbl *iShellItemVtbl
}

type iShellItemVtbl struct {
	QueryInterface uintptr
	AddRef         uintptr
	Release        uintptr
	BindToHandler  uintptr
	GetParent      uintptr
	GetDisplayName uintptr
	GetAttributes  uintptr
	Compare        uintptr
}

func (it *iShellItem) Release() {
	_, _, _ = syscall.SyscallN(it.vtbl.Release, uintptr(unsafe.Pointer(it)))
}

func (it *iShellItem) GetDisplayName(kind uint32, out **uint16) uintptr {
	hr, _, _ := syscall.SyscallN(it.vtbl.GetDisplayName,
		uintptr(unsafe.Pointer(it)), uintptr(kind), uintptr(unsafe.Pointer(out)))
	return hr
}
