# Group kb60 of Ecd Runtime: story 25, kinds as typed bundles. A kind of the catalog lists its components, each
# required or optional; the generator writes <K>Entity, Spawn<K> and Get<K> for it. The number 91 is above every group
# file present.
# Sourced by the rows harness from /w; the harness supplies say, sel, offline, head_of, count, why_of, go_row and
# HAVE_GO.
#
#   KB-01    TestKind/*/spawn (internal/llm): SpawnCnv records one spawn then the sets of turn, usage and the optional
#            parent, in that order; Result.Spawned names the entity and GetCnv reads it back whole, on model and jacquard
#   KB-01p   plant: the generated SpawnCnv skips its last set (internal/llm/llm_gen.go; TestKind, case model/spawn)
#   KB-02    TestKindComponents (internal/gen): a listed component that is undeclared, a tag, a repeat, a string in place
#            of a member, or whose Has field another component makes is refused at its path; a clash of the kind's names
#   KB-02p   plant: the undeclared refusal is dropped (internal/gen/check.go; TestKindComponents, case undeclared)
#   KB-03    TestKind/*/kind (internal/llm): GetCnv of an entity of the kind ses answers an error that matches ecd.ErrKind
#   KB-03p   plant: the generated GetCnv drops its kind test (internal/llm/llm_gen.go; TestKind, case model/kind)
#   KB-04    TestKind/*/optional (internal/llm): SpawnCnv with HasParent false sets turn and usage only, and GetCnv reads
#            the entity whole with HasParent false
#   KB-04p   plant: the generated SpawnCnv sets parent whatever HasParent says (internal/llm/llm_gen.go; TestKind, case
#            model/optional)
#   KB-05    TestKind/*/required (internal/llm): an entity of cnv without its required usage answers false and nil
#   KB-05p   plant: the generated GetCnv reads past an absent usage (internal/llm/llm_gen.go; TestKind, case
#            model/required)
#
kb60_plant() {
  kb60_id=$1 kb60_ex=$2 kb60_mod=$3 kb60_pkg=$4 kb60_test=$5 kb60_case=$6 kb60_file=$7 kb60_sed=$8
  sel "$kb60_id" kb60 || return 0
  if [ $HAVE_GO -eq 0 ]; then say "$kb60_id" "$kb60_ex" "go not on PATH" CNM; return 0; fi
  kb60_name=$kb60_test
  if [ -n "$kb60_case" ]; then kb60_name=$kb60_test/$kb60_case; fi
  kb60_pd=$(mktemp -d /tmp/rows/plant.XXXXXX)
  cp -a /w/go "$kb60_pd/go"
  kb60_f=$kb60_pd/go/$kb60_file
  if [ ! -f "$kb60_f" ]; then
    say "$kb60_id" "$kb60_ex" "go/$kb60_file is absent" CNM
    rm -rf "$kb60_pd"
    return 0
  fi
  cp "$kb60_f" "$kb60_pd/orig.go"
  sed -i "$kb60_sed" "$kb60_f"
  kb60_n=$(diff "$kb60_pd/orig.go" "$kb60_f" | grep -c '^>')
  if [ "$kb60_n" -ne 1 ]; then
    say "$kb60_id" "$kb60_ex" "the sed changed $kb60_n lines of go/$kb60_file, want 1" CNM
  else
    kb60_log=/tmp/rows/$kb60_id.log
    (cd "$kb60_pd/go/$kb60_mod" && go test -count=1 -v -run "^$kb60_test\$" "$kb60_pkg" >"$kb60_log" 2>&1)
    kb60_rc=$?
    if offline "$kb60_log"; then say "$kb60_id" "$kb60_ex" "module not found" CNM
    elif grep -q '^panic:' "$kb60_log"; then
      say "$kb60_id" "$kb60_ex" "rc=$kb60_rc, $kb60_test panicked: $(why_of "$kb60_log")" CNM
    elif grep -q -E "^ *--- FAIL: $kb60_name " "$kb60_log"; then
      say "$kb60_id" "$kb60_ex" "rc=$kb60_rc, $kb60_name FAIL: $(why_of "$kb60_log")" KILLED
    elif grep -q "^--- FAIL: $kb60_test " "$kb60_log"; then
      say "$kb60_id" "$kb60_ex" "rc=$kb60_rc, $kb60_test FAIL, but not in $kb60_name" SURVIVED
    elif [ $kb60_rc -eq 0 ] && grep -q "^--- PASS: $kb60_test " "$kb60_log"; then
      say "$kb60_id" "$kb60_ex" "rc=0, $kb60_test PASS" SURVIVED
    else say "$kb60_id" "$kb60_ex" "rc=$kb60_rc, the test did not run" CNM; head_of "$kb60_log" 8; fi
  fi
  rm -rf "$kb60_pd"
}


go_row KB-01 kb60 "TestKind spawn rc=0" /w/go test -run '^TestKind$/^(model|jacquard)$/^spawn$' ./internal/llm/
kb60_plant KB-01p "TestKind/model/spawn fails on the plant" . ./internal/llm/ TestKind model/spawn internal/llm/llm_gen.go \
  's/^\t\tParent\.Set(b, r, v\.Parent)$/\t\t_ = v.Parent/'
go_row KB-02 kb60 "TestKindComponents rc=0" /w/go test -run '^TestKindComponents$' ./internal/gen/
kb60_plant KB-02p "TestKindComponents/undeclared fails on the plant" . ./internal/gen/ TestKindComponents undeclared internal/gen/check.go \
  's/^\t\t\tcase !declared:$/\t\t\tcase false \&\& !declared:/'
go_row KB-03 kb60 "TestKind kind rc=0" /w/go test -run '^TestKind$/^(model|jacquard)$/^kind$' ./internal/llm/
kb60_plant KB-03p "TestKind/model/kind fails on the plant" . ./internal/llm/ TestKind model/kind internal/llm/llm_gen.go \
  's/if id\.System() != System || id\.Kind() != CodeCnv {/if false {/'
go_row KB-04 kb60 "TestKind optional rc=0" /w/go test -run '^TestKind$/^(model|jacquard)$/^optional$' ./internal/llm/
kb60_plant KB-04p "TestKind/model/optional fails on the plant" . ./internal/llm/ TestKind model/optional internal/llm/llm_gen.go \
  's/^\tif v\.HasParent {$/\tif true {/'
go_row KB-05 kb60 "TestKind required rc=0" /w/go test -run '^TestKind$/^(model|jacquard)$/^required$' ./internal/llm/
kb60_plant KB-05p "TestKind/model/required fails on the plant" . ./internal/llm/ TestKind model/required internal/llm/llm_gen.go \
  's/if v\.Usage, ok, err = Usage\.Get(e, id); !ok || err != nil {/if v.Usage, ok, err = Usage.Get(e, id); err != nil {/'
