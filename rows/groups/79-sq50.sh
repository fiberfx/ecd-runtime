# Group sq50 of Ecd Runtime: the engine's Start and the store's Rows. The rows show that every engine of the repository
# brings a new engine to a seq without loading a kind and refuses another seq on an engine that is not new, that Rows
# yields the stored values of given entities of one kind at the store's seq, in eid order and then CID order, that a
# host serves a miss on a Touched kind through Rows and the engine's Hydrate, and that Hydrate with a catalog without
# All kinds brings a new engine to the store's seq through Start.
# Sourced by the rows harness from /w; the harness supplies say, sel, offline, head_of, count, why_of, go_row and
# HAVE_GO.
#
#   ST-01    on each engine case of go/rt (model, jacquard), Start(5) of a new engine sets At() to 5, leaves the All
#            kind Cold and the Touched kind Warm and empty, and Apply(6) succeeds; on an engine hydrated at 3, Start(5)
#            answers ErrGap with At() 3, and Start(3) answers nil and changes nothing (TestStart in go/rt)
#   ST-01p   plant: the reference model reads every engine as new, so Start(5) on an engine hydrated at 3 answers nil
#            (engine/jacquard/model/model.go; TestStart, case model)
#   ST-01q   plant: Jacquard reads every engine as new, so Start(5) on an engine hydrated at 3 answers nil
#            (engine/jacquard/hydrate.go; TestStart, case jacquard)
#   ST-02    the cases of ST-01 on the ark adapter (TestStart in go/enginecmp/ark)
#   ST-02p   plant: the ark adapter reads every engine as new, so Start(5) on an engine hydrated at 3 answers nil
#            (enginecmp/ark/ark.go; TestStart)
#   SQ-20    Rows with ids unsorted and one repeated yields exactly the live values of the entities, each once, in eid
#            order and then CID order, at the seq of the last commit; an eid of another kind is an error before f is
#            called, and an error of f is returned unchanged (TestRows)
#   SQ-20p   plant: Rows does not sort the ids, so the rows come out of eid order and the repeated id twice
#            (sqlite/read.go; TestRows)
#   SQ-20q   plant: Rows does not check the kind of the ids, so an eid of another kind reaches f (sqlite/read.go;
#            TestRows)
#   SQ-21    a miss on a Touched entity is served by Rows into the engine's Hydrate and Get answers the stored value;
#            after a commit the engine did not apply, the engine's Hydrate answers ErrGap (TestRowsHydrateMiss)
#   SQ-22    Hydrate with a catalog of no kind brings a new engine to the store's seq with every All kind Cold and
#            refuses an engine at another seq with ErrGap; a catalog of Touched kinds only does the same with the
#            Touched kinds Warm and empty (TestHydrateAllOnly, TestHydrateTouchedOnly)
#   SQ-22p   plant: Hydrate starts the engine at seq 0, so At() stays 0 (sqlite/read.go; TestHydrateAllOnly)
#
# Rows SQ-20 to SQ-22 load the driver and its modules from the module inputs, as the rows of group sq40 do.
#
# sq50_plant id expect mod pkg test case file sedexpr: copies /w/go to a temporary directory, applies the one-line sed
# to go/<file> there and runs only the named test of package pkg in module directory go/<mod> of the copy. The copy is
# the whole of /w/go, since the nested modules replace the main module by a parent directory. KILLED: the test failed
# in its case (the whole test when case is empty), and the observed text names its first failing line. SURVIVED: it
# passed, or failed in another case only. CNM: the sed did not change exactly one line, the file or a module was
# missing, the test panicked, or the test did not run, which includes a mutant that does not compile.
sq50_plant() {
  sq50_id=$1 sq50_ex=$2 sq50_mod=$3 sq50_pkg=$4 sq50_test=$5 sq50_case=$6 sq50_file=$7 sq50_sed=$8
  sel "$sq50_id" sq50 || return 0
  if [ $HAVE_GO -eq 0 ]; then say "$sq50_id" "$sq50_ex" "go not on PATH" CNM; return 0; fi
  sq50_name=$sq50_test
  if [ -n "$sq50_case" ]; then sq50_name=$sq50_test/$sq50_case; fi
  sq50_pd=$(mktemp -d /tmp/rows/plant.XXXXXX)
  cp -a /w/go "$sq50_pd/go"
  sq50_f=$sq50_pd/go/$sq50_file
  if [ ! -f "$sq50_f" ]; then
    say "$sq50_id" "$sq50_ex" "go/$sq50_file is absent" CNM
    rm -rf "$sq50_pd"
    return 0
  fi
  cp "$sq50_f" "$sq50_pd/orig.go"
  sed -i "$sq50_sed" "$sq50_f"
  sq50_n=$(diff "$sq50_pd/orig.go" "$sq50_f" | grep -c '^>')
  if [ "$sq50_n" -ne 1 ]; then
    say "$sq50_id" "$sq50_ex" "the sed changed $sq50_n lines of go/$sq50_file, want 1" CNM
  else
    sq50_log=/tmp/rows/$sq50_id.log
    (cd "$sq50_pd/go/$sq50_mod" && go test -count=1 -v -run "^$sq50_test\$" "$sq50_pkg" >"$sq50_log" 2>&1)
    sq50_rc=$?
    if offline "$sq50_log"; then say "$sq50_id" "$sq50_ex" "module not found" CNM
    elif grep -q '^panic:' "$sq50_log"; then
      say "$sq50_id" "$sq50_ex" "rc=$sq50_rc, $sq50_test panicked: $(why_of "$sq50_log")" CNM
    elif grep -q -E "^ *--- FAIL: $sq50_name " "$sq50_log"; then
      say "$sq50_id" "$sq50_ex" "rc=$sq50_rc, $sq50_name FAIL: $(why_of "$sq50_log")" KILLED
    elif grep -q "^--- FAIL: $sq50_test " "$sq50_log"; then
      say "$sq50_id" "$sq50_ex" "rc=$sq50_rc, $sq50_test FAIL, but not in $sq50_name" SURVIVED
    elif [ $sq50_rc -eq 0 ] && grep -q "^--- PASS: $sq50_test " "$sq50_log"; then
      say "$sq50_id" "$sq50_ex" "rc=0, $sq50_test PASS" SURVIVED
    else say "$sq50_id" "$sq50_ex" "rc=$sq50_rc, the test did not run" CNM; head_of "$sq50_log" 8; fi
  fi
  rm -rf "$sq50_pd"
}

