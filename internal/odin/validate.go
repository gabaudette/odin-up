package odin

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"odin-up/internal/release"
	"odin-up/internal/system"
)

var (
	looksLikeVersionRe = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9.+-]*$`)
	// prefixColonRe matches lines such as "Odin: dev-2026-08" where the label
	// follows a single-word prefix and a colon.
	prefixColonRe = regexp.MustCompile(`^\S+:\s*(\S+)$`)
)

func looksLikeVersion(s string) bool {
	if s == "" || !containsDigit(s) {
		return false
	}
	return looksLikeVersionRe.MatchString(s)
}

func containsDigit(s string) bool {
	for _, r := range s {
		if r >= '0' && r <= '9' {
			return true
		}
	}
	return false
}

func extractVersion(line string) string {
	var candidates []string
	add := func(c string) {
		c = strings.TrimSpace(c)
		candidates = append(candidates, c)
		if i := strings.IndexByte(c, ':'); i >= 0 {
			candidates = append(candidates, c[:i])
		}
	}
	if i := strings.LastIndex(line, " version "); i >= 0 {
		add(line[i+len(" version "):])
	}
	if m := prefixColonRe.FindStringSubmatch(line); m != nil {
		add(m[1])
	}
	// Bare label with a build sha: "dev-2026-08-nightly:abcdef12".
	if !strings.ContainsAny(line, " \t") {
		add(line)
	}
	add(line)
	for _, c := range candidates {
		if looksLikeVersion(c) {
			return c
		}
	}
	return ""
}

// ParseVersion extracts an Odin version label from the output of
// "odin version". Known formats: "<path> version <label>", "Odin: <label>",
// a bare "<label>" and "<label>:<sha>". When the output cannot be parsed,
// versionFromDirName is used as a fallback.
func ParseVersion(output, versionFromDirName string) string {
	for _, line := range strings.Split(output, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		if v := extractVersion(line); v != "" {
			return normalizeVersion(v)
		}
	}
	if looksLikeVersion(versionFromDirName) {
		return versionFromDirName
	}
	return ""
}

// normalizeVersion strips build markers appended by the compiler (-nightly,
// :<sha>) from a version label.
func normalizeVersion(v string) string {
	v = strings.TrimSpace(v)
	if i := strings.IndexByte(v, ':'); i >= 0 {
		v = v[:i]
	}
	if strings.HasSuffix(v, "-nightly") && len(v) > len("-nightly") {
		v = strings.TrimSuffix(v, "-nightly")
	}
	return v
}

// validateInstall verifies a candidate installation directory: the odin
// executable exists and runs, and the core library ships alongside it.
// It returns the detected version.
func validateInstall(runner system.Runner, dir string) (string, error) {
	bin := filepath.Join(dir, "odin")
	info, err := os.Stat(bin)
	if err != nil {
		return "", fmt.Errorf("missing Odin executable: %s (expected at %s)", err, bin)
	}
	if info.IsDir() {
		return "", fmt.Errorf("%s is a directory, not the Odin executable", bin)
	}
	if info.Mode().Perm()&0o111 == 0 {
		return "", fmt.Errorf("%s is not executable", bin)
	}
	if coreInfo, err := os.Stat(filepath.Join(dir, "core")); err != nil || !coreInfo.IsDir() {
		return "", fmt.Errorf("invalid installation: Odin core library directory is missing next to the compiler")
	}
	out, err := runner.Output(bin, "version")
	if err != nil {
		return "", fmt.Errorf("running '%s version' failed: %w", bin, err)
	}
	version := ParseVersion(out, release.VersionFromDirName(filepath.Base(dir)))
	if version == "" {
		return "", fmt.Errorf("could not determine Odin version from '%s version'", bin)
	}
	return version, nil
}
