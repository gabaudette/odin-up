package tui

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"

	"charm.land/bubbles/v2/list"
	"charm.land/bubbles/v2/progress"
	"charm.land/bubbles/v2/spinner"
	tea "charm.land/bubbletea/v2"

	"odin-up/internal/githubclient"
	"odin-up/internal/odin"
	"odin-up/internal/paths"
)

// OpKind identifies the operation an interactive session runs.
type OpKind int

const (
	OpMenu OpKind = iota
	OpInstall
	OpUpdate
	OpStatus
	OpUninstall
	OpExit
)

type screenKind int

const (
	screenMenu screenKind = iota
	screenWork
	screenConfirm
	screenResult
)

// Messages exchanged between the operation goroutine and the model.
type (
	eventMsg          struct{ ev odin.Event }
	runPrivRequestMsg struct {
		argv []string
		ch   chan error
	}
	runPrivDoneMsg struct {
		ch  chan error
		err error
	}
	opDoneMsg struct {
		text string
		err  error
	}
)

type menuItem struct {
	kind  OpKind
	title string
	desc  string
}

func (i menuItem) Title() string       { return i.title }
func (i menuItem) Description() string { return i.desc }
func (i menuItem) FilterValue() string { return i.title }

var menuItems = []menuItem{
	{kind: OpInstall, title: "Install Odin", desc: "Install the latest Odin release"},
	{kind: OpUpdate, title: "Update Odin", desc: "Update to the latest Odin release"},
	{kind: OpStatus, title: "View Status", desc: "Show the current installation status"},
	{kind: OpUninstall, title: "Uninstall Odin", desc: "Remove the odin-up managed installation"},
	{kind: OpExit, title: "Exit", desc: "Leave odin-up"},
}

type model struct {
	op         OpKind
	screen     screenKind
	backToMenu bool
	width      int
	height     int

	list list.Model

	steps        []string
	lastLabel    string
	downloadPct  float64
	showProgress bool
	spinner      spinner.Model
	bar          progress.Model

	confirm      *confirmRequestMsg
	confirmIndex int

	resultText string
	resultErr  error

	prog *tea.Program
}

func newModel(op OpKind) *model {
	m := &model{
		op:      op,
		screen:  screenWork,
		spinner: spinner.New(spinner.WithStyle(spinnerStyle)),
		bar: progress.New(
			progress.WithDefaultBlend(),
			progress.WithWidth(40),
		),
	}
	if op == OpMenu {
		m.screen = screenMenu
		items := make([]list.Item, 0, len(menuItems))

		for _, it := range menuItems {
			items = append(items, it)
		}

		m.list = list.New(items, list.NewDefaultDelegate(), 42, 9)

		m.list.Title = "What would you like to do?"
		m.list.SetShowStatusBar(false)
		m.list.SetShowPagination(false)
		m.list.SetShowHelp(false)
		m.list.DisableQuitKeybindings()
	}
	return m
}

// Run starts an interactive session for the given operation and returns the
// process exit code.
func Run(op OpKind) int {
	m := newModel(op)
	p := tea.NewProgram(m)
	m.prog = p
	mode, err := p.Run()

	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}

	mm, ok := mode.(*model)
	if !ok {
		return 0
	}

	if mm.screen == screenResult {
		if mm.resultText != "" {
			fmt.Println(strings.TrimRight(mm.resultText, "\n"))
		}

		if mm.resultErr != nil {
			fmt.Fprintln(os.Stderr, "Error: "+mm.resultErr.Error())
			return 1
		}
	}

	return 0
}

func (m *model) Init() tea.Cmd {
	batch := []tea.Cmd{m.spinner.Tick}

	if m.op != OpMenu {
		batch = append(batch, m.runOperation())
	}

	return tea.Batch(batch...)
}

// runOperation performs the requested operation in a goroutine, driving the
// effective work through events and request messages.
func (m *model) runOperation() tea.Cmd {
	op := m.op

	return func() tea.Msg {
		prog := m.prog
		runner := &ProgramRunner{Prog: prog}
		inst := &odin.Installer{
			Client:   githubclient.New(),
			Runner:   runner,
			UI:       newUI(prog),
			Reporter: func(ev odin.Event) { prog.Send(eventMsg{ev: ev}) },
		}
		var text string
		var err error
		switch op {
		case OpInstall:
			err = inst.Install(context.Background())

			if err == nil {
				v := odin.CurrentVersion(runner)
				text = fmt.Sprintf("Odin successfully installed.\n\nVersion:\n%s\n\nExecutable:\n%s", v, paths.OdinBinLink)
			}
		case OpUpdate:
			err = inst.Update(context.Background())

			if errors.Is(err, odin.ErrUpToDate) {
				installed := odin.CurrentVersion(runner)
				text = fmt.Sprintf("Odin is already up to date.\n\nInstalled version: %s", installed)
				err = nil
			} else if err == nil {
				v := odin.CurrentVersion(runner)
				text = fmt.Sprintf("Odin successfully updated.\n\nVersion:\n%s\n\nExecutable:\n%s", v, paths.OdinBinLink)
			}
		case OpUninstall:
			err = inst.Uninstall()

			if err == nil {
				text = "Odin has been uninstalled.\n\nThe managed installation under /opt/odin was removed."
			}
		case OpStatus:
			st, serr := inst.Status(context.Background())

			if serr != nil {
				err = serr
			} else {
				text = odin.FormatStatus(st)
			}
		}

		return opDoneMsg{text: text, err: err}
	}
}

