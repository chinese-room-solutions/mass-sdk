package registry

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestParseIndex(t *testing.T) {
	tests := []struct {
		name    string
		yaml    string
		wantErr error
		check   func(t *testing.T, idx *Index)
	}{
		{
			name: "valid",
			yaml: testIndexYAML,
			check: func(t *testing.T, idx *Index) {
				require.Equal(t, 1, idx.SchemaVersion)
				require.Len(t, idx.Packages, 3)
				rt := idx.FindPackage("mass-runtime-gateway-llama-cpp")
				require.NotNil(t, rt)
				require.Equal(t, KindRuntime, rt.Kind)
				require.Equal(t, "llama-cpp", rt.RuntimeName)
				require.Len(t, rt.Versions, 2)
				art := rt.Versions[0].Artifacts["linux/amd64"]
				require.Equal(t, "https://example.test/runtime/0.1.0/linux_amd64.mass", art.URL)
			},
		},
		{
			name:    "wrong schema version",
			yaml:    "schema_version: 2\npackages: []\n",
			wantErr: ErrUnsupportedSchema,
		},
		{
			name:    "missing schema version defaults to zero and is rejected",
			yaml:    "packages: []\n",
			wantErr: ErrUnsupportedSchema,
		},
		{
			name:    "malformed yaml",
			yaml:    "schema_version: 1\npackages: [oops\n",
			wantErr: nil, // parse error, not a sentinel
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			idx, err := ParseIndex([]byte(tt.yaml))
			if tt.name == "malformed yaml" {
				require.Error(t, err)
				return
			}
			if tt.wantErr != nil {
				require.ErrorIs(t, err, tt.wantErr)
				return
			}
			require.NoError(t, err)
			require.NotNil(t, idx)
			if tt.check != nil {
				tt.check(t, idx)
			}
		})
	}
}

func TestArtifactIsPlaceholder(t *testing.T) {
	require.True(t, Artifact{SHA256: "TBD"}.IsPlaceholder())
	require.False(t, Artifact{SHA256: "abcd"}.IsPlaceholder())
}

func TestErrUnsupportedSchemaWrapped(t *testing.T) {
	_, err := ParseIndex([]byte("schema_version: 9\npackages: []\n"))
	require.True(t, errors.Is(err, ErrUnsupportedSchema))
}
