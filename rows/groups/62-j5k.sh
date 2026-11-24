# Group j5k of Ecd Runtime: the referenced bitset of package jacquard that orders the eviction of entities.
# Sourced by the rows harness from /w; the harness supplies say, sel, offline, head_of, count, tree_of, go_row and HAVE_GO.
#
#   J5K-01   sweep returns the unmarked resident indexes in hand order, clears the marks it passes, skips an index that is
#            not resident, and continues from the index after its last victim (TestJ5KSweep)
#   J5K-02   an index marked since the last pass is never a victim while an unmarked resident remains; marks from 8
#            goroutines that share words all read back (TestJ5KSecondChance)
#   J5K-01p  plant: the hand does not clear the marks it passes (clock.go)
#   J5K-02p  plant: mark sets no bit (clock.go)
#
# j5k_plant id expect test file sedexpr: copies /w/go to a temporary directory, applies the sed to
# engine/jacquard/<file> and runs only the named test there. KILLED: the test failed. SURVIVED: it passed.
# CNM: the sed did not change exactly one line, the module was missing, or the test did not run.
j5k_plant() {
  j5k_id=$1 j5k_ex=$2 j5k_test=$3 j5k_file=$4 j5k_sed=$5
  sel "$j5k_id" j5k || return 0
  if [ $HAVE_GO -eq 0 ]; then say "$j5k_id" "$j5k_ex" "go not on PATH" CNM; return 0; fi
  j5k_pd=$(mktemp -d /tmp/rows/plant.XXXXXX)
  cp -a /w/go "$j5k_pd/go" && cp "$j5k_pd/go/engine/jacquard/$j5k_file" "$j5k_pd/orig.go"
  (cd "$j5k_pd/go/engine/jacquard" && sed -i "$j5k_sed" "$j5k_file")
  j5k_n=$(diff "$j5k_pd/orig.go" "$j5k_pd/go/engine/jacquard/$j5k_file" | grep -c '^>')
  if [ "$j5k_n" -ne 1 ]; then
    say "$j5k_id" "$j5k_ex" "the sed changed $j5k_n lines of $j5k_file, want 1" CNM
  else
    j5k_log=/tmp/rows/$j5k_id.log
    (cd "$j5k_pd/go" && go test -count=1 -v -run "^$j5k_test\$" ./engine/jacquard/ >"$j5k_log" 2>&1); j5k_rc=$?
    if offline "$j5k_log"; then say "$j5k_id" "$j5k_ex" "module not found" CNM
    elif grep -q "^--- FAIL: $j5k_test" "$j5k_log"; then say "$j5k_id" "$j5k_ex" "rc=$j5k_rc, $j5k_test FAIL" KILLED
    elif [ $j5k_rc -eq 0 ] && grep -q "^--- PASS: $j5k_test" "$j5k_log"; then say "$j5k_id" "$j5k_ex" "rc=0, $j5k_test PASS" SURVIVED
    else say "$j5k_id" "$j5k_ex" "rc=$j5k_rc, the test did not run" CNM; head_of "$j5k_log" 8; fi
  fi
  rm -rf "$j5k_pd"
}

go_row J5K-01 j5k "TestJ5KSweep rc=0" /w/go test -run '^TestJ5KSweep$' ./engine/jacquard/
go_row J5K-02 j5k "TestJ5KSecondChance rc=0" /w/go test -run '^TestJ5KSecondChance$' ./engine/jacquard/
j5k_plant J5K-01p "TestJ5KSweep fails on the plant"        TestJ5KSweep        clock.go 's/c\.bits\[w\]\.And(^bit)/_ = bit/'
j5k_plant J5K-02p "TestJ5KSecondChance fails on the plant" TestJ5KSecondChance clock.go 's/c\.bits\[w\]\.Or(bit)/c.bits[w].Or(bit \& 0)/'
