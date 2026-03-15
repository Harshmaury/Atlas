// @atlas-project: atlas
// @atlas-path: internal/store/db.go
// Package store manages the SQLite index database for Atlas.
// Uses FTS5 for full-text search over source files and documents.
// Versioned migrations follow the same pattern as Nexus state/db.go.
//
// Phase 2 additions (migration v2):
//   capabilities  — structured capability claims from architecture docs
//   graph_edges   — directional relationships between workspace entities
package store

import (
	"database/sql"
	"fmt"
	"time"

	_ "github.com/mattn/go-sqlite3"
)

// ── STORE ─────────────────────────────────────────────────────────────────────

// Store is the Atlas SQLite index store.
type Store struct {
	db *sql.DB
}

// New opens or creates the Atlas index database at dbPath.
// Runs versioned migrations automatically.
func New(dbPath string) (*Store, error) {
	db, err := sql.Open("sqlite3", dbPath+"?_journal_mode=WAL&_foreign_keys=on")
	if err != nil {
		return nil, fmt.Errorf("open atlas db: %w", err)
	}
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)

	if err := db.Ping(); err != nil {
		return nil, fmt.Errorf("ping atlas db: %w", err)
	}

	s := &Store{db: db}
	if err := s.migrate(); err != nil {
		return nil, fmt.Errorf("atlas migrations: %w", err)
	}
	return s, nil
}

func (s *Store) Close() error { return s.db.Close() }

// ── PROJECTS ──────────────────────────────────────────────────────────────────

func (s *Store) UpsertProject(p *Project) error {
	now := time.Now().UTC()
	_, err := s.db.Exec(`
		INSERT INTO projects (id, name, path, language, type, source, indexed_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET
			name=excluded.name, path=excluded.path,
			language=excluded.language, type=excluded.type,
			source=excluded.source, indexed_at=excluded.indexed_at
	`, p.ID, p.Name, p.Path, p.Language, p.Type, p.Source, now)
	if err != nil {
		return fmt.Errorf("upsert project %s: %w", p.ID, err)
	}
	return nil
}

func (s *Store) GetProject(id string) (*Project, error) {
	row := s.db.QueryRow(
		`SELECT id, name, path, language, type, source, indexed_at FROM projects WHERE id = ?`, id)
	p := &Project{}
	err := row.Scan(&p.ID, &p.Name, &p.Path, &p.Language, &p.Type, &p.Source, &p.IndexedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get project %s: %w", id, err)
	}
	return p, nil
}

func (s *Store) GetAllProjects() ([]*Project, error) {
	rows, err := s.db.Query(
		`SELECT id, name, path, language, type, source, indexed_at FROM projects ORDER BY name`)
	if err != nil {
		return nil, fmt.Errorf("get all projects: %w", err)
	}
	defer rows.Close()

	var projects []*Project
	for rows.Next() {
		p := &Project{}
		if err := rows.Scan(&p.ID, &p.Name, &p.Path, &p.Language, &p.Type, &p.Source, &p.IndexedAt); err != nil {
			return nil, fmt.Errorf("scan project: %w", err)
		}
		projects = append(projects, p)
	}
	return projects, rows.Err()
}

func (s *Store) DeleteProject(id string) error {
	_, err := s.db.Exec(`DELETE FROM projects WHERE id = ?`, id)
	return err
}

// ── FILES ─────────────────────────────────────────────────────────────────────

func (s *Store) UpsertFile(f *File) error {
	now := time.Now().UTC()
	_, err := s.db.Exec(`
		INSERT INTO files (project_id, path, language, size_bytes, indexed_at)
		VALUES (?, ?, ?, ?, ?)
		ON CONFLICT(path) DO UPDATE SET
			project_id=excluded.project_id, language=excluded.language,
			size_bytes=excluded.size_bytes, indexed_at=excluded.indexed_at
	`, f.ProjectID, f.Path, f.Language, f.SizeBytes, now)
	if err != nil {
		return fmt.Errorf("upsert file %s: %w", f.Path, err)
	}
	return nil
}

func (s *Store) GetFilesByProject(projectID string) ([]*File, error) {
	rows, err := s.db.Query(
		`SELECT id, project_id, path, language, size_bytes, indexed_at
		 FROM files WHERE project_id = ? ORDER BY path`, projectID)
	if err != nil {
		return nil, fmt.Errorf("get files for %s: %w", projectID, err)
	}
	defer rows.Close()
	return scanFiles(rows)
}

func (s *Store) DeleteFilesByProject(projectID string) error {
	_, err := s.db.Exec(`DELETE FROM files WHERE project_id = ?`, projectID)
	return err
}

