package main

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sort"

	"github.com/Masterminds/semver/v3"
	"github.com/chinese-room-solutions/mass-sdk/registry"
	"gopkg.in/yaml.v3"
)

// Finding is one validation failure, located as precisely as the check allows:
// a whole-file problem carries no package, a package-level one no version.
type Finding struct {
	File    string
	Package string
	Version string
	Problem string
}

func (f Finding) String() string {
	loc := f.File
	if f.Package != "" {
		loc += ": " + f.Package
		if f.Version != "" {
			loc += "@" + f.Version
		}
	}
	return loc + ": " + f.Problem
}

// validateIndex checks one index.yml against the schema the SDK's registry
// client expects. It returns the parsed index (nil only when the YAML itself is
// undecodable) alongside every finding, so a caller can keep checking artifacts
// of an index that already has schema problems.
func validateIndex(file string, data []byte) (*registry.Index, []Finding) {
	var idx registry.Index
	if err := yaml.Unmarshal(data, &idx); err != nil {
		return nil, []Finding{{File: file, Problem: fmt.Sprintf("parsing yaml: %v", err)}}
	}

	var findings []Finding
	if idx.SchemaVersion != registry.SchemaVersion {
		findings = append(findings, Finding{
			File:    file,
			Problem: fmt.Sprintf("schema_version is %d, want %d", idx.SchemaVersion, registry.SchemaVersion),
		})
	}

	seen := make(map[string]bool, len(idx.Packages))
	for i, pkg := range idx.Packages {
		add := func(problem string, args ...any) {
			findings = append(findings, Finding{
				File: file, Package: pkg.Name, Problem: fmt.Sprintf(problem, args...),
			})
		}
		switch {
		case pkg.Name == "":
			add("package #%d has an empty name", i+1)
		case seen[pkg.Name]:
			add("duplicate package name")
		default:
			seen[pkg.Name] = true
		}
		if pkg.Kind == "" {
			add("kind is empty")
		}
		if pkg.Kind == registry.KindWorker && pkg.RuntimeName == "" {
			add("kind worker requires runtime_name")
		}
		findings = append(findings, validateVersions(file, pkg)...)
	}
	return &idx, findings
}

// validateVersions checks a package's version list: semver, ascending order,
// parseable compat ranges, and artifacts carrying a url and a usable digest.
func validateVersions(file string, pkg registry.Package) []Finding {
	var findings []Finding
	var prev *semver.Version
	for _, v := range pkg.Versions {
		add := func(problem string, args ...any) {
			findings = append(findings, Finding{
				File: file, Package: pkg.Name, Version: v.Version, Problem: fmt.Sprintf(problem, args...),
			})
		}

		sv, err := semver.NewVersion(v.Version)
		if err != nil {
			add("version %q is not semver: %v", v.Version, err)
		} else {
			if prev != nil && !sv.GreaterThan(prev) {
				add("version %q does not sort after %q", v.Version, prev.Original())
			}
			prev = sv
		}

		for _, r := range []struct{ label, raw string }{
			{"mass", v.Mass}, {"runtime", v.Runtime}, {"grimoire", v.Grimoire},
		} {
			if r.raw == "" {
				continue
			}
			if _, err := semver.NewConstraint(r.raw); err != nil {
				add("%s range %q does not parse: %v", r.label, r.raw, err)
			}
		}

		if len(v.Artifacts) == 0 {
			add("has no artifacts")
		}
		for _, key := range sortedKeys(v.Artifacts) {
			artifact := v.Artifacts[key]
			if artifact.URL == "" {
				add("artifact %s has no url", key)
			}
			if !validDigest(artifact.SHA256) {
				add("artifact %s has sha256 %q, want 64 hex characters or %s",
					key, artifact.SHA256, registry.PlaceholderSHA256)
			}
		}
	}
	return findings
}

// validDigest reports whether s is a real sha256 or the unreleased placeholder.
func validDigest(s string) bool {
	if s == registry.PlaceholderSHA256 {
		return true
	}
	if len(s) != sha256.Size*2 {
		return false
	}
	_, err := hex.DecodeString(s)
	return err == nil
}

// sortedKeys orders artifact platform keys so findings come out deterministically.
func sortedKeys(artifacts map[string]registry.Artifact) []string {
	keys := make([]string, 0, len(artifacts))
	for key := range artifacts {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

// countVersions totals the versions across an index, for the success summary.
func countVersions(idx *registry.Index) int {
	n := 0
	for _, pkg := range idx.Packages {
		n += len(pkg.Versions)
	}
	return n
}
