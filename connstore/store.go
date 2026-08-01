// Package connstore provides per-MASS-endpoint connection storage for app GUIs.
//
// Each endpoint's connection settings (auth token and an optional custom CA
// certificate) are persisted in a single JSON file at
// os.UserConfigDir()/mass/auth.json keyed by the MASS endpoint URL. This lets a
// single user connect to multiple MASS instances and have each app GUI remember
// the correct connection per endpoint. The tokens are secrets, so they live in
// the user's CONFIG dir, not the cache dir (caches are fair game for cleanup
// tools and often laxer backup/permission policies); Load migrates a store left
// at the old cache-dir location.
package connstore

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

	"github.com/chinese-room-solutions/mass-sdk/fsutil"
)

// fileName is the JSON filename within the config directory.
const fileName = "auth.json"

// dirName is the directory under os.UserConfigDir() used for MASS state.
const dirName = "mass"

// Conn is the connection settings stored for one MASS endpoint: the auth token
// and an optional path to a custom CA certificate (PEM) to trust when the
// endpoint is served over HTTPS with a private or self-signed CA. An empty
// CACert means the system root pool is used.
type Conn struct {
	Token  string `json:"token,omitempty"`
	CACert string `json:"caCert,omitempty"`
}

// Store persists endpoint→connection mappings on disk.
//
// Safe for concurrent use within a single process. Cross-process safety is
// best-effort via atomic temp+rename writes; concurrent writers from different
// processes may overwrite each other's last-write.
type Store struct {
	path  string
	mu    sync.RWMutex
	conns map[string]Conn
}

// Load opens (or creates) the connection store at the default location:
// os.UserConfigDir()/mass/auth.json. When that file doesn't exist yet but a
// store from the old default (os.UserCacheDir()/mass/auth.json — where secrets
// don't belong) does, its contents are migrated: loaded and saved to the new
// location. The old file is left in place so an older binary sharing the
// machine keeps working; it simply stops receiving updates.
func Load() (*Store, error) {
	configDir, err := os.UserConfigDir()
	if err != nil {
		return nil, fmt.Errorf("locating user config dir: %w", err)
	}
	path := filepath.Join(configDir, dirName, fileName)

	oldPath := ""
	if cacheDir, cerr := os.UserCacheDir(); cerr == nil {
		oldPath = filepath.Join(cacheDir, dirName, fileName)
	}
	return loadMigrating(path, oldPath)
}

// loadMigrating opens the store at path, seeding it once from oldPath when path
// doesn't exist yet. Split from Load so the migration is testable without the
// real user dirs.
func loadMigrating(path, oldPath string) (*Store, error) {
	if _, err := os.Stat(path); errors.Is(err, os.ErrNotExist) && oldPath != "" {
		if legacy, lerr := LoadFrom(oldPath); lerr == nil && len(legacy.conns) > 0 {
			s := &Store{path: path, conns: legacy.conns}
			if werr := s.write(maps.Clone(s.conns)); werr != nil {
				return nil, fmt.Errorf("migrating connection store to %s: %w", path, werr)
			}
			return s, nil
		}
	}
	return LoadFrom(path)
}

// LoadFrom opens (or creates) the connection store at the given path. Useful for
// tests and for callers that want to override the default location.
//
// The backing file is created lazily on first write; missing or empty files
// load as an empty store. A legacy file whose values are bare token strings
// (the pre-Conn format) is migrated in memory to Conn values; the new shape is
// written on the next Set/SetConn.
func LoadFrom(path string) (*Store, error) {
	s := &Store{path: path, conns: map[string]Conn{}}

	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) || len(data) == 0 {
		return s, nil
	}
	if err != nil {
		return nil, fmt.Errorf("reading %s: %w", path, err)
	}
	if err := json.Unmarshal(data, &s.conns); err != nil {
		// Fall back to the legacy endpoint→token format and migrate it.
		var legacy map[string]string
		if lerr := json.Unmarshal(data, &legacy); lerr != nil {
			return nil, fmt.Errorf("parsing %s: %w", path, err)
		}
		for k, tok := range legacy {
			s.conns[k] = Conn{Token: tok}
		}
	}
	return s, nil
}

// Path returns the on-disk file path the store reads/writes.
func (s *Store) Path() string { return s.path }

// GetConn returns the stored connection settings for the given endpoint, if any.
func (s *Store) GetConn(endpoint string) (Conn, bool) {
	key, ok := normalize(endpoint)
	if !ok {
		return Conn{}, false
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	c, ok := s.conns[key]
	return c, ok
}

// SetConn stores the connection settings for the given endpoint URL and
// persists to disk atomically.
func (s *Store) SetConn(endpoint string, conn Conn) error {
	key, ok := normalize(endpoint)
	if !ok {
		return fmt.Errorf("invalid endpoint URL: %q", endpoint)
	}
	s.mu.Lock()
	s.conns[key] = conn
	snapshot := maps.Clone(s.conns)
	s.mu.Unlock()
	return s.write(snapshot)
}

// Get returns the stored token for the given endpoint URL, if any. It is a
// token-only view over GetConn.
func (s *Store) Get(endpoint string) (string, bool) {
	c, ok := s.GetConn(endpoint)
	if !ok {
		return "", false
	}
	return c.Token, true
}

// Set stores token for the given endpoint URL, preserving any other connection
// settings (e.g. a custom CA) already stored for it, and persists.
func (s *Store) Set(endpoint, token string) error {
	c, _ := s.GetConn(endpoint)
	c.Token = token
	return s.SetConn(endpoint, c)
}

// Delete removes the connection for the given endpoint and persists.
func (s *Store) Delete(endpoint string) error {
	key, ok := normalize(endpoint)
	if !ok {
		return fmt.Errorf("invalid endpoint URL: %q", endpoint)
	}
	s.mu.Lock()
	if _, present := s.conns[key]; !present {
		s.mu.Unlock()
		return nil
	}
	delete(s.conns, key)
	snapshot := maps.Clone(s.conns)
	s.mu.Unlock()
	return s.write(snapshot)
}

// write serializes the snapshot to disk atomically (temp + rename), 0600.
func (s *Store) write(snapshot map[string]Conn) error {
	if err := os.MkdirAll(filepath.Dir(s.path), 0o700); err != nil {
		return fmt.Errorf("creating dir: %w", err)
	}
	data, err := json.MarshalIndent(snapshot, "", "  ")
	if err != nil {
		return fmt.Errorf("marshalling tokens: %w", err)
	}
	return fsutil.WriteFileAtomic(s.path, data, 0o600)
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