func (s *Store) SearchFiles(query string, limit int) ([]*File, error) {
	if limit <= 0 {
		limit = 20
	}
	rows, err := s.db.Query(`
		SELECT f.id, f.project_id, f.path, f.language, f.size_bytes, f.indexed_at
		FROM files f
		JOIN files_fts fts ON fts.rowid = f.id
		WHERE files_fts MATCH ?
		ORDER BY rank LIMIT ?
	`, query, limit)
	if err != nil {
		return nil, fmt.Errorf("search files: %w", err)
	}
	defer rows.Close()
	return scanFiles(rows)
}

// ── DOCUMENTS ────────────────────────────────────────────────────────────────

func (s *Store) UpsertDocument(d *Document) error {
	now := time.Now().UTC()
	_, err := s.db.Exec(`
		INSERT INTO documents (project_id, path, doc_type, indexed_at)
		VALUES (?, ?, ?, ?)
		ON CONFLICT(path) DO UPDATE SET
			project_id=excluded.project_id, doc_type=excluded.doc_type,
			indexed_at=excluded.indexed_at
	`, d.ProjectID, d.Path, d.DocType, now)
	if err != nil {
		return fmt.Errorf("upsert document %s: %w", d.Path, err)
	}
	return nil
}

func (s *Store) GetDocumentsByProject(projectID string) ([]*Document, error) {
	rows, err := s.db.Query(
		`SELECT id, project_id, path, doc_type, indexed_at
		 FROM documents WHERE project_id = ? ORDER BY path`, projectID)
	if err != nil {
		return nil, fmt.Errorf("get docs for %s: %w", projectID, err)
	}
	defer rows.Close()
	return scanDocuments(rows)
}

func (s *Store) SearchDocuments(query string, limit int) ([]*Document, error) {
	if limit <= 0 {
		limit = 20
	}
	rows, err := s.db.Query(`
		SELECT d.id, d.project_id, d.path, d.doc_type, d.indexed_at
		FROM documents d
		JOIN documents_fts fts ON fts.rowid = d.id
		WHERE documents_fts MATCH ?
		ORDER BY rank LIMIT ?
	`, query, limit)
	if err != nil {
		return nil, fmt.Errorf("search documents: %w", err)
	}
	defer rows.Close()
	return scanDocuments(rows)
}

// ── STATS ─────────────────────────────────────────────────────────────────────

func (s *Store) CountFiles() (int, error) {
	var n int
	err := s.db.QueryRow(`SELECT COUNT(*) FROM files`).Scan(&n)
	return n, err
}

func (s *Store) CountDocuments() (int, error) {
	var n int
	err := s.db.QueryRow(`SELECT COUNT(*) FROM documents`).Scan(&n)
	return n, err
}

// ── CAPABILITIES (PHASE 2) ────────────────────────────────────────────────────

func (s *Store) UpsertCapability(c *Capability) error {
	now := time.Now().UTC()
	_, err := s.db.Exec(`
		INSERT INTO capabilities (project_id, owner, domain, name, doc_path, doc_type, indexed_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(owner, name) DO UPDATE SET
			project_id=excluded.project_id, domain=excluded.domain,
			doc_path=excluded.doc_path, doc_type=excluded.doc_type,
			indexed_at=excluded.indexed_at
	`, c.ProjectID, c.Owner, c.Domain, c.Name, c.DocPath, c.DocType, now)
	if err != nil {
		return fmt.Errorf("upsert capability %s/%s: %w", c.Owner, c.Name, err)
	}
	return nil
}

func (s *Store) GetCapabilitiesByOwner(owner string) ([]*Capability, error) {
	rows, err := s.db.Query(`
		SELECT id, project_id, owner, domain, name, doc_path, doc_type, indexed_at
		FROM capabilities WHERE owner = ? ORDER BY name
	`, owner)
	if err != nil {
		return nil, fmt.Errorf("get capabilities for %s: %w", owner, err)
	}
	defer rows.Close()
	return scanCapabilities(rows)
}

func (s *Store) GetAllCapabilities() ([]*Capability, error) {
	rows, err := s.db.Query(`
		SELECT id, project_id, owner, domain, name, doc_path, doc_type, indexed_at
		FROM capabilities ORDER BY domain, owner, name
	`)
	if err != nil {
		return nil, fmt.Errorf("get all capabilities: %w", err)
	}
	defer rows.Close()
	return scanCapabilities(rows)
}

func (s *Store) DeleteCapabilitiesByDoc(docPath string) error {
	_, err := s.db.Exec(`DELETE FROM capabilities WHERE doc_path = ?`, docPath)
	return err
}

