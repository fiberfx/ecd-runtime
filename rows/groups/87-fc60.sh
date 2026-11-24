# Group fc60 of Ecd Runtime: the nested module go/ecr, the facade package ecr, which gives a host one import and one
# requirement: aliases of the ids, the seam types, the Runtime and the owner store, the constants and sentinels of
# their packages, and Open and Close of a Host. The rows show that the tests of the module pass, that go mod tidy
# leaves its go.mod and go.sum unchanged, and that the facade is a leaf: no package outside go/ecr imports it, and it
# imports only the standard library, package eid and the packages ecd, rt, engine/jacquard and sqlite. The number 87
# is above every group file present once group sp60 moves from 82-sp60.sh to 86-sp60.sh.
# Sourced by the rows harness from /w; the harness supplies say, sel, offline, head_of, count, why_of, go_row and
# HAVE_GO.
#
#   FC-01    go test in go/ecr: the types, sentinels, constants and functions of ecr are those of their packages
#            (TestAliases); Open creates, opens and refuses as its steps state (TestOpen); Close closes the engine and
#            the store (TestClose)
#   FC-01p   plant: Open hydrates the engine with the whole catalog, so the All kind is Warm after a reopen
#            (TestOpen, case create)
#   FC-01q   plant: Close leaves the store open (TestClose, case both)
#   FC-01r   plant: the alias QID names ecd.CID (TestAliases, case types)
#   FC-01s   plant: ErrRef is a new error with the text of rt.ErrRef (TestAliases, case sentinels)
#   FC-01t   plant: the constant Unknown holds rt.Refused (TestAliases, case constants)
#   FC-01u   plant: Open builds Jacquard whatever the Engine func of the Options (TestOpen, case engine)
#   FC-01v   plant: a failed Open leaves the engine it built open (TestOpen, case missing)
#   FC-01w   plant: Open checks the DeedKind only in rt.New, after the store file is made (TestOpen, case nilcatalog)
#   FC-02    go mod tidy in a copy of go/ leaves go/ecr/go.mod and go.sum byte-equal
#   FC-03    no Go file under go/ outside go/ecr imports github.com/fiberfx/ecd-runtime/go/ecr, and the imports of the
#            package ecr and of its tests, as go list prints them, are the standard library, ecd-id, ecr itself (the
#            external test) and the packages ecd, rt, engine/jacquard and sqlite of Ecd Runtime
#   FC-03p   plant: go/rt/doc.go imports ecr
#   FC-03q   plant: go/ecr/ecr.go imports modernc.org/sqlite
#
# The rows FC-01 to FC-03 load the driver and its modules from the module inputs, as the rows of group sq40 do: the
# build graph of go/ecr is that of go/sqlite, so go/ecr/go.sum holds the lines of go/sqlite/go.sum. The replace
# directives of go/ecr/go.mod make every build of go/ecr here compile against the main module and go/sqlite of the
# same checkout; the require lines are what a consumer outside the repository resolves.
#
# fc60_plant id expect test case sedexpr: copies /w/go to a temporary directory, applies the one-line sed to
# go/ecr/ecr.go there and runs only the named test in go/ecr of the copy. The copy is the whole of /w/go, since go/ecr
# replaces the main module and go/sqlite by directories of the copy. KILLED: the test failed in its case, and the
# observed text names its first failing line. SURVIVED: it passed, or failed in another case only. CNM: the sed did
# not change exactly one line, the file or a module was missing, the test panicked, or the test did not run, which
# includes a mutant that does not compile.
fc60_plant() {
  fc60_id=$1 fc60_ex=$2 fc60_test=$3 fc60_case=$4 fc60_sed=$5
  sel "$fc60_id" fc60 || return 0
  if [ $HAVE_GO -eq 0 ]; then say "$fc60_id" "$fc60_ex" "go not on PATH" CNM; return 0; fi
  fc60_name=$fc60_test/$fc60_case
  fc60_pd=$(mktemp -d /tmp/rows/plant.XXXXXX)
  cp -a /w/go "$fc60_pd/go"
  fc60_f=$fc60_pd/go/ecr/ecr.go
  if [ ! -f "$fc60_f" ]; then
    say "$fc60_id" "$fc60_ex" "go/ecr/ecr.go is absent" CNM
    rm -rf "$fc60_pd"
    return 0
  fi
  cp "$fc60_f" "$fc60_pd/orig.go"
  sed -i "$fc60_sed" "$fc60_f"
  fc60_n=$(diff "$fc60_pd/orig.go" "$fc60_f" | grep -c '^>')
  if [ "$fc60_n" -ne 1 ]; then
    say "$fc60_id" "$fc60_ex" "the sed changed $fc60_n lines of go/ecr/ecr.go, want 1" CNM
  else
    fc60_log=/tmp/rows/$fc60_id.log
    (cd "$fc60_pd/go/ecr" && go test -count=1 -v -run "^$fc60_test\$" . >"$fc60_log" 2>&1)
    fc60_rc=$?
    if offline "$fc60_log"; then say "$fc60_id" "$fc60_ex" "module not found" CNM
    elif grep -q '^panic:' "$fc60_log"; then
      say "$fc60_id" "$fc60_ex" "rc=$fc60_rc, $fc60_test panicked: $(why_of "$fc60_log")" CNM
    elif grep -q -E "^ *--- FAIL: $fc60_name " "$fc60_log"; then
      say "$fc60_id" "$fc60_ex" "rc=$fc60_rc, $fc60_name FAIL: $(why_of "$fc60_log")" KILLED
    elif grep -q "^--- FAIL: $fc60_test " "$fc60_log"; then
      say "$fc60_id" "$fc60_ex" "rc=$fc60_rc, $fc60_test FAIL, but not in $fc60_name" SURVIVED
    elif [ $fc60_rc -eq 0 ] && grep -q "^--- PASS: $fc60_test " "$fc60_log"; then
      say "$fc60_id" "$fc60_ex" "rc=0, $fc60_test PASS" SURVIVED
    else say "$fc60_id" "$fc60_ex" "rc=$fc60_rc, the test did not run" CNM; head_of "$fc60_log" 8; fi
  fi
  rm -rf "$fc60_pd"
}

