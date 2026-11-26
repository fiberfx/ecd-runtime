#!/bin/sh
# The benchmark batch of Entity Component Runtime: the inputs of rule E9. Jacquard against Ark on the gates GJ1, GJ3
# and GJ4, with GJ2 recorded; and Jacquard alone on the gates G1 to G6 of the contract. The script runs the benchmarks
# in 6 rounds, prints the raw output whole, takes the median of the samples of each metric and prints one line per
# gate. It measures: a gate that is NOT-MET changes no exit code, and a gate that was not measured is CNM.
#
# Run command (flow:row-container, image mx-trafikist-toolchain:go1.27.4-rust1.100.0-r2, whose /usr/local/go is
# go1.27.4), on a box that runs no other container, since a second load changes the timings:
#   kan flow run flow:row-container --param script=rows/gj.bench.rows.sh \
#     --param checkout=CHECKOUT --param revision=SHA \
#     --param paths='go rows README.md LICENSE' --param name=gj \
#     --param image=mx-trafikist-toolchain:go1.27.4-rust1.100.0-r2 \
#     --param inputs=DIR \
#     --param cpus=2 --param memory=8g --param timeout=1800
# The timeout of the flow is at most 1800 s; the packages are compiled once before the rounds, so the rounds time
# only benchmarks.
# CHECKOUT is the directory of a checkout of this repository on the machine that starts the run.
# inputs is required and holds the GOPROXY file tree of the rows script: github.com/fiberfx/ecd-id/go, the Ark that
# go/enginecmp requires and the modules of go/sqlite. Without it GOPROXY is off, the modules do not load, and every
# gate is CNM.
#
#   gate  benchmark (package)                    metric             target
#   GJ1   BenchmarkGJ1Scan (gj)                  ns/entity          jacquard at or below ark
#   GJ2   BenchmarkGJ2Bytes (gj)                 B/entity           recorded, no target
#   GJ3   BenchmarkGJ3Apply (gj)                 ns/deed            jacquard at or below ark
#   GJ4   BenchmarkGJ4Hydrate (gj)               entities/s         jacquard at or above ark
#   G1    BenchmarkG1TypedGet/typed (llm)        allocs/op          0; ns/op recorded, and the seam's Get into a
#                                                                   caller-owned buffer beside it
#   G2    BenchmarkG2Query/q0 to q7 (sqlite)     allocs/op          0 for every declared query; ns/eid recorded
#   G3    BenchmarkGJ3Apply/jacquard (gj)        allocs/op          0
#         BenchmarkG3Bulk/bulk, single (gj)      ns/entity          bulk at or below single
#   G4    BenchmarkG4StatsBytes/jacquard (gj)    stats/heap         from 0.9 to 1.1; ark recorded
#   G5    BenchmarkG5Scan/jacquard (gj)          scan/cols          below 0.01
#   G6    BenchmarkG6PeakGC/jacquard (gj)        peakgc/resident    at most 2, the high-water with the collector on
#         BenchmarkG6HydratePeak (gj)            peak/resident      recorded: jacquard's upper bound with the
#                                                                   collector off, and ark's; the rate is GJ4's
#
# The packages: gj is go/enginecmp/gj, llm is go/internal/llm and sqlite is go/sqlite, each in its own module
# directory. G1 to G6 bind every engine and are measured on Jacquard; the values of Ark beside G4 and G6 are recorded.
# G6's verdict is read from the heap objects' high-water during a hydration with the collector on, which another
# goroutine samples: a sample can miss a higher value between two reads, so it bounds the true high-water from below.
# The value with the collector off counts every byte the hydration allocates and bounds it from above.
# GJ2 is recorded and not compared: the two engines count KindStats.Bytes differently, Jacquard the allocated capacity
# and the adapter over Ark the bytes of the held values, so no target applies. Not measured here: G4's budget of
# Touched kinds and G6's one read transaction per kind, which are rows of their own.
# Each benchmark reports its metric with b.ReportMetric under the unit of the table, or as allocs/op with -benchmem,
# and the script reads that unit and never ns/op, except for the recorded time of G1. The reported name ends in -N,
# the GOMAXPROCS of the run, which the script drops. A round runs every benchmark once with -count=1, in the order
# gj, llm, sqlite, so the two engines and the forms of G3 interleave round by round and a drift of the machine
# between rounds reaches all of them. The median of a metric is the middle value of its 6 samples sorted, and with an
# even count the mean of the two middle values. A metric with other than 6 samples leaves its gate CNM.
# The script does not use benchstat: the raw lines stay in the output so that a table can be made from them.
#
# Output: START, MACHINE, TOOL and ENGINE lines, the raw go test output of the 6 rounds in order, one line per gate
#   GATE id | target | observed ... | MET, NOT-MET, RECORDED or CNM
# with medians to 4 decimals, and END. ENGINE is the sha256 of the sha256sum list of every file under go/ except the
# _test.go files and go/enginecmp/, in name order: the engine code that the batch measured. The raw output is also kept
# in /tmp/rows/gj.bench.out.
# Exit: 0 every gate measured, 3 any gate CNM or go test not rc=0 in a round (a NOT-MET gate does not change the exit
# code).
set -u
set -f
ROOT=${GJ_ROOT:-/w}
cd "$ROOT" || exit 3
mkdir -p /tmp/rows
export GOTOOLCHAIN=local GOWORK=off GOFLAGS=-modcacherw GOMODCACHE=${GOMODCACHE_GJ:-/tmp/gomod} GOCACHE=${GOCACHE_GJ:-/tmp/gocache} GOSUMDB=off
if [ -d "$ROOT/inputs" ]; then export GOPROXY=file://$ROOT/inputs; else export GOPROXY=off; fi
export PATH="/usr/local/go/bin:$PATH"
export LANG=C.UTF-8 LC_ALL=C.UTF-8 TMPDIR=/tmp
export GIT_CONFIG_NOSYSTEM=1 GIT_CONFIG_GLOBAL=/dev/null
WANT=${GJ_ROUNDS:-6} # samples per metric: the number of rounds below, each of which takes one sample
OUT=/tmp/rows/gj.bench.out
RCF=/tmp/rows/gj.bench.rc
VALS=/tmp/rows/gj.bench.vals
rm -f "$RCF" "$VALS"

