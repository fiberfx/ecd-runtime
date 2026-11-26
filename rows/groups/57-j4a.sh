# Group j4a of Entity Component Runtime: queries in the Engine of package jacquard.
# Sourced by the rows harness from /w; the harness supplies say, sel, offline, head_of, count, why_of, go_row and
# HAVE_GO.
#
#   J4A-01    All, None, Any and Exact select the hand-written eids on an engine with entities of seven component sets over
#             two kinds, tags included, and Kinds restricts the kinds; the answers follow a later transaction that moves
#             and destroys entities (TestJ4Terms)
#   J4A-01p   plant: None is ignored
#   J4A-01pa  plant: Any is ignored
#   J4A-01pe  plant: Exact is ignored
#   J4A-01pk  plant: Kinds is ignored
#   J4A-02    an unknown QID, a non-empty arg and an undeclared kind answer ecd.ErrQuery and append nothing; a stale engine
#             answers ecd.ErrStale first; a QID declared twice is refused by addQuery; an undeclared CID in All, or Any
#             that lists only undeclared CIDs, matches nothing (TestJ4Refusals)
#   J4A-02p   plant: a non-empty arg is accepted
#   J4A-02pn  plant: an undeclared CID in All no longer makes the query match nothing
#   J4A-02pa  plant: Any that lists only undeclared CIDs no longer makes the query match nothing
#   J4A-02pk  plant: an undeclared kind no longer makes the query answer ecd.ErrQuery
#   J4A-03    the answer is appended after the caller's IDs, in eid order across kinds and tables, and Count grows by the
#             number appended (TestJ4Order)
#   J4A-03p   plant: the appended part is not sorted
#
# j4a_plant id expect test file sedexpr [flags]: copies /w/go to a temporary directory, applies the one-line sed to
# engine/jacquard/<file> and runs only the named test there. KILLED: the test failed. SURVIVED: it passed.
# CNM: the sed did not change exactly one line, the module was missing, or the test did not run.
j4a_plant() {
  j4a_id=$1 j4a_ex=$2 j4a_test=$3 j4a_file=$4 j4a_sed=$5 j4a_flags=${6:-}
  sel "$j4a_id" j4a || return 0
  if [ $HAVE_GO -eq 0 ]; then say "$j4a_id" "$j4a_ex" "go not on PATH" CNM; return 0; fi
  j4a_pd=$(mktemp -d /tmp/rows/plant.XXXXXX)
  cp -a /w/go "$j4a_pd/go" && cp "$j4a_pd/go/engine/jacquard/$j4a_file" "$j4a_pd/orig.go"
  (cd "$j4a_pd/go/engine/jacquard" && sed -i "$j4a_sed" "$j4a_file")
  j4a_n=$(diff "$j4a_pd/orig.go" "$j4a_pd/go/engine/jacquard/$j4a_file" | grep -c '^>')
  if [ "$j4a_n" -ne 1 ]; then
    say "$j4a_id" "$j4a_ex" "the sed changed $j4a_n lines of $j4a_file, want 1" CNM
  else
    j4a_log=/tmp/rows/$j4a_id.log
    (cd "$j4a_pd/go" && go test -count=1 -v $j4a_flags -run "^$j4a_test\$" ./engine/jacquard/ >"$j4a_log" 2>&1); j4a_rc=$?
    if offline "$j4a_log"; then say "$j4a_id" "$j4a_ex" "module not found" CNM
    elif grep -q "^--- FAIL: $j4a_test" "$j4a_log"; then
      say "$j4a_id" "$j4a_ex" "rc=$j4a_rc, $j4a_test FAIL: $(why_of "$j4a_log")" KILLED
    elif [ $j4a_rc -eq 0 ] && grep -q "^--- PASS: $j4a_test" "$j4a_log"; then say "$j4a_id" "$j4a_ex" "rc=0, $j4a_test PASS" SURVIVED
    else say "$j4a_id" "$j4a_ex" "rc=$j4a_rc, the test did not run" CNM; head_of "$j4a_log" 8; fi
  fi
  rm -rf "$j4a_pd"
}

go_row J4A-01 j4a "TestJ4Terms rc=0" /w/go test -run '^TestJ4Terms$' ./engine/jacquard/
j4a_plant J4A-01p  "TestJ4Terms fails on the plant"    TestJ4Terms    query.go 's/if m\.containsAny(&q\.none) {/if false {/'
j4a_plant J4A-01pa "TestJ4Terms fails on the plant"    TestJ4Terms    query.go 's/if !q\.any\.isZero() && !m\.containsAny(&q\.any) {/if false {/'
j4a_plant J4A-01pe "TestJ4Terms fails on the plant"    TestJ4Terms    query.go 's/return !q\.exact || \*m == q\.all/return true/'
j4a_plant J4A-01pk "TestJ4Terms fails on the plant"    TestJ4Terms    query.go 's/return len(q\.kinds) == 0 || slices\.Contains(q\.kinds, kind)/return true/'

go_row J4A-02 j4a "TestJ4Refusals rc=0" /w/go test -run '^TestJ4Refusals$' ./engine/jacquard/
j4a_plant J4A-02p  "TestJ4Refusals fails on the plant" TestJ4Refusals query.go 's/if len(arg) != 0 {/if false {/'
j4a_plant J4A-02pn "TestJ4Refusals fails on the plant" TestJ4Refusals query.go 's/^\t\t\tq\.never = true$/\t\t\tq.never = false/'
j4a_plant J4A-02pa "TestJ4Refusals fails on the plant" TestJ4Refusals query.go 's/^\t\tq\.never = true$/\t\tq.never = false/'
j4a_plant J4A-02pk "TestJ4Refusals fails on the plant" TestJ4Refusals query.go 's/q\.badKind = true/q.badKind = false/'

go_row J4A-03 j4a "TestJ4Order rc=0" /w/go test -run '^TestJ4Order$' ./engine/jacquard/
j4a_plant J4A-03p  "TestJ4Order fails on the plant"    TestJ4Order    query.go 's/slices\.SortFunc(added, eid\.ID\.Compare)/_ = added/'
