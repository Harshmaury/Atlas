# SERVICE-CONTRACT.md — Atlas

**Service:** atlas
**Domain:** Knowledge
**Port:** 8081
**ADRs:** ADR-001 (sources from Nexus), ADR-006 (context for Forge),
         ADR-008 (auth), ADR-009 (nexus.yaml contract)
**Version:** 0.5.0-phase3
**Updated:** 2026-03-18

---

## Role

Workspace knowledge service. Indexes the workspace, builds the project
capability graph, validates nexus.yaml descriptors, and serves structured
workspace context to Forge. Atlas understands the system — it never acts on it.

---

## Inputs

- `Nexus GET /events?since=<id>` — workspace change events (polled every 3s)
- Local filesystem scan at startup — workspace indexing
- `nexus.yaml` files in project roots — capability and dependency declarations

---

## Outputs

- `GET /health` — health check (no auth)
- `GET /workspace/projects` — all indexed projects with status (stable contract)
- `GET /workspace/project/:id` — single project detail
- `GET /graph/services` — verified projects only (stable Forge preflight feed)
- `GET /workspace/graph` — graph edges (dependency relationships)

---

## Dependencies

| Service | Used for                          | Auth required   |
|---------|-----------------------------------|-----------------|
| Nexus   | Workspace events (polling)        | X-Service-Token |

Atlas does not depend on Forge, Guardian, Navigator, Observer, Metrics, or Sentinel.

---

## Guarantees

- `GET /workspace/projects` and `GET /graph/services` are stable contracts.
  Breaking changes require a new ADR.
- `status=verified` is only assigned to projects with a valid `nexus.yaml`.
  Heuristic-detected projects are always `status=unverified`.
- `nexus.yaml` parsing is lenient — unknown fields are ignored.
  Parse errors produce `status=unverified`, never a crash.
- `GetVerifiedProjects()` is the correct method for the Forge preflight feed.
- All migrations are in a single ordered slice in `internal/store/db.go`.

---

## Non-Responsibilities

- Atlas does not start or stop services — that is Nexus's domain.
- Atlas does not execute commands — that is Forge's domain.
- Atlas does not own the project registry — Nexus does (ADR-001).
  Atlas builds a derived view from Nexus events.
- Atlas never calls Forge, Guardian, Navigator, Observer, or Sentinel.
- Atlas does not make permit/deny decisions — it provides facts.
  Forge decides policy from Atlas facts (ADR-006).

---

## Data Authority

**Primary authority for:**
- Workspace project capability index — `~/.nexus/atlas.db`
- Project verification status — derived from nexus.yaml validity
- Workspace dependency graph — computed from nexus.yaml `depends_on` fields

**Derived from Nexus** — Atlas does not own project registration or runtime state.

---

## Concurrency Model

- SQLite store accessed through `store.Storer` interface.
- Nexus subscriber goroutine polls events independently of HTTP handlers.
- HTTP handlers read from SQLite directly — no in-memory cache to synchronise.
- `X-Trace-ID` middleware propagates trace IDs on all responses.
