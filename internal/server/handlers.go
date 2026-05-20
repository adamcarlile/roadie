package server

import (
	"context"
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"

	"roadie/internal/manifest"
	"roadie/internal/reconcile"
	"roadie/internal/scan"
	"roadie/internal/syncer"
)

// writeJSON sends v as a JSON response.
func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}

func (s *Server) handleCollections(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, s.cfg.Collections)
}

// handleBrowse lists pickable objects in a collection, optionally under a
// relative path (used to drill into tv shows).
func (s *Server) handleBrowse(w http.ResponseWriter, r *http.Request) {
	col, ok := s.cfg.Collection(r.PathValue("id"))
	if !ok {
		http.Error(w, "unknown collection", http.StatusNotFound)
		return
	}
	dir := col.Source
	if rel := r.URL.Query().Get("path"); rel != "" {
		dir = filepath.Join(col.Source, filepath.Clean("/"+rel))
	}
	objs, err := scan.Browse(dir)
	if err != nil {
		http.Error(w, "source unavailable", http.StatusServiceUnavailable)
		return
	}
	writeJSON(w, http.StatusOK, objs)
}

// manifestView is the GET /api/manifest response.
type manifestView struct {
	Plan      reconcile.Plan `json:"plan"`
	PendingMB int64          `json:"pendingMB"`
	FreeMB    int64          `json:"freeMB"`
}

func (s *Server) handleManifest(w http.ResponseWriter, r *http.Request) {
	entries := s.store.Snapshot().Entries
	disk := s.buildDiskState(entries)
	plan := reconcile.Reconcile(entries, disk)

	var pending int64
	for _, es := range plan.Entries {
		if es.Status != reconcile.Pending {
			continue
		}
		col, _ := s.cfg.Collection(es.Entry.Collection)
		if sz, err := scan.PathSize(filepath.Join(col.Source, es.Entry.Path)); err == nil {
			pending += sz
		}
	}
	// All collection dests live on the same physical drive (the carnet's exfat
	// disk), so the first collection's dest is a representative free-space probe.
	var free int64
	if len(s.cfg.Collections) > 0 {
		if f, err := syncer.FreeBytes(s.cfg.Collections[0].Dest); err == nil {
			free = f
		}
	}
	writeJSON(w, http.StatusOK, manifestView{
		Plan: plan, PendingMB: pending / (1 << 20), FreeMB: free / (1 << 20),
	})
}

// buildDiskState gathers the disk facts reconcile needs.
func (s *Server) buildDiskState(entries []manifest.Entry) reconcile.DiskState {
	ds := reconcile.DiskState{
		SourceAvailable: map[string]bool{},
		DestPresent:     map[string]bool{},
		DestTopLevel:    map[string][]string{},
	}
	for _, col := range s.cfg.Collections {
		_, srcErr := os.Stat(col.Source)
		ds.SourceAvailable[col.ID] = srcErr == nil
		if objs, err := scan.Browse(col.Dest); err == nil {
			for _, o := range objs {
				ds.DestTopLevel[col.ID] = append(ds.DestTopLevel[col.ID], o.Name)
			}
		}
	}
	for _, e := range entries {
		col, ok := s.cfg.Collection(e.Collection)
		if !ok {
			continue
		}
		_, err := os.Stat(filepath.Join(col.Dest, e.Path))
		ds.DestPresent[e.Collection+"\x00"+e.Path] = err == nil
	}
	return ds
}

type entryReq struct {
	Collection string `json:"collection"`
	Path       string `json:"path"`
}

