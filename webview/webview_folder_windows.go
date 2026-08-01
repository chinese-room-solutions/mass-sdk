//go:build windows

package webview

import (
	"fmt"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
)

// pickFolder shows the modern IFileOpenDialog (Vista+) configured to pick a
// folder, modal to parent. It drives the COM object through its vtable by
// syscall — no cgo, no extra deps. Returns ok=false when the user cancels.
//
// COM is initialized on this goroutine for the call; the caller should run this
// off the UI event loop (a normal HTTP handler goroutine is fine).
func pickFolder(parent unsafe.Pointer, title string) (string, bool, error) {
	if err := windows.CoInitializeEx(0, windows.COINIT_APARTMENTTHREADED); err != nil {
		// RPC_E_CHANGED_MODE means COM is already up in another mode on this
		// thread — usable, just don't uninit it.
		if errno, ok := err.(syscall.Errno); !ok || uintptr(errno) != rpcEChangedMode {
			return "", false, fmt.Errorf("CoInitializeEx: %w", err)
		}
	} else {
		defer windows.CoUninitialize()
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

	// Force folder-picker mode.
	var opts uint32
	if hr := dialog.GetOptions(&opts); hr == 0 {
		dialog.SetOptions(opts | fosPickFolders | fosForceFilesystem | fosPathMustExist)
	}
	if title != "" {
		if p, err := windows.UTF16PtrFromString(title); err == nil {
			dialog.SetTitle(p)
		}
	}

	if hr := dialog.Show(parent); hr != 0 {
		// HRESULT for "cancelled" (ERROR_CANCELLED mapped to HRESULT).
		if uint32(hr) == hresultCancelled {
			return "", false, nil
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

// ── COM plumbing ─────────────────────────────────────────────────────

const (
	clsctxInprocServer = 0x1
	rpcEChangedMode    = 0x80010106
	hresultCancelled   = 0x800704C7 // HRESULT_FROM_WIN32(ERROR_CANCELLED)

	fosPickFolders     = 0x00000020
	fosForceFilesystem = 0x00000040
	fosPathMustExist   = 0x00000800

	sigdnFileSysPath = 0x80058000
)

var (
	ole32                = windows.NewLazySystemDLL("ole32.dll")
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

// iFileOpenDialog mirrors the COM vtable layout up to the methods we call.
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
