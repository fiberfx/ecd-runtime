# Group j5p of Ecd Runtime: the residency policy of jacquard.New, which keeps a Touched kind within its byte budget by
# CLOCK eviction, refuses the hydration of an All kind that exceeds its budget, and sets the mark of a read outside the
# record.
# Sourced by the rows harness from /w; the harness supplies say, sel, offline, head_of, count, go_row and HAVE_GO.
#
#   J11         four seeds of 2,000 transactions through New and a store, with two All kinds and one Touched kind that
#               exceeds its budget many times over: after every Apply and Hydrate the Touched kind holds at most its
#               budget or no entity, its evictions never fall, and each All kind and every Get equals the store's
#               (TestJ11)
#   L15-jacquard  the Touched entities are spawned in one transaction, so the kind holds its full bytes before the first
#               trim, with a budget of a tenth of those bytes: 100,000 random reads each equal the store or answer
#               ErrMiss, and a miss is hydrated from the store and read again (TestL15)
#   L15v        an All kind with a budget below the bytes of its rows refuses the hydration with ErrBudget, stays Cold
#               with no entity, and leaves At and the seq rule as before; a budget equal to those bytes hydrates the
#               kind Warm (TestL15Budget)
#   L35-jacquard  1,000 Get hits on one Touched entity leave its record equal and set its bit in the clock of its kind
#               (TestL35Record)
#   J5P-01      a read entity is passed over by the hand and an unread one is the victim; an entity that is evicted
#               after a read leaves no mark for the entity that takes its index (TestJ5PClock)
#   J5P-02      a Touched kind with Budget 0 keeps no entity after a spawn or a hydration, and a nil catalog and a
#               duplicate CID are refused with a nil engine (TestJ5PZero)
#   J5P-03      a Warm All kind that is hydrated again with rows over its budget answers ErrBudget after the whole
#               source is read, and its state, Stats, seq and every value that Get reads are as before
#               (TestJ5PBudgetWarm)
#   J5P-04      a refusal for a row, under a budget that is not reached, is not ErrBudget and leaves a Warm All kind as
#               it was (TestJ5PBudgetOp)
#   J5P-05      the Bytes that hydratedBytes predicts before a hydration equal the Bytes that kindStats and Stats
#               report after it, over four steps on one engine that cover several tables, several chunks, a table
#               that shrinks, a table that the rows do not use and new nodes (TestJ5PBudgetPredict)
#   J5P-06      rows far over the budget are refused before the source is read to its end, and the Cold kind stays Cold
#               (TestJ5PBudgetEarly)
#   J5P-07      rows that fit the budget are not refused although the bytes of their first part, as if the source ended
#               there, are over it; a budget one byte lower is refused (TestJ5PBudgetBound)
#   J5P-08      an entity that is destroyed after a read leaves no mark for the entity that takes its index, and a
#               refused Apply that destroys a marked entity leaves the mark (TestJ5PApplyDestroy)
#   J11p        plant: the trim covers every kind, so an All kind is evicted (policy.go; TestJ11)
#   J11q        plant: an eviction by the hand is not counted (policy.go; TestJ11)
#   L15s        plant: the tables of a kind keep their chunks after an eviction (policy.go; TestL15)
#   L15p        plant: a hydration of an All kind is never refused for its budget after staging (hydrate.go;
#               TestL15Budget)
#   L35p        plant: a read writes a flag bit of the record in place of the mark (policy.go; TestL35Record)
#   J5P-01p     plant: a read sets no mark (policy.go; TestJ5PClock)
#   J5P-01q     plant: an eviction by the caller leaves the mark of the index (policy.go; TestJ5PClock)
#   J5P-02p     plant: a hydration is not followed by a trim (policy.go; TestJ5PZero)
#   J5P-03p     plant: a hydration of an All kind is never refused for its budget after staging (hydrate.go;
#               TestJ5PBudgetWarm)
#   J5P-04p     plant: the size of a value is not checked, so a refusal for a row comes after the first change
#               (hydrate.go; TestJ5PBudgetOp)
#   J5P-05p     plant: a table is predicted to keep every chunk that it held, not one (bytes.go; TestJ5PBudgetPredict)
#   J5P-05q     plant: a node that the rows need and the kind does not hold is not predicted (bytes.go;
#               TestJ5PBudgetPredict)
#   J5P-05r     plant: the chunks that n rows fill are rounded down (bytes.go; TestJ5PBudgetPredict)
#   J5P-06p     plant: the lower bound refuses no row, so the source is read to its end (hydrate.go; TestJ5PBudgetEarly)
#   J5P-07p     plant: the lower bound counts eight times the index cost of an entity, so it exceeds the Bytes of the
#               kind and refuses rows that fit (hydrate.go; TestJ5PBudgetBound)
#   J5P-08p     plant: an Apply that destroys a read entity leaves the mark of its index (policy.go;
#               TestJ5PApplyDestroy)
#
# j5p_plant id expect test file sedexpr: copies /w/go to a temporary directory, applies the one-line sed to
# engine/jacquard/<file> and runs only the named test there. KILLED: the test failed, and the observed text names its
# first failing line. SURVIVED: it passed. CNM: the sed did not change exactly one line, the module was missing, or the
# test did not run.
j5p_plant() {
  j5p_id=$1 j5p_ex=$2 j5p_test=$3 j5p_file=$4 j5p_sed=$5
  sel "$j5p_id" j5p || return 0
  if [ $HAVE_GO -eq 0 ]; then say "$j5p_id" "$j5p_ex" "go not on PATH" CNM; return 0; fi
  j5p_pd=$(mktemp -d /tmp/rows/plant.XXXXXX)
  cp -a /w/go "$j5p_pd/go" && cp "$j5p_pd/go/engine/jacquard/$j5p_file" "$j5p_pd/orig.go"
  (cd "$j5p_pd/go/engine/jacquard" && sed -i "$j5p_sed" "$j5p_file")
  j5p_n=$(diff "$j5p_pd/orig.go" "$j5p_pd/go/engine/jacquard/$j5p_file" | grep -c '^>')
  if [ "$j5p_n" -ne 1 ]; then
    say "$j5p_id" "$j5p_ex" "the sed changed $j5p_n lines of $j5p_file, want 1" CNM
  else
    j5p_log=/tmp/rows/$j5p_id.log
    (cd "$j5p_pd/go" &&
      go test -count=1 -v -timeout 900s -run "^$j5p_test\$" ./engine/jacquard/ >"$j5p_log" 2>&1); j5p_rc=$?
    if offline "$j5p_log"; then say "$j5p_id" "$j5p_ex" "module not found" CNM
    elif grep -q "^--- FAIL: $j5p_test" "$j5p_log"; then
      say "$j5p_id" "$j5p_ex" \
        "rc=$j5p_rc, $j5p_test FAIL: $(grep -m1 -E '_test\.go:[0-9]+:|^panic:' "$j5p_log" | cut -c1-100)" KILLED
    elif [ $j5p_rc -eq 0 ] && grep -q "^--- PASS: $j5p_test" "$j5p_log"; then
      say "$j5p_id" "$j5p_ex" "rc=0, $j5p_test PASS" SURVIVED
    else say "$j5p_id" "$j5p_ex" "rc=$j5p_rc, the test did not run" CNM; head_of "$j5p_log" 8; fi
  fi
  rm -rf "$j5p_pd"
}

