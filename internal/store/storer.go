// @atlas-project: atlas
// @atlas-path: internal/store/storer.go
// Storer is the read/write contract for the Atlas index database.
// *Store satisfies this interface. Tests supply a mock.
package store

import "time"

// Project is a workspace project known to Atlas.
type Project struct {
	ID         string
	Name       string
	Path       string
	Language   string
	Type       string
	Source     string // "nexus" | "detected"
	IndexedAt  time.Time
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
	DocType   string // "adr" | "spec" | "readme" | "constraint" | "guide"
	IndexedAt time.Time
}

// Storer is the Atlas index store contract.
type Storer interface {
	// ── Lifecycle ──────────────────────────────────────
	Close() error

	// ── Projects ───────────────────────────────────────
	UpsertProject(p *Project) error
	GetProject(id string) (*Project, error)
	GetAllProjects() ([]*Project, error)
	DeleteProject(id string) error

	// ── Files ──────────────────────────────────────────
	UpsertFile(f *File) error
	GetFilesByProject(projectID string) ([]*File, error)
	DeleteFilesByProject(projectID string) error
	SearchFiles(query string, limit int) ([]*File, error)

	// ── Documents ──────────────────────────────────────
	UpsertDocument(d *Document) error
	GetDocumentsByProject(projectID string) ([]*Document, error)
	SearchDocuments(query string, limit int) ([]*Document, error)

	// ── Stats ──────────────────────────────────────────
	CountFiles() (int, error)
	CountDocuments() (int, error)
}
