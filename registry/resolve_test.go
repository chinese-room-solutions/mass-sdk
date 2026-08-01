package registry

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func testIndex(t *testing.T) *Index {
	t.Helper()
	idx, err := ParseIndex([]byte(testIndexYAML))
	require.NoError(t, err)
	return idx
}

func TestPlatformKey(t *testing.T) {
	require.Equal(t, "linux/amd64", RuntimePlatform("linux", "amd64").Key())
	require.Equal(t, "linux/amd64/vulkan", WorkerPlatform("linux", "amd64", "vulkan").Key())
}

func TestResolveRuntime(t *testing.T) {
	idx := testIndex(t)
	tests := []struct {
		name        string
		pkg         string
		platform    Platform
		massVersion string
		wantVersion string
		wantErr     bool
		plainErr    bool // wantErr but not the ErrNotResolved sentinel
	}{
		{
			name:        "mass 0.1 gets 0.1.0 only",
			pkg:         "mass-runtime-gateway-llama-cpp",
			platform:    RuntimePlatform("linux", "amd64"),
			massVersion: "0.1.3",
			wantVersion: "0.1.0",
		},
		{
			name:        "mass 0.2 gets newest covering 0.2.0",
			pkg:         "mass-runtime-gateway-llama-cpp",
			platform:    RuntimePlatform("linux", "amd64"),
			massVersion: "0.2.1",
			wantVersion: "0.2.0",
		},
		{
			name:        "platform miss: 0.2.0 has no darwin artifact, 0.1.0 out of range for mass 0.2",
			pkg:         "mass-runtime-gateway-llama-cpp",
			platform:    RuntimePlatform("darwin", "arm64"),
			massVersion: "0.2.0",
			wantErr:     true,
		},
		{
			name:        "darwin arm64 resolves 0.1.0 under mass 0.1",
			pkg:         "mass-runtime-gateway-llama-cpp",
			platform:    RuntimePlatform("darwin", "arm64"),
			massVersion: "0.1.0",
			wantVersion: "0.1.0",
		},
		{
			name:        "no version covers mass 0.9 on darwin",
			pkg:         "mass-runtime-gateway-llama-cpp",
			platform:    RuntimePlatform("darwin", "arm64"),
			massVersion: "0.9.0",
			wantErr:     true,
		},
		{
			name:        "unknown platform arch",
			pkg:         "mass-runtime-gateway-llama-cpp",
			platform:    RuntimePlatform("linux", "arm64"),
			massVersion: "0.1.0",
			wantErr:     true,
		},
		{
			name:        "unknown package",
			pkg:         "nope",
			platform:    RuntimePlatform("linux", "amd64"),
			massVersion: "0.1.0",
			wantErr:     true,
		},
		{
			name:        "worker package rejected by ResolveRuntime",
			pkg:         "mass-worker-llama-cpp",
			platform:    RuntimePlatform("linux", "amd64"),
			massVersion: "0.1.0",
			wantErr:     true,
		},
		{
			name:        "bad installed version",
			pkg:         "mass-runtime-gateway-llama-cpp",
			platform:    RuntimePlatform("linux", "amd64"),
			massVersion: "not-a-version",
			wantErr:     true,
			plainErr:    true, // caller input error, not ErrNotResolved
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			res, err := idx.ResolveRuntime(tt.pkg, tt.platform, tt.massVersion)
			if tt.wantErr {
				require.Error(t, err)
				if !tt.plainErr {
					require.ErrorIs(t, err, ErrNotResolved)
				}
				return
			}
			require.NoError(t, err)
			require.Equal(t, tt.wantVersion, res.Version.Version)
			require.NotEmpty(t, res.Artifact.URL)
		})
	}
}

