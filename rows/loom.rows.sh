#!/bin/sh
# Rows of Ecd Loom: the checks of the repository, run offline from the root of the unpacked tree (/w).
# The script needs the Go toolchain and git. Rows sit in groups, and ROWS selects them.
#
# Run command (flow:row-container, image mx-trafikist-toolchain:go1.27.4-rust1.100.0, whose /usr/local/go is go1.26.3):
#   kan flow run flow:row-container --param script=rows/loom.rows.sh \
#     --param checkout=/Users/jonny/dev/ecd/ecd-loom --param revision=SHA --param paths='go rows' --param name=loom \
#     --param image=mx-trafikist-toolchain:go1.27.4-rust1.100.0 \
#     [--param inputs=DIR] \
#     --param cpus=2 --param memory=4g --param timeout=1800
# inputs is optional: a GOPROXY file tree that /w/inputs provides for modules the repository requires. Without it
# GOPROXY is off, and the module has no requirements.
#
# Rows live in group files, rows/groups/NN-<group>.sh, which the script sources in name order between R0 and the
# final line. A group file is POSIX sh sourced from /w. It owns its row ids and its group name, calls "sel ID GROUP"
# before each row, may use the helpers (say, sel, offline, head_of, count, tree_of, go_row, HAVE_GO), and never
# exits. A later group is added as a file named NN-<group>.sh with NN above the files present.
#
# ROWS is a comma-separated list of row ids and group names. The default is every row. R0 always runs.
#
# Output: ROW id | expect X | observed Y | VERDICT. Verdicts: PASS, FAIL, KILLED and SURVIVED (a plant), CNM (could not
# measure: the row measured nothing, which is never a PASS).
#
#   group     row   check
#   tools     R0    go and git are on PATH
#   repo      RP1   go vet ./... in /w/go
#   repo      RP2   go build ./... in /w/go
#   cut       AK1   go test ./... in /w/go/third_party/ark: Ark's tests on the cut
#   cut       AK2   the tree hash of /w/go/third_party/ark/ecs equals the hash in third_party/ark/SOURCE.txt
#   cut       AK2p  plant: one byte appended to ecs/entity.go in a temporary copy, the hash comparison must turn red
#   jacquard  JQ1   go test ./engine/jacquard/... in /w/go: Ark's tests on the copy
#   jacquard  JQ1p  plant: in a temporary copy of the module, Entity.IsZero in engine/jacquard/entity.go tests
#                   "e.id == 1" instead of "e.id == 0"; TestReservedEntities (the reserved zero entity) must fail
#
# A plant is KILLED when its row turns red and SURVIVED when it stays green. A plant that cannot be applied, or in
# which no test ran, is CNM.
# Group files: 10-repo.sh (RP1, RP2), 20-cut.sh (AK1, AK2, AK2p), 30-jacquard.sh (JQ1, JQ1p).
# Exit: 0 every selected row as expected, 1 a row FAIL or a plant SURVIVED, 3 any CNM (and no FAIL or SURVIVED).
set -u
cd /w || exit 3
mkdir -p /tmp/rows
export GOTOOLCHAIN=local GOWORK=off GOFLAGS=-modcacherw GOMODCACHE=/tmp/gomod GOCACHE=/tmp/gocache GOSUMDB=off
if [ -d /w/inputs ]; then export GOPROXY=file:///w/inputs; else export GOPROXY=off; fi
export PATH="/usr/local/go/bin:$PATH"
export LANG=C.UTF-8 LC_ALL=C.UTF-8 TMPDIR=/tmp
export GIT_CONFIG_NOSYSTEM=1 GIT_CONFIG_GLOBAL=/dev/null
BAD=0
CNM=0
RAN=0
HAVE_GO=0
SEL=$(printf '%s' "${ROWS:-}" | tr -d ' ')

say() { # id expect observed verdict
  echo "ROW $1 | expect $2 | observed $3 | $4"
  RAN=$((RAN + 1))
  case $4 in FAIL|SURVIVED) BAD=1 ;; CNM) CNM=1 ;; esac
}
# sel id group: true when ROWS is empty or names the row or its group.
sel() {
  [ -z "$SEL" ] && return 0
  case ",$SEL," in *",$1,"*|*",$2,"*) return 0 ;; esac
  return 1
}
# A log that shows the offline toolchain lacking a module measured nothing.
offline() { grep -q -E 'no matching versions|module lookup disabled|reading file:///|dial tcp|no such host|cannot find module|missing go.sum entry|unrecognized import path|could not read Username|failed to download|GOPROXY list is not the empty string' "$1"; }
head_of() { head -"${2:-20}" "$1" | cut -c1-220 | sed 's/^/    | /'; }
count() { grep -c "$1" "$2" 2>/dev/null; }

