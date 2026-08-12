package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/chinese-room-solutions/mass-sdk/registry"
	"github.com/stretchr/testify/require"
)

// validYAML is a minimal well-formed index: a runtime, its worker, a
// platform-independent package carrying a grimoire range, and a kind the
// validator has never heard of (unknown kinds are legal by design).
const validYAML = `
schema_version: 1
packages:
  - name: mass-runtime-gateway-llama-cpp
    kind: runtime
    runtime_name: llama-cpp
    versions:
      - version: 0.1.0
        mass: ">=0.1 <0.2"
        artifacts:
          linux/amd64:
            url: https://example.test/runtime_0.1.0
            sha256: 62003d9c0750755c78475b7562f64db4692cafcb7b625deb90b6dd5b6a74643b
      - version: 0.2.0
        mass: ">=0.2"
        artifacts:
          linux/amd64:
            url: https://example.test/runtime_0.2.0
            sha256: TBD
  - name: mass-worker-llama-cpp
    kind: worker
    runtime_name: llama-cpp
    versions:
      - version: 0.1.0
        runtime: ">=0.1 <0.2"
        artifacts:
          linux/amd64/vulkan:
            url: https://example.test/worker_0.1.0
            sha256: 09ED50C32C077E72943545C0797E11FACEBBE8F5756F96AA9C9E0F41F79B9979
  - name: mass-theme-synthwave
    kind: theme
    versions:
      - version: 0.1.0
        grimoire: ">=0.1"
        artifacts:
          any:
            url: https://example.test/synthwave.css
            sha256: 7ec006ab62f1d9718447b81e8326a2ce6d225cadaa1af6605eec39effc5a67be
  - name: some-future-package
    kind: not-a-kind-this-client-knows
    versions:
      - version: "1.26"
        artifacts:
          any:
            url: https://example.test/future_1.26.zip
            sha256: 8e05702b83fc2861960249932e38c46adfaa7f1a07604509b3b3052f22cb3dd8
`

func TestValidateIndex(t *testing.T) {
	tests := []struct {
		name string
		yaml string
		// want holds a substring of each expected finding, in order. Empty means
		// the index is valid.
		want []string
	}{
		{
			name: "valid index",
			yaml: validYAML,
		},
		{
			name: "undecodable yaml",
			yaml: "schema_version: 1\npackages: [\n",
			want: []string{"parsing yaml"},
		},
		{
			name: "wrong schema version",
			yaml: `
schema_version: 2
packages: []
`,
			want: []string{"schema_version is 2, want 1"},
		},
		{
			name: "empty and duplicate package names",
			yaml: `
schema_version: 1
packages:
  - name: ""
    kind: theme
  - name: dup
    kind: theme
  - name: dup
    kind: theme
`,
			want: []string{"package #1 has an empty name", "duplicate package name"},
		},
		{
			name: "missing kind and worker runtime_name",
			yaml: `
schema_version: 1
packages:
  - name: nokind
  - name: mass-worker-orphan
    kind: worker
`,
			want: []string{"kind is empty", "kind worker requires runtime_name"},
		},
		{
			name: "non-semver version",
			yaml: `
schema_version: 1
packages:
  - name: p
    kind: theme
    versions:
      - version: "not-a-version"
        artifacts:
          any:
            url: https://example.test/a
            sha256: TBD
`,
			want: []string{`version "not-a-version" is not semver`},
		},
		{
			name: "versions out of order",
			yaml: `
schema_version: 1
packages:
  - name: p
    kind: theme
    versions:
      - version: 0.2.0
        artifacts:
          any: {url: https://example.test/a, sha256: TBD}
      - version: 0.1.0
        artifacts:
          any: {url: https://example.test/b, sha256: TBD}
      - version: 0.1.0
        artifacts:
          any: {url: https://example.test/c, sha256: TBD}
`,
			want: []string{`version "0.1.0" does not sort after "0.2.0"`, `version "0.1.0" does not sort after "0.1.0"`},
		},
		{
			name: "unparseable ranges",
			yaml: `
schema_version: 1
packages:
  - name: p
    kind: worker
    runtime_name: r
    versions:
      - version: 0.1.0
        mass: "not a range"
        runtime: ">=0.1"
        grimoire: "<<0.2"
        artifacts:
          any: {url: https://example.test/a, sha256: TBD}
`,
			want: []string{`mass range "not a range" does not parse`, `grimoire range "<<0.2" does not parse`},
		},
		{
			name: "version without artifacts",
			yaml: `
schema_version: 1
packages:
  - name: p
    kind: theme
    versions:
      - version: 0.1.0
`,
			want: []string{"has no artifacts"},
		},
		{
			name: "artifact without url and with a bad digest",
			yaml: `
schema_version: 1
packages:
  - name: p
    kind: theme
    versions:
      - version: 0.1.0
        artifacts:
          any: {url: "", sha256: deadbeef}
`,
			want: []string{"artifact any has no url", `artifact any has sha256 "deadbeef", want 64 hex characters or TBD`},
		},
		{
			name: "non-hex digest of the right length",
			yaml: `
schema_version: 1
packages:
  - name: p
    kind: theme
    versions:
      - version: 0.1.0
        artifacts:
          any: {url: https://example.test/a, sha256: zzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzz}
`,
			want: []string{"want 64 hex characters or TBD"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, findings := validateIndex("index.yml", []byte(tt.yaml))
			require.Len(t, findings, len(tt.want), "findings: %v", findings)
			for i, want := range tt.want {
				require.Contains(t, findings[i].String(), want)
			}
		})
	}
}

