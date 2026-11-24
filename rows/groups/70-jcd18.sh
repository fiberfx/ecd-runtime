# Group jcd18 of Ecd Runtime: the plants of the random comparison of the Engine of package jacquard with the reference
# model that concern a hydrated row of the wrong length and the sentinels of an Apply error.
# Sourced by the rows harness from /w; the harness supplies say, sel, offline, head_of, count, why_of, go_row and
# HAVE_GO.
#
# TestCompare (JC-01, group jcmp) refuses a hydration for a row whose value has the wrong length and requires the
# answer ErrOp from the Engine and from the model, and it requires the sentinels of package ecd that each Apply error
# matches to be the same set for both. TestCompareSeam (JC-05, group jcmp5) refuses such a row of a Cold kind with the
# same answer. TestJ5HRefused (J5H-02, group j5h) compares the model on the four hydrations of the wrong length. The
# rows themselves run in their own groups; this group holds the plants that target them. The plants on model.go
# act on the check of the length of a hydrated value that the model states.
#
#   JC-01ph  plant: the Engine accepts a hydrated row of the wrong length (hydrate.go; TestCompare)
#   JC-01pn  plant: the model accepts a hydrated row of a component whose value is not its Size (model/model.go;
#            TestCompare)
#   JC-01pa  plant: the Engine answers a Set of an entity that was never spawned with a cause that matches no
#            sentinel, so the error matches ErrStale alone (engine.go; TestCompare)
#   JC-01pe  plant: the Engine answers a Spawn of a held entity with a cause that also matches ErrAbsent (engine.go;
#            TestCompare)
#   JC-05h   plant: the Engine accepts a hydrated row of the wrong length (hydrate.go; TestCompareSeam)
#   J5H-02q  plant: the model accepts a hydrated row of a component whose value is not its Size (model/model.go;
#            TestJ5HRefused)
#
# jcd18_plant id expect test file sedexpr: copies /w/go to a temporary directory, applies the one-line sed to
# engine/jacquard/<file> and runs only the named test there. KILLED: the test failed, and the observed text names its
# first failing line. SURVIVED: it passed. CNM: the sed did not change exactly one line, the module was missing, or the
# test did not run.
jcd18_plant() {
  jcd18_id=$1 jcd18_ex=$2 jcd18_test=$3 jcd18_file=$4 jcd18_sed=$5
  sel "$jcd18_id" jcd18 || return 0
  if [ $HAVE_GO -eq 0 ]; then say "$jcd18_id" "$jcd18_ex" "go not on PATH" CNM; return 0; fi
  jcd18_pd=$(mktemp -d /tmp/rows/plant.XXXXXX)
  cp -a /w/go "$jcd18_pd/go" && cp "$jcd18_pd/go/engine/jacquard/$jcd18_file" "$jcd18_pd/orig.go"
  (cd "$jcd18_pd/go/engine/jacquard" && sed -i "$jcd18_sed" "$jcd18_file")
  jcd18_n=$(diff "$jcd18_pd/orig.go" "$jcd18_pd/go/engine/jacquard/$jcd18_file" | grep -c '^>')
  if [ "$jcd18_n" -ne 1 ]; then
    say "$jcd18_id" "$jcd18_ex" "the sed changed $jcd18_n lines of $jcd18_file, want 1" CNM
  else
    jcd18_log=/tmp/rows/$jcd18_id.log
    (cd "$jcd18_pd/go" &&
      go test -count=1 -v -timeout 900s -run "^$jcd18_test\$" ./engine/jacquard/ >"$jcd18_log" 2>&1); jcd18_rc=$?
    if offline "$jcd18_log"; then say "$jcd18_id" "$jcd18_ex" "module not found" CNM
    elif grep -q "^--- FAIL: $jcd18_test" "$jcd18_log"; then
      say "$jcd18_id" "$jcd18_ex" "rc=$jcd18_rc, $jcd18_test FAIL: $(why_of "$jcd18_log")" KILLED
    elif [ $jcd18_rc -eq 0 ] && grep -q "^--- PASS: $jcd18_test" "$jcd18_log"; then
      say "$jcd18_id" "$jcd18_ex" "rc=0, $jcd18_test PASS" SURVIVED
    else say "$jcd18_id" "$jcd18_ex" "rc=$jcd18_rc, the test did not run" CNM; head_of "$jcd18_log" 8; fi
  fi
  rm -rf "$jcd18_pd"
}

jcd18_plant JC-01ph "TestCompare fails on the plant" TestCompare hydrate.go \
  's/if len(r\.Value) != cd\.size {/if false {/'
jcd18_plant JC-01pn "TestCompare fails on the plant" TestCompare model/model.go \
  's/if def\.Form != ecd\.Tag && !def\.Pointers && len(r\.Value) != def\.Size {/if false {/'
jcd18_plant JC-01pa "TestCompare fails on the plant" TestCompare engine.go \
  's/op\.Entity, ecd\.ErrAbsent)$/op.Entity, errExists)/'
jcd18_plant JC-01pe "TestCompare fails on the plant" TestCompare engine.go \
  's/spawn of %s: %w", op\.Entity, errExists)/spawn of %s: %w: %w", op.Entity, errExists, ecd.ErrAbsent)/'
jcd18_plant JC-05h "TestCompareSeam fails on the plant" TestCompareSeam hydrate.go \
  's/if len(r\.Value) != cd\.size {/if false {/'
jcd18_plant J5H-02q "TestJ5HRefused fails on the plant" TestJ5HRefused model/model.go \
  's/if def\.Form != ecd\.Tag && !def\.Pointers && len(r\.Value) != def\.Size {/if false {/'
