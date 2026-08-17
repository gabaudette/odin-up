package release

import (
	"strconv"
	"strings"
)

// cleanVersion normalizes an Odin version label by removing build markers
// appended to ODIN_VERSION by the compiler, such as "-nightly" and ":<sha>".
func cleanVersion(v string) string {
	v = strings.TrimSpace(v)
	if i := strings.IndexByte(v, ':'); i >= 0 {
		v = v[:i]
	}
	if strings.HasSuffix(v, "-nightly") && len(v) > len("-nightly") {
		v = strings.TrimSuffix(v, "-nightly")
	}
	return v
}

// tokenizeVersion splits a version into comparable components. Numeric runs
// and a single trailing letter (e.g. dev-2026-07a) are kept separate.
func tokenizeVersion(v string) []int {
	tokens := make([]int, 0)
	for _, part := range strings.FieldsFunc(cleanVersion(v), func(r rune) bool {
		return !isAlnum(r)
	}) {
		tokens = append(tokens, partTokens(part)...)
	}
	return tokens
}

// partTokens converts a single alphanumeric component into comparable tokens.
// A trailing letter (07a) is split into its numeric value plus a rank so that
// dev-2026-08 sorts after dev-2026-07a. A bare channel word (dev, nightly, v)
// becomes a semantic rank.
func partTokens(part string) []int {
	if part == "" {
		return nil
	}
	var res []int
	i := 0
	for i < len(part) {
		c := part[i]
		if c >= '0' && c <= '9' {
			j := i
			for j < len(part) && part[j] >= '0' && part[j] <= '9' {
				j++
			}
			res = append(res, atoi(part[i:j]))
			i = j
			continue
		}
		j := i
		for j < len(part) && (part[j] < '0' || part[j] > '9') {
			j++
		}
		word := part[i:j]
		if i > 0 && part[i-1] >= '0' && part[i-1] <= '9' {
			// Trailing letter chain right after digits (07a): rank each letter.
			for k := i; k < j; k++ {
				res = append(res, 2000+toAlpha(part[k]))
			}
		} else {
			res = append(res, alphaWord(word))
		}
		i = j
	}
	return res
}

func isAlnum(r rune) bool {
	return (r >= '0' && r <= '9') || (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z')
}

func atoi(s string) int {
	n, _ := strconv.Atoi(s)
	return n
}

func toAlpha(s byte) int {
	l := byte(s)
	if l >= 'a' && l <= 'z' {
		return int(l - 'a' + 1)
	}
	return int(l - 'A' + 1)
}

// alphaWord ranks channel words so v < dev < nightly and unknown words fall
// back to a stable ordering.
func alphaWord(s string) int {
	switch strings.ToLower(s) {
	case "v":
		return 2
	case "dev":
		return 10
	case "nightly":
		return 20
	default:
		return 100
	}
}

// CompareVersions orders two Odin version labels. It returns -1 when a is
// older than b, 0 when they are equal, and 1 when a is newer than b.
// Unknown formats fall back to lexicographic comparison.
func CompareVersions(a, b string) int {
	ta, tb := tokenizeVersion(a), tokenizeVersion(b)
	if len(ta) == 0 && len(tb) == 0 {
		return strings.Compare(a, b) // fallback
	}
	if len(ta) == 0 {
		return -1
	}
	if len(tb) == 0 {
		return 1
	}
	for i := 0; i < len(ta) && i < len(tb); i++ {
		if ta[i] != tb[i] {
			if ta[i] < tb[i] {
				return -1
			}
			return 1
		}
	}
	switch {
	case len(ta) < len(tb):
		return -1
	case len(ta) > len(tb):
		return 1
	}
	return 0
}

// VersionsEqual reports whether two labels identify the same release.
func VersionsEqual(a, b string) bool {
	return CompareVersions(a, b) == 0
}
