# WORKFLOW-SESSION.md
# @version: 2.0.0
# @updated: 2026-03-16
# @repo: https://github.com/Harshmaury/Atlas

---

## START A SESSION

```bash
cd ~/workspace/projects/apps/atlas && ./scripts/verify.sh
```

Paste the output into Claude. Claude reads hash, confirms, asks for task.

---

## SESSION KEY

Format: `AT-<git-short-hash>-<YYYYMMDD>`

---

## IDENTITY

Developer: Harsh Maury  |  GitHub: https://github.com/Harshmaury
OS: Ubuntu 24.04 (WSL2) + Windows 11
Go: 1.25.0  SQLite: mattn/go-sqlite3 v1.14.34  YAML: gopkg.in/yaml.v3

---

## PLATFORM

```
Control    Nexus   ~/workspace/projects/apps/nexus   :8080
Knowledge  Atlas   ~/workspace/projects/apps/atlas   :8081  <- this
Execution  Forge   ~/workspace/projects/apps/forge   :8082
```

Atlas reads. It never writes to Nexus state, never starts services.

---

## BUILD STATUS
# Last verified: 2026-03-16

✅ Phase 1  Workspace knowledge index
✅ Phase 2  Capability model + graph builder + conflict detection
✅ v0.3.0-fixes-complete  All criticals + highs resolved

Tag: v0.3.0-fixes-complete -> commit 95ded78

---

## API ENDPOINTS

```
GET  /health
GET  /workspace                  workspace summary
GET  /workspace/projects         indexed projects
GET  /workspace/project/:id      project + files + docs
GET  /workspace/search?q=        full-text search
GET  /workspace/context          AI context snapshot
GET  /workspace/graph            relationship graph
GET  /workspace/capabilities     capability claims
GET  /workspace/conflicts        duplicate ownership, undefined consumers, orphaned ADRs
```

---

## ENVIRONMENT VARIABLES

  ATLAS_HTTP_ADDR    :8081
  ATLAS_WORKSPACE    ~/workspace
  ATLAS_DB_PATH      ~/.nexus/atlas.db
  NEXUS_HTTP_ADDR    http://127.0.0.1:8080

---

## BUILD + RUN

```bash
go build -o ~/bin/atlas ./cmd/atlas/
~/bin/atlas &
```

---

## ROADMAP

Phase 3 not started. ADR-driven when requirements emerge.

---

## CHANGELOG

2026-03-16  v2.0.0  All criticals + highs fixed, tagged v0.3.0-fixes-complete
2026-03-15  v1.8.0  Phase 2 complete — capability model, graph, conflict detection
2026-03-15  v1.1.0  Phase 1 complete — workspace knowledge index
