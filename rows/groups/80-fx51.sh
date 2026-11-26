# Group fx51 of Entity Component Runtime: Start on a closed engine, a deed with no op, and the wrap of Hydrate's start
# error. The rows show that every engine of the repository is Stale once closed and answers Start with ErrStale, on an
# engine that was new and on one at the seq of the call; that the store commits a deed with no op, whose body is of
# length 0 and not NULL, and Log yields it with no op; and that Hydrate wraps an error of e.Start with the prefix
# "sqlite: hydrate: start: ".
# Sourced by the rows harness from /w; the harness supplies say, sel, offline, head_of, count, why_of, go_row and
# HAVE_GO.
#
#   ST-03    on each engine case of go/rt (model, jacquard), a closed new engine is Stale and Start(5) answers
#            ErrStale; a closed engine hydrated at 3 answers Start(3) with ErrStale (TestStartClosed in go/rt)
#   ST-03p   plant: the reference model's Start does not consult its gate, so a closed new engine takes Start(5)
#            (engine/jacquard/model/model.go; TestStartClosed, case model)
#   ST-03q   plant: Jacquard's Start does not test its stale flag, so a closed new engine takes Start(5)
#            (engine/jacquard/hydrate.go; TestStartClosed, case jacquard)
#   ST-04    the cases of ST-03 on the ark adapter and the reference model (TestStartClosed in go/enginecmp/ark)
#   ST-04p   plant: the ark adapter's Start does not consult its gate, so a closed new engine takes Start(5)
#            (enginecmp/ark/ark.go; TestStartClosed, case ark)
#   SQ-30    Commit of one deed with no op answers Committed at seq 1, and Log yields the deed with its eid, its Ord
#            and no op (TestCommitEmptyDeed)
#   SQ-30p   plant: the body of a deed with no op is nil, which binds NULL, so the commit fails
#            (sqlite/commit.go; TestCommitEmptyDeed)
#   SQ-22q   plant: Hydrate wraps an error of e.Start without "start: ", so the error does not begin with the
#            prefix (sqlite/read.go; TestHydrateAllOnly)
#   SQ-22r   plant: the same wrap, seen by the Touched-only catalog (sqlite/read.go; TestHydrateTouchedOnly)
#
# Rows SQ-30, SQ-22q and SQ-22r load the driver and its modules from the module inputs, as the rows of group sq40
# do.
#
# fx51_plant id expect mod pkg test case file sedexpr: copies /w/go to a temporary directory, applies the one-line sed
# to go/<file> there and runs only the named test of package pkg in module directory go/<mod> of the copy. The copy is
# the whole of /w/go, since the nested modules replace the main module by a parent directory. KILLED: the test failed
# in its case (the whole test when case is empty), and the observed text names its first failing line. SURVIVED: it
# passed, or failed in another case only. CNM: the sed did not change exactly one line, the file or a module was
# missing, the test panicked, or the test did not run, which includes a mutant that does not compile.
fx51_plant() {
  fx51_id=$1 fx51_ex=$2 fx51_mod=$3 fx51_pkg=$4 fx51_test=$5 fx51_case=$6 fx51_file=$7 fx51_sed=$8
  sel "$fx51_id" fx51 || return 0
  if [ $HAVE_GO -eq 0 ]; then say "$fx51_id" "$fx51_ex" "go not on PATH" CNM; return 0; fi
  fx51_name=$fx51_test
  if [ -n "$fx51_case" ]; then fx51_name=$fx51_test/$fx51_case; fi
  fx51_pd=$(mktemp -d /tmp/rows/plant.XXXXXX)
  cp -a /w/go "$fx51_pd/go"
  fx51_f=$fx51_pd/go/$fx51_file
  if [ ! -f "$fx51_f" ]; then
    say "$fx51_id" "$fx51_ex" "go/$fx51_file is absent" CNM
    rm -rf "$fx51_pd"
    return 0
  fi
  cp "$fx51_f" "$fx51_pd/orig.go"
  sed -i "$fx51_sed" "$fx51_f"
  fx51_n=$(diff "$fx51_pd/orig.go" "$fx51_f" | grep -c '^>')
  if [ "$fx51_n" -ne 1 ]; then
    say "$fx51_id" "$fx51_ex" "the sed changed $fx51_n lines of go/$fx51_file, want 1" CNM
  else
    fx51_log=/tmp/rows/$fx51_id.log
    (cd "$fx51_pd/go/$fx51_mod" && go test -count=1 -v -run "^$fx51_test\$" "$fx51_pkg" >"$fx51_log" 2>&1)
    fx51_rc=$?
    if offline "$fx51_log"; then say "$fx51_id" "$fx51_ex" "module not found" CNM
    elif grep -q '^panic:' "$fx51_log"; then
      say "$fx51_id" "$fx51_ex" "rc=$fx51_rc, $fx51_test panicked: $(why_of "$fx51_log")" CNM
    elif grep -q -E "^ *--- FAIL: $fx51_name " "$fx51_log"; then
      say "$fx51_id" "$fx51_ex" "rc=$fx51_rc, $fx51_name FAIL: $(why_of "$fx51_log")" KILLED
    elif grep -q "^--- FAIL: $fx51_test " "$fx51_log"; then
      say "$fx51_id" "$fx51_ex" "rc=$fx51_rc, $fx51_test FAIL, but not in $fx51_name" SURVIVED
    elif [ $fx51_rc -eq 0 ] && grep -q "^--- PASS: $fx51_test " "$fx51_log"; then
      say "$fx51_id" "$fx51_ex" "rc=0, $fx51_test PASS" SURVIVED
    else say "$fx51_id" "$fx51_ex" "rc=$fx51_rc, the test did not run" CNM; head_of "$fx51_log" 8; fi
  fi
  rm -rf "$fx51_pd"
}