func (s *Server) handleAddEntry(w http.ResponseWriter, r *http.Request) {
	var req entryReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Collection == "" || req.Path == "" {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	if _, ok := s.cfg.Collection(req.Collection); !ok {
		http.Error(w, "unknown collection", http.StatusNotFound)
		return
	}
	if err := s.store.Add(req.Collection, req.Path); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (s *Server) handleRemoveEntry(w http.ResponseWriter, r *http.Request) {
	col := r.URL.Query().Get("collection")
	path := r.URL.Query().Get("path")
	if col == "" || path == "" {
		http.Error(w, "collection and path required", http.StatusBadRequest)
		return
	}
	if err := s.store.Remove(col, path); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// handleSync starts a sync run if one is not already in progress. The disk
// scan, reconcile, and rsync transfers all run in the background goroutine, so
// the request returns 202 immediately and progress arrives over the SSE stream.
func (s *Server) handleSync(w http.ResponseWriter, r *http.Request) {
	if !s.tryClaimRun() {
		http.Error(w, "a sync is already running", http.StatusConflict)
		return
	}
	go func() {
		defer s.releaseRun()
		entries := s.store.Snapshot().Entries
		plan := reconcile.Reconcile(entries, s.buildDiskState(entries))
		var jobs []syncer.Job
		for _, es := range plan.Entries {
			if es.Status != reconcile.Pending {
				continue
			}
			col, _ := s.cfg.Collection(es.Entry.Collection)
			jobs = append(jobs, syncer.Job{
				Collection: es.Entry.Collection, Path: es.Entry.Path,
				Source: col.Source, Dest: col.Dest,
			})
		}
		s.runner.Run(context.Background(), jobs, s.hub.publish)
	}()
	writeJSON(w, http.StatusAccepted, map[string]string{"status": "started"})
}

// handlePrune deletes a drifted object from a collection's dest. It refuses
// to delete anything a manifest entry still covers — only reconcile-detected
// drift may be removed.
func (s *Server) handlePrune(w http.ResponseWriter, r *http.Request) {
	var req entryReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Collection == "" || req.Path == "" {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	col, ok := s.cfg.Collection(req.Collection)
	if !ok {
		http.Error(w, "unknown collection", http.StatusNotFound)
		return
	}
	entries := s.store.Snapshot().Entries
	plan := reconcile.Reconcile(entries, s.buildDiskState(entries))
	isDrift := false
	for _, d := range plan.Drift {
		if d.Collection == req.Collection && d.Path == req.Path {
			isDrift = true
			break
		}
	}
	if !isDrift {
		http.Error(w, "path is not drift — refusing to delete", http.StatusConflict)
		return
	}
	rel := filepath.Clean("/" + req.Path)
	if rel == "/" {
		// req.Path was ".", "..", "/" or similar — the target would be
		// col.Dest itself. Refuse: this is the only deletion path in roadie.
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	target := filepath.Join(col.Dest, rel)
	if err := os.RemoveAll(target); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "removed"})
}

// handleStream serves sync events as Server-Sent Events, first replaying the
// current run's snapshot so a mid-run page load resumes cleanly.
func (s *Server) handleStream(w http.ResponseWriter, r *http.Request) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming unsupported", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	ch, snap := s.hub.subscribe()
	defer s.hub.unsubscribe(ch)
	enc := json.NewEncoder(w)
	send := func(e syncer.Event) bool {
		if _, err := w.Write([]byte("data: ")); err != nil {
			return false
		}
		if err := enc.Encode(e); err != nil { // Encode writes a trailing newline
			return false
		}
		if _, err := w.Write([]byte("\n")); err != nil { // blank line ends the SSE event
			return false
		}
		flusher.Flush()
		return true
	}
	for _, e := range snap {
		if !send(e) {
			return
		}
	}
	for {
		select {
		case <-r.Context().Done():
			return
		case e, ok := <-ch:
			if !ok || !send(e) {
				return
			}
		}
	}
}

// handleStatus reports whether a sync is running, with the current run's events.
func (s *Server) handleStatus(w http.ResponseWriter, r *http.Request) {
	// Read running and the event snapshot under one lock hold so the response
	// is internally consistent — running cannot flip between the two reads.
	s.mu.Lock()
	running := s.running
	events := s.hub.snapshot()
	s.mu.Unlock()
	writeJSON(w, http.StatusOK, map[string]any{
		"running": running,
		"events":  events,
	})
}
