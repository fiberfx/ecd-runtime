# Group cfg18 of Entity Component Runtime: the refusals of the catalog constructor of package jacquard for a Budget
# below 0, for an Exact query whose All repeats a CID and for a query whose Kinds repeat a kind. The row J5S-02 of group
# j5s holds the fault cases of the two query refusals and of the Budget; the plants of the query refusals are in this
# group.
# Sourced by the rows harness from /w; the harness supplies say, sel, offline, head_of, count, why_of, go_row and
# HAVE_GO.
#
#   J5S-03   build, New and the reference model refuse a catalog with a kind whose Budget is below 0, an All kind and a
#            Touched kind, with an error that matches ecd.ErrOp and without an engine; build and New also wrap the
#            catalog error; all three build a Budget of 0 and of 1 (TestJ5SBudget)
#   J5S-03p  plant: build does not refuse a Budget below 0 (residency.go)
#   J5S-03q  plant: the reference model does not refuse a Budget below 0 (model/model.go)
#   J5S-02q  plant: an Exact query whose All repeats a CID is not refused (query.go; TestJ5SCatalog)
#   J5S-02r  plant: a query whose Kinds repeat a kind is not refused (query.go; TestJ5SCatalog)
#
# The rows J5S-03, J5S-03q and the two query cases of J5S-02 hold only with the Budget and query refusals of the
# reference model, go/engine/jacquard/model/model.go.
#
# cfg18_plant id expect test file sedexpr: copies /w/go to a temporary directory, applies the one-line sed to
# engine/jacquard/<file> and runs only the named test there. KILLED: the test failed, and the observed text names its
# first failing line. SURVIVED: it passed. CNM: the sed did not change exactly one line, the module was missing, or the
# test did not run.
cfg18_plant() {
  cfg18_id=$1 cfg18_ex=$2 cfg18_test=$3 cfg18_file=$4 cfg18_sed=$5
  sel "$cfg18_id" cfg18 || return 0
  if [ $HAVE_GO -eq 0 ]; then say "$cfg18_id" "$cfg18_ex" "go not on PATH" CNM; return 0; fi
  cfg18_pd=$(mktemp -d /tmp/rows/plant.XXXXXX)
  cp -a /w/go "$cfg18_pd/go" && cp "$cfg18_pd/go/engine/jacquard/$cfg18_file" "$cfg18_pd/orig.go"
  (cd "$cfg18_pd/go/engine/jacquard" && sed -i "$cfg18_sed" "$cfg18_file")
  cfg18_n=$(diff "$cfg18_pd/orig.go" "$cfg18_pd/go/engine/jacquard/$cfg18_file" | grep -c '^>')
  if [ "$cfg18_n" -ne 1 ]; then
    say "$cfg18_id" "$cfg18_ex" "the sed changed $cfg18_n lines of $cfg18_file, want 1" CNM
  else
    cfg18_log=/tmp/rows/$cfg18_id.log
    (cd "$cfg18_pd/go" && go test -count=1 -v -run "^$cfg18_test\$" ./engine/jacquard/ >"$cfg18_log" 2>&1); cfg18_rc=$?
    if offline "$cfg18_log"; then say "$cfg18_id" "$cfg18_ex" "module not found" CNM
    elif grep -q "^--- FAIL: $cfg18_test" "$cfg18_log"; then
      say "$cfg18_id" "$cfg18_ex" "rc=$cfg18_rc, $cfg18_test FAIL: $(why_of "$cfg18_log")" KILLED
    elif [ $cfg18_rc -eq 0 ] && grep -q "^--- PASS: $cfg18_test" "$cfg18_log"; then
      say "$cfg18_id" "$cfg18_ex" "rc=0, $cfg18_test PASS" SURVIVED
    else say "$cfg18_id" "$cfg18_ex" "rc=$cfg18_rc, the test did not run" CNM; head_of "$cfg18_log" 8; fi
  fi
  rm -rf "$cfg18_pd"
}

go_row J5S-03 cfg18 "TestJ5SBudget rc=0" /w/go test -run '^TestJ5SBudget$' ./engine/jacquard/
cfg18_plant J5S-03p "TestJ5SBudget fails on the plant" TestJ5SBudget residency.go 's/if d\.Budget < 0 {/if false {/'
cfg18_plant J5S-03q "TestJ5SBudget fails on the plant" TestJ5SBudget model/model.go 's/if k\.Budget < 0 {/if false {/'
cfg18_plant J5S-02q "TestJ5SCatalog fails on the plant" TestJ5SCatalog query.go \
  's/if def\.Exact && repeats(def\.All) {/if false {/'
cfg18_plant J5S-02r "TestJ5SCatalog fails on the plant" TestJ5SCatalog query.go 's/if repeats(def\.Kinds) {/if false {/'