# The benchmark commands of a round: module directory, package, the -bench pattern and the timeout of go test.
GJ_CMD="$ROOT/go/enginecmp ./gj/ ^Benchmark(GJ[1-4]|G[3-6]) 420s"
LLM_CMD="$ROOT/go ./internal/llm/ ^BenchmarkG1TypedGet\$ 120s"
SQL_CMD="$ROOT/go/sqlite . ^BenchmarkG2Query\$ 300s"

# med bench sub unit: prints the number of samples and their median of the lines "bench sub unit value" of $VALS,
# "0 -" with none.
med() {
  awk -v b="$1" -v s="$2" -v u="$3" '$1 == b && $2 == s && $3 == u { print $4 }' "$VALS" | sort -g | awk '
    { v[NR] = $1 }
    END {
      if (NR == 0) { print 0, "-"; exit }
      if (NR % 2 == 1) m = v[(NR + 1) / 2]; else m = (v[NR / 2] + v[NR / 2 + 1]) / 2
      printf "%d %.4f\n", NR, m
    }'
}
# cmp A op B: true when the number A op B holds, op one of le, ge, lt, eq.
cmp() { awk -v a="$1" -v o="$2" -v b="$3" 'BEGIN {
  if (o == "le") ok = (a + 0 <= b + 0); else if (o == "ge") ok = (a + 0 >= b + 0)
  else if (o == "lt") ok = (a + 0 < b + 0); else ok = (a + 0 == b + 0)
  exit !ok }'; }

echo "START $(date -u +%Y-%m-%dT%H:%M:%SZ) nproc $(nproc 2>/dev/null) gomaxprocs ${GOMAXPROCS:-unset}"
echo "MACHINE $(uname -srm 2>/dev/null); cpu $(awk -F': ' '/^model name/ { print $2; exit }' /proc/cpuinfo 2>/dev/null)"
GOV=$(go version 2>&1) || GOV="go absent: $GOV"
echo "TOOL $GOV"
ENG=$(cd "$ROOT" && find go -type f ! -name '*_test.go' ! -path 'go/enginecmp/*' | LC_ALL=C sort | xargs sha256sum | sha256sum | cut -c1-64)
ENGN=$(cd "$ROOT" && find go -type f ! -name '*_test.go' ! -path 'go/enginecmp/*' | wc -l | tr -d ' ')
echo "ENGINE sha256 $ENG files $ENGN"

