package registry

import (
	"fmt"
	"sort"
	"strings"

	"github.com/KernelPryanic/ctxerr"
	"github.com/Masterminds/semver/v3"
)

// Platform identifies a build target. Backend is empty for runtimes and set for
// workers (the GPU/compute backend, e.g. "vulkan").
type Platform struct {
	OS      string
	Arch    string
	Backend string
}

// RuntimePlatform builds a runtime platform key (os, arch).
func RuntimePlatform(os, arch string) Platform {
	return Platform{OS: os, Arch: arch}
}

// WorkerPlatform builds a worker platform key (os, arch, backend).
func WorkerPlatform(os, arch, backend string) Platform {
	return Platform{OS: os, Arch: arch, Backend: backend}
}

// Key returns the artifact map key for this platform: "os/arch" when Backend is
// empty, "os/arch/backend" otherwise.
func (p Platform) Key() string {
	if p.Backend == "" {
		return p.OS + "/" + p.Arch
	}
	return p.OS + "/" + p.Arch + "/" + p.Backend
}

// AnyArtifactKey is the artifact map key of a package that ships one asset for
// every platform.
const AnyArtifactKey = "any"

// Resolved is a successful resolution: the chosen version and the artifact for
// the requested platform.
type Resolved struct {
	Package  *Package
	Version  *Version
	Artifact Artifact
}

// FindPackage returns the package with the given name, or nil.
func (idx *Index) FindPackage(name string) *Package {
	for i := range idx.Packages {
		if idx.Packages[i].Name == name {
			return &idx.Packages[i]
		}
	}
	return nil
}

// SearchOptions filters a package listing.
type SearchOptions struct {
	// Kind, when non-empty, restricts results to that kind.
	Kind Kind
	// Query, when non-empty, is a case-insensitive substring matched against
	// name, display_name, and description.
	Query string
	// RuntimeName, when non-empty, restricts results to packages joined to that
	// runtime.
	RuntimeName string
}

// Search returns packages matching opts, in index order.
func (idx *Index) Search(opts SearchOptions) []Package {
	q := strings.ToLower(opts.Query)
	var out []Package
	for _, pkg := range idx.Packages {
		if opts.Kind != "" && pkg.Kind != opts.Kind {
			continue
		}
		if opts.RuntimeName != "" && pkg.RuntimeName != opts.RuntimeName {
			continue
		}
		if q != "" && !pkgMatchesQuery(pkg, q) {
			continue
		}
		out = append(out, pkg)
	}
	return out
}

func pkgMatchesQuery(pkg Package, lowerQuery string) bool {
	return strings.Contains(strings.ToLower(pkg.Name), lowerQuery) ||
		strings.Contains(strings.ToLower(pkg.DisplayName), lowerQuery) ||
		strings.Contains(strings.ToLower(pkg.Description), lowerQuery)
}

// ResolveRuntime resolves a runtime package for the platform, picking the newest
// version whose mass range covers massVersion and that has an artifact for the
// platform key. massVersion is the MASS version installed.
func (idx *Index) ResolveRuntime(name string, platform Platform, massVersion string) (*Resolved, error) {
	pkg := idx.FindPackage(name)
	if pkg == nil {
		return nil, ctxerr.With(fmt.Errorf("%w: no package %q", ErrNotResolved, name), map[string]any{"name": name})
	}
	if pkg.Kind != KindRuntime {
		return nil, ctxerr.With(
			fmt.Errorf("%w: package %q is a %s, not a runtime", ErrNotResolved, name, pkg.Kind),
			map[string]any{"name": name, "kind": pkg.Kind},
		)
	}
	return resolve(pkg, platform.Key(), versionRange{
		label:   "mass",
		version: massVersion,
		rangeOf: massRange,
	})
}

// ResolveWorker resolves a worker package for the platform, picking the newest
// version whose runtime range covers runtimeVersion, whose mass range covers
// massVersion, and that has an artifact for the platform key. runtimeVersion is
// the installed runtime's version and massVersion is the MASS server's version
// (a worker talks to the server directly). An empty mass range on a worker
// version means no MASS constraint.
func (idx *Index) ResolveWorker(name string, platform Platform, runtimeVersion, massVersion string) (*Resolved, error) {
	pkg := idx.FindPackage(name)
	if pkg == nil {
		return nil, ctxerr.With(fmt.Errorf("%w: no package %q", ErrNotResolved, name), map[string]any{"name": name})
	}
	if pkg.Kind != KindWorker {
		return nil, ctxerr.With(
			fmt.Errorf("%w: package %q is a %s, not a worker", ErrNotResolved, name, pkg.Kind),
			map[string]any{"name": name, "kind": pkg.Kind},
		)
	}
	return resolve(pkg, platform.Key(),
		versionRange{label: "runtime", version: runtimeVersion, rangeOf: func(v Version) string { return v.Runtime }},
		versionRange{label: "mass", version: massVersion, rangeOf: massRange, allowEmpty: true},
	)
}

