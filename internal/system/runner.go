package system

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
)

// Runner executes privileged and unprivileged commands. The TUI provides an
// implementation that hands the terminal to interactive subprocesses; other
// callers may use PlainRunner.
type Runner interface {
	IsRoot() bool
	// Output runs a command directly and returns its combined output.
	Output(name string, args ...string) (string, error)
	// RunPrivileged runs a command with elevated privileges when the current
	// user is not root. Output is passed through to the terminal.
	RunPrivileged(name string, args ...string) error
	// EnsurePrivileges verifies that the user can gain elevated privileges
	// (for example via sudo), prompting for a password on the terminal when
	// necessary.
	EnsurePrivileges() error
}

// PlainRunner runs commands using os/exec directly.
type PlainRunner struct{}

func (PlainRunner) IsRoot() bool {
	return os.Geteuid() == 0
}

func (PlainRunner) Output(name string, args ...string) (string, error) {
	out, err := exec.Command(name, args...).CombinedOutput()
	if err != nil {
		if len(out) > 0 {
			return string(out), fmt.Errorf("%w: %s", err, strings.TrimSpace(string(out)))
		}
		return "", err
	}
	return string(out), nil
}

func (r PlainRunner) RunPrivileged(name string, args ...string) error {
	cmd := exec.Command(name, args...)
	if !r.IsRoot() {
		cmd = exec.Command("sudo", append([]string{name}, args...)...)
	}
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

func (r PlainRunner) EnsurePrivileges() error {
	if r.IsRoot() {
		return nil
	}
	cmd := exec.Command("sudo", "-v")
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to obtain elevated privileges: %w", err)
	}
	return nil
}

var ErrNotRoot = errors.New("this operation requires administrator privileges")

// Reusable error constructors for license/validation failures exposed to the UI.
func PrivilegeError(op string, err error) error {
	return fmt.Errorf("%s failed. This operation requires administrator privileges: %w", op, err)
}
