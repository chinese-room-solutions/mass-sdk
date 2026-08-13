package selfupdate

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/chinese-room-solutions/mass-sdk/registry"
	"github.com/stretchr/testify/require"
)

func TestLatest(t *testing.T) {
	tests := []struct {
		name    string
		handler http.HandlerFunc
		want    string
		wantErr error
	}{
		{
			name: "tag redirect",
			handler: func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Location", "https://github.com/owner/repo/releases/tag/v0.4.1")
				w.WriteHeader(http.StatusFound)
			},
			want: "v0.4.1",
		},
		{
			name: "relative tag redirect",
			handler: func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Location", "/owner/repo/releases/tag/v1.2.3")
				w.WriteHeader(http.StatusFound)
			},
			want: "v1.2.3",
		},
		{
			name: "no redirect at all",
			handler: func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusOK)
			},
			wantErr: ErrNoRelease,
		},
		{
			name: "404 without a location",
			handler: func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusNotFound)
			},
			wantErr: ErrNoRelease,
		},
		{
			// No published release: GitHub bounces to the releases index instead.
			name: "redirect to the releases index",
			handler: func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Location", "https://github.com/owner/repo/releases")
				w.WriteHeader(http.StatusFound)
			},
			wantErr: ErrNoRelease,
		},
		{
			name: "location pointing somewhere else entirely",
			handler: func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Location", "https://example.com/totally/unrelated")
				w.WriteHeader(http.StatusFound)
			},
			wantErr: ErrNoRelease,
		},
		{
			name: "tag-like path that is not under /releases/tag",
			handler: func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Location", "https://github.com/owner/repo/tree/tag/v0.4.1")
				w.WriteHeader(http.StatusFound)
			},
			wantErr: ErrNoRelease,
		},
		{
			name: "location with a trailing slash",
			handler: func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Location", "https://github.com/owner/repo/releases/tag/v0.9.0/")
				w.WriteHeader(http.StatusFound)
			},
			want: "v0.9.0",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				require.Equal(t, "/releases/latest", r.URL.Path)
				tt.handler(w, r)
			}))
			defer srv.Close()

			got, err := Latest(t.Context(), srv.URL)
			if tt.wantErr != nil {
				require.ErrorIs(t, err, tt.wantErr)
				return
			}
			require.NoError(t, err)
			require.Equal(t, tt.want, got)
		})
	}
}

// net/http parses the Location header before CheckRedirect ever runs, so a
// syntactically broken one fails the request rather than reaching our parser.
// Still an error, just not ErrNoRelease — asserted so the distinction is known.
func TestLatestUnparseableLocation(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Location", "not a url at all ::::")
		w.WriteHeader(http.StatusFound)
	}))
	defer srv.Close()

	_, err := Latest(t.Context(), srv.URL)
	require.Error(t, err)
}

func TestLatestUnreachable(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	url := srv.URL
	srv.Close() // nothing listening now

	_, err := Latest(t.Context(), url)
	require.Error(t, err)
}

func TestLatestHonorsContext(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		<-r.Context().Done()
	}))
	defer srv.Close()

	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	_, err := Latest(ctx, srv.URL)
	require.ErrorIs(t, err, context.Canceled)
}

func TestIsNewer(t *testing.T) {
	tests := []struct {
		name    string
		current string
		latest  string
		want    bool
	}{
		{"newer patch", "v0.4.0", "v0.4.1", true},
		{"newer minor", "v0.4.9", "v0.5.0", true},
		{"newer major", "v0.9.0", "v1.0.0", true},
		{"same version", "v0.4.1", "v0.4.1", false},
		{"older latest", "v0.4.1", "v0.4.0", false},
		{"no v prefix on either side", "0.4.0", "0.4.1", true},
		{"mixed v prefix", "v0.4.0", "0.4.1", true},

		// Dev builds must never see an update prompt.
		{"dev literal", "dev", "v0.4.1", false},
		{"git describe dirty", "v0.1.0-3-gabc-dirty", "v0.4.1", false},
		{"bare commit sha", "0eef4f3", "v0.4.1", false},
		{"empty current", "", "v0.4.1", false},
		{"prerelease current", "v0.4.1-rc.1", "v0.4.2", false},
		{"build metadata current", "v0.4.1+build.7", "v0.4.2", false},

		// An unparseable or unreleased "latest" is equally untrustworthy.
		{"garbage latest", "v0.4.0", "nightly", false},
		{"empty latest", "v0.4.0", "", false},
		{"prerelease latest", "v0.4.0", "v0.5.0-rc.1", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.want, IsNewer(tt.current, tt.latest))
		})
	}
}

