// @atlas-project: atlas
// @atlas-path: internal/capability/indexer_test.go
package capability

import (
	"log"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/Harshmaury/Atlas/internal/store"
)

// ── MOCK STORE ────────────────────────────────────────────────────────────────

// mockStore implements store.Storer for testing.
// Only the methods used by the Indexer are implemented.
type mockStore struct {
	projects     []*store.Project
	documents    map[string][]*store.Document // projectID → docs
	capabilities []*store.Capability
	deletedDocs  []string
}

func newMockStore() *mockStore {
	return &mockStore{documents: make(map[string][]*store.Document)}
}

func (m *mockStore) GetAllProjects() ([]*store.Project, error) { return m.projects, nil }

func (m *mockStore) GetDocumentsByProject(pid string) ([]*store.Document, error) {
	return m.documents[pid], nil
}

func (m *mockStore) DeleteCapabilitiesByDoc(path string) error {
	m.deletedDocs = append(m.deletedDocs, path)
	filtered := m.capabilities[:0]
	for _, c := range m.capabilities {
		if c.DocPath != path {
			filtered = append(filtered, c)
		}
	}
	m.capabilities = filtered
	return nil
}

func (m *mockStore) UpsertCapability(c *store.Capability) error {
	m.capabilities = append(m.capabilities, c)
	return nil
}

// Unused Storer methods — satisfy the interface.
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
func (m *mockStore) CountFiles() (int, error)        { return 0, nil }
func (m *mockStore) CountDocuments() (int, error)    { return 0, nil }
func (m *mockStore) CountCapabilities() (int, error) { return len(m.capabilities), nil }
func (m *mockStore) GetCapabilitiesByOwner(owner string) ([]*store.Capability, error) {
	return nil, nil
}
func (m *mockStore) GetAllCapabilities() ([]*store.Capability, error) {
	return m.capabilities, nil
}
func (m *mockStore) UpsertEdge(e *store.GraphEdge) error                           { return nil }
func (m *mockStore) GetEdgesFrom(fromID string) ([]*store.GraphEdge, error)        { return nil, nil }
func (m *mockStore) GetEdgesTo(toID string) ([]*store.GraphEdge, error)            { return nil, nil }
func (m *mockStore) GetAllEdges() ([]*store.GraphEdge, error)                      { return nil, nil }
func (m *mockStore) DeleteEdgesBySource(source string) error                       { return nil }

// ── TESTS ─────────────────────────────────────────────────────────────────────

func TestIndexAll_ExtractsCapabilities(t *testing.T) {
	// Write a real capability boundary doc to disk.
	dir := t.TempDir()
	docPath := filepath.Join(dir, "platform-capability-boundaries.md")
	content := `# Platform Capability Boundaries

| Capability               | Nexus | Atlas | Forge |
|--------------------------|-------|-------|-------|
| Project registry         | ✓     | ✗     | ✗     |
| Workspace discovery      | ✗     | ✓     | ✗     |
| Command intake           | ✗     | ✗     | ✓     |
`
	if err := os.WriteFile(docPath, []byte(content), 0644); err != nil {
		t.Fatalf("write doc: %v", err)
	}

	s := newMockStore()
	s.projects = []*store.Project{{ID: "platform", Name: "Platform", Path: dir}}
	s.documents["platform"] = []*store.Document{
		{ID: 1, ProjectID: "platform", Path: docPath, DocType: "capability", IndexedAt: time.Now()},
	}

	idx := NewIndexer(s, log.New(os.Stderr, "[test] ", 0))
	result, err := idx.IndexAll()
	if err != nil {
		t.Fatalf("IndexAll error: %v", err)
	}

	if result.DocsScanned != 1 {
		t.Errorf("DocsScanned = %d, want 1", result.DocsScanned)
	}
	if result.DocsIndexed != 1 {
		t.Errorf("DocsIndexed = %d, want 1", result.DocsIndexed)
	}
	if result.ClaimsFound != 3 {
		t.Errorf("ClaimsFound = %d, want 3", result.ClaimsFound)
	}
	if result.ClaimsStored != 3 {
		t.Errorf("ClaimsStored = %d, want 3", result.ClaimsStored)
	}
	if len(result.Errors) != 0 {
		t.Errorf("unexpected errors: %v", result.Errors)
	}
}

