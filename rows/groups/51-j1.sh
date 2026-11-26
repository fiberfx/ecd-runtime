# Group j1 of Entity Component Runtime: the identity core of package jacquard (J1-01 to J1-10 and their plants, the rows
# ending in p).
# Sourced by the rows harness from /w; the harness supplies say, sel, offline, head_of, count, why_of, go_row and
# HAVE_GO.
# Entity (8 B handle), record (16 B), the record pool with its free list, and the index that maps eid <-> Entity.
#
#   J1-01   the declared kinds spawn; an eid of an undeclared kind answers ecd.ErrKind and changes nothing; newIndex
#           refuses a duplicate kind and more than 65,535 kinds                                   (TestJ1_01)
#   J1-01p  plant: the kind check of spawn is dropped (index.go)
#   J1-02   10,000 random eids over 3 kinds: the record kind index equals kindIndex(KindOf(id))   (TestJ1_02)
#   J1-02p  plant: the record takes kind index 0 (index.go)
#   J1-03   free advances the generation; the old Entity fails valid, eidOf and rec; the predicted handle {i, g+1} is
#           refused; 1,000,000 free-and-spawn cycles reuse one index with a strictly greater generation (TestJ1_03)
#   J1-03p  plant: nextGen keeps the generation (pool.go)
#   J1-04   Sizeof(Entity{}) == 8 and Sizeof(record{}) == 16; Entity, record and eid.ID hold no pointer (TestJ1_04)
#   J1-04p  plant: record gains a fifth uint32 field (record.go)
#   J1-05   nextGen retires at MaxUint32; a retired index never returns to the free list or to a spawn (TestJ1_05)
#   J1-05p  plant: nextGen never retires (pool.go)
#   J1-06   eid -> Entity answers the Entity spawn gave, generation included, for 10,000 eids; a freed eid answers
#           false                                                                                (TestJ1_06)
#   J1-06p  plant: entity drops the generation (index.go)
#   J1-07   Entity -> eid answers the spawned eid; a stale Entity after free and reuse answers false (TestJ1_07)
#   J1-07p  plant: eidOf skips valid, so the generation is not checked (index.go)
#   J1-08   the null handle and handles outside the records are refused without a panic; the predicted handle of a
#           freed, unreused index is refused                                                     (TestJ1_08)
#   J1-08p  plant: valid skips the alive flag (index.go)
#   J1-09   neither Entity nor *Entity implements a JSON, text or binary encoding interface      (TestJ1_09)
#   J1-09p  plant: Entity gains a MarshalJSON method (entity.go)
#   J1-10   newIndex holds only the reserved record 0; the first spawn gets Entity{index: 1, gen: 0}; record 0 is never
#           alive                                                                                (TestJ1_10)
#   J1-10p  plant: the pool starts with no reserved record (pool.go)
#
# j1_plant id expect test file sedexpr ndel nadd: copies /w/go to a temporary directory, applies sedexpr to one file of
# engine/jacquard and runs only the named test there. The sed must remove ndel lines and add nadd lines of the file,
# else CNM. KILLED: the test failed. SURVIVED: it passed. CNM: the sed changed other than expected, the module was
# missing, the plant did not compile, or the test did not run.
j1_plant() {
  j1_id=$1 j1_ex=$2 j1_test=$3 j1_file=$4 j1_sed=$5 j1_nd=$6 j1_na=$7
  sel "$j1_id" j1 || return 0
  if [ $HAVE_GO -eq 0 ]; then say "$j1_id" "$j1_ex" "go not on PATH" CNM; return 0; fi
  j1_pd=$(mktemp -d /tmp/rows/plant.XXXXXX)
  cp -a /w/go "$j1_pd/go" && cp "$j1_pd/go/engine/jacquard/$j1_file" "$j1_pd/orig"
  (cd "$j1_pd/go/engine/jacquard" && sed -i "$j1_sed" "$j1_file")
  j1_del=$(diff "$j1_pd/orig" "$j1_pd/go/engine/jacquard/$j1_file" | grep -c '^<')
  j1_add=$(diff "$j1_pd/orig" "$j1_pd/go/engine/jacquard/$j1_file" | grep -c '^>')
  if [ "$j1_del" != "$j1_nd" ] || [ "$j1_add" != "$j1_na" ]; then
    say "$j1_id" "$j1_ex" "the sed removed $j1_del and added $j1_add lines of $j1_file ($j1_nd and $j1_na expected)" CNM
  else
    j1_log=/tmp/rows/$j1_id.log
    (cd "$j1_pd/go" && go test -count=1 -v -run "^$j1_test\$" ./engine/jacquard/ >"$j1_log" 2>&1); j1_rc=$?
    if offline "$j1_log"; then say "$j1_id" "$j1_ex" "module not found" CNM
    elif grep -q "^--- FAIL: $j1_test" "$j1_log"; then
      say "$j1_id" "$j1_ex" "rc=$j1_rc, $j1_test FAIL: $(why_of "$j1_log")" KILLED
    elif [ $j1_rc -eq 0 ] && grep -q "^--- PASS: $j1_test" "$j1_log"; then say "$j1_id" "$j1_ex" "rc=0, $j1_test PASS" SURVIVED
    else say "$j1_id" "$j1_ex" "rc=$j1_rc, the test did not run" CNM; head_of "$j1_log" 8; fi
  fi
  rm -rf "$j1_pd"
}

