package tui

import (
	"os"
	"os/exec"
	"strings"

	tea "charm.land/bubbletea/v2"
)

// ProgramRunner implements system.Runner on top of a running Bubble Tea
// program. Privileged commands are executed through tea.ExecProcess, which
// hands the terminal to the subprocess so sudo can prompt and apt can print
// its progress, then resumes the TUI.
type ProgramRunner struct {
	Prog *tea.Program
}

func (r *ProgramRunner) IsRoot() bool {
	return os.Geteuid() == 0
}

// Output runs a command directly and returns its combined output. Non-nil
// errors carry the output.
func (r *ProgramRunner) Output(name string, args ...string) (string, error) {
	out, err := exec.Command(name, args...).CombinedOutput()
	if err != nil {
		if len(out) > 0 {
			return string(out), &ExecError{Err: err, Output: string(out)}
		}
		return "", err
	}
	return string(out), nil
}

// RunPrivileged asks the model to run a command with sudo (when not root) via
// ExecProcess and waits for the result.
func (r *ProgramRunner) RunPrivileged(name string, args ...string) error {
	argv := append([]string{name}, args...)
	if !r.IsRoot() {
		argv = append([]string{"sudo"}, argv...)
	}
	ch := make(chan error, 1)
	r.Prog.Send(runPrivRequestMsg{argv: argv, ch: ch})
	return <-ch
}

// EnsurePrivileges validates sudo access without running anything else.
func (r *ProgramRunner) EnsurePrivileges() error {
	if r.IsRoot() {
		return nil
	}
	ch := make(chan error, 1)
	r.Prog.Send(runPrivRequestMsg{argv: []string{"sudo", "-v"}, ch: ch})
	return <-ch
}

// ExecError wraps an OS command failure with its output.
type ExecError struct {
	Err    error
	Output string
}

func (e *ExecError) Error() string {
	out := strings.TrimSpace(e.Output)
	if out == "" {
		return e.Err.Error()
	}
	return e.Err.Error() + ": " + out
}

func (e *ExecError) Unwrap() error { return e.Err }
