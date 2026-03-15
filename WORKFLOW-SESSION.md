# WORKFLOW-SESSION.md
# @version: 1.13.0
# @updated: 2026-03-16
# @repo: https://github.com/Harshmaury/Atlas

---

## HOW TO START A SESSION

```bash
cd ~/workspace/projects/apps/atlas && ./scripts/verify.sh
```

Paste the output block into Claude. Confirm + ask for task.

---

## SESSION KEY

Format: AT-<git-short-hash>-<YYYYMMDD>
Claude: fetch this file → match hash → confirm → ask for task.

---

## IDENTITY

Developer: Harsh Maury  |  GitHub: https://github.com/Harshmaury
Atlas: https://github.com/Harshmaury/Atlas
Domain: Knowledge — reads the workspace, never writes state
OS: Ubuntu 24.04 (WSL2) + Windows 11

---

## PLATFORM CONTEXT

  Control    Nexus   ~/workspace/projects/apps/nexus   :8080
  Knowledge  Atlas   ~/workspace/projects/apps/atlas   :8081  ← this
  Execution  Forge   ~/workspace/projects/apps/forge   :8082

Platform architecture:  ~/workspace/architecture/
Governance repo:        ~/workspace/developer-platform/

---

## MACHINE

Go:1.23.0  mattn/go-sqlite3:v1.14.34  gopkg.in/yaml.v3:v3.0.1
Nexus eventbus imported for topic constants (ADR-002)

---

## BUILD STATUS
# Last verified: 2026-03-15

### ✅ Phase 1 — Workspace Knowledge Index
  cmd/atlas/main.go                   daemon entry point, wiring, initial index
  internal/config/env.go              EnvOrDefault, ExpandHome
  internal/store/db.go                SQLite + FTS5 (projects, files, documents)
  internal/store/storer.go            Storer interface
  internal/nexus/client.go            Nexus HTTP client (ADR-001)
  internal/nexus/subscriber.go        workspace event polling (ADR-002)
  internal/discovery/scanner.go       workspace walker + project detection
  internal/indexer/source.go          source file indexer
  internal/indexer/document.go        architecture document indexer
  internal/context/generator.go       AI context JSON generation
  internal/api/server.go              HTTP server :8081 (ADR-003)
  internal/api/handler/workspace.go   GET /workspace routes

### ✅ Phase 2 — Structured Capability Model (COMPLETE)
  internal/capability/parser.go    ParseDocument — capability/boundary/ADR/spec extraction
  internal/capability/parser_test.go  table-driven tests, all three doc types
  internal/store/storer.go        Capability + GraphEdge types, 8 new interface methods
  internal/store/db.go             v2 migration — capabilities + graph_edges tables
  Requires Phase 1 index running and populated

---

## API ENDPOINTS (Phase 1)

  GET  http://127.0.0.1:8081/health
  GET  http://127.0.0.1:8081/workspace
  GET  http://127.0.0.1:8081/workspace/projects
  GET  http://127.0.0.1:8081/workspace/project/:id
  GET  http://127.0.0.1:8081/workspace/search?q=<query>
  GET  http://127.0.0.1:8081/workspace/context

---

## ENVIRONMENT VARIABLES

  ATLAS_HTTP_ADDR    default :8081
  ATLAS_WORKSPACE    default ~/workspace
  ATLAS_DB_PATH      default ~/.nexus/atlas.db
  NEXUS_HTTP_ADDR    default http://127.0.0.1:8080

---

## DELIVERY PATTERN

Zip naming:  atlas-<phase>-<what>-<YYYYMMDD>-<HHMM>.zip
Drop folder: /mnt/c/Users/harsh/Downloads/atlas-drop/

Apply:
  cd ~/workspace/projects/apps/atlas && \
  unzip -o /mnt/c/Users/harsh/Downloads/atlas-drop/<ZIP>.zip -d . && \
  go build ./... && \
  git add <files> WORKFLOW-SESSION.md && \
  git commit -m "<type>: <description>" && \
  git push origin <branch>

---

## CRITICAL FIXES

✅ AT-Fix-01  Path containment in reindexOnEvent (filepath.Rel) (2026-03-16)
✅ AT-Fix-02  Targeted re-index per file event, BuildAll deferred to WorkspaceUpdated
✅ AT-Fix-03  FindOrphanedADRs: GetAllDocuments (no N+1), self-ref fix (2026-03-16)
  internal/store/storer.go     GetAllDocuments added to interface
  internal/store/db.go         GetAllDocuments implementation
  internal/graph/queries.go    FindOrphanedADRs rewritten

## ATLAS CRITICALS — ALL COMPLETE ✅

## ATLAS HIGHS

✅ AT-H-01  extractGoImports uses go/ast parser (2026-03-16)
✅ AT-H-02  BuildAll delete+rebuild atomic per source (2026-03-16)
✅ AT-H-03  cutPrefix → strings.CutPrefix (2026-03-16)
✅ AT-H-04  language detection deterministic via ordered slice (2026-03-16)
✅ AT-H-05  DocumentIndexer.IndexProject returns WalkDir errors (2026-03-16)
✅ AT-H-06  subscriber.poll logs errors at WARNING level (2026-03-16)

## ATLAS HIGHS — ALL COMPLETE ✅
  internal/store/storer.go    WithEdgeTransaction added to interface
  internal/store/db.go        BEGIN/COMMIT/ROLLBACK implementation
  internal/graph/builder.go   three sources each wrapped in transaction
  internal/graph/builder.go  parser.ParseFile(ImportsOnly)
  cmd/atlas/main.go  reindexOnEvent — filepath.Rel + IndexDocument
                     TopicWorkspaceUpdated subscriber for BuildAll

## CHANGELOG

2026-03-16  v1.9.0  fix: AT-Fix-01+02 — path containment + targeted re-index on events
2026-03-16  v1.10.0 fix: AT-Fix-03 — orphan detector N+1 + self-reference false negative
2026-03-16  v1.11.0 fix: AT-H-01  — extractGoImports rewritten with go/ast parser
2026-03-16  v1.12.0 fix: AT-H-02  — BuildAll delete+rebuild atomic via WithEdgeTransaction
2026-03-16  v1.13.0 fix: AT-H-03+04+05+06 — cutPrefix, lang detection, walk errors, poll logging
2026-03-15  v1.0.0  Project scaffolded
2026-03-15  v1.8.0  Phase 2 complete — main.go wired, all 8 steps done
2026-03-15  v1.7.0  Phase 2 step 6 — API endpoints, capabilities + conflicts + graph
2026-03-15  v1.6.0  Phase 2 step 5 — conflict detection queries + tests
2026-03-15  v1.5.0  Phase 2 step 4 — graph builder + tests
2026-03-15  v1.4.0  Phase 2 step 3 — capability indexer + tests
2026-03-15  v1.3.0  Phase 2 step 2 — capability parser + tests
2026-03-15  v1.2.0  Phase 2 step 1 — store migration, Capability + GraphEdge schema
2026-03-15  v1.1.0  Phase 1 complete — workspace knowledge index
