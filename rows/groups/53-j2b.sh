# Group j2b of Ecd Runtime: archetypes and the graph of package jacquard.
# Sourced by rows/loom.rows.sh from /w; the harness supplies say, sel, offline, head_of, count, tree_of, go_row and HAVE_GO.
#
#   J04      after random withAdd/withRemove walks over 3 kinds, every edge equals find(kind, mask with the bit toggled)
#            and has its back edge (TestGraphEdges, JI3)
#   J04p     plant: graph.go toggle stores the source node as the edge target, f.setEdge(c, f.id); TestGraphEdges must fail
#   J2B-01   nodes of two kinds with equal masks are two nodes, and every node reachable from a kind's empty node has
#            that kind (TestKindsSeparate, JI2)
#   J2B-01p  plant: key builds nodeKey{0, *m}, so the kind leaves the key; TestKindsSeparate must fail
#   J2B-02   cols holds the set bits of the mask that are not tags, ascending (TestArchCols)
#   J2B-02p  plant: newArchetype keeps tag bits (if tags.get(c) becomes if false); TestArchCols must fail
#   J2B-03   ids start at 1 in creation order, nodes are never removed, a kind has one empty node (TestNodeIDs)
#   J2B-03p  plant: empty calls create, so each call adds a node; TestNodeIDs must fail
#   J2B-04   a *archetype taken before 1,000 finds of new masks is the node arch answers afterwards, with equal fields
#            (TestArchStable)
#   J2B-04p  plant: arch answers a copy of the node; TestArchStable must fail
#   J2B-05   withAdd of a set bit and withRemove of a clear bit answer from, change len() and edges not at all and do
#            not panic (TestToggleNoOp)
#   J2B-05p  plant: the guard of withAdd never holds, so a set bit toggles; TestToggleNoOp must fail
#   J2B-06   lookup of an unseen key answers (0, false) and leaves len() unchanged; after find it answers the same id;
#            two finds of one key create one node (TestLookup)
#   J2B-06p  plant: lookup answers find's result, which creates the node; TestLookup must fail
#
# j2b_plant id expect test file sedexpr: copies /w/go to a temporary directory, applies sedexpr to one file of
# engine/jacquard and runs only the named test there. KILLED: the test failed. SURVIVED: it passed.
# CNM: the sed changed nothing, the module was missing, or the test did not run.
j2b_plant() {
  j2b_id=$1 j2b_ex=$2 j2b_test=$3 j2b_file=$4 j2b_sed=$5
  sel "$j2b_id" j2b || return 0
  if [ $HAVE_GO -eq 0 ]; then say "$j2b_id" "$j2b_ex" "go not on PATH" CNM; return 0; fi
  j2b_pd=$(mktemp -d /tmp/rows/plant.XXXXXX)
  cp -a /w/go "$j2b_pd/go" && cp -a "$j2b_pd/go/engine/jacquard" "$j2b_pd/orig"
  (cd "$j2b_pd/go/engine/jacquard" && sed -i "$j2b_sed" "$j2b_file")
  if diff -rq "$j2b_pd/orig" "$j2b_pd/go/engine/jacquard" >/dev/null 2>&1; then
    say "$j2b_id" "$j2b_ex" "the sed changed nothing" CNM
  else
    j2b_log=/tmp/rows/$j2b_id.log
    (cd "$j2b_pd/go" && go test -count=1 -v -run "^$j2b_test\$" ./engine/jacquard/ >"$j2b_log" 2>&1); j2b_rc=$?
    if offline "$j2b_log"; then say "$j2b_id" "$j2b_ex" "module not found" CNM
    elif grep -q "^--- FAIL: $j2b_test" "$j2b_log"; then say "$j2b_id" "$j2b_ex" "rc=$j2b_rc, $j2b_test FAIL" KILLED
    elif [ $j2b_rc -eq 0 ] && grep -q "^--- PASS: $j2b_test" "$j2b_log"; then say "$j2b_id" "$j2b_ex" "rc=0, $j2b_test PASS" SURVIVED
    else say "$j2b_id" "$j2b_ex" "rc=$j2b_rc, the test did not run" CNM; head_of "$j2b_log" 8; fi
  fi
  rm -rf "$j2b_pd"
}

go_row J04 j2b "TestGraphEdges rc=0" /w/go test -run '^TestGraphEdges$' ./engine/jacquard/
j2b_plant J04p "TestGraphEdges fails on the plant" TestGraphEdges graph.go 's/^\tf\.setEdge(c, t)$/\tf.setEdge(c, f.id)/'

go_row J2B-01 j2b "TestKindsSeparate rc=0" /w/go test -run '^TestKindsSeparate$' ./engine/jacquard/
j2b_plant J2B-01p "TestKindsSeparate fails on the plant" TestKindsSeparate graph.go 's/return nodeKey{kind, \*m}/return nodeKey{0, *m}/'

go_row J2B-02 j2b "TestArchCols rc=0" /w/go test -run '^TestArchCols$' ./engine/jacquard/
j2b_plant J2B-02p "TestArchCols fails on the plant" TestArchCols archetype.go 's/^\t\tif tags\.get(c) {$/\t\tif false {/'

go_row J2B-03 j2b "TestNodeIDs rc=0" /w/go test -run '^TestNodeIDs$' ./engine/jacquard/
j2b_plant J2B-03p "TestNodeIDs fails on the plant" TestNodeIDs graph.go 's/return g.find(kind, &mask{})/return g.create(kind, \&mask{})/'

go_row J2B-04 j2b "TestArchStable rc=0" /w/go test -run '^TestArchStable$' ./engine/jacquard/
j2b_plant J2B-04p "TestArchStable fails on the plant" TestArchStable graph.go 's/^\treturn g\.nodes\[id\]$/\tn := *g.nodes[id]; return \&n/'

go_row J2B-05 j2b "TestToggleNoOp rc=0" /w/go test -run '^TestToggleNoOp$' ./engine/jacquard/
j2b_plant J2B-05p "TestToggleNoOp fails on the plant" TestToggleNoOp graph.go 's/^\tif f\.mask\.get(c) {$/\tif false {/'

go_row J2B-06 j2b "TestLookup rc=0" /w/go test -run '^TestLookup$' ./engine/jacquard/
j2b_plant J2B-06p "TestLookup fails on the plant" TestLookup graph.go 's/^\tid, ok := g\.byKey\[key(kind, m)\]$/\tid, ok := g.find(kind, m), true/'
