// @atlas-project: atlas
// @atlas-path: internal/api/handler/workspace.go
// WorkspaceHandler handles all /workspace routes.
// Handlers are thin adapters — parse → store/generator query → respond.
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
func (h *WorkspaceHandler) Projects(w http.ResponseWriter, r *http.Request) {
	projects, err := h.store.GetAllProjects()
	if err != nil {
		respondErr(w, http.StatusInternalServerError, fmt.Errorf("get projects: %w", err))
		return
	}
	respondOK(w, projects)
}

// Project handles GET /workspace/project/:id
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

	files, _  := h.store.GetFilesByProject(id)
	docs, _   := h.store.GetDocumentsByProject(id)

	respondOK(w, map[string]any{
		"project":   p,
		"files":     len(files),
		"documents": len(docs),
		"file_list": files,
		"doc_list":  docs,
	})
}

// Search handles GET /workspace/search?q=<query>
func (h *WorkspaceHandler) Search(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query().Get("q")
	if q == "" {
		respondErr(w, http.StatusBadRequest, fmt.Errorf("q parameter required"))
		return
	}

	files, err := h.store.SearchFiles(q, 20)
	if err != nil {
		files = nil
	}

	docs, err := h.store.SearchDocuments(q, 10)
	if err != nil {
		docs = nil
	}

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
