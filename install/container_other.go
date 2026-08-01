//go:build !linux && !darwin

package install

import (
	"fmt"
	"runtime"
)

// buildContainer is unsupported off Linux/macOS: a Windows console binary already
// gets a terminal from the OS, so no double-click wrapper is needed there.
func buildContainer(_ ContainerSpec) (string, error) {
	return "", fmt.Errorf("install: BuildContainer is not supported on %s", runtime.GOOS)
}
