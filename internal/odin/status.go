package odin

import (
	"context"
	"fmt"
	"os"
	"strings"

	"odin-up/internal/paths"
	"odin-up/internal/release"
	"odin-up/internal/system"
)

// Status describes the current Odin installation and the latest release.
type Status struct {
	Installed       bool
	Version         string
	VersionName     string
	Arch            string
	Location        string
	CurrentTarget   string
	BinLinkPresent  bool
	BinLinkManaged  bool
	BinLinkTarget   string
	LatestVersion   string
	UpdateAvailable bool
	LatestError     string
}

// Status collects installation and release information without modifying the
// system.
func (in *Installer) Status(ctx context.Context) (*Status, error) {
	arch, err := system.DetectArch()
	if err != nil {
		return nil, err
	}
	st := &Status{Arch: arch, Location: paths.CurrentLink}

	if _, err := os.Lstat(paths.CurrentLink); err == nil {
		st.Installed = true
		st.VersionName = ActiveVersionName()
		if target, err := os.Readlink(paths.CurrentLink); err == nil {
			st.CurrentTarget = paths.ActiveTarget(target)
		}
		st.Version = CurrentVersion(in.Runner)
		if st.Version == "" {
			st.Version = release.VersionFromDirName(st.VersionName)
		}
	}

	if target, err := os.Readlink(paths.OdinBinLink); err == nil {
		st.BinLinkPresent = true
		st.BinLinkTarget = target
		st.BinLinkManaged = isBinLinkManaged(target)
	} else if info, err := os.Lstat(paths.OdinBinLink); err == nil && info.Mode()&os.ModeSymlink == 0 {
		st.BinLinkPresent = true
	}

	rel, err := in.Client.LatestRelease(ctx, Owner, Repo)
	if err != nil {
		st.LatestError = err.Error()
	} else {
		st.LatestVersion = rel.TagName
		if st.Installed && st.Version != "" && !release.VersionsEqual(st.Version, rel.TagName) {
			st.UpdateAvailable = true
		}
	}
	return st, nil
}

const (
	labelWidth = 12
)

func row(label, value string) string {
	return fmt.Sprintf("%-*s %s", labelWidth, label, value)
}

// FormatStatus renders a human-readable status report.
func FormatStatus(st *Status) string {
	var b strings.Builder
	b.WriteString("Odin installation\n\n")
	if !st.Installed {
		b.WriteString(row("Status", "Not installed") + "\n")
	} else {
		b.WriteString(row("Status", "Installed") + "\n")
		b.WriteString(row("Version", st.Version) + "\n")
		b.WriteString(row("Architecture", system.TokenToName(st.Arch)) + "\n")
		b.WriteString(row("Location", st.Location) + "\n")
		b.WriteString(row("Executable", paths.OdinBinLink) + "\n")
		if !st.BinLinkPresent {
			b.WriteString(row("Bin link", "missing") + "\n")
		} else if !st.BinLinkManaged {
			b.WriteString(row("Bin link", "not managed by odin-up") + "\n")
		}
	}

	b.WriteString("\nLatest release\n\n")
	if st.LatestError != "" {
		b.WriteString(row("Version", "unavailable") + "\n")
		b.WriteString(row("Error", st.LatestError) + "\n")
	} else {
		b.WriteString(row("Version", st.LatestVersion) + "\n")
		if st.Installed {
			statusText := "Up to date"
			if st.UpdateAvailable {
				statusText = "Update available"
			}
			b.WriteString(row("Status", statusText) + "\n")
		}
	}

	if !st.Installed {
		b.WriteString("\nRun:\n\n")
		b.WriteString("  odin-up install\n")
	}
	return b.String()
}
