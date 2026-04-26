// Package huggingface provides a client for the HuggingFace Hub API
// with model search, file listing, pagination, and optional caching.
package huggingface

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/KernelPryanic/ctxerr"
)

const apiBase = "https://huggingface.co/api/models"

// Model represents a HuggingFace model with its GGUF files.
type Model struct {
	RepoID      string     `json:"repo_id"`
	Description string     `json:"description"`
	Downloads   int64      `json:"downloads"`
	Likes       int64      `json:"likes"`
	PipelineTag string     `json:"pipeline_tag"`
	AvatarURL   string     `json:"avatar_url,omitempty"`
	Params      int64      `json:"params,omitempty"` // Total parameter count (from safetensors metadata)
	Files       []GGUFFile `json:"files"`
}

// GGUFFile represents a single GGUF file within a model repository.
type GGUFFile struct {
	Filename  string `json:"filename"`
	SizeBytes int64  `json:"size_bytes"`
}

// apiModelResponse is the JSON structure returned by the HuggingFace models API.
type apiModelResponse struct {
	ID          string `json:"id"`
	Description string `json:"description"`
	Downloads   int64  `json:"downloads"`
	Likes       int64  `json:"likes"`
	PipelineTag string `json:"pipeline_tag"`
	GGUF        *struct {
		Total int64 `json:"total"`
	} `json:"gguf,omitempty"`
	Safetensors *struct {
		Total int64 `json:"total"`
	} `json:"safetensors,omitempty"`
}

// paramCount returns the total parameter count from gguf or safetensors metadata.
func (m *apiModelResponse) paramCount() int64 {
	if m.GGUF != nil && m.GGUF.Total > 0 {
		return m.GGUF.Total
	}
	if m.Safetensors != nil && m.Safetensors.Total > 0 {
		return m.Safetensors.Total
	}
	return 0
}

// apiTreeEntry is a file entry returned by the HuggingFace tree API.
type apiTreeEntry struct {
	Type string `json:"type"`
	Path string `json:"path"`
	Size int64  `json:"size"`
}

// SearchOptions configures a HuggingFace model search.
type SearchOptions struct {
	// FileExts filters results to models that have at least one file matching any
	// of the given extensions (e.g. []string{".gguf"}). An empty slice means no
	// filtering — all models are returned regardless of file types.
	FileExts []string
	// PipelineTags filters results client-side to models whose pipeline_tag
	// matches one of the given values. Models with an empty pipeline_tag always
	// pass through. An empty slice means no pipeline tag filtering.
	PipelineTags []string
	// Limit is the maximum number of results to return per page (default 5).
	Limit int
	// Cursor is an opaque pagination cursor returned by a previous SearchResult.
	// Pass it to continue fetching from where the last search left off.
	// Empty string means start from the beginning.
	Cursor string
	// ExcludeIDs is a set of repo IDs already displayed in previous pages.
	// Models with these IDs are skipped to avoid duplicates across "Show More" calls.
	ExcludeIDs []string
}

// SearchResult holds the results of a paginated search.
type SearchResult struct {
	Models     []Model
	NextCursor string // Opaque cursor for the next page; empty means no more results.
	HasMore    bool   // True if there are likely more results available.
}

// CacheStoreInterface is the interface used for optional caching of HF API results.
type CacheStoreInterface interface {
	Get(ctx context.Context, key string) ([]byte, error)
	Set(ctx context.Context, key string, value []byte, ttl time.Duration) error
}

// Client provides HuggingFace API access with optional caching.
type Client struct {
	cache CacheStoreInterface
}

// NewClient creates a HuggingFace client. If cache is nil, all calls
// go directly to the HF API with no caching.
func NewClient(cache CacheStoreInterface) *Client {
	return &Client{cache: cache}
}

// Cache TTL durations.
const (
	ttlTree   = 48 * time.Hour
	ttlAvatar = 48 * time.Hour
	ttlParams = 48 * time.Hour
)

// Search queries the HuggingFace API for models matching the given query.
// Results are sorted by downloads descending. This is a convenience wrapper
// that uses an uncached client.
func Search(ctx context.Context, query string, opts SearchOptions) (*SearchResult, error) {
	return NewClient(nil).Search(ctx, query, opts)
}

