// @atlas-project: atlas
// @atlas-path: internal/graph/builder_test.go
package graph

import (
	"log"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/Harshmaury/Atlas/internal/store"
)

// ── MOCK STORE ────────────────────────────────────────────────────────────────

type mockStore struct {
	projects      []*store.Project
	documents     map[string][]*store.Document
	edges         []*store.GraphEdge
	deletedSource []string
}

func newMockStore() *mockStore {
	return &mockStore{documents: make(map[string][]*store.Document)}
}

func (m *mockStore) GetAllProjects() ([]*store.Project, error)  { return m.projects, nil }
func (m *mockStore) GetDocumentsByProject(pid string) ([]*store.Document, error) {
	return m.documents[pid], nil
}
func (m *mockStore) DeleteEdgesBySource(source string) error {
	m.deletedSource = append(m.deletedSource, source)
	filtered := m.edges[:0]
	for _, e := range m.edges {
		if e.Source != source {
			filtered = append(filtered, e)
		}
	}
	m.edges = filtered
	return nil
}
func (m *mockStore) UpsertEdge(e *store.GraphEdge) error {
	m.edges = append(m.edges, e)
	return nil
}

// Unused interface methods.
func (m *mockStore) Close() error                                              { return nil }
func (m *mockStore) UpsertProject(p *store.Project) error                     { return nil }
func (m *mockStore) GetProject(id string) (*store.Project, error)             { return nil, nil }
func (m *mockStore) DeleteProject(id string) error                            { return nil }
func (m *mockStore) UpsertFile(f *store.File) error                           { return nil }
func (m *mockStore) GetFilesByProject(pid string) ([]*store.File, error)      { return nil, nil }
func (m *mockStore) DeleteFilesByProject(pid string) error                    { return nil }
func (m *mockStore) SearchFiles(q string, limit int) ([]*store.File, error)   { return nil, nil }
func (m *mockStore) UpsertDocument(d *store.Document) error                   { return nil }
func (m *mockStore) SearchDocuments(q string, limit int) ([]*store.Document, error) {
	return nil, nil
}
func (m *mockStore) CountFiles() (int, error)                                       { return 0, nil }
func (m *mockStore) CountDocuments() (int, error)                                   { return 0, nil }
func (m *mockStore) CountCapabilities() (int, error)                                { return 0, nil }
func (m *mockStore) UpsertCapability(c *store.Capability) error                     { return nil }
func (m *mockStore) GetCapabilitiesByOwner(o string) ([]*store.Capability, error)   { return nil, nil }
func (m *mockStore) GetAllCapabilities() ([]*store.Capability, error)               { return nil, nil }
func (m *mockStore) DeleteCapabilitiesByDoc(path string) error                      { return nil }
func (m *mockStore) GetEdgesFrom(id string) ([]*store.GraphEdge, error)             { return nil, nil }
func (m *mockStore) GetEdgesTo(id string) ([]*store.GraphEdge, error)               { return nil, nil }
func (m *mockStore) GetAllEdges() ([]*store.GraphEdge, error)                       { return m.edges, nil }

// ── SOURCE 1 — nexus.yaml ────────────────────────────────────────────────────

func TestBuildNexusYAMLEdges(t *testing.T) {
	dir := t.TempDir()
	nexusYAML := `name: my-project
services:
  - id: api
    depends_on: [db, cache]
  - id: db
  - id: cache
`
	if err := os.WriteFile(filepath.Join(dir, ".nexus.yaml"), []byte(nexusYAML), 0644); err != nil {
		t.Fatalf("write .nexus.yaml: %v", err)
	}

	s := newMockStore()
	s.projects = []*store.Project{{ID: "my-project", Path: dir}}

	b := NewBuilder(s, log.New(os.Stderr, "[test] ", 0))
	result, err := b.BuildAll()
	if err != nil {
		t.Fatalf("BuildAll: %v", err)
	}

	if result.EdgesFromNexusYAML != 2 {
		t.Errorf("EdgesFromNexusYAML = %d, want 2 (api→db, api→cache)", result.EdgesFromNexusYAML)
	}

	// Verify edge contents.
	byKey := make(map[string]*store.GraphEdge)
	for _, e := range s.edges {
		byKey[e.FromID+"→"+e.ToID] = e
	}

	for _, key := range []string{"my-project:api→my-project:db", "my-project:api→my-project:cache"} {
		e, ok := byKey[key]
		if !ok {
			t.Errorf("missing edge %q", key)
			continue
		}
		if e.EdgeType != "depends_on" {
			t.Errorf("edge %q: type = %q, want depends_on", key, e.EdgeType)
		}
		if e.Source != "nexus.yaml" {
			t.Errorf("edge %q: source = %q, want nexus.yaml", key, e.Source)
		}
	}
}

