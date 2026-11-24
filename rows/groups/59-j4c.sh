# Group j4c of Ecd Runtime: the node list of a declared query in the Engine of package jacquard.
# Sourced by the rows harness from /w; the harness supplies say, sel, offline, head_of, count, tree_of, go_row and HAVE_GO.
#
#   J03     the node list of every declared query equals a fresh match over the nodes that hold a table after each step of
#           four seeds of 2,000 random transactions: a node created after the declaration is announced, and a declaration
#           on an engine that holds tables starts from them (TestJ03)
#   J03p    plant: no table is announced (move.go)
#   J03pq   plant: the same, run against the comparison with the reference model (TestCompareQuery)
#   J03p1   plant: node 2 is never announced (query.go)
#   J03ps   plant: a declaration does not seed its node list from the existing tables (query.go)
#
# j4c_plant id expect test file sedexpr [flags]: copies /w/go to a temporary directory, applies the one-line sed to
# engine/jacquard/<file> and runs only the named test there. KILLED: the test failed. SURVIVED: it passed.
# CNM: the sed did not change exactly one line, the module was missing, or the test did not run.
j4c_plant() {
  j4c_id=$1 j4c_ex=$2 j4c_test=$3 j4c_file=$4 j4c_sed=$5 j4c_flags=${6:-}
  sel "$j4c_id" j4c || return 0
  if [ $HAVE_GO -eq 0 ]; then say "$j4c_id" "$j4c_ex" "go not on PATH" CNM; return 0; fi
  j4c_pd=$(mktemp -d /tmp/rows/plant.XXXXXX)
  cp -a /w/go "$j4c_pd/go" && cp "$j4c_pd/go/engine/jacquard/$j4c_file" "$j4c_pd/orig.go"
  (cd "$j4c_pd/go/engine/jacquard" && sed -i "$j4c_sed" "$j4c_file")
  j4c_n=$(diff "$j4c_pd/orig.go" "$j4c_pd/go/engine/jacquard/$j4c_file" | grep -c '^>')
  if [ "$j4c_n" -ne 1 ]; then
    say "$j4c_id" "$j4c_ex" "the sed changed $j4c_n lines of $j4c_file, want 1" CNM
  else
    j4c_log=/tmp/rows/$j4c_id.log
    (cd "$j4c_pd/go" && go test -count=1 -v $j4c_flags -run "^$j4c_test\$" ./engine/jacquard/ >"$j4c_log" 2>&1); j4c_rc=$?
    if offline "$j4c_log"; then say "$j4c_id" "$j4c_ex" "module not found" CNM
    elif grep -q "^--- FAIL: $j4c_test" "$j4c_log"; then say "$j4c_id" "$j4c_ex" "rc=$j4c_rc, $j4c_test FAIL" KILLED
    elif [ $j4c_rc -eq 0 ] && grep -q "^--- PASS: $j4c_test" "$j4c_log"; then say "$j4c_id" "$j4c_ex" "rc=0, $j4c_test PASS" SURVIVED
    else say "$j4c_id" "$j4c_ex" "rc=$j4c_rc, the test did not run" CNM; head_of "$j4c_log" 8; fi
  fi
  rm -rf "$j4c_pd"
}

go_row J03 j4c "TestJ03 rc=0" /w/go test -run '^TestJ03$' ./engine/jacquard/
j4c_plant J03p  "TestJ03 fails on the plant"          TestJ03          move.go  's/^\t\ten\.announce(node)$/\t\t_ = node/'
j4c_plant J03pq "TestCompareQuery fails on the plant" TestCompareQuery move.go  's/^\t\ten\.announce(node)$/\t\t_ = node/'
j4c_plant J03p1 "TestJ03 fails on the plant"          TestJ03          query.go 's/if q\.wants(a) {/if q.wants(a) \&\& node != 2 {/'
j4c_plant J03ps "TestJ03 fails on the plant"          TestJ03          query.go 's/^\tq\.nodes = en\.scan(q)$/\tq.nodes = nil/'
