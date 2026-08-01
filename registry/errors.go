package registry

import "errors"

var (
	// ErrUnsupportedSchema is returned when the index declares a schema version
	// this client cannot parse.
	ErrUnsupportedSchema = errors.New("unsupported index schema version")
	// ErrNotResolved is returned when no version both covers the installed
	// version and has an artifact for the requested platform.
	ErrNotResolved = errors.New("no compatible version with an artifact for the platform")
	// ErrChecksumMismatch is returned when a downloaded artifact's sha256 does
	// not match the digest pinned in the index.
	ErrChecksumMismatch = errors.New("artifact checksum mismatch")
	// ErrPlaceholderArtifact is returned when an artifact's digest is the "TBD"
	// placeholder — its asset is unreleased and must never be installed.
	ErrPlaceholderArtifact = errors.New("artifact has a placeholder checksum")
)