# fc60_copy: makes the temporary directory fc60_pd holding a copy of /w/go; 1 when it cannot.
fc60_copy() {
  fc60_pd=$(mktemp -d /tmp/rows/fc.XXXXXX) || return 1
  cp -a /w/go "$fc60_pd/go" && return 0
  rm -rf "$fc60_pd"
  return 1
}

# fc60_allowed: the imports of the package ecr and its tests that are not of the standard library.
fc60_allowed='github.com/fiberfx/ecd-id/go
github.com/fiberfx/ecd-runtime/go/ecd
github.com/fiberfx/ecd-runtime/go/ecr
github.com/fiberfx/ecd-runtime/go/engine/jacquard
github.com/fiberfx/ecd-runtime/go/rt
github.com/fiberfx/ecd-runtime/go/sqlite'

# fc60_leaf dir log: measures FC-03 on dir, /w/go or a copy of it. fc60_v is PASS, FAIL or CNM and fc60_obs the
# observed text. An import path whose first element holds no dot is of the standard library. The control of the grep
# is the external test of go/ecr, which imports ecr; the control of go list is the import of go/sqlite.
fc60_leaf() {
  fc60_d=$1 fc60_l=$2
  fc60_pat='"github.com/fiberfx/ecd-runtime/go/ecr"'
  if [ ! -d "$fc60_d/ecr" ]; then fc60_v=CNM fc60_obs="go/ecr is absent"; return 0; fi
  fc60_ctl=$(grep -rlF "$fc60_pat" --include='*.go' "$fc60_d/ecr" | wc -l)
  fc60_out=$(grep -rlF "$fc60_pat" --include='*.go' "$fc60_d" | grep -vc "^$fc60_d/ecr/")
  if [ "$fc60_ctl" -eq 0 ]; then fc60_v=CNM fc60_obs="the pattern finds no file of go/ecr either"; return 0; fi
  if [ "$fc60_out" -ne 0 ]; then fc60_v=FAIL fc60_obs="$fc60_out Go files outside go/ecr import ecr"; return 0; fi
  (cd "$fc60_d/ecr" && go list -f \
    '{{join .Imports "\n"}}{{"\n"}}{{join .TestImports "\n"}}{{"\n"}}{{join .XTestImports "\n"}}' .) >"$fc60_l" 2>&1
  fc60_rc=$?
  if offline "$fc60_l"; then fc60_v=CNM fc60_obs="module not found"; return 0; fi
  if [ $fc60_rc -ne 0 ]; then fc60_v=FAIL fc60_obs="go list rc=$fc60_rc"; return 0; fi
  if ! grep -qx 'github.com/fiberfx/ecd-runtime/go/sqlite' "$fc60_l"; then
    fc60_v=CNM fc60_obs="go list names no import of go/sqlite"
    return 0
  fi
  fc60_bad=$(grep -v '^$' "$fc60_l" | sort -u | grep '^[^/]*\.' | grep -vxF "$fc60_allowed" | tr '\n' ' ')
  fc60_std=$(grep -v '^$' "$fc60_l" | sort -u | grep -vc '^[^/]*\.')
  if [ -n "$fc60_bad" ]; then fc60_v=FAIL fc60_obs="0 files outside go/ecr; go/ecr imports $fc60_bad"
  else fc60_v=PASS fc60_obs="0 files outside go/ecr; go/ecr imports $fc60_std standard packages and allowed ones"; fi
}

