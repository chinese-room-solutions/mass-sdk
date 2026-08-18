// Package selfupdate lets an installed MASS app find, fetch, and install a newer
// release of itself from GitHub.
//
// Apps use the two types: [Checker] keeps the answer to "is there a newer
// release?", refreshed in the background and on demand, and [Applier] installs
// one by re-running the app's own setup binary over its recorded install.
//
// Underneath, [Latest] reads the newest published tag from the redirect that
// /releases/latest serves, [IsNewer] decides whether that tag beats the running
// build, and [FetchSetup] downloads the release's setup binary, refusing to hand
// back anything whose SHA256SUMS entry does not match. The setup binary's own
// --relaunch half is [WaitReplaceable] plus [StartApp], which poll [Replaceable]
// on Windows, where the old binaries must have exited before they can be renamed
// over.
package selfupdate

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"strings"
	"time"

	"github.com/KernelPryanic/ctxerr"
	"github.com/Masterminds/semver/v3"
	"github.com/chinese-room-solutions/mass-sdk/registry"
)

// checksumsName is the checksum manifest every MASS release publishes beside its
// assets, in the `sha256␠␠name` format sha256sum writes.
const checksumsName = "SHA256SUMS"

// defaultTimeout caps the metadata requests — the redirect probe and the
// checksum manifest — which are both small and quick. The asset download runs
// through registry.Download, which is deliberately untimed because a setup
// binary is tens of megabytes on an arbitrarily slow link.
const defaultTimeout = 30 * time.Second

// maxChecksumsSize bounds the manifest read: a release lists a handful of
// assets, so anything larger is not a SHA256SUMS and must not be buffered.
const maxChecksumsSize = 1 << 20

var (
	// ErrNoRelease is returned when the project has no published release to
	// point at — /releases/latest answered without a tag redirect.
	ErrNoRelease = errors.New("selfupdate: no published release")
	// ErrChecksumMissing is returned when SHA256SUMS carries no line for the
	// requested asset. An updater must never run an unverified binary, so this
	// is fatal rather than a skipped check.
	ErrChecksumMissing = errors.New("selfupdate: asset missing from " + checksumsName)
)

// Latest returns the newest published release tag (e.g. "v0.4.1") for the
// project at baseURL (e.g. "https://github.com/chinese-room-solutions/grimoire").
//
// It GETs baseURL+"/releases/latest" without following the redirect and reads
// the tag out of the Location header's ".../releases/tag/<tag>" path, which
// costs no GitHub API quota. Draft releases never redirect here — publishing is
// the rollout gate.
func Latest(ctx context.Context, baseURL string) (string, error) {
	target := strings.TrimSuffix(baseURL, "/") + "/releases/latest"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, target, nil)
	if err != nil {
		return "", ctxerr.With(fmt.Errorf("selfupdate: building request: %w", err), map[string]any{"url": target})
	}
	// Never follow: the redirect target IS the answer, and following it would
	// download a release page for nothing.
	hc := &http.Client{
		Timeout:       defaultTimeout,
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
	}
	resp, err := hc.Do(req)
	if err != nil {
		return "", ctxerr.With(fmt.Errorf("selfupdate: fetching latest release: %w", err), map[string]any{"url": target})
	}
	defer func() { _ = resp.Body.Close() }()

	loc := resp.Header.Get("Location")
	if loc == "" {
		return "", ctxerr.With(
			fmt.Errorf("%w: %s answered %d without a redirect", ErrNoRelease, target, resp.StatusCode),
			map[string]any{"url": target, "status": resp.StatusCode},
		)
	}
	tag, err := tagFromLocation(loc)
	if err != nil {
		return "", ctxerr.With(err, map[string]any{"url": target, "location": loc})
	}
	return tag, nil
}

// tagFromLocation pulls "<tag>" out of a ".../releases/tag/<tag>" redirect
// target. Anything else means the project has no published release (GitHub
// bounces to the releases index) or the answer isn't GitHub's at all.
func tagFromLocation(loc string) (string, error) {
	u, err := url.Parse(loc)
	if err != nil {
		return "", fmt.Errorf("%w: unparseable redirect %q: %w", ErrNoRelease, loc, err)
	}
	dir, tag := path.Split(strings.TrimSuffix(u.Path, "/"))
	if tag == "" || !strings.HasSuffix(strings.TrimSuffix(dir, "/"), "/releases/tag") {
		return "", fmt.Errorf("%w: redirect %q is not a release tag", ErrNoRelease, loc)
	}
	return tag, nil
}

// IsNewer reports whether latest is a strictly newer release than current.
//
// It is false whenever the comparison can't be trusted: when either side fails
// to parse as semver, or when current is not a clean release version. A dev
// build ("dev", "v0.1.0-3-gabc-dirty", a bare commit sha) is deliberately never
// offered an update — its user builds from source and an installer would
// clobber that.
func IsNewer(current, latest string) bool {
	cur, err := semver.NewVersion(current)
	if err != nil || !isRelease(cur) {
		return false
	}
	next, err := semver.NewVersion(latest)
	if err != nil || !isRelease(next) {
		return false
	}
	return next.GreaterThan(cur)
}

