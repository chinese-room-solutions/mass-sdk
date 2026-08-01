//go:build darwin

package install

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestStagedExePath_AppBundle(t *testing.T) {
	a := AppSpec{Name: "mass", DisplayName: "MASS", ExeName: "mass"}
	tests := []struct {
		name       string
		installDir string
		want       string
	}{
		{
			name:       "app bundle puts exe under Contents/MacOS",
			installDir: "/Applications/MASS.app",
			want:       "/Applications/MASS.app/Contents/MacOS/mass",
		},
		{
			name:       "plain dir keeps exe at top",
			installDir: "/opt/mass",
			want:       "/opt/mass/mass",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.want, a.StagedExePath(tt.installDir))
		})
	}
}

func TestWriteMacBundleMetadata(t *testing.T) {
	a := AppSpec{
		Name:        "mass",
		DisplayName: "MASS",
		ExeName:     "mass",
		BundleID:    "solutions.chineseroom.mass",
	}
	bundle := filepath.Join(t.TempDir(), "MASS.app")
	require.NoError(t, os.MkdirAll(filepath.Join(bundle, "Contents", "MacOS"), 0o755))

	require.NoError(t, a.writeMacBundleMetadata(bundle))

	plist, err := os.ReadFile(filepath.Join(bundle, "Contents", "Info.plist"))
	require.NoError(t, err)
	got := string(plist)
	require.Contains(t, got, "<key>CFBundleExecutable</key><string>mass</string>")
	require.Contains(t, got, "<key>CFBundleIdentifier</key><string>solutions.chineseroom.mass</string>")
	require.Contains(t, got, "<key>CFBundleName</key><string>MASS</string>")
}

func TestWriteMacBundleMetadata_NonBundleIsNoop(t *testing.T) {
	a := AppSpec{Name: "mass", DisplayName: "MASS", ExeName: "mass"}
	dir := t.TempDir()
	require.NoError(t, a.writeMacBundleMetadata(dir))
	entries, err := os.ReadDir(dir)
	require.NoError(t, err)
	require.Empty(t, entries, "non-.app dir should get no bundle scaffolding")
}

func TestWriteMacBundleMetadata_DefaultBundleID(t *testing.T) {
	a := AppSpec{Name: "grimoire", DisplayName: "Grimoire", ExeName: "grimoire"}
	bundle := filepath.Join(t.TempDir(), "Grimoire.app")
	require.NoError(t, os.MkdirAll(bundle, 0o755))
	require.NoError(t, a.writeMacBundleMetadata(bundle))
	plist, err := os.ReadFile(filepath.Join(bundle, "Contents", "Info.plist"))
	require.NoError(t, err)
	require.Contains(t, string(plist),
		"<key>CFBundleIdentifier</key><string>com.grimoire</string>")
	require.True(t, strings.HasSuffix(strings.TrimSpace(string(plist)), "</plist>"))
}
