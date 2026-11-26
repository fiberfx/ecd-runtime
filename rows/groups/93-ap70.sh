# Group ap70 of Ecd Runtime: story 27, the reference application chatlog in the module apps, outside go/. The number
# 93 is above every group file present.
# Sourced by the rows harness from /w; the harness supplies say, sel, offline, head_of, count, why_of, go_row and
# HAVE_GO. The module apps reaches the runtime through a replace of ../go, ../go/ecr and ../go/sqlite.
#
#   AP-01    TestChatlog (apps/chatlog): open, say twice, show, pin, show, journal and replay, each in a run of its own,
#            so that each reopens the store through Migrate and reads the room through to the store
#   AP-01p   plant: say leaves the room's total unset (apps/chatlog/main.go; TestChatlog)
#   AP-01q   plant: the read through on ErrMiss loads nothing (apps/chatlog/main.go; TestChatlog)
#   AP-02    TestRefusals (apps/chatlog): usage errors exit 2; an eid of the wrong kind, an unknown room and a bad eid
#            exit 1 with their cause; replay answers a stored total that the deeds do not give
#   AP-02p   plant: replay compares no total (apps/chatlog/main.go; TestRefusals, case replay)
#   AP-02q   plant: -sys takes a registered code (apps/chatlog/main.go; TestRefusals, case system)
#   AP-03    TestIter (apps/chatlog): Iter of the host's engine answers ErrCold; the deeds replayed into ecrtest's model
#            give, through Iter, the lines that journal scans from the store
#   AP-03p   plant: journal scans the kind rom (apps/chatlog/main.go; TestIter)
#   AP-04    the package chatlog imports the standard library, ecd-id, ecr and chat only, chat imports the standard
#            library and ecr only, and go generate in a copy leaves chat_gen.go byte-equal
#   AP-04p   plant: a hand edit of chat_gen.go (one doc line); the copy's go generate restores it
#   AP-04q   plant: main.go imports go/sqlite beside ecr
#   AP-05    the binary, built and vetted, runs open, say, say, show, pin, journal and replay on a new store, each
#            exit 0, replay last with "total 2 replayed 2"; pin of a room exits 1
#   AP-06    TestMigrate (apps/chatlog): a store of user_version 1 (deed without trace_id and span_id) is migrated and
#            then read with its data intact; user_version reads 2 afterwards
#   AP-06p   plant: the Migrate call is dropped (apps/chatlog/main.go; TestMigrate)
#   AP-07    TestOtherSystem (apps/chatlog): -sys 7ab works end to end and every eid carries it; llm, 2ab and the
#            malformed codes are refused with exit 2
#   AP-07p   plant: run ignores -sys and uses 3cl (apps/chatlog/main.go; TestOtherSystem)
#   AP-08    TestMessageServed (apps/chatlog): on an engine that serves msg, a message the engine does not hold is
#            absent (ecr.ErrAbsent), not found
#   AP-08p   plant: message returns no error for it (apps/chatlog/main.go; TestMessageServed)
#
ap70_plant() {
  ap70_id=$1 ap70_ex=$2 ap70_test=$3 ap70_case=$4 ap70_file=$5 ap70_sed=$6
  sel "$ap70_id" ap70 || return 0
  if [ $HAVE_GO -eq 0 ]; then say "$ap70_id" "$ap70_ex" "go not on PATH" CNM; return 0; fi
  ap70_name=$ap70_test
  if [ -n "$ap70_case" ]; then ap70_name=$ap70_test/$ap70_case; fi
  ap70_pd=$(mktemp -d /tmp/rows/plant.XXXXXX)
  cp -a /w/go "$ap70_pd/go"
  cp -a /w/apps "$ap70_pd/apps"
  ap70_f=$ap70_pd/apps/$ap70_file
  if [ ! -f "$ap70_f" ]; then
    say "$ap70_id" "$ap70_ex" "apps/$ap70_file is absent" CNM
    rm -rf "$ap70_pd"
    return 0
  fi
  cp "$ap70_f" "$ap70_pd/orig.go"
  sed -i "$ap70_sed" "$ap70_f"
  ap70_n=$(diff "$ap70_pd/orig.go" "$ap70_f" | grep -c '^>')
  if [ "$ap70_n" -ne 1 ]; then
    say "$ap70_id" "$ap70_ex" "the sed changed $ap70_n lines of apps/$ap70_file, want 1" CNM
  else
    ap70_log=/tmp/rows/$ap70_id.log
    (cd "$ap70_pd/apps" && go test -count=1 -v -run "^$ap70_test\$" ./chatlog/ >"$ap70_log" 2>&1)
    ap70_rc=$?
    if offline "$ap70_log"; then say "$ap70_id" "$ap70_ex" "module not found" CNM
    elif grep -q '^panic:' "$ap70_log"; then
      say "$ap70_id" "$ap70_ex" "rc=$ap70_rc, $ap70_test panicked: $(why_of "$ap70_log" | tr -d '\\')" CNM
    elif grep -q -E "^ *--- FAIL: $ap70_name " "$ap70_log"; then
      say "$ap70_id" "$ap70_ex" "rc=$ap70_rc, $ap70_name FAIL: $(why_of "$ap70_log" | tr -d '\\')" KILLED
    elif grep -q "^--- FAIL: $ap70_test " "$ap70_log"; then
      say "$ap70_id" "$ap70_ex" "rc=$ap70_rc, $ap70_test FAIL, but not in $ap70_name" SURVIVED
    elif [ $ap70_rc -eq 0 ] && grep -q "^--- PASS: $ap70_test " "$ap70_log"; then
      say "$ap70_id" "$ap70_ex" "rc=0, $ap70_test PASS" SURVIVED
    else say "$ap70_id" "$ap70_ex" "rc=$ap70_rc, the test did not run" CNM; head_of "$ap70_log" 8; fi
  fi
  rm -rf "$ap70_pd"
}

