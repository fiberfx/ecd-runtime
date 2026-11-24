# Group j0m of Ecd Runtime: the reference model and its op-sequence vectors, package model of engine/jacquard.
# Sourced by the rows harness from /w; the harness supplies say, sel, offline, head_of, count, tree_of, go_row and HAVE_GO.
#
#   L07-model    Apply 1..1,000 and then 1,001..2,000 equal a reference built beside the model, and At() is 2,000;
#                Apply of At()+2 and of At() answer ErrGap and change nothing (TestApplyCatchUp)
#   L07-model-p  plant: the gap check is dropped; TestApplyCatchUp must fail
#   L13-model    after an injected panic, a commit-pass error and a commit-pass error before the first change, every
#                call answers ErrStale; a new model rebuilt from the same deeds equals the reference (TestStale)
#   L13-model-p  plant: a failed commit pass leaves the model serving; TestStale must fail
#   L14-model    a cold All kind answers ErrCold to Get and to every query that names it, and answers once hydrated;
#                an entity spawned while the kind was cold is absent after hydration (TestCold)
#   L14-model-p  plant: a query skips a cold kind and answers empty; TestCold must fail
#   J0M-01       every vector in testdata/ops replays with its hand-written expectations (TestVectors)
#   J0M-01-p     plant: OpUnset changes nothing; TestVectors must fail
#   J0M-02       queries 0 to 4 and 6 equal a brute-force filter of the reference; a query on a Touched kind, an
#                unknown QID and a query with an argument answer ErrQuery (TestQueries)
#   J0M-02-p     plant: the Any term is skipped; TestQueries must fail
#   J0M-02-pn    plant: the None term is skipped; TestQueries must fail
#   J0M-03       a transaction refused by the check pass leaves the model and At() unchanged, whichever deed holds the
#                fault; with several faults the first in commit order is returned, and a gap before any op
#                (TestCheckAtomic, TestCheckFirstFault)
#   J0M-03-p     plant: the check pass reads no deed; TestCheckAtomic must fail
#   J0M-04       a Destroy and a Spawn of one eid in one transaction answer ErrOp, in either order, in one deed or in
#                two, and change nothing (TestDestroySpawn)
#   J0M-04-p     plant: the return of the spawn-and-destroy check is replaced by `_ = ecd.ErrOp`; the pair spawn then
#                destroy is accepted and TestDestroySpawn must fail
#   J0M-05       a buffer overwritten after Apply or Hydrate leaves Get unchanged, and a write into an answer changes
#                nothing (TestInboundCopy)
#   J0M-05-p     plant: a Set stores the caller's slice; TestInboundCopy must fail
#   L12-model    CNM: the model against a SQL twin needs a SQLite driver (modernc.org/sqlite v1.37.0), and the module
#                inputs hold ecd-id only
#
# j0m_plant id expect tests sedexpr: copies /w/go to a temporary directory, applies the one-line sed to
# engine/jacquard/model/model.go and runs only the named tests there (several names joined with |).
# KILLED: a named test failed. SURVIVED: they passed. CNM: the sed did not change exactly one line, the module was
# missing, or the tests did not run.
j0m_plant() {
  j0m_id=$1 j0m_ex=$2 j0m_test=$3 j0m_sed=$4
  sel "$j0m_id" j0m || return 0
  if [ $HAVE_GO -eq 0 ]; then say "$j0m_id" "$j0m_ex" "go not on PATH" CNM; return 0; fi
  j0m_pd=$(mktemp -d /tmp/rows/plant.XXXXXX)
  cp -a /w/go "$j0m_pd/go" && cp "$j0m_pd/go/engine/jacquard/model/model.go" "$j0m_pd/orig.go"
  (cd "$j0m_pd/go/engine/jacquard/model" && sed -i "$j0m_sed" model.go)
  j0m_n=$(diff "$j0m_pd/orig.go" "$j0m_pd/go/engine/jacquard/model/model.go" | grep -c '^>')
  if [ "$j0m_n" -ne 1 ]; then
    say "$j0m_id" "$j0m_ex" "the sed changed $j0m_n lines of model.go, want 1" CNM
  else
    j0m_log=/tmp/rows/$j0m_id.log
    (cd "$j0m_pd/go" && go test -count=1 -v -run "^($j0m_test)\$" ./engine/jacquard/model/ >"$j0m_log" 2>&1); j0m_rc=$?
    if offline "$j0m_log"; then say "$j0m_id" "$j0m_ex" "module not found" CNM
    elif grep -q -E "^--- FAIL: ($j0m_test)" "$j0m_log"; then say "$j0m_id" "$j0m_ex" "rc=$j0m_rc, a named test FAIL" KILLED
    elif [ $j0m_rc -eq 0 ] && grep -q -E "^--- PASS: ($j0m_test)" "$j0m_log"; then say "$j0m_id" "$j0m_ex" "rc=0, the named tests PASS" SURVIVED
    else say "$j0m_id" "$j0m_ex" "rc=$j0m_rc, the tests did not run" CNM; head_of "$j0m_log" 8; fi
  fi
  rm -rf "$j0m_pd"
}

