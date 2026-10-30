// Package activity keeps what the service rendered since it started. It is
// memory only: the service stores nothing, so the counters reset on every
// restart and deploy. It records metadata, never the content of a document.
package activity

import (
	"sort"
	"sync"
	"time"
)

// Outcome is how a render request ended.
type Outcome string

const (
	// OutcomeOK means a PDF was returned.
	OutcomeOK Outcome = "ok"
	// OutcomeInvalid means the request was rejected (bad JSON or failed validation).
	OutcomeInvalid Outcome = "invalid"
	// OutcomeBusy means no render slot freed up in time.
	OutcomeBusy Outcome = "busy"
	// OutcomeError means rendering failed unexpectedly.
	OutcomeError Outcome = "error"
)

// recentLimit is how many recent events are kept.
const recentLimit = 50

// Event is one render request.
type Event struct {
	At         time.Time `json:"at"`
	System     string    `json:"system"`
	Template   string    `json:"template"`
	Format     string    `json:"format"`
	Items      int       `json:"items"`
	Bytes      int       `json:"bytes"`
	DurationMS int64     `json:"duration_ms"`
	Outcome    Outcome   `json:"outcome"`
}

// Totals counts events by outcome.
type Totals struct {
	OK      int `json:"ok"`
	Invalid int `json:"invalid"`
	Busy    int `json:"busy"`
	Error   int `json:"error"`
}

// Breakdown counts one system or format.
type Breakdown struct {
	Name   string `json:"name"`
	OK     int    `json:"ok"`
	Failed int    `json:"failed"`
}

// Snapshot is a copy of the recorded activity.
type Snapshot struct {
	Since         time.Time   `json:"since"`
	Totals        Totals      `json:"totals"`
	AvgDurationMS int64       `json:"avg_duration_ms"`
	BySystem      []Breakdown `json:"by_system"`
	ByFormat      []Breakdown `json:"by_format"`
	// Recent lists the latest events, newest first.
	Recent []Event `json:"recent"`
}

// Recorder is safe for concurrent use.
type Recorder struct {
	mu         sync.Mutex
	now        func() time.Time
	since      time.Time
	totals     Totals
	okDuration int64
	bySystem   map[string]*Breakdown
	byFormat   map[string]*Breakdown
	recent     []Event
}

// New returns an empty Recorder that starts counting now.
func New() *Recorder { return newWithClock(time.Now) }

func newWithClock(now func() time.Time) *Recorder {
	return &Recorder{
		now:      now,
		since:    now(),
		bySystem: map[string]*Breakdown{},
		byFormat: map[string]*Breakdown{},
	}
}

// Record adds an event. When At is zero it is stamped with the current time.
func (r *Recorder) Record(e Event) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if e.At.IsZero() {
		e.At = r.now()
	}
	switch e.Outcome {
	case OutcomeOK:
		r.totals.OK++
		r.okDuration += e.DurationMS
	case OutcomeBusy:
		r.totals.Busy++
	case OutcomeError:
		r.totals.Error++
	default:
		e.Outcome = OutcomeInvalid
		r.totals.Invalid++
	}
	count(r.bySystem, e.System, e.Outcome)
	count(r.byFormat, e.Format, e.Outcome)

	r.recent = append(r.recent, e)
	if len(r.recent) > recentLimit {
		r.recent = r.recent[len(r.recent)-recentLimit:]
	}
}

func count(groups map[string]*Breakdown, name string, outcome Outcome) {
	if name == "" {
		name = "unknown"
	}
	group := groups[name]
	if group == nil {
		group = &Breakdown{Name: name}
		groups[name] = group
	}
	if outcome == OutcomeOK {
		group.OK++
	} else {
		group.Failed++
	}
}

// Snapshot returns a copy; the caller may keep it.
func (r *Recorder) Snapshot() Snapshot {
	r.mu.Lock()
	defer r.mu.Unlock()
	snapshot := Snapshot{
		Since:    r.since,
		Totals:   r.totals,
		BySystem: flatten(r.bySystem),
		ByFormat: flatten(r.byFormat),
		Recent:   make([]Event, len(r.recent)),
	}
	if r.totals.OK > 0 {
		snapshot.AvgDurationMS = r.okDuration / int64(r.totals.OK)
	}
	for i, e := range r.recent {
		snapshot.Recent[len(r.recent)-1-i] = e
	}
	return snapshot
}

func flatten(groups map[string]*Breakdown) []Breakdown {
	list := make([]Breakdown, 0, len(groups))
	for _, group := range groups {
		list = append(list, *group)
	}
	sort.Slice(list, func(i, j int) bool {
		if total := list[i].OK + list[i].Failed; total != list[j].OK+list[j].Failed {
			return total > list[j].OK+list[j].Failed
		}
		return list[i].Name < list[j].Name
	})
	return list
}
