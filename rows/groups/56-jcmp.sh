# Group jcmp of Ecd Runtime: the comparison of the Engine of package jacquard with the reference model.
# Sourced by the rows harness from /w; the harness supplies say, sel, offline, head_of, count, tree_of, go_row and HAVE_GO.
#
#   JC-01    the Engine and the reference model answer alike, in answer class and in every byte that Get returns, on four
#            seeds of 2,000 random transactions: valid ones with out-of-order Ord, tag values nil and empty, bare spawns
#            and destroys; refused ones (kind, CID, value length, op order, gap); and ones that make both stale
#            (TestCompare)
#   JC-01p   plant: the check pass lets a second Spawn of one eid through (engine.go)
#   JC-01pt  plant: the check pass refuses an empty non-nil tag value (engine.go)
#   JC-01pd  plant: get drops the caller's buffer when the entity lacks the component (engine.go)
#
# jc_plant id expect test file sedexpr [flags]: copies /w/go to a temporary directory, applies the one-line sed to
# engine/jacquard/<file> and runs only the named test there. KILLED: the test failed. SURVIVED: it passed.
# CNM: the sed did not change exactly one line, the module was missing, or the test did not run.
jc_plant() {
  jc_id=$1 jc_ex=$2 jc_test=$3 jc_file=$4 jc_sed=$5 jc_flags=${6:-}
  sel "$jc_id" jcmp || return 0
  if [ $HAVE_GO -eq 0 ]; then say "$jc_id" "$jc_ex" "go not on PATH" CNM; return 0; fi
  jc_pd=$(mktemp -d /tmp/rows/plant.XXXXXX)
  cp -a /w/go "$jc_pd/go" && cp "$jc_pd/go/engine/jacquard/$jc_file" "$jc_pd/orig.go"
  (cd "$jc_pd/go/engine/jacquard" && sed -i "$jc_sed" "$jc_file")
  jc_n=$(diff "$jc_pd/orig.go" "$jc_pd/go/engine/jacquard/$jc_file" | grep -c '^>')
  if [ "$jc_n" -ne 1 ]; then
    say "$jc_id" "$jc_ex" "the sed changed $jc_n lines of $jc_file, want 1" CNM
  else
    jc_log=/tmp/rows/$jc_id.log
    (cd "$jc_pd/go" && go test -count=1 -v $jc_flags -run "^$jc_test\$" ./engine/jacquard/ >"$jc_log" 2>&1); jc_rc=$?
    if offline "$jc_log"; then say "$jc_id" "$jc_ex" "module not found" CNM
    elif grep -q "^--- FAIL: $jc_test" "$jc_log"; then say "$jc_id" "$jc_ex" "rc=$jc_rc, $jc_test FAIL" KILLED
    elif [ $jc_rc -eq 0 ] && grep -q "^--- PASS: $jc_test" "$jc_log"; then say "$jc_id" "$jc_ex" "rc=0, $jc_test PASS" SURVIVED
    else say "$jc_id" "$jc_ex" "rc=$jc_rc, the test did not run" CNM; head_of "$jc_log" 8; fi
  fi
  rm -rf "$jc_pd"
}

go_row JC-01 jcmp "TestCompare rc=0" /w/go test -run '^TestCompare$' ./engine/jacquard/
jc_plant JC-01p "TestCompare fails on the plant" TestCompare engine.go 's/if op\.Code == ecd\.OpSpawn && st\.ops > 0 {/if false {/'
jc_plant JC-01pt "TestCompare fails on the plant" TestCompare engine.go 's/case cd\.tag && len(op\.Value) != 0:/case cd.tag \&\& op.Value != nil:/'
jc_plant JC-01pd "TestCompare fails on the plant" TestCompare engine.go 's/return dst, fmt\.Errorf("%w: entity %s holds no component %d"/return nil, fmt.Errorf("%w: entity %s holds no component %d"/'
