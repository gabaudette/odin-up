package odin

import (
	"archive/tar"
	"compress/gzip"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestValidArchiveEntry(t *testing.T) {
	valid := []string{"odin", "core/foo.odin", "core/vendor/bar", "./odin", "a/b/c/odin"}

	for _, name := range valid {
		if !validArchiveEntry(name) {
			t.Errorf("validArchiveEntry(%q) = false, want true", name)
		}
	}

	invalid := []string{"", "..", "../escape", "a/../escape", "/abs", "/abs/odin", "a/../../escape"}

	for _, name := range invalid {
		if validArchiveEntry(name) {
			t.Errorf("validArchiveEntry(%q) = true, want false", name)
		}
	}
}

func TestSanitizeArchivePath(t *testing.T) {
	dst := t.TempDir()
	good := filepath.Join(dst, "odin")
	got, ok := sanitizeArchivePath(dst, "odin")

	if !ok || got != good {
		t.Fatalf("sanitizeArchivePath(odin) = %q, %v; want %q", got, ok, good)
	}

	for _, name := range []string{"../escape", "/etc/passwd", "a/../../escape", ".", ""} {
		if _, ok := sanitizeArchivePath(dst, name); ok {
			t.Errorf("sanitizeArchivePath(%q): expected rejection", name)
		}
	}
}

func writeTarGz(t *testing.T, files map[string]string, traversal string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "archive.tar.gz")
	f, err := os.Create(path)

	if err != nil {
		t.Fatal(err)
	}

	defer f.Close()

	gz := gzip.NewWriter(f)
	tw := tar.NewWriter(gz)

	if traversal != "" {
		_ = tw.WriteHeader(&tar.Header{Name: traversal, Mode: 0o755, Typeflag: tar.TypeDir})
	}

	for name, content := range files {
		hdr := &tar.Header{Name: name, Mode: 0o644, Size: int64(len(content)), Typeflag: tar.TypeReg}

		if err := tw.WriteHeader(hdr); err != nil {
			t.Fatal(err)
		}

		if _, err := tw.Write([]byte(content)); err != nil {
			t.Fatal(err)
		}
	}

	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}

	if err := gz.Close(); err != nil {
		t.Fatal(err)
	}

	return path
}

func TestExtractArchive(t *testing.T) {
	archive := writeTarGz(t, map[string]string{
		"odin-linux-amd64-dev-2026-08/odin":      "#!/bin/sh\necho odin\n",
		"odin-linux-amd64-dev-2026-08/core/core": "core file\n",
	}, "")

	dst := t.TempDir()

	if err := extractArchive(archive, dst); err != nil {
		t.Fatal(err)
	}

	if _, err := os.Stat(filepath.Join(dst, "odin-linux-amd64-dev-2026-08", "odin")); err != nil {
		t.Fatalf("odin binary not extracted: %v", err)
	}

	if _, err := os.Stat(filepath.Join(dst, "odin-linux-amd64-dev-2026-08", "core", "core")); err != nil {
		t.Fatalf("core dir not extracted: %v", err)
	}
}

func TestExtractArchiveRejectsTraversal(t *testing.T) {
	archive := writeTarGz(t, map[string]string{
		"odin": "content",
	}, "../evil")

	dst := t.TempDir()

	if err := extractArchive(archive, dst); err == nil {
		t.Fatal("expected error extracting traversal entry")
	}

	if _, err := os.Stat(filepath.Join(t.TempDir(), "evil")); err == nil {
		t.Fatal("traversal entry escaped destination")
	}
}

