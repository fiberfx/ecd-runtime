# Group jcmp5 of Entity Component Runtime: the comparison of the Engine of package jacquard with the reference model
# through the seam, with Cold and Touched kinds.
# Sourced by the rows harness from /w; the harness supplies say, sel, offline, head_of, count, why_of, go_row and
# HAVE_GO.
#
# TestCompare (JC-01, group jcmp) and TestCompareQuery (JC-02, group jcmpq) drive the Engine that build returns and the
# model through ecd.Engine on a catalog with two All kinds that start Cold and one Touched kind, with the hydration of
# an All kind and of Touched entities from a store, and the eviction of Touched entities, among the transactions. The
# plants of this group target the lines that decide the residency answers and run those two tests.
#
#   JC-05    one short history for each difference that the comparison closes, compared call by call through the seam
#            methods: every refusal of the check pass and of a hydrated row matches ecd.ErrOp; an op on a Cold kind
#            and a Set, Unset or Destroy of a Touched entity that is not resident are accepted, change nothing and
#            reach the store; Get answers ErrCold, ErrMiss and ErrAbsent as the model does; Hydrate and Evict, a
#            refused hydration and Close leave the Engine and the model in the same state (TestCompareSeam)
#   JC-06    a query on the Touched kind answers ErrQuery, a query that runs on a Cold kind answers ErrCold, neither
#            appends an eid or changes Count even when another kind of the query is Warm, a Get of a Touched entity
#            after an Evict answers ErrMiss, and an Evict of an All kind is refused and changes nothing
#            (TestCompareSeamRefusals)
#   JC-01pc  plant: an op on a Cold kind is not skipped (residency.go; TestCompare)
#   JC-01pm  plant: a Get of a Touched entity that is not resident answers ErrAbsent, not ErrMiss (engine.go;
#            TestCompare)
#   JC-01pu  plant: a Set, Unset or Destroy of a Touched entity that is not resident is not skipped (residency.go;
#            TestCompare)
#   JC-01pk  plant: a Get on a Cold kind answers no ErrCold (engine.go; TestCompare)
#   JC-02pc  plant: a query on a Cold kind answers no ErrCold (query.go; TestCompareQuery)
#   JC-02pt  plant: a query with no kinds runs on the nodes of the Touched kind (query.go; TestCompareQuery)
#   JC-05p   plant: the refusal of the check pass does not match ecd.ErrOp (engine.go; TestCompareSeam)
#
# jc5_plant id expect test file sedexpr [flags]: copies /w/go to a temporary directory, applies the one-line sed to
# engine/jacquard/<file> and runs only the named test there. KILLED: the test failed. SURVIVED: it passed.
# CNM: the sed did not change exactly one line, the module was missing, or the test did not run.
jc5_plant() {
  jc5_id=$1 jc5_ex=$2 jc5_test=$3 jc5_file=$4 jc5_sed=$5 jc5_flags=${6:-}
  sel "$jc5_id" jcmp5 || return 0
  if [ $HAVE_GO -eq 0 ]; then say "$jc5_id" "$jc5_ex" "go not on PATH" CNM; return 0; fi
  jc5_pd=$(mktemp -d /tmp/rows/plant.XXXXXX)
  cp -a /w/go "$jc5_pd/go" && cp "$jc5_pd/go/engine/jacquard/$jc5_file" "$jc5_pd/orig.go"
  (cd "$jc5_pd/go/engine/jacquard" && sed -i "$jc5_sed" "$jc5_file")
  jc5_n=$(diff "$jc5_pd/orig.go" "$jc5_pd/go/engine/jacquard/$jc5_file" | grep -c '^>')
  if [ "$jc5_n" -ne 1 ]; then
    say "$jc5_id" "$jc5_ex" "the sed changed $jc5_n lines of $jc5_file, want 1" CNM
  else
    jc5_log=/tmp/rows/$jc5_id.log
    (cd "$jc5_pd/go" && go test -count=1 -v $jc5_flags -run "^$jc5_test\$" ./engine/jacquard/ >"$jc5_log" 2>&1); jc5_rc=$?
    if offline "$jc5_log"; then say "$jc5_id" "$jc5_ex" "module not found" CNM
    elif grep -q "^--- FAIL: $jc5_test" "$jc5_log"; then
      say "$jc5_id" "$jc5_ex" "rc=$jc5_rc, $jc5_test FAIL: $(why_of "$jc5_log")" KILLED
    elif [ $jc5_rc -eq 0 ] && grep -q "^--- PASS: $jc5_test" "$jc5_log"; then say "$jc5_id" "$jc5_ex" "rc=0, $jc5_test PASS" SURVIVED
    else say "$jc5_id" "$jc5_ex" "rc=$jc5_rc, the test did not run" CNM; head_of "$jc5_log" 8; fi
  fi
  rm -rf "$jc5_pd"
}

go_row JC-05 jcmp5 "TestCompareSeam rc=0"         /w/go test -run '^TestCompareSeam$' ./engine/jacquard/
go_row JC-06 jcmp5 "TestCompareSeamRefusals rc=0" /w/go test -run '^TestCompareSeamRefusals$' ./engine/jacquard/
jc5_plant JC-01pc "TestCompare fails on the plant" TestCompare residency.go 's/return r\.cold || (r\.touched/return (r.touched/'
jc5_plant JC-01pm "TestCompare fails on the plant" TestCompare engine.go 's/if en\.touchedKind(ki) {/if false {/'
jc5_plant JC-02pc "TestCompareQuery fails on the plant" TestCompareQuery query.go 's/if en\.coldKind(ki) {/if false {/'
jc5_plant JC-02pt "TestCompareQuery fails on the plant" TestCompareQuery query.go 's/return q\.wants(a) && (len(q\.kinds) > 0 || !en\.touchedKind(a\.kind))/return q.wants(a)/'
jc5_plant JC-05p "TestCompareSeam fails on the plant" TestCompareSeam engine.go 's/^var errOp = fmt\.Errorf("jacquard: op refused: %w", ecd\.ErrOp)$/var errOp = errors.New("jacquard: op refused")/'
jc5_plant JC-01pu "TestCompare fails on the plant" TestCompare residency.go \
  's/return r\.cold || (r\.touched \&\& !present \&\& !spawn)/return r.cold/'
jc5_plant JC-01pk "TestCompare fails on the plant" TestCompare engine.go 's/if en\.coldKind(ki) {/if false {/'
