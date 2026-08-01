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
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/KernelPryanic/ctxerr"
)

const (
	apiHost = "huggingface.co"
	apiBase = "https://" + apiHost + "/api/models"

	// defaultTimeout caps each HF API request when the caller doesn't supply its
	// own http.Client — the hub occasionally hangs, and an unbounded
	// http.DefaultClient call would hang the search with it.
	defaultTimeout = 30 * time.Second
)

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
	hc    *http.Client
	token string
}

// Option configures a Client.
type Option func(*Client)

// WithHTTPClient replaces the default HTTP client (which carries
// defaultTimeout) — e.g. to route through a proxy or set custom TLS.
func WithHTTPClient(hc *http.Client) Option {
	return func(c *Client) { c.hc = hc }
}

// WithToken sends the given HF access token as a Bearer Authorization header
// on every request, for gated/private repos and higher rate limits.
func WithToken(token string) Option {
	return func(c *Client) { c.token = token }
}

// NewClient creates a HuggingFace client. If cache is nil, all calls
// go directly to the HF API with no caching.
func NewClient(cache CacheStoreInterface, opts ...Option) *Client {
	c := &Client{cache: cache, hc: &http.Client{Timeout: defaultTimeout}}
	for _, opt := range opts {
		opt(c)
	}
	return c
}

// Rate-limit cool-off bounds. A search resolves files and avatars five requests
// at a time, which is exactly when the hub answers 429, and a single short wait
// clears it. Longer cool-offs are not waited out: the caller is a person looking
// at a model picker, and blocking them is worse than showing the error.
const (
	retryAfterMax     = 10 * time.Second
	retryAfterDefault = 1 * time.Second
)

// get issues an authenticated GET through the client's HTTP client, retrying a
// 429 once after the hub's Retry-After cool-off. A cool-off longer than
// retryAfterMax is not waited for — the 429 is returned as-is.
func (c *Client) get(ctx context.Context, u string) (*http.Response, error) {
	resp, err := c.do(ctx, u)
	if err != nil || resp.StatusCode != http.StatusTooManyRequests {
		return resp, err
	}

	delay, worthWaiting := retryAfter(resp.Header.Get("Retry-After"))
	if !worthWaiting {
		return resp, nil
	}
	// Drain and close the 429 so its connection is reusable for the retry.
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 1024))
	if closeErr := resp.Body.Close(); closeErr != nil {
		return nil, closeErr
	}

	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-time.After(delay):
	}
	return c.do(ctx, u)
}

// do performs one authenticated GET.
func (c *Client) do(ctx context.Context, u string) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, err
	}
	if c.token != "" {
		req.Header.Set("Authorization", "Bearer "+c.token)
	}
	return c.hc.Do(req)
}

