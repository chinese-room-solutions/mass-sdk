//go:build !darwin

package install

// writeMacBundleMetadata is macOS-only; everywhere else staging needs no bundle
// scaffolding, so this is a no-op.
func (a AppSpec) writeMacBundleMetadata(_ string) error { return nil }
