# Group j3 of Ecd Runtime: the Engine of package jacquard, its two-pass apply and its moves.
# Sourced by the rows harness from /w; the harness supplies say, sel, offline, head_of, count, tree_of, go_row and HAVE_GO.
#
#   J01     after 10,000 random deeds, column 0 of every row equals the eid that the row's record index maps to (TestJ01)
#   J01p    plant: a move pushes the zero eid instead of the entity's eid
#   J02     after every deed, the record of every row names that row and its table, and the row of every live entity
#           holds it (TestJ02)
#   J02p    plant: the record of the row that fills a swap-removed gap is not patched
#   J06     a deed with several Set and Unset ops on one entity moves it once; a deed that keeps the component set moves
#           it never (TestJ06)
#   J06p    plant: exchangePerDeed is false, so an entity moves once per added or removed component
#   J07     after 10,000 random deeds over three kinds, no entity sits in a node or holds a record of another kind than
#           its eid names; a spawn of an undeclared kind answers ecd.ErrKind and changes nothing (TestJ07)
#   J07p    plant: graph.go keys a node by its mask alone
#   J08     destroy A, spawn B of the same kind: B has A's index and the next generation, the Entity of A fails valid,
#           eidOf and rec, get of A answers ecd.ErrAbsent, get of B answers its values (TestJ08)
#   J08p    plant: pool.go keeps the generation when an index is freed
#   J3-01   get equals a map reference after 10,000 random deeds; answers are copies; the tables' row counts sum to the
#           live entities and each live eid sits in one row; a pointer-holding component answers errOp (TestJ3_01)
#   J3-01p  plant: a value is written to the next column
#   J3-02   an Unset of the last component leaves the entity alive in the empty node of its kind; get answers
#           ecd.ErrAbsent; a later Set moves it out (TestJ3_02)
#   J3-02p  plant: an entity with an empty final component set is destroyed
#   J3-03   newEngine refuses, wrapping errCatalog, a duplicate CID, CID 0, one entry more than a mask holds, a tag with
#           a size, a component of size 0 or of a size its type lacks, and a duplicate kind (TestJ3_03)
#   J3-03p  plant: the duplicate CID check is skipped
#   J3-04   a transaction that the check pass refuses leaves entities, nodes, tables and appliedAt as they were and the
#           engine not stale; two faults answer the first in commit order (TestJ3_04)
#   J3-04p  plant: the check pass reads no deed
#   J3-05   a Set, Unset or Destroy of an absent entity and a Spawn of a present one make the engine stale, wrapping
#           ecd.ErrStale and the cause; get and apply then answer ecd.ErrStale (TestJ3_05)
#   J3-05p  plant: fail does not mark the engine stale
#   J3-06   a Destroy and a Spawn of one eid in one transaction answer errOp and change nothing (TestJ3_06)
#   J3-06p  plant: the destroy and spawn refusal is not returned
#   J3-07   deeds commit in Ord order whatever their order in the list, and the check pass reads them in the same order
#           (TestJ3Ord)
#   J3-07p  plant: inOrder never sorts
#   J3-07pc plant: the commit pass reads the deeds in list order; TestJ3Ord must fail
#
# j3_plant id expect test file sedexpr [flags]: copies /w/go to a temporary directory, applies the one-line sed to
# engine/jacquard/<file> and runs only the named test there. KILLED: the test failed. SURVIVED: it passed.
# CNM: the sed did not change exactly one line, the module was missing, or the test did not run.
j3_plant() {
  j3_id=$1 j3_ex=$2 j3_test=$3 j3_file=$4 j3_sed=$5 j3_flags=${6:-}
  sel "$j3_id" j3 || return 0
  if [ $HAVE_GO -eq 0 ]; then say "$j3_id" "$j3_ex" "go not on PATH" CNM; return 0; fi
  j3_pd=$(mktemp -d /tmp/rows/plant.XXXXXX)
  cp -a /w/go "$j3_pd/go" && cp "$j3_pd/go/engine/jacquard/$j3_file" "$j3_pd/orig.go"
  (cd "$j3_pd/go/engine/jacquard" && sed -i "$j3_sed" "$j3_file")
  j3_n=$(diff "$j3_pd/orig.go" "$j3_pd/go/engine/jacquard/$j3_file" | grep -c '^>')
  if [ "$j3_n" -ne 1 ]; then
    say "$j3_id" "$j3_ex" "the sed changed $j3_n lines of $j3_file, want 1" CNM
  else
    j3_log=/tmp/rows/$j3_id.log
    (cd "$j3_pd/go" && go test -count=1 -v $j3_flags -run "^$j3_test\$" ./engine/jacquard/ >"$j3_log" 2>&1); j3_rc=$?
    if offline "$j3_log"; then say "$j3_id" "$j3_ex" "module not found" CNM
    elif grep -q "^--- FAIL: $j3_test" "$j3_log"; then say "$j3_id" "$j3_ex" "rc=$j3_rc, $j3_test FAIL" KILLED
    elif [ $j3_rc -eq 0 ] && grep -q "^--- PASS: $j3_test" "$j3_log"; then say "$j3_id" "$j3_ex" "rc=0, $j3_test PASS" SURVIVED
    else say "$j3_id" "$j3_ex" "rc=$j3_rc, the test did not run" CNM; head_of "$j3_log" 8; fi
  fi
  rm -rf "$j3_pd"
}

