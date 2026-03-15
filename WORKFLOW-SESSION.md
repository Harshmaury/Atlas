# WORKFLOW-SESSION.md
# @version: 1.0.0
# @updated: 2026-03-15
# @repo: https://github.com/Harshmaury/Atlas

---

## HOW TO START A SESSION

```bash
cd ~/workspace/projects/apps/atlas && ./scripts/verify.sh
```

Paste the output block into Claude. Claude reads KEY + this file.

---

## SESSION KEY

Format: AT-<git-short-hash>-<YYYYMMDD>
Example: AT-abc1234-20260315

Claude protocol:
  1. Fetch this file from raw GitHub URL in the block
  2. Match commit hash to build status below
  3. Confirm: "Loaded AT-<hash>. Phase: <current>. Ready."
  4. Ask for task — never assume

---

## IDENTITY

Developer:  Harsh Maury
GitHub:     https://github.com/Harshmaury
Atlas:      https://github.com/Harshmaury/Atlas
Domain:     Knowledge — reads the workspace, never writes state
OS:         Ubuntu 24.04 (WSL2) + Windows 11

---

## PLATFORM CONTEXT

Atlas is part of a three-domain developer platform:

  Control    Nexus   ~/workspace/projects/apps/nexus
  Knowledge  Atlas   ~/workspace/projects/apps/atlas  ← this repo
  Execution  Forge   ~/workspace/projects/apps/forge

Platform architecture:  ~/workspace/architecture/
ADRs:                   ~/workspace/architecture/decisions/

Atlas port: 127.0.0.1:8081
Nexus port: 127.0.0.1:8080

---

## MACHINE

Go:1.23.0  SQLite:FTS5  yaml.v3
Nexus eventbus imported for topic constants (ADR-002)

---

## BUILD STATUS

### ⏳ Phase 1 — Workspace Knowledge Index (NOT STARTED)
  cmd/atlas/main.go             daemon entry point
  internal/discovery/scanner.go workspace walker + project detection
  internal/indexer/source.go    source file indexer
  internal/indexer/document.go  architecture document indexer
  internal/store/db.go          SQLite + FTS5 migrations
  internal/nexus/client.go      Nexus HTTP client
  internal/nexus/subscriber.go  event bus topic subscription
  internal/api/server.go        HTTP server on :8081
  internal/context/generator.go AI context JSON

### ⏳ Phase 2 — Structured Capability Model (NOT STARTED)
  Requires Phase 1 complete
  internal/capability/extractor.go  structured claim extraction
  internal/graph/builder.go         workspace knowledge graph
  internal/conflict/detector.go     conflict detection queries

---

## DELIVERY PATTERN

Zip naming:  atlas-<phase>-<what>-<YYYYMMDD>-<HHMM>.zip
Drop folder: /mnt/c/Users/harsh/Downloads/atlas-drop/

Apply command:
  cd ~/workspace/projects/apps/atlas && \
  unzip -o /mnt/c/Users/harsh/Downloads/atlas-drop/<ZIP>.zip -d . && \
  go build ./... && \
  git add <files> WORKFLOW-SESSION.md && \
  git commit -m "<type>: <description>" && \
  git push origin <branch>

Full protocol: WORKFLOW-DELIVERY.md

---

## ATLAS DESIGN RULES

- Atlas reads only — never writes to Nexus, never starts services
- Import Nexus eventbus for topic constants only — never call Publish
- HTTP/JSON on 127.0.0.1:8081 — consistent with ADR-003
- Phase 2 does not start until Phase 1 index exists
- Storage is an internal detail — never expose SQLite schema externally

---

## CHANGELOG

2026-03-15  v1.0.0  Project scaffolded — documentation phase complete
