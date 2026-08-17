package githubclient

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

const (
	apiBase       = "https://api.github.com"
	userAgent     = "odin-up"
	version       = "0.1.0"
	maxKBReadBack = 8 << 10
)

// Asset is a downloadable artifact attached to a release.
type Asset struct {
	Name        string `json:"name"`
	DownloadURL string `json:"browser_download_url"`
	Size        int64  `json:"size"`
}

// Release is a subset of the GitHub releases API payload.
type Release struct {
	TagName    string  `json:"tag_name"`
	URL        string  `json:"html_url"`
	Draft      bool    `json:"draft"`
	Prerelease bool    `json:"prerelease"`
	Assets     []Asset `json:"assets"`
}

// Client talks to the GitHub REST API.
type Client struct {
	HTTP *http.Client
	// Owner and Repo default to the Odin repository.
	URL string
}

func New() *Client {
	return &Client{
		HTTP: &http.Client{Timeout: 30 * time.Second},
		URL:  apiBase,
	}
}

type APIError struct {
	StatusCode int
	Message    string
}

func (e *APIError) Error() string {
	if e.Message != "" {
		return fmt.Sprintf("GitHub API request failed (HTTP %d): %s", e.StatusCode, e.Message)
	}
	return fmt.Sprintf("GitHub API request failed (HTTP %d)", e.StatusCode)
}

// LatestRelease returns the newest published release of the repository. If no
// "latest" release exists (for example when every release is a prerelease) it
// falls back to the most recent non-draft release.
func (c *Client) LatestRelease(ctx context.Context, owner, repo string) (*Release, error) {
	rel, err := c.getRelease(ctx, owner, repo, "/releases/latest")
	if err == nil {
		return rel, nil
	}
	var apiErr *APIError
	if ok := findAPIError(err, &apiErr); ok && apiErr.StatusCode == http.StatusNotFound {
		releases := []Release{}
		obj := &releases
		if err := c.get(ctx, c.URL+"/repos/"+owner+"/"+repo+"/releases?per_page=10", obj); err != nil {
			return nil, err
		}
		for _, r := range releases {
			if r.Draft {
				continue
			}
			rel := r
			return &rel, nil
		}
		return nil, fmt.Errorf("no published releases found for %s/%s", owner, repo)
	}
	return nil, err
}

func (c *Client) getRelease(ctx context.Context, owner, repo, path string) (*Release, error) {
	rel := &Release{}
	if err := c.get(ctx, c.URL+"/repos/"+owner+"/"+repo+path, rel); err != nil {
		return nil, err
	}
	return rel, nil
}

func (c *Client) get(ctx context.Context, url string, out any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("User-Agent", userAgent+"/"+version)

	resp, err := c.HTTP.Do(req)
	if err != nil {
		return fmt.Errorf("request to %s failed: %w", url, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body := readLimit(resp.Body)
		return &APIError{StatusCode: resp.StatusCode, Message: summarize(body)}
	}

	if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
		return fmt.Errorf("decoding response from %s: %w", url, err)
	}
	return nil
}

func readLimit(r io.Reader) string {
	lr := io.LimitReader(r, maxKBReadBack)
	b, _ := io.ReadAll(lr)
	return string(b)
}

func summarize(body string) string {
	msg := struct {
		Message string `json:"message"`
	}{}
	if json.Unmarshal([]byte(body), &msg) == nil && msg.Message != "" {
		return msg.Message
	}
	trimmed := strings.TrimSpace(body)
	if len(trimmed) > 300 {
		trimmed = trimmed[:300] + " ..."
	}
	return trimmed
}

func findAPIError(err error, out **APIError) bool {
	for err != nil {
		if apiErr, ok := err.(*APIError); ok {
			*out = apiErr
			return true
		}
		type unwrapper interface{ Unwrap() error }
		u, ok := err.(unwrapper)
		if !ok {
			return false
		}
		err = u.Unwrap()
	}
	return false
}
