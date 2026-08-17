package tui

import (
	"errors"
	"strings"

	"odin-up/internal/odin"
)

func (m *model) opTitle() string {
	switch m.op {
	case OpInstall:
		return "Installing Odin"
	case OpUpdate:
		return "Updating Odin"
	case OpUninstall:
		return "Uninstalling Odin"
	case OpStatus:
		return "Odin status"
	}
	return "odin-up"
}

func (m *model) render() string {
	switch m.screen {
	case screenMenu:
		return m.renderMenu()
	case screenConfirm:
		return m.renderConfirm()
	case screenResult:
		return m.renderResult()
	default:
		return m.renderWork()
	}
}

func (m *model) renderMenu() string {
	var b strings.Builder
	b.WriteString(titleStyle.Render("odin-up") + "\n")
	b.WriteString(subtitleStyle.Render("Odin compiler manager") + "\n\n")
	b.WriteString(m.list.View() + "\n")
	b.WriteString(helpStyle.Render("Use arrow keys to select, Enter to confirm, q/esc to quit"))
	return b.String()
}

func (m *model) renderWork() string {
	var b strings.Builder
	b.WriteString(titleStyle.Render(m.opTitle()) + "\n\n")
	for _, s := range m.steps {
		b.WriteString("  " + dimStyle.Render(s) + "\n")
	}
	if m.showProgress {
		b.WriteString("  " + infoStyle.Render(m.lastLabel) + "\n")
		b.WriteString("  " + m.bar.ViewAs(m.downloadPct) + "\n\n")
		b.WriteString(helpStyle.Render("Press q to cancel"))
		return b.String()
	}
	if m.lastLabel != "" {
		b.WriteString("  " + m.spinner.View() + " " + infoStyle.Render(m.lastLabel) + "\n")
	}
	return b.String()
}

func (m *model) renderConfirm() string {
	var b strings.Builder
	b.WriteString(titleStyle.Render("Confirm") + "\n\n")
	for _, line := range strings.Split(m.confirm.question, "\n") {
		b.WriteString(line + "\n")
	}
	b.WriteString("\n")
	for i, opt := range m.confirm.options {
		if i == m.confirmIndex {
			b.WriteString("  " + infoStyle.Render("> "+opt) + "\n")
		} else {
			b.WriteString("  " + dimStyle.Render("  "+opt) + "\n")
		}
	}
	b.WriteString("\n" + helpStyle.Render("Use arrow keys, j/k then Enter. Esc cancels."))
	return b.String()
}

func (m *model) renderResult() string {
	var b strings.Builder
	if m.resultErr != nil {
		b.WriteString(errorStyle.Render("Operation failed") + "\n\n")
		b.WriteString(m.resultErr.Error() + "\n")
		switch {
		case errors.Is(m.resultErr, odin.ErrAlreadyInstalled):
			b.WriteString("\nRun 'odin-up update' to update it.\n")
		case errors.Is(m.resultErr, odin.ErrNotInstalled):
			b.WriteString("\nRun 'odin-up install' to install Odin.\n")
		case errors.Is(m.resultErr, odin.ErrCanceled):
			b.WriteString("\nNo changes were made.\n")
		case errors.Is(m.resultErr, odin.ErrUpToDate):
			b.WriteString("\nNo changes were made.\n")
		}
	} else {
		b.WriteString(resultTitleStyle.Render("Done") + "\n\n")
		b.WriteString(m.resultText + "\n")
	}
	if m.backToMenu {
		b.WriteString("\n" + helpStyle.Render("Press any key to return to the menu"))
	} else {
		b.WriteString("\n" + helpStyle.Render("Press any key to exit"))
	}
	return b.String()
}
