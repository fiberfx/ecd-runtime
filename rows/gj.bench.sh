#!/bin/sh
# Comparison benchmarks of Ecd Runtime: Jacquard against Ark on the gates GJ1 to GJ4, with G4 and G6 recorded. The
# script runs the benchmarks of package gj in go/enginecmp once, prints the raw output whole, takes the median of the
# samples of each metric per engine and prints one line per gate. It measures: a gate that is NOT-MET changes no exit
# code, and a gate that was not measured is CNM.
#
# Run command (flow:row-container, image mx-trafikist-toolchain:go1.27.4-rust1.100.0-r2, whose /usr/local/go is
# go1.27.4), on a box that runs no other container, since a second load changes the timings:
#   kan flow run flow:row-container --param script=rows/gj.bench.sh \
#     --param checkout=/Users/jonny/dev/ecd/ecd-runtime --param revision=SHA \
#     --param paths='go rows README.md LICENSE' --param name=gj \
#     --param image=mx-trafikist-toolchain:go1.27.4-rust1.100.0-r2 \
#     --param inputs=DIR \
#     --param cpus=2 --param memory=4g --param timeout=1800
# inputs is required and holds the GOPROXY file tree of the rows script: github.com/fiberfx/ecd-id/go and the Ark that
# go/enginecmp requires. Without it GOPROXY is off, the module does not load, and every gate is CNM.
#
#   gate  benchmark                metric         target
#   GJ1   BenchmarkGJ1Scan         ns/entity      jacquard at or below ark
#   GJ2   BenchmarkGJ2Bytes        B/entity       jacquard at or below ark
#   GJ3   BenchmarkGJ3Apply        ns/deed        jacquard at or below ark
#   GJ4   BenchmarkGJ4Hydrate      entities/s     jacquard at or above ark
#   G4    BenchmarkG4StatsBytes    stats/heap     recorded
#   G6    BenchmarkG6HydratePeak   peak/resident  recorded
#
# Each benchmark has the sub-benchmarks jacquard and ark and reports its metric with b.ReportMetric under the unit of
# the table; the script reads that unit and never ns/op. The reported name ends in -N, the GOMAXPROCS of the run, which
# the script drops. With -count=6 the two engines run in turn round by round, so a drift of the machine reaches both.
# The median of an engine is the middle value of its 6 samples sorted, and with an even count the mean of the two
# middle values. An engine with other than 6 samples of a metric leaves the gate CNM.
# The script does not use benchstat: the raw lines stay in the output so that a table can be made from them.
#
# Output: START, the TOOL line, the raw go test output, one line per gate
#   GATE id | target | observed jacquard X, ark Y | MET, NOT-MET, RECORDED or CNM
# with X and Y the medians to 4 decimals, and END. The raw output is also kept in /tmp/rows/gj.bench.out.
# Exit: 0 every gate measured, 3 any gate CNM or go test not rc=0 (a NOT-MET gate does not change the exit code).
set -u
cd /w || exit 3
mkdir -p /tmp/rows
export GOTOOLCHAIN=local GOWORK=off GOFLAGS=-modcacherw GOMODCACHE=/tmp/gomod GOCACHE=/tmp/gocache GOSUMDB=off
if [ -d /w/inputs ]; then export GOPROXY=file:///w/inputs; else export GOPROXY=off; fi
export PATH="/usr/local/go/bin:$PATH"
export LANG=C.UTF-8 LC_ALL=C.UTF-8 TMPDIR=/tmp
export GIT_CONFIG_NOSYSTEM=1 GIT_CONFIG_GLOBAL=/dev/null
WANT=6 # samples per engine and metric: the -count of the command below
OUT=/tmp/rows/gj.bench.out
RCF=/tmp/rows/gj.bench.rc
TAB=/tmp/rows/gj.bench.gates
VALS=/tmp/rows/gj.bench.vals
# The gates, one per line: id, benchmark, metric unit, and the target (le: jacquard at or below ark, ge: at or above,
# rec: recorded).
GATES='GJ1 BenchmarkGJ1Scan ns/entity le
GJ2 BenchmarkGJ2Bytes B/entity le
GJ3 BenchmarkGJ3Apply ns/deed le
GJ4 BenchmarkGJ4Hydrate entities/s ge
G4 BenchmarkG4StatsBytes stats/heap rec
G6 BenchmarkG6HydratePeak peak/resident rec'
printf '%s\n' "$GATES" >"$TAB"
rm -f "$RCF" "$VALS"