// isRelease reports whether v is a clean release version: no prerelease and no
// build metadata. Mirrors grimoire's internal isReleaseVersion; the SDK can't
// import an app, so the rule lives in both places.
func isRelease(v *semver.Version) bool {
	return v.Prerelease() == "" && v.Metadata() == ""
}

// FetchSetup downloads the release asset baseURL+"/releases/download/<tag>/<asset>"
// into destDir, verifies it against the SHA256SUMS published under the same tag,
// marks it executable, and returns its path.
//
// A missing SHA256SUMS, an unfetchable one, or one with no line for asset is an
// error: an updater that runs an unverified binary is a remote code execution
// path, so this is deliberately stricter than install.sh's best-effort check.
// URLs are pinned to tag rather than the "latest" alias, which lags publishing
// by minutes and would otherwise fetch the previous release's bytes.
func FetchSetup(ctx context.Context, baseURL, tag, asset, destDir string) (string, error) {
	base := strings.TrimSuffix(baseURL, "/") + "/releases/download/" + url.PathEscape(tag)

	sums, err := fetchChecksums(ctx, base+"/"+checksumsName)
	if err != nil {
		return "", err
	}
	sum, ok := sums[asset]
	if !ok {
		return "", ctxerr.With(
			fmt.Errorf("%w: %s", ErrChecksumMissing, asset),
			map[string]any{"asset": asset, "tag": tag},
		)
	}

	dest := filepath.Join(destDir, asset)
	art := registry.Artifact{URL: base + "/" + url.PathEscape(asset), SHA256: sum}
	if err := registry.Download(ctx, nil, art, dest); err != nil {
		return "", fmt.Errorf("selfupdate: downloading %s: %w", asset, err)
	}
	// The asset is a setup binary the caller is about to exec; GitHub serves it
	// without a mode and Download creates the temp file 0600.
	if err := os.Chmod(dest, 0o755); err != nil {
		return "", ctxerr.With(fmt.Errorf("selfupdate: marking %s executable: %w", asset, err), map[string]any{"path": dest})
	}
	return dest, nil
}

// fetchChecksums downloads the SHA256SUMS manifest and parses it into
// asset→digest. The manifest is the trust root for the asset, so it has no
// checksum of its own — its integrity rests on TLS to the release host.
func fetchChecksums(ctx context.Context, sumsURL string) (map[string]string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, sumsURL, nil)
	if err != nil {
		return nil, ctxerr.With(fmt.Errorf("selfupdate: building request: %w", err), map[string]any{"url": sumsURL})
	}
	resp, err := (&http.Client{Timeout: defaultTimeout}).Do(req)
	if err != nil {
		return nil, ctxerr.With(fmt.Errorf("selfupdate: fetching %s: %w", checksumsName, err), map[string]any{"url": sumsURL})
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return nil, ctxerr.With(
			fmt.Errorf("selfupdate: fetching %s: unexpected status %d", checksumsName, resp.StatusCode),
			map[string]any{"url": sumsURL, "status": resp.StatusCode},
		)
	}

	sums := map[string]string{}
	sc := bufio.NewScanner(io.LimitReader(resp.Body, maxChecksumsSize))
	for sc.Scan() {
		// sha256sum format: "<hex>␠␠<name>", with '*' marking a binary-mode name.
		digest, name, ok := strings.Cut(strings.TrimSpace(sc.Text()), " ")
		if !ok {
			continue
		}
		name = strings.TrimPrefix(strings.TrimSpace(name), "*")
		if digest != "" && name != "" {
			sums[name] = digest
		}
	}
	if err := sc.Err(); err != nil {
		return nil, ctxerr.With(fmt.Errorf("selfupdate: reading %s: %w", checksumsName, err), map[string]any{"url": sumsURL})
	}
	return sums, nil
}

// Replaceable reports whether the file at path can be replaced right now, by
// probing the rename the installer will perform: path is renamed aside and back.
// On Windows that fails while another process holds the file open, which is
// exactly the condition a caller polls on before re-running the installer. On
// POSIX a rename over a running binary always succeeds, so this is normally
// true. A path that doesn't exist is replaceable — there is nothing in the way.
func Replaceable(path string) bool {
	if _, err := os.Stat(path); errors.Is(err, os.ErrNotExist) {
		return true
	}
	probe := path + ".replaceprobe"
	if err := os.Rename(path, probe); err != nil {
		return false
	}
	// Put it back. If this fails the file now lives under probe — report false so
	// the caller keeps waiting rather than acting on a half-moved install.
	return os.Rename(probe, path) == nil
}