go_row ST-03 fx51 "TestStartClosed rc=0" /w/go test -run '^TestStartClosed$' ./rt/
fx51_plant ST-03p "TestStartClosed/model fails on the plant" . ./rt/ TestStartClosed model \
  engine/jacquard/model/model.go \
  '/^func (m \*Model) Start(/,/^}/s/m\.gate(); err != nil {$/m.gate(); err != nil \&\& false {/'
fx51_plant ST-03q "TestStartClosed/jacquard fails on the plant" . ./rt/ TestStartClosed jacquard \
  engine/jacquard/hydrate.go '/^func (en \*Engine) Start(/,/^}/s/^\(\t*\)if en\.stale {$/\1if en.stale \&\& false {/'

go_row ST-04 fx51 "ark TestStartClosed rc=0" /w/go/enginecmp test -run '^TestStartClosed$' ./ark/
fx51_plant ST-04p "ark TestStartClosed/ark fails on the plant" enginecmp ./ark/ TestStartClosed ark \
  enginecmp/ark/ark.go \
  '/^func (e \*Engine) Start(/,/^}/s/e\.gate(); err != nil {$/e.gate(); err != nil \&\& false {/'

go_row SQ-30 fx51 "TestCommitEmptyDeed rc=0" /w/go/sqlite test -run '^TestCommitEmptyDeed$' .
fx51_plant SQ-30p "TestCommitEmptyDeed fails on the plant" sqlite . TestCommitEmptyDeed '' sqlite/commit.go \
  's/^\(\t*\)b := make(\[\]byte, 0, n)$/\1b := append([]byte(nil), make([]byte, 0, n)...)/'

fx51_plant SQ-22q "TestHydrateAllOnly fails on the plant" sqlite . TestHydrateAllOnly '' sqlite/read.go \
  's/"sqlite: hydrate: start: %w"/"sqlite: hydrate: %w"/'
fx51_plant SQ-22r "TestHydrateTouchedOnly fails on the plant" sqlite . TestHydrateTouchedOnly '' sqlite/read.go \
  's/"sqlite: hydrate: start: %w"/"sqlite: hydrate: %w"/'