// retryAfter reads a Retry-After value — delta-seconds or an HTTP-date, per RFC
// 9110 — and reports whether waiting it out is worthwhile. A missing or
// unparseable value falls back to retryAfterDefault; a cool-off past
// retryAfterMax reports false.
func retryAfter(v string) (time.Duration, bool) {
	v = strings.TrimSpace(v)
	if v == "" {
		return retryAfterDefault, true
	}
	var d time.Duration
	if secs, err := strconv.Atoi(v); err == nil {
		d = time.Duration(secs) * time.Second
	} else if t, err := http.ParseTime(v); err == nil {
		d = time.Until(t)
	} else {
		return retryAfterDefault, true
	}
	switch {
	case d <= 0:
		return 0, true // already elapsed: retry at once.
	case d > retryAfterMax:
		return 0, false
	default:
		return d, true
	}
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
// Results are sorted by downloads descending. A non-empty opts.Cursor must be
// an https huggingface.co URL (the value a previous SearchResult supplied);
// anything else is rejected before any request is made.
func (c *Client) Search(ctx context.Context, query string, opts SearchOptions) (*SearchResult, error) {
	if opts.Cursor != "" {
		if err := validateCursor(opts.Cursor); err != nil {
			return nil, err
		}
	}

	limit := opts.Limit
	if limit <= 0 {
		limit = 5
	}

	apiBatchSize := limit

	// The HF API keeps free-text (`search=`) and library/tag matching (`filter=`)
	// separate. Extension filtering belongs in `filter` — folding the extension
	// keyword into `search` (as this once did) turns "pdf2html" into "pdf2html
	// gguf", which the full-text index matches against the repo id and so returns
	// nothing for a repo that has .gguf files but no "gguf" in its name. Map each
	// extension to its library tag (".gguf" → "gguf") and pass those as filters.
	searchQuery := query
	filters := make([]string, 0, len(opts.FileExts))
	for _, ext := range opts.FileExts {
		if tag := strings.TrimPrefix(strings.ToLower(ext), "."); tag != "" {
			filters = append(filters, tag)
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
			batch, linkNext, err = c.fetchAPIURL(ctx, nextURL)
		} else {
			batch, linkNext, err = c.fetchAPIBatch(ctx, searchQuery, "", filters, apiBatchSize)
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
// This is a convenience wrapper that uses an uncached client.
func FindMmproj(ctx context.Context, repoID string) (string, error) {
	return NewClient(nil).FindMmproj(ctx, repoID)
}

// FindMmproj queries the HuggingFace tree API for a repo and returns the
// filename of an mmproj (vision projector) GGUF if one exists, or empty string.
func (c *Client) FindMmproj(ctx context.Context, repoID string) (string, error) {
	files, err := c.fetchMatchingFiles(ctx, repoID, nil)
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

// ListFiles returns every file in the given HuggingFace repo's tree. This is a
// convenience wrapper that uses an uncached client.
func ListFiles(ctx context.Context, repoID string, exts []string) ([]GGUFFile, error) {
	return NewClient(nil).ListFiles(ctx, repoID, exts)
}

// ListFiles returns every file in the given HuggingFace repo's tree.
// When exts is non-empty, only files whose name ends with one of the given
// extensions are returned. Used by gateways planning multi-file model
// installs (e.g. companion mmproj alongside a chat GGUF).
func (c *Client) ListFiles(ctx context.Context, repoID string, exts []string) ([]GGUFFile, error) {
	return c.fetchMatchingFiles(ctx, repoID, exts)
}

// repoIDPattern is the {owner}/{name} shape of a HuggingFace repo id: exactly
// one "/", each segment starting with an alphanumeric and continuing with
// alphanumerics, "_", "." or "-". Requiring an alphanumeric head also rules
// out "." / ".." path segments.
var repoIDPattern = regexp.MustCompile(`^[A-Za-z0-9][\w.-]*/[A-Za-z0-9][\w.-]*$`)

// SanitizeRepoID validates that repoID has the {owner}/{name} HuggingFace repo
// shape and returns it for use as a two-level relative directory path
// ({publisher}/{repo}). Anything else — absolute paths, traversal segments,
// extra separators — is rejected, so a repo id from an untrusted source can't
// escape the download root.
func SanitizeRepoID(repoID string) (string, error) {
	if !repoIDPattern.MatchString(repoID) {
		return "", fmt.Errorf("invalid HuggingFace repo id %q (want owner/name)", repoID)
	}
	return repoID, nil
}

// validateCursor ensures the opaque pagination cursor — a URL taken from a
// previous response's Link header — still points at the HuggingFace API over
// https, so a stored/tampered cursor can't redirect the client (and its bearer
// token) to an arbitrary host.
func validateCursor(cursor string) error {
	u, err := url.Parse(cursor)
	if err != nil {
		return fmt.Errorf("invalid search cursor: %w", err)
	}
	if u.Scheme != "https" || u.Hostname() != apiHost {
		return fmt.Errorf("invalid search cursor %q: not an https %s URL", cursor, apiHost)
	}
	return nil
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

func (c *Client) fetchAPIBatch(ctx context.Context, searchQuery, pipelineTag string, filters []string, limit int) ([]apiModelResponse, string, error) {
	u, _ := url.Parse(apiBase)
	q := u.Query()
	q.Set("search", searchQuery)
	q.Set("sort", "downloads")
	q.Set("direction", "-1")
	q.Set("limit", fmt.Sprintf("%d", limit))
	if pipelineTag != "" {
		q.Set("pipeline_tag", pipelineTag)
	}
	// Library/tag filters (e.g. "gguf"). Repeated `filter=` params AND together
	// on the HF side — a model must carry every tag to match.
	for _, f := range filters {
		q.Add("filter", f)
	}
	u.RawQuery = q.Encode()

	return c.fetchAPIURL(ctx, u.String())
}

func (c *Client) fetchAPIURL(ctx context.Context, apiURL string) ([]apiModelResponse, string, error) {
	resp, err := c.get(ctx, apiURL)
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

	allFiles, err := c.fetchMatchingFiles(ctx, repoID, nil)
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

	n := c.fetchParamCount(ctx, repoID)

	if c.cache != nil {
		key := "hf:params:" + repoID
		if data, err := json.Marshal(n); err == nil {
			_ = c.cache.Set(ctx, key, data, ttlParams)
		}
	}

	return n
}

func (c *Client) fetchParamCount(ctx context.Context, repoID string) int64 {
	u := fmt.Sprintf("%s/%s?expand[]=gguf&expand[]=safetensors", apiBase, repoID)
	resp, err := c.get(ctx, u)
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

func (c *Client) fetchMatchingFiles(ctx context.Context, repoID string, exts []string) ([]GGUFFile, error) {
	treeURL := fmt.Sprintf("%s/%s/tree/main", apiBase, repoID)

	resp, err := c.get(ctx, treeURL)
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

	avatarURL := c.fetchAuthorAvatar(ctx, author)

	if c.cache != nil {
		key := "hf:avatar:" + author
		_ = c.cache.Set(ctx, key, []byte(avatarURL), ttlAvatar)
	}

	return avatarURL
}

func (c *Client) fetchAuthorAvatar(ctx context.Context, author string) string {
	for _, kind := range []string{"users", "organizations"} {
		if u := c.tryFetchAvatar(ctx, kind, author); u != "" {
			return u
		}
	}
	return ""
}

func (c *Client) tryFetchAvatar(ctx context.Context, kind, author string) string {
	u := fmt.Sprintf("https://%s/api/%s/%s/overview", apiHost, kind, url.PathEscape(author))
	resp, err := c.get(ctx, u)
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
