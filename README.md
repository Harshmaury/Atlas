# Atlas

Workspace Knowledge Service — Developer Platform Knowledge Domain

---

## What Atlas Is

Atlas is the knowledge system of the developer platform.
It provides structured awareness of the workspace so developers,
platform services, and AI systems can understand project structure,
source relationships, and architectural state.

Atlas reads. It never writes to Nexus state, never starts services,
and never executes workflows.

---

## Position in Platform

```
Control    Nexus   coordinates the system
Knowledge  Atlas   understands the system   ← this project
Execution  Forge   acts on the system
```

---

## Responsibilities

**Phase 1 — Workspace Knowledge Index**
- Workspace discovery and project detection
- Source file indexing (language, module, package, entry points)
- Architecture document indexing
- Project metadata ingestion from Nexus
- AI context generation

**Phase 2 — Structured Capability Model**
- Structured capability claim extraction from architecture documents
- Workspace knowledge graph (projects, services, modules, documents)
- Architecture conflict detection queries

---

## API

```
GET  http://127.0.0.1:8081/workspace            workspace summary
GET  http://127.0.0.1:8081/workspace/projects   indexed projects
GET  http://127.0.0.1:8081/workspace/project/:id project detail
GET  http://127.0.0.1:8081/workspace/search?q=  search workspace
GET  http://127.0.0.1:8081/workspace/context    AI context snapshot
GET  http://127.0.0.1:8081/health               liveness probe
```

---

## CLI (via engx)

```bash
engx workspace
engx workspace projects
engx workspace info <id>
engx workspace search <query>
engx workspace context
```

---

## Build

```bash
go build -o ~/bin/atlas ./cmd/atlas/
```

---

## Architecture

See `architecture/atlas-specification.md`

Platform-wide rules: `~/workspace/architecture/`
