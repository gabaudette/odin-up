package odin

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"odin-up/internal/githubclient"
	"odin-up/internal/paths"
	"odin-up/internal/release"
	"odin-up/internal/system"
)

const (
	Owner = "odin-lang"
	Repo  = "Odin"
)

var (
	ErrAlreadyInstalled = errors.New("Odin is already installed")
	ErrNotInstalled     = errors.New("Odin is not installed")
	ErrUpToDate         = errors.New("Odin is already up to date")
	ErrCanceled         = errors.New("operation canceled")
)

// UI lets the installer ask the user questions. The TUI implements it.
type UI interface {
	ConfirmInstallDeps(missing []system.Dependency) (bool, error)
	ConfirmUninstall() (bool, error)
	ConfirmAdopt(location string) (bool, error)
}

// Installer coordinates every Odin installation operation. It performs all
// system mutations through system.Runner and reports progress through
// Reporter, so it can run under the TUI, tests, or plain CLI callers.
type Installer struct {
	Client   *githubclient.Client
	Runner   system.Runner
	UI       UI
	Reporter Reporter
}

// MissingDepsError reports required system packages that are not installed.
type MissingDepsError struct {
	Missing []system.Dependency
}

func (e *MissingDepsError) Error() string {
	names := make([]string, 0, len(e.Missing))
	for _, d := range e.Missing {
		names = append(names, d.Package)
	}
	return "missing required dependencies: " + strings.Join(names, ", ")
}

func (in *Installer) checkOS() error {
	return system.CheckOS(runtime.GOOS)
}

func (in *Installer) report(ev Event) {
	if in.Reporter != nil {
		in.Reporter(ev)
	}
}

func (in *Installer) step(label string) {
	in.report(Event{Kind: EventStep, Label: label})
}

// Install performs a fresh Odin installation.
func (in *Installer) Install(ctx context.Context) error {
	if err := in.checkOS(); err != nil {
		return err
	}
	arch, err := system.DetectArch()
	if err != nil {
		return err
	}
	if _, err := os.Lstat(paths.CurrentLink); err == nil {
		return ErrAlreadyInstalled
	}

	// If an unmanaged Odin installation already exists and we can ask the
	// user, offer to adopt it into the managed layout instead of downloading
	// a fresh copy.
	if in.UI != nil {
		if srcDir := existingInstallDir(); srcDir != "" {
			ok, err := in.UI.ConfirmAdopt(srcDir)
			if err != nil {
				return err
			}
			if ok {
				in.step("Adopting existing installation")
				return adoptExisting(in.Runner, srcDir)
			}
		}
	}

	in.step("Checking dependencies")
	missing := system.MissingDependencies()
	if len(missing) > 0 {
		if in.UI == nil {
			return &MissingDepsError{Missing: missing}
		}
		ok, err := in.UI.ConfirmInstallDeps(missing)
		if err != nil {
			return err
		}
		if !ok {
			return ErrCanceled
		}
		in.step("Installing dependencies")
		if err := system.InstallDependencies(in.Runner, missing); err != nil {
			return err
		}
	}

	rel, err := in.fetchLatest(ctx)
	if err != nil {
		return err
	}
	asset, err := release.SelectAsset(rel.Assets, arch)
	if err != nil {
		return err
	}
	in.step("Downloading " + rel.TagName)
	return in.installRelease(ctx, asset, "")
}

// Update upgrades the managed installation to the latest release. The existing
// installation is never touched until the new one has been downloaded,
// extracted and validated.
func (in *Installer) Update(ctx context.Context) error {
	if err := in.checkOS(); err != nil {
		return err
	}
	arch, err := system.DetectArch()
	if err != nil {
		return err
	}
	if _, err := os.Lstat(paths.CurrentLink); err != nil {
		return ErrNotInstalled
	}

	in.step("Detecting installed version")
	installed := CurrentVersion(in.Runner)
	if installed == "" {
		installed = release.VersionFromDirName(ActiveVersionName())
	}
	rel, err := in.fetchLatest(ctx)
	if err != nil {
		return err
	}
	if installed != "" && release.VersionsEqual(installed, rel.TagName) {
		return ErrUpToDate
	}
	asset, err := release.SelectAsset(rel.Assets, arch)
	if err != nil {
		return err
	}
	in.step("Downloading " + rel.TagName)
	return in.installRelease(ctx, asset, installed)
}

