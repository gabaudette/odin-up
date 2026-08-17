package system

import (
	"fmt"
	"runtime"
)

const ErrLinuxOnlyText = "odin-up currently supports Linux only."

// CheckOS verifies that odin-up runs on a supported operating system.
func CheckOS(goos string) error {
	if goos != "linux" {
		return fmt.Errorf("%s", ErrLinuxOnlyText)
	}

	return nil
}

// DetectArch returns the Odin release architecture token for the running
// system. Supported architectures are amd64 and arm64.
func DetectArch() (string, error) {
	return mapArchToToken(runtime.GOARCH)
}

func mapArchToToken(goarch string) (string, error) {
	switch goarch {
	case "amd64":
		return "amd64", nil
	case "arm64":
		return "arm64", nil
	default:
		return "", fmt.Errorf("unsupported architecture: %s", goarch)
	}
}

func TokenToName(token string) string {
	switch token {
	case "amd64":
		return "AMD64"
	case "arm64":
		return "ARM64"
	}

	return token
}