# fc60_leafplant id expect file sedexpr: applies the one-line sed to go/<file> in a copy of /w/go and measures FC-03
# there. KILLED: FC-03 is FAIL. SURVIVED: it is PASS. CNM: the sed did not change exactly one line, or FC-03 is CNM.
fc60_leafplant() {
  fc60_id=$1 fc60_ex=$2 fc60_file=$3 fc60_sed=$4
  sel "$fc60_id" fc60 || return 0
  if [ $HAVE_GO -eq 0 ]; then say "$fc60_id" "$fc60_ex" "go not on PATH" CNM; return 0; fi
  if ! fc60_copy; then say "$fc60_id" "$fc60_ex" "the copy failed" CNM; return 0; fi
  fc60_f=$fc60_pd/go/$fc60_file
  if [ ! -f "$fc60_f" ]; then
    say "$fc60_id" "$fc60_ex" "go/$fc60_file is absent" CNM
    rm -rf "$fc60_pd"
    return 0
  fi
  cp "$fc60_f" "$fc60_pd/orig.go"
  sed -i "$fc60_sed" "$fc60_f"
  fc60_n=$(diff "$fc60_pd/orig.go" "$fc60_f" | grep -c '^>')
  if [ "$fc60_n" -ne 1 ]; then
    say "$fc60_id" "$fc60_ex" "the sed changed $fc60_n lines of go/$fc60_file, want 1" CNM
  else
    fc60_leaf "$fc60_pd/go" "/tmp/rows/$fc60_id.log"
    case $fc60_v in
      FAIL) say "$fc60_id" "$fc60_ex" "$fc60_obs" KILLED ;;
      PASS) say "$fc60_id" "$fc60_ex" "$fc60_obs" SURVIVED ;;
      *) say "$fc60_id" "$fc60_ex" "$fc60_obs" CNM ;;
    esac
  fi
  rm -rf "$fc60_pd"
}

go_row FC-01 fc60 "go test rc=0 in go/ecr" /w/go/ecr test .
fc60_plant FC-01p "TestOpen/create fails on the plant" TestOpen create \
  's/s\.Hydrate(e, withoutAll(o\.Catalog))/s.Hydrate(e, o.Catalog)/'
fc60_plant FC-01q "TestClose/both fails on the plant" TestClose both 's/^\(\t*\)se = h\.Store\.Close()$/\1se = nil/'
fc60_plant FC-01r "TestAliases/types fails on the plant" TestAliases types 's/^\(\tQID *\)= ecd\.QID$/\1= ecd.CID/'
fc60_plant FC-01s "TestAliases/sentinels fails on the plant" TestAliases sentinels \
  's/^\(\tErrRef *\)= rt\.ErrRef$/\1= errors.New("rt: ref refused")/'