func TestBuildNexusYAMLEdges_NoManifest(t *testing.T) {
	dir := t.TempDir() // no .nexus.yaml

	s := newMockStore()
	s.projects = []*store.Project{{ID: "no-manifest", Path: dir}}

	b := NewBuilder(s, log.New(os.Stderr, "[test] ", 0))
	result, err := b.BuildAll()
	if err != nil {
		t.Fatalf("BuildAll: %v", err)
	}
	if result.EdgesFromNexusYAML != 0 {
		t.Errorf("want 0 nexus.yaml edges for project without manifest, got %d",
			result.EdgesFromNexusYAML)
	}
}

// ── SOURCE 2 — Go imports ────────────────────────────────────────────────────

func TestBuildImportEdges(t *testing.T) {
	dir := t.TempDir()
	goFile := filepath.Join(dir, "main.go")
	content := `package main

import (
	"fmt"
	nexusevents "github.com/Harshmaury/Nexus/pkg/events"
	"github.com/Harshmaury/Atlas/internal/store"
)

func main() { fmt.Println(nexusevents.TopicWorkspaceUpdated) }
`
	if err := os.WriteFile(goFile, []byte(content), 0644); err != nil {
		t.Fatalf("write go file: %v", err)
	}

	s := newMockStore()
	s.projects = []*store.Project{{ID: "forge", Path: dir}}

	b := NewBuilder(s, log.New(os.Stderr, "[test] ", 0))
	result, err := b.BuildAll()
	if err != nil {
		t.Fatalf("BuildAll: %v", err)
	}

	// forge imports Nexus and Atlas → 2 edges
	if result.EdgesFromImports != 2 {
		t.Errorf("EdgesFromImports = %d, want 2 (forge→nexus, forge→atlas)", result.EdgesFromImports)
	}

	byKey := make(map[string]*store.GraphEdge)
	for _, e := range s.edges {
		if e.Source == "import" {
			byKey[e.FromID+"→"+e.ToID] = e
		}
	}

	for _, key := range []string{"forge→nexus", "forge→atlas"} {
		e, ok := byKey[key]
		if !ok {
			t.Errorf("missing import edge %q", key)
			continue
		}
		if e.EdgeType != "references" {
			t.Errorf("import edge %q: type = %q, want references", key, e.EdgeType)
		}
	}
}

func TestBuildImportEdges_NoSelfReference(t *testing.T) {
	dir := t.TempDir()
	goFile := filepath.Join(dir, "server.go")
	content := `package api

import (
	"github.com/Harshmaury/Atlas/internal/store"
)
`
	if err := os.WriteFile(goFile, []byte(content), 0644); err != nil {
		t.Fatalf("write go file: %v", err)
	}

	s := newMockStore()
	s.projects = []*store.Project{{ID: "atlas", Path: dir}}

	b := NewBuilder(s, log.New(os.Stderr, "[test] ", 0))
	result, err := b.BuildAll()
	if err != nil {
		t.Fatalf("BuildAll: %v", err)
	}
	if result.EdgesFromImports != 0 {
		t.Errorf("self-reference should be skipped, got %d import edges", result.EdgesFromImports)
	}
}

// ── SOURCE 3 — ADR references ────────────────────────────────────────────────

func TestBuildADRRefEdges(t *testing.T) {
	dir := t.TempDir()
	specPath := filepath.Join(dir, "atlas-specification.md")
	content := `# Atlas Specification

Atlas subscribes to workspace events per ADR-002.
Project data sourced from Nexus per ADR-001.
Communication protocol follows ADR-003.
ADR-002 is referenced again here but should only produce one edge.
`
	if err := os.WriteFile(specPath, []byte(content), 0644); err != nil {
		t.Fatalf("write spec: %v", err)
	}

	s := newMockStore()
	s.projects = []*store.Project{{ID: "atlas", Path: dir}}
	s.documents["atlas"] = []*store.Document{
		{ID: 1, ProjectID: "atlas", Path: specPath, DocType: "spec", IndexedAt: time.Now()},
	}

	b := NewBuilder(s, log.New(os.Stderr, "[test] ", 0))
	result, err := b.BuildAll()
	if err != nil {
		t.Fatalf("BuildAll: %v", err)
	}

	// 3 unique ADRs referenced → 3 edges
	if result.EdgesFromADRRefs != 3 {
		t.Errorf("EdgesFromADRRefs = %d, want 3 (ADR-001, ADR-002, ADR-003)", result.EdgesFromADRRefs)
	}
}

// ── Stale edge cleanup ────────────────────────────────────────────────────────

func TestBuildAll_ClearsStaleEdgesBeforeRebuild(t *testing.T) {
	s := newMockStore()
	s.projects = []*store.Project{}

	b := NewBuilder(s, log.New(os.Stderr, "[test] ", 0))
	if _, err := b.BuildAll(); err != nil {
		t.Fatalf("BuildAll: %v", err)
	}

	// All three sources must be cleared.
	sources := map[string]bool{}
	for _, src := range s.deletedSource {
		sources[src] = true
	}
	for _, want := range []string{"nexus.yaml", "import", "adr"} {
		if !sources[want] {
			t.Errorf("DeleteEdgesBySource(%q) was not called", want)
		}
	}
}
