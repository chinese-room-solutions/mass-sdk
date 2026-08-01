//go:build windows

package install

import "golang.org/x/sys/windows"

// isElevated reports whether the process token is elevated (running as
// Administrator). Uses the token's elevation flag, which reflects the actual
// elevation state under UAC rather than mere group membership.
func isElevated() bool {
	var token windows.Token
	if err := windows.OpenProcessToken(windows.CurrentProcess(), windows.TOKEN_QUERY, &token); err != nil {
		return false
	}
	defer token.Close() //nolint:errcheck // teardown, no recovery
	return token.IsElevated()
}
