// @atlas-project: atlas
// @atlas-path: SERVICE-CONTRACT.md
# SERVICE-CONTRACT.md — Atlas
# @version: 0.5.0-phase3
# @updated: 2026-03-25

**Port:** 8081 · **DB:** `~/.nexus/atlas.db` · **Domain:** Knowledge

---

## Code

```
cmd/atlas/main.go              startup wiring
internal/api/handler/          workspace.go · graph.go · capabilities.go
internal/nexus/subscriber.go   polls GET /events?since=<id> every 3s
internal/validator/nexus_yaml.go  parses nexus.yaml, imports Canon/descriptor
internal/graph/builder.go      builds dependency graph from depends_on fields
internal/store/db.go           Storer interface, SQLite, versioned migrations
internal/capability/           parses capability claims from source files
```

---

## Contract

### Endpoints

| Method | Path | Auth | Description |
|--------|------|------|-------------|
| GET | `/health` | none | `{"ok":true,"status":"healthy","service":"atlas"}` |
| GET | `/workspace/projects` | token | All indexed projects with `status` field |
| GET | `/workspace/project/:id` | token | Single project detail |
| GET | `/graph/services` | token | Verified projects only — Forge preflight feed |
| GET | `/workspace/graph` | token | Dependency edges |

`GET /workspace/projects` and `GET /graph/services` are stable contracts. Breaking changes require ADR.

### Project status values

`status=verified` — `nexus.yaml` present and valid against `Canon/descriptor.ValidTypes`.
`status=unverified` — heuristic detection only or parse error. Never crashes on bad YAML.

### Failure conditions

| Code | Condition |
|------|-----------|
| 401 | Missing or invalid `X-Service-Token` |
| 404 | Project not found |

---

## Control

**Startup:** full workspace scan → index all `nexus.yaml` → HTTP server starts.

**Poll loop** (`internal/nexus/subscriber.go`): `GET /events?since=<lastID>` every 3s. On `workspace.project.detected` → re-index project. On `workspace.file.*` → re-index source files.

**Graph rebuild:** triggered after each indexing pass. Atomic: new graph replaces old under write lock before handlers can read it.

**Token enforcement:** `X-Service-Token` in `internal/api/middleware/service_auth.go`.

---

## Context

- Derives its project list from Nexus events. Does not own project registration.
- `GET /graph/services` is the authoritative feed for Forge preflight (ADR-010).
- Atlas never calls Forge, Guardian, Navigator, Observer, Metrics, or Sentinel.
- Atlas reads. It never starts services, never writes to Nexus state.
