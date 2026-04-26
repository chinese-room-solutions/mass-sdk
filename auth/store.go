// Package auth provides per-MASS-endpoint token storage for app GUIs.
//
// Tokens are persisted in a single JSON file at os.UserCacheDir()/mass/auth.json
// keyed by the MASS endpoint URL. This lets a single user log into multiple
// MASS instances and have each app GUI remember the correct token per endpoint.
package auth

import (
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

// ErrNotFound is returned when no token is stored for the given URL.
var ErrNotFound = errors.New("no token stored for endpoint")

// fileName is the JSON filename within the cache directory.
const fileName = "auth.json"

// dirName is the directory under os.UserCacheDir() used for MASS state.
const dirName = "mass"

// Store persists endpoint→token mappings on disk.
//
// Safe for concurrent use within a single process. Cross-process safety is
// best-effort via atomic temp+rename writes; concurrent writers from different
// processes may overwrite each other's last-write.
type Store struct {
	path   string
	mu     sync.RWMutex
	tokens map[string]string
}

// Load opens (or creates) the auth store at the default location:
// os.UserCacheDir()/mass/auth.json.
func Load() (*Store, error) {
	cacheDir, err := os.UserCacheDir()
	if err != nil {
		return nil, fmt.Errorf("locating user cache dir: %w", err)
	}
	return LoadFrom(filepath.Join(cacheDir, dirName, fileName))
}

// LoadFrom opens (or creates) the auth store at the given path. Useful for
// tests and for callers that want to override the default location.
//
// The backing file is created lazily on first Set; missing or empty files
// load as an empty store.
func LoadFrom(path string) (*Store, error) {
	s := &Store{path: path, tokens: map[string]string{}}

	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) || len(data) == 0 {
		return s, nil
	}
	if err != nil {
		return nil, fmt.Errorf("reading %s: %w", path, err)
	}
	if err := json.Unmarshal(data, &s.tokens); err != nil {
		return nil, fmt.Errorf("parsing %s: %w", path, err)
	}
	return s, nil
}

// Path returns the on-disk file path the store reads/writes.
func (s *Store) Path() string { return s.path }

// Get returns the stored token for the given endpoint URL, if any.
func (s *Store) Get(endpoint string) (string, bool) {
	key, ok := normalize(endpoint)
	if !ok {
		return "", false
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	tok, ok := s.tokens[key]
	return tok, ok
}

// Set stores token for the given endpoint URL and persists to disk atomically.
func (s *Store) Set(endpoint, token string) error {
	key, ok := normalize(endpoint)
	if !ok {
		return fmt.Errorf("invalid endpoint URL: %q", endpoint)
	}
	s.mu.Lock()
	s.tokens[key] = token
	snapshot := maps.Clone(s.tokens)
	s.mu.Unlock()
	return s.write(snapshot)
}

// Delete removes the token for the given endpoint and persists.
func (s *Store) Delete(endpoint string) error {
	key, ok := normalize(endpoint)
	if !ok {
		return fmt.Errorf("invalid endpoint URL: %q", endpoint)
	}
	s.mu.Lock()
	if _, present := s.tokens[key]; !present {
		s.mu.Unlock()
		return nil
	}
	delete(s.tokens, key)
	snapshot := maps.Clone(s.tokens)
	s.mu.Unlock()
	return s.write(snapshot)
}

// write serializes the snapshot to disk atomically (temp + rename), 0600.
func (s *Store) write(snapshot map[string]string) error {
	if err := os.MkdirAll(filepath.Dir(s.path), 0o700); err != nil {
		return fmt.Errorf("creating dir: %w", err)
	}
	data, err := json.MarshalIndent(snapshot, "", "  ")
	if err != nil {
		return fmt.Errorf("marshalling tokens: %w", err)
	}
	tmp, err := os.CreateTemp(filepath.Dir(s.path), ".auth.json.*.tmp")
	if err != nil {
		return fmt.Errorf("creating temp file: %w", err)
	}
	tmpName := tmp.Name()
	cleanup := func() { _ = os.Remove(tmpName) }

	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		cleanup()
		return fmt.Errorf("writing temp file: %w", err)
	}
	if err := tmp.Chmod(0o600); err != nil {
		_ = tmp.Close()
		cleanup()
		return fmt.Errorf("chmod temp file: %w", err)
	}
	if err := tmp.Close(); err != nil {
		cleanup()
		return fmt.Errorf("closing temp file: %w", err)
	}
	if err := os.Rename(tmpName, s.path); err != nil {
		cleanup()
		return fmt.Errorf("renaming into place: %w", err)
	}
	return nil
}

// Normalize returns a canonical key for the endpoint URL: scheme+host
// lowercased, trailing slash stripped, no path/query/fragment. Returns false
// if the input cannot be parsed as a URL with both scheme and host.
func normalize(endpoint string) (string, bool) {
	endpoint = strings.TrimSpace(endpoint)
	if endpoint == "" {
		return "", false
	}
	u, err := url.Parse(endpoint)
	if err != nil {
		return "", false
	}
	if u.Scheme == "" || u.Host == "" {
		return "", false
	}
	scheme := strings.ToLower(u.Scheme)
	host := strings.ToLower(u.Host)
	return scheme + "://" + host, true
}
