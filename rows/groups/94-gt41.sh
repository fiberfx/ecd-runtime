# Group gt41 of Entity Component Runtime: the rows of the gates G1, G2, G3 and G5 of the contract
# (specs/ecd_runtime/ecd_runtime.specs.md#gates), which bind every engine, measured on Jacquard (story 41). Each row
# runs one test that measures its gate's target; its plant puts one fault in the measured path. The timed values of
# the gates are rows/gj.bench.rows.sh, which this group does not run.
# Sourced by the rows harness from /w; the harness supplies say, sel, offline, head_of, count, why_of, go_row and
# HAVE_GO.
#
#   G1-01    a point read hit through the typed path, llm.Turn.Get over ecd.Engine on Jacquard, allocates 0 times
#            per read (TestG1TypedGet in go/internal/llm)
#   G1-01p   plant: getValue of the generated code copies the bytes it read once more, one allocation more per read
#            (internal/llm/llm_gen.go)
#   G2-01    on Jacquard, each declared query of the store's catalog answers its SQL twin over 100,000 entities of
#            kind aaa and 1,000 of kind ccc, and then allocates 0 times into a caller-owned Answer (TestG2Query in
#            go/sqlite)
#   G2-01p   plant: Jacquard's query clips the caller's slice to its length before it appends, so the answer grows
#            in new memory; its eids stay equal to the twin's (engine/jacquard/query.go)
#   G3-01    Apply of one deed of 8 sets on a resident entity allocates 0 times per call at steady state
#            (TestG3Allocs in go/enginecmp/gj)
#   G3-01p   plant: inOrder copies deeds that are already in order, once in the check pass and once in the commit
#            pass of every Apply (engine/jacquard/engine.go)
#   G5-01    1,000,000 resident entities of 8 pointer-free components of 8 bytes add less than 1% of their column
#            bytes to /gc/scan/heap:bytes (TestG5Scan in go/enginecmp/gj)
#   G5-01p   plant: a column's array in a chunk's block is typed as pointers of the component's type, of the same
#            size, so the collector scans the block (engine/jacquard/table.go)
#
# Every test of this group logs its measured value as "GATEVAL <gate> <value>" (G2 once per query). A row is PASS
# when its test passes and FAIL when the target is not met.
#
# gt41_plant id expect mod pkg test file sedexpr: copies /w/go to a temporary directory and runs the named test of
# package pkg in module directory go/<mod> there twice, first as it is (the base value) and then after the one-line
# sed on go/<file> (the planted value); each value is the largest GATEVAL of its log. KILLED: the planted test failed
# and its value is above the base value, so the fault moved the measure the wrong way whether the base meets the
# target or not. SURVIVED: the planted test passed, or its value is not above the base. CNM: the sed did not change
# exactly one line, the file or a module was missing, a test panicked or logged no GATEVAL, or a test did not run,
# which includes a mutant that does not compile.
# ECDRT_GATES=1 makes the gate tests run; they skip without it. It is set for this group only.
ECDRT_GATES=1
export ECDRT_GATES
gt41_val() { # log: the largest GATEVAL value of the log, empty when it has none
  awk '$0 ~ /GATEVAL / { for (i = 1; i < NF; i++) if ($i == "GATEVAL") { v = $(i + 2); if (n == 0 || v + 0 > m + 0) m = v; n++ } }
       END { if (n > 0) print m }' "$1"
}
gt41_plant() {
  gt41_id=$1 gt41_ex=$2 gt41_mod=$3 gt41_pkg=$4 gt41_test=$5 gt41_file=$6 gt41_sed=$7
  sel "$gt41_id" gt41 || return 0
  if [ $HAVE_GO -eq 0 ]; then say "$gt41_id" "$gt41_ex" "go not on PATH" CNM; return 0; fi
  gt41_pd=$(mktemp -d /tmp/rows/plant.XXXXXX)
  cp -a /w/go "$gt41_pd/go"
  gt41_f=$gt41_pd/go/$gt41_file
  if [ ! -f "$gt41_f" ]; then
    say "$gt41_id" "$gt41_ex" "go/$gt41_file is absent" CNM
    rm -rf "$gt41_pd"
    return 0
  fi
  gt41_blog=/tmp/rows/$gt41_id.base.log
  gt41_log=/tmp/rows/$gt41_id.log
  (cd "$gt41_pd/go/$gt41_mod" && go test -count=1 -v -timeout 900s -run "^$gt41_test\$" "$gt41_pkg" >"$gt41_blog" 2>&1)
  gt41_bv=$(gt41_val "$gt41_blog")
  cp "$gt41_f" "$gt41_pd/orig.go"
  sed -i "$gt41_sed" "$gt41_f"
  gt41_n=$(diff "$gt41_pd/orig.go" "$gt41_f" | grep -c '^>')
  if [ "$gt41_n" -ne 1 ]; then
    say "$gt41_id" "$gt41_ex" "the sed changed $gt41_n lines of go/$gt41_file, want 1" CNM
  elif offline "$gt41_blog"; then say "$gt41_id" "$gt41_ex" "module not found" CNM
  elif grep -q '^panic:' "$gt41_blog" || [ -z "$gt41_bv" ]; then
    say "$gt41_id" "$gt41_ex" "the base run of $gt41_test logged no GATEVAL or panicked: $(why_of "$gt41_blog")" CNM
  else
    (cd "$gt41_pd/go/$gt41_mod" && go test -count=1 -v -timeout 900s -run "^$gt41_test\$" "$gt41_pkg" >"$gt41_log" 2>&1)
    gt41_rc=$?
    gt41_pv=$(gt41_val "$gt41_log")
    if offline "$gt41_log"; then say "$gt41_id" "$gt41_ex" "module not found" CNM
    elif grep -q '^panic:' "$gt41_log"; then
      say "$gt41_id" "$gt41_ex" "rc=$gt41_rc, $gt41_test panicked: $(why_of "$gt41_log")" CNM
    elif [ -z "$gt41_pv" ]; then
      say "$gt41_id" "$gt41_ex" "rc=$gt41_rc, the planted $gt41_test logged no GATEVAL (no test ran)" CNM
    elif grep -q "^--- FAIL: $gt41_test " "$gt41_log" && awk -v p="$gt41_pv" -v b="$gt41_bv" 'BEGIN { exit !(p + 0 > b + 0) }'; then
      say "$gt41_id" "$gt41_ex" "rc=$gt41_rc, $gt41_test FAIL, value $gt41_pv above the base $gt41_bv" KILLED
    else
      say "$gt41_id" "$gt41_ex" "rc=$gt41_rc, value $gt41_pv, base $gt41_bv" SURVIVED
    fi
  fi
  rm -rf "$gt41_pd"
}