# The "not new" test of each engine: one if line that ends with the comment "// start: not new". The plant keeps
# every variable of the condition used, so the mutant compiles.
sq50_notnew='s/^\(\t*\)if \(.*\) { \/\/ start: not new$/\1if (\2) \&\& false { \/\/ start: not new/'

go_row ST-01 sq50 "TestStart rc=0" /w/go test -run '^TestStart$' ./rt/
sq50_plant ST-01p "TestStart/model/hydrated fails on the plant" . ./rt/ TestStart model/hydrated \
  engine/jacquard/model/model.go "$sq50_notnew"
sq50_plant ST-01q "TestStart/jacquard/hydrated fails on the plant" . ./rt/ TestStart jacquard/hydrated \
  engine/jacquard/hydrate.go "$sq50_notnew"

go_row ST-02 sq50 "ark TestStart rc=0" /w/go/enginecmp test -run '^TestStart$' ./ark/
sq50_plant ST-02p "ark TestStart fails on the plant" enginecmp ./ark/ TestStart '' enginecmp/ark/ark.go "$sq50_notnew"

go_row SQ-20 sq50 "TestRows rc=0" /w/go/sqlite test -run '^TestRows$' .
sq50_plant SQ-20p "TestRows fails on the plant" sqlite . TestRows '' sqlite/read.go \
  's/\(slices\.SortFunc(ids, func(a, b eid\.ID) int { return bytes\.Compare(a\[:\], b\[:\]) })\)$/if false { \1 }/'
sq50_plant SQ-20q "TestRows fails on the plant" sqlite . TestRows '' sqlite/read.go \
  's/^\(\t*\)if ecd\.KindOf(id) != k {$/\1if ecd.KindOf(id) != k \&\& false {/'

go_row SQ-21 sq50 "TestRowsHydrateMiss rc=0" /w/go/sqlite test -run '^TestRowsHydrateMiss$' .

go_row SQ-22 sq50 "TestHydrateAllOnly, TestHydrateTouchedOnly rc=0" /w/go/sqlite test \
  -run '^(TestHydrateAllOnly|TestHydrateTouchedOnly)$' .
sq50_plant SQ-22p "TestHydrateAllOnly fails on the plant" sqlite . TestHydrateAllOnly '' sqlite/read.go \
  's/e\.Start(seq)/e.Start(0)/'