func (s *Store) CountCapabilities() (int, error) {
	var n int
	err := s.db.QueryRow(`SELECT COUNT(*) FROM capabilities`).Scan(&n)
	return n, err
}

// ── GRAPH EDGES (PHASE 2) ─────────────────────────────────────────────────────

func (s *Store) UpsertEdge(e *GraphEdge) error {
	now := time.Now().UTC()
	_, err := s.db.Exec(`
		INSERT INTO graph_edges (from_id, to_id, edge_type, source, created_at)
		VALUES (?, ?, ?, ?, ?)
		ON CONFLICT(from_id, to_id, edge_type) DO UPDATE SET
			source=excluded.source, created_at=excluded.created_at
	`, e.FromID, e.ToID, e.EdgeType, e.Source, now)
	if err != nil {
		return fmt.Errorf("upsert edge %s→%s: %w", e.FromID, e.ToID, err)
	}
	return nil
}

func (s *Store) GetEdgesFrom(fromID string) ([]*GraphEdge, error) {
	rows, err := s.db.Query(`
		SELECT id, from_id, to_id, edge_type, source, created_at
		FROM graph_edges WHERE from_id = ? ORDER BY edge_type, to_id
	`, fromID)
	if err != nil {
		return nil, fmt.Errorf("get edges from %s: %w", fromID, err)
	}
	defer rows.Close()
	return scanEdges(rows)
}

func (s *Store) GetEdgesTo(toID string) ([]*GraphEdge, error) {
	rows, err := s.db.Query(`
		SELECT id, from_id, to_id, edge_type, source, created_at
		FROM graph_edges WHERE to_id = ? ORDER BY edge_type, from_id
	`, toID)
	if err != nil {
		return nil, fmt.Errorf("get edges to %s: %w", toID, err)
	}
	defer rows.Close()
	return scanEdges(rows)
}

func (s *Store) GetAllEdges() ([]*GraphEdge, error) {
	rows, err := s.db.Query(`
		SELECT id, from_id, to_id, edge_type, source, created_at
		FROM graph_edges ORDER BY from_id, edge_type, to_id
	`)
	if err != nil {
		return nil, fmt.Errorf("get all edges: %w", err)
	}
	defer rows.Close()
	return scanEdges(rows)
}

func (s *Store) DeleteEdgesBySource(source string) error {
	_, err := s.db.Exec(`DELETE FROM graph_edges WHERE source = ?`, source)
	return err
}

// ── MIGRATIONS ────────────────────────────────────────────────────────────────

type schemaVersion struct {
	version int
	up      string
}

var allMigrations = []schemaVersion{
	// v1 — initial schema (Phase 1)
	{1, `CREATE TABLE IF NOT EXISTS projects (
		id         TEXT PRIMARY KEY,
		name       TEXT NOT NULL DEFAULT '',
		path       TEXT NOT NULL DEFAULT '',
		language   TEXT NOT NULL DEFAULT '',
		type       TEXT NOT NULL DEFAULT '',
		source     TEXT NOT NULL DEFAULT 'detected',
		indexed_at DATETIME NOT NULL
	)`},
	{1, `CREATE TABLE IF NOT EXISTS files (
		id         INTEGER PRIMARY KEY AUTOINCREMENT,
		project_id TEXT NOT NULL DEFAULT '',
		path       TEXT NOT NULL UNIQUE,
		language   TEXT NOT NULL DEFAULT '',
		size_bytes INTEGER NOT NULL DEFAULT 0,
		indexed_at DATETIME NOT NULL
	)`},
	{1, `CREATE TABLE IF NOT EXISTS documents (
		id         INTEGER PRIMARY KEY AUTOINCREMENT,
		project_id TEXT NOT NULL DEFAULT '',
		path       TEXT NOT NULL UNIQUE,
		doc_type   TEXT NOT NULL DEFAULT '',
		indexed_at DATETIME NOT NULL
	)`},
	{1, `CREATE VIRTUAL TABLE IF NOT EXISTS files_fts USING fts5(
		path, language, content=files, content_rowid=id
	)`},
	{1, `CREATE VIRTUAL TABLE IF NOT EXISTS documents_fts USING fts5(
		path, doc_type, content=documents, content_rowid=id
	)`},
	{1, `CREATE INDEX IF NOT EXISTS idx_files_project    ON files(project_id)`},
	{1, `CREATE INDEX IF NOT EXISTS idx_files_language   ON files(language)`},
	{1, `CREATE INDEX IF NOT EXISTS idx_docs_project     ON documents(project_id)`},
	{1, `CREATE INDEX IF NOT EXISTS idx_docs_type        ON documents(doc_type)`},

	// v2 — structured capability model + graph (Phase 2)
	{2, `CREATE TABLE IF NOT EXISTS capabilities (
		id         INTEGER PRIMARY KEY AUTOINCREMENT,
		project_id TEXT    NOT NULL DEFAULT '',
		owner      TEXT    NOT NULL DEFAULT '',
		domain     TEXT    NOT NULL DEFAULT '',
		name       TEXT    NOT NULL DEFAULT '',
		doc_path   TEXT    NOT NULL DEFAULT '',
		doc_type   TEXT    NOT NULL DEFAULT '',
		indexed_at DATETIME NOT NULL,
		UNIQUE(owner, name)
	)`},
	{2, `CREATE TABLE IF NOT EXISTS graph_edges (
		id         INTEGER PRIMARY KEY AUTOINCREMENT,
		from_id    TEXT    NOT NULL DEFAULT '',
		to_id      TEXT    NOT NULL DEFAULT '',
		edge_type  TEXT    NOT NULL DEFAULT '',
		source     TEXT    NOT NULL DEFAULT '',
		created_at DATETIME NOT NULL,
		UNIQUE(from_id, to_id, edge_type)
	)`},
	{2, `CREATE INDEX IF NOT EXISTS idx_cap_owner  ON capabilities(owner)`},
	{2, `CREATE INDEX IF NOT EXISTS idx_cap_domain ON capabilities(domain)`},
	{2, `CREATE INDEX IF NOT EXISTS idx_edge_from  ON graph_edges(from_id)`},
	{2, `CREATE INDEX IF NOT EXISTS idx_edge_to    ON graph_edges(to_id)`},
	{2, `CREATE INDEX IF NOT EXISTS idx_edge_type  ON graph_edges(edge_type)`},
}