# tree_of dir: the git tree hash of dir's content, computed in a temporary repository.
tree_of() {
  d=$(mktemp -d /tmp/rows/tree.XXXXXX) || return 1
  cp -a "$1" "$d/ecs" || { rm -rf "$d"; return 1; }
  (
    cd "$d" && git init -q . && git -c core.autocrlf=false -c core.excludesFile=/dev/null add -A >/dev/null 2>&1 &&
      t=$(git write-tree) && git rev-parse "$t:ecs"
  )
  rc=$?
  rm -rf "$d"
  return $rc
}

# go_row id group expect dir verb args...: go vet, build or test in dir. A row that finds no package or runs no test is CNM.
go_row() {
  id=$1 grp=$2 expect=$3 dir=$4 verb=$5
  shift 5
  sel "$id" "$grp" || return 0
  if [ $HAVE_GO -eq 0 ]; then say "$id" "$expect" "go not on PATH" CNM; return 0; fi
  log=/tmp/rows/$id.log
  case $verb in
    test)
      (cd "$dir" && go test -count=1 -v -timeout 900s "$@" >"$log" 2>&1); rc=$?
      np=$(count '^--- PASS' "$log"); nf=$(count '^--- FAIL' "$log"); ns=$(count '^--- SKIP' "$log")
      if offline "$log"; then say "$id" "$expect" "module not found" CNM; head_of "$log" 8
      elif [ $rc -ne 0 ]; then say "$id" "$expect" "rc=$rc pass=$np fail=$nf skip=$ns" FAIL; grep -B2 -A8 -E '^--- FAIL|^panic:|^FAIL|^# ' "$log" | head -40 | cut -c1-220 | sed 's/^/    | /'
      elif [ "$np" -eq 0 ]; then say "$id" "$expect" "rc=0, no test ran" CNM; head_of "$log" 8
      else say "$id" "$expect" "rc=0 pass=$np fail=0 skip=$ns" PASS; fi
      ;;
    *)
      pk=$(cd "$dir" && go list ./... 2>"$log.list" | wc -l)
      (cd "$dir" && go "$verb" "$@" >"$log" 2>&1); rc=$?
      if offline "$log" || offline "$log.list"; then say "$id" "$expect" "module not found" CNM; head_of "$log" 8
      elif [ "$pk" -eq 0 ]; then say "$id" "$expect" "no package found" CNM; head_of "$log.list" 8
      elif [ $rc -ne 0 ]; then say "$id" "$expect" "rc=$rc packages=$pk" FAIL; head_of "$log" 20
      else say "$id" "$expect" "rc=0 packages=$pk" PASS; fi
      ;;
  esac
}

echo "START $(date -u +%Y-%m-%dT%H:%M:%SZ) nproc $(nproc 2>/dev/null)"
GOV=$(go version 2>&1) && HAVE_GO=1
GITV=$(git --version 2>&1) && HAVE_GIT=1 || HAVE_GIT=0
echo "TOOL $GOV"
echo "TOOL $GITV"
if [ $HAVE_GO -eq 1 ] && [ $HAVE_GIT -eq 1 ]; then say R0 "go and git on PATH" "$GOV; $GITV" PASS
else say R0 "go and git on PATH" "go: $([ $HAVE_GO -eq 1 ] && echo found || echo absent); git: $([ $HAVE_GIT -eq 1 ] && echo found || echo absent)" CNM; fi

# groups
for f in /w/rows/groups/*.sh; do
  [ -f "$f" ] && . "$f"
done

if [ $RAN -le 1 ] && [ -n "$SEL" ] && [ "$SEL" != R0 ]; then echo "NOTE ROWS=$SEL selected no row besides R0"; CNM=1; fi
echo "END $(date -u +%Y-%m-%dT%H:%M:%SZ) rows $RAN bad $BAD cnm $CNM"
if [ $BAD -ne 0 ]; then exit 1; elif [ $CNM -ne 0 ]; then exit 3; fi
exit 0
