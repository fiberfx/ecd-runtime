# Group j5b of Entity Component Runtime: the per-kind statistics of the Engine of package jacquard.
# Sourced by the rows harness from /w; the harness supplies say, sel, offline, head_of, count, why_of, go_row and
# HAVE_GO.
#
#   J5B-01   after four seeds of 1,500 random transactions over three kinds, the Entities, Archetypes and Nodes of every
#            kind equal a recount from the records and the graph, Bytes and PointerBytes equal the formula over the
#            allocated chunks plus the fixed index cost per entity, and the fixed cases give the stated numbers
#            (TestJ5BStats)
#   J5B-02   a table emptied by destroys or by a move is no active archetype, and the numbers of one kind never include
#            the tables of another kind (TestJ5BStatsKinds)
#   J5B-01p  plant: the node filter by kind is dropped, so every kind counts every table (bytes.go)
#   J5B-02p  plant: an archetype counts when it has a table, not when the table holds a row (bytes.go)
#
# j5b_plant id expect test file sedexpr [flags]: copies /w/go to a temporary directory, applies the one-line sed to
# engine/jacquard/<file> and runs only the named test there. KILLED: the test failed. SURVIVED: it passed.
# CNM: the sed did not change exactly one line, the module was missing, or the test did not run.
j5b_plant() {
  j5b_id=$1 j5b_ex=$2 j5b_test=$3 j5b_file=$4 j5b_sed=$5 j5b_flags=${6:-}
  sel "$j5b_id" j5b || return 0
  if [ $HAVE_GO -eq 0 ]; then say "$j5b_id" "$j5b_ex" "go not on PATH" CNM; return 0; fi
  j5b_pd=$(mktemp -d /tmp/rows/plant.XXXXXX)
  cp -a /w/go "$j5b_pd/go" && cp "$j5b_pd/go/engine/jacquard/$j5b_file" "$j5b_pd/orig.go"
  (cd "$j5b_pd/go/engine/jacquard" && sed -i "$j5b_sed" "$j5b_file")
  j5b_n=$(diff "$j5b_pd/orig.go" "$j5b_pd/go/engine/jacquard/$j5b_file" | grep -c '^>')
  if [ "$j5b_n" -ne 1 ]; then
    say "$j5b_id" "$j5b_ex" "the sed changed $j5b_n lines of $j5b_file, want 1" CNM
  else
    j5b_log=/tmp/rows/$j5b_id.log
    (cd "$j5b_pd/go" && go test -count=1 -v $j5b_flags -run "^$j5b_test\$" ./engine/jacquard/ >"$j5b_log" 2>&1); j5b_rc=$?
    if offline "$j5b_log"; then say "$j5b_id" "$j5b_ex" "module not found" CNM
    elif grep -q "^--- FAIL: $j5b_test" "$j5b_log"; then
      say "$j5b_id" "$j5b_ex" "rc=$j5b_rc, $j5b_test FAIL: $(why_of "$j5b_log")" KILLED
    elif [ $j5b_rc -eq 0 ] && grep -q "^--- PASS: $j5b_test" "$j5b_log"; then say "$j5b_id" "$j5b_ex" "rc=0, $j5b_test PASS" SURVIVED
    else say "$j5b_id" "$j5b_ex" "rc=$j5b_rc, the test did not run" CNM; head_of "$j5b_log" 8; fi
  fi
  rm -rf "$j5b_pd"
}

go_row J5B-01 j5b "TestJ5BStats rc=0" /w/go test -run '^TestJ5BStats$' ./engine/jacquard/
go_row J5B-02 j5b "TestJ5BStatsKinds rc=0" /w/go test -run '^TestJ5BStatsKinds$' ./engine/jacquard/
j5b_plant J5B-01p "TestJ5BStatsKinds fails on the plant" TestJ5BStatsKinds bytes.go 's/if a\.kind != ki {/if a.kind != ki \&\& false {/'
j5b_plant J5B-02p "TestJ5BStatsKinds fails on the plant" TestJ5BStatsKinds bytes.go 's/if t\.len() > 0 {/if t != nil {/'
