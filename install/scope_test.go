package install

import (
	"runtime"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestAvailableScopes(t *testing.T) {
	got := AvailableScopes()
	// User leads (no-elevation default) and both scopes are offered on every OS.
	require.Equal(t, []Scope{ScopeUser, ScopeSystem}, got)
}

func TestParseScope(t *testing.T) {
	valid := map[string]Scope{
		"user":   ScopeUser,
		"User":   ScopeUser,
		"USER":   ScopeUser,
		"system": ScopeSystem,
		"System": ScopeSystem,
	}
	for in, want := range valid {
		t.Run(in, func(t *testing.T) {
			got, err := ParseScope(in)
			require.NoError(t, err)
			require.Equal(t, want, got)
		})
	}

	// Unrecognized input errors instead of silently defaulting to ScopeUser.
	for _, in := range []string{"", "garbage", "all-users"} {
		t.Run("invalid "+in, func(t *testing.T) {
			_, err := ParseScope(in)
			require.Error(t, err)
			require.Contains(t, err.Error(), "unknown scope")
		})
	}
}

func TestScopeLabel(t *testing.T) {
	require.Equal(t, "User", ScopeUser.Label())
	require.Equal(t, "System", ScopeSystem.Label())
}

func TestScopeInstallDir(t *testing.T) {
	a := testApp()

	user := a.ScopeInstallDir(ScopeUser)
	system := a.ScopeInstallDir(ScopeSystem)
	require.NotEmpty(t, user)
	require.Equal(t, a.DefaultInstallDir(), system)
	// The two scopes resolve to different locations (unless home is unresolvable,
	// in which case user falls back to system — tolerated).
	if a.UserInstallDir() != "" {
		require.NotEqual(t, system, user)
		require.Equal(t, a.UserInstallDir(), user)
	}
}

func TestSystemDataDir(t *testing.T) {
	a := testApp()
	got := a.SystemDataDir()
	require.NotEmpty(t, got)
	switch runtime.GOOS {
	case "windows":
		require.Contains(t, got, "TestApp")
	case "darwin":
		require.Equal(t, "/Library/Application Support/testapp", got)
	default:
		require.Equal(t, "/var/lib/testapp", got)
	}
	// System data must not coincide with the user data root's convention.
	require.True(t, strings.HasPrefix(got, "/var/lib") ||
		strings.HasPrefix(got, "/Library") ||
		strings.Contains(got, "ProgramData"))
}