fc60_plant FC-01t "TestAliases/constants fails on the plant" TestAliases constants \
  's/^\(\tUnknown *\)= rt\.Unknown$/\1= rt.Refused/'
fc60_plant FC-01u "TestOpen/engine fails on the plant" TestOpen engine 's/^\(\t*\)if build == nil {$/\1if true {/'
fc60_plant FC-01v "TestOpen/missing fails on the plant" TestOpen missing \
  's/^\(\t*\)h := &Host{Store: s, Engine: e}$/\1h := \&Host{Store: s}/'
fc60_plant FC-01w "TestOpen/nilcatalog fails on the plant" TestOpen nilcatalog \
  's/^\(\t*\)if !o\.DeedKind\.IsKind() {$/\1if false {/'

# FC-02
fc60_ex="go mod tidy leaves go.mod and go.sum of go/ecr byte-equal"
if sel FC-02 fc60; then
  if [ $HAVE_GO -eq 0 ]; then say FC-02 "$fc60_ex" "go not on PATH" CNM
  elif ! fc60_copy; then say FC-02 "$fc60_ex" "the copy failed" CNM
  elif [ ! -f "$fc60_pd/go/ecr/go.mod" ] || [ ! -f "$fc60_pd/go/ecr/go.sum" ]; then
    say FC-02 "$fc60_ex" "go/ecr holds no go.mod or go.sum" CNM; rm -rf "$fc60_pd"
  elif [ -z "$(find "$fc60_pd/go/ecr" -name '*.go' -print -quit)" ]; then
    say FC-02 "$fc60_ex" "go/ecr holds no Go file, so tidy would drop every requirement" CNM; rm -rf "$fc60_pd"
  else
    fc60_e=$fc60_pd/go/ecr
    cp "$fc60_e/go.mod" "$fc60_pd/before.mod"
    cp "$fc60_e/go.sum" "$fc60_pd/before.sum"
    fc60_log=/tmp/rows/FC-02.log
    (cd "$fc60_e" && go mod tidy) >"$fc60_log" 2>&1
    fc60_rc=$?
    if offline "$fc60_log"; then say FC-02 "$fc60_ex" "module not found" CNM; head_of "$fc60_log" 8
    elif [ $fc60_rc -ne 0 ]; then say FC-02 "$fc60_ex" "go mod tidy rc=$fc60_rc" FAIL; head_of "$fc60_log" 8
    elif ! cmp -s "$fc60_pd/before.mod" "$fc60_e/go.mod" || ! cmp -s "$fc60_pd/before.sum" "$fc60_e/go.sum"; then
      say FC-02 "$fc60_ex" "go mod tidy rc=0, the files changed" FAIL
      diff "$fc60_pd/before.mod" "$fc60_e/go.mod" | head -20 | sed 's/^/    | mod /'
      diff "$fc60_pd/before.sum" "$fc60_e/go.sum" | head -20 | sed 's/^/    | sum /'
    else say FC-02 "$fc60_ex" "go mod tidy rc=0, go.mod and go.sum unchanged" PASS; fi
    rm -rf "$fc60_pd"
  fi
fi

# FC-03
fc60_ex="no package outside go/ecr imports ecr, and ecr imports only the allowed packages"
if sel FC-03 fc60; then
  if [ $HAVE_GO -eq 0 ]; then say FC-03 "$fc60_ex" "go not on PATH" CNM
  else
    fc60_leaf /w/go /tmp/rows/FC-03.log
    say FC-03 "$fc60_ex" "$fc60_obs" "$fc60_v"
    if [ "$fc60_v" != PASS ]; then head_of /tmp/rows/FC-03.log 8; fi
  fi
fi
fc60_leafplant FC-03p "FC-03 fails on the plant" rt/doc.go \
  's|^package rt$|package rt; import _ "github.com/fiberfx/ecd-runtime/go/ecr"|'
fc60_leafplant FC-03q "FC-03 fails on the plant" ecr/ecr.go 's|^\t"fmt"$|\t"fmt"; _ "modernc.org/sqlite"|'
