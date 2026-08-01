//go:build windows

package install

import (
	"testing"

	"github.com/stretchr/testify/require"
	"golang.org/x/sys/windows"
)

func TestClassifyShellExecute(t *testing.T) {
	tests := []struct {
		name     string
		ret      uintptr
		callErr  error
		hInstApp uintptr
		want     ElevationOutcome
	}{
		{"launched", 1, windows.ERROR_SUCCESS, 42, ElevatedChildStarted},
		{"UAC dismissed", 0, windows.ERROR_CANCELLED, 0, ElevationDeclined},
		{"FALSE with non-cancel error", 0, windows.ERROR_ACCESS_DENIED, 0, ElevationFailed},
		{"TRUE but legacy error code", 1, windows.ERROR_SUCCESS, 5, ElevationFailed},
		{"TRUE at the 32 boundary", 1, windows.ERROR_SUCCESS, 32, ElevationFailed},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			require.Equal(t, tc.want, classifyShellExecute(tc.ret, tc.callErr, tc.hInstApp))
		})
	}
}