// releaseServer serves a release whose SHA256SUMS lists sums and whose asset
// files come from assets. A nil sums map means SHA256SUMS is absent (404).
func releaseServer(t *testing.T, tag string, assets map[string][]byte, sums map[string]string) *httptest.Server {
	t.Helper()
	prefix := "/releases/download/" + tag + "/"
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		name, ok := strings.CutPrefix(r.URL.Path, prefix)
		if !ok {
			http.NotFound(w, r)
			return
		}
		if name == checksumsName {
			if sums == nil {
				http.NotFound(w, r)
				return
			}
			var b strings.Builder
			for asset, sum := range sums {
				fmt.Fprintf(&b, "%s  %s\n", sum, asset)
			}
			_, _ = w.Write([]byte(b.String()))
			return
		}
		body, ok := assets[name]
		if !ok {
			http.NotFound(w, r)
			return
		}
		_, _ = w.Write(body)
	}))
}

func sha256Hex(b []byte) string {
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}

func TestFetchSetup(t *testing.T) {
	const tag = "v0.4.1"
	const asset = "grimoire-setup-linux-amd64"
	body := []byte("#!/bin/sh\necho setup\n")

	t.Run("good checksum", func(t *testing.T) {
		srv := releaseServer(t, tag,
			map[string][]byte{asset: body},
			map[string]string{asset: sha256Hex(body)})
		defer srv.Close()

		dir := t.TempDir()
		got, err := FetchSetup(t.Context(), srv.URL, tag, asset, dir)
		require.NoError(t, err)
		require.Equal(t, filepath.Join(dir, asset), got)

		onDisk, err := os.ReadFile(got)
		require.NoError(t, err)
		require.Equal(t, body, onDisk)

		info, err := os.Stat(got)
		require.NoError(t, err)
		if runtime.GOOS != "windows" { // perms are ACL-governed there
			require.NotZero(t, info.Mode().Perm()&0o100, "setup binary must be executable")
		}
	})

	t.Run("bad checksum", func(t *testing.T) {
		srv := releaseServer(t, tag,
			map[string][]byte{asset: body},
			map[string]string{asset: sha256Hex([]byte("something else"))})
		defer srv.Close()

		dir := t.TempDir()
		_, err := FetchSetup(t.Context(), srv.URL, tag, asset, dir)
		require.ErrorIs(t, err, registry.ErrChecksumMismatch)
		require.NoFileExists(t, filepath.Join(dir, asset), "a mismatched asset must not be left behind")
	})

	t.Run("asset missing from SHA256SUMS", func(t *testing.T) {
		srv := releaseServer(t, tag,
			map[string][]byte{asset: body},
			map[string]string{"some-other-asset": sha256Hex(body)})
		defer srv.Close()

		_, err := FetchSetup(t.Context(), srv.URL, tag, asset, t.TempDir())
		require.ErrorIs(t, err, ErrChecksumMissing)
	})

	t.Run("SHA256SUMS missing entirely", func(t *testing.T) {
		srv := releaseServer(t, tag, map[string][]byte{asset: body}, nil)
		defer srv.Close()

		_, err := FetchSetup(t.Context(), srv.URL, tag, asset, t.TempDir())
		require.Error(t, err)
		require.Contains(t, err.Error(), checksumsName)
	})

	t.Run("listed asset absent from the release", func(t *testing.T) {
		srv := releaseServer(t, tag,
			map[string][]byte{}, // asset itself 404s
			map[string]string{asset: sha256Hex(body)})
		defer srv.Close()

		_, err := FetchSetup(t.Context(), srv.URL, tag, asset, t.TempDir())
		require.Error(t, err)
	})

	// Binary-mode sha256sum output marks the name with '*'; both forms must parse.
	t.Run("binary-mode checksum line", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if strings.HasSuffix(r.URL.Path, checksumsName) {
				_, _ = fmt.Fprintf(w, "%s *%s\n", sha256Hex(body), asset)
				return
			}
			_, _ = w.Write(body)
		}))
		defer srv.Close()

		got, err := FetchSetup(t.Context(), srv.URL, tag, asset, t.TempDir())
		require.NoError(t, err)
		require.FileExists(t, got)
	})
}

func TestReplaceable(t *testing.T) {
	t.Run("a plain file is replaceable and survives the probe", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "app")
		require.NoError(t, os.WriteFile(path, []byte("binary"), 0o755))

		require.True(t, Replaceable(path))

		got, err := os.ReadFile(path)
		require.NoError(t, err)
		require.Equal(t, "binary", string(got), "the probe must put the file back")
	})

	t.Run("a missing path is replaceable", func(t *testing.T) {
		require.True(t, Replaceable(filepath.Join(t.TempDir(), "absent")))
	})

	t.Run("an unwritable parent directory is not replaceable", func(t *testing.T) {
		if runtime.GOOS == "windows" || os.Geteuid() == 0 {
			t.Skip("directory permissions do not block rename here")
		}
		dir := t.TempDir()
		path := filepath.Join(dir, "app")
		require.NoError(t, os.WriteFile(path, []byte("binary"), 0o755))
		require.NoError(t, os.Chmod(dir, 0o500)) // r-x: no renames within
		t.Cleanup(func() { _ = os.Chmod(dir, 0o700) })

		require.False(t, Replaceable(path))
	})
}
