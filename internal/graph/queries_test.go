// @atlas-project: atlas
// @atlas-path: internal/graph/queries_test.go
package graph

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/Harshmaury/Atlas/internal/store"
)

// ── MOCK STORE FOR QUERIES ────────────────────────────────────────────────────

type queryMockStore struct {
	projects     []*store.Project
	documents    map[string][]*store.Document
	capabilities []*store.Capability
	edges        []*store.GraphEdge
}

func newQueryMock() *queryMockStore {
	return &queryMockStore{documents: make(map[string][]*store.Document)}
}

func (m *queryMockStore) GetAllProjects() ([]*store.Project, error)  { return m.projects, nil }
func (m *queryMockStore) GetAllCapabilities() ([]*store.Capability, error) {
	return m.capabilities, nil
}
func (m *queryMockStore) GetAllEdges() ([]*store.GraphEdge, error) { return m.edges, nil }
func (m *queryMockStore) GetDocumentsByProject(pid string) ([]*store.Document, error) {
	return m.documents[pid], nil
}

// Unused interface methods.
func (m *queryMockStore) Close() error                                              { return nil }
func (m *queryMockStore) UpsertProject(p *store.Project) error                     { return nil }
func (m *queryMockStore) GetProject(id string) (*store.Project, error)             { return nil, nil }
func (m *queryMockStore) DeleteProject(id string) error                            { return nil }
func (m *queryMockStore) UpsertFile(f *store.File) error                           { return nil }
func (m *queryMockStore) GetFilesByProject(pid string) ([]*store.File, error)      { return nil, nil }
func (m *queryMockStore) DeleteFilesByProject(pid string) error                    { return nil }
func (m *queryMockStore) SearchFiles(q string, limit int) ([]*store.File, error)   { return nil, nil }
func (m *queryMockStore) UpsertDocument(d *store.Document) error                   { return nil }
func (m *queryMockStore) SearchDocuments(q string, limit int) ([]*store.Document, error) {
	return nil, nil
}
func (m *queryMockStore) CountFiles() (int, error)                                     { return 0, nil }
func (m *queryMockStore) CountDocuments() (int, error)                                 { return 0, nil }
func (m *queryMockStore) CountCapabilities() (int, error)                              { return 0, nil }
func (m *queryMockStore) UpsertCapability(c *store.Capability) error                   { return nil }
func (m *queryMockStore) GetCapabilitiesByOwner(o string) ([]*store.Capability, error) { return nil, nil }
func (m *queryMockStore) DeleteCapabilitiesByDoc(path string) error                    { return nil }
func (m *queryMockStore) UpsertEdge(e *store.GraphEdge) error                          { return nil }
func (m *queryMockStore) GetEdgesFrom(id string) ([]*store.GraphEdge, error)           { return nil, nil }
func (m *queryMockStore) GetEdgesTo(id string) ([]*store.GraphEdge, error)             { return nil, nil }
func (m *queryMockStore) DeleteEdgesBySource(source string) error                      { return nil }

// ── DETECTOR 1 — Duplicate ownership ─────────────────────────────────────────

func TestFindDuplicateOwnerships_Detected(t *testing.T) {
	s := newQueryMock()
	s.capabilities = []*store.Capability{
		{Owner: "nexus", Name: "project-registry", DocPath: "nexus/spec.md"},
		{Owner: "atlas", Name: "project-registry", DocPath: "atlas/spec.md"}, // duplicate!
		{Owner: "nexus", Name: "event-bus", DocPath: "nexus/spec.md"},        // single owner — OK
	}

	q := NewQueryRunner(s)
	results, err := q.FindDuplicateOwnerships()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("want 1 duplicate, got %d", len(results))
	}
	if results[0].CapabilityName != "project-registry" {
		t.Errorf("want project-registry, got %q", results[0].CapabilityName)
	}
	if len(results[0].Owners) != 2 {
		t.Errorf("want 2 owners, got %d", len(results[0].Owners))
	}
}

func TestFindDuplicateOwnerships_Clean(t *testing.T) {
	s := newQueryMock()
	s.capabilities = []*store.Capability{
		{Owner: "nexus", Name: "project-registry"},
		{Owner: "atlas", Name: "source-indexing"},
		{Owner: "forge", Name: "command-intake"},
	}

	q := NewQueryRunner(s)
	results, err := q.FindDuplicateOwnerships()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(results) != 0 {
		t.Errorf("expected no duplicates in clean architecture, got %d", len(results))
	}
}

