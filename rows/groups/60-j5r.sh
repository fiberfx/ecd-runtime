# Group j5r of Ecd Runtime: the residency answers of the Engine of package jacquard, checked against the reference model.
# Sourced by the rows harness from /w; the harness supplies say, sel, offline, head_of, count, why_of, go_row and
# HAVE_GO.
#
#   J5R-01   Apply and Get of an Engine with a residency declaration answer as the reference model does, on a Cold All
#            kind, a Warm All kind and a Touched kind: a Set, Unset or Destroy of a Touched entity that is not resident
#            changes nothing, a Spawn makes it resident, a Get answers ErrCold, ErrMiss or ErrAbsent by kind and
#            residency, and seeded random transactions read alike (TestJ5RResidency)
#   J5R-02   an op that changes nothing never makes the Engine stale and creates no entity, graph node, table or row:
#            a Set, Unset or Destroy of a Touched entity that is not resident, and every op on a Cold kind
#            (TestJ5RNoChange)
#   J5R-03   every declared query answers as the reference model does for each mix of Cold and Warm All kinds, in the
#            eids, their order and Count, and in the order of refusals: ErrStale, unknown QID, argument, Touched or
#            undeclared kind, Cold kind (TestJ5RQuery)
#   J5R-04   every refusal of the check pass matches ecd.ErrOp, on a Warm, a Cold and a Touched kind, and leaves the
#            Engine unchanged (TestJ5RErrOp)
#   J5R-01p  plant: a Touched entity that is not resident is not skipped (residency.go)
#   J5R-01pc plant: a Cold kind is not skipped (residency.go)
#   J5R-02p  plant: a Get of a Touched entity that is not resident answers ErrAbsent (engine.go)
#   J5R-03p  plant: a query on a Cold kind is answered (query.go)
#   J5R-03pt plant: a query with no kinds runs on the nodes of a Touched kind (query.go)
#   J5R-04p  plant: a refusal of the check pass does not match ecd.ErrOp (engine.go)
#
# j5r_plant id expect test file sedexpr [flags]: copies /w/go to a temporary directory, applies the one-line sed to
# engine/jacquard/<file> and runs only the named test there. KILLED: the test failed. SURVIVED: it passed.
# CNM: the sed did not change exactly one line, the module was missing, or the test did not run.
j5r_plant() {
  j5r_id=$1 j5r_ex=$2 j5r_test=$3 j5r_file=$4 j5r_sed=$5 j5r_flags=${6:-}
  sel "$j5r_id" j5r || return 0
  if [ $HAVE_GO -eq 0 ]; then say "$j5r_id" "$j5r_ex" "go not on PATH" CNM; return 0; fi
  j5r_pd=$(mktemp -d /tmp/rows/plant.XXXXXX)
  cp -a /w/go "$j5r_pd/go" && cp "$j5r_pd/go/engine/jacquard/$j5r_file" "$j5r_pd/orig.go"
  (cd "$j5r_pd/go/engine/jacquard" && sed -i "$j5r_sed" "$j5r_file")
  j5r_n=$(diff "$j5r_pd/orig.go" "$j5r_pd/go/engine/jacquard/$j5r_file" | grep -c '^>')
  if [ "$j5r_n" -ne 1 ]; then
    say "$j5r_id" "$j5r_ex" "the sed changed $j5r_n lines of $j5r_file, want 1" CNM
  else
    j5r_log=/tmp/rows/$j5r_id.log
    (cd "$j5r_pd/go" && go test -count=1 -v $j5r_flags -run "^$j5r_test\$" ./engine/jacquard/ >"$j5r_log" 2>&1); j5r_rc=$?
    if offline "$j5r_log"; then say "$j5r_id" "$j5r_ex" "module not found" CNM
    elif grep -q "^--- FAIL: $j5r_test" "$j5r_log"; then
      say "$j5r_id" "$j5r_ex" "rc=$j5r_rc, $j5r_test FAIL: $(why_of "$j5r_log")" KILLED
    elif [ $j5r_rc -eq 0 ] && grep -q "^--- PASS: $j5r_test" "$j5r_log"; then say "$j5r_id" "$j5r_ex" "rc=0, $j5r_test PASS" SURVIVED
    else say "$j5r_id" "$j5r_ex" "rc=$j5r_rc, the test did not run" CNM; head_of "$j5r_log" 8; fi
  fi
  rm -rf "$j5r_pd"
}

go_row J5R-01 j5r "TestJ5RResidency rc=0" /w/go test -run '^TestJ5RResidency$' ./engine/jacquard/
go_row J5R-02 j5r "TestJ5RNoChange rc=0"  /w/go test -run '^TestJ5RNoChange$' ./engine/jacquard/
go_row J5R-03 j5r "TestJ5RQuery rc=0"     /w/go test -run '^TestJ5RQuery$' ./engine/jacquard/
go_row J5R-04 j5r "TestJ5RErrOp rc=0"     /w/go test -run '^TestJ5RErrOp$' ./engine/jacquard/
j5r_plant J5R-01p  "TestJ5RResidency fails on the plant" TestJ5RResidency residency.go 's/return r\.cold || (r\.touched \&\& !present \&\& !spawn)/return r.cold/'
j5r_plant J5R-01pc "TestJ5RNoChange fails on the plant"  TestJ5RNoChange  residency.go 's/return r\.cold || (r\.touched/return (r.touched/'
j5r_plant J5R-02p  "TestJ5RResidency fails on the plant" TestJ5RResidency engine.go    's/if en\.touchedKind(ki) {/if false {/'
j5r_plant J5R-03p  "TestJ5RQuery fails on the plant"     TestJ5RQuery     query.go     's/if en\.coldKind(ki) {/if false {/'
j5r_plant J5R-04p  "TestJ5RErrOp fails on the plant"     TestJ5RErrOp     engine.go    's/^var errOp = fmt\.Errorf("jacquard: op refused: %w", ecd\.ErrOp)$/var errOp = errors.New("jacquard: op refused")/'
j5r_plant J5R-03pt "TestJ5RQuery fails on the plant"     TestJ5RQuery     query.go     's/return q\.wants(a) \&\& (len(q\.kinds) > 0 || !en\.touchedKind(a\.kind))/return q.wants(a)/'
