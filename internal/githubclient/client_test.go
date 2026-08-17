package githubclient

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
)

func jsonBody(t *testing.T, v any) string {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func TestLatestRelease(t *testing.T) {
	asset := Asset{Name: "odin-linux-amd64-dev-2026-08.tar.gz", Size: 100}
	latest := Release{TagName: "dev-2026-08", Assets: []Asset{asset}}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/repos/odin-lang/Odin/releases/latest" {
			t.Errorf("unexpected path %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(jsonBody(t, latest)))
	}))
	defer srv.Close()

	c := &Client{HTTP: srv.Client(), URL: srv.URL}
	rel, err := c.LatestRelease(context.Background(), "odin-lang", "Odin")
	if err != nil {
		t.Fatal(err)
	}
	if rel.TagName != "dev-2026-08" || len(rel.Assets) != 1 {
		t.Fatalf("unexpected release: %+v", rel)
	}
	if rel.Assets[0].Name != asset.Name {
		t.Fatalf("unexpected asset: %+v", rel.Assets[0])
	}
}

func TestLatestReleaseFallsBackToList(t *testing.T) {
	draft := Release{TagName: "dev-2026-07", Draft: true}
	published := Release{TagName: "dev-2026-08", Prerelease: true, Assets: []Asset{{Name: "a.tar.gz"}}}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/repos/odin-lang/Odin/releases/latest":
			w.WriteHeader(http.StatusNotFound)
			_, _ = w.Write([]byte(`{"message":"Not Found","documentation_url":"https://docs.github.com/rest"}`))
		case "/repos/odin-lang/Odin/releases":
			if r.URL.Query().Get("per_page") != "10" {
				t.Errorf("unexpected per_page %q", r.URL.Query().Get("per_page"))
			}
			_, _ = w.Write([]byte(jsonBody(t, []Release{draft, published})))
		default:
			t.Errorf("unexpected path %s", r.URL.Path)
		}
	}))
	defer srv.Close()

	c := &Client{HTTP: srv.Client(), URL: srv.URL}
	rel, err := c.LatestRelease(context.Background(), "odin-lang", "Odin")
	if err != nil {
		t.Fatal(err)
	}
	if rel.TagName != "dev-2026-08" {
		t.Fatalf("expected fallback release, got %+v", rel)
	}
}

func TestLatestReleaseNoPublished(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/repos/o/r/releases/latest":
			w.WriteHeader(http.StatusNotFound)
			_, _ = w.Write([]byte(`{"message":"Not Found"}`))
		case "/repos/o/r/releases":
			_, _ = w.Write([]byte("[]"))
		}
	}))
	defer srv.Close()

	c := &Client{HTTP: srv.Client(), URL: srv.URL}
	if _, err := c.LatestRelease(context.Background(), "o", "r"); err == nil {
		t.Fatal("expected error with no published releases")
	}
}

func TestAPIErrorWrapping(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		_, _ = w.Write([]byte(`{"message":"rate limit exceeded"}`))
	}))
	defer srv.Close()

	c := &Client{HTTP: srv.Client(), URL: srv.URL}
	_, err := c.LatestRelease(context.Background(), "o", "r")
	var apiErr *APIError
	if !errors.As(err, &apiErr) {
		t.Fatalf("expected *APIError, got %v", err)
	}
	if apiErr.StatusCode != http.StatusForbidden {
		t.Fatalf("expected status 403, got %d", apiErr.StatusCode)
	}
	if apiErr.Message != "rate limit exceeded" {
		t.Fatalf("unexpected message %q", apiErr.Message)
	}
}