func (s *Store) migrate() error {
	if _, err := s.db.Exec(`
		CREATE TABLE IF NOT EXISTS schema_migrations (
			version    INTEGER PRIMARY KEY,
			applied_at DATETIME NOT NULL
		)
	`); err != nil {
		return fmt.Errorf("create schema_migrations: %w", err)
	}

	var current int
	if err := s.db.QueryRow(`SELECT COALESCE(MAX(version), 0) FROM schema_migrations`).Scan(&current); err != nil {
		return fmt.Errorf("read schema version: %w", err)
	}

	applied := map[int]bool{}
	for _, m := range allMigrations {
		if m.version <= current {
			continue
		}
		if _, err := s.db.Exec(m.up); err != nil {
			return fmt.Errorf("migration v%d: %w\nSQL: %s", m.version, err, m.up)
		}
		if !applied[m.version] {
			applied[m.version] = true
			if _, err := s.db.Exec(
				`INSERT OR IGNORE INTO schema_migrations (version, applied_at) VALUES (?, ?)`,
				m.version, time.Now().UTC(),
			); err != nil {
				return fmt.Errorf("record migration v%d: %w", m.version, err)
			}
		}
	}
	return nil
}

// ── SCAN HELPERS ─────────────────────────────────────────────────────────────

func scanFiles(rows *sql.Rows) ([]*File, error) {
	var files []*File
	for rows.Next() {
		f := &File{}
		if err := rows.Scan(&f.ID, &f.ProjectID, &f.Path, &f.Language, &f.SizeBytes, &f.IndexedAt); err != nil {
			return nil, fmt.Errorf("scan file: %w", err)
		}
		files = append(files, f)
	}
	return files, rows.Err()
}

func scanDocuments(rows *sql.Rows) ([]*Document, error) {
	var docs []*Document
	for rows.Next() {
		d := &Document{}
		if err := rows.Scan(&d.ID, &d.ProjectID, &d.Path, &d.DocType, &d.IndexedAt); err != nil {
			return nil, fmt.Errorf("scan document: %w", err)
		}
		docs = append(docs, d)
	}
	return docs, rows.Err()
}

func scanCapabilities(rows *sql.Rows) ([]*Capability, error) {
	var caps []*Capability
	for rows.Next() {
		c := &Capability{}
		if err := rows.Scan(&c.ID, &c.ProjectID, &c.Owner, &c.Domain, &c.Name, &c.DocPath, &c.DocType, &c.IndexedAt); err != nil {
			return nil, fmt.Errorf("scan capability: %w", err)
		}
		caps = append(caps, c)
	}
	return caps, rows.Err()
}

func scanEdges(rows *sql.Rows) ([]*GraphEdge, error) {
	var edges []*GraphEdge
	for rows.Next() {
		e := &GraphEdge{}
		if err := rows.Scan(&e.ID, &e.FromID, &e.ToID, &e.EdgeType, &e.Source, &e.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan edge: %w", err)
		}
		edges = append(edges, e)
	}
	return edges, rows.Err()
}