func TestResolveWorker(t *testing.T) {
	idx := testIndex(t)
	tests := []struct {
		name           string
		pkg            string
		platform       Platform
		runtimeVersion string
		massVersion    string
		wantVersion    string
		wantErr        bool
		plainErr       bool // wantErr but not the ErrNotResolved sentinel
	}{
		{
			// 0.1.5 needs mass >=0.3; under mass 0.1 only 0.1.0 (mass >=0.1) survives.
			name:           "mass range excludes newest, picks 0.1.0",
			pkg:            "mass-worker-llama-cpp",
			platform:       WorkerPlatform("linux", "amd64", "vulkan"),
			runtimeVersion: "0.1.0",
			massVersion:    "0.1.0",
			wantVersion:    "0.1.0",
		},
		{
			// Under mass 0.3 both mass ranges pass; newest 0.1.5 wins.
			name:           "mass 0.3 admits newest 0.1.5",
			pkg:            "mass-worker-llama-cpp",
			platform:       WorkerPlatform("linux", "amd64", "vulkan"),
			runtimeVersion: "0.1.0",
			massVersion:    "0.3.0",
			wantVersion:    "0.1.5",
		},
		{
			// Both worker versions' mass ranges exclude mass 0.0.
			name:           "mass too old excludes every version",
			pkg:            "mass-worker-llama-cpp",
			platform:       WorkerPlatform("linux", "amd64", "vulkan"),
			runtimeVersion: "0.1.0",
			massVersion:    "0.0.1",
			wantErr:        true,
		},
		{
			// The extra package has no mass range: unconstrained, resolves regardless.
			name:           "empty mass range passes any mass version",
			pkg:            "mass-worker-llama-cpp-extra",
			platform:       WorkerPlatform("linux", "amd64", "vulkan"),
			runtimeVersion: "0.1.0",
			massVersion:    "0.0.1",
			wantVersion:    "0.2.0",
		},
		{
			name:           "runtime 0.2 out of every worker runtime range",
			pkg:            "mass-worker-llama-cpp",
			platform:       WorkerPlatform("linux", "amd64", "vulkan"),
			runtimeVersion: "0.2.0",
			massVersion:    "0.3.0",
			wantErr:        true,
		},
		{
			name:           "backend miss",
			pkg:            "mass-worker-llama-cpp",
			platform:       WorkerPlatform("linux", "amd64", "cuda"),
			runtimeVersion: "0.1.0",
			massVersion:    "0.3.0",
			wantErr:        true,
		},
		{
			name:           "runtime package rejected by ResolveWorker",
			pkg:            "mass-runtime-gateway-llama-cpp",
			platform:       WorkerPlatform("linux", "amd64", "vulkan"),
			runtimeVersion: "0.1.0",
			massVersion:    "0.3.0",
			wantErr:        true,
		},
		{
			name:           "bad mass version",
			pkg:            "mass-worker-llama-cpp",
			platform:       WorkerPlatform("linux", "amd64", "vulkan"),
			runtimeVersion: "0.1.0",
			massVersion:    "not-a-version",
			wantErr:        true,
			plainErr:       true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			res, err := idx.ResolveWorker(tt.pkg, tt.platform, tt.runtimeVersion, tt.massVersion)
			if tt.wantErr {
				require.Error(t, err)
				if !tt.plainErr {
					require.ErrorIs(t, err, ErrNotResolved)
				}
				return
			}
			require.NoError(t, err)
			require.Equal(t, tt.wantVersion, res.Version.Version)
		})
	}
}

func TestCompatibleWorkers(t *testing.T) {
	idx := testIndex(t)

	// Under mass 0.3 every worker version's mass range is satisfied: both versions
	// of mass-worker-llama-cpp plus the extra package's single version, ordered by
	// package then newest version first.
	got, err := idx.CompatibleWorkers("llama-cpp", "0.1.2", "0.3.0")
	require.NoError(t, err)
	require.Len(t, got, 3)
	require.Equal(t, "mass-worker-llama-cpp", got[0].Package.Name)
	require.Equal(t, "0.1.5", got[0].Version.Version)
	require.Equal(t, "mass-worker-llama-cpp", got[1].Package.Name)
	require.Equal(t, "0.1.0", got[1].Version.Version)
	require.Equal(t, "mass-worker-llama-cpp-extra", got[2].Package.Name)
	require.Equal(t, "0.2.0", got[2].Version.Version)

	// Under mass 0.1 the 0.1.5 version (mass >=0.3) drops out; 0.1.0 and the
	// unconstrained extra remain.
	old, err := idx.CompatibleWorkers("llama-cpp", "0.1.2", "0.1.0")
	require.NoError(t, err)
	require.Len(t, old, 2)
	require.Equal(t, "0.1.0", old[0].Version.Version)
	require.Equal(t, "mass-worker-llama-cpp-extra", old[1].Package.Name)

	// Runtime out of every worker range yields nothing.
	none, err := idx.CompatibleWorkers("llama-cpp", "0.9.0", "0.3.0")
	require.NoError(t, err)
	require.Empty(t, none)

	// Unknown runtime name yields nothing.
	unknown, err := idx.CompatibleWorkers("no-such-runtime", "0.1.0", "0.3.0")
	require.NoError(t, err)
	require.Empty(t, unknown)

	// Bad runtime version is an error.
	_, err = idx.CompatibleWorkers("llama-cpp", "bad", "0.3.0")
	require.Error(t, err)

	// Bad mass version is an error.
	_, err = idx.CompatibleWorkers("llama-cpp", "0.1.2", "bad")
	require.Error(t, err)
}

