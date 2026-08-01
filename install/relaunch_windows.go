//go:build windows

package install

import (
	"errors"
	"os"
	"strings"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
)

var (
	modshell32         = windows.NewLazySystemDLL("shell32.dll")
	procShellExecuteEx = modshell32.NewProc("ShellExecuteExW")
)

// shellExecuteInfo mirrors SHELLEXECUTEINFOW (the fields we set; the rest stay
// zero). Laid out to match the C struct's size/alignment on amd64.
type shellExecuteInfo struct {
	cbSize       uint32
	fMask        uint32
	hwnd         windows.Handle
	lpVerb       *uint16
	lpFile       *uint16
	lpParameters *uint16
	lpDirectory  *uint16
	nShow        int32
	hInstApp     windows.Handle
	lpIDList     uintptr
	lpClass      *uint16
	hkeyClass    windows.Handle
	dwHotKey     uint32
	hIconOrMonitor windows.Handle
	hProcess     windows.Handle
}

const swShowNormal = 1

// relaunchElevated re-launches this process elevated via UAC (ShellExecuteEx with
// the "runas" verb), passing args verbatim so the elevated copy runs the chosen
// action non-interactively. Returns ElevatedChildStarted when the elevated
// process was launched — the work happens in the CHILD, so the caller must exit
// immediately (two copies must not both run). Returns ElevationDeclined when
// the user dismissed the UAC prompt, ElevationFailed on any other launch error —
// the caller stays unelevated and reports the original access error.
func relaunchElevated(args []string) ElevationOutcome {
	exe, err := os.Executable()
	if err != nil {
		return ElevationFailed
	}
	verb, err := windows.UTF16PtrFromString("runas")
	if err != nil {
		return ElevationFailed
	}
	file, err := windows.UTF16PtrFromString(exe)
	if err != nil {
		return ElevationFailed
	}
	params, err := windows.UTF16PtrFromString(joinArgs(args))
	if err != nil {
		return ElevationFailed
	}

	info := shellExecuteInfo{
		lpVerb:       verb,
		lpFile:       file,
		lpParameters: params,
		nShow:        swShowNormal,
	}
	info.cbSize = uint32(unsafe.Sizeof(info))

	ret, _, callErr := procShellExecuteEx.Call(uintptr(unsafe.Pointer(&info)))
	return classifyShellExecute(ret, callErr, uintptr(info.hInstApp))
}

// classifyShellExecute maps ShellExecuteEx's three outputs onto an
// ElevationOutcome. Split out from the syscall so the mapping is testable.
func classifyShellExecute(ret uintptr, callErr error, hInstApp uintptr) ElevationOutcome {
	if ret == 0 {
		// FALSE: GetLastError carries the reason; ERROR_CANCELLED means the user
		// dismissed the UAC prompt.
		if errors.Is(callErr, windows.ERROR_CANCELLED) {
			return ElevationDeclined
		}
		return ElevationFailed
	}
	// ShellExecuteEx can return TRUE while still failing; the legacy hInstApp field
	// carries the result, where a value <= 32 is an error code. Only a genuine
	// launch (> 32) means an elevated copy is running.
	if hInstApp <= 32 {
		return ElevationFailed
	}
	return ElevatedChildStarted
}

// joinArgs quotes each argument for the Windows command line so it survives
// re-parsing by the elevated copy (CommandLineToArgvW rules).
func joinArgs(args []string) string {
	var b strings.Builder
	for i, a := range args {
		if i > 0 {
			b.WriteByte(' ')
		}
		b.WriteString(syscall.EscapeArg(a))
	}
	return b.String()
}
