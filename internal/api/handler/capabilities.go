// @atlas-project: atlas
// @atlas-path: internal/api/handler/capabilities.go
// CapabilityHandler serves Phase 2 capability and conflict detection endpoints.
package handler

import (
	"fmt"
	"net/http"

	"github.com/Harshmaury/Atlas/internal/graph"
	"github.com/Harshmaury/Atlas/internal/store"
)

// CapabilityHandler handles /workspace/capabilities and /workspace/conflicts.
type CapabilityHandler struct {
	store   store.Storer
	queries *graph.QueryRunner
}

// NewCapabilityHandler creates a CapabilityHandler.
func NewCapabilityHandler(s store.Storer, q *graph.QueryRunner) *CapabilityHandler {
	return &CapabilityHandler{store: s, queries: q}
}

// Capabilities handles GET /workspace/capabilities
// Returns all indexed capability claims grouped by domain and owner.
func (h *CapabilityHandler) Capabilities(w http.ResponseWriter, r *http.Request) {
	caps, err := h.store.GetAllCapabilities()
	if err != nil {
		respondErr(w, http.StatusInternalServerError, fmt.Errorf("get capabilities: %w", err))
		return
	}

	total, _ := h.store.CountCapabilities()

	respondOK(w, map[string]any{
		"total":        total,
		"capabilities": caps,
	})
}

// Conflicts handles GET /workspace/conflicts
// Runs all conflict detectors and returns the combined report.
func (h *CapabilityHandler) Conflicts(w http.ResponseWriter, r *http.Request) {
	report, err := h.queries.RunAll()
	if err != nil {
		respondErr(w, http.StatusInternalServerError, fmt.Errorf("conflict detection: %w", err))
		return
	}

	total := len(report.DuplicateOwnerships) +
		len(report.UndefinedConsumers) +
		len(report.OrphanedADRs) +
		len(report.CircularDependencies) +
		len(report.MissingDependencies) +
		len(report.UndeclaredImports)

	respondOK(w, map[string]any{
		"total_conflicts":       total,
		"duplicate_ownerships":  report.DuplicateOwnerships,
		"undefined_consumers":   report.UndefinedConsumers,
		"orphaned_adrs":         report.OrphanedADRs,
		"circular_dependencies": report.CircularDependencies,
		"missing_dependencies":  report.MissingDependencies,
		"undeclared_imports":    report.UndeclaredImports,
	})
}