// Search queries the HuggingFace API for models matching the given query.
// Results are sorted by downloads descending.
func (c *Client) Search(ctx context.Context, query string, opts SearchOptions) (*SearchResult, error) {
	limit := opts.Limit
	if limit <= 0 {
		limit = 5
	}

	apiBatchSize := limit

	searchQuery := query
	if len(opts.FileExts) > 0 {
		for _, ext := range opts.FileExts {
			kw := strings.TrimPrefix(strings.ToLower(ext), ".")
			if kw != "" && !strings.Contains(strings.ToLower(query), kw) {
				searchQuery = query + " " + kw
				break
			}
		}
	}

	excludeIDs := make(map[string]bool, len(opts.ExcludeIDs))
	for _, id := range opts.ExcludeIDs {
		excludeIDs[id] = true
	}

	const maxFruitlessRounds = 5
	fruitlessRounds := 0

	var collected []Model
	nextURL := opts.Cursor
	hasMore := true

	for hasMore && len(collected) < limit {
		var batch []apiModelResponse
		var linkNext string
		var err error

		if nextURL != "" {
			batch, linkNext, err = fetchAPIURL(ctx, nextURL)
		} else {
			batch, linkNext, err = fetchAPIBatch(ctx, searchQuery, "", apiBatchSize)
		}
		if err != nil {
			if len(collected) > 0 {
				hasMore = false
				break
			}
			return nil, err
		}

		if linkNext == "" {
			hasMore = false
		}
		if len(batch) == 0 {
			hasMore = false
			break
		}

		nextURL = linkNext

		if len(opts.PipelineTags) > 0 {
			batch = filterByPipelineTag(batch, opts.PipelineTags)
		}

		var candidates []apiModelResponse
		for _, am := range batch {
			if excludeIDs[am.ID] {
				continue
			}
			if len(opts.FileExts) > 0 && !repoLikelyHasExt(am.ID, opts.FileExts) {
				continue
			}
			candidates = append(candidates, am)
		}
		if len(candidates) == 0 {
			fruitlessRounds++
			if fruitlessRounds >= maxFruitlessRounds {
				hasMore = false
				break
			}
			continue
		}

		models := c.resolveFiles(ctx, candidates, opts.FileExts)

		collectedBefore := len(collected)
		for _, m := range models {
			if len(opts.FileExts) > 0 && len(m.Files) == 0 {
				continue
			}
			excludeIDs[m.RepoID] = true
			collected = append(collected, m)
			if len(collected) == limit {
				break
			}
		}

		if len(collected) == collectedBefore {
			fruitlessRounds++
			if fruitlessRounds >= maxFruitlessRounds {
				hasMore = false
				break
			}
		} else {
			fruitlessRounds = 0
		}
	}

	c.resolveAvatars(ctx, collected)

	return &SearchResult{
		Models:     collected,
		NextCursor: nextURL,
		HasMore:    hasMore && len(collected) == limit,
	}, nil
}

// FindMmproj queries the HuggingFace tree API for a repo and returns the
// filename of an mmproj (vision projector) GGUF if one exists, or empty string.
func FindMmproj(ctx context.Context, repoID string) (string, error) {
	files, err := fetchMatchingFiles(ctx, repoID, nil)
	if err != nil {
		return "", err
	}
	for _, f := range files {
		name := strings.ToLower(f.Filename)
		if strings.Contains(name, "mmproj") && strings.HasSuffix(name, ".gguf") {
			return f.Filename, nil
		}
	}
	return "", nil
}

// SanitizeRepoID converts a repo ID to a directory path.
// The result preserves the "/" separator so downloads use a
// two-level directory structure: {publisher}/{repo}.
func SanitizeRepoID(repoID string) string {
	return repoID
}

func filterByPipelineTag(models []apiModelResponse, tags []string) []apiModelResponse {
	tagSet := make(map[string]bool, len(tags))
	for _, t := range tags {
		tagSet[t] = true
	}
	var out []apiModelResponse
	for _, m := range models {
		if m.PipelineTag == "" || tagSet[m.PipelineTag] {
			out = append(out, m)
		}
	}
	return out
}

func fetchAPIBatch(ctx context.Context, searchQuery, pipelineTag string, limit int) ([]apiModelResponse, string, error) {
	u, _ := url.Parse(apiBase)
	q := u.Query()
	q.Set("search", searchQuery)
	q.Set("sort", "downloads")
	q.Set("direction", "-1")
	q.Set("limit", fmt.Sprintf("%d", limit))
	if pipelineTag != "" {
		q.Set("pipeline_tag", pipelineTag)
	}
	u.RawQuery = q.Encode()

	return fetchAPIURL(ctx, u.String())
}

