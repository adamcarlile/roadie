package syncer

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestRunCopiesJobsAndEmitsEvents(t *testing.T) {
	root := t.TempDir()
	src := filepath.Join(root, "src")
	dst := filepath.Join(root, "dst")
	// A foldered film and a bare film.
	if err := os.MkdirAll(filepath.Join(src, "Cars (2006)"), 0o755); err != nil {
		t.Fatal(err)
	}
	write(t, filepath.Join(src, "Cars (2006)", "Cars (2006).mkv"), "movie-data")
	write(t, filepath.Join(src, "Up (2009).mkv"), "bare-data")
	if err := os.MkdirAll(dst, 0o755); err != nil {
		t.Fatal(err)
	}

	jobs := []Job{
		{Collection: "movies", Path: "Cars (2006)", Source: src, Dest: dst},
		{Collection: "movies", Path: "Up (2009).mkv", Source: src, Dest: dst},
	}
	var events []Event
	r := NewRunner()
	summary := r.Run(context.Background(), jobs, func(e Event) { events = append(events, e) })

	if summary.Copied != 2 || summary.Failed != 0 {
		t.Fatalf("summary = %+v, want 2 copied", summary)
	}
	if _, err := os.Stat(filepath.Join(dst, "Cars (2006)", "Cars (2006).mkv")); err != nil {
		t.Errorf("foldered film not copied: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dst, "Up (2009).mkv")); err != nil {
		t.Errorf("bare film not copied: %v", err)
	}
	if events[0].Type != EvRunStart || events[len(events)-1].Type != EvRunDone {
		t.Errorf("events not bracketed by run-start/run-done: %+v", events)
	}
}

func TestRunContinuesPastFailedJob(t *testing.T) {
	root := t.TempDir()
	src := filepath.Join(root, "src")
	dst := filepath.Join(root, "dst")
	for _, d := range []string{src, dst} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	write(t, filepath.Join(src, "Good (2020).mkv"), "ok")

	jobs := []Job{
		// Source file does not exist — rsync exits non-zero, job fails.
		{Collection: "movies", Path: "Missing (1999).mkv", Source: src, Dest: dst},
		{Collection: "movies", Path: "Good (2020).mkv", Source: src, Dest: dst},
	}
	var events []Event
	summary := NewRunner().Run(context.Background(), jobs, func(e Event) { events = append(events, e) })

	if summary.Failed != 1 || summary.Copied != 1 {
		t.Fatalf("summary = %+v, want 1 failed + 1 copied", summary)
	}
	if _, err := os.Stat(filepath.Join(dst, "Good (2020).mkv")); err != nil {
		t.Errorf("run did not continue past the failed job: %v", err)
	}
	var sawFailure bool
	for _, e := range events {
		if e.Type == EvEntryDone && e.Path == "Missing (1999).mkv" && e.Err != "" {
			sawFailure = true
		}
	}
	if !sawFailure {
		t.Error("expected an entry-done event with a non-empty Err for the missing source")
	}
}

func write(t *testing.T, p, body string) {
	t.Helper()
	if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}
