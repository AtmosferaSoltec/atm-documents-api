package activity

import (
	"sync"
	"testing"
	"time"
)

func TestSnapshotCountsOutcomesAndBreakdowns(t *testing.T) {
	r := New()
	r.Record(Event{System: "lavendimia", Format: "a4", Outcome: OutcomeOK, DurationMS: 10})
	r.Record(Event{System: "lavendimia", Format: "ticket", Outcome: OutcomeOK, DurationMS: 30})
	r.Record(Event{System: "lavendimia", Format: "a4", Outcome: OutcomeInvalid})
	r.Record(Event{System: "pulso", Format: "a4", Outcome: OutcomeBusy})
	r.Record(Event{System: "pulso", Format: "ticket", Outcome: OutcomeError})

	s := r.Snapshot()
	if s.Totals != (Totals{OK: 2, Invalid: 1, Busy: 1, Error: 1}) {
		t.Fatalf("totals = %+v", s.Totals)
	}
	if s.AvgDurationMS != 20 {
		t.Fatalf("avg = %d, want 20 (only successful renders count)", s.AvgDurationMS)
	}
	if len(s.BySystem) != 2 || s.BySystem[0] != (Breakdown{Name: "lavendimia", OK: 2, Failed: 1}) {
		t.Fatalf("by system = %+v", s.BySystem)
	}
	if len(s.ByFormat) != 2 || s.ByFormat[0].Name != "a4" || s.ByFormat[0].OK != 1 || s.ByFormat[0].Failed != 2 {
		t.Fatalf("by format = %+v", s.ByFormat)
	}
}

func TestRecentIsNewestFirstAndBounded(t *testing.T) {
	r := New()
	for i := 0; i < recentLimit+10; i++ {
		r.Record(Event{System: "lavendimia", Items: i, Outcome: OutcomeOK})
	}
	s := r.Snapshot()
	if len(s.Recent) != recentLimit {
		t.Fatalf("recent = %d, want %d", len(s.Recent), recentLimit)
	}
	if s.Recent[0].Items != recentLimit+9 || s.Recent[recentLimit-1].Items != 10 {
		t.Fatalf("recent order: first %d last %d", s.Recent[0].Items, s.Recent[recentLimit-1].Items)
	}
	if s.Totals.OK != recentLimit+10 {
		t.Fatal("totals must count events that fell out of the recent list")
	}
}

func TestStampsMissingTimeAndNamesUnknowns(t *testing.T) {
	at := time.Date(2026, 10, 30, 12, 0, 0, 0, time.UTC)
	r := newWithClock(func() time.Time { return at })
	r.Record(Event{Outcome: "weird"})
	s := r.Snapshot()
	if !s.Since.Equal(at) || !s.Recent[0].At.Equal(at) {
		t.Fatal("events and start must use the clock")
	}
	if s.Recent[0].Outcome != OutcomeInvalid || s.BySystem[0].Name != "unknown" {
		t.Fatalf("event = %+v, by system = %+v", s.Recent[0], s.BySystem)
	}
}

func TestSnapshotIsACopy(t *testing.T) {
	r := New()
	r.Record(Event{Outcome: OutcomeOK})
	s := r.Snapshot()
	s.Recent[0].Items = 99
	if r.Snapshot().Recent[0].Items == 99 {
		t.Fatal("snapshot shares memory with the recorder")
	}
}

func TestConcurrentRecording(t *testing.T) {
	r := New()
	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			r.Record(Event{System: "lavendimia", Format: "a4", Outcome: OutcomeOK})
			_ = r.Snapshot()
		}()
	}
	wg.Wait()
	if got := r.Snapshot().Totals.OK; got != 50 {
		t.Fatalf("ok = %d, want 50", got)
	}
}
