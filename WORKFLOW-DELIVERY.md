# WORKFLOW-DELIVERY.md
# Mandatory delivery protocol for Atlas
# @version: 1.0.0
# @updated: 2026-03-15

---

## ZIP NAMING

```
atlas-<phase>-<what>-<YYYYMMDD>-<HHMM>.zip
```

Examples:
  atlas-phase1-workspace-discovery-20260315-0900.zip
  atlas-fix1-fts5-query-20260315-1130.zip

---

## DROP FOLDER

Windows:  C:\Users\harsh\Downloads\atlas-drop\
WSL2:     /mnt/c/Users/harsh/Downloads/atlas-drop/

---

## ZIP STRUCTURE

Files inside mirror the repo directory tree exactly.
No wrapper folder. Unzip with -o -d . from repo root.

---

## STANDARD APPLY COMMAND

```bash
cd ~/workspace/projects/apps/atlas && \
unzip -o /mnt/c/Users/harsh/Downloads/atlas-drop/<ZIP>.zip -d . && \
go build ./... && \
git add <file1> <file2> ... WORKFLOW-SESSION.md && \
git commit -m "<type>: <description>" && \
git push origin <branch>
```

Single-file hotfix:
```bash
cd ~/workspace/projects/apps/atlas && \
unzip -oj /mnt/c/Users/harsh/Downloads/atlas-drop/<ZIP>.zip -d <target-dir>/ && \
go build ./... && \
git add <file> && \
git commit -m "fix: <description>" && \
git push origin <branch>
```

## RULE

go build ./... MUST pass before git add runs.
WORKFLOW-SESSION.md is always in git add.
Commit message follows: feat | fix | refactor | test | docs | chore

---

## CHECKLIST

- [ ] Zip named atlas-<phase>-<what>-<YYYYMMDD>-<HHMM>.zip
- [ ] Zip is in atlas-drop folder
- [ ] Running from ~/workspace/projects/apps/atlas
- [ ] On the correct branch
- [ ] go build ./... passes before git add
- [ ] WORKFLOW-SESSION.md is in git add
- [ ] Commit message follows type: description
