package tui

import (
	"strings"

	tea "charm.land/bubbletea/v2"

	"odin-up/internal/odin"
	"odin-up/internal/system"
)

// ui implements odin.UI by prompting the user through the running program.
type ui struct {
	prog *tea.Program
}

func newUI(prog *tea.Program) odin.UI {
	return &ui{prog: prog}
}

func (u *ui) confirm(question string, options []string) (int, error) {
	ch := make(chan int, 1)
	u.prog.Send(confirmRequestMsg{question: question, options: options, ch: ch})
	return <-ch, nil
}

func (u *ui) ConfirmInstallDeps(missing []system.Dependency) (bool, error) {
	var names []string
	for _, d := range missing {
		names = append(names, d.Package)
	}
	question := "Missing required dependencies:\n\n  " + strings.Join(names, "\n  ") +
		"\n\nInstall them with apt before continuing?"
	idx, err := u.confirm(question, []string{"Cancel", "Install dependencies"})
	if err != nil {
		return false, err
	}
	return idx == 1, nil
}

func (u *ui) ConfirmUninstall() (bool, error) {
	question := "Uninstall Odin?\n\nThis will remove:\n\n  /opt/odin\n  /usr/local/bin/odin\n\nThis action cannot be undone."
	idx, err := u.confirm(question, []string{"Cancel", "Uninstall"})
	if err != nil {
		return false, err
	}
	return idx == 1, nil
}

// confirmRequestMsg is sent from the operation goroutine to ask the model to
// show a selection prompt and return the chosen index.
type confirmRequestMsg struct {
	question string
	options  []string
	ch       chan int
}
