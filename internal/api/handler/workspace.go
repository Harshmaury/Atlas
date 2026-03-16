// @atlas-project: atlas
// @atlas-path: internal/api/handler/workspace.go
// WorkspaceHandler handles all /workspace routes.
// Handlers are thin adapters — parse → store/generator query → respond.
//
// Phase 3 (ADR-009): stable contract endpoints.
//   GET /workspace/projects  — all projects with status field
//   GET /workspace/project/:id — single project with capabilities, depends_on
//   GET /graph/services — verified projects only (graph membership)
// Breaking changes to these endpoints require a new ADR.
package handler

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/Harshmaury/Atlas/internal/context"
	"github.com/Harshmaury/Atlas/internal/store"
)

// WorkspaceHandler handles all /workspace routes.
type WorkspaceHandler struct {
	store     store.Storer
	generator *context.Generator
}

// NewWorkspaceHandler creates a WorkspaceHandler.
func NewWorkspaceHandler(s store.Storer, g *context.Generator) *WorkspaceHandler {
	return &WorkspaceHandler{store: s, generator: g}
}

// Summary handles GET /workspace
func (h *WorkspaceHandler) Summary(w http.ResponseWriter, r *http.Request) {
	ctx, err := h.generator.Generate()
	if err != nil {
		respondErr(w, http.StatusInternalServerError, fmt.Errorf("generate context: %w", err))
		return
	}
	respondOK(w, ctx)
}

// Projects handles GET /workspace/projects
// Phase 3: returns all projects (verified + unverified) with status field.
// Stable contract endpoint — shape must not change without a new ADR.
func (h *WorkspaceHandler) Projects(w http.ResponseWriter, r *http.Request) {
	projects, err := h.store.GetAllProjects()
	if err != nil {
		respondErr(w, http.StatusInternalServerError, fmt.Errorf("get projects: %w", err))
		return
	}
	respondOK(w, toProjectResponses(projects))
}

// Project handles GET /workspace/project/:id
// Phase 3: includes capabilities, depends_on, and status in response.
func (h *WorkspaceHandler) Project(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if id == "" {
		respondErr(w, http.StatusBadRequest, fmt.Errorf("project id required"))
		return
	}

	p, err := h.store.GetProject(id)
	if err != nil {
		respondErr(w, http.StatusInternalServerError, fmt.Errorf("get project: %w", err))
		return
	}
	if p == nil {
		respondErr(w, http.StatusNotFound, fmt.Errorf("project %q not found", id))
		return
	}

	files, _ := h.store.GetFilesByProject(id)
	docs, _   := h.store.GetDocumentsByProject(id)

	respondOK(w, map[string]any{
		"project":      toProjectResponse(p),
		"files":        len(files),
		"documents":    len(docs),
		"file_list":    files,
		"doc_list":     docs,
	})
}

// GraphServices handles GET /graph/services
// Phase 3 stable contract: returns only verified projects.
// Used by Forge Phase 4 pre-execution validation (ADR-010).
func (h *WorkspaceHandler) GraphServices(w http.ResponseWriter, r *http.Request) {
	projects, err := h.store.GetVerifiedProjects()
	if err != nil {
		respondErr(w, http.StatusInternalServerError, fmt.Errorf("get verified projects: %w", err))
		return
	}
	respondOK(w, toProjectResponses(projects))
}

// Search handles GET /workspace/search?q=<query>
func (h *WorkspaceHandler) Search(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query().Get("q")
	if q == "" {
		respondErr(w, http.StatusBadRequest, fmt.Errorf("q parameter required"))
		return
	}

	files, _ := h.store.SearchFiles(q, 20)
	docs, _   := h.store.SearchDocuments(q, 10)

	respondOK(w, map[string]any{
		"query":     q,
		"files":     files,
		"documents": docs,
	})
}

// Context handles GET /workspace/context
func (h *WorkspaceHandler) Context(w http.ResponseWriter, r *http.Request) {
	ctx, err := h.generator.Generate()
	if err != nil {
		respondErr(w, http.StatusInternalServerError, fmt.Errorf("generate context: %w", err))
		return
	}
	respondOK(w, ctx)
}

// ── RESPONSE TYPES ────────────────────────────────────────────────────────────

// projectResponse is the stable API shape for a project (ADR-009 contract).
// Adding fields is backward compatible. Removing or renaming fields requires ADR.
type projectResponse struct {
	ID           string   `json:"id"`
	Name         string   `json:"name"`
	Path         string   `json:"path"`
	Language     string   `json:"language"`
	Type         string   `json:"type"`
	Source       string   `json:"source"`
	Status       string   `json:"status"`
	Capabilities []string `json:"capabilities"`
	DependsOn    []string `json:"depends_on"`
	IndexedAt    string   `json:"indexed_at"`
}

// toProjectResponse converts a store.Project to the stable API shape.
func toProjectResponse(p *store.Project) projectResponse {
	caps := parseStringSlice(p.CapabilitiesJSON)
	deps := parseStringSlice(p.DependsOnJSON)
	return projectResponse{
		ID:           p.ID,
		Name:         p.Name,
		Path:         p.Path,
		Language:     p.Language,
		Type:         p.Type,
		Source:       p.Source,
		Status:       p.Status,
		Capabilities: caps,
		DependsOn:    deps,
		IndexedAt:    p.IndexedAt.Format("2006-01-02T15:04:05Z"),
	}
}

func toProjectResponses(projects []*store.Project) []projectResponse {
	out := make([]projectResponse, 0, len(projects))
	for _, p := range projects {
		out = append(out, toProjectResponse(p))
	}
	return out
}

// parseStringSlice unmarshals a JSON array string into a []string.
// Returns an empty slice on any error — never nil.
func parseStringSlice(raw string) []string {
	if raw == "" || raw == "[]" {
		return []string{}
	}
	var s []string
	if err := json.Unmarshal([]byte(raw), &s); err != nil {
		return []string{}
	}
	return s
}

// ── RESPONSE HELPERS ─────────────────────────────────────────────────────────

type apiResponse struct {
	OK    bool   `json:"ok"`
	Data  any    `json:"data,omitempty"`
	Error string `json:"error,omitempty"`
}

func respondOK(w http.ResponseWriter, data any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(apiResponse{OK: true, Data: data}) //nolint:errcheck
}

func respondErr(w http.ResponseWriter, status int, err error) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(apiResponse{OK: false, Error: err.Error()}) //nolint:errcheck
}
