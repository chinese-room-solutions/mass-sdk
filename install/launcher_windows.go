//go:build windows

package install

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
)

// createLauncher creates a Start Menu shortcut (.lnk) via COM (IShellLinkW +
// IPersistFile) — no third-party dependency. Per-user → the user's Start Menu
// Programs; machine-wide → the common (all-users) Start Menu Programs (needs
// admin to write).
func (a AppSpec) createLauncher(spec LauncherSpec) (string, error) {
	dir, err := startMenuProgramsDir(spec.PerUser)
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	lnk := filepath.Join(dir, a.DisplayName+".lnk")
	if err := writeShellLink(lnk, spec.ExePath, windowsIconSource(spec.IconPath)); err != nil {
		return "", err
	}
	return lnk, nil
}

// windowsIconSource keeps only an icon source a .lnk can actually read. Windows
// takes a shortcut icon from an .ico, .exe or .dll; pointed at anything else —
// callers pass the same PNG they hand to Linux and macOS — the shell shows a
// blank page icon rather than falling back. Empty leaves the shortcut on the
// target exe's own icon resource, which carries the same artwork.
func windowsIconSource(iconPath string) string {
	switch strings.ToLower(filepath.Ext(iconPath)) {
	case ".ico", ".exe", ".dll":
		return iconPath
	}
	return ""
}

func (a AppSpec) removeLauncher(perUser bool) error {
	dir, err := startMenuProgramsDir(perUser)
	if err != nil {
		return err
	}
	lnk := filepath.Join(dir, a.DisplayName+".lnk")
	if err := os.Remove(lnk); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return nil
}

// startMenuProgramsDir resolves the Start Menu Programs folder via the known
// folder ids: FOLDERID_Programs (per-user) / FOLDERID_CommonPrograms (all-users).
func startMenuProgramsDir(perUser bool) (string, error) {
	id := windows.FOLDERID_CommonPrograms
	if perUser {
		id = windows.FOLDERID_Programs
	}
	p, err := windows.KnownFolderPath(id, 0)
	if err != nil {
		return "", fmt.Errorf("install: resolving Start Menu folder: %w", err)
	}
	return p, nil
}

// --- COM IShellLinkW / IPersistFile plumbing --------------------------------

var (
	modole32             = windows.NewLazySystemDLL("ole32.dll")
	procCoInitializeEx   = modole32.NewProc("CoInitializeEx")
	procCoUninitialize   = modole32.NewProc("CoUninitialize")
	procCoCreateInstance = modole32.NewProc("CoCreateInstance")
)

// HRESULTs we branch on.
const (
	sOK              = 0
	rpcEChangedMode  = 0x80010106 // COM already initialized in another mode
	clsctxInProc     = 0x1        // CLSCTX_INPROC_SERVER
	coinitApartment  = 0x2        // COINIT_APARTMENTTHREADED
)

var (
	clsidShellLink = windows.GUID{Data1: 0x00021401, Data2: 0x0000, Data3: 0x0000,
		Data4: [8]byte{0xC0, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x46}}
	iidShellLinkW = windows.GUID{Data1: 0x000214F9, Data2: 0x0000, Data3: 0x0000,
		Data4: [8]byte{0xC0, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x46}}
	iidPersistFile = windows.GUID{Data1: 0x0000010b, Data2: 0x0000, Data3: 0x0000,
		Data4: [8]byte{0xC0, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x46}}
)

// IUnknown vtable slots 0..2, then IShellLinkW slots; IPersistFile slots after
// its own IUnknown. We only call the few methods we need, by vtable index.
type iShellLinkW struct{ vtbl *iShellLinkWVtbl }
type iShellLinkWVtbl struct {
	QueryInterface uintptr
	AddRef         uintptr
	Release        uintptr
	GetPath        uintptr
	GetIDList      uintptr
	SetIDList      uintptr
	GetDescription uintptr
	SetDescription uintptr
	GetWorkingDir  uintptr
	SetWorkingDir  uintptr
	GetArguments   uintptr
	SetArguments   uintptr
	GetHotkey      uintptr
	SetHotkey      uintptr
	GetShowCmd     uintptr
	SetShowCmd     uintptr
	GetIconLoc     uintptr
	SetIconLoc     uintptr
	SetRelative    uintptr
	Resolve        uintptr
	SetPath        uintptr
}