go_row J1-01 j1 "TestJ1_01 rc=0" /w/go test -run '^TestJ1_01$' ./engine/jacquard/
j1_plant J1-01p "TestJ1_01 fails when spawn drops the kind check" TestJ1_01 index.go 's/^\tif err != nil {$/\tif false {/' 1 1

go_row J1-02 j1 "TestJ1_02 rc=0" /w/go test -run '^TestJ1_02$' ./engine/jacquard/
j1_plant J1-02p "TestJ1_02 fails when the record kind comes from index 0" TestJ1_02 index.go 's/flagAlive | uint32(ki)$/flagAlive | uint32(ki\&0)/' 1 1

go_row J1-03 j1 "TestJ1_03 rc=0" /w/go test -run '^TestJ1_03$' ./engine/jacquard/
j1_plant J1-03p "TestJ1_03 fails when free keeps the generation" TestJ1_03 pool.go 's/^\tnext = g + 1$/\tnext = g/' 1 1

go_row J1-04 j1 "TestJ1_04 rc=0" /w/go test -run '^TestJ1_04$' ./engine/jacquard/
j1_plant J1-04p "TestJ1_04 fails on a 20-byte record" TestJ1_04 record.go 's/table, row, gen, flags uint32/table, row, gen, flags, pad uint32/' 1 1

go_row J1-05 j1 "TestJ1_05 rc=0" /w/go test -run '^TestJ1_05$' ./engine/jacquard/
j1_plant J1-05p "TestJ1_05 fails when nextGen never retires" TestJ1_05 pool.go 's/return next, next == math.MaxUint32$/return next, false/' 1 1

go_row J1-06 j1 "TestJ1_06 rc=0" /w/go test -run '^TestJ1_06$' ./engine/jacquard/
j1_plant J1-06p "TestJ1_06 fails when entity drops the generation" TestJ1_06 index.go 's/^\treturn e, ok$/\treturn Entity{index: e.index}, ok/' 1 1

go_row J1-07 j1 "TestJ1_07 rc=0" /w/go test -run '^TestJ1_07$' ./engine/jacquard/
j1_plant J1-07p "TestJ1_07 fails when eidOf skips the generation check" TestJ1_07 index.go '/^func (x \*index) eidOf/,/^}/s/if !x.valid(e) {/if e.index == 0 || int(e.index) >= len(x.eids) {/' 1 1

go_row J1-08 j1 "TestJ1_08 rc=0" /w/go test -run '^TestJ1_08$' ./engine/jacquard/
j1_plant J1-08p "TestJ1_08 fails when valid skips the alive flag" TestJ1_08 index.go 's/return r.gen == e.gen && r.flags&flagAlive != 0$/return r.gen == e.gen/' 1 1

go_row J1-09 j1 "TestJ1_09 rc=0" /w/go test -run '^TestJ1_09$' ./engine/jacquard/
j1_plant J1-09p "TestJ1_09 fails when Entity has a MarshalJSON method" TestJ1_09 entity.go '$a func (e Entity) MarshalJSON() ([]byte, error) { return nil, nil }' 0 1

go_row J1-10 j1 "TestJ1_10 rc=0" /w/go test -run '^TestJ1_10$' ./engine/jacquard/
j1_plant J1-10p "TestJ1_10 fails when the pool has no reserved record 0" TestJ1_10 pool.go 's/make(\[\]record, 1, 64)/make([]record, 0, 64)/' 1 1
