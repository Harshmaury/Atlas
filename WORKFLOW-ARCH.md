# WORKFLOW-ARCH.md
# Atlas architecture rules and AI coding standards
# @version: 1.0.0
# @updated: 2026-03-15

---

## LAYER MAP

```
cmd/atlas/main.go       Entry point — wires all components
internal/api/           HTTP server and handlers (port 8081)
internal/discovery/     Workspace scanning and project detection
internal/indexer/       Source and document indexing
internal/store/         SQLite state (FTS5 index)
internal/nexus/         Nexus HTTP client + event subscription
internal/context/       AI context generation
internal/config/        Env helpers (same pattern as Nexus)
```

---

## PLATFORM RULES (from ADRs)

ADR-001: Nexus owns project registry
  Atlas queries GET /projects — never maintains its own canonical list

ADR-002: Nexus owns filesystem observation
  Atlas subscribes to workspace event topics from Nexus event bus
  Atlas never runs a filesystem watcher
  Import: github.com/Harshmaury/Nexus/internal/eventbus (constants only)

ADR-003: HTTP/JSON on 127.0.0.1:8081
  Response envelope: { ok, data, error }

---

## ATLAS-SPECIFIC DESIGN RULES

1. Read-only — Atlas never writes to Nexus state or any external system
2. No watcher — filesystem events come from Nexus subscription only
3. Phase gate — Phase 2 capabilities require Phase 1 index to exist
4. Internal storage — SQLite schema is never exposed through the API
5. Structured claims — Phase 2 indexes capability claims as structured
   records, not as raw document text
6. Single source — project list always sourced from Nexus, never derived
   independently

---

## AI CODING RULES

BEFORE WRITING CODE:
  State understanding in 2 lines
  List every file to create or modify
  Wait for approval

FILE NAMING:
  Format:  atlas_<package>_<filename>__<YYYYMMDD>_<HHMM>.go
  Example: atlas_indexer_source__20260315_0900.go
  Line 1:  // @atlas-project: atlas
  Line 2:  // @atlas-path: internal/indexer/source.go

CODE STANDARDS:
  SOLID — no exceptions
  Max 40 lines per function
  All errors handled explicitly
  Named constants — no magic numbers
  Dependency injection everywhere
  Interfaces over concrete types

TESTING:
  Every new component gets a test file
  Mock the Storer interface — never use real SQLite in tests
  Table-driven tests for multiple cases

---

## NEXUS EVENT SUBSCRIPTION PATTERN

```go
// Correct — import constants, never redefine
import "github.com/Harshmaury/Nexus/internal/eventbus"

bus.Subscribe(eventbus.TopicFileCreated, handler)
bus.Subscribe(eventbus.TopicWorkspaceUpdated, handler)

// Wrong — never do this
const myTopic = "workspace.file.created"  // redefines — breaks single source
```

---

## WHAT ATLAS MUST NEVER DO

- Run a filesystem watcher (Nexus owns this — ADR-002)
- Write to Nexus SQLite database
- Import Nexus internal packages other than eventbus
- Start, stop, or modify any service
- Maintain a canonical project list independently of Nexus
- Build Phase 2 graph before Phase 1 index is complete
