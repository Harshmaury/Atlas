# WORKFLOW-SESSION.md
# @version: 2.0.0
# @updated: 2026-03-16
# @repo: https://github.com/Harshmaury/Atlas

---

## Start a Session

```bash
cd ~/workspace/projects/apps/atlas && ./scripts/verify.sh
```

Paste output into Claude. Session key format: `AT-<hash>-<YYYYMMDD>`

---

## Identity

Developer: Harsh Maury | OS: Ubuntu 24.04 (WSL2) + Windows 11
Go: 1.25.0 | Drop folder: /mnt/c/Users/harsh/Downloads/engx-drop/

---

## Platform

```
Control    Nexus  :8080
Knowledge  Atlas  :8081  ← this
Execution  Forge  :8082
```

Atlas reads. It never writes to Nexus state or starts services.

---

## Build Status
# Last verified: 2026-03-16

✅ Phase 1    Workspace knowledge index
✅ Phase 2    Capability model + graph + conflict detection
✅ ADR-008    Inter-service auth (ServiceAuth middleware)
✅ v0.3.0-fixes-complete  All criticals + highs resolved

---

## Environment Variables

```
ATLAS_HTTP_ADDR         :8081
ATLAS_WORKSPACE         ~/workspace
ATLAS_DB_PATH           ~/.nexus/atlas.db
NEXUS_HTTP_ADDR         http://127.0.0.1:8080
ATLAS_SERVICE_TOKEN     from ~/.nexus/service-tokens
```

---

## API

```
GET  /health
GET  /workspace                   workspace summary
GET  /workspace/projects          all indexed projects
GET  /workspace/project/:id       project detail + files + docs
GET  /workspace/search?q=         full-text search
GET  /workspace/context           AI context snapshot
GET  /workspace/graph             relationship graph
GET  /workspace/capabilities      capability claims
GET  /workspace/conflicts         duplicate ownership, undefined consumers, orphaned ADRs
```

---

## Key Files

```
internal/api/middleware/service_auth.go  ServiceAuth — validates Forge token (ADR-008)
internal/nexus/client.go                 get() helper — injects X-Service-Token on all calls
internal/nexus/subscriber.go             poll() — uses client.get(), sends token on events
internal/store/db.go                     allMigrations + WithEdgeTransaction (BEGIN/COMMIT)
internal/graph/builder.go                BuildAll — atomic per source, go/ast import parser
internal/graph/queries.go                FindOrphanedADRs — GetAllDocuments, self-ref fix
internal/discovery/scanner.go            manifestPriority — deterministic language detection
```

---

## Roadmap

Phase 3 not started. ADR required before implementation begins.

---

## Commands

All commands in `~/workspace/developer-platform/RUNBOOK.md`.

---

## Changelog

2026-03-16  v2.0.0  All criticals + highs fixed, ADR-008 implemented
2026-03-15  v1.8.0  Phase 2 complete — capability model, graph, conflict detection
2026-03-15  v1.1.0  Phase 1 complete — workspace knowledge index
