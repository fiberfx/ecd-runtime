# Group mdl18 of Entity Component Runtime: the refusals of the reference model, package model of engine/jacquard.
# Sourced by the rows harness from /w; the harness supplies say, sel, offline, head_of, count, why_of, go_row and
# HAVE_GO.
#
#   J0M-07     a hydrated row whose value is not the Size of its component, or a tag's row with a value, is refused
#              with an error that matches ErrOp, on a Cold and a Warm All kind and on a Touched kind; the model does not
#              become stale and nothing changes (TestHydrateRows)
#   J0M-07-p   plant: the length check of a row of a component is skipped (model.go; TestHydrateRows)
#   J0M-07-pt  plant: the check of a tag's row with a value is skipped (model.go; TestHydrateRows)
#   J0M-08     New refuses a kind whose Budget is below 0, All or Touched, with an error that matches ErrOp, an Exact
#              query whose All repeats a CID and a query whose Kinds repeat a kind, and builds no engine; it builds a
#              Budget of 0 and a query that repeats a CID without Exact (TestCatalogRefusals)
#   J0M-08-pb  plant: the Budget check of a kind is skipped (model.go; TestCatalogRefusals)
#   J0M-08-pq  plant: the check of an Exact query whose All repeats a CID is skipped (model.go; TestCatalogRefusals)
#   J0M-08-pk  plant: the check of a query whose Kinds repeat a kind is skipped (model.go; TestCatalogRefusals)
#
# mdl18_plant id expect test sedexpr: copies /w/go to a temporary directory, applies the one-line sed to
# engine/jacquard/model/model.go and runs only the named test there. KILLED: the test failed, and the observed text
# names its first failing line. SURVIVED: it passed. CNM: the sed did not change exactly one line, the module was
# missing, or the test did not run.
mdl18_plant() {
  mdl18_id=$1 mdl18_ex=$2 mdl18_test=$3 mdl18_sed=$4
  sel "$mdl18_id" mdl18 || return 0
  if [ $HAVE_GO -eq 0 ]; then say "$mdl18_id" "$mdl18_ex" "go not on PATH" CNM; return 0; fi
  mdl18_pd=$(mktemp -d /tmp/rows/plant.XXXXXX)
  cp -a /w/go "$mdl18_pd/go" && cp "$mdl18_pd/go/engine/jacquard/model/model.go" "$mdl18_pd/orig.go"
  (cd "$mdl18_pd/go/engine/jacquard/model" && sed -i "$mdl18_sed" model.go)
  mdl18_n=$(diff "$mdl18_pd/orig.go" "$mdl18_pd/go/engine/jacquard/model/model.go" | grep -c '^>')
  if [ "$mdl18_n" -ne 1 ]; then
    say "$mdl18_id" "$mdl18_ex" "the sed changed $mdl18_n lines of model.go, want 1" CNM
  else
    mdl18_log=/tmp/rows/$mdl18_id.log
    (cd "$mdl18_pd/go" && go test -count=1 -v -run "^$mdl18_test\$" ./engine/jacquard/model/ >"$mdl18_log" 2>&1)
    mdl18_rc=$?
    if offline "$mdl18_log"; then say "$mdl18_id" "$mdl18_ex" "module not found" CNM
    elif grep -q "^--- FAIL: $mdl18_test" "$mdl18_log"; then
      say "$mdl18_id" "$mdl18_ex" "rc=$mdl18_rc, $mdl18_test FAIL: $(why_of "$mdl18_log")" KILLED
    elif [ $mdl18_rc -eq 0 ] && grep -q "^--- PASS: $mdl18_test" "$mdl18_log"; then
      say "$mdl18_id" "$mdl18_ex" "rc=0, $mdl18_test PASS" SURVIVED
    else say "$mdl18_id" "$mdl18_ex" "rc=$mdl18_rc, the test did not run" CNM; head_of "$mdl18_log" 8; fi
  fi
  rm -rf "$mdl18_pd"
}

go_row J0M-07 mdl18 "TestHydrateRows rc=0" /w/go test -run '^TestHydrateRows$' ./engine/jacquard/model/
mdl18_plant J0M-07-p "TestHydrateRows fails on the plant" TestHydrateRows \
  's/if def\.Form != ecd\.Tag && !def\.Pointers && len(r\.Value) != def\.Size {/if false {/'
mdl18_plant J0M-07-pt "TestHydrateRows fails on the plant" TestHydrateRows \
  's/if def\.Form == ecd\.Tag && len(r\.Value) != 0 {/if false {/'

go_row J0M-08 mdl18 "TestCatalogRefusals rc=0" /w/go test -run '^TestCatalogRefusals$' ./engine/jacquard/model/
mdl18_plant J0M-08-pb "TestCatalogRefusals fails on the plant" TestCatalogRefusals 's/if k\.Budget < 0 {/if false {/'
mdl18_plant J0M-08-pq "TestCatalogRefusals fails on the plant" TestCatalogRefusals \
  's/if q\.Exact && repeats(q\.All) {/if false {/'
mdl18_plant J0M-08-pk "TestCatalogRefusals fails on the plant" TestCatalogRefusals \
  's/if repeats(q\.Kinds) {/if false {/'
