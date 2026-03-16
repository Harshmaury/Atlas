// @atlas-project: atlas
// @atlas-path: internal/store/storer.go
// Storer is the read/write contract for the Atlas index database.
// *Store satisfies this interface. Tests supply a mock.
//
// Phase 2 additions:
//   Capability — structured capability claim extracted from architecture docs
//   GraphEdge  — directional relationship between two workspace entities
package store

import "time"

// ── PHASE 1 TYPES ─────────────────────────────────────────────────────────────

// Project is a workspace project known to Atlas.
//
// Phase 3 additions (ADR-009):
//   Status           — "verified" | "unverified" based on nexus.yaml validation
//   CapabilitiesJSON — JSON array of declared capability strings from nexus.yaml
//   DependsOnJSON    — JSON array of project IDs this project depends on
type Project struct {
	ID               string
	Name             string
	Path             string
	Language         string
	Type             string
	Source           string // "nexus" | "detected"
	Status           string // "verified" | "unverified" (Phase 3 / ADR-009)
	CapabilitiesJSON string // JSON array e.g. ["rest-api","event-emitter"]
	DependsOnJSON    string // JSON array e.g. ["postgres","redis"]
	IndexedAt        time.Time
}

// File is an indexed source file.
type File struct {
	ID        int64
	ProjectID string
	Path      string
	Language  string
	SizeBytes int64
	IndexedAt time.Time
}

// Document is an indexed architecture or documentation file.
type Document struct {
	ID        int64
	ProjectID string
	Path      string
	DocType   string // "adr" | "spec" | "readme" | "constraint" | "guide" | "capability" | "workflow" | "architecture"
	IndexedAt time.Time
}

// ── PHASE 2 TYPES ─────────────────────────────────────────────────────────────

// Capability is a structured capability claim extracted from an architecture document.
// It records which service owns a named capability, what domain it belongs to,
// and which document is the authoritative source.
type Capability struct {
	ID        int64
	ProjectID string    // project the owning service belongs to
	Owner     string    // service that owns this capability (e.g. "nexus", "atlas")
	Domain    string    // "Control" | "Knowledge" | "Execution"
	Name      string    // capability name (e.g. "project-registry", "source-indexing")
	DocPath   string    // absolute path to the document this was extracted from
	DocType   string    // "adr" | "spec" | "capability"
	IndexedAt time.Time
}

// GraphEdge is a directional relationship between two workspace entities.
// Entities may be projects, services, or documents (identified by ID or path).
type GraphEdge struct {
	ID        int64
	FromID    string    // source entity ID or path
	ToID      string    // target entity ID or path
	EdgeType  string    // "depends_on" | "implements" | "references" | "contains"
	Source    string    // where this edge was detected: "nexus.yaml" | "import" | "adr" | "spec"
	CreatedAt time.Time
}

// ── STORER INTERFACE ──────────────────────────────────────────────────────────

// Storer is the Atlas index store contract.
type Storer interface {
	// ── Lifecycle ──────────────────────────────────────────────
	Close() error

	// ── Projects ───────────────────────────────────────────────
	UpsertProject(p *Project) error
	GetProject(id string) (*Project, error)
	GetAllProjects() ([]*Project, error)
	GetVerifiedProjects() ([]*Project, error) // Phase 3: verified only
	DeleteProject(id string) error

	// ── Files ──────────────────────────────────────────────────
	UpsertFile(f *File) error
	GetFilesByProject(projectID string) ([]*File, error)
	DeleteFilesByProject(projectID string) error
	SearchFiles(query string, limit int) ([]*File, error)

	// ── Documents ──────────────────────────────────────────────
	UpsertDocument(d *Document) error
	GetDocumentsByProject(projectID string) ([]*Document, error)
	GetAllDocuments() ([]*Document, error)
	SearchDocuments(query string, limit int) ([]*Document, error)

	// ── Stats ──────────────────────────────────────────────────
	CountFiles() (int, error)
	CountDocuments() (int, error)

	// ── Capabilities (Phase 2) ─────────────────────────────────
	UpsertCapability(c *Capability) error
	GetCapabilitiesByOwner(owner string) ([]*Capability, error)
	GetAllCapabilities() ([]*Capability, error)
	DeleteCapabilitiesByDoc(docPath string) error
	CountCapabilities() (int, error)

	// ── Graph Edges (Phase 2) ──────────────────────────────────
	UpsertEdge(e *GraphEdge) error
	GetEdgesFrom(fromID string) ([]*GraphEdge, error)
	GetEdgesTo(toID string) ([]*GraphEdge, error)
	GetAllEdges() ([]*GraphEdge, error)
	DeleteEdgesBySource(source string) error

	// WithEdgeTransaction executes fn inside a SQLite transaction.
	// If fn returns an error the transaction is rolled back.
	// Used by BuildAll to make delete+rebuild atomic per edge source (AT-H-02).
	WithEdgeTransaction(fn func() error) error
}