# ap70_bound dir: the boundary of AP-04 in the module copy dir. It prints the first breach and returns 1, or
# returns 0. A go list or go generate that fails returns 2.
ap70_bound() {
  ap70_bd=$1
  ap70_imp=$( (cd "$ap70_bd/apps" && go list -f '{{.ImportPath}} {{join .Imports " "}}' ./chatlog/ ./chatlog/chat/) 2>/tmp/rows/AP-04.list) || return 2
  ap70_bad=$(printf '%s\n' "$ap70_imp" | awk '
    $1 ~ /\/chatlog$/      { for (i = 2; i <= NF; i++) if ($i ~ /\./ && $i != "github.com/fiberfx/ecd-id/go" && $i != "github.com/fiberfx/ecd-runtime/go/ecr" && $i != "github.com/fiberfx/ecd-runtime/apps/chatlog/chat") print $1 " imports " $i }
    $1 ~ /\/chatlog\/chat$/ { for (i = 2; i <= NF; i++) if ($i ~ /\./ && $i != "github.com/fiberfx/ecd-runtime/go/ecr") print $1 " imports " $i }')
  if [ -n "$ap70_bad" ]; then echo "$ap70_bad" | head -1; return 1; fi
  cp "$ap70_bd/apps/chatlog/chat/chat_gen.go" "$ap70_bd/committed.go"
  (cd "$ap70_bd/apps" && go generate ./chatlog/chat/) >/tmp/rows/AP-04.gen 2>&1 || return 2
  if ! cmp -s "$ap70_bd/committed.go" "$ap70_bd/apps/chatlog/chat/chat_gen.go"; then echo "chat_gen.go differs from go generate"; return 1; fi
  return 0
}

# ap70_boundrow id expect file sed: AP-04 with no file and no sed; a plant of AP-04 with them.
ap70_boundrow() {
  ap70_id=$1 ap70_ex=$2 ap70_file=$3 ap70_sed=$4
  sel "$ap70_id" ap70 || return 0
  if [ $HAVE_GO -eq 0 ]; then say "$ap70_id" "$ap70_ex" "go not on PATH" CNM; return 0; fi
  if [ ! -d /w/apps ]; then say "$ap70_id" "$ap70_ex" "/w/apps is absent" CNM; return 0; fi
  ap70_pd=$(mktemp -d /tmp/rows/bound.XXXXXX)
  cp -a /w/go "$ap70_pd/go"
  cp -a /w/apps "$ap70_pd/apps"
  ap70_n=1
  if [ -n "$ap70_file" ]; then
    cp "$ap70_pd/apps/$ap70_file" "$ap70_pd/orig"
    sed -i "$ap70_sed" "$ap70_pd/apps/$ap70_file"
    ap70_n=$(diff "$ap70_pd/orig" "$ap70_pd/apps/$ap70_file" | grep -c '^>')
  fi
  if [ "$ap70_n" -ne 1 ]; then
    say "$ap70_id" "$ap70_ex" "the sed changed $ap70_n lines of apps/$ap70_file, want 1" CNM
  else
    ap70_why=$(ap70_bound "$ap70_pd"); ap70_rc=$?
    if offline /tmp/rows/AP-04.list || { [ -f /tmp/rows/AP-04.gen ] && offline /tmp/rows/AP-04.gen; }; then
      say "$ap70_id" "$ap70_ex" "module not found" CNM
    elif [ $ap70_rc -eq 2 ]; then say "$ap70_id" "$ap70_ex" "go list or go generate failed" CNM; head_of /tmp/rows/AP-04.list 4
    elif [ -z "$ap70_file" ] && [ $ap70_rc -eq 0 ]; then say "$ap70_id" "$ap70_ex" "imports bounded; chat_gen.go regenerates byte-equal" PASS
    elif [ -z "$ap70_file" ]; then say "$ap70_id" "$ap70_ex" "$ap70_why" FAIL
    elif [ $ap70_rc -eq 1 ]; then say "$ap70_id" "$ap70_ex" "$ap70_why" KILLED
    else say "$ap70_id" "$ap70_ex" "the boundary holds on the plant" SURVIVED; fi
  fi
  rm -rf "$ap70_pd" /tmp/rows/AP-04.gen
}

# ap70_go id expect test: go_row of one test of chatlog; a run whose paths leave out apps measures nothing (CNM).
ap70_go() {
  sel "$1" ap70 || return 0
  if [ ! -d /w/apps ]; then say "$1" "$2" "/w/apps is absent: the run's paths leave out apps" CNM; return 0; fi
  go_row "$1" ap70 "$2" /w/apps test -run "^$3\$" ./chatlog/
}

# ap70_run: AP-05, the binary run end to end on a new store.
ap70_run() {
  sel AP-05 ap70 || return 0
  ap70_ex="seven commands exit 0, replay last with total 2 replayed 2; pin of a room exits 1"
  if [ $HAVE_GO -eq 0 ]; then say AP-05 "$ap70_ex" "go not on PATH" CNM; return 0; fi
  if [ ! -d /w/apps ]; then say AP-05 "$ap70_ex" "/w/apps is absent" CNM; return 0; fi
  ap70_d=$(mktemp -d /tmp/rows/chatlog.XXXXXX)
  ap70_log=/tmp/rows/AP-05.log
  (cd /w/apps && go vet ./... && go build -o "$ap70_d/chatlog" ./chatlog/) >"$ap70_log" 2>&1
  ap70_rc=$?
  if offline "$ap70_log"; then say AP-05 "$ap70_ex" "module not found" CNM; rm -rf "$ap70_d"; return 0; fi
  if [ $ap70_rc -ne 0 ]; then say AP-05 "$ap70_ex" "vet or build rc=$ap70_rc" FAIL; head_of "$ap70_log" 8; rm -rf "$ap70_d"; return 0; fi
  ap70_c="$ap70_d/chatlog -store $ap70_d/chat.db"
  ap70_fail=""
  ap70_room=$($ap70_c open lobby) || ap70_fail="open"
  ap70_rid=$(echo "$ap70_room" | cut -d' ' -f2)
  ap70_msg=$($ap70_c say "$ap70_rid" "hello world") || ap70_fail="$ap70_fail say"
  ap70_mid=$(echo "$ap70_msg" | cut -d' ' -f2)
  $ap70_c say "$ap70_rid" "second line" >>"$ap70_log" 2>&1 || ap70_fail="$ap70_fail say"
  $ap70_c show "$ap70_rid" >>"$ap70_log" 2>&1 || ap70_fail="$ap70_fail show"
  $ap70_c pin "$ap70_mid" >>"$ap70_log" 2>&1 || ap70_fail="$ap70_fail pin"
  ap70_lines=$($ap70_c journal | wc -l | tr -d ' ') || ap70_fail="$ap70_fail journal"
  ap70_last=$($ap70_c replay | tail -1) || ap70_fail="$ap70_fail replay"
  $ap70_c pin "$ap70_rid" >>"$ap70_log" 2>&1; ap70_bad=$?
  if [ -z "$ap70_fail" ] && [ "$ap70_lines" = 2 ] && [ "$ap70_last" = "room $ap70_rid total 2 replayed 2" ] && [ $ap70_bad -eq 1 ]; then
    say AP-05 "$ap70_ex" "journal 2 lines; $ap70_last; pin of a room rc=1" PASS
  else
    say AP-05 "$ap70_ex" "failed:${ap70_fail:- none}; journal $ap70_lines lines; last '$ap70_last'; pin of a room rc=$ap70_bad" FAIL
    head_of "$ap70_log" 8
  fi
  rm -rf "$ap70_d"
}

ap70_go AP-01 "TestChatlog rc=0" TestChatlog
ap70_plant AP-01p "TestChatlog fails on the plant" TestChatlog "" chatlog/main.go \
  's/^\tchat\.Total\.Set(b, ecr\.Of(rid), total)$/\t_ = total/'
ap70_plant AP-01q "TestChatlog fails on the plant" TestChatlog "" chatlog/main.go \
  's/^\t\t\treturn a\.h\.Engine\.Hydrate(k, at, src)$/\t\t\treturn nil/'
ap70_go AP-02 "TestRefusals rc=0" TestRefusals
ap70_plant AP-02p "TestRefusals/replay fails on the plant" TestRefusals replay chatlog/main.go \
  's/^\t\tif v\.Total != counts\[id\] {$/\t\tif false {/'
ap70_plant AP-02q "TestRefusals/system fails on the plant" TestRefusals system chatlog/main.go \
  's/^\tcase !ok || sys\.Class() != eid\.Detached:$/\tcase !ok:/'
ap70_go AP-03 "TestIter rc=0" TestIter
ap70_plant AP-03p "TestIter fails on the plant" TestIter "" chatlog/main.go \
  's/chat\.Line\.Scan(a\.h\.Store, chat\.KindMsg(a\.sys),/chat.Line.Scan(a.h.Store, chat.KindRom(a.sys),/'
ap70_boundrow AP-04 "imports bounded; chat_gen.go is go generate's output" "" ""
ap70_boundrow AP-04p "the hand edit is found" chatlog/chat/chat_gen.go \
  's/^\/\/ Total is the handle of the component total: .*/\/\/ Total is the handle of the component total, edited by hand./'
ap70_boundrow AP-04q "the import of go/sqlite is found" chatlog/main.go \
  's#^\t"github.com/fiberfx/ecd-runtime/go/ecr"$#\t"github.com/fiberfx/ecd-runtime/go/ecr"; _ "github.com/fiberfx/ecd-runtime/go/sqlite"#'
ap70_run
ap70_go AP-06 "TestMigrate rc=0" TestMigrate
ap70_plant AP-06p "TestMigrate fails on the plant" TestMigrate "" chatlog/main.go \
  's/^\t\tif err := ecr\.Migrate(path); err != nil {$/\t\tif err := error(nil); err != nil {/'
ap70_go AP-07 "TestOtherSystem rc=0" TestOtherSystem
ap70_plant AP-07p "TestOtherSystem fails on the plant" TestOtherSystem "" chatlog/main.go \
  's/sys, ok := ecr\.ParseCode(\*code)/sys, ok := ecr.ParseCode("3cl")/'
ap70_go AP-08 "TestMessageServed rc=0" TestMessageServed
ap70_plant AP-08p "TestMessageServed fails on the plant" TestMessageServed "" chatlog/main.go \
  's#^\t\treturn v, false, fmt\.Errorf("no message %v: %w", id, ecr\.ErrAbsent) // an engine that serves msg says absent$#\t\treturn v, false, nil#'