# The compile: each package's test binary is built and run with no test and no benchmark, so that the rounds time
# only benchmarks. A failed compile is reported and leaves the rounds to fail the same way.
for c in "$GJ_CMD" "$LLM_CMD" "$SQL_CMD"; do
  set -- $c
  ( cd "$1" && go test -count=1 -run '^$' "$2" >/tmp/rows/gj.build.log 2>&1 ) || {
    echo "NOTE the compile of $2 in $1 failed:"; head -20 /tmp/rows/gj.build.log | sed 's/^/    | /'; }
done
echo "BUILT $(date -u +%Y-%m-%dT%H:%M:%SZ)"

# The run: WANT rounds, each with -count=1 for every command, so that a round runs every sub-benchmark once and the
# engines interleave round by round. The output of every round is printed and appended to $OUT. The rc of each command
# is one line of a file, since a pipe to tee hides it; RC is the first rc that is not 0, 0 when every command was 0,
# and none when none recorded one.
: >"$OUT"
round=0
while [ "$round" -lt "$WANT" ]; do
  round=$((round + 1))
  echo "ROUND $round $(date -u +%Y-%m-%dT%H:%M:%SZ)" | tee -a "$OUT"
  for c in "$GJ_CMD" "$LLM_CMD" "$SQL_CMD"; do
    set -- $c
    ( cd "$1" && go test -count=1 -run '^$' -bench "$3" -benchmem -timeout "$4" "$2" 2>&1
      echo $? >>"$RCF" ) | tee -a "$OUT"
  done
done
RC=$(awk '
  NF == 0 { next }
  { k++; if ($1 != 0 && r == "") r = $1 }
  END { if (k == 0) print "none"; else if (r == "") print 0; else print r }' "$RCF" 2>/dev/null)
[ -n "$RC" ] || RC=none

# The samples: one line "bench sub unit value" per value and unit of every result line of a sub-benchmark.
awk '
  $1 ~ /^Benchmark[^ ]*\/[^ ]*$/ {
    name = $1
    sub(/^Benchmark/, "", name)
    sub(/-[0-9]+$/, "", name)
    if (split(name, p, "/") != 2) next
    for (i = 3; i < NF; i += 2)
      if ($i ~ /^[0-9]+(\.[0-9]+)?([eE][+-]?[0-9]+)?$/) print p[1], p[2], $(i + 1), $i
  }' "$OUT" >"$VALS"
PROCS=$(awk '$1 ~ /^Benchmark/ { p = $1; if (p ~ /-[0-9]+$/) sub(/^.*-/, "", p); else p = 1; print p; exit }' "$OUT")
[ -n "$PROCS" ] || PROCS=none

MET=0 NOTMET=0 REC=0 CNM=0
# gate id target observed verdict: prints the GATE line and counts the verdict.
gate() {
  case $4 in
    MET) MET=$((MET + 1)) ;;
    NOT-MET) NOTMET=$((NOTMET + 1)) ;;
    RECORDED) REC=$((REC + 1)) ;;
    *) CNM=$((CNM + 1)) ;;
  esac
  echo "GATE $1 | $2 | observed $3 | $4"
}
# get bench sub unit: sets N and M, the number of samples and the median, and OK=0 when N is not WANT.
get() {
  r=$(med "$1" "$2" "$3"); N=${r%% *}; M=${r#* }
  if [ "$N" -ne "$WANT" ]; then OK=0; echo "NOTE $1/$2 samples of $3: $N, want $WANT"; fi
}
verdict() { if [ "$OK" -eq 0 ]; then echo CNM; elif [ "$1" -eq 1 ]; then echo MET; else echo NOT-MET; fi; }

# GJ1, GJ3 and GJ4: jacquard against ark; GJ2 recorded.
for g in "GJ1 GJ1Scan ns/entity le" "GJ2 GJ2Bytes B/entity rec" "GJ3 GJ3Apply ns/deed le" "GJ4 GJ4Hydrate entities/s ge"; do
  set -- $g
  OK=1
  get "$2" jacquard "$3"; jm=$M
  get "$2" ark "$3"; am=$M
  case $4 in
    le) t="$3, jacquard at or below ark"; cmp "$jm" le "$am" && v=1 || v=0; v=$(verdict $v) ;;
    ge) t="$3, jacquard at or above ark"; cmp "$jm" ge "$am" && v=1 || v=0; v=$(verdict $v) ;;
    *) t="$3, recorded, no target"; if [ "$OK" -eq 1 ]; then v=RECORDED; else v=CNM; fi ;;
  esac
  gate "$1" "$t" "jacquard $jm, ark $am" "$v"
