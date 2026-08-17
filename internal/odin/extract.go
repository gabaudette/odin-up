package odin

import (
	"archive/tar"
	"compress/gzip"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// validArchiveEntry reports whether a tar entry name is safe to extract.
func validArchiveEntry(name string) bool {
	if name == "" || strings.HasPrefix(name, "/") {
		return false
	}

	for _, part := range strings.Split(filepath.ToSlash(name), "/") {
		if part == ".." {
			return false
		}
	}

	return true
}

// sanitizeArchivePath joins a tar entry name under dest after cleaning. It
// rejects traversal and absolute paths. Independent of the filesystem for
// testability.
func sanitizeArchivePath(dest, name string) (string, bool) {
	if !validArchiveEntry(name) {
		return "", false
	}

	cleaned := filepath.Clean(filepath.FromSlash(name))

	if cleaned == "." || cleaned == "" {
		return "", false
	}

	return filepath.Join(dest, cleaned), true
}

// extractArchive extracts a gzip-compressed tar archive into dest, rejecting
// unsafe paths and ignoring special entries (no symlinks are ever created).
func extractArchive(archivePath, dest string) error {
	file, err := os.Open(archivePath)

	if err != nil {
		return err
	}

	defer file.Close()

	gz, err := gzip.NewReader(file)

	if err != nil {
		return fmt.Errorf("archive is not a valid gzip file: %w", err)
	}

	defer gz.Close()

	tr := tar.NewReader(gz)

	for {
		hdr, err := tr.Next()

		if err == io.EOF {
			break
		}

		if err != nil {
			return fmt.Errorf("reading archive: %w", err)
		}

		target, ok := sanitizeArchivePath(dest, hdr.Name)

		if !ok {
			// A "." root entry is harmless; ignore it. Anything else is
			// a traversal attempt or malformed path.
			if filepath.Clean(filepath.FromSlash(hdr.Name)) == "." {
				continue
			}

			return fmt.Errorf("archive contains an unsafe path: %q", hdr.Name)
		}

		mode := os.FileMode(hdr.Mode) & 0o777

		switch hdr.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(target, mode|0o700); err != nil {
				return fmt.Errorf("creating directory %s: %w", target, err)
			}
		case tar.TypeReg:
			if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
				return fmt.Errorf("creating directory for %s: %w", target, err)
			}

			out, err := os.OpenFile(target, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, mode|0o600)

			if err != nil {
				return fmt.Errorf("creating file %s: %w", target, err)
			}

			if _, err := io.Copy(out, tr); err != nil {
				out.Close()
				return fmt.Errorf("writing file %s: %w", target, err)
			}

			if err := out.Close(); err != nil {
				return fmt.Errorf("closing file %s: %w", target, err)
			}
		default:
			// Ignore symlinks, hard links and special files; they are not
			// needed and must not be recreated.
		}
	}
	return nil
}

// findOdinDir locates the extracted Odin installation directory under root.
// It prefers a single candidate, then a candidate that also carries the core
// library directory.
func findOdinDir(root string) (string, error) {
	entries, err := os.ReadDir(root)

	if err != nil {
		return "", err
	}

	var candidates []string

	for _, e := range entries {
		if !e.IsDir() {
			continue
		}

		if _, err := os.Stat(filepath.Join(root, e.Name(), "odin")); err == nil {
			candidates = append(candidates, e.Name())
		}
	}

	switch len(candidates) {
	case 0:
		return "", fmt.Errorf("extracted archive does not contain an Odin installation")
	case 1:
		return filepath.Join(root, candidates[0]), nil
	}

	for _, name := range candidates {
		if _, err := os.Stat(filepath.Join(root, name, "core")); err == nil {
			return filepath.Join(root, name), nil
		}
	}

	return filepath.Join(root, candidates[0]), nil
}
