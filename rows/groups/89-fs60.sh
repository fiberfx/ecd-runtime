# Group fs60 of Entity Component Runtime: story 23, the eids of the spawns of a flush. Result.Spawned answers, by its
# placeholder, the eid that the flush minted for a spawn of a deed answered Committed, equal to the spawn in the
# committed deed and in record order; it refuses with ErrRef a Ref that is not a placeholder, a placeholder of another
# buffer or of another flush, and the spawn of a deed answered Conflict, Refused or Unknown. The number 89 is above
# every group file present.
# Sourced by the rows harness from /w; the harness supplies say, sel, offline, head_of, count, why_of, go_row and
# HAVE_GO.
#
#   FS-01    go test -run '^TestSpawned$' ./rt/: on model and jacquard, the eids that Spawned answers for two deeds
#            equal the spawns of the committed deeds in record order and the engine holds their values; a deed
#            answered Conflict and the Refs of another flush, of a buffer with no deed, of an eid, the zero Ref and
#            the zero Result answer ErrRef
#   FS-01p   plant: Spawned does not compare the epoch of a placeholder (rt/spawned.go; TestSpawned, case refs)
#   FS-01q   plant: Spawned answers the spawn of a deed not answered Committed (rt/spawned.go; TestSpawned, case
#            conflict)
#   FS-01r   plant: Spawned answers the spawns in reverse record order (rt/spawned.go; TestSpawned, case model)
#   FS-02    go test -run '^TestSpawnedLog$' . in go/sqlite: on the SQLite store, the eids that Spawned answers equal
#            the spawns of each deed in the store's log; the spawn of a deed the store answered Refused answers ErrRef
#            beside the committed deed of the same flush
#   FS-02p   plant: Spawned answers the spawn of a deed not answered Committed (rt/spawned.go; TestSpawnedLog, case
#            refused)
#
# The facade answers through the alias ecr.Result: Example_spawned in go/ecr is run by FC-01.
#
# fs60_plant id expect mod pkg test case file sedexpr: copies /w/go to a temporary directory, applies the one-line sed
# to go/<file> there and runs only the named test of package pkg in module directory go/<mod> of the copy. The copy is
# the whole of /w/go, since the nested modules replace the main module by a parent directory. KILLED: the test failed
# in its case (the whole test when case is empty), and the observed text names its first failing line. SURVIVED: it
# passed, or failed in another case only. CNM: the sed did not change exactly one line, the file or a module was
# missing, the test panicked, or the test did not run, which includes a mutant that does not compile.
fs60_plant() {
  fs60_id=$1 fs60_ex=$2 fs60_mod=$3 fs60_pkg=$4 fs60_test=$5 fs60_case=$6 fs60_file=$7 fs60_sed=$8
  sel "$fs60_id" fs60 || return 0
  if [ $HAVE_GO -eq 0 ]; then say "$fs60_id" "$fs60_ex" "go not on PATH" CNM; return 0; fi
  fs60_name=$fs60_test
  if [ -n "$fs60_case" ]; then fs60_name=$fs60_test/$fs60_case; fi
  fs60_pd=$(mktemp -d /tmp/rows/plant.XXXXXX)
  cp -a /w/go "$fs60_pd/go"
  fs60_f=$fs60_pd/go/$fs60_file
  if [ ! -f "$fs60_f" ]; then
    say "$fs60_id" "$fs60_ex" "go/$fs60_file is absent" CNM
    rm -rf "$fs60_pd"
    return 0
  fi
  cp "$fs60_f" "$fs60_pd/orig.go"
  sed -i "$fs60_sed" "$fs60_f"
  fs60_n=$(diff "$fs60_pd/orig.go" "$fs60_f" | grep -c '^>')
  if [ "$fs60_n" -ne 1 ]; then
    say "$fs60_id" "$fs60_ex" "the sed changed $fs60_n lines of go/$fs60_file, want 1" CNM
  else
    fs60_log=/tmp/rows/$fs60_id.log
    (cd "$fs60_pd/go/$fs60_mod" && go test -count=1 -v -run "^$fs60_test\$" "$fs60_pkg" >"$fs60_log" 2>&1)
    fs60_rc=$?
    if offline "$fs60_log"; then say "$fs60_id" "$fs60_ex" "module not found" CNM
    elif grep -q '^panic:' "$fs60_log"; then
      say "$fs60_id" "$fs60_ex" "rc=$fs60_rc, $fs60_test panicked: $(why_of "$fs60_log")" CNM
    elif grep -q -E "^ *--- FAIL: $fs60_name " "$fs60_log"; then
      say "$fs60_id" "$fs60_ex" "rc=$fs60_rc, $fs60_name FAIL: $(why_of "$fs60_log")" KILLED
    elif grep -q "^--- FAIL: $fs60_test " "$fs60_log"; then
      say "$fs60_id" "$fs60_ex" "rc=$fs60_rc, $fs60_test FAIL, but not in $fs60_name" SURVIVED
    elif [ $fs60_rc -eq 0 ] && grep -q "^--- PASS: $fs60_test " "$fs60_log"; then
      say "$fs60_id" "$fs60_ex" "rc=0, $fs60_test PASS" SURVIVED
    else say "$fs60_id" "$fs60_ex" "rc=$fs60_rc, the test did not run" CNM; head_of "$fs60_log" 8; fi
  fi
  rm -rf "$fs60_pd"
}

go_row FS-01 fs60 "TestSpawned rc=0" /w/go test -run '^TestSpawned$' ./rt/
fs60_plant FS-01p "TestSpawned/refs fails on the plant" . ./rt/ TestSpawned refs rt/spawned.go \
  's/if s\.buf != ref\.buf || s\.epoch != ref\.epoch {/if s.buf != ref.buf {/'
fs60_plant FS-01q "TestSpawned/conflict fails on the plant" . ./rt/ TestSpawned conflict rt/spawned.go \
  's/if r\.Outcomes\[s\.ord\] != Committed {/if false {/'
fs60_plant FS-01r "TestSpawned/model fails on the plant" . ./rt/ TestSpawned model rt/spawned.go \
  's/return s\.ids\[ref\.n\], nil/return s.ids[len(s.ids)-1-int(ref.n)], nil/'

go_row FS-02 fs60 "TestSpawnedLog rc=0" /w/go/sqlite test -run '^TestSpawnedLog$' .
fs60_plant FS-02p "TestSpawnedLog/refused fails on the plant" sqlite . TestSpawnedLog refused rt/spawned.go \
  's/if r\.Outcomes\[s\.ord\] != Committed {/if false {/'