go_row L07-model j0m "TestApplyCatchUp rc=0" /w/go test -run '^TestApplyCatchUp$' ./engine/jacquard/model/
j0m_plant L07-model-p "TestApplyCatchUp fails on the plant" TestApplyCatchUp 's/if seq != m\.at+1 {/if false {/'

go_row L13-model j0m "TestStale rc=0" /w/go test -run '^TestStale$' ./engine/jacquard/model/
j0m_plant L13-model-p "TestStale fails on the plant" TestStale 's/m\.stale = true/m.stale = false/'

go_row L14-model j0m "TestCold rc=0" /w/go test -run '^TestCold$' ./engine/jacquard/model/
j0m_plant L14-model-p "TestCold fails on the plant" TestCold 's/return fmt\.Errorf("model: query %d: kind %v is cold: %w", q, k, ecd\.ErrCold)/continue/'

go_row J0M-01 j0m "TestVectors rc=0" /w/go test -run '^TestVectors$' ./engine/jacquard/model/
j0m_plant J0M-01-p "TestVectors fails on the plant" TestVectors 's/delete(vals, op\.CID)/_ = vals/'

go_row J0M-02 j0m "TestQueries rc=0" /w/go test -run '^TestQueries$' ./engine/jacquard/model/
j0m_plant J0M-02-p "TestQueries fails on the plant" TestQueries 's/if len(q\.Any) > 0 && !anyOf(comps, q\.Any) {/if false {/'
j0m_plant J0M-02-pn "TestQueries fails on the plant" TestQueries 's/if anyOf(comps, q\.None) {/if false {/'

go_row J0M-03 j0m "TestCheckAtomic and TestCheckFirstFault rc=0" /w/go test -run '^(TestCheckAtomic|TestCheckFirstFault)$' ./engine/jacquard/model/
j0m_plant J0M-03-p "TestCheckAtomic fails on the plant" 'TestCheckAtomic|TestCheckFirstFault' 's/if err := m\.check(seq, deeds); err != nil {/if err := m.check(seq, deeds[:0]); err != nil {/'

go_row J0M-04 j0m "TestDestroySpawn rc=0" /w/go test -run '^TestDestroySpawn$' ./engine/jacquard/model/
j0m_plant J0M-04-p "TestDestroySpawn fails on the plant" TestDestroySpawn 's/return fmt\.Errorf("model: spawn and destroy of %v in one transaction: %w", e, ecd\.ErrOp)/_ = ecd.ErrOp/'

go_row J0M-05 j0m "TestInboundCopy rc=0" /w/go test -run '^TestInboundCopy$' ./engine/jacquard/model/
j0m_plant J0M-05-p "TestInboundCopy fails on the plant" TestInboundCopy 's/vals\[op.CID\] = append(\[\]byte(nil), op.Value...)/vals[op.CID] = op.Value/'

if sel L12-model j0m; then
  say L12-model "every declared query equals its SQL twin on 100,000 entities" "no SQLite driver: modernc.org/sqlite v1.37.0 is not in the module inputs, which hold ecd-id only" CNM
fi
