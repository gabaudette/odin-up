package tui

import (
	"io"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"odin-up/internal/odin"
)

func headless(t *testing.T, m *model, input string) *model {
	t.Helper()
	p := tea.NewProgram(m, tea.WithInput(strings.NewReader(input)), tea.WithOutput(io.Discard), tea.WithoutRenderer())
	m.prog = p
	got, err := p.Run()
	if err != nil {
		t.Fatalf("program error: %v", err)
	}
	mm, ok := got.(*model)
	if !ok {
		t.Fatalf("unexpected model type %T", got)
	}
	return mm
}

func TestMenuQuitsOnQ(t *testing.T) {
	mm := headless(t, newModel(OpMenu), "q")
	if mm.screen != screenMenu {
		t.Fatalf("expected menu screen, got %d", mm.screen)
	}
}

func TestMenuExitItemQuits(t *testing.T) {
	// Move to the last item (Exit) and press enter.
	mm := headless(t, newModel(OpMenu), "jjjj\n")
	if mm.screen != screenMenu {
		t.Fatalf("expected menu screen, got %d", mm.screen)
	}
}

func TestMenuEnterSelectsFirstItem(t *testing.T) {
	m := newModel(OpMenu)
	m.updateMenu(tea.KeyPressMsg{Code: tea.KeyEnter})
	if m.op != OpInstall {
		t.Fatalf("expected install operation selected, got %d", m.op)
	}
	if m.screen != screenWork {
		t.Fatalf("expected work screen, got %d", m.screen)
	}
}

func TestResultReturnsToMenu(t *testing.T) {
	m := newModel(OpMenu)
	m.screen = screenResult
	m.backToMenu = true
	// enter returns to the menu, then q quits it.
	mm := headless(t, m, "\nq")
	if mm.screen != screenMenu {
		t.Fatalf("expected return to menu, got %d", mm.screen)
	}
}

func TestConfirmNavigation(t *testing.T) {
	m := newModel(OpInstall)
	m.screen = screenConfirm
	ch := make(chan int, 1)
	m.confirm = &confirmRequestMsg{question: "q?", options: []string{"Cancel", "Proceed"}, ch: ch}
	// select the second option and confirm
	m.updateConfirm(tea.KeyPressMsg{Code: 'j', Text: "j"})
	m.updateConfirm(tea.KeyPressMsg{Code: tea.KeyEnter})
	got := <-ch
	if got != 1 {
		t.Fatalf("expected second option selected, got %d", got)
	}
	if m.screen != screenWork {
		t.Fatalf("expected work screen after confirm, got %d", m.screen)
	}
}

func TestEventAccumulatesSteps(t *testing.T) {
	m := newModel(OpInstall)
	m.applyEvent(odinEventStep("Checking dependencies"))
	m.applyEvent(odinEventStep("Fetching release"))
	if len(m.steps) != 1 || m.lastLabel != "Fetching release" {
		t.Fatalf("unexpected steps=%v last=%q", m.steps, m.lastLabel)
	}
	if m.steps[0] != "Checking dependencies" {
		t.Fatalf("unexpected completed step %q", m.steps[0])
	}
}
func odinEventStep(label string) odin.Event {
	return odin.Event{Kind: odin.EventStep, Label: label}
}
