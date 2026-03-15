// @atlas-project: atlas
// @atlas-path: internal/nexus/client.go
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
	baseURL    string
	httpClient *http.Client
}

// New creates a Nexus Client.
func New(nexusAddr string) *Client {
	return &Client{
		baseURL:    nexusAddr,
		httpClient: &http.Client{Timeout: defaultTimeout},
	}
}

// GetProjects fetches all projects from the Nexus project registry.
// This is the ADR-001 authoritative project list.
func (c *Client) GetProjects(ctx context.Context) ([]*NexusProject, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+"/projects", nil)
	if err != nil {
		return nil, fmt.Errorf("nexus: build request: %w", err)
	}

	resp, err := c.httpClient.Do(req)
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
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+"/health", nil)
	if err != nil {
		return fmt.Errorf("nexus: ping: %w", err)
	}
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("nexus unreachable at %s: %w", c.baseURL, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("nexus health check: HTTP %d", resp.StatusCode)
	}
	return nil
}
