//go:build windows

package install

import (
	"fmt"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/registry"
)

// linkOnPath exposes the app on PATH by appending its install directory to the
// persistent PATH environment variable — user scope writes HKCU\Environment,
// system/elevated scope writes the machine environment under
// HKLM\...\Session Manager\Environment — then broadcasts WM_SETTINGCHANGE so
// newly-launched shells pick it up.
//
// It read-modify-writes the value (NEVER setx, which truncates a PATH over 1024
// chars) and preserves the value's type: a REG_EXPAND_SZ PATH (with %vars%) is
// rewritten as REG_EXPAND_SZ so the variables still expand. Idempotent: if the
// dir is already present the value is left untouched.
func (a AppSpec) linkOnPath(stagedExe string, scope Scope) (CLIResult, error) {
	dir := installDirOf(stagedExe)
	key, err := openEnvKey(scope, registry.QUERY_VALUE|registry.SET_VALUE)
	if err != nil {
		return CLIResult{}, err
	}
	defer key.Close() //nolint:errcheck // registry handle, close error is moot

	cur, valType, err := readPath(key)
	if err != nil {
		return CLIResult{}, err
	}
	if pathContains(cur, dir) {
		// Already on PATH — nothing to write, and record nothing new (uninstall of a
		// dir we didn't add would wrongly strip a pre-existing entry).
		return CLIResult{OnPath: true}, nil
	}

	next := pathAppend(cur, dir)
	if err := writePath(key, next, valType); err != nil {
		return CLIResult{}, err
	}
	broadcastEnvChange()

	return CLIResult{
		Created: dir,
		OnPath:  true,
		Hint:    fmt.Sprintf("Open a new terminal to run %s (PATH was updated for %s).", a.ExeName, envScopeLabel(scope)),
	}, nil
}

// unlinkFromPath removes exactly the install dir we appended (recorded in
// created) from the persistent PATH, then broadcasts the change. An empty created
// is a no-op (we added nothing, e.g. the dir was already present at install).
func (a AppSpec) unlinkFromPath(created, _ string, scope Scope) error {
	if created == "" {
		return nil
	}
	key, err := openEnvKey(scope, registry.QUERY_VALUE|registry.SET_VALUE)
	if err != nil {
		return err
	}
	defer key.Close() //nolint:errcheck // registry handle, close error is moot

	cur, valType, err := readPath(key)
	if err != nil {
		return err
	}
	next := pathRemove(cur, created)
	if next == cur {
		return nil // not present — nothing to write
	}
	if err := writePath(key, next, valType); err != nil {
		return err
	}
	broadcastEnvChange()
	return nil
}

// openEnvKey opens the environment registry key for the scope with the given
// access. User scope → HKCU\Environment; system scope → the machine environment
// under HKLM\...\Session Manager\Environment (needs elevation to write).
func openEnvKey(scope Scope, access uint32) (registry.Key, error) {
	if scope == ScopeSystem {
		const sys = `SYSTEM\CurrentControlSet\Control\Session Manager\Environment`
		k, err := registry.OpenKey(registry.LOCAL_MACHINE, sys, access)
		if err != nil {
			return 0, fmt.Errorf("install: opening system environment key: %w", err)
		}
		return k, nil
	}
	k, err := registry.OpenKey(registry.CURRENT_USER, "Environment", access)
	if err != nil {
		return 0, fmt.Errorf("install: opening user environment key: %w", err)
	}
	return k, nil
}

// readPath reads the Path value and its registry type. A missing value is treated
// as an empty REG_EXPAND_SZ (the correct type for a PATH, which may hold %vars%),
// so a first write creates it correctly.
func readPath(key registry.Key) (val string, valType uint32, err error) {
	val, valType, err = key.GetStringValue("Path")
	if err == registry.ErrNotExist {
		return "", registry.EXPAND_SZ, nil
	}
	if err != nil {
		return "", 0, fmt.Errorf("install: reading Path value: %w", err)
	}
	// Only REG_SZ and REG_EXPAND_SZ are valid PATH types; anything else we coerce
	// to REG_EXPAND_SZ on write to keep %var% expansion working.
	if valType != registry.SZ && valType != registry.EXPAND_SZ {
		valType = registry.EXPAND_SZ
	}
	return val, valType, nil
}

// writePath writes the Path value back preserving its registry type (REG_SZ or
// REG_EXPAND_SZ), so a value with %vars% keeps expanding.
func writePath(key registry.Key, val string, valType uint32) error {
	var err error
	if valType == registry.SZ {
		err = key.SetStringValue("Path", val)
	} else {
		err = key.SetExpandStringValue("Path", val)
	}
	if err != nil {
		return fmt.Errorf("install: writing Path value: %w", err)
	}
	return nil
}

// envScopeLabel is the human phrase naming which environment was changed.
func envScopeLabel(scope Scope) string {
	if scope == ScopeSystem {
		return "all users"
	}
	return "your account"
}

var (
	moduser32              = windows.NewLazySystemDLL("user32.dll")
	procSendMessageTimeout = moduser32.NewProc("SendMessageTimeoutW")
)

const (
	hwndBroadcast   = 0xffff // HWND_BROADCAST
	wmSettingChange = 0x001A // WM_SETTINGCHANGE
	smtoAbortIfHung = 0x0002 // SMTO_ABORTIFHUNG
	broadcastTimeMS = 5000   // per-window timeout for the broadcast
)

// broadcastEnvChange notifies running processes (Explorer, new shells) that the
// environment changed, so a freshly-opened terminal sees the new PATH without a
// logout. Best-effort: a hung listener or failure doesn't undo the registry
// write (the change is persisted; only the live notification is missed).
func broadcastEnvChange() {
	env, err := syscall.UTF16PtrFromString("Environment")
	if err != nil {
		return
	}
	_, _, _ = procSendMessageTimeout.Call(
		uintptr(hwndBroadcast),
		uintptr(wmSettingChange),
		0,
		uintptr(unsafe.Pointer(env)),
		uintptr(smtoAbortIfHung),
		uintptr(broadcastTimeMS),
		0, // lpdwResult — we don't need the reply
	)
}