done

# G1: the typed read allocates 0 times; its time and the seam's are recorded.
OK=1
get G1TypedGet typed allocs/op; ta=$M
get G1TypedGet typed ns/op; tn=$M
get G1TypedGet seam allocs/op; sa=$M
get G1TypedGet seam ns/op; sn=$M
cmp "$ta" eq 0 && v=1 || v=0
gate G1 "allocs/op of the typed read 0, 1,000 resident entities; ns/op recorded" \
  "typed $ta allocs/op, $tn ns/op; seam into a caller-owned buffer $sa allocs/op, $sn ns/op" "$(verdict $v)"

# G2: every declared query allocates 0 times into a caller-owned Answer; ns/eid recorded per query.
OK=1 v=1 obs=""
for q in q0 q1 q2 q3 q4 q5 q6 q7; do
  get G2Query $q allocs/op; qa=$M
  get G2Query $q ns/eid; qn=$M
  cmp "$qa" eq 0 || v=0
  obs="$obs${obs:+; }$q $qa allocs/op $qn ns/eid"
done
gate G2 "allocs/op 0 for each declared query, 100,000 entities of an All kind and 1,000 of another, answers equal to the SQL twin; ns/eid recorded" \
  "$obs" "$(verdict $v)"

# G3: Apply allocates 0 times at steady state, and the bulk deed costs at most the single deeds per entity.
OK=1
get GJ3Apply jacquard allocs/op; aa=$M
get G3Bulk bulk ns/entity; bk=$M
get G3Bulk single ns/entity; sg=$M
v=1
cmp "$aa" eq 0 || v=0
cmp "$bk" le "$sg" || v=0
gate G3 "allocs/op of Apply of one deed of 8 sets 0, 100,000 entities; a bulk deed of 1,000 entities at or below the same ops as 1,000 deeds, ns/entity" \
  "Apply $aa allocs/op; bulk $bk ns/entity, single $sg ns/entity" "$(verdict $v)"

# G4: KindStats.Bytes over the heap growth from 0.9 to 1.1 on Jacquard; Ark recorded.
OK=1
get G4StatsBytes jacquard stats/heap; jm=$M
get G4StatsBytes ark stats/heap; am=$M
v=1
cmp "$jm" ge 0.9 || v=0
cmp "$jm" le 1.1 || v=0
gate G4 "stats/heap of jacquard from 0.9 to 1.1, 100,000 entities; ark recorded" "jacquard $jm, ark $am" "$(verdict $v)"

# G5: the scannable heap that 1,000,000 resident entities add, over their column bytes, below 0.01.
OK=1
get G5Scan jacquard scan/cols; jm=$M
cmp "$jm" lt 0.01 && v=1 || v=0
gate G5 "scan/cols of jacquard below 0.01, 1,000,000 entities of 8 pointer-free components of 8 bytes" "jacquard $jm" "$(verdict $v)"

# G6: the peak heap of a hydration, sampled with the collector on, at most 2 times the final heap growth; the bound
# with the collector off and Ark's value recorded; the rate is GJ4's.
OK=1
get G6PeakGC jacquard peakgc/resident; jg=$M
get G6HydratePeak jacquard peak/resident; jm=$M
get G6HydratePeak ark peak/resident; am=$M
cmp "$jg" le 2 && v=1 || v=0
gate G6 "peakgc/resident of jacquard at most 2 (the sampled high-water with the collector on), 100,000 entities; the bound with the collector off and ark recorded; the rate is GJ4's entities/s" \
  "jacquard $jg with the collector on; with it off jacquard $jm, ark $am" "$(verdict $v)"

[ "$RC" = 0 ] || echo "NOTE go test rc=$RC, the rc of each command: $(paste -s -d ' ' "$RCF" 2>/dev/null)"
echo "END $(date -u +%Y-%m-%dT%H:%M:%SZ) gates $((MET + NOTMET + REC + CNM)) met $MET not-met $NOTMET" \
  "recorded $REC cnm $CNM procs $PROCS"
if [ "$CNM" -ne 0 ] || [ "$RC" != 0 ]; then exit 3; fi
exit 0