func TestExtractArchiveIgnoresSymlinks(t *testing.T) {
	path := filepath.Join(t.TempDir(), "archive.tar.gz")
	f, err := os.Create(path)

	if err != nil {
		t.Fatal(err)
	}

	defer f.Close()
	gz := gzip.NewWriter(f)
	tw := tar.NewWriter(gz)

	_ = tw.WriteHeader(&tar.Header{Name: "odin", Linkname: "/etc/passwd", Typeflag: tar.TypeSymlink})
	_ = tw.WriteHeader(&tar.Header{Name: "odin", Mode: 0o644, Size: 4, Typeflag: tar.TypeReg})

	_, _ = tw.Write([]byte("data"))

	_ = tw.Close()
	_ = gz.Close()

	dst := t.TempDir()

	if err := extractArchive(path, dst); err != nil {
		t.Fatal(err)
	}

	if _, err := os.Stat(filepath.Join(dst, "odin")); err != nil {
		t.Fatalf("regular odin file not extracted: %v", err)
	}
}

func TestFindOdinDir(t *testing.T) {
	root := t.TempDir()

	for _, name := range []string{"bin", "doc"} {
		_ = os.Mkdir(filepath.Join(root, name), 0o755)
	}

	// No odin binary anywhere: must not return the root itself.
	if _, err := findOdinDir(root); err == nil {
		t.Fatal("expected error when no odin binary is present")
	}

	// Candidate with an odin executable wins, even without core.
	inst := filepath.Join(root, "odin-linux-amd64-dev-2026-08")

	_ = os.MkdirAll(inst, 0o755)
	_ = os.WriteFile(filepath.Join(inst, "odin"), []byte("x"), 0o755)

	if dir, err := findOdinDir(root); err != nil || dir != inst {
		t.Fatalf("findOdinDir = %q, %v; want %q", dir, err, inst)
	}

	// With multiple candidates, prefer the one carrying core.
	inst2 := filepath.Join(root, "odin-linux-amd64-dev-2026-07")

	_ = os.MkdirAll(filepath.Join(inst2, "core"), 0o755)
	_ = os.WriteFile(filepath.Join(inst2, "odin"), []byte("x"), 0o755)

	if dir, err := findOdinDir(root); err != nil || dir != inst2 {
		t.Fatalf("findOdinDir = %q, %v; want %q", dir, err, inst2)
	}
}

func TestParseVersion(t *testing.T) {
	cases := []struct{ out, dir, want string }{
		{"Odin: dev-2026-08\n", "odin-linux-amd64-dev-2026-08", "dev-2026-08"},
		{"Odin: v0.13.0\n", "odin-linux-amd64-v0.13.0", "v0.13.0"},
		// Build markers are normalized away.
		{"Odin: dev-2026-08-nightly:abcdef12\n", "odin-linux-amd64-dev-2026-08", "dev-2026-08"},
		// The " version " marker wins over the raw label.
		{"/opt/odin/current/odin version v1.2.3\n", "odin-linux-amd64-dev-2026-08", "v1.2.3"},
		// Fallback is the literal dir name when it looks like a version.
		{"", "odin-linux-amd64-dev-2026-08", "odin-linux-amd64-dev-2026-08"},
		{"", "some-random-dir", ""},
		{"garbage output", "", ""},
	}

	for _, c := range cases {
		got := ParseVersion(c.out, c.dir)

		if got != c.want {
			t.Errorf("ParseVersion(%q, %q) = %q, want %q", c.out, c.dir, got, c.want)
		}
	}
}

func TestIsBinLinkManaged(t *testing.T) {
	tests := []struct {
		name string
		ok   bool
	}{
		{"/opt/odin/current/odin", true},
		{"/opt/odin/current", true},
		{"odin", false},
		{"current/odin", false},
		{"../local/bin/odin", false},
		{"/tmp/odin/current/odin", false},
		{"/opt/odinx/current/odin", false},
		{"/opt/odin", false},
		{"", false},
	}
	for _, c := range tests {
		if got := isBinLinkManaged(c.name); got != c.ok {
			t.Errorf("isBinLinkManaged(%q) = %v, want %v", c.name, got, c.ok)
		}
	}
}

