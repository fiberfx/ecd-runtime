# Group j5h of Ecd Runtime: the hydration and the eviction of the Engine of package jacquard, checked against the reference model.
# Sourced by the rows harness from /w; the harness supplies say, sel, offline, head_of, count, tree_of, go_row and HAVE_GO.
#
#   J5H-01   Hydrate and Evict of an Engine with a residency declaration answer as the reference model does, on a Cold All
#            kind, a Warm All kind and a Touched kind: the seq rule, the replacement of an All kind and of a resident
#            Touched entity, rows out of order with a repeated component, the refusals of an undeclared kind, an All kind
#            and a stale engine, the evictions counted, a new engine that takes the seq of its first hydration, unload,
#            and seeded random calls; after each call the seq, the state, the entities and the evictions of every kind,
#            every component and every declared query read alike (TestJ5HHydrate)
#   J5H-02   a refused or failed hydration changes nothing: a wrong seq, a row of another kind, an undeclared CID, a value
#            of the wrong length, a tag with a value, an error of the row source after some rows, and an undeclared kind,
#            each on a Cold All kind, a Warm All kind and a Touched kind; the answer matches its error, the kind keeps its
#            state, the Engine is not stale, an entity resident before the call answers its old value, the whole Engine
#            reads as before the call, and a good hydration of the kind is then accepted (TestJ5HRefused)
#   J5H-03   a hydration of an evicted or a resident Touched entity, and the replacement of an All kind, give a new Entity;
#            the earlier handle is not valid and the answers equal the model's (TestJ5HRehydrate)
#   J5H-01p  plant: Evict does not count the entities that it drops (hydrate.go)
#   J5H-02p  plant: the length of a hydrated value is not checked against the component's size (hydrate.go)
#   J5H-02pk plant: a row of another kind is not refused (hydrate.go)
#   J5H-02pa plant: a hydration at a seq other than the Engine's is not refused (hydrate.go)
#   J5H-03p  plant: a resident entity is not freed before it is spawned again (hydrate.go)
#   J5H-04p  plant: a hydrated value is kept as the row source hands it out, not copied (hydrate.go)
#
# j5h_plant id expect test file sedexpr: copies /w/go to a temporary directory, applies the one-line sed to
# engine/jacquard/<file> and runs only the named test there. KILLED: the test failed. SURVIVED: it passed.
# CNM: the sed did not change exactly one line, the module was missing, or the test did not run.
j5h_plant() {
  j5h_id=$1 j5h_ex=$2 j5h_test=$3 j5h_file=$4 j5h_sed=$5
  sel "$j5h_id" j5h || return 0
  if [ $HAVE_GO -eq 0 ]; then say "$j5h_id" "$j5h_ex" "go not on PATH" CNM; return 0; fi
  j5h_pd=$(mktemp -d /tmp/rows/plant.XXXXXX)
  cp -a /w/go "$j5h_pd/go" && cp "$j5h_pd/go/engine/jacquard/$j5h_file" "$j5h_pd/orig.go"
  (cd "$j5h_pd/go/engine/jacquard" && sed -i "$j5h_sed" "$j5h_file")
  j5h_n=$(diff "$j5h_pd/orig.go" "$j5h_pd/go/engine/jacquard/$j5h_file" | grep -c '^>')
  if [ "$j5h_n" -ne 1 ]; then
    say "$j5h_id" "$j5h_ex" "the sed changed $j5h_n lines of $j5h_file, want 1" CNM
  else
    j5h_log=/tmp/rows/$j5h_id.log
    (cd "$j5h_pd/go" && go test -count=1 -v -run "^$j5h_test\$" ./engine/jacquard/ >"$j5h_log" 2>&1); j5h_rc=$?
    if offline "$j5h_log"; then say "$j5h_id" "$j5h_ex" "module not found" CNM
    elif grep -q "^--- FAIL: $j5h_test" "$j5h_log"; then say "$j5h_id" "$j5h_ex" "rc=$j5h_rc, $j5h_test FAIL" KILLED
    elif [ $j5h_rc -eq 0 ] && grep -q "^--- PASS: $j5h_test" "$j5h_log"; then say "$j5h_id" "$j5h_ex" "rc=0, $j5h_test PASS" SURVIVED
    else say "$j5h_id" "$j5h_ex" "rc=$j5h_rc, the test did not run" CNM; head_of "$j5h_log" 8; fi
  fi
  rm -rf "$j5h_pd"
}

go_row J5H-01 j5h "TestJ5HHydrate rc=0"   /w/go test -run '^TestJ5HHydrate$' ./engine/jacquard/
go_row J5H-02 j5h "TestJ5HRefused rc=0"   /w/go test -run '^TestJ5HRefused$' ./engine/jacquard/
go_row J5H-03 j5h "TestJ5HRehydrate rc=0" /w/go test -run '^TestJ5HRehydrate$' ./engine/jacquard/
j5h_plant J5H-01p  "TestJ5HHydrate fails on the plant"   TestJ5HHydrate   hydrate.go 's/en\.res\[ki\]\.evictions++/_ = ki/'
j5h_plant J5H-02p  "TestJ5HRefused fails on the plant"   TestJ5HRefused   hydrate.go 's/if len(r\.Value) != cd\.size {/if false {/'
j5h_plant J5H-02pk "TestJ5HRefused fails on the plant"   TestJ5HRefused   hydrate.go 's/if ecd\.KindOf(r\.Entity) != k {/if false {/'
j5h_plant J5H-02pa "TestJ5HRefused fails on the plant"   TestJ5HRefused   hydrate.go 's/if at != en\.applied && (en\.applied != 0 || en\.loaded()) {/if false {/'
j5h_plant J5H-03p  "TestJ5HRehydrate fails on the plant" TestJ5HRehydrate hydrate.go 's/if _, ok := en\.idx\.entity(s\.id); ok {/if false {/'
j5h_plant J5H-04p  "TestJ5HHydrate fails on the plant"   TestJ5HHydrate   hydrate.go 's/vals\[r\.CID\] = append(\[\]byte(nil), r\.Value\.\.\.)/vals[r.CID] = r.Value/'