func fetchAPIURL(ctx context.Context, apiURL string) ([]apiModelResponse, string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, apiURL, nil)
	if err != nil {
		return nil, "", fmt.Errorf("creating search request: %w", err)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, "", ctxerr.With(fmt.Errorf("searching HuggingFace: %w", err), map[string]any{"url": apiURL})
	}
	defer resp.Body.Close() //nolint:errcheck // read-only response

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		return nil, "", ctxerr.With(fmt.Errorf("HuggingFace API returned %d: %s", resp.StatusCode, strings.TrimSpace(string(body))), map[string]any{"url": apiURL, "status": resp.StatusCode})
	}

	var models []apiModelResponse
	if err := json.NewDecoder(resp.Body).Decode(&models); err != nil {
		return nil, "", fmt.Errorf("decoding search results: %w", err)
	}

	nextURL := parseLinkNext(resp.Header.Get("Link"))
	return models, nextURL, nil
}

func parseLinkNext(header string) string {
	if header == "" {
		return ""
	}
	for _, part := range strings.Split(header, ",") {
		part = strings.TrimSpace(part)
		if !strings.Contains(part, `rel="next"`) {
			continue
		}
		start := strings.Index(part, "<")
		end := strings.Index(part, ">")
		if start >= 0 && end > start {
			return part[start+1 : end]
		}
	}
	return ""
}

func (c *Client) resolveFiles(ctx context.Context, apiModels []apiModelResponse, exts []string) []Model {
	type indexedResult struct {
		idx    int
		files  []GGUFFile
		params int64
	}
	models := make([]Model, len(apiModels))
	ch := make(chan indexedResult, len(apiModels))
	sem := make(chan struct{}, 5)
	var wg sync.WaitGroup
	for i, am := range apiModels {
		models[i] = Model{
			RepoID:      am.ID,
			Description: am.Description,
			Downloads:   am.Downloads,
			Likes:       am.Likes,
			PipelineTag: am.PipelineTag,
		}
		wg.Add(1)
		go func(idx int, repoID string) {
			defer wg.Done()
			sem <- struct{}{}
			files, _ := c.fetchMatchingFilesCached(ctx, repoID, exts)
			params := c.fetchParamCountCached(ctx, repoID)
			<-sem
			ch <- indexedResult{idx: idx, files: files, params: params}
		}(i, am.ID)
	}
	wg.Wait()
	close(ch)
	for r := range ch {
		models[r.idx].Files = r.files
		models[r.idx].Params = r.params
	}
	return models
}

func (c *Client) fetchMatchingFilesCached(ctx context.Context, repoID string, exts []string) ([]GGUFFile, error) {
	if c.cache != nil {
		key := "hf:tree:" + repoID
		if data, err := c.cache.Get(ctx, key); err == nil && data != nil {
			var files []GGUFFile
			if json.Unmarshal(data, &files) == nil {
				return filterByExt(files, exts), nil
			}
		}
	}

	allFiles, err := fetchMatchingFiles(ctx, repoID, nil)
	if err != nil {
		return nil, err
	}

	if c.cache != nil {
		key := "hf:tree:" + repoID
		if data, marshalErr := json.Marshal(allFiles); marshalErr == nil {
			_ = c.cache.Set(ctx, key, data, ttlTree)
		}
	}

	return filterByExt(allFiles, exts), nil
}

func (c *Client) fetchParamCountCached(ctx context.Context, repoID string) int64 {
	if c.cache != nil {
		key := "hf:params:" + repoID
		if data, err := c.cache.Get(ctx, key); err == nil && data != nil {
			var n int64
			if json.Unmarshal(data, &n) == nil {
				return n
			}
		}
	}

	n := fetchParamCount(ctx, repoID)

	if c.cache != nil {
		key := "hf:params:" + repoID
		if data, err := json.Marshal(n); err == nil {
			_ = c.cache.Set(ctx, key, data, ttlParams)
		}
	}

	return n
}

func fetchParamCount(ctx context.Context, repoID string) int64 {
	u := fmt.Sprintf("%s/%s?expand[]=gguf&expand[]=safetensors", apiBase, repoID)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return 0
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return 0
	}
	defer resp.Body.Close() //nolint:errcheck // read-only response
	if resp.StatusCode != http.StatusOK {
		return 0
	}
	var data apiModelResponse
	if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
		return 0
	}
	return data.paramCount()
}

