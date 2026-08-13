package uikit

import (
	"runtime"
	"testing"
)

// pointConfigDirAtTemp redirects os.UserConfigDir at a temp dir so theme
// install/load tests can't touch the real user config. XDG_CONFIG_HOME alone is
// not enough: os.UserConfigDir reads %AppData% on Windows and ~/Library on
// macOS, where a stray theme file would leak into the developer's own config
// and break later runs.
func pointConfigDirAtTemp(t *testing.T) {
	t.Helper()
	tmp := t.TempDir()
	switch runtime.GOOS {
	case "windows":
		t.Setenv("AppData", tmp)
	case "darwin":
		t.Setenv("HOME", tmp) // ~/Library/Application Support
	default:
		t.Setenv("XDG_CONFIG_HOME", tmp)
	}
}
