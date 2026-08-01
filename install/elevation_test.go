package install

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestIsUserScoped(t *testing.T) {
	home, err := os.UserHomeDir()
	require.NoError(t, err)

	type tc struct {
		name string
		dir  string
		want bool
	}
	tests := []tc{
		{"home itself", home, true},
		{"subdir of home", filepath.Join(home, ".local", "lib", "app"), true},
		{"deep subdir", filepath.Join(home, "a", "b", "c"), true},
		{"home with trailing dot", filepath.Join(home, "."), true},
		{"sibling of home is not scoped", filepath.Join(filepath.Dir(home), "someone-else"), false},
		{"escape via dotdot lands outside", filepath.Join(home, "..", "elsewhere"), false},
	}
	if runtime.GOOS != "windows" {
		tests = append(tests,
			tc{"system /opt", "/opt/app", false},
			tc{"system /usr/local", "/usr/local/app", false},
		)
	}

	for _, c := range tests {
		t.Run(c.name, func(t *testing.T) {
			require.Equal(t, c.want, IsUserScoped(c.dir))
		})
	}
}

// A home-prefix string match (not path-aware) would wrongly classify a sibling
// like "<home>-backup" as inside home. Guard against that regression.
func TestIsUserScoped_SiblingPrefixNotScoped(t *testing.T) {
	home, err := os.UserHomeDir()
	require.NoError(t, err)
	require.False(t, IsUserScoped(home+"-backup"))
}

// A relative path is resolved against the working directory, so scoping follows
// cwd: under home → scoped, outside home → not. Chdir to a known location to
// pin the behavior deterministically.
func TestIsUserScoped_RelativeFollowsCwd(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("absolute non-home temp roots aren't guaranteed outside home on Windows")
	}
	t.Chdir(t.TempDir()) // t.TempDir() is under the system temp root, outside home
	require.False(t, IsUserScoped("relative/dir"))

	home, err := os.UserHomeDir()
	require.NoError(t, err)
	t.Chdir(home)
	require.True(t, IsUserScoped("relative/dir"))
}

func TestNeedsElevation(t *testing.T) {
	// A freshly created, user-owned temp dir is user-writable: no elevation. It's
	// also typically under the home/tmp tree, but even when it isn't, CanWriteDir
	// makes the gate false — exercising the "dir the user owns" branch.
	t.Run("writable user dir needs none", func(t *testing.T) {
		require.False(t, NeedsElevation(t.TempDir()))
	})

	// A dir under home is user-scoped → no elevation regardless of existence.
	t.Run("user-scoped path needs none", func(t *testing.T) {
		home, err := os.UserHomeDir()
		require.NoError(t, err)
		require.False(t, NeedsElevation(filepath.Join(home, ".local", "lib", "app-x")))
	})

	// A machine-wide-style path the unprivileged process can't write needs
	// elevation. Build the unwritable dir instead of assuming one exists —
	// /opt and /usr/local are runner-writable on GitHub-hosted images. Skip
	// as root (mode bits don't bind root) and on Windows (chmod doesn't
	// model ACLs there).
	if runtime.GOOS != "windows" && os.Geteuid() != 0 {
		t.Run("unwritable system path needs elevation", func(t *testing.T) {
			base := t.TempDir()
			if IsUserScoped(base) {
				t.Skip("temp root is under home (TMPDIR override); no non-user-scoped dir to test with")
			}
			locked := filepath.Join(base, "locked")
			require.NoError(t, os.Mkdir(locked, 0o555))
			t.Cleanup(func() { _ = os.Chmod(locked, 0o755) })
			require.True(t, NeedsElevation(filepath.Join(locked, "app")))
		})
	}
}

func TestElevationOutcomeString(t *testing.T) {
	tests := []struct {
		outcome ElevationOutcome
		want    string
	}{
		{ElevationFailed, "elevation failed"},
		{ElevationDeclined, "elevation declined"},
		{ElevatedChildStarted, "elevated child started"},
		{ElevatedWorkSucceeded, "elevated work succeeded"},
	}
	for _, tc := range tests {
		t.Run(tc.want, func(t *testing.T) {
			require.Equal(t, tc.want, tc.outcome.String())
		})
	}
}
