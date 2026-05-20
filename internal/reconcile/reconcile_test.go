package reconcile

import (
	"testing"

	"roadie/internal/manifest"
)

func TestReconcileClassifiesEntries(t *testing.T) {
	entries := []manifest.Entry{
		{Collection: "movies", Path: "Up (2009).mkv"}, // synced
		{Collection: "movies", Path: "Cars (2006)"},   // pending
		{Collection: "kids-tv", Path: "Bluey (2018)"}, // blocked (source down)
	}
	disk := DiskState{
		SourceAvailable: map[string]bool{"movies": true, "kids-tv": false},
		DestPresent: map[string]bool{
			key("movies", "Up (2009).mkv"): true,
		},
	}
	plan := Reconcile(entries, disk)
	want := map[string]Status{
		"Up (2009).mkv": Synced,
		"Cars (2006)":   Pending,
		"Bluey (2018)":  Blocked,
	}
	for _, es := range plan.Entries {
		if es.Status != want[es.Entry.Path] {
			t.Errorf("%s: status %s, want %s", es.Entry.Path, es.Status, want[es.Entry.Path])
		}
	}
}

func TestReconcileFlagsOverlap(t *testing.T) {
	entries := []manifest.Entry{
		{Collection: "kids-tv", Path: "Bluey (2018)"},
		{Collection: "kids-tv", Path: "Bluey (2018)/Series 1"},
	}
	disk := DiskState{SourceAvailable: map[string]bool{"kids-tv": true}}
	plan := Reconcile(entries, disk)
	for _, es := range plan.Entries {
		switch es.Entry.Path {
		case "Bluey (2018)/Series 1":
			if !es.Overlap {
				t.Error("season entry should be flagged as overlapping the whole-show entry")
			}
		case "Bluey (2018)":
			if es.Overlap {
				t.Error("whole-show entry should NOT be flagged as overlapping")
			}
		}
	}
}

func TestReconcileDetectsDrift(t *testing.T) {
	entries := []manifest.Entry{
		{Collection: "movies", Path: "Up (2009).mkv"},
	}
	disk := DiskState{
		SourceAvailable: map[string]bool{"movies": true},
		DestPresent:     map[string]bool{key("movies", "Up (2009).mkv"): true},
		DestTopLevel: map[string][]string{
			"movies": {"Up (2009).mkv", "Frozen (2013).mkv"},
		},
	}
	plan := Reconcile(entries, disk)
	// Drift order across collections is not guaranteed (map iteration); this
	// fixture has a single collection, so Drift[0] is deterministic here.
	if len(plan.Drift) != 1 || plan.Drift[0].Path != "Frozen (2013).mkv" {
		t.Fatalf("drift = %+v, want only Frozen (2013).mkv", plan.Drift)
	}
}

func TestReconcileEmptyManifest(t *testing.T) {
	disk := DiskState{
		SourceAvailable: map[string]bool{"movies": true},
		DestTopLevel:    map[string][]string{"movies": {"Old (2001).mkv"}},
	}
	plan := Reconcile(nil, disk)
	if len(plan.Entries) != 0 {
		t.Fatalf("entries = %+v, want none", plan.Entries)
	}
	if len(plan.Drift) != 1 || plan.Drift[0].Path != "Old (2001).mkv" {
		t.Fatalf("drift = %+v, want the one dest object", plan.Drift)
	}
}