go_row J11 j5p "TestJ11 rc=0" /w/go test -run '^TestJ11$' ./engine/jacquard/
go_row L15-jacquard j5p "TestL15 rc=0" /w/go test -run '^TestL15$' ./engine/jacquard/
go_row L15v j5p "TestL15Budget rc=0" /w/go test -run '^TestL15Budget$' ./engine/jacquard/
go_row L35-jacquard j5p "TestL35Record rc=0" /w/go test -run '^TestL35Record$' ./engine/jacquard/
go_row J5P-01 j5p "TestJ5PClock rc=0" /w/go test -run '^TestJ5PClock$' ./engine/jacquard/
go_row J5P-02 j5p "TestJ5PZero rc=0" /w/go test -run '^TestJ5PZero$' ./engine/jacquard/
go_row J5P-03 j5p "TestJ5PBudgetWarm rc=0" /w/go test -run '^TestJ5PBudgetWarm$' ./engine/jacquard/
go_row J5P-04 j5p "TestJ5PBudgetOp rc=0" /w/go test -run '^TestJ5PBudgetOp$' ./engine/jacquard/
go_row J5P-05 j5p "TestJ5PBudgetPredict rc=0" /w/go test -run '^TestJ5PBudgetPredict$' ./engine/jacquard/
go_row J5P-06 j5p "TestJ5PBudgetEarly rc=0" /w/go test -run '^TestJ5PBudgetEarly$' ./engine/jacquard/
go_row J5P-07 j5p "TestJ5PBudgetBound rc=0" /w/go test -run '^TestJ5PBudgetBound$' ./engine/jacquard/
go_row J5P-08 j5p "TestJ5PApplyDestroy rc=0" /w/go test -run '^TestJ5PApplyDestroy$' ./engine/jacquard/
j5p_plant J11p "TestJ11 fails on the plant" TestJ11 policy.go 's/if p\.res\[ki\]\.touched {/if true {/'
j5p_plant J11q "TestJ11 fails on the plant" TestJ11 policy.go 's/\t\t\tp\.res\[ki\]\.evictions++/\t\t\t_ = i/'
j5p_plant L15s "TestL15 fails on the plant" TestL15 policy.go 's/^\t\t\tt\.shrink()$/\t\t\t_ = t/'
j5p_plant L15p "TestL15Budget fails on the plant" TestL15Budget hydrate.go \
  's/if b := en\.hydratedBytes(ki, rows); b > limit {/if b := en.hydratedBytes(ki, rows); b < 0 {/'