// ── DETECTOR 2 — Undefined consumers ─────────────────────────────────────────

func TestFindUndefinedConsumers_Detected(t *testing.T) {
	s := newQueryMock()
	s.capabilities = []*store.Capability{
		{Owner: "nexus", Name: "project-registry"},
		{Owner: "atlas", Name: "source-indexing"},
	}
	s.edges = []*store.GraphEdge{
		// forge references nexus — known owner, OK
		{FromID: "forge", ToID: "nexus", EdgeType: "references", Source: "import"},
		// forge references "unknown-service" — not a known capability owner
		{FromID: "forge", ToID: "unknown-service", EdgeType: "references", Source: "import"},
	}

	q := NewQueryRunner(s)
	results, err := q.FindUndefinedConsumers()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("want 1 undefined consumer, got %d", len(results))
	}
	if results[0].ToID != "unknown-service" {
		t.Errorf("want ToID=unknown-service, got %q", results[0].ToID)
	}
}

func TestFindUndefinedConsumers_SkipsADRRefs(t *testing.T) {
	s := newQueryMock()
	s.capabilities = []*store.Capability{}
	s.edges = []*store.GraphEdge{
		// ADR references should be skipped — not project references
		{FromID: "atlas/spec.md", ToID: "ADR-002", EdgeType: "references", Source: "adr"},
		{FromID: "atlas/spec.md", ToID: "ADR-001", EdgeType: "references", Source: "adr"},
	}

	q := NewQueryRunner(s)
	results, err := q.FindUndefinedConsumers()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(results) != 0 {
		t.Errorf("ADR refs should be skipped, got %d undefined consumers", len(results))
	}
}

// ── DETECTOR 3 — Orphaned ADRs ────────────────────────────────────────────────

func TestFindOrphanedADRs_Detected(t *testing.T) {
	adr001 := filepath.Join("decisions", "ADR-001-project-registry.md")
	adr002 := filepath.Join("decisions", "ADR-002-workspace-observation.md")

	s := newQueryMock()
	s.projects = []*store.Project{{ID: "platform", Path: "/workspace"}}
	s.documents["platform"] = []*store.Document{
		{DocType: "adr", Path: adr001, IndexedAt: time.Now()},
		{DocType: "adr", Path: adr002, IndexedAt: time.Now()},
	}
	// Only ADR-001 is referenced — ADR-002 is orphaned.
	s.edges = []*store.GraphEdge{
		{FromID: "atlas/spec.md", ToID: "ADR-001", EdgeType: "references", Source: "adr"},
	}

	q := NewQueryRunner(s)
	results, err := q.FindOrphanedADRs()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("want 1 orphaned ADR, got %d", len(results))
	}
	if extractADRID(results[0].DocPath) != "ADR-002" {
		t.Errorf("expected ADR-002 to be orphaned, got %q", results[0].DocPath)
	}
}

func TestFindOrphanedADRs_AllReferenced(t *testing.T) {
	adr001 := filepath.Join("decisions", "ADR-001-project-registry.md")

	s := newQueryMock()
	s.projects = []*store.Project{{ID: "platform", Path: "/workspace"}}
	s.documents["platform"] = []*store.Document{
		{DocType: "adr", Path: adr001, IndexedAt: time.Now()},
	}
	s.edges = []*store.GraphEdge{
		{FromID: "spec.md", ToID: "ADR-001", EdgeType: "references", Source: "adr"},
	}

	q := NewQueryRunner(s)
	results, err := q.FindOrphanedADRs()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(results) != 0 {
		t.Errorf("expected no orphaned ADRs, got %d", len(results))
	}
}

// ── RunAll ────────────────────────────────────────────────────────────────────

func TestRunAll_ReturnsReport(t *testing.T) {
	s := newQueryMock()
	s.projects = []*store.Project{}
	s.capabilities = []*store.Capability{}
	s.edges = []*store.GraphEdge{}

	q := NewQueryRunner(s)
	report, err := q.RunAll()
	if err != nil {
		t.Fatalf("RunAll: %v", err)
	}
	if report == nil {
		t.Fatal("RunAll returned nil report")
	}
}
