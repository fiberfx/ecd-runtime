# Group j2c of Ecd Runtime: tables, chunks, typed columns and change stamps of package jacquard.
# Sourced by rows/loom.rows.sh from /w; the harness supplies say, sel, offline, head_of, count, tree_of, go_row and HAVE_GO.
#
#   J09      pointer columns survive the collector: a string and a slice in a component stay intact across pushes, copies
#            and swap-removes with two collections between operations, and the finalizer of an object that only a resident
#            value references never runs (TestJ09); a control object shows that a run is observable
#   J09p     plant: columns are allocated as []byte arrays; TestJ09 must fail
#   J2C-01   eid and ix columns and both value columns equal a reference after random push and swapRemove; swapRemove
#            returns the moved row's ix; a pushed row and every slot beyond the length read zero (TestTableRows)
#   J2C-01p  plant: swapRemove returns the removed row's ix
#   J2C-02   rows read back across three chunks; a chunk holds 1<<chunkShift(rowB) rows (TestTableChunks)
#   J2C-02p  plant: the chunk of a row is row/64
#   J2C-03   after removing every row one spare chunk remains (TestTableShrink)
#   J2C-03p  plant: shrink keeps every chunk
#   J2C-04   copyShared copies equal compIDs only; a column absent from the source reads zero (TestCopyShared)
#   J2C-04p  plant: columns are matched by position
#   J2C-05   stamps: a write at stamp s is seen by readers below s and by none at s or above, per chunk and column;
#            push and swapRemove stamp the rows of the chunks they touch (TestStamps)
#   J2C-05p  plant: a column write records stamp-1
#   J2C-06   col(c) is the position of c in ids, and (0, false) for every other component (TestColRank)
#   J2C-06p  plant: the rank counts the bit of c itself
#   J2C-07   the vacated tail slot reads zero in eid, ix and every column, and the finalizer of what the removed value
#            referenced runs (TestTailZero)
#   J2C-07p  plant: swapRemove does not clear the tail slot
#   J2C-08   hasPointers finds pointers through fields and array elements (TestHasPointers)
#   J2C-08p  plant: unsafe.Pointer is not a pointer kind
#   J2C-09   setBytes copies in and refuses a wrong length or a pointer column; appendBytes copies out (TestSetBytes)
#   J2C-09p  plant: setBytes accepts a shorter slice
#
# j2c_plant id expect test file sedexpr [flags]: copies /w/go to a temporary directory, applies the one-line sed to
# engine/jacquard/<file> and runs only the named test there. KILLED: the test failed. SURVIVED: it passed.
# CNM: the sed did not change exactly one line, the module was missing, or the test did not run.
j2c_plant() {
  j2c_id=$1 j2c_ex=$2 j2c_test=$3 j2c_file=$4 j2c_sed=$5 j2c_flags=${6:-}
  sel "$j2c_id" j2c || return 0
  if [ $HAVE_GO -eq 0 ]; then say "$j2c_id" "$j2c_ex" "go not on PATH" CNM; return 0; fi
  j2c_pd=$(mktemp -d /tmp/rows/plant.XXXXXX)
  cp -a /w/go "$j2c_pd/go" && cp "$j2c_pd/go/engine/jacquard/$j2c_file" "$j2c_pd/orig.go"
  (cd "$j2c_pd/go/engine/jacquard" && sed -i "$j2c_sed" "$j2c_file")
  j2c_n=$(diff "$j2c_pd/orig.go" "$j2c_pd/go/engine/jacquard/$j2c_file" | grep -c '^>')
  if [ "$j2c_n" -ne 1 ]; then
    say "$j2c_id" "$j2c_ex" "the sed changed $j2c_n lines of $j2c_file, want 1" CNM
  else
    j2c_log=/tmp/rows/$j2c_id.log
    (cd "$j2c_pd/go" && go test -count=1 -v $j2c_flags -run "^$j2c_test\$" ./engine/jacquard/ >"$j2c_log" 2>&1); j2c_rc=$?
    if offline "$j2c_log"; then say "$j2c_id" "$j2c_ex" "module not found" CNM
    elif grep -q "^--- FAIL: $j2c_test" "$j2c_log"; then say "$j2c_id" "$j2c_ex" "rc=$j2c_rc, $j2c_test FAIL" KILLED
    elif [ $j2c_rc -eq 0 ] && grep -q "^--- PASS: $j2c_test" "$j2c_log"; then say "$j2c_id" "$j2c_ex" "rc=0, $j2c_test PASS" SURVIVED
    else say "$j2c_id" "$j2c_ex" "rc=$j2c_rc, the test did not run" CNM; head_of "$j2c_log" 8; fi
  fi
  rm -rf "$j2c_pd"
}