func TestObsoleteVersionNames(t *testing.T) {
	root := t.TempDir()

	_ = os.Mkdir(filepath.Join(root, "dev-2026-07"), 0o755)
	_ = os.Mkdir(filepath.Join(root, "dev-2026-08"), 0o755)
	_ = os.Mkdir(filepath.Join(root, "dev-2026-06"), 0o755)
	_ = os.WriteFile(filepath.Join(root, "note.txt"), []byte("hi"), 0o644)
	_ = os.Symlink("dev-2026-08", filepath.Join(root, "dev-2026-09"))

	obsolete, err := obsoleteVersionNames(root, "dev-2026-08")

	if err != nil {
		t.Fatal(err)
	}

	if len(obsolete) != 2 {
		t.Fatalf("expected 2 obsolete versions, got %v", obsolete)
	}

	for _, name := range obsolete {
		if name == "dev-2026-08" || name == "note.txt" || name == "dev-2026-09" {
			t.Errorf("unexpected obsolete entry %q", name)
		}
	}
}

func TestObsoleteVersionNamesMissingDir(t *testing.T) {
	obsolete, err := obsoleteVersionNames(t.TempDir()+"/nonexistent", "")

	if err != nil {
		t.Fatal(err)
	}
	if len(obsolete) != 0 {
		t.Fatalf("expected no obsolete versions, got %v", obsolete)
	}
}

func TestFormatStatusReportsUnmanagedOdin(t *testing.T) {
	st := &Status{Arch: "amd64", UnmanagedOdin: "/home/gabriel/odin/odin"}
	out := FormatStatus(st)

	if !strings.Contains(out, "Not installed") {
		t.Fatalf("expected Not installed state in:\n%s", out)
	}

	if !strings.Contains(out, "Note: an unmanaged odin was found at /home/gabriel/odin/odin") {
		t.Fatalf("expected unmanaged note in:\n%s", out)
	}
}

func TestFormatStatusNoUnmanagedNote(t *testing.T) {
	out := FormatStatus(&Status{Arch: "amd64"})

	if strings.Contains(out, "unmanaged") {
		t.Fatalf("unexpected unmanaged note in:\n%s", out)
	}
}

func TestPathWithin(t *testing.T) {
	cases := []struct {
		p    string
		base string
		ok   bool
	}{
		{"/opt/odin/x/odin", "/opt/odin", true},
		{"/opt/odin/versions/v1/odin", "/opt/odin/versions", true},
		{"/opt/odin", "/opt/odin", false},
		{"/opt/odinx/odin", "/opt/odin", false},
		{"/usr/local/bin/odin", "/opt/odin", false},
		{"", "/opt/odin", false},
	}
	for _, c := range cases {
		if got := pathWithin(c.p, c.base); got != c.ok {
			t.Errorf("pathWithin(%q, %q) = %v, want %v", c.p, c.base, got, c.ok)
		}
	}
}

func TestExistingInstallDir(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "odin"), []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}

	t.Setenv("PATH", dir)

	want, _ := filepath.EvalSymlinks(dir)

	if got := existingInstallDir(); got != want {
		t.Fatalf("existingInstallDir() = %q, want %q", got, want)
	}

	// No odin on PATH: nothing to adopt.
	t.Setenv("PATH", t.TempDir())

	if got := existingInstallDir(); got != "" {
		t.Fatalf("expected no existing install, got %q", got)
	}
}

func TestFindUnmanagedOdin(t *testing.T) {
	dir := t.TempDir()
	cli := filepath.Join(dir, "odin")

	if err := os.WriteFile(cli, []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}

	t.Setenv("PATH", dir)
	got, err := findUnmanagedOdin()

	if err != nil {
		t.Fatal(err)
	}

	want, _ := filepath.EvalSymlinks(cli)

	if got != want {
		t.Fatalf("findUnmanagedOdin() = %q, want %q", got, want)
	}
}

func TestFindUnmanagedOdinSkipsNonExecutable(t *testing.T) {
	dir := t.TempDir()

	if err := os.WriteFile(filepath.Join(dir, "odin"), []byte("#!/bin/sh\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	t.Setenv("PATH", dir)

	got, err := findUnmanagedOdin()

	if err != nil {
		t.Fatal(err)
	}

	if got != "" {
		t.Fatalf("expected no unmanaged odin, got %q", got)
	}
}