// adoptExisting moves an unmanaged Odin installation on PATH into the managed
// layout and activates it. /usr/local/bin/odin is only ever replaced when it
// points at the adopted directory; anything else is refused.
func adoptExisting(runner system.Runner, srcDir string) error {
	version, err := validateInstall(runner, srcDir)
	if err != nil {
		return fmt.Errorf("cannot adopt existing installation: %w", err)
	}
	name, err := release.VersionDirName(version)
	if err != nil {
		name, err = release.VersionDirName(filepath.Base(srcDir))
		if err != nil {
			return fmt.Errorf("cannot adopt existing installation: %w", err)
		}
	}

	// Decide whether /usr/local/bin/odin may be touched before the source
	// directory moves and its link starts dangling.
	managed, present, err := binLinkStatus()
	if err != nil {
		return err
	}
	replaceLink := false
	if present && !managed {
		target, err := os.Readlink(paths.OdinBinLink)
		if err != nil {
			return err
		}
		resolved := target
		if !filepath.IsAbs(target) {
			resolved = filepath.Join(filepath.Dir(paths.OdinBinLink), target)
		}
		if pathWithin(filepath.Clean(resolved), srcDir) {
			replaceLink = true
		} else {
			return fmt.Errorf("%s points elsewhere and is not managed by odin-up. Refusing to overwrite it", paths.OdinBinLink)
		}
	}

	if err := runner.EnsurePrivileges(); err != nil {
		return err
	}
	if err := ensureVersionsDir(runner); err != nil {
		return err
	}
	if err := moveIntoVersions(runner, srcDir, name); err != nil {
		return err
	}
	if err := switchCurrent(runner, name); err != nil {
		return err
	}

	switch {
	case managed:
	case present && replaceLink:
		if err := runner.RunPrivileged("ln", "-sfn", paths.CurrentBinary(), paths.OdinBinLink); err != nil {
			return fmt.Errorf("failed to update %s: %w", paths.OdinBinLink, err)
		}
	default:
		if err := runner.RunPrivileged("ln", "-s", paths.CurrentBinary(), paths.OdinBinLink); err != nil {
			return fmt.Errorf("failed to create %s: %w", paths.OdinBinLink, err)
		}
	}

	if _, err := validateInstall(runner, paths.VersionDir(name)); err != nil {
		return fmt.Errorf("adopted installation failed verification: %w", err)
	}
	return nil
}

// pathWithin reports whether p is lexically located inside base.
func pathWithin(p, base string) bool {
	return strings.HasPrefix(filepath.Clean(p), filepath.Clean(base)+string(filepath.Separator))
}

// existingInstallDir returns the directory of an unmanaged Odin installation
// found on PATH, or "" when none exists.
func existingInstallDir() string {
	bin, err := findUnmanagedOdin()
	if err != nil || bin == "" {
		return ""
	}
	return filepath.Dir(bin)
}

func (in *Installer) fetchLatest(ctx context.Context) (*githubclient.Release, error) {
	in.step("Fetching latest release")
	return in.Client.LatestRelease(ctx, Owner, Repo)
}

