package release

import (
	"errors"
	"fmt"
	"regexp"
	"strings"

	"odin-up/internal/githubclient"
)

// ErrNoMatchingAsset is returned when a release has no Linux archive for the
// requested architecture.
var ErrNoMatchingAsset = errors.New("no matching Linux archive in release assets")

// SelectAsset finds the Linux archive for the requested architecture in a
// release's asset list. Asset names follow the pattern
// odin-linux-<arch>-<tag>.tar.gz.
func SelectAsset(assets []githubclient.Asset, arch string) (*githubclient.Asset, error) {
	prefix := "odin-linux-" + arch + "-"
	for i := range assets {
		a := assets[i]
		if strings.HasPrefix(a.Name, prefix) && strings.HasSuffix(a.Name, ".tar.gz") {
			return &a, nil
		}
	}
	return nil, fmt.Errorf("%w: %s", ErrNoMatchingAsset, arch)
}

// safeNamePattern matches version directory names we create under
// /opt/odin/versions. Only allow safe characters and never slashes.
var safeNamePattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._+-]*$`)

// VersionDirName validates a plain (non-path) name for use as a version
// directory under /opt/odin/versions.
func VersionDirName(name string) (string, error) {
	if !safeNamePattern.MatchString(name) {
		return "", fmt.Errorf("invalid version directory name: %q", name)
	}
	return name, nil
}

// VersionFromDirName derives the Odin version label from an extracted
// directory name such as "odin-linux-amd64-dev-2026-08".
func VersionFromDirName(dir string) string {
	re := regexp.MustCompile(`^odin-linux-(?:amd64|arm64)-`)
	return re.ReplaceAllString(dir, "")
}