type iPersistFile struct{ vtbl *iPersistFileVtbl }
type iPersistFileVtbl struct {
	QueryInterface uintptr
	AddRef         uintptr
	Release        uintptr
	GetClassID     uintptr
	IsDirty        uintptr
	Load           uintptr
	Save           uintptr
	SaveCompleted  uintptr
	GetCurFile     uintptr
}

func writeShellLink(lnkPath, target, icon string) (err error) {
	hr, _, _ := procCoInitializeEx.Call(0, coinitApartment)
	switch uint32(hr) {
	case sOK:
		defer procCoUninitialize.Call() //nolint:errcheck // teardown, no recovery
	case rpcEChangedMode:
		// COM already initialized in another mode on this thread; that's fine, we
		// can still use it, and we must NOT call CoUninitialize (we didn't init).
	default:
		return fmt.Errorf("install: CoInitializeEx: hr=0x%x", uint32(hr))
	}

	var psl *iShellLinkW
	if hr, _, _ := procCoCreateInstance.Call(
		uintptr(unsafe.Pointer(&clsidShellLink)), 0, clsctxInProc,
		uintptr(unsafe.Pointer(&iidShellLinkW)), uintptr(unsafe.Pointer(&psl))); uint32(hr) != sOK {
		return fmt.Errorf("install: CoCreateInstance(ShellLink): hr=0x%x", uint32(hr))
	}
	defer comRelease(psl.vtbl.Release, unsafe.Pointer(psl))

	tgt, err := windows.UTF16PtrFromString(target)
	if err != nil {
		return err
	}
	if hr, _, _ := syscall.SyscallN(psl.vtbl.SetPath,
		uintptr(unsafe.Pointer(psl)), uintptr(unsafe.Pointer(tgt))); hr != 0 {
		return fmt.Errorf("install: IShellLink::SetPath: hr=0x%x", hr)
	}

	// Working directory = the binary's directory, so relative paths the app
	// resolves land in the install tree. Decoration only; ignore failure.
	if wd, err := windows.UTF16PtrFromString(filepath.Dir(target)); err == nil {
		_, _, _ = syscall.SyscallN(psl.vtbl.SetWorkingDir,
			uintptr(unsafe.Pointer(psl)), uintptr(unsafe.Pointer(wd)))
	}

	if icon != "" {
		if ic, err := windows.UTF16PtrFromString(icon); err == nil {
			_, _, _ = syscall.SyscallN(psl.vtbl.SetIconLoc,
				uintptr(unsafe.Pointer(psl)), uintptr(unsafe.Pointer(ic)), 0)
		}
	}

	// QueryInterface for IPersistFile and Save the .lnk.
	var ppf *iPersistFile
	if hr, _, _ := syscall.SyscallN(psl.vtbl.QueryInterface,
		uintptr(unsafe.Pointer(psl)), uintptr(unsafe.Pointer(&iidPersistFile)),
		uintptr(unsafe.Pointer(&ppf))); hr != 0 {
		return fmt.Errorf("install: QueryInterface(IPersistFile): hr=0x%x", hr)
	}
	defer comRelease(ppf.vtbl.Release, unsafe.Pointer(ppf))

	lnk, err := windows.UTF16PtrFromString(lnkPath)
	if err != nil {
		return err
	}
	if hr, _, _ := syscall.SyscallN(ppf.vtbl.Save,
		uintptr(unsafe.Pointer(ppf)), uintptr(unsafe.Pointer(lnk)), 1 /*fRemember*/); hr != 0 {
		return fmt.Errorf("install: IPersistFile::Save: hr=0x%x", hr)
	}
	return nil
}

func comRelease(release uintptr, this unsafe.Pointer) {
	_, _, _ = syscall.SyscallN(release, uintptr(this)) // teardown, no recovery
}