j5p_plant L35p "TestL35Record fails on the plant" TestL35Record policy.go \
  's/p\.clocks\[ki\]\.mark(e\.index)/p.idx.pool.records[e.index].flags |= 1 << 17/'
j5p_plant J5P-01p "TestJ5PClock fails on the plant" TestJ5PClock policy.go \
  's/^\t\t\tp\.clocks\[ki\]\.mark(e\.index)$/\t\t\t_ = e/'
j5p_plant J5P-01q "TestJ5PClock fails on the plant" TestJ5PClock policy.go \
  's/^\tc\.bits\[w\]\.And(^bit)$/\t_, _ = w, bit/'
j5p_plant J5P-02p "TestJ5PZero fails on the plant" TestJ5PZero policy.go 's/^\t\tp\.trim()$/\t\t_ = p/'
j5p_plant J5P-03p "TestJ5PBudgetWarm fails on the plant" TestJ5PBudgetWarm hydrate.go \
  's/if b := en\.hydratedBytes(ki, rows); b > limit {/if b := en.hydratedBytes(ki, rows); b < 0 {/'
j5p_plant J5P-04p "TestJ5PBudgetOp fails on the plant" TestJ5PBudgetOp hydrate.go \
  's/if len(r\.Value) != cd\.size {/if false {/'
j5p_plant J5P-05p "TestJ5PBudgetPredict fails on the plant" TestJ5PBudgetPredict bytes.go \
  's/kept := min(len(t\.chunks), 1)/kept := len(t.chunks)/'
j5p_plant J5P-05q "TestJ5PBudgetPredict fails on the plant" TestJ5PBudgetPredict bytes.go \
  's/for m, n := range count {/for m, n := range map[mask]int{} {/'
j5p_plant J5P-05r "TestJ5PBudgetPredict fails on the plant" TestJ5PBudgetPredict bytes.go \
  's/return (n + 1<<shift - 1) >> shift/return n >> shift/'
j5p_plant J5P-06p "TestJ5PBudgetEarly fails on the plant" TestJ5PBudgetEarly hydrate.go \
  's/if limit > 0 \&\& low > limit {/if limit > 0 \&\& low < 0 {/'
j5p_plant J5P-07p "TestJ5PBudgetBound fails on the plant" TestJ5PBudgetBound hydrate.go \
  's/low += indexEntityBytes$/low += 8 * indexEntityBytes/'
j5p_plant J5P-08p "TestJ5PApplyDestroy fails on the plant" TestJ5PApplyDestroy policy.go \
  's/^\t\tp\.clocks\[f\.kind\]\.unmark(f\.index)$/\t\t_ = f/'
