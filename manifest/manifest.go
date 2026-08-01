// Package manifest reads a runtime gateway's runtime.yml — the single
// source of truth for its name, version, display name, and
// description. MASS extracts the .mass package to
// <runtimesDir>/<runtimeName>/ with runtime.yml at the root and the
// binary at bin/<binary>. Gateways re-surface these values via the
// Init RPC so MASS shows the same version before and after launch.
package manifest

import (
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

// Manifest is the read-only view a gateway needs of its runtime.yml.
// MASS's internal type carries additional fields (Binary, AutoStart) that
// gateways don't need to know about.
type Manifest struct {
	RuntimeName string `yaml:"runtime_name"`
	Version     string `yaml:"version"`
	DisplayName string `yaml:"display_name"`
	Description string `yaml:"description,omitempty"`
}

// LoadAdjacentToBinary reads runtime.yml from the directory above the
// running binary — matching MASS's install layout (manifest at the install
// root, binary in bin/). Returns an error if runtime.yml is missing,
// unreadable, or empty in a required field.
func LoadAdjacentToBinary() (Manifest, error) {
	exe, err := os.Executable()
	if err != nil {
		return Manifest{}, fmt.Errorf("locating own executable: %w", err)
	}
	root := filepath.Dir(filepath.Dir(exe))
	return Load(filepath.Join(root, "runtime.yml"))
}

// Load reads and parses runtime.yml at the given path.
func Load(path string) (Manifest, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Manifest{}, fmt.Errorf("reading %s: %w", path, err)
	}
	var m Manifest
	if err := yaml.Unmarshal(data, &m); err != nil {
		return Manifest{}, fmt.Errorf("parsing %s: %w", path, err)
	}
	if m.RuntimeName == "" {
		return Manifest{}, fmt.Errorf("%s: runtime_name required", path)
	}
	if m.Version == "" {
		return Manifest{}, fmt.Errorf("%s: version required", path)
	}
	return m, nil
}
