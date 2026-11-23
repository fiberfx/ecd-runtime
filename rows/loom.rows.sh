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

# group repo
go_row RP1 repo "go vet ./... rc=0" /w/go vet ./...
go_row RP2 repo "go build ./... rc=0" /w/go build ./...

# group cut
go_row AK1 cut "go test ./... rc=0 in third_party/ark" /w/go/third_party/ark test ./...
if sel AK2 cut || sel AK2p cut; then
  want=$(sed -n 's/^Tree of ecs\/: *\([0-9a-f]\{40\}\).*/\1/p' /w/go/third_party/ark/SOURCE.txt)
  if [ -z "$want" ]; then
    sel AK2 cut && say AK2 "the tree hash of ecs/ from SOURCE.txt" "no hash in third_party/ark/SOURCE.txt" CNM
    sel AK2p cut && say AK2p "the hash comparison turns red on a changed file" "no hash in third_party/ark/SOURCE.txt" CNM
  else
    if sel AK2 cut; then
      got=$(tree_of /w/go/third_party/ark/ecs 2>/tmp/rows/AK2.err)
      if [ -z "$got" ]; then say AK2 "tree hash of third_party/ark/ecs = $want" "git could not compute the hash" CNM; head_of /tmp/rows/AK2.err 6
      elif [ "$got" = "$want" ]; then say AK2 "tree hash of third_party/ark/ecs = $want (SOURCE.txt)" "$got" PASS
      else say AK2 "tree hash of third_party/ark/ecs = $want (SOURCE.txt)" "$got" FAIL; fi
    fi
    if sel AK2p cut; then
      pd=$(mktemp -d /tmp/rows/plant.XXXXXX)
      cp -a /w/go/third_party/ark/ecs "$pd/ecs" && printf 'x' >>"$pd/ecs/entity.go"
      got=$(tree_of "$pd/ecs" 2>/tmp/rows/AK2p.err)
      rm -rf "$pd"
      if [ -z "$got" ]; then say AK2p "a byte appended to ecs/entity.go changes the hash" "git could not compute the hash" CNM
      elif [ "$got" = "$want" ]; then say AK2p "a byte appended to ecs/entity.go changes the hash" "hash unchanged: $got" SURVIVED
      else say AK2p "a byte appended to ecs/entity.go changes the hash" "hash $got differs from $want" KILLED; fi
    fi
  fi
fi

# group jacquard
go_row JQ1 jacquard "go test ./engine/jacquard/... rc=0" /w/go test ./engine/jacquard/...
if sel JQ1p jacquard; then
  if [ $HAVE_GO -eq 0 ]; then say JQ1p "TestReservedEntities fails on the plant" "go not on PATH" CNM
  else
    pd=$(mktemp -d /tmp/rows/plant.XXXXXX)
    cp -a /w/go "$pd/go" && cp "$pd/go/engine/jacquard/entity.go" "$pd/entity.go.orig" &&
      sed -i 's/return e\.id == 0$/return e.id == 1/' "$pd/go/engine/jacquard/entity.go"
    if cmp -s "$pd/entity.go.orig" "$pd/go/engine/jacquard/entity.go"; then
      say JQ1p "the plant changes entity.go" "entity.go unchanged after sed" CNM
    else
      log=/tmp/rows/JQ1p.log
      (cd "$pd/go" && go test -count=1 -v -run '^TestReservedEntities$' ./engine/jacquard/ >"$log" 2>&1); rc=$?
      if offline "$log"; then say JQ1p "TestReservedEntities fails on the plant" "module not found" CNM
      elif grep -q '^--- FAIL: TestReservedEntities' "$log"; then say JQ1p "TestReservedEntities fails on the plant" "rc=$rc, TestReservedEntities FAIL" KILLED
      elif [ $rc -eq 0 ] && grep -q '^--- PASS: TestReservedEntities' "$log"; then say JQ1p "TestReservedEntities fails on the plant" "rc=0, TestReservedEntities PASS" SURVIVED
      else say JQ1p "TestReservedEntities fails on the plant" "rc=$rc, the test did not run" CNM; head_of "$log" 8; fi
    fi
    rm -rf "$pd"
  fi
fi

if [ $RAN -le 1 ] && [ -n "$SEL" ] && [ "$SEL" != R0 ]; then echo "NOTE ROWS=$SEL selected no row besides R0"; CNM=1; fi
echo "END $(date -u +%Y-%m-%dT%H:%M:%SZ) rows $RAN bad $BAD cnm $CNM"
if [ $BAD -ne 0 ]; then exit 1; elif [ $CNM -ne 0 ]; then exit 3; fi
exit 0