go_row J09 j2c "TestJ09 rc=0" /w/go test -run '^TestJ09$' ./engine/jacquard/
j2c_plant J09p "TestJ09 fails on the plant" TestJ09 column.go 's/reflect\.SliceOf(d\.typ), n, n)/reflect.SliceOf(reflect.TypeFor[byte]()), n*int(d.typ.Size()), n*int(d.typ.Size()))/'

go_row J2C-01 j2c "TestTableRows rc=0" /w/go test -run '^TestTableRows$' ./engine/jacquard/
j2c_plant J2C-01p "TestTableRows fails on the plant" TestTableRows table.go 's/movedIx = tc\.ix\[ti\]$/movedIx = dc.ix[di]/'

go_row J2C-02 j2c "TestTableChunks rc=0" /w/go test -run '^TestTableChunks$' ./engine/jacquard/
j2c_plant J2C-02p "TestTableChunks fails on the plant" TestTableChunks table.go 's/return t\.chunks\[row>>t\.shift\], /return t.chunks[row\/64], /'

go_row J2C-03 j2c "TestTableShrink rc=0" /w/go test -run '^TestTableShrink$' ./engine/jacquard/
j2c_plant J2C-03p "TestTableShrink fails on the plant" TestTableShrink table.go 's/if keep >= len(t\.chunks) {/if keep >= 0 {/'

go_row J2C-04 j2c "TestCopyShared rc=0" /w/go test -run '^TestCopyShared$' ./engine/jacquard/
j2c_plant J2C-04p "TestCopyShared fails on the plant" TestCopyShared table.go 's/j, ok := src\.col(c)/j, ok := i, i < len(src.ids) \&\& c < maskBits/'

go_row J2C-05 j2c "TestStamps rc=0" /w/go test -run '^TestStamps$' ./engine/jacquard/
j2c_plant J2C-05p "TestStamps fails on the plant" TestStamps table.go 's/{ ch\.colAt\[col\] = \*t\.at }/{ ch.colAt[col] = *t.at - 1 }/'

go_row J2C-06 j2c "TestColRank rc=0" /w/go test -run '^TestColRank$' ./engine/jacquard/
j2c_plant J2C-06p "TestColRank fails on the plant" TestColRank table.go 's/m\[w\] & (1<<(c&63) - 1)/m[w] \& (1<<(c\&63+1) - 1)/'

go_row J2C-07 j2c "TestTailZero rc=0" /w/go test -run '^TestTailZero$' ./engine/jacquard/
j2c_plant J2C-07p "TestTailZero fails on the plant" TestTailZero table.go 's/^\tt\.clearRow(tc, ti)$/\t_, _ = tc, ti/'

go_row J2C-08 j2c "TestHasPointers rc=0" /w/go test -run '^TestHasPointers$' ./engine/jacquard/
j2c_plant J2C-08p "TestHasPointers fails on the plant" TestHasPointers column.go 's/reflect\.Pointer, reflect\.UnsafePointer, /reflect.Pointer, /'

go_row J2C-09 j2c "TestSetBytes rc=0" /w/go test -run '^TestSetBytes$' ./engine/jacquard/
j2c_plant J2C-09p "TestSetBytes fails on the plant" TestSetBytes table.go 's/len(b) != t\.sizes\[col\]/len(b) > t.sizes[col]/'