// installRelease downloads, validates and activates a release. When keepVersion
// is non-empty (update), obsolete versions are pruned afterwards.
func (in *Installer) installRelease(ctx context.Context, asset *githubclient.Asset, keepVersion string) error {
	tmpDir, err := os.MkdirTemp("", "odin-up-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(tmpDir)

	archive := filepath.Join(tmpDir, asset.Name)
	if err := in.download(ctx, asset, archive); err != nil {
		return err
	}

	in.step("Extracting archive")
	extractDir := filepath.Join(tmpDir, "extract")
	if err := os.MkdirAll(extractDir, 0o755); err != nil {
		return err
	}
	if err := extractArchive(archive, extractDir); err != nil {
		return err
	}
	candidate, err := findOdinDir(extractDir)
	if err != nil {
		return err
	}
	name := filepath.Base(candidate)
	if err := validateVersionDirName(name); err != nil {
		return err
	}

	in.step("Validating installation")
	if _, err := validateInstall(in.Runner, candidate); err != nil {
		return err
	}

	if err := in.Runner.EnsurePrivileges(); err != nil {
		return err
	}

	in.step("Installing to " + paths.VersionsDir)
	if err := ensureVersionsDir(in.Runner); err != nil {
		return err
	}
	if err := moveIntoVersions(in.Runner, candidate, name); err != nil {
		return err
	}

	in.step("Activating version " + name)
	if err := switchCurrent(in.Runner, name); err != nil {
		return err
	}

	in.step("Updating " + paths.OdinBinLink)
	if err := ensureBinLink(in.Runner); err != nil {
		return err
	}

	in.step("Final verification")
	if _, err := validateInstall(in.Runner, paths.VersionDir(name)); err != nil {
		return err
	}

	if keepVersion != "" {
		in.step("Cleaning up old versions")
		if err := removeObsoleteVersions(in.Runner, name); err != nil {
			return err
		}
	}
	in.step("Cleaning up")
	return nil
}

// Uninstall removes the managed installation after user confirmation. Only
// odin-up managed paths are removed.
func (in *Installer) Uninstall() error {
	if !isManagedRoot() {
		return fmt.Errorf("no odin-up managed installation found at %s", paths.OdinRoot)
	}
	if in.UI == nil {
		return errors.New("uninstall requires interactive confirmation")
	}
	ok, err := in.UI.ConfirmUninstall()
	if err != nil {
		return err
	}
	if !ok {
		return ErrCanceled
	}

	managed, present, err := binLinkStatus()
	if err != nil {
		return err
	}
	if err := in.Runner.EnsurePrivileges(); err != nil {
		return err
	}

	in.step("Removing " + paths.OdinRoot)
	if err := removePrivRecursive(in.Runner, paths.OdinRoot); err != nil {
		return err
	}
	if present && managed {
		in.step("Removing " + paths.OdinBinLink)
		if err := removePrivRecursive(in.Runner, paths.OdinBinLink); err != nil {
			return err
		}
	}
	if _, err := os.Lstat(paths.OdinRoot); err == nil {
		return fmt.Errorf("failed to remove %s", paths.OdinRoot)
	}
	if present && managed {
		if _, err := os.Lstat(paths.OdinBinLink); err == nil {
			return fmt.Errorf("failed to remove %s", paths.OdinBinLink)
		}
	}
	return nil
}

// download streams a release asset to disk, reporting progress.
func (in *Installer) download(ctx context.Context, asset *githubclient.Asset, dest string) error {
	client := &http.Client{Timeout: 0}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, asset.DownloadURL, nil)
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", "odin-up")

	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("failed to download Odin release.\nURL: %s\nReason: %v", asset.DownloadURL, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("failed to download Odin release (HTTP %d).\nURL: %s", resp.StatusCode, asset.DownloadURL)
	}

	file, err := os.Create(dest)
	if err != nil {
		return err
	}
	pw := &progressWriter{total: resp.ContentLength, reporter: in.Reporter}
	written, err := io.Copy(io.MultiWriter(file, pw), resp.Body)
	if err != nil {
		file.Close()
		os.Remove(dest)
		return fmt.Errorf("download interrupted: %w", err)
	}
	if err := file.Close(); err != nil {
		os.Remove(dest)
		return err
	}
	if resp.ContentLength > 0 && written != resp.ContentLength {
		os.Remove(dest)
		return fmt.Errorf("download incomplete: expected %d bytes, received %d", resp.ContentLength, written)
	}
	return nil
}

type progressWriter struct {
	total    int64
	written  int64
	last     float64
	reporter Reporter
}

func (w *progressWriter) Write(p []byte) (int, error) {
	n := len(p)
	w.written += int64(n)
	if w.total > 0 {
		pct := float64(w.written) / float64(w.total)
		if pct-w.last >= 0.01 || pct >= 1.0 {
			w.last = pct
			w.reporter(Event{Kind: EventProgress, Percent: pct})
		}
	}
	return n, nil
}
