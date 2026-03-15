# Atlas Architecture Specification

Version: 1.0.0
Updated: 2026-03-15
Domain: Knowledge
Port: 127.0.0.1:8081
Repo: github.com/Harshmaury/Atlas

---

## Capability Domain

Atlas is the knowledge domain of the developer platform.

Platform-wide capability boundaries:
  ~/workspace/architecture/platform-capability-boundaries.md

Platform ADRs that govern Atlas:
  ADR-001 — Nexus is the canonical project registry
  ADR-002 — Nexus owns filesystem observation; Atlas subscribes
  ADR-003 — HTTP/JSON on 127.0.0.1:8081

---

## What Atlas Owns

- Workspace discovery and project detection
- Source file indexing
- Architecture document indexing
- Structured capability claim model (Phase 2)
- Workspace knowledge graph (Phase 2)
- Architecture conflict detection queries (Phase 2)
- AI context generation

## What Atlas Does Not Own

- Project registry authority (Nexus — ADR-001)
- Filesystem watchers (Nexus — ADR-002)
- Service lifecycle or runtime state
- Workflow execution (Forge)
- Runtime providers

---

## Technology Stack

Language:    Go 1.23.0
HTTP server: net/http stdlib
Database:    SQLite via mattn/go-sqlite3
             FTS5 extension for full-text search over source and docs
YAML:        gopkg.in/yaml.v3 — parse .nexus.yaml and architecture docs
Nexus:       github.com/Harshmaury/Nexus/internal/eventbus
             imported for topic constants only (ADR-002)
             Atlas never calls bus.Publish — subscribe only

### Why Go

Consistency with Nexus. Same build toolchain, same deployment model,
same HTTP/JSON API pattern. No additional runtime on the developer machine.

### Why SQLite + FTS5

SQLite is already proven in Nexus. FTS5 is a built-in SQLite extension
that provides full-text search over indexed source and document content
without a separate search service. Sufficient for workspace-scale data.

### Why No Heavy Dependencies

Atlas is a local developer service. It does not need Elasticsearch,
a graph database, or a message broker. SQLite adjacency tables handle
the Phase 2 graph. The only new dependency over Nexus is gopkg.in/yaml.v3.

---

## Project Structure

```
atlas/
  cmd/
    atlas/
      main.go             daemon entry point
  internal/
    api/
      server.go           HTTP server + routes
      handler/
        workspace.go      GET /workspace routes
        health.go         GET /health
    config/
      env.go              EnvOrDefault, ExpandHome (same pattern as Nexus)
    discovery/
      scanner.go          workspace directory walker
      detector.go         project detection from .nexus.yaml + structure
    indexer/
      source.go           source file indexing
      document.go         architecture document indexing
      language.go         extension → language mapping
    store/
      db.go               SQLite store + migrations
      storer.go           Storer interface
    context/
      generator.go        AI context JSON generation
    nexus/
      client.go           HTTP client for Nexus API queries
      subscriber.go       event bus topic subscription
  architecture/
    atlas-specification.md   ← this file
  go.mod
  go.sum
  .nexus.yaml
  README.md
  WORKFLOW-SESSION.md
  WORKFLOW-ARCH.md
  WORKFLOW-DELIVERY.md
```

---

## Implementation Phases

### Phase 1 — Workspace Knowledge Index

**Workspace discovery**
Walk ~/workspace/projects/ recursively.
Detect projects via: .nexus.yaml presence, go.mod, package.json,
*.csproj, Cargo.toml, pyproject.toml.
Ingest authoritative project list from Nexus GET /projects (ADR-001).

**Source indexing**
Index each source file: path, language, size, last modified.
Detect language from file extension.
Store in SQLite with FTS5 virtual table for content search.
Update on workspace events from Nexus event bus (ADR-002).

**Architecture document indexing**
Detect architecture documents: files in architecture/ directories,
ADR files matching ADR-NNN-*.md pattern, README.md files.
Index by: document type, project, date, referenced services.

**Project metadata ingestion**
On startup: GET http://127.0.0.1:8080/projects
On project events: subscribe to Nexus event bus topics.

**AI context generation**
Produce structured JSON on demand:
  workspace_root, projects, languages, services, architecture_docs

### Phase 2 — Structured Capability Model

Requires Phase 1 index to exist before beginning.

**Structured capability claims**
Parse architecture documents for capability declarations.
Store as structured records: capability, owner, scope, interfaces, deps.
This is what enables conflict detection — not just text indexing.

**Workspace graph**
Nodes: projects, services, modules, documents
Edges: depends_on, implements, references, contains
Built from source imports, .nexus.yaml, ADR cross-references.

**Conflict detection queries**
- Duplicate capability ownership
- Undefined capability consumers
- Orphaned ADRs

---

## Nexus Integration

### Project data (ADR-001)
Query on startup: GET http://127.0.0.1:8080/projects
Subscribe to: project lifecycle events from Nexus event bus

### Workspace events (ADR-002)
Subscribe to topics (import constants from Nexus eventbus package):
  workspace.file.created
  workspace.file.modified
  workspace.file.deleted
  workspace.project.detected

Import path: github.com/Harshmaury/Nexus/internal/eventbus
Rule: import constants only — never redefine topic strings locally

---

## HTTP API

Port: 127.0.0.1:8081 (ADR-003)
Override: ATLAS_HTTP_ADDR environment variable

Response envelope (consistent with Nexus):
  { "ok": true|false, "data": <payload>, "error": "<message>" }

Phase 1 endpoints:
  GET  /workspace                  workspace summary
  GET  /workspace/projects         list indexed projects
  GET  /workspace/project/:id      project detail
  GET  /workspace/search?q=        full-text search
  GET  /workspace/context          AI context snapshot
  GET  /health                     liveness probe

Phase 2 endpoints:
  GET  /workspace/graph            knowledge graph
  GET  /workspace/capabilities     capability claim list
  GET  /workspace/conflicts        conflict detection report
  GET  /workspace/architecture     architecture document summary

---

## Database Schema (Phase 1)

migrations table: schema_migrations (same versioned pattern as Nexus)

projects table:
  id, name, path, language, type, source (nexus|detected), indexed_at

files table:
  id, project_id, path, language, size_bytes, indexed_at
  FTS5 virtual table: files_fts (path, content snippet)

documents table:
  id, project_id, path, doc_type (adr|spec|readme|constraint), indexed_at
  FTS5 virtual table: documents_fts (path, content)

---

## Environment Variables

  ATLAS_HTTP_ADDR     default :8081
  ATLAS_WORKSPACE     default ~/workspace
  NEXUS_HTTP_ADDR     default 127.0.0.1:8080

---

## Atlas Design Principles

1. Atlas reads — never writes to Nexus state or service state
2. Atlas indexes — never orchestrates or executes
3. Atlas serves — answers queries, never pushes unsolicited data
4. Atlas defers — project authority in Nexus, events from Nexus
5. Phase 2 requires Phase 1 — graph builds on the index, not before it
