# Group j5s of Entity Component Runtime: the seam methods and the catalog constructor of the Engine of package jacquard.
# Sourced by the rows harness from /w; the harness supplies say, sel, offline, head_of, count, why_of, go_row and
# HAVE_GO.
#
#   J5S-01   an Engine built from a catalog reports the name "jacquard" and answers At, State, Apply, Get, Query and Stats
#            as the reference model does, on a Cold All kind, a Warm All kind and a Touched kind; a stale Engine and a
#            closed Engine answer ErrStale to every call and Stale for every kind (TestJ5SSeam)
#   J5S-02   the catalog constructor refuses each catalog that the reference model refuses, one fault per case, with an
#            error that wraps the catalog error and without an Engine: a duplicate CID, CID 0, a duplicate kind, a tag
#            with a Size, a Component or an Edge with a Size of 0 or less, an unknown form, an unknown residency and a
#            duplicate QID, a kind whose Budget is below 0, an Exact query whose All repeats a CID and a query whose
#            Kinds repeat a kind; it builds the Edge and pointer-holding forms and refuses a Set of the latter
#            (TestJ5SCatalog)
#   J5S-01p  plant: Close does not mark the Engine closed (seam.go)
#   J5S-02p  plant: a duplicate CID is not refused (engine.go)
#
# j5s_plant id expect test file sedexpr [flags]: copies /w/go to a temporary directory, applies the one-line sed to
# engine/jacquard/<file> and runs only the named test there. KILLED: the test failed. SURVIVED: it passed.
# CNM: the sed did not change exactly one line, the module was missing, or the test did not run.
j5s_plant() {
  j5s_id=$1 j5s_ex=$2 j5s_test=$3 j5s_file=$4 j5s_sed=$5 j5s_flags=${6:-}
  sel "$j5s_id" j5s || return 0
  if [ $HAVE_GO -eq 0 ]; then say "$j5s_id" "$j5s_ex" "go not on PATH" CNM; return 0; fi
  j5s_pd=$(mktemp -d /tmp/rows/plant.XXXXXX)
  cp -a /w/go "$j5s_pd/go" && cp "$j5s_pd/go/engine/jacquard/$j5s_file" "$j5s_pd/orig.go"
  (cd "$j5s_pd/go/engine/jacquard" && sed -i "$j5s_sed" "$j5s_file")
  j5s_n=$(diff "$j5s_pd/orig.go" "$j5s_pd/go/engine/jacquard/$j5s_file" | grep -c '^>')
  if [ "$j5s_n" -ne 1 ]; then
    say "$j5s_id" "$j5s_ex" "the sed changed $j5s_n lines of $j5s_file, want 1" CNM
  else
    j5s_log=/tmp/rows/$j5s_id.log
    (cd "$j5s_pd/go" && go test -count=1 -v $j5s_flags -run "^$j5s_test\$" ./engine/jacquard/ >"$j5s_log" 2>&1); j5s_rc=$?
    if offline "$j5s_log"; then say "$j5s_id" "$j5s_ex" "module not found" CNM
    elif grep -q "^--- FAIL: $j5s_test" "$j5s_log"; then
      say "$j5s_id" "$j5s_ex" "rc=$j5s_rc, $j5s_test FAIL: $(why_of "$j5s_log")" KILLED
    elif [ $j5s_rc -eq 0 ] && grep -q "^--- PASS: $j5s_test" "$j5s_log"; then say "$j5s_id" "$j5s_ex" "rc=0, $j5s_test PASS" SURVIVED
    else say "$j5s_id" "$j5s_ex" "rc=$j5s_rc, the test did not run" CNM; head_of "$j5s_log" 8; fi
  fi
  rm -rf "$j5s_pd"
}

go_row J5S-01 j5s "TestJ5SSeam rc=0"    /w/go test -run '^TestJ5SSeam$' ./engine/jacquard/
go_row J5S-02 j5s "TestJ5SCatalog rc=0" /w/go test -run '^TestJ5SCatalog$' ./engine/jacquard/
j5s_plant J5S-01p "TestJ5SSeam fails on the plant"    TestJ5SSeam    seam.go   's/^\ten\.stale = true$/\t_ = en/'
j5s_plant J5S-02p "TestJ5SCatalog fails on the plant" TestJ5SCatalog engine.go 's/if _, dup := en\.byCID\[c\.cid\]; dup {/if false {/'