func fetchMatchingFiles(ctx context.Context, repoID string, exts []string) ([]GGUFFile, error) {
	treeURL := fmt.Sprintf("%s/%s/tree/main", apiBase, repoID)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, treeURL, nil)
	if err != nil {
		return nil, err
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close() //nolint:errcheck // read-only response

	if resp.StatusCode != http.StatusOK {
		return nil, ctxerr.With(fmt.Errorf("tree API returned %d", resp.StatusCode), map[string]any{"repo_id": repoID, "status": resp.StatusCode})
	}

	var entries []apiTreeEntry
	if err := json.NewDecoder(resp.Body).Decode(&entries); err != nil {
		return nil, err
	}

	var files []GGUFFile
	for _, e := range entries {
		if e.Type != "file" {
			continue
		}
		if len(exts) == 0 {
			files = append(files, GGUFFile{Filename: e.Path, SizeBytes: e.Size})
			continue
		}
		lname := strings.ToLower(e.Path)
		for _, ext := range exts {
			if strings.HasSuffix(lname, strings.ToLower(ext)) {
				files = append(files, GGUFFile{Filename: e.Path, SizeBytes: e.Size})
				break
			}
		}
	}
	return files, nil
}

func filterByExt(files []GGUFFile, exts []string) []GGUFFile {
	if len(exts) == 0 {
		return files
	}
	var filtered []GGUFFile
	for _, f := range files {
		lname := strings.ToLower(f.Filename)
		for _, ext := range exts {
			if strings.HasSuffix(lname, strings.ToLower(ext)) {
				filtered = append(filtered, f)
				break
			}
		}
	}
	return filtered
}

func (c *Client) resolveAvatars(ctx context.Context, models []Model) {
	authorIndices := make(map[string][]int)
	for i, m := range models {
		author := strings.SplitN(m.RepoID, "/", 2)[0]
		if author != "" {
			authorIndices[author] = append(authorIndices[author], i)
		}
	}
	if len(authorIndices) == 0 {
		return
	}

	type result struct {
		author string
		url    string
	}
	ch := make(chan result, len(authorIndices))
	sem := make(chan struct{}, 5)
	var wg sync.WaitGroup
	for author := range authorIndices {
		wg.Add(1)
		go func(a string) {
			defer wg.Done()
			sem <- struct{}{}
			u := c.fetchAuthorAvatarCached(ctx, a)
			<-sem
			ch <- result{author: a, url: u}
		}(author)
	}
	wg.Wait()
	close(ch)

	for r := range ch {
		if r.url == "" {
			continue
		}
		for _, idx := range authorIndices[r.author] {
			models[idx].AvatarURL = r.url
		}
	}
}

func (c *Client) fetchAuthorAvatarCached(ctx context.Context, author string) string {
	if c.cache != nil {
		key := "hf:avatar:" + author
		if data, err := c.cache.Get(ctx, key); err == nil && data != nil {
			return string(data)
		}
	}

	avatarURL := fetchAuthorAvatar(ctx, author)

	if c.cache != nil {
		key := "hf:avatar:" + author
		_ = c.cache.Set(ctx, key, []byte(avatarURL), ttlAvatar)
	}

	return avatarURL
}

func fetchAuthorAvatar(ctx context.Context, author string) string {
	for _, kind := range []string{"users", "organizations"} {
		if u := tryFetchAvatar(ctx, kind, author); u != "" {
			return u
		}
	}
	return ""
}

func tryFetchAvatar(ctx context.Context, kind, author string) string {
	u := fmt.Sprintf("https://huggingface.co/api/%s/%s/overview", kind, url.PathEscape(author))
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return ""
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return ""
	}
	defer resp.Body.Close() //nolint:errcheck // read-only response
	if resp.StatusCode != http.StatusOK {
		return ""
	}
	var data struct {
		AvatarURL string `json:"avatarUrl"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
		return ""
	}
	if data.AvatarURL != "" {
		if strings.HasPrefix(data.AvatarURL, "/") {
			return "https://huggingface.co" + data.AvatarURL
		}
		return data.AvatarURL
	}
	return ""
}

func repoLikelyHasExt(repoID string, exts []string) bool {
	lower := strings.ToLower(repoID)
	for _, ext := range exts {
		kw := strings.TrimPrefix(strings.ToLower(ext), ".")
		if strings.Contains(lower, kw) {
			return true
		}
	}
	return false
}