func TestFindingString(t *testing.T) {
	tests := []struct {
		name    string
		finding Finding
		want    string
	}{
		{
			name:    "file only",
			finding: Finding{File: "index.yml", Problem: "schema_version is 2, want 1"},
			want:    "index.yml: schema_version is 2, want 1",
		},
		{
			name:    "package level",
			finding: Finding{File: "index.yml", Package: "p", Problem: "kind is empty"},
			want:    "index.yml: p: kind is empty",
		},
		{
			name:    "version level",
			finding: Finding{File: "index.yml", Package: "p", Version: "0.1.0", Problem: "has no artifacts"},
			want:    "index.yml: p@0.1.0: has no artifacts",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.want, tt.finding.String())
		})
	}
}

func TestCountVersions(t *testing.T) {
	idx, findings := validateIndex("index.yml", []byte(validYAML))
	require.Empty(t, findings)
	require.Equal(t, 5, countVersions(idx))
}

func TestVerifyDigests(t *testing.T) {
	body := []byte("artifact bytes")
	sum := sha256.Sum256(body)
	digest := hex.EncodeToString(sum[:])

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/missing" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		_, _ = w.Write(body)
	}))
	t.Cleanup(srv.Close)

	tests := []struct {
		name   string
		sha    string
		path   string
		want   string
		wantOK bool
	}{
		{name: "digest matches", sha: digest, path: "/ok", wantOK: true},
		{name: "placeholder is skipped", sha: "TBD", path: "/ok", wantOK: true},
		{
			name: "digest mismatch",
			sha:  "0000000000000000000000000000000000000000000000000000000000000000",
			path: "/ok",
			want: "sha256 mismatch",
		},
		{name: "asset missing", sha: digest, path: "/missing", want: "unexpected status 404"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			idx := &registry.Index{
				SchemaVersion: 1,
				Packages: []registry.Package{{
					Name: "p", Kind: "theme",
					Versions: []registry.Version{{
						Version: "0.1.0",
						Artifacts: map[string]registry.Artifact{
							"any": {URL: srv.URL + tt.path, SHA256: tt.sha},
						},
					}},
				}},
			}
			findings := verifyDigests(context.Background(), "index.yml", idx)
			if tt.wantOK {
				require.Empty(t, findings)
				return
			}
			require.Len(t, findings, 1)
			require.Contains(t, findings[0].String(), tt.want)
			require.Contains(t, findings[0].String(), "index.yml: p@0.1.0: artifact any")
		})
	}
}