func TestWorkerPackagesFor(t *testing.T) {
	idx := testIndex(t)
	tests := []struct {
		name        string
		runtimeName string
		wantNames   []string
	}{
		{
			name:        "two worker packages, sorted by name",
			runtimeName: "llama-cpp",
			wantNames:   []string{"mass-worker-llama-cpp", "mass-worker-llama-cpp-extra"},
		},
		{
			name:        "no worker packages for unknown runtime",
			runtimeName: "no-such-runtime",
			wantNames:   nil,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := idx.WorkerPackagesFor(tt.runtimeName)
			var names []string
			for _, p := range got {
				names = append(names, p.Name)
			}
			require.Equal(t, tt.wantNames, names)
		})
	}
}

func TestWorkerPackagesForSingle(t *testing.T) {
	idx, err := ParseIndex([]byte(`
schema_version: 1
packages:
  - name: only-worker
    kind: worker
    runtime_name: solo
    versions:
      - version: 0.1.0
        runtime: ">=0.1"
        artifacts:
          linux/amd64/vulkan:
            url: https://example.test/w
            sha256: aaaa
`))
	require.NoError(t, err)
	got := idx.WorkerPackagesFor("solo")
	require.Len(t, got, 1)
	require.Equal(t, "only-worker", got[0].Name)
}

func TestSearch(t *testing.T) {
	idx := testIndex(t)
	tests := []struct {
		name      string
		opts      SearchOptions
		wantNames []string
	}{
		{
			name:      "no filters returns all",
			opts:      SearchOptions{},
			wantNames: []string{"mass-runtime-gateway-llama-cpp", "mass-worker-llama-cpp", "mass-worker-llama-cpp-extra"},
		},
		{
			name:      "kind runtime",
			opts:      SearchOptions{Kind: KindRuntime},
			wantNames: []string{"mass-runtime-gateway-llama-cpp"},
		},
		{
			name:      "kind worker",
			opts:      SearchOptions{Kind: KindWorker},
			wantNames: []string{"mass-worker-llama-cpp", "mass-worker-llama-cpp-extra"},
		},
		{
			name:      "query matches name case-insensitively",
			opts:      SearchOptions{Query: "MASS-WORKER"},
			wantNames: []string{"mass-worker-llama-cpp", "mass-worker-llama-cpp-extra"},
		},
		{
			name:      "query matches description",
			opts:      SearchOptions{Query: "Vulkan backend"},
			wantNames: []string{"mass-worker-llama-cpp"},
		},
		{
			name:      "query matches runtime description only",
			opts:      SearchOptions{Query: "gateway"},
			wantNames: []string{"mass-runtime-gateway-llama-cpp"},
		},
		{
			name:      "query matches name substring on all",
			opts:      SearchOptions{Query: "llama-cpp"},
			wantNames: []string{"mass-runtime-gateway-llama-cpp", "mass-worker-llama-cpp", "mass-worker-llama-cpp-extra"},
		},
		{
			name:      "runtime name filter",
			opts:      SearchOptions{RuntimeName: "llama-cpp", Kind: KindWorker},
			wantNames: []string{"mass-worker-llama-cpp", "mass-worker-llama-cpp-extra"},
		},
		{
			name:      "no match",
			opts:      SearchOptions{Query: "nonexistent"},
			wantNames: nil,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := idx.Search(tt.opts)
			var names []string
			for _, p := range got {
				names = append(names, p.Name)
			}
			require.Equal(t, tt.wantNames, names)
		})
	}
}
