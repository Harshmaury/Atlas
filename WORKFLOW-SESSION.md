# WORKFLOW-SESSION.md
# Session: AT-phase3-metadata-contract
# Date: 2026-03-17

## What changed — Atlas Phase 3 (ADR-009)

nexus.yaml metadata contract. Projects gain verified/unverified status.
GET /graph/services stable contract endpoint added.
X-Trace-ID propagation wired into Atlas API server.

## New files

- internal/validator/nexus_yaml.go      — parse + validate nexus.yaml
- internal/validator/nexus_yaml_test.go — table-driven tests (14 cases)
- internal/api/middleware/traceid.go    — X-Trace-ID middleware

## Modified files

- internal/store/db.go        — migration v3: status, capabilities_json, depends_on_json columns
                                 GetVerifiedProjects() added. scanProjects() helper added.
- internal/store/storer.go    — Project model updated. GetVerifiedProjects added to interface.
- internal/discovery/scanner.go — validates nexus.yaml, sets status per project
- internal/api/handler/workspace.go — stable contract responses, GraphServices handler
- internal/api/server.go      — GET /graph/services registered, TraceID middleware wired

## nexus.yaml files to place at each project root

- nexus.yaml  → ~/workspace/projects/apps/nexus/nexus.yaml
- atlas.yaml  → ~/workspace/projects/apps/atlas/nexus.yaml   (rename to nexus.yaml)
- forge.yaml  → ~/workspace/projects/apps/forge/nexus.yaml   (rename to nexus.yaml)

## Apply — Atlas

cd ~/workspace/projects/apps/atlas && \
unzip -o /mnt/c/Users/harsh/Downloads/engx-drop/atlas-phase3-metadata-contract-20260317.zip -d . && \
go build ./... && \
go test ./internal/validator/... && \
git add \
  internal/validator/nexus_yaml.go \
  internal/validator/nexus_yaml_test.go \
  internal/api/middleware/traceid.go \
  internal/store/db.go \
  internal/store/storer.go \
  internal/discovery/scanner.go \
  internal/api/handler/workspace.go \
  internal/api/server.go \
  nexus.yaml \
  WORKFLOW-SESSION.md && \
git commit -m "feat(phase3): nexus.yaml contract + verified status + /graph/services" && \
git tag v0.5.0-phase3 && \
git push origin main --tags

## Place nexus.yaml for Nexus and Forge projects

cp /mnt/c/Users/harsh/Downloads/engx-drop/atlas-phase3-metadata-contract-20260317.zip /tmp/ && \
cd ~/workspace/projects/apps/nexus && \
unzip -o /tmp/atlas-phase3-metadata-contract-20260317.zip nexus.yaml -d . && \
git add nexus.yaml && git commit -m "chore: add nexus.yaml descriptor (ADR-009)" && git push origin main

cd ~/workspace/projects/apps/forge && \
unzip -o /tmp/atlas-phase3-metadata-contract-20260317.zip forge.yaml -d . && \
mv forge.yaml nexus.yaml && \
git add nexus.yaml && git commit -m "chore: add nexus.yaml descriptor (ADR-009)" && git push origin main

## Verify

go test ./internal/validator/...
curl -s http://127.0.0.1:8081/workspace/projects | jq '.data[] | {id, status}'
curl -s http://127.0.0.1:8081/graph/services | jq '.data[] | {id, status, capabilities}'
curl -sv http://127.0.0.1:8081/health 2>&1 | grep X-Trace-ID
