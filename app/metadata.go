package app

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/KernelPryanic/ctxerr"
	"gopkg.in/yaml.v3"
)

// Metadata is the parsed contents of an app's app.yml file.
type Metadata struct {
	Name        string `yaml:"name"`
	Version     string `yaml:"version"`
	Description string `yaml:"description,omitempty"`
	SDKVersion  string `yaml:"sdk_version"`
	Command     string `yaml:"command"`
}

// ModelsDir returns the global models directory for the named app.
// MASS relocates bundled model files to {MASS_DATA_DIR}/models/{appName}/
// during installation. Apps should use this path to find their model files.
func ModelsDir(appName string) (string, error) {
	dataDir := os.Getenv("MASS_DATA_DIR")
	if dataDir == "" {
		return "", fmt.Errorf("MASS_DATA_DIR not set")
	}
	return filepath.Join(dataDir, "models", appName), nil
}

// ReadMetadata reads and parses app.yml from the directory containing the
// running executable. Apps call this at startup to populate AppInfo
// with values from the single source of truth.
func ReadMetadata() (*Metadata, error) {
	exe, err := os.Executable()
	if err != nil {
		return nil, fmt.Errorf("resolving executable path: %w", err)
	}
	dir := filepath.Dir(exe)

	for _, name := range []string{"app.yml", "app.yaml"} {
		path := filepath.Join(dir, name)
		data, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		var meta Metadata
		if err := yaml.Unmarshal(data, &meta); err != nil {
			return nil, ctxerr.With(fmt.Errorf("parsing metadata: %w", err), map[string]any{
				"path": path,
			})
		}
		return &meta, nil
	}

	return nil, ctxerr.With(fmt.Errorf("app.yml not found"), map[string]any{
		"dir": dir,
	})
}
