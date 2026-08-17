package release

import (
	"errors"
	"strings"
	"testing"

	"odin-up/internal/githubclient"
)

func testAssets() []githubclient.Asset {
	return []githubclient.Asset{
		{Name: "odin-linux-amd64-dev-2026-08.tar.gz", Size: 100},
		{Name: "odin-linux-arm64-dev-2026-08.tar.gz", Size: 200},
		{Name: "odin-macos-amd64-dev-2026-08.tar.gz", Size: 300},
		{Name: "odin-linux-amd64-dev-2026-08.zip", Size: 400},
		{Name: "odin-linux-amd64-dev-2026-08.tar.gz.sha256", Size: 500},
	}
}

func TestSelectAsset(t *testing.T) {
	a, err := SelectAsset(testAssets(), "amd64")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if a.Name != "odin-linux-amd64-dev-2026-08.tar.gz" {
		t.Fatalf("got %s", a.Name)
	}
	if !strings.HasPrefix(a.Name, "odin-linux-amd64-") {
		t.Fatal("asset must match linux amd64 prefix")
	}
	if !strings.HasSuffix(a.Name, ".tar.gz") {
		t.Fatal("asset must be a tar.gz")
	}
}

func TestSelectAssetMissing(t *testing.T) {
	_, err := SelectAsset(testAssets(), "arm32")
	if !errors.Is(err, ErrNoMatchingAsset) {
		t.Fatalf("expected ErrNoMatchingAsset, got %v", err)
	}
	_, err = SelectAsset(nil, "amd64")
	if !errors.Is(err, ErrNoMatchingAsset) {
		t.Fatalf("expected ErrNoMatchingAsset on empty list, got %v", err)
	}
}

func TestVersionDirName(t *testing.T) {
	cases := []struct{ in, want string }{
		{"dev-2026-08", "dev-2026-08"},
		{"dev-2026-08-nightly", "dev-2026-08-nightly"},
		{"v1.2.3", "v1.2.3"},
		{"  spaced  ", ""},
		{"dev-2026-08/../../etc", ""},
		{"dev 2026 08", ""},
	}
	for _, c := range cases {
		got, err := VersionDirName(c.in)
		if c.want == "" {
			if err == nil {
				t.Fatalf("VersionDirName(%q): expected error, got %q", c.in, got)
			}
			continue
		}
		if err != nil {
			t.Fatalf("VersionDirName(%q): %v", c.in, err)
		}
		if got != c.want {
			t.Fatalf("VersionDirName(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestCompareVersions(t *testing.T) {
	ordered := []string{
		"v0.1.0",
		"v0.2.0",
		"v1.0.0",
		"dev-2026-01",
		"dev-2026-07a",
		"dev-2026-07b",
		"dev-2026-08",
	}
	for i, a := range ordered {
		for j, b := range ordered {
			want := -1
			if i == j {
				want = 0
			} else if i > j {
				want = 1
			}
			if got := CompareVersions(a, b); got != want {
				t.Fatalf("CompareVersions(%q, %q) = %d, want %d", a, b, got, want)
			}
		}
	}
}

func TestVersionsEqual(t *testing.T) {
	if !VersionsEqual("dev-2026-08", "dev-2026-08") {
		t.Fatal("same versions must be equal")
	}
	if !VersionsEqual("dev-2026-08", "dev-2026-08:abc123") {
		t.Fatal("nightly with sha must equal plain nightly tag")
	}
	if !VersionsEqual("dev-2026-08", "dev-2026-08-nightly") {
		t.Fatal("nightly build must equal its tag")
	}
	if VersionsEqual("dev-2026-08", "dev-2026-07") {
		t.Fatal("different versions must not be equal")
	}
}

func TestCompareVersionsNightly(t *testing.T) {
	// Nightly builds compare equal to their tag; the sha marker is ignored.
	if CompareVersions("dev-2026-08-nightly:abc123", "dev-2026-08") != 0 {
		t.Fatal("dev-2026-08-nightly:abc123 must equal dev-2026-08")
	}
}
