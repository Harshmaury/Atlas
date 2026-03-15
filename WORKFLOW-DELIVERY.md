# WORKFLOW-DELIVERY.md
# @version: 2.0.0
# @updated: 2026-03-16

---

## DROP FOLDER

Windows:  C:\Users\harsh\Downloads\engx-drop\
WSL2:     /mnt/c/Users/harsh/Downloads/engx-drop/

---

## ZIP NAMING

```
atlas-<what>-<YYYYMMDD>-<HHMM>.zip
```

Examples: `atlas-fix-orphan-detector-20260316-1530.zip`
          `atlas-phase3-graph-api-20260316-0900.zip`

---

## ZIP STRUCTURE

Mirror the repo tree exactly. No wrapper folder.

---

## APPLY COMMAND

```bash
cd ~/workspace/projects/apps/atlas && \
unzip -o /mnt/c/Users/harsh/Downloads/engx-drop/<ZIP>.zip -d . && \
go build ./... && \
git add <files> WORKFLOW-SESSION.md && \
git commit -m "<type>: <description>" && \
git push origin <branch>
```

`go build ./...` must pass before `git add`. Always.

---

## RULES

- WORKFLOW-SESSION.md travels in every zip
- Version bumps on every delivery
- One logical unit per zip
- Grep all import usages before removing any import
