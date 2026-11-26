# Group mq60 of Entity Component Runtime: the SQL twin of the declared queries. The rows show that every declared query
# of the store's catalog answers on each engine of the repository what its SQL twin over the store's tables answers at
# the same seq, in eid order, with Count the number of IDs, and that a write into a returned answer changes no later
# answer. Row L12-model of group j0m runs the model's case of the same test; its plants are here.
# Sourced by the rows harness from /w; the harness supplies say, sel, offline, head_of, count, why_of, go_row and
# HAVE_GO.
#
#   MQ-01         on Jacquard, every declared query of the store's catalog answers what its SQL twin answers at the
#                 store's seq, on 100,000 entities of kind aaa and 1,000 of kind ccc after destroys, unsets and sets
#                 (TestQueryTwin, case jacquard, in go/sqlite)
#   MQ-01p        plant: Jacquard does not sort the eids of an answer, so they come in table order
#                 (engine/jacquard/query.go; TestQueryTwin, case jacquard/q0)
#   MQ-01q        plant: Jacquard sets Count to the number of eids it added, so an answer appended to one eid and
#                 Count 1 has Count one short (engine/jacquard/query.go; TestQueryTwin, case jacquard/q0)
#   L12-model-p   plant: the reference model does not sort the eids of an answer, so they come in map order
#                 (engine/jacquard/model/model.go; TestQueryTwin, case model/q0)
#   L12-model-px  plant: the reference model ignores Exact, so query 5 answers entities with components beyond
#                 component 1 (engine/jacquard/model/model.go; TestQueryTwin, case model/q5)
#   L12-model-pc  plant: the reference model sets Count to the number of eids it added, so an answer appended to one
#                 eid and Count 1 has Count one short (engine/jacquard/model/model.go; TestQueryTwin, case model/q0)
#
# The rows load the driver and its modules from the module inputs, as the rows of group sq40 do.
#
# mq60_plant id expect mod pkg test case file sedexpr: copies /w/go to a temporary directory, applies the one-line sed
# to go/<file> there and runs only the named test of package pkg in module directory go/<mod> of the copy. The copy is
# the whole of /w/go, since the nested modules replace the main module by a parent directory. KILLED: the test failed
# in its case (the whole test when case is empty), and the observed text names its first failing line. SURVIVED: it
# passed, or failed in another case only. CNM: the sed did not change exactly one line, the file or a module was
# missing, the test panicked, or the test did not run, which includes a mutant that does not compile.
mq60_plant() {
  mq60_id=$1 mq60_ex=$2 mq60_mod=$3 mq60_pkg=$4 mq60_test=$5 mq60_case=$6 mq60_file=$7 mq60_sed=$8
  sel "$mq60_id" mq60 || return 0
  if [ $HAVE_GO -eq 0 ]; then say "$mq60_id" "$mq60_ex" "go not on PATH" CNM; return 0; fi
  mq60_name=$mq60_test
  if [ -n "$mq60_case" ]; then mq60_name=$mq60_test/$mq60_case; fi
  mq60_pd=$(mktemp -d /tmp/rows/plant.XXXXXX)
  cp -a /w/go "$mq60_pd/go"
  mq60_f=$mq60_pd/go/$mq60_file
  if [ ! -f "$mq60_f" ]; then
    say "$mq60_id" "$mq60_ex" "go/$mq60_file is absent" CNM
    rm -rf "$mq60_pd"
    return 0
  fi
  cp "$mq60_f" "$mq60_pd/orig.go"
  sed -i "$mq60_sed" "$mq60_f"
  mq60_n=$(diff "$mq60_pd/orig.go" "$mq60_f" | grep -c '^>')
  if [ "$mq60_n" -ne 1 ]; then
    say "$mq60_id" "$mq60_ex" "the sed changed $mq60_n lines of go/$mq60_file, want 1" CNM
  else
    mq60_log=/tmp/rows/$mq60_id.log
    (cd "$mq60_pd/go/$mq60_mod" && go test -count=1 -v -run "^$mq60_test\$" "$mq60_pkg" >"$mq60_log" 2>&1)
    mq60_rc=$?
    if offline "$mq60_log"; then say "$mq60_id" "$mq60_ex" "module not found" CNM
    elif grep -q '^panic:' "$mq60_log"; then
      say "$mq60_id" "$mq60_ex" "rc=$mq60_rc, $mq60_test panicked: $(why_of "$mq60_log")" CNM
    elif grep -q -E "^ *--- FAIL: $mq60_name " "$mq60_log"; then
      say "$mq60_id" "$mq60_ex" "rc=$mq60_rc, $mq60_name FAIL: $(why_of "$mq60_log")" KILLED
    elif grep -q "^--- FAIL: $mq60_test " "$mq60_log"; then
      say "$mq60_id" "$mq60_ex" "rc=$mq60_rc, $mq60_test FAIL, but not in $mq60_name" SURVIVED
    elif [ $mq60_rc -eq 0 ] && grep -q "^--- PASS: $mq60_test " "$mq60_log"; then
      say "$mq60_id" "$mq60_ex" "rc=0, $mq60_test PASS" SURVIVED
    else say "$mq60_id" "$mq60_ex" "rc=$mq60_rc, the test did not run" CNM; head_of "$mq60_log" 8; fi
  fi
  rm -rf "$mq60_pd"
}

# The sort and the count of an answer, one line each in both engines.
mq60_sort='s/slices\.SortFunc(added, eid\.ID\.Compare)/slices.SortFunc(added[:0], eid.ID.Compare)/'
mq60_count='s/out\.Count += int64(len(added))/out.Count = int64(len(added))/'

go_row MQ-01 mq60 "TestQueryTwin/jacquard rc=0" /w/go/sqlite test -run '^TestQueryTwin$/^jacquard$' .
mq60_plant MQ-01p "TestQueryTwin/jacquard/q0 fails on the plant" sqlite . TestQueryTwin jacquard/q0 \
  engine/jacquard/query.go "$mq60_sort"
mq60_plant MQ-01q "TestQueryTwin/jacquard/q0 fails on the plant" sqlite . TestQueryTwin jacquard/q0 \
  engine/jacquard/query.go "$mq60_count"

mq60_plant L12-model-p "TestQueryTwin/model/q0 fails on the plant" sqlite . TestQueryTwin model/q0 \
  engine/jacquard/model/model.go "$mq60_sort"
mq60_plant L12-model-px "TestQueryTwin/model/q5 fails on the plant" sqlite . TestQueryTwin model/q5 \
  engine/jacquard/model/model.go 's/return !q\.Exact || len(comps) == len(q\.All)$/return true/'
mq60_plant L12-model-pc "TestQueryTwin/model/q0 fails on the plant" sqlite . TestQueryTwin model/q0 \
  engine/jacquard/model/model.go "$mq60_count"