# median id engine: prints the number of samples and their median of the lines "id engine value" of $VALS, "0 -" with
# none.
median() {
  awk -v k="$1" -v e="$2" '$1 == k && $2 == e { print $3 }' "$VALS" | sort -n | awk '
    { v[NR] = $1 }
    END {
      if (NR == 0) { print 0, "-"; exit }
      if (NR % 2 == 1) m = v[(NR + 1) / 2]; else m = (v[NR / 2] + v[NR / 2 + 1]) / 2
      printf "%d %.4f\n", NR, m
    }'
}

echo "START $(date -u +%Y-%m-%dT%H:%M:%SZ) nproc $(nproc 2>/dev/null) gomaxprocs ${GOMAXPROCS:-unset}"
GOV=$(go version 2>&1) || GOV="go absent: $GOV"
echo "TOOL $GOV"

# The run: the one command. Its rc goes to a file, since a pipe to tee hides it.
( cd /w/go/enginecmp && go test -run '^$' -bench '^Benchmark(GJ[1-4]|G4|G6)' -benchmem -count=6 \
    -timeout 1500s ./gj/ 2>&1
  echo $? >"$RCF" ) | tee "$OUT"
RC=$(cat "$RCF" 2>/dev/null)
[ -n "$RC" ] || RC=none

# The samples: one line "id engine value" per result line that carries the unit of its benchmark.
awk '
  NR == FNR { id[$2] = $1; un[$2] = $3; next }
  $1 ~ /^Benchmark/ {
    name = $1
    sub(/-[0-9]+$/, "", name)
    if (split(name, p, "/") != 2 || !(p[1] in id) || (p[2] != "jacquard" && p[2] != "ark")) next
    for (i = 3; i < NF; i += 2)
      if ($(i + 1) == un[p[1]] && $i ~ /^[0-9]+(\.[0-9]+)?([eE][+-]?[0-9]+)?$/) { print id[p[1]], p[2], $i; break }
  }' "$TAB" "$OUT" >"$VALS"
PROCS=$(awk '$1 ~ /^Benchmark/ { p = $1; if (p ~ /-[0-9]+$/) sub(/^.*-/, "", p); else p = 1; print p; exit }' "$OUT")
[ -n "$PROCS" ] || PROCS=none

MET=0 NOTMET=0 REC=0 CNM=0
while read -r id bench unit mode; do
  r=$(median "$id" jacquard); jn=${r%% *}; jm=${r#* }
  r=$(median "$id" ark); an=${r%% *}; am=${r#* }
  case $mode in
    le) target="$unit, jacquard at or below ark" ;;
    ge) target="$unit, jacquard at or above ark" ;;
    *) target="$unit, recorded" ;;
  esac
  if [ "$jn" -ne "$WANT" ] || [ "$an" -ne "$WANT" ]; then
    v=CNM
    echo "NOTE $id samples of $unit: jacquard $jn, ark $an, want $WANT"
  elif [ "$mode" = rec ]; then
    v=RECORDED
  else
    v=$(awk -v j="$jm" -v a="$am" -v m="$mode" 'BEGIN {
      if (m == "le") ok = (j + 0 <= a + 0); else ok = (j + 0 >= a + 0)
      v = ok ? "MET" : "NOT-MET"
      print v }')
  fi
  case $v in
    MET) MET=$((MET + 1)) ;;
    NOT-MET) NOTMET=$((NOTMET + 1)) ;;
    RECORDED) REC=$((REC + 1)) ;;
    CNM) CNM=$((CNM + 1)) ;;
  esac
  echo "GATE $id | $target | observed jacquard $jm, ark $am | $v"
done <<EOF
$GATES
EOF

[ "$RC" = 0 ] || echo "NOTE go test rc=$RC"
echo "END $(date -u +%Y-%m-%dT%H:%M:%SZ) gates $((MET + NOTMET + REC + CNM)) met $MET not-met $NOTMET" \
  "recorded $REC cnm $CNM procs $PROCS"
if [ "$CNM" -ne 0 ] || [ "$RC" != 0 ]; then exit 3; fi
exit 0
