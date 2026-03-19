// @atlas-project: atlas
// @atlas-path: internal/api/server.go
// Phase 4: ServerConfig.Logger now threaded through to WorkspaceHandler
// and CapabilityHandler so they can log store errors (audit #5).
package api

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"time"

	"github.com/Harshmaury/Atlas/internal/api/handler"
	"github.com/Harshmaury/Atlas/internal/api/middleware"
	atlascontext "github.com/Harshmaury/Atlas/internal/context"
	"github.com/Harshmaury/Atlas/internal/graph"
	"github.com/Harshmaury/Atlas/internal/store"
)

// ServerConfig holds all dependencies for the Atlas HTTP server.
type ServerConfig struct {
	Addr         string
	Store        store.Storer
	Generator    *atlascontext.Generator
	QueryRunner  *graph.QueryRunner // Phase 2 — nil-safe, routes disabled if nil
	Logger       *log.Logger
	ServiceToken string // ADR-008: expected token from Forge; empty = unauthenticated
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

	// Phase 4: logger passed to handlers so store errors are logged (audit #5).
	workspaceH := handler.NewWorkspaceHandler(cfg.Store, cfg.Generator, logger)
	graphH     := handler.NewGraphHandler(cfg.Store)

	mux := http.NewServeMux()

	// ── Phase 1 routes ──────────────────────────────────────────────────────
	mux.HandleFunc("GET /health",                 handleHealth)
	mux.HandleFunc("GET /workspace",              workspaceH.Summary)
	mux.HandleFunc("GET /workspace/projects",     workspaceH.Projects)
	mux.HandleFunc("GET /workspace/project/{id}", workspaceH.Project)
	mux.HandleFunc("GET /workspace/search",       workspaceH.Search)
	mux.HandleFunc("GET /workspace/context",      workspaceH.Context)

	// ── Phase 2 routes ──────────────────────────────────────────────────────
	mux.HandleFunc("GET /workspace/graph", graphH.Graph)

	if cfg.QueryRunner != nil {
		capH := handler.NewCapabilityHandler(cfg.Store, cfg.QueryRunner, logger)
		mux.HandleFunc("GET /workspace/capabilities", capH.Capabilities)
		mux.HandleFunc("GET /workspace/conflicts",    capH.Conflicts)
	}

	// ── Phase 3 routes (ADR-009) ─────────────────────────────────────────────
	mux.HandleFunc("GET /graph/services", workspaceH.GraphServices)

	// ADR-008: wrap mux with ServiceAuth
	serviceTokens := map[string]string{}
	if cfg.ServiceToken != "" {
		serviceTokens["forge"] = cfg.ServiceToken
	}

	var h http.Handler = mux
	h = middleware.ServiceAuth(serviceTokens, logger)(h)
	h = middleware.TraceID(h)

	return &Server{
		http: &http.Server{
			Addr:         cfg.Addr,
			Handler:      h,
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
