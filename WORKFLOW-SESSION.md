# WORKFLOW-SESSION.md
# @version: 1.2.0
# @updated: 2026-03-15
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

### 🔄 Phase 2 — Structured Capability Model (IN PROGRESS)
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

## CHANGELOG

2026-03-15  v1.0.0  Project scaffolded
2026-03-15  v1.2.0  Phase 2 step 1 — store migration, Capability + GraphEdge schema
2026-03-15  v1.1.0  Phase 1 complete — workspace knowledge index
