// @atlas-project: atlas
// @atlas-path: internal/api/middleware/traceid.go
// TraceID middleware for Atlas — Phase 15 / Phase 3.
// Mirrors the Nexus middleware/traceid.go pattern exactly.
// Generates or propagates X-Trace-ID on every request.
package middleware

import (
	"context"
	"fmt"
	"net/http"
	"time"

	nexusevents "github.com/Harshmaury/Nexus/pkg/events"
)

// traceIDKey is the unexported context key for the trace ID.
type traceIDKey struct{}

// TraceID returns middleware that ensures every request carries a trace ID.
// If the inbound request already has X-Trace-ID, it is reused.
// Otherwise a new ID is generated. Stored in context and echoed in response.
func TraceID(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id := r.Header.Get(nexusevents.TraceIDHeader)
		if id == "" {
			id = fmt.Sprintf("atlas-%d", time.Now().UnixNano())
		}
		ctx := context.WithValue(r.Context(), traceIDKey{}, id)
		w.Header().Set(nexusevents.TraceIDHeader, id)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// TraceIDFromContext extracts the trace ID from a context.
func TraceIDFromContext(ctx context.Context) string {
	id, _ := ctx.Value(traceIDKey{}).(string)
	return id
}
