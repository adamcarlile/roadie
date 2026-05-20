// Package server exposes the roadie JSON API and embedded web UI.
package server

import (
	"net/http"
	"sync"

	"roadie/internal/config"
	"roadie/internal/manifest"
	"roadie/internal/syncer"
)

// Server holds everything the HTTP handlers need.
type Server struct {
	cfg    *config.Config
	store  *manifest.Store
	runner *syncer.Runner
	hub    *hub
	web    http.Handler // embedded UI file server

	mu      sync.Mutex // guards running
	running bool

	// destReady reports whether a collection's destination drive is mounted.
	// It is a field so tests can substitute a stub for the real filesystem.
	destReady func(dest string) bool
}

// New builds a Server. web serves the embedded UI assets.
func New(cfg *config.Config, store *manifest.Store, web http.Handler) *Server {
	return &Server{
		cfg:       cfg,
		store:     store,
		runner:    syncer.NewRunner(),
		hub:       newHub(),
		web:       web,
		destReady: syncer.DriveMounted,
	}
}

// Handler returns the HTTP handler with all routes registered.
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/collections", s.handleCollections)
	mux.HandleFunc("GET /api/collections/{id}/browse", s.handleBrowse)
	mux.HandleFunc("GET /api/manifest", s.handleManifest)
	mux.HandleFunc("POST /api/manifest/entries", s.handleAddEntry)
	mux.HandleFunc("DELETE /api/manifest/entries", s.handleRemoveEntry)
	mux.HandleFunc("POST /api/sync", s.handleSync)
	mux.HandleFunc("GET /api/sync/stream", s.handleStream)
	mux.HandleFunc("GET /api/sync/status", s.handleStatus)
	mux.HandleFunc("POST /api/prune", s.handlePrune)
	mux.Handle("/", s.web)
	return mux
}

// tryClaimRun marks a run in progress; false means one is already running.
func (s *Server) tryClaimRun() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.running {
		return false
	}
	s.running = true
	return true
}

func (s *Server) releaseRun() {
	s.mu.Lock()
	s.running = false
	s.mu.Unlock()
}
