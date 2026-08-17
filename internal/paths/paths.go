package paths

import "path/filepath"

const (
	OdinRoot    = "/opt/odin"
	VersionsDir = "/opt/odin/versions"
	CurrentLink = "/opt/odin/current"
	OdinBinLink = "/usr/local/bin/odin"
)

func VersionDir(name string) string {
	return filepath.Join(VersionsDir, name)
}

func CurrentBinary() string {
	return filepath.Join(OdinRoot, "current", "odin")
}

// ActiveTarget resolves the target of the current link to an absolute path.
// The current link points at "versions/<name>" relative to OdinRoot.
func ActiveTarget(linkTarget string) string {
	if filepath.IsAbs(linkTarget) {
		return linkTarget
	}
	return filepath.Join(OdinRoot, linkTarget)
}