// ResolveForGrimoire resolves a package consumed by a Grimoire host: among the
// versions carrying an artifact under AnyArtifactKey, it picks the newest whose
// grimoire range covers grimoireVersion. An empty grimoire range means no
// Grimoire constraint. It is kind-agnostic — the caller has already picked the
// package it wants. requestedVersion, when non-empty, pins the choice: the
// newest compatible version is resolved and then asserted to equal it, so a pin
// naming an unknown or incompatible version fails instead of silently
// installing another.
func (idx *Index) ResolveForGrimoire(name, grimoireVersion, requestedVersion string) (*Resolved, error) {
	pkg := idx.FindPackage(name)
	if pkg == nil {
		return nil, ctxerr.With(fmt.Errorf("%w: no package %q", ErrNotResolved, name), map[string]any{"name": name})
	}
	resolved, err := resolve(pkg, AnyArtifactKey, versionRange{
		label:      "grimoire",
		version:    grimoireVersion,
		rangeOf:    func(v Version) string { return v.Grimoire },
		allowEmpty: true,
	})
	if err != nil {
		return nil, err
	}
	if requestedVersion != "" && resolved.Version.Version != requestedVersion {
		return nil, ctxerr.With(
			fmt.Errorf("%w: %s has no version %q for grimoire %s", ErrNotResolved, name, requestedVersion, grimoireVersion),
			map[string]any{"package": name, "version": requestedVersion, "grimoire": grimoireVersion},
		)
	}
	return resolved, nil
}

// versionRange is one semver constraint a version must satisfy to be a
// candidate: the range string (from rangeOf) must cover version. When allowEmpty
// is set an empty range is treated as unconstrained (always satisfied);
// otherwise an empty range is a malformed-index error, as before.
type versionRange struct {
	label      string
	version    string
	rangeOf    func(Version) string
	allowEmpty bool
}

// massRange reads a version's mass constraint, a range over MASS server
// versions (empty means unconstrained, valid only on worker versions).
func massRange(v Version) string { return v.Mass }

// resolve implements the README rule: among versions of pkg that have an
// artifact under key and satisfy every range, pick the newest by semver.
func resolve(pkg *Package, key string, ranges ...versionRange) (*Resolved, error) {
	parsed := make([]*semver.Version, len(ranges))
	for i, r := range ranges {
		installed, err := semver.NewVersion(r.version)
		if err != nil {
			return nil, ctxerr.With(
				fmt.Errorf("parsing installed version %q: %w", r.version, err),
				map[string]any{"version": r.version},
			)
		}
		parsed[i] = installed
	}

	candidates := make([]*Version, 0, len(pkg.Versions))
	for i := range pkg.Versions {
		v := &pkg.Versions[i]
		if _, ok := v.Artifacts[key]; !ok {
			continue
		}
		ok, err := versionSatisfies(pkg, v, ranges, parsed)
		if err != nil {
			return nil, err
		}
		if ok {
			candidates = append(candidates, v)
		}
	}
	if len(candidates) == 0 {
		return nil, ctxerr.With(
			fmt.Errorf("%w: %s for %s (installed %s)", ErrNotResolved, pkg.Name, key, installedVersions(ranges)),
			map[string]any{"package": pkg.Name, "platform": key, "installed": installedVersions(ranges)},
		)
	}

	newest, err := newestVersion(candidates)
	if err != nil {
		return nil, err
	}
	return &Resolved{Package: pkg, Version: newest, Artifact: newest.Artifacts[key]}, nil
}

// versionSatisfies reports whether v satisfies every range. An empty range is
// unconstrained only when that range allows it; any other unparseable range is
// an error (mirroring how a malformed index range is fatal, not a silent skip).
func versionSatisfies(pkg *Package, v *Version, ranges []versionRange, parsed []*semver.Version) (bool, error) {
	for i, r := range ranges {
		raw := r.rangeOf(*v)
		if raw == "" && r.allowEmpty {
			continue
		}
		constraint, err := semver.NewConstraint(raw)
		if err != nil {
			return false, ctxerr.With(
				fmt.Errorf("parsing %s %q of %s@%s: %w", r.label, raw, pkg.Name, v.Version, err),
				map[string]any{"range": raw, "label": r.label, "package": pkg.Name, "version": v.Version},
			)
		}
		if !constraint.Check(parsed[i]) {
			return false, nil
		}
	}
	return true, nil
}

// installedVersions joins the installed versions across ranges for an error
// message, e.g. "0.1.0, 0.2.0".
func installedVersions(ranges []versionRange) string {
	parts := make([]string, len(ranges))
	for i, r := range ranges {
		parts[i] = r.version
	}
	return strings.Join(parts, ", ")
}

