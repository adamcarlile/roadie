package syncer

// EventType identifies a sync-run event.
type EventType string

const (
	EvRunStart      EventType = "run-start"
	EvEntryStart    EventType = "entry-start"
	EvEntryProgress EventType = "entry-progress"
	EvEntryDone     EventType = "entry-done"
	EvRunDone       EventType = "run-done"
)

// EvJob names one job in a run by collection and path. The run-start event
// carries the full list so the UI can show every entry before it starts.
// It is the public projection of Job, omitting the server-internal Source/Dest.
type EvJob struct {
	Collection string `json:"collection"`
	Path       string `json:"path"`
}

// Event is a single update emitted during a sync run.
type Event struct {
	Type         EventType `json:"type"`
	Collection   string    `json:"collection,omitempty"`
	Path         string    `json:"path,omitempty"`
	Percent      int       `json:"percent"` // always serialised (no omitempty) — the UI reads it as a number
	Rate         string    `json:"rate,omitempty"`
	ETA          string    `json:"eta,omitempty"`
	TotalEntries int       `json:"totalEntries,omitempty"`
	DoneEntries  int       `json:"doneEntries,omitempty"`
	Copied       int       `json:"copied,omitempty"`
	Failed       int       `json:"failed,omitempty"`
	Err          string    `json:"err,omitempty"`
	Jobs         []EvJob   `json:"jobs,omitempty"` // populated on run-start only
}

// Job is one media object to copy: relative path under a collection's
// source, into the matching dest.
type Job struct {
	Collection string
	Path       string
	Source     string // absolute collection source root
	Dest       string // absolute collection dest root
}
