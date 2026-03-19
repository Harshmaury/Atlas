// @atlas-project: atlas
// @atlas-path: internal/api/handler/capabilities.go
// Phase 4: CountCapabilities error now logged + graceful fallback (audit #5).
// CapabilityHandler gains a *log.Logger field — injected from ServerConfig.Logger.
package handler

import (
	"fmt"
	"log"
	"net/http"

	"github.com/Harshmaury/Atlas/internal/graph"
	"github.com/Harshmaury/Atlas/internal/store"
)

// CapabilityHandler handles /workspace/capabilities and /workspace/conflicts.
type CapabilityHandler struct {
	store   store.Storer
	queries *graph.QueryRunner
	logger  *log.Logger
}

// NewCapabilityHandler creates a CapabilityHandler.
func NewCapabilityHandler(s store.Storer, q *graph.QueryRunner, logger *log.Logger) *CapabilityHandler {
	if logger == nil {
		logger = log.Default()
	}
	return &CapabilityHandler{store: s, queries: q, logger: logger}
}

// Capabilities handles GET /workspace/capabilities
func (h *CapabilityHandler) Capabilities(w http.ResponseWriter, r *http.Request) {
	caps, err := h.store.GetAllCapabilities()
	if err != nil {
		respondErr(w, http.StatusInternalServerError, fmt.Errorf("get capabilities: %w", err))
		return
	}

	total, err := h.store.CountCapabilities()
	if err != nil {
		h.logger.Printf("WARNING: CountCapabilities: %v — using len(caps) as fallback", err)
		total = len(caps)
	}

	respondOK(w, map[string]any{
		"total":        total,
		"capabilities": caps,
	})
}

// Conflicts handles GET /workspace/conflicts
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
