# Group fx19a of Ecd Runtime: the refusal of a catalog with two faults by the catalog constructors, and the comparison
# of a query whose All repeats a CID without Exact.
# Sourced by the rows harness from /w; the harness supplies say, sel, offline, head_of, count, why_of, go_row and
# HAVE_GO.
#
#   J5S-04   build, New and the reference model refuse a catalog with a Budget below 0 and a later kind declared
#            twice, without an engine (TestJ5SMultiFault); no plant: each constructor holds two refusals for this
#            catalog, and a one-line plant removes one
#   JC-02pq  plant: the engine refuses a query whose All repeats a CID without Exact (query.go; TestCompareQuery)
#   JC-02pm  plant: the reference model refuses a query whose All repeats a CID without Exact (model/model.go;
#            TestCompareQuery)
#
# fx19a_plant id expect test file sedexpr: copies /w/go to a temporary directory, applies the one-line sed to
# engine/jacquard/<file> and runs only the named test there. KILLED: the test failed, and the observed text names its
# first failing line. SURVIVED: it passed. CNM: the sed did not change exactly one line, the module was missing, or the
# test did not run.
fx19a_plant() {
  fx19a_id=$1 fx19a_ex=$2 fx19a_test=$3 fx19a_file=$4 fx19a_sed=$5
  sel "$fx19a_id" fx19a || return 0
  if [ $HAVE_GO -eq 0 ]; then say "$fx19a_id" "$fx19a_ex" "go not on PATH" CNM; return 0; fi
  fx19a_pd=$(mktemp -d /tmp/rows/plant.XXXXXX)
  cp -a /w/go "$fx19a_pd/go" && cp "$fx19a_pd/go/engine/jacquard/$fx19a_file" "$fx19a_pd/orig.go"
  (cd "$fx19a_pd/go/engine/jacquard" && sed -i "$fx19a_sed" "$fx19a_file")
  fx19a_n=$(diff "$fx19a_pd/orig.go" "$fx19a_pd/go/engine/jacquard/$fx19a_file" | grep -c '^>')
  if [ "$fx19a_n" -ne 1 ]; then
    say "$fx19a_id" "$fx19a_ex" "the sed changed $fx19a_n lines of $fx19a_file, want 1" CNM
  else
    fx19a_log=/tmp/rows/$fx19a_id.log
    (cd "$fx19a_pd/go" && go test -count=1 -v -run "^$fx19a_test\$" ./engine/jacquard/ >"$fx19a_log" 2>&1); fx19a_rc=$?
    if offline "$fx19a_log"; then say "$fx19a_id" "$fx19a_ex" "module not found" CNM
    elif grep -q "^--- FAIL: $fx19a_test" "$fx19a_log"; then
      say "$fx19a_id" "$fx19a_ex" "rc=$fx19a_rc, $fx19a_test FAIL: $(why_of "$fx19a_log")" KILLED
    elif [ $fx19a_rc -eq 0 ] && grep -q "^--- PASS: $fx19a_test" "$fx19a_log"; then
      say "$fx19a_id" "$fx19a_ex" "rc=0, $fx19a_test PASS" SURVIVED
    else say "$fx19a_id" "$fx19a_ex" "rc=$fx19a_rc, the test did not run" CNM; head_of "$fx19a_log" 8; fi
  fi
  rm -rf "$fx19a_pd"
}

go_row J5S-04 fx19a "TestJ5SMultiFault rc=0" /w/go test -run '^TestJ5SMultiFault$' ./engine/jacquard/
fx19a_plant JC-02pq "TestCompareQuery fails on the plant" TestCompareQuery query.go \
  's/if def\.Exact && repeats(def\.All) {/if repeats(def.All) {/'
fx19a_plant JC-02pm "TestCompareQuery fails on the plant" TestCompareQuery model/model.go \
  's/if q\.Exact && repeats(q\.All) {/if repeats(q.All) {/'