func TestIndexAll_SkipsNonParseable(t *testing.T) {
	dir := t.TempDir()
	docPath := filepath.Join(dir, "WORKFLOW-SESSION.md")
	if err := os.WriteFile(docPath, []byte("# Session\nsome content\n"), 0644); err != nil {
		t.Fatalf("write doc: %v", err)
	}

	s := newMockStore()
	s.projects = []*store.Project{{ID: "nexus", Name: "Nexus", Path: dir}}
	s.documents["nexus"] = []*store.Document{
		{ID: 1, ProjectID: "nexus", Path: docPath, DocType: "workflow", IndexedAt: time.Now()},
	}

	idx := NewIndexer(s, log.New(os.Stderr, "[test] ", 0))
	result, err := idx.IndexAll()
	if err != nil {
		t.Fatalf("IndexAll error: %v", err)
	}

	if result.DocsScanned != 1 {
		t.Errorf("DocsScanned = %d, want 1", result.DocsScanned)
	}
	if result.DocsIndexed != 0 {
		t.Errorf("DocsIndexed = %d, want 0 (non-parseable)", result.DocsIndexed)
	}
	if result.ClaimsStored != 0 {
		t.Errorf("ClaimsStored = %d, want 0", result.ClaimsStored)
	}
}

func TestIndexAll_ClearsStaleClaimsBeforeReindex(t *testing.T) {
	dir := t.TempDir()
	docPath := filepath.Join(dir, "platform-capability-boundaries.md")
	content := `| Project registry | ✓ | ✗ | ✗ |
`
	if err := os.WriteFile(docPath, []byte(content), 0644); err != nil {
		t.Fatalf("write doc: %v", err)
	}

	s := newMockStore()
	// Pre-populate a stale claim from the same doc.
	s.capabilities = []*store.Capability{
		{Owner: "nexus", Name: "stale-claim", DocPath: docPath},
	}
	s.projects = []*store.Project{{ID: "platform", Path: dir}}
	s.documents["platform"] = []*store.Document{
		{ID: 1, ProjectID: "platform", Path: docPath, DocType: "capability", IndexedAt: time.Now()},
	}

	idx := NewIndexer(s, log.New(os.Stderr, "[test] ", 0))
	if _, err := idx.IndexAll(); err != nil {
		t.Fatalf("IndexAll error: %v", err)
	}

	// DeleteCapabilitiesByDoc must have been called for this doc.
	found := false
	for _, d := range s.deletedDocs {
		if d == docPath {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected DeleteCapabilitiesByDoc(%q) to be called", docPath)
	}

	// Stale claim should be gone.
	for _, c := range s.capabilities {
		if c.Name == "stale-claim" {
			t.Errorf("stale claim should have been removed before re-indexing")
		}
	}
}

func TestIndexDocument_SingleFile(t *testing.T) {
	dir := t.TempDir()
	docPath := filepath.Join(dir, "atlas-specification.md")
	content := `# Atlas Spec

## What Atlas Owns

- Workspace discovery and project detection
- Source file indexing
`
	if err := os.WriteFile(docPath, []byte(content), 0644); err != nil {
		t.Fatalf("write doc: %v", err)
	}

	s := newMockStore()
	idx := NewIndexer(s, log.New(os.Stderr, "[test] ", 0))

	n, err := idx.IndexDocument(docPath, "atlas")
	if err != nil {
		t.Fatalf("IndexDocument error: %v", err)
	}
	if n != 2 {
		t.Errorf("IndexDocument returned %d stored, want 2", n)
	}
	if len(s.capabilities) != 2 {
		t.Errorf("store has %d capabilities, want 2", len(s.capabilities))
	}
}
