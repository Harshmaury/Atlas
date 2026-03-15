// @atlas-project: atlas
// @atlas-path: internal/api/server.go
// Atlas HTTP API server on 127.0.0.1:8081 (ADR-003).
//
// Phase 2 additions:
//   GET /workspace/capabilities  — all indexed capability claims
//   GET /workspace/conflicts     — duplicate ownership, undefined consumers, orphaned ADRs
//   GET /workspace/graph         — workspace relationship graph edges
package api

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"time"

	"github.com/Harshmaury/Atlas/internal/api/handler"
	atlascontext "github.com/Harshmaury/Atlas/internal/context"
	"github.com/Harshmaury/Atlas/internal/graph"
	"github.com/Harshmaury/Atlas/internal/store"
)

// ServerConfig holds all dependencies for the Atlas HTTP server.
type ServerConfig struct {
	Addr        string
	Store       store.Storer
	Generator   *atlascontext.Generator
	QueryRunner *graph.QueryRunner   // Phase 2 — nil-safe, routes disabled if nil
	Logger      *log.Logger
}

// Server is the Atlas HTTP server.
type Server struct {
	http   *http.Server
	logger *log.Logger
}

// NewServer creates the Atlas HTTP server and registers all routes.
func NewServer(cfg ServerConfig) *Server {
	logger := cfg.Logger
	if logger == nil {
		logger = log.Default()
	}

	workspaceH   := handler.NewWorkspaceHandler(cfg.Store, cfg.Generator)
	graphH       := handler.NewGraphHandler(cfg.Store)

	mux := http.NewServeMux()

	// ── Phase 1 routes ──────────────────────────────────────────────────────
	mux.HandleFunc("GET /health",                handleHealth)
	mux.HandleFunc("GET /workspace",             workspaceH.Summary)
	mux.HandleFunc("GET /workspace/projects",    workspaceH.Projects)
	mux.HandleFunc("GET /workspace/project/{id}", workspaceH.Project)
	mux.HandleFunc("GET /workspace/search",      workspaceH.Search)
	mux.HandleFunc("GET /workspace/context",     workspaceH.Context)

	// ── Phase 2 routes ──────────────────────────────────────────────────────
	mux.HandleFunc("GET /workspace/graph", graphH.Graph)

	if cfg.QueryRunner != nil {
		capH := handler.NewCapabilityHandler(cfg.Store, cfg.QueryRunner)
		mux.HandleFunc("GET /workspace/capabilities", capH.Capabilities)
		mux.HandleFunc("GET /workspace/conflicts",    capH.Conflicts)
	}

	return &Server{
		http: &http.Server{
			Addr:         cfg.Addr,
			Handler:      mux,
			ReadTimeout:  15 * time.Second,
			WriteTimeout: 15 * time.Second,
			IdleTimeout:  60 * time.Second,
		},
		logger: logger,
	}
}

// Run starts the HTTP server and blocks until ctx is cancelled.
func (s *Server) Run(ctx context.Context) error {
	errCh := make(chan error, 1)
	go func() {
		s.logger.Printf("Atlas API listening on %s", s.http.Addr)
		if err := s.http.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			errCh <- fmt.Errorf("atlas http: %w", err)
		}
	}()

	select {
	case err := <-errCh:
		return err
	case <-ctx.Done():
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	s.logger.Println("Atlas API shutting down...")
	return s.http.Shutdown(shutdownCtx)
}

func handleHealth(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	w.Write([]byte(`{"ok":true,"status":"healthy","service":"atlas"}`)) //nolint:errcheck
}
