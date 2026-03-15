#!/usr/bin/env bash
# @atlas-project: atlas
# @atlas-path: scripts/verify.sh
# ─────────────────────────────────────────────────────────────
# ATLAS VERIFY v1.0
# Run at the start of every AI session.
# Prints a compact state snapshot + a paste block for Claude.
#
# Usage:
#   ./scripts/verify.sh          → full snapshot + paste block
#   ./scripts/verify.sh --short  → build status + key only
# ─────────────────────────────────────────────────────────────
set -euo pipefail

ATLAS_HOME="$HOME/workspace/projects/apps/atlas"
MODE="${1:---full}"

# ── COLORS ───────────────────────────────────────────────────
R='\033[0;31m' G='\033[0;32m' Y='\033[1;33m'
C='\033[0;36m' W='\033[1;37m' D='\033[2m' NC='\033[0m'

cd "$ATLAS_HOME"

# ── SESSION KEY ──────────────────────────────────────────────
# Format: AT-<git-short-hash>-<YYYYMMDD>
GIT_HASH=$(git rev-parse --short HEAD 2>/dev/null || echo "nogit")
SESSION_KEY="AT-${GIT_HASH}-$(date +%Y%m%d)"

# ── SHORT MODE ───────────────────────────────────────────────
if [ "$MODE" = "--short" ]; then
  echo ""
  echo -e "${W}KEY:${NC} $SESSION_KEY"
  echo -e "${D}WORKFLOW: https://raw.githubusercontent.com/Harshmaury/Atlas/main/WORKFLOW-SESSION.md${NC}"
  echo ""
  BUILD=$(go build ./... 2>&1)
  [ -z "$BUILD" ] \
    && echo -e "${G}✓ build PASS${NC}" \
    || echo -e "${R}✗ build FAIL${NC}\n$BUILD"
  echo ""
  exit 0
fi

# ── FULL SNAPSHOT ────────────────────────────────────────────
echo ""
echo -e "${C}${W}╔══════════════════════════════════════════════╗${NC}"
echo -e "${C}${W}║         ATLAS SESSION VERIFY  v1.0          ║${NC}"
echo -e "${C}${W}╚══════════════════════════════════════════════╝${NC}"
echo ""

# SESSION KEY
echo -e "${W}  SESSION KEY │ ${Y}$SESSION_KEY${NC}"
echo -e "${W}  WORKFLOW    │ ${D}https://raw.githubusercontent.com/Harshmaury/Atlas/main/WORKFLOW-SESSION.md${NC}"
echo ""

# GIT
echo -e "${C}── GIT ────────────────────────────────────────────${NC}"
BRANCH=$(git branch --show-current)
LAST=$(git log --oneline -1)
DIRTY=$(git status --short | wc -l | tr -d ' ')
echo -e "  branch  $BRANCH"
echo -e "  last    $LAST"
if [ "$DIRTY" -gt 0 ]; then
  echo -e "  status  ${R}$DIRTY uncommitted file(s)${NC}"
  git status --short | head -6 | sed 's/^/    /'
else
  echo -e "  status  ${G}clean${NC}"
fi
echo ""

# BUILD
echo -e "${C}── BUILD ──────────────────────────────────────────${NC}"
BUILD_OUT=$(go build ./... 2>&1)
if [ -z "$BUILD_OUT" ]; then
  echo -e "  go build ./...  ${G}PASS ✓${NC}"
else
  echo -e "  go build ./...  ${R}FAIL ✗${NC}"
  echo "$BUILD_OUT" | sed 's/^/    /'
fi
echo ""

# TESTS
echo -e "${C}── TESTS ──────────────────────────────────────────${NC}"
TEST_OUT=$(go test ./... -count=1 2>&1)
PASS=$(echo "$TEST_OUT" | grep -c "^ok" || true)
FAIL=$(echo "$TEST_OUT" | grep -c "^FAIL" || true)
if [ "$FAIL" -eq 0 ]; then
  echo -e "  go test ./...   ${G}PASS ✓  ($PASS packages)${NC}"
else
  echo -e "  go test ./...   ${R}FAIL ✗  ($FAIL failures)${NC}"
  echo "$TEST_OUT" | grep "FAIL" | sed 's/^/    /'