go_row G1-01 gt41 "TestG1TypedGet rc=0" /w/go test -run '^TestG1TypedGet$' ./internal/llm/
gt41_plant G1-01p "KILLED" . ./internal/llm/ TestG1TypedGet internal/llm/llm_gen.go \
  '/^func getValue\[/,/^}/s/^\(\t\)b, err := e\.Get(id, c, nil)$/\1b, err := e.Get(id, c, nil); b = append([]byte(nil), b...)/'
go_row G2-01 gt41 "TestG2Query rc=0" /w/go/sqlite test -run '^TestG2Query$' .
gt41_plant G2-01p "KILLED" sqlite . TestG2Query engine/jacquard/query.go \
  's/^\(\t\)start := len(out\.IDs)$/\1start := len(out.IDs); out.IDs = slices.Clip(out.IDs)/'
go_row G3-01 gt41 "TestG3Allocs rc=0" /w/go/enginecmp test -run '^TestG3Allocs$' ./gj/
gt41_plant G3-01p "KILLED" enginecmp ./gj/ TestG3Allocs engine/jacquard/engine.go \
  's/^\(\t\)if slices\.IsSortedFunc(deeds, cmp) {$/\1if false \&\& slices.IsSortedFunc(deeds, cmp) {/'
go_row G5-01 gt41 "TestG5Scan rc=0" /w/go/enginecmp test -run '^TestG5Scan$' ./gj/
gt41_plant G5-01p "KILLED" enginecmp ./gj/ TestG5Scan engine/jacquard/table.go \
  's/Type: reflect\.ArrayOf(n, d\.typ)}/Type: reflect.ArrayOf(n, reflect.PointerTo(d.typ))}/'
unset ECDRT_GATES
