# Group acmp19 of Entity Component Runtime: the comparison of the ark adapter with the reference model on random
# histories, and the comparison of the sentinels of package ecd that the errors of package jacquard and of the reference
# model match.
# Sourced by the rows harness from /w; the harness supplies say, sel, offline, head_of, count, why_of, go_row and
# HAVE_GO.
#
#   AC-01    ark and the reference model answer alike on four seeds of 2,000 random transactions (TestArkCompare)
#   AC-01pm  plant: the reference model treats every query as Exact (model/model.go; TestArkCompare)
#   AC-01pn  plant: the reference model ignores None (model/model.go; TestArkCompare)
#   JC-01ps  plant: a wrong-length row's error also matches ErrBudget; its class stays ErrOp (hydrate.go; TestCompare)
#   JC-01pv  plant: Apply on a stale engine also matches ErrOp; its class stays ErrStale (engine.go; TestCompare)
#
# The rows AC-01, AC-01pm and AC-01pn hold only with the ark adapter of go/enginecmp/ark. The rows JC-01ps and JC-01pv
# hold with the Hydrate and stale-engine comparisons of go/engine/jacquard/compare_test.go.
#
# acmp19_plant id expect test file sedexpr: copies /w/go to a temporary directory, applies the one-line sed to
# <file>, a path under go/, and runs only the named test in go/enginecmp on ./ark/. KILLED: the test failed, and the
# observed text names its first failing line. SURVIVED: it passed. CNM: the sed did not change exactly one line, the
# module was missing, or the test did not run.
acmp19_plant() {
  acmp19_id=$1 acmp19_ex=$2 acmp19_test=$3 acmp19_file=$4 acmp19_sed=$5
  sel "$acmp19_id" acmp19 || return 0
  if [ $HAVE_GO -eq 0 ]; then say "$acmp19_id" "$acmp19_ex" "go not on PATH" CNM; return 0; fi
  acmp19_pd=$(mktemp -d /tmp/rows/plant.XXXXXX)
  cp -a /w/go "$acmp19_pd/go" && cp "$acmp19_pd/go/$acmp19_file" "$acmp19_pd/orig.go"
  (cd "$acmp19_pd/go" && sed -i "$acmp19_sed" "$acmp19_file")
  acmp19_n=$(diff "$acmp19_pd/orig.go" "$acmp19_pd/go/$acmp19_file" | grep -c '^>')
  if [ "$acmp19_n" -ne 1 ]; then
    say "$acmp19_id" "$acmp19_ex" "the sed changed $acmp19_n lines of $acmp19_file, want 1" CNM
  else
    acmp19_log=/tmp/rows/$acmp19_id.log
    (cd "$acmp19_pd/go/enginecmp" && go test -count=1 -v -run "^$acmp19_test\$" ./ark/ >"$acmp19_log" 2>&1)
    acmp19_rc=$?
    if offline "$acmp19_log"; then say "$acmp19_id" "$acmp19_ex" "module not found" CNM
    elif grep -q "^--- FAIL: $acmp19_test" "$acmp19_log"; then
      say "$acmp19_id" "$acmp19_ex" "rc=$acmp19_rc, $acmp19_test FAIL: $(why_of "$acmp19_log")" KILLED
    elif [ $acmp19_rc -eq 0 ] && grep -q "^--- PASS: $acmp19_test" "$acmp19_log"; then
      say "$acmp19_id" "$acmp19_ex" "rc=0, $acmp19_test PASS" SURVIVED
    else say "$acmp19_id" "$acmp19_ex" "rc=$acmp19_rc, the test did not run" CNM; head_of "$acmp19_log" 8; fi
  fi
  rm -rf "$acmp19_pd"
}

# acmp19_jplant id expect test file sedexpr: copies /w/go to a temporary directory, applies the one-line sed to
# engine/jacquard/<file> and runs only the named test there. The verdicts are those of acmp19_plant.
acmp19_jplant() {
  acmp19_id=$1 acmp19_ex=$2 acmp19_test=$3 acmp19_file=$4 acmp19_sed=$5
  sel "$acmp19_id" acmp19 || return 0
  if [ $HAVE_GO -eq 0 ]; then say "$acmp19_id" "$acmp19_ex" "go not on PATH" CNM; return 0; fi
  acmp19_pd=$(mktemp -d /tmp/rows/plant.XXXXXX)
  cp -a /w/go "$acmp19_pd/go" && cp "$acmp19_pd/go/engine/jacquard/$acmp19_file" "$acmp19_pd/orig.go"
  (cd "$acmp19_pd/go/engine/jacquard" && sed -i "$acmp19_sed" "$acmp19_file")
  acmp19_n=$(diff "$acmp19_pd/orig.go" "$acmp19_pd/go/engine/jacquard/$acmp19_file" | grep -c '^>')
  if [ "$acmp19_n" -ne 1 ]; then
    say "$acmp19_id" "$acmp19_ex" "the sed changed $acmp19_n lines of $acmp19_file, want 1" CNM
  else
    acmp19_log=/tmp/rows/$acmp19_id.log
    (cd "$acmp19_pd/go" && go test -count=1 -v -run "^$acmp19_test\$" ./engine/jacquard/ >"$acmp19_log" 2>&1)
    acmp19_rc=$?
    if offline "$acmp19_log"; then say "$acmp19_id" "$acmp19_ex" "module not found" CNM
    elif grep -q "^--- FAIL: $acmp19_test" "$acmp19_log"; then
      say "$acmp19_id" "$acmp19_ex" "rc=$acmp19_rc, $acmp19_test FAIL: $(why_of "$acmp19_log")" KILLED
    elif [ $acmp19_rc -eq 0 ] && grep -q "^--- PASS: $acmp19_test" "$acmp19_log"; then
      say "$acmp19_id" "$acmp19_ex" "rc=0, $acmp19_test PASS" SURVIVED
    else say "$acmp19_id" "$acmp19_ex" "rc=$acmp19_rc, the test did not run" CNM; head_of "$acmp19_log" 8; fi
  fi
  rm -rf "$acmp19_pd"
}

# The sed of JC-01ps, in two parts: the text that the line holds, and the text that replaces it.
acmp19_ps='s/want %d", errOp, k, r\.Entity, r\.CID, len(r\.Value), cd\.size)$/'
acmp19_ps="$acmp19_ps"'want %d: %w", errOp, k, r.Entity, r.CID, len(r.Value), cd.size, ecd.ErrBudget)/'

go_row AC-01 acmp19 "TestArkCompare rc=0" /w/go/enginecmp test -run '^TestArkCompare$' ./ark/
acmp19_plant AC-01pm "TestArkCompare fails on the plant" TestArkCompare engine/jacquard/model/model.go \
  's/return !q\.Exact || len(comps) == len(q\.All)$/return len(comps) == len(q.All)/'
acmp19_plant AC-01pn "TestArkCompare fails on the plant" TestArkCompare engine/jacquard/model/model.go \
  's/if anyOf(comps, q\.None) {/if false {/'
acmp19_jplant JC-01ps "TestCompare fails on the plant" TestCompare hydrate.go "$acmp19_ps"
acmp19_jplant JC-01pv "TestCompare fails on the plant" TestCompare engine.go \
  's/^\t\treturn ecd\.ErrStale$/\t\treturn fmt.Errorf("%w: %w", ecd.ErrStale, ecd.ErrOp)/'
