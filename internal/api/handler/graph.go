// @atlas-project: atlas
// @atlas-path: internal/api/handler/graph.go
// GraphHandler serves the workspace relationship graph endpoint.
package handler

import (
	"fmt"
	"net/http"

	"github.com/Harshmaury/Atlas/internal/store"
)

// GraphHandler handles GET /workspace/graph.
type GraphHandler struct {
	store store.Storer
}

// NewGraphHandler creates a GraphHandler.
func NewGraphHandler(s store.Storer) *GraphHandler {
	return &GraphHandler{store: s}
}

// Graph handles GET /workspace/graph
// Returns all graph edges grouped by edge type.
func (h *GraphHandler) Graph(w http.ResponseWriter, r *http.Request) {
	edges, err := h.store.GetAllEdges()
	if err != nil {
		respondErr(w, http.StatusInternalServerError, fmt.Errorf("get edges: %w", err))
		return
	}

	// Group edges by type for readability.
	byType := make(map[string][]*store.GraphEdge)
	for _, e := range edges {
		byType[e.EdgeType] = append(byType[e.EdgeType], e)
	}

	respondOK(w, map[string]any{
		"total": len(edges),
		"edges": edges,
		"by_type": byType,
	})
}