func (m *model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd

	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height

		if m.screen == screenMenu {
			m.list.SetSize(minInt(msg.Width, 50), minInt(maxInt(msg.Height-6, 6), 12))
		}
	case tea.KeyMsg:
		switch {
		case m.screen == screenMenu:
			return m.updateMenu(msg)
		case m.screen == screenConfirm:
			return m.updateConfirm(msg)
		case m.screen == screenResult:
			if m.backToMenu {
				m.screen = screenMenu
				m.resetWork()
				return m, nil
			}

			return m, tea.Quit
		case m.screen == screenWork:
			k := msg.String()
			if k == "ctrl+c" || k == "q" || k == "esc" {
				return m, tea.Quit
			}
		}
	case spinner.TickMsg:
		m.spinner, cmd = m.spinner.Update(msg)

		if m.screen != screenWork {
			return m, nil
		}

		return m, cmd
	case eventMsg:
		m.applyEvent(msg.ev)

		return m, nil
	case runPrivRequestMsg:
		m.lastLabel = "Running: " + strings.Join(msg.argv, " ")
		c := exec.Command(msg.argv[0], msg.argv[1:]...)
		return m, tea.ExecProcess(c, func(err error) tea.Msg {
			return runPrivDoneMsg{ch: msg.ch, err: err}
		})

	case runPrivDoneMsg:
		if msg.ch != nil {
			msg.ch <- msg.err
		}
		return m, nil
	case confirmRequestMsg:
		m.confirm = &msg
		m.confirmIndex = 0
		m.screen = screenConfirm

		return m, nil

	case opDoneMsg:
		m.screen = screenResult
		m.resultText = msg.text
		m.resultErr = msg.err

		return m, nil
	}

	return m, nil
}

func (m *model) applyEvent(ev odin.Event) {
	switch ev.Kind {
	case odin.EventStep:
		if m.lastLabel != "" {
			m.steps = append(m.steps, m.lastLabel)

			if len(m.steps) > 6 {
				m.steps = m.steps[len(m.steps)-6:]
			}
		}

		m.lastLabel = ev.Label
		m.showProgress = false
	case odin.EventProgress:
		m.downloadPct = ev.Percent
		m.showProgress = true
	case odin.EventLog:
		m.lastLabel = ev.Label
	}
}

func (m *model) updateMenu(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	nl, cmd := m.list.Update(msg)
	m.list = nl

	switch msg.String() {
	case "q", "esc", "ctrl+c":
		return m, tea.Quit
	case "enter", "ctrl+j":
		sel, ok := m.list.SelectedItem().(menuItem)

		if !ok {
			return m, cmd
		}

		if sel.kind == OpExit {
			return m, tea.Quit
		}

		m.op = sel.kind
		m.backToMenu = true
		m.resetWork()
		m.screen = screenWork

		return m, m.runOperation()
	}
	return m, cmd
}

func (m *model) updateConfirm(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "up", "k":
		if m.confirmIndex > 0 {
			m.confirmIndex--
		}
	case "down", "j", "tab":
		if m.confirmIndex < len(m.confirm.options)-1 {
			m.confirmIndex++
		}
	case "enter":
		if m.confirm != nil {
			m.confirm.ch <- m.confirmIndex
			m.confirm = nil
		}
		m.screen = screenWork
	case "esc", "q", "ctrl+c":
		if m.confirm != nil {
			m.confirm.ch <- 0
			m.confirm = nil
		}
		m.screen = screenWork
	}
	return m, nil
}

func (m *model) resetWork() {
	m.steps = nil
	m.lastLabel = ""
	m.downloadPct = 0
	m.showProgress = false
	m.resultText = ""
	m.resultErr = nil
}

func (m *model) View() tea.View {
	v := tea.NewView(m.render())
	v.AltScreen = true

	return v
}

func minInt(a, b int) int {
	if a < b {
		return a
	}

	return b
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}

	return b
}
