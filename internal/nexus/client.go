// @atlas-project: atlas
// @atlas-path: internal/nexus/client.go
// ADR-008: serviceToken field added; get() helper injects X-Service-Token
// on every outbound request except /health (which is always open).
// ISSUE-002 fix: removed duplicate traceIDKey — now uses middleware.TraceIDFromContext
// so the context key type matches what the TraceID middleware sets.
// Package nexus provides an HTTP client for querying the Nexus API.
// Atlas reads project data from Nexus — it never writes to Nexus state.
// ADR-001: Nexus is the canonical project registry.
package nexus

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	canon "github.com/Harshmaury/Canon/identity"
	"github.com/Harshmaury/Atlas/internal/api/middleware"
	nexusevents "github.com/Harshmaury/Nexus/pkg/events"
)

const defaultTimeout = 10 * time.Second

// NexusProject is the project record as returned by GET /projects.
type NexusProject struct {
	ID           string `json:"id"`
	Name         string `json:"name"`
	Path         string `json:"path"`
	Language     string `json:"language"`
	ProjectType  string `json:"project_type"`
	RegisteredAt string `json:"registered_at"`
}

// Client queries the Nexus HTTP API.
type Client struct {
	baseURL      string
	httpClient   *http.Client
	serviceToken string // ADR-008: sent as X-Service-Token on all non-health requests
}

// New creates a Nexus Client.
func New(nexusAddr string) *Client {
	return &Client{
		baseURL:    nexusAddr,
		httpClient: &http.Client{Timeout: defaultTimeout},
	}
}

// WithServiceToken sets the outbound X-Service-Token for ADR-008 inter-service auth.
func (c *Client) WithServiceToken(token string) *Client {
	c.serviceToken = token
	return c
}

// get is an internal helper that creates an authenticated GET request.
// Uses middleware.TraceIDFromContext so the context key type matches
// what the TraceID middleware sets — fixing the ISSUE-002 key mismatch.
func (c *Client) get(ctx context.Context, path string) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+path, nil)
	if err != nil {
		return nil, err
	}
	if c.serviceToken != "" && path != "/health" {
		req.Header.Set(canon.ServiceTokenHeader, c.serviceToken)
	}
	if traceID := middleware.TraceIDFromContext(ctx); traceID != "" {
		req.Header.Set(nexusevents.TraceIDHeader, traceID)
	}
	return c.httpClient.Do(req)
}

// GetProjects fetches all projects from the Nexus project registry.
// This is the ADR-001 authoritative project list.
func (c *Client) GetProjects(ctx context.Context) ([]*NexusProject, error) {
	resp, err := c.get(ctx, "/projects")
	if err != nil {
		return nil, fmt.Errorf("nexus: GET /projects: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("nexus: GET /projects returned HTTP %d", resp.StatusCode)
	}

	var envelope struct {
		OK   bool            `json:"ok"`
		Data json.RawMessage `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&envelope); err != nil {
		return nil, fmt.Errorf("nexus: decode response: %w", err)
	}
	if !envelope.OK {
		return nil, fmt.Errorf("nexus: API returned ok=false")
	}

	var projects []*NexusProject
	if err := json.Unmarshal(envelope.Data, &projects); err != nil {
		return nil, fmt.Errorf("nexus: decode projects: %w", err)
	}
	return projects, nil
}

// Ping checks if the Nexus daemon is reachable.
func (c *Client) Ping(ctx context.Context) error {
	resp, err := c.get(ctx, "/health")
	if err != nil {
		return fmt.Errorf("nexus unreachable at %s: %w", c.baseURL, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("nexus health check: HTTP %d", resp.StatusCode)
	}
	return nil
}



