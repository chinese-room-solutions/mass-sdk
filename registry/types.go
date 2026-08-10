// Package registry is a client for the MASS package index (mass-registry):
// fetching the index with conditional caching, resolving the newest compatible
// version+artifact for a platform, and downloading artifacts with sha256
// verification. The index schema is documented in the mass-registry repo.
package registry

import (
	"fmt"

	"github.com/KernelPryanic/ctxerr"
	"gopkg.in/yaml.v3"
)

// SchemaVersion is the only index schema version this client understands.
const SchemaVersion = 1

// Kind is a package kind. It is an open string: consumers declare the kinds
// they install, and a kind this client does not know is not an error.
type Kind string

const (
	// KindRuntime is a runtime gateway package. Its versions carry mass and its
	// artifacts are keyed os/arch.
	KindRuntime Kind = "runtime"
	// KindWorker is a worker package. Its versions carry runtime (and optionally
	// mass) and its artifacts are keyed os/arch/backend.
	KindWorker Kind = "worker"
)

// Index is the parsed index.yml.
type Index struct {
	SchemaVersion int       `yaml:"schema_version"`
	Packages      []Package `yaml:"packages"`
}

// Package is one entry in the index: a runtime or worker identified by name and
// joined to its counterparts by RuntimeName.
type Package struct {
	Name        string    `yaml:"name"`
	Kind        Kind      `yaml:"kind"`
	RuntimeName string    `yaml:"runtime_name"`
	DisplayName string    `yaml:"display_name"`
	Description string    `yaml:"description"`
	Versions    []Version `yaml:"versions"`
}

// Version is a released semver of a package with its per-platform artifacts.
// Mass is a range over MASS server versions this version works with — required
// on runtime versions, optional on worker versions (empty means unconstrained).
// Runtime is set on worker versions only: a range over runtime versions whose
// payloads the worker decodes. Grimoire is a range over Grimoire core versions
// this version works with (empty means unconstrained). Artifacts is keyed by
// platform: os/arch for runtimes, os/arch/backend for workers, "any" for
// packages that ship one platform-independent asset.
type Version struct {
	Version   string              `yaml:"version"`
	Mass      string              `yaml:"mass,omitempty"`
	Runtime   string              `yaml:"runtime,omitempty"`
	Grimoire  string              `yaml:"grimoire,omitempty"`
	Artifacts map[string]Artifact `yaml:"artifacts"`
}

// Artifact is a single downloadable asset. SHA256 may be the literal "TBD" for
// an unreleased asset; Download refuses those.
type Artifact struct {
	URL    string `yaml:"url"`
	SHA256 string `yaml:"sha256"`
}

// PlaceholderSHA256 marks an artifact whose real digest is not yet published.
// Download refuses to install a placeholder artifact.
const PlaceholderSHA256 = "TBD"

// IsPlaceholder reports whether the artifact's digest is the unreleased
// placeholder rather than a real sha256.
func (a Artifact) IsPlaceholder() bool { return a.SHA256 == PlaceholderSHA256 }

// ParseIndex decodes index.yml and rejects any schema version this client does
// not understand.
func ParseIndex(data []byte) (*Index, error) {
	var idx Index
	if err := yaml.Unmarshal(data, &idx); err != nil {
		return nil, ctxerr.With(fmt.Errorf("parsing index: %w", err), nil)
	}
	if idx.SchemaVersion != SchemaVersion {
		return nil, ctxerr.With(
			fmt.Errorf("%w: index has schema_version %d, this client supports %d",
				ErrUnsupportedSchema, idx.SchemaVersion, SchemaVersion),
			map[string]any{"got": idx.SchemaVersion, "want": SchemaVersion},
		)
	}
	return &idx, nil
}