go_row J01 j3 "TestJ01 rc=0" /w/go test -run '^TestJ01$' ./engine/jacquard/
j3_plant J01p "TestJ01 fails on the plant" TestJ01 move.go 's/dst\.push(en\.idx\.eids\[e\.index\], /dst.push(eid.ID{}, /'

go_row J02 j3 "TestJ02 rc=0" /w/go test -run '^TestJ02$' ./engine/jacquard/
j3_plant J02p "TestJ02 fails on the plant" TestJ02 move.go 's/en\.idx\.pool\.records\[movedIx\]\.row = row/_ = movedIx/'

go_row J06 j3 "TestJ06 rc=0" /w/go test -run '^TestJ06$' ./engine/jacquard/
j3_plant J06p "TestJ06 fails on the plant" TestJ06 move.go 's/^const exchangePerDeed = true$/const exchangePerDeed = false/'

go_row J07 j3 "TestJ07 rc=0" /w/go test -run '^TestJ07$' ./engine/jacquard/
j3_plant J07p "TestJ07 fails on the plant" TestJ07 graph.go 's/return nodeKey{kind, \*m}/return nodeKey{0, *m}/'

go_row J08 j3 "TestJ08 rc=0" /w/go test -run '^TestJ08$' ./engine/jacquard/
j3_plant J08p "TestJ08 fails on the plant" TestJ08 pool.go 's/^\tnext = g + 1$/\tnext = g/'

go_row J3-01 j3 "TestJ3_01 rc=0" /w/go test -run '^TestJ3_01$' ./engine/jacquard/
j3_plant J3-01p "TestJ3_01 fails on the plant" TestJ3_01 move.go 's/t\.setBytes(row, col, v)/t.setBytes(row, col+1, v)/'

go_row J3-02 j3 "TestJ3_02 rc=0" /w/go test -run '^TestJ3_02$' ./engine/jacquard/
j3_plant J3-02p "TestJ3_02 fails on the plant" TestJ3_02 move.go 's/destroy := st\.destroyed$/destroy := st.destroyed || st.mask.isZero()/'

go_row J3-03 j3 "TestJ3_03 rc=0" /w/go test -run '^TestJ3_03$' ./engine/jacquard/
j3_plant J3-03p "TestJ3_03 fails on the plant" TestJ3_03 engine.go 's/if _, dup := en\.byCID\[c\.cid\]; dup {/if false {/'

go_row J3-04 j3 "TestJ3_04 rc=0" /w/go test -run '^TestJ3_04$' ./engine/jacquard/
j3_plant J3-04p "TestJ3_04 fails on the plant" TestJ3_04 engine.go 's/if err := en\.check(seq, deeds); err != nil {/if err := en.check(seq, deeds[:0]); err != nil {/'

go_row J3-05 j3 "TestJ3_05 rc=0" /w/go test -run '^TestJ3_05$' ./engine/jacquard/
j3_plant J3-05p "TestJ3_05 fails on the plant" TestJ3_05 engine.go 's/^\ten\.stale = true$/\t_ = cause/'

go_row J3-06 j3 "TestJ3_06 rc=0" /w/go test -run '^TestJ3_06$' ./engine/jacquard/
j3_plant J3-06p "TestJ3_06 fails on the plant" TestJ3_06 engine.go 's/return fmt\.Errorf("%w: destroy and spawn of %s in one transaction", errOp, op\.Entity)/_ = fmt.Errorf("%w", errOp)/'

go_row J3-07 j3 "TestJ3Ord rc=0" /w/go test -run '^TestJ3Ord$' ./engine/jacquard/
j3_plant J3-07p "TestJ3Ord fails on the plant" TestJ3Ord engine.go 's/if slices\.IsSortedFunc(deeds, cmp) {/if true {/'
j3_plant J3-07pc "TestJ3Ord fails on the plant" TestJ3Ord engine.go '/en\.stamp = seq/,/inOrder/s/range inOrder(deeds) {/range deeds {/'
