package odin

// EventKind describes the type of an operation event.
type EventKind int

const (
	// EventStep marks the start of a discrete operation phase.
	EventStep EventKind = iota
	// EventProgress reports download progress as a fraction in [0, 1].
	EventProgress
	// EventLog reports a free-form status message.
	EventLog
)

// Event is emitted by the installer so that any front end (TUI, CLI) can
// render progress without owning the installation logic.
type Event struct {
	Kind    EventKind
	Label   string
	Percent float64
}

// Reporter receives install/update events.
type Reporter func(Event)

func (r Reporter) step(label string) {
	if r == nil {
		return
	}
	r(Event{Kind: EventStep, Label: label})
}

func (r Reporter) log(label string) {
	if r == nil {
		return
	}
	r(Event{Kind: EventLog, Label: label})
}

func (r Reporter) progress(p float64) {
	if r == nil {
		return
	}
	r(Event{Kind: EventProgress, Percent: p})
}