fi
echo ""

# PACKAGES
echo -e "${C}── PACKAGES ───────────────────────────────────────${NC}"
find . -name "*.go" -not -path "./.git/*" \
  | sed 's|^\./||' \
  | awk -F'/' 'NF>1{print $1"/"$2}' \
  | sort -u \
  | while read -r pkg; do
      COUNT=$(find "./$pkg" -name "*.go" 2>/dev/null | wc -l | tr -d ' ')
      echo "  $pkg  ($COUNT files)"
    done
echo ""

# PHASE STATUS from WORKFLOW-SESSION.md
echo -e "${C}── PHASE STATUS ───────────────────────────────────${NC}"
grep -E "^### [✅🔄⏳]" WORKFLOW-SESSION.md 2>/dev/null | sed 's/^/  /' || echo "  (no phase status found)"
echo ""

# API HEALTH
echo -e "${C}── API ────────────────────────────────────────────${NC}"
if pgrep -f "bin/atlas" > /dev/null 2>&1; then
  HEALTH=$(curl -s --connect-timeout 2 http://127.0.0.1:8081/health 2>/dev/null || echo "unreachable")
  if echo "$HEALTH" | grep -q '"ok":true'; then
    echo -e "  atlas   ${G}RUNNING${NC}  :8081"
    # Show index stats
    STATS=$(curl -s --connect-timeout 2 http://127.0.0.1:8081/workspace 2>/dev/null || echo "")
    if echo "$STATS" | grep -q '"ok":true'; then
      FILES=$(echo "$STATS" | python3 -c "import sys,json; d=json.load(sys.stdin)['data']; print(d.get('total_files',0))" 2>/dev/null || echo "?")
      CAPS=$(curl -s http://127.0.0.1:8081/workspace/capabilities 2>/dev/null \
             | python3 -c "import sys,json; print(json.load(sys.stdin)['data']['total'])" 2>/dev/null || echo "?")
      CONFLICTS=$(curl -s http://127.0.0.1:8081/workspace/conflicts 2>/dev/null \
             | python3 -c "import sys,json; print(json.load(sys.stdin)['data']['total_conflicts'])" 2>/dev/null || echo "?")
      echo -e "  files   $FILES indexed"
      echo -e "  caps    $CAPS capabilities"
      echo -e "  conflicts ${CONFLICTS}"
    fi
  else
    echo -e "  atlas   ${R}unreachable${NC}  :8081"
  fi
else
  echo -e "  atlas   ${R}stopped${NC}"
fi
echo ""

# DATABASE
echo -e "${C}── DATABASE ───────────────────────────────────────${NC}"
DB="$HOME/.nexus/atlas.db"
if [ -f "$DB" ]; then
  SIZE=$(du -sh "$DB" | cut -f1)
  echo -e "  atlas.db  ${G}present${NC}  ($SIZE)"
else
  echo -e "  atlas.db  ${R}missing${NC}  run atlas to create"
fi
echo ""

# ── PASTE BLOCK ──────────────────────────────────────────────
echo -e "${C}${W}╔══════════════════════════════════════════════╗${NC}"
echo -e "${C}${W}║  PASTE THIS BLOCK TO CLAUDE:                 ║${NC}"
echo -e "${C}${W}╚══════════════════════════════════════════════╝${NC}"
echo ""
echo "---ATLAS-SESSION-START---"
echo "KEY:     $SESSION_KEY"
echo "WORKFLOW: https://raw.githubusercontent.com/Harshmaury/Atlas/main/WORKFLOW-SESSION.md"
echo "BRANCH:  $BRANCH"
echo "COMMIT:  $LAST"
echo "BUILD:   $([ -z "$BUILD_OUT" ] && echo 'PASS' || echo 'FAIL')"
echo "TESTS:   $PASS packages passing, $FAIL failing"
echo "PHASE:   $(grep -E "^### [✅🔄⏳]" WORKFLOW-SESSION.md 2>/dev/null | tail -1 | sed 's/^### //' || echo 'unknown')"
echo "---ATLAS-SESSION-END---"
echo ""
