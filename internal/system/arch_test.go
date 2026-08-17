package system

import (
	"testing"
)

func TestCheckOS(t *testing.T) {
	if err := CheckOS("linux"); err != nil {
		t.Fatalf("expected linux to pass, got %v", err)
	}
	for _, os := range []string{"darwin", "windows", "freebsd"} {
		err := CheckOS(os)
		if err == nil {
			t.Fatalf("expected error for %s", os)
		}
		if err.Error() != ErrLinuxOnlyText {
			t.Fatalf("unexpected message for %s: %q", os, err)
		}
	}
}

func TestMapArchToToken(t *testing.T) {
	cases := []struct {
		goarch string
		want   string
		ok     bool
	}{
		{"amd64", "amd64", true},
		{"arm64", "arm64", true},
		{"386", "", false},
		{"ppc64le", "", false},
		{"riscv64", "", false},
	}
	for _, c := range cases {
		got, err := mapArchToToken(c.goarch)
		if c.ok && err != nil {
			t.Fatalf("mapArchToToken(%s): %v", c.goarch, err)
		}
		if !c.ok && err == nil {
			t.Fatalf("mapArchToToken(%s): expected error, got %q", c.goarch, got)
		}
		if c.ok && got != c.want {
			t.Fatalf("mapArchToToken(%s) = %q, want %q", c.goarch, got, c.want)
		}
	}
}

func TestLookPathFindsSystemTools(t *testing.T) {
	for _, bin := range []string{"sh", "true", "false"} {
		if _, ok := LookPath(bin); !ok {
			t.Fatalf("expected %s to be found", bin)
		}
	}
	if _, ok := LookPath("definitely-not-a-real-binary-odin-up"); ok {
		t.Fatal("expected unknown binary to not be found")
	}
}

func TestMissingDependenciesListsOnlyMissing(t *testing.T) {
	missing := MissingDependencies()
	// Must never list an executable that is present.
	for _, d := range missing {
		if _, ok := LookPath(d.Binary); ok {
			t.Fatalf("%s reported missing but is present", d.Binary)
		}
	}
}
