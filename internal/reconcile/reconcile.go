// Package reconcile is the pure logic that diffs the manifest against disk.
package reconcile

import (
	"strings"

	"roadie/internal/manifest"
)

// Status is the computed sync state of a manifest entry.
type Status string

const (
	Synced  Status = "synced"
	Pending Status = "pending"
	Blocked Status = "blocked"
)

// EntryStatus pairs a manifest entry with its computed status.
type EntryStatus struct {
	Entry   manifest.Entry `json:"entry"`
	Status  Status         `json:"status"`
	Overlap bool           `json:"overlap"` // true when another entry in the same collection is an ancestor of this path
}

// DriftItem is a top-level dest object not covered by any manifest entry.
type DriftItem struct {
	Collection string `json:"collection"`
	Path       string `json:"path"`
}

// Plan is the result of reconciling the manifest against disk.
type Plan struct {
	Entries []EntryStatus `json:"entries"`
	Drift   []DriftItem   `json:"drift"`
}

// DiskState is the disk information Reconcile needs. The caller (IO layer)
// supplies it so that Reconcile itself stays pure and testable.
type DiskState struct {
	// SourceAvailable maps collection id → whether its NFS source is reachable.
	SourceAvailable map[string]bool
	// DestPresent maps key(collection,path) → whether that path exists on dest.
	DestPresent map[string]bool
	// DestTopLevel maps collection id → top-level object names on the dest.
	DestTopLevel map[string][]string
}

// key builds the DestPresent map key for a (collection, path) pair.
func key(collection, path string) string { return collection + "\x00" + path }

// covers reports whether path a is equal to, or an ancestor directory of, b.
func covers(a, b string) bool {
	return a == b || strings.HasPrefix(b, a+"/")
}

// related reports whether two paths are on the same branch (either covers the other).
func related(a, b string) bool { return covers(a, b) || covers(b, a) }

// Reconcile classifies each manifest entry and finds drift. It is pure.
func Reconcile(entries []manifest.Entry, disk DiskState) Plan {
	// Entries and Drift are non-nil so the JSON layer emits [] rather than null.
	plan := Plan{
		Entries: make([]EntryStatus, 0, len(entries)),
		Drift:   []DriftItem{},
	}
	for _, e := range entries {
		st := EntryStatus{Entry: e}
		switch {
		case !disk.SourceAvailable[e.Collection]:
			st.Status = Blocked
		case disk.DestPresent[key(e.Collection, e.Path)]:
			st.Status = Synced
		default:
			st.Status = Pending
		}
		for _, other := range entries {
			if other.Collection == e.Collection && other.Path != e.Path &&
				covers(other.Path, e.Path) {
				st.Overlap = true
				break
			}
		}
		plan.Entries = append(plan.Entries, st)
	}
	for col, objs := range disk.DestTopLevel {
		for _, obj := range objs {
			// A top-level dest object is drift unless some manifest entry is
			// related to it. related(entry, obj) holds whether the entry equals
			// obj, is an ancestor of obj (a whole-show entry), or is a
			// descendant of obj (a season entry whose show dir is obj) — all
			// three mean the object is wanted.
			drift := true
			for _, e := range entries {
				if e.Collection == col && related(e.Path, obj) {
					drift = false
					break
				}
			}
			if drift {
				plan.Drift = append(plan.Drift, DriftItem{Collection: col, Path: obj})
			}
		}
	}
	return plan
}
