// Command registry-validate checks a MASS or Grimoire package index (index.yml)
// against the schema the SDK's registry client expects: schema version, unique
// package names, semver versions in ascending order, parseable compat ranges,
// and artifacts carrying a url and a real-or-placeholder sha256. With
// --verify-artifacts it also downloads every non-placeholder artifact and
// compares the streamed digest against the index. It is meant to run in the
// registry repos' CI; any finding exits non-zero.
package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"flag"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/chinese-room-solutions/mass-sdk/registry"
)

func main() {
	verifyArtifacts := flag.Bool("verify-artifacts", false,
		"download every non-placeholder artifact and check its sha256 against the index")
	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "usage: registry-validate [--verify-artifacts] index.yml [index.yml ...]\n\n")
		flag.PrintDefaults()
	}
	flag.Parse()
	if flag.NArg() == 0 {
		flag.Usage()
		os.Exit(2)
	}

	if !run(context.Background(), flag.Args(), *verifyArtifacts) {
		os.Exit(1)
	}
}

// run validates every path and reports whether all of them passed. Findings go
// to stderr one per line, the per-file summary to stdout.
func run(ctx context.Context, paths []string, verifyArtifacts bool) bool {
	ok := true
	for _, path := range paths {
		data, err := os.ReadFile(path)
		if err != nil {
			fmt.Fprintf(os.Stderr, "%s: reading: %v\n", path, err)
			ok = false
			continue
		}
		idx, findings := validateIndex(path, data)
		if idx != nil && verifyArtifacts {
			findings = append(findings, verifyDigests(ctx, path, idx)...)
		}
		for _, f := range findings {
			fmt.Fprintln(os.Stderr, f)
		}
		if len(findings) > 0 {
			ok = false
			continue
		}
		fmt.Printf("%s: ok (%d packages, %d versions)\n", path, len(idx.Packages), countVersions(idx))
	}
	return ok
}

// verifyDigests downloads every artifact with a real digest and compares it
// against the index. Placeholder ("TBD") rows are skipped: their assets are
// unreleased by design.
func verifyDigests(ctx context.Context, file string, idx *registry.Index) []Finding {
	hc := &http.Client{Transport: &http.Transport{
		DialContext:           (&net.Dialer{Timeout: 10 * time.Second}).DialContext,
		TLSHandshakeTimeout:   10 * time.Second,
		ResponseHeaderTimeout: 30 * time.Second,
	}}

	var findings []Finding
	for _, pkg := range idx.Packages {
		for _, v := range pkg.Versions {
			for _, key := range sortedKeys(v.Artifacts) {
				artifact := v.Artifacts[key]
				if artifact.IsPlaceholder() || artifact.URL == "" {
					continue
				}
				if err := verifyOne(ctx, hc, artifact); err != nil {
					findings = append(findings, Finding{
						File: file, Package: pkg.Name, Version: v.Version,
						Problem: fmt.Sprintf("artifact %s: %v", key, err),
					})
				}
			}
		}
	}
	return findings
}

// verifyOne streams the artifact and compares its sha256 with the index digest.
func verifyOne(ctx context.Context, hc *http.Client, artifact registry.Artifact) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, artifact.URL, nil)
	if err != nil {
		return fmt.Errorf("building request for %s: %w", artifact.URL, err)
	}
	resp, err := hc.Do(req)
	if err != nil {
		return fmt.Errorf("fetching %s: %w", artifact.URL, err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("fetching %s: unexpected status %d", artifact.URL, resp.StatusCode)
	}

	h := sha256.New()
	if _, err := io.Copy(h, resp.Body); err != nil {
		return fmt.Errorf("streaming %s: %w", artifact.URL, err)
	}
	sum := hex.EncodeToString(h.Sum(nil))
	if !strings.EqualFold(sum, artifact.SHA256) {
		return fmt.Errorf("sha256 mismatch: index has %s, %s hashes to %s", artifact.SHA256, artifact.URL, sum)
	}
	return nil
}
