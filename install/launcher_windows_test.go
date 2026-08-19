//go:build windows

package install

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestWindowsIconSource(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		icon string
		want string
	}{
		{name: "ico is used as given", icon: `C:\Programs\MASS\app.ico`, want: `C:\Programs\MASS\app.ico`},
		{name: "exe is an icon source too", icon: `C:\Windows\explorer.exe`, want: `C:\Windows\explorer.exe`},
		{name: "dll is an icon source too", icon: `C:\Windows\System32\shell32.dll`, want: `C:\Windows\System32\shell32.dll`},
		{name: "extension case is ignored", icon: `C:\Programs\MASS\App.ICO`, want: `C:\Programs\MASS\App.ICO`},
		{name: "png falls back to the exe icon", icon: `C:\Temp\mass-setup-icon.png`, want: ""},
		{name: "empty stays empty", icon: "", want: ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			require.Equal(t, tt.want, windowsIconSource(tt.icon))
		})
	}
}