// newestVersion returns the candidate with the highest semver.
func newestVersion(candidates []*Version) (*Version, error) {
	type ranked struct {
		v  *Version
		sv *semver.Version
	}
	ranks := make([]ranked, 0, len(candidates))
	for _, c := range candidates {
		sv, err := semver.NewVersion(c.Version)
		if err != nil {
			return nil, ctxerr.With(
				fmt.Errorf("parsing version %q: %w", c.Version, err),
				map[string]any{"version": c.Version},
			)
		}
		ranks = append(ranks, ranked{v: c, sv: sv})
	}
	sort.Slice(ranks, func(i, j int) bool { return ranks[i].sv.GreaterThan(ranks[j].sv) })
	return ranks[0].v, nil
}

// CompatibleWorker is a worker package version whose runtime range covers a
// given runtime version — one entry per matching (package, version).
type CompatibleWorker struct {
	Package *Package
	Version *Version
}

// CompatibleWorkers lists worker package versions joined to runtimeName whose
// runtime range covers runtimeVersion and whose mass range covers massVersion
// (an empty mass range is unconstrained). It does not filter by
// platform (the caller resolves an artifact later, when the target platform is
// known). Results are ordered by package then newest version first. It is used
// by the join command and by fleet compatibility flagging.
func (idx *Index) CompatibleWorkers(runtimeName, runtimeVersion, massVersion string) ([]CompatibleWorker, error) {
	runtime, err := semver.NewVersion(runtimeVersion)
	if err != nil {
		return nil, ctxerr.With(
			fmt.Errorf("parsing runtime version %q: %w", runtimeVersion, err),
			map[string]any{"version": runtimeVersion},
		)
	}
	mass, err := semver.NewVersion(massVersion)
	if err != nil {
		return nil, ctxerr.With(
			fmt.Errorf("parsing mass version %q: %w", massVersion, err),
			map[string]any{"version": massVersion},
		)
	}

	var out []CompatibleWorker
	for pi := range idx.Packages {
		pkg := &idx.Packages[pi]
		if pkg.Kind != KindWorker || pkg.RuntimeName != runtimeName {
			continue
		}
		matching := make([]*Version, 0, len(pkg.Versions))
		for vi := range pkg.Versions {
			v := &pkg.Versions[vi]
			runtimeRange, err := semver.NewConstraint(v.Runtime)
			if err != nil {
				return nil, ctxerr.With(
					fmt.Errorf("parsing runtime range %q of %s@%s: %w", v.Runtime, pkg.Name, v.Version, err),
					map[string]any{"range": v.Runtime, "package": pkg.Name, "version": v.Version},
				)
			}
			if !runtimeRange.Check(runtime) {
				continue
			}
			ok, err := massConstraintCovers(pkg, v, mass)
			if err != nil {
				return nil, err
			}
			if ok {
				matching = append(matching, v)
			}
		}
		if len(matching) == 0 {
			continue
		}
		sortVersionsDesc(matching)
		for _, v := range matching {
			out = append(out, CompatibleWorker{Package: pkg, Version: v})
		}
	}
	return out, nil
}

// massConstraintCovers reports whether v's mass range covers mass. An empty mass
// range is unconstrained; a non-empty one that fails to parse is an error.
func massConstraintCovers(pkg *Package, v *Version, mass *semver.Version) (bool, error) {
	if v.Mass == "" {
		return true, nil
	}
	constraint, err := semver.NewConstraint(v.Mass)
	if err != nil {
		return false, ctxerr.With(
			fmt.Errorf("parsing mass range %q of %s@%s: %w", v.Mass, pkg.Name, v.Version, err),
			map[string]any{"range": v.Mass, "package": pkg.Name, "version": v.Version},
		)
	}
	return constraint.Check(mass), nil
}

// WorkerPackagesFor returns every worker package joined to runtimeName, sorted
// by Name for a deterministic order. Multiple worker packages per runtime are
// legal by design (e.g. distinct backends packaged separately).
func (idx *Index) WorkerPackagesFor(runtimeName string) []*Package {
	var out []*Package
	for i := range idx.Packages {
		pkg := &idx.Packages[i]
		if pkg.Kind == KindWorker && pkg.RuntimeName == runtimeName {
			out = append(out, pkg)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

// sortVersionsDesc sorts in place, newest semver first. Unparseable versions
// sort last (they should not reach here — the index is validated on parse).
func sortVersionsDesc(vs []*Version) {
	sort.SliceStable(vs, func(i, j int) bool {
		a, errA := semver.NewVersion(vs[i].Version)
		b, errB := semver.NewVersion(vs[j].Version)
		switch {
		case errA != nil:
			return false
		case errB != nil:
			return true
		default:
			return a.GreaterThan(b)
		}
	})
}
