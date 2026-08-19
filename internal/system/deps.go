package system

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// Dep is a system dependency required to run Odin.
type Dependency struct {
	Binary  string
	Package string
}

var RequiredDependencies = []Dependency{
	{Binary: "curl", Package: "curl"},
	{Binary: "tar", Package: "tar"},
	{Binary: "gcc", Package: "build-essential"},
	{Binary: "clang", Package: "clang"},
	{Binary: "llvm-config", Package: "llvm"},
	{Binary: "git", Package: "git"},
}

// LookPath finds an executable on PATH or in well-known system directories.
func LookPath(name string) (string, bool) {
	if p, err := exec.LookPath(name); err == nil {
		return p, true
	}

	for _, dir := range []string{"/usr/bin", "/bin", "/usr/local/bin"} {
		p := filepath.Join(dir, name)

		if info, err := os.Stat(p); err == nil && !info.IsDir() && info.Mode().Perm()&0o111 != 0 {
			return p, true
		}
	}

	return "", false
}

// MissingDependencies returns the required dependencies that are not
// installed, determined by executable availability.
func MissingDependencies() []Dependency {
	var missing []Dependency

	for _, dep := range RequiredDependencies {
		if _, ok := LookPath(dep.Binary); !ok {
			missing = append(missing, dep)
		}
	}

	return missing
}

func (d Dependency) String() string {
	return d.Binary
}

func depNames(deps []Dependency) []string {
	var names []string

	for _, d := range deps {
		names = append(names, d.Package)
	}

	return names
}

// hasApt reports whether the system has the Debian/Ubuntu apt toolchain.
func hasApt() bool {
	if _, ok := LookPath("apt-get"); ok {
		return true
	}

	_, ok := LookPath("apt")

	return ok
}

func ErrUnsupportedPackageManager() error {
	return fmt.Errorf("no apt package manager found on this system. Install the missing dependencies manually before continuing")
}

// InstallDependencies installs the given apt packages. It is only valid on
// Debian/Ubuntu-based systems that provide apt.
func InstallDependencies(runner Runner, missing []Dependency) error {
	if !hasApt() {
		return ErrUnsupportedPackageManager()
	}

	names := depNames(missing)

	// Run update and install as a single privileged invocation so the
	// terminal only has to hand off to sudo once instead of twice.
	script := "apt-get update && apt-get install -y --no-install-recommends " + strings.Join(names, " ")

	if err := runner.RunPrivileged("sh", "-c", script); err != nil {
		return fmt.Errorf("failed to install dependencies (%s): %w", strings.Join(names, " "), err)
	}

	return nil
}
