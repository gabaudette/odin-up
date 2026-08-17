package odin

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"odin-up/internal/paths"
	"odin-up/internal/release"
	"odin-up/internal/system"
)

// Managed filesystem operations. Every privileged write is performed through
// the Runner so the process never needs to run as root. Go-side calls are
// read-only inspections and never follow untrusted symlinks.

// ActiveVersionName returns the name of the active version directory, or ""
// when no installation exists.
func ActiveVersionName() string {
	target, err := os.Readlink(paths.CurrentLink)
	if err != nil {
		return ""
	}
	return filepath.Base(paths.ActiveTarget(target))
}

// CurrentVersion runs "odin version" against the managed binary and returns
// the detected version, or "" when nothing is installed.
func CurrentVersion(runner system.Runner) string {
	if _, err := os.Lstat(paths.CurrentBinary()); err != nil {
		return ""
	}
	out, err := runner.Output(paths.CurrentBinary(), "version")
	if err != nil {
		return ""
	}
	return ParseVersion(out, "")
}

// binLinkStatus inspects /usr/local/bin/odin without modifying it.
func binLinkStatus() (managed bool, present bool, err error) {
	info, err := os.Lstat(paths.OdinBinLink)
	if errors.Is(err, os.ErrNotExist) {
		return false, false, nil
	}
	if err != nil {
		return false, true, err
	}
	if info.Mode()&os.ModeSymlink == 0 {
		return false, true, nil
	}
	target, err := os.Readlink(paths.OdinBinLink)
	if err != nil {
		return false, true, err
	}
	return isBinLinkManaged(target), true, nil
}

// isBinLinkManaged reports whether a /usr/local/bin/odin symlink target is
// managed by odin-up. Pure lexical check, independent of the filesystem, for
// determinism and testability.
func isBinLinkManaged(target string) bool {
	if target == "" {
		return false
	}
	resolved := target
	if !filepath.IsAbs(target) {
		resolved = filepath.Join(filepath.Dir(paths.OdinBinLink), target)
	}
	clean := filepath.Clean(resolved)
	if clean == filepath.Clean(paths.CurrentBinary()) {
		return true
	}
	root := filepath.Clean(paths.OdinRoot)
	return strings.HasPrefix(clean, root+string(filepath.Separator))
}

// isManagedRoot reports whether /opt/odin is an installation we manage.
func isManagedRoot() bool {
	info, err := os.Stat(paths.OdinRoot)
	if err != nil || !info.IsDir() {
		return false
	}
	if _, err := os.Lstat(paths.VersionsDir); err == nil {
		return true
	}
	if _, err := os.Lstat(paths.CurrentLink); err == nil {
		return true
	}
	return false
}

// ensureVersionsDir creates the managed versions directory.
func ensureVersionsDir(runner system.Runner) error {
	return runner.RunPrivileged("mkdir", "-p", paths.VersionsDir)
}

// moveIntoVersions moves a validated installation directory into
// /opt/odin/versions/<name>. The name must be pre-validated with
// release.VersionDirName.
func moveIntoVersions(runner system.Runner, srcDir, name string) error {
	target := paths.VersionDir(name)
	if _, err := os.Lstat(target); err == nil {
		if err := runner.RunPrivileged("rm", "-rf", target); err != nil {
			return fmt.Errorf("failed to remove stale version directory %s: %w", target, err)
		}
	}
	if err := runner.RunPrivileged("mv", srcDir, target); err != nil {
		return fmt.Errorf("failed to move %s into %s: %w", srcDir, paths.VersionsDir, err)
	}
	return nil
}

// switchCurrent atomically repoints /opt/odin/current via a temporary symlink
// and rename so the link never disappears.
func switchCurrent(runner system.Runner, name string) error {
	tmpName, err := randomName(".current-")
	if err != nil {
		return err
	}
	tmpPath := filepath.Join(paths.OdinRoot, tmpName)
	defer func() {
		_ = runner.RunPrivileged("rm", "-f", tmpPath)
	}()
	if err := runner.RunPrivileged("ln", "-s", "versions/"+name, tmpPath); err != nil {
		return fmt.Errorf("failed to create temporary current symlink: %w", err)
	}
	if err := runner.RunPrivileged("mv", "-T", tmpPath, paths.CurrentLink); err != nil {
		return fmt.Errorf("failed to activate version %s: %w", name, err)
	}
	return nil
}

// ensureBinLink creates or updates /usr/local/bin/odin. It never overwrites a
// file that is not managed by odin-up.
func ensureBinLink(runner system.Runner) error {
	managed, present, err := binLinkStatus()
	if err != nil {
		return err
	}
	if present && !managed {
		return fmt.Errorf("%s already exists and is not managed by odin-up. Refusing to overwrite it", paths.OdinBinLink)
	}
	if present {
		if err := runner.RunPrivileged("ln", "-sfn", paths.CurrentBinary(), paths.OdinBinLink); err != nil {
			return fmt.Errorf("failed to update %s: %w", paths.OdinBinLink, err)
		}
		return nil
	}
	if err := runner.RunPrivileged("ln", "-s", paths.CurrentBinary(), paths.OdinBinLink); err != nil {
		return fmt.Errorf("failed to create %s: %w", paths.OdinBinLink, err)
	}
	return nil
}

// removePrivRecursive removes a managed path and everything below it.
func removePrivRecursive(runner system.Runner, path string) error {
	if path == "/" || path == "" {
		return errors.New("refusing to remove filesystem root")
	}
	return runner.RunPrivileged("rm", "-rf", path)
}

// obsoleteVersionNames returns the real directories under versionsDir that
// are not the active version. Symlinks and non-directories are never selected.
func obsoleteVersionNames(versionsDir, active string) ([]string, error) {
	entries, err := os.ReadDir(versionsDir)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var obsolete []string
	for _, e := range entries {
		name := e.Name()
		if name == active {
			continue
		}
		info, err := os.Lstat(filepath.Join(versionsDir, name))
		if err != nil || info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
			continue
		}
		obsolete = append(obsolete, name)
	}
	return obsolete, nil
}

// removeObsoleteVersions deletes every non-active version directory. Only real
// directories are removed; symlinks and unknown entries are left untouched.
func removeObsoleteVersions(runner system.Runner, active string) error {
	obsolete, err := obsoleteVersionNames(paths.VersionsDir, active)
	if err != nil {
		return err
	}
	for _, name := range obsolete {
		if err := runner.RunPrivileged("rm", "-rf", paths.VersionDir(name)); err != nil {
			return fmt.Errorf("failed to remove obsolete version %s: %w", name, err)
		}
	}
	return nil
}

// randomName returns a random hex suffix for temporary paths.
func randomName(prefix string) (string, error) {
	var b [6]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}
	return prefix + hex.EncodeToString(b[:]), nil
}

// validateVersionDirName ensures a candidate directory is usable as a managed
// version name.
func validateVersionDirName(name string) error {
	_, err := release.VersionDirName(name)
	if err != nil {
		return fmt.Errorf("refusing to use unsafe archive directory name %q", name)
	}
	return nil
}
