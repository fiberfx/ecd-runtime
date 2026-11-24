# Group jcmpq of Ecd Runtime: the comparison of the queries of the Engine of package jacquard with the reference model.
# Sourced by the rows harness from /w; the harness supplies say, sel, offline, head_of, count, why_of, go_row and
# HAVE_GO.
#
#   JC-02    the Engine and the reference model answer every declared query, an undeclared QID and a query with an
#            argument alike, in answer class, in the eids appended after a caller's prefix and in Count, after each of
#            four seeds of 2,000 random transactions, and on a stale engine before it is rebuilt (TestCompareQuery)
#   JC-02pk  plant: the kind list of a query is ignored (query.go)
#   JC-02pe  plant: Exact is ignored (query.go)
#   JC-02pr  plant: the last row of every table is skipped (query.go)
#
# jcq_plant id expect test file sedexpr [flags]: copies /w/go to a temporary directory, applies the one-line sed to
# engine/jacquard/<file> and runs only the named test there. KILLED: the test failed. SURVIVED: it passed.
# CNM: the sed did not change exactly one line, the module was missing, or the test did not run.
jcq_plant() {
  jcq_id=$1 jcq_ex=$2 jcq_test=$3 jcq_file=$4 jcq_sed=$5 jcq_flags=${6:-}
  sel "$jcq_id" jcmpq || return 0
  if [ $HAVE_GO -eq 0 ]; then say "$jcq_id" "$jcq_ex" "go not on PATH" CNM; return 0; fi
  jcq_pd=$(mktemp -d /tmp/rows/plant.XXXXXX)
  cp -a /w/go "$jcq_pd/go" && cp "$jcq_pd/go/engine/jacquard/$jcq_file" "$jcq_pd/orig.go"
  (cd "$jcq_pd/go/engine/jacquard" && sed -i "$jcq_sed" "$jcq_file")
  jcq_n=$(diff "$jcq_pd/orig.go" "$jcq_pd/go/engine/jacquard/$jcq_file" | grep -c '^>')
  if [ "$jcq_n" -ne 1 ]; then
    say "$jcq_id" "$jcq_ex" "the sed changed $jcq_n lines of $jcq_file, want 1" CNM
  else
    jcq_log=/tmp/rows/$jcq_id.log
    (cd "$jcq_pd/go" && go test -count=1 -v $jcq_flags -run "^$jcq_test\$" ./engine/jacquard/ >"$jcq_log" 2>&1); jcq_rc=$?
    if offline "$jcq_log"; then say "$jcq_id" "$jcq_ex" "module not found" CNM
    elif grep -q "^--- FAIL: $jcq_test" "$jcq_log"; then
      say "$jcq_id" "$jcq_ex" "rc=$jcq_rc, $jcq_test FAIL: $(why_of "$jcq_log")" KILLED
    elif [ $jcq_rc -eq 0 ] && grep -q "^--- PASS: $jcq_test" "$jcq_log"; then say "$jcq_id" "$jcq_ex" "rc=0, $jcq_test PASS" SURVIVED
    else say "$jcq_id" "$jcq_ex" "rc=$jcq_rc, the test did not run" CNM; head_of "$jcq_log" 8; fi
  fi
  rm -rf "$jcq_pd"
}

go_row JC-02 jcmpq "TestCompareQuery rc=0" /w/go test -run '^TestCompareQuery$' ./engine/jacquard/
jcq_plant JC-02pk "TestCompareQuery fails on the plant" TestCompareQuery query.go 's/if !q\.has(a\.kind) || !q\.matches(&a\.mask) {/if !q.matches(\&a.mask) {/'
jcq_plant JC-02pe "TestCompareQuery fails on the plant" TestCompareQuery query.go 's/return !q\.exact || \*m == q\.all/return true/'
jcq_plant JC-02pr "TestCompareQuery fails on the plant" TestCompareQuery query.go 's/for row := uint32(0); row < t\.len(); row++ {/for row := uint32(0); row+1 < t.len(); row++ {/'
