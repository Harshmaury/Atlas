# WORKFLOW-ARCH.md
# @version: 2.0.0
# @updated: 2026-03-16

---

## LAYER MAP

```
cmd/atlas/main.go       Entry point — wires all components
internal/api/           HTTP server :8081, handlers
internal/discovery/     Workspace scanner, project detection
internal/indexer/       Source + document indexers
internal/capability/    Capability claim parser (ADR, spec, boundary docs)
internal/graph/         Graph builder (O(n+e)), conflict detection queries
internal/store/         SQLite + FTS5, Storer interface, versioned migrations
internal/nexus/         Nexus HTTP client + event subscriber (polling)
internal/context/       AI context generator
internal/config/        Env helpers
```

---

## PLATFORM RULES

ADR-001  Nexus owns project registry.
         Atlas queries GET /projects — never maintains its own list.

ADR-002  Nexus owns filesystem observation.
         Atlas subscribes to workspace events via HTTP polling — never runs a watcher.
         Import topic constants from github.com/Harshmaury/Nexus/pkg/events only.

ADR-003  HTTP/JSON on 127.0.0.1:8081.
         Response envelope: { ok, data, error }

---

## DESIGN RULES

1. Atlas reads — never writes to Nexus state or any external system.
2. No watcher — filesystem events come from Nexus polling only.
3. Phase gate — Phase 2 runs after Phase 1 index exists.
4. All migrations in store/db.go allMigrations slice — never in init().
5. extractGoImports uses go/ast parser — never hand-rolled line scanning.
6. BuildAll delete+rebuild is atomic per source via WithEdgeTransaction.
7. reindexOnEvent uses filepath.Rel for containment — never raw string prefix.
8. capIndexer.IndexDocument per file event — never IndexAll on every file change.
9. graphBuilder.BuildAll on TopicWorkspaceUpdated only — once per debounce window.

---

## AI CODING RULES

BEFORE WRITING CODE:
  State understanding in 2 lines
  List every file to create or modify
  Grep all import usages before adding or removing any import
  Wait for approval

FILE NAMING:
  Format:  atlas_<package>_<filename>__<YYYYMMDD>_<HHMM>.go
  Line 1:  // @atlas-project: atlas
  Line 2:  // @atlas-path: <relative/path/to/file.go>

CODE STANDARDS:
  SOLID — no exceptions
  Max 40 lines per function
  All errors handled explicitly
  Named constants — no magic numbers
  Dependency injection — no package-level mutable state
  Interfaces over concrete types

TESTING:
  Mock Storer — never use real SQLite in tests
  Table-driven tests for multiple cases

---

## DROP FOLDER

All deliveries go to: C:\Users\harsh\Downloads\engx-drop\
WSL2:                 /mnt/c/Users/harsh/Downloads/engx-drop/

---

## WHAT ATLAS MUST NEVER DO

- Run a filesystem watcher
- Write to Nexus state
- Import Nexus internal packages other than pkg/events
- Start, stop, or modify any service
- Redefine workspace topic strings locally
- Build the Phase 2 graph before Phase 1 index exists
