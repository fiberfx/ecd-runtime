# Group sp60 of Ecd Runtime: a deed carries the W3C trace id and span id of the span that made it. The rows show that
# ecd.Deed holds them as ecd.TraceID and ecd.SpanID and that no engine reads them; that a buffer's Span call puts
# them on the buffer's next deed and the flush that empties the buffer resets them; that the owner store keeps them
# in deed.trace_id and deed.span_id, NULL for the zero values, and Log returns them; that Open refuses any user
# version but 2; that Migrate brings a store of user version 1 to 2; and that the nested module go/sqlite requires
# the main module at v0.6.0 and replaces it by its parent directory.
# Sourced by the rows harness from /w; the harness supplies say, sel, offline, head_of, count, why_of, go_row and
# HAVE_GO.
#
#   SP-01    Deed has the fields TraceID of type ecd.TraceID and SpanID of type ecd.SpanID, and engine.go states their
#            zero values and that no engine reads them (TestDeedSpanFields in go/ecd)
#   SP-01p   plant: Deed.TraceID is of type SpanID (ecd/engine.go; TestDeedSpanFields, case types)
#   SP-01q   plant: the Deed comment drops "No engine reads them." (ecd/engine.go; TestDeedSpanFields, case comment)
#   SP-07    no non-test file of go/engine or go/enginecmp reads a field TraceID or SpanID; the type names ecd.TraceID
#            and ecd.SpanID are not reads, and go/sqlite's reads are the positive control
#   SP-07p   plant: a copy of engine/jacquard/engine.go whose deed order compares TraceID; the scan must find it
#   SP-02    a deed carries its buffer's last Span ids, the zero values without one, and a flush that empties the
#            buffer, committed or refused, resets them; a buffer with no record keeps them (TestBufferSpan in go/rt)
#   SP-02p   plant: the flush builds the deed without the buffer's ids (rt/flush.go; TestBufferSpan, case carried)
#   SP-02q   plant: emptying a buffer keeps its ids (rt/buffer.go; TestBufferSpan, case reset)
#   SP-03    Commit stores a deed's ids as 16 and 8 bytes and the zero values as NULL, Log returns them, the schema
#            refuses an id of another length, and Log answers an all-zero stored id with ErrSchema (TestDeedSpan)
#   SP-03p   plant: Commit binds the zero trace id (sqlite/commit.go; TestDeedSpan, case ids)
#   SP-03q   plant: decodeDeed skips the span id (sqlite/read.go; TestDeedSpan, case ids)
#   SP-03r   plant: a refused stored trace id does not match ErrSchema (sqlite/read.go; TestDeedSpan, case malformed)
#   SP-03s   plant: the span_id check admits 16 bytes (sqlite/store.go; TestDeedSpan, case length)
#   SP-03t   plant: a refused span id does not match ErrSchema (sqlite/read.go; TestDeedSpan, case malformed_span)
#   SP-04    Create writes user version 2, and Open refuses a store of user version 1 (TestCreate)
#   SP-04p   plant: Open admits user version 1 (sqlite/store.go; TestCreate)
#   SP-04q   plant: Create writes user version 1 (sqlite/store.go; TestCreate)
#   SP-05    Migrate brings a store of user version 1 to 2 with its deed intact, leaves a store of version 2 unchanged
#            and refuses other files with ErrSchema and no change (TestMigrate)
#   SP-05p   plant: Migrate sets user version 1 (sqlite/migrate.go; TestMigrate, case v1)
#   SP-05q   plant: Migrate alters a store of user version 2 (sqlite/migrate.go; TestMigrate, case current)
#   SP-05r   plant: Migrate skips the journal mode check (sqlite/migrate.go; TestMigrate, case refused)
#   SP-06    go/sqlite/go.mod requires github.com/fiberfx/ecd-runtime/go v0.6.0 and replaces it by ../
#
# Rows SP-03, SP-04 and SP-05 and their plants load the driver and its modules from the module inputs, as the rows of
# group sq40 do. The replace in go/sqlite/go.mod makes every build of go/sqlite here compile against the main module
# of the same checkout, so these rows measure the two modules at one revision; the require line is what a consumer
# outside the repository resolves, and SP-06 reads it.
#
# sp60_plant id expect mod pkg test case file sedexpr: copies /w/go to a temporary directory, applies the one-line sed
# to go/<file> there and runs only the named test of package pkg in module directory go/<mod> of the copy. The copy is
# the whole of /w/go, since the nested modules replace the main module by a parent directory. KILLED: the test failed
# in its case (the whole test when case is empty), and the observed text names its first failing line. SURVIVED: it
# passed, or failed in another case only. CNM: the sed did not change exactly one line, the file or a module was
# missing, the test panicked, or the test did not run, which includes a mutant that does not compile.
sp60_plant() {
  sp60_id=$1 sp60_ex=$2 sp60_mod=$3 sp60_pkg=$4 sp60_test=$5 sp60_case=$6 sp60_file=$7 sp60_sed=$8
  sel "$sp60_id" sp60 || return 0
  if [ $HAVE_GO -eq 0 ]; then say "$sp60_id" "$sp60_ex" "go not on PATH" CNM; return 0; fi
  sp60_name=$sp60_test
  if [ -n "$sp60_case" ]; then sp60_name=$sp60_test/$sp60_case; fi
  sp60_pd=$(mktemp -d /tmp/rows/plant.XXXXXX)
  cp -a /w/go "$sp60_pd/go"
  sp60_f=$sp60_pd/go/$sp60_file
  if [ ! -f "$sp60_f" ]; then
    say "$sp60_id" "$sp60_ex" "go/$sp60_file is absent" CNM
    rm -rf "$sp60_pd"
    return 0
  fi
  cp "$sp60_f" "$sp60_pd/orig.go"
  sed -i "$sp60_sed" "$sp60_f"
  sp60_n=$(diff "$sp60_pd/orig.go" "$sp60_f" | grep -c '^>')
  if [ "$sp60_n" -ne 1 ]; then
    say "$sp60_id" "$sp60_ex" "the sed changed $sp60_n lines of go/$sp60_file, want 1" CNM
  else
    sp60_log=/tmp/rows/$sp60_id.log
    (cd "$sp60_pd/go/$sp60_mod" && go test -count=1 -v -run "^$sp60_test\$" "$sp60_pkg" >"$sp60_log" 2>&1)
    sp60_rc=$?
    if offline "$sp60_log"; then say "$sp60_id" "$sp60_ex" "module not found" CNM
    elif grep -q '^panic:' "$sp60_log"; then
      say "$sp60_id" "$sp60_ex" "rc=$sp60_rc, $sp60_test panicked: $(why_of "$sp60_log")" CNM
    elif grep -q -E "^ *--- FAIL: $sp60_name " "$sp60_log"; then
      say "$sp60_id" "$sp60_ex" "rc=$sp60_rc, $sp60_name FAIL: $(why_of "$sp60_log")" KILLED
    elif grep -q "^--- FAIL: $sp60_test " "$sp60_log"; then
      say "$sp60_id" "$sp60_ex" "rc=$sp60_rc, $sp60_test FAIL, but not in $sp60_name" SURVIVED
    elif [ $sp60_rc -eq 0 ] && grep -q "^--- PASS: $sp60_test " "$sp60_log"; then
      say "$sp60_id" "$sp60_ex" "rc=0, $sp60_test PASS" SURVIVED
    else say "$sp60_id" "$sp60_ex" "rc=$sp60_rc, the test did not run" CNM; head_of "$sp60_log" 8; fi
  fi
  rm -rf "$sp60_pd"
}

go_row SP-01 sp60 "TestDeedSpanFields rc=0" /w/go test -run '^TestDeedSpanFields$' ./ecd/
sp60_plant SP-01p "TestDeedSpanFields/types fails on the plant" . ./ecd/ TestDeedSpanFields types ecd/engine.go \
  's/^\tTraceID TraceID$/\tTraceID SpanID/'
sp60_plant SP-01q "TestDeedSpanFields/comment fails on the plant" . ./ecd/ TestDeedSpanFields comment ecd/engine.go \
  's/ No engine reads them\.$//'

# sp60_reads dir...: the lines of the non-test Go files under the directories that name TraceID or SpanID after a dot,
# once a use of the type names ecd.TraceID and ecd.SpanID is struck out, so that only a field read or write is left.
sp60_reads() {
  grep -rnE '\.(TraceID|SpanID)\b' --include='*.go' "$@" 2>/dev/null | grep -v '_test\.go:' |
    sed -E 's/\becd\.(TraceID|SpanID)\b//g' | grep -E '\.(TraceID|SpanID)\b'
}

if sel SP-07 sp60; then
  sp60_ctl=$(sp60_reads /w/go/sqlite | wc -l | tr -d ' ')
  if [ ! -d /w/go/engine ] || [ ! -d /w/go/enginecmp ]; then
    say SP-07 "no engine file reads TraceID or SpanID" "go/engine or go/enginecmp is absent" CNM
  elif [ "$sp60_ctl" -eq 0 ]; then
    say SP-07 "no engine file reads TraceID or SpanID" "the pattern finds no line of go/sqlite either" CNM
  else
    sp60_n=$(sp60_reads /w/go/engine /w/go/enginecmp | wc -l | tr -d ' ')
    if [ "$sp60_n" -eq 0 ]; then say SP-07 "no engine file reads TraceID or SpanID" "0 lines; go/sqlite $sp60_ctl" PASS
    else say SP-07 "no engine file reads TraceID or SpanID" "$sp60_n lines" FAIL; fi
  fi
fi

# SP-07p copies /w/go, changes the line of go/engine/jacquard/engine.go that orders two deeds by Ord into one that
# compares their TraceID, and scans the copy as SP-07 scans the tree. KILLED: the scan finds that line, so SP-07 would
# turn red. SURVIVED: it finds none. CNM: the file is absent or the sed did not change exactly one line.
if sel SP-07p sp60; then
  sp60_want="the SP-07 scan finds the plant"
  sp60_pd=$(mktemp -d /tmp/rows/plant.XXXXXX)
  cp -a /w/go "$sp60_pd/go"
  sp60_f=$sp60_pd/go/engine/jacquard/engine.go
  if [ ! -f "$sp60_f" ]; then
    say SP-07p "$sp60_want" "go/engine/jacquard/engine.go is absent" CNM
  else
    cp "$sp60_f" "$sp60_pd/orig.go"
    sed -i 's/^\t\tcase a\.Ord < b\.Ord:$/\t\tcase a.TraceID == b.TraceID:/' "$sp60_f"
    sp60_c=$(diff "$sp60_pd/orig.go" "$sp60_f" | grep -c '^>')
    sp60_n=$(sp60_reads "$sp60_pd/go/engine" "$sp60_pd/go/enginecmp" | wc -l | tr -d ' ')
    if [ "$sp60_c" -ne 1 ]; then
      say SP-07p "$sp60_want" "the sed changed $sp60_c lines, want 1" CNM
    elif [ "$sp60_n" -ge 1 ]; then say SP-07p "$sp60_want" "$sp60_n lines" KILLED
    else say SP-07p "$sp60_want" "0 lines" SURVIVED; fi
  fi
  rm -rf "$sp60_pd"
fi

go_row SP-02 sp60 "TestBufferSpan rc=0" /w/go test -run '^TestBufferSpan$' ./rt/
sp60_plant SP-02p "TestBufferSpan/carried fails on the plant" . ./rt/ TestBufferSpan carried rt/flush.go \
  's/TraceID: b\.trace, SpanID: b\.span, //'
sp60_plant SP-02q "TestBufferSpan/reset fails on the plant" . ./rt/ TestBufferSpan reset rt/buffer.go \
  's/^\tb\.trace, b\.span = ecd\.TraceID{}, ecd\.SpanID{}$/\t\/\/ the reset of the ids is dropped/'

go_row SP-03 sp60 "TestDeedSpan rc=0" /w/go/sqlite test -run '^TestDeedSpan$' .
sp60_plant SP-03p "TestDeedSpan/ids fails on the plant" sqlite . TestDeedSpan ids sqlite/commit.go \
  's/at, d\.TraceID, d\.SpanID))$/at, ecd.TraceID{}, d.SpanID))/'
sp60_plant SP-03q "TestDeedSpan/ids fails on the plant" sqlite . TestDeedSpan ids sqlite/read.go \
  's/^\tif span != nil {$/\tif span != nil \&\& false {/'
sp60_plant SP-03r "TestDeedSpan/malformed fails on the plant" sqlite . TestDeedSpan malformed sqlite/read.go \
  's/"sqlite: trace id: %w: %w", err, ErrSchema/"sqlite: trace id: %w", err/'
sp60_plant SP-03s "TestDeedSpan/length fails on the plant" sqlite . TestDeedSpan length sqlite/store.go \
  's/CHECK (span_id IS NULL OR length(span_id) = 8)/CHECK (span_id IS NULL OR length(span_id) >= 8)/'
sp60_plant SP-03t "TestDeedSpan/malformed_span fails on the plant" sqlite . TestDeedSpan malformed_span sqlite/read.go \
  's/"sqlite: span id: %w: %w", err, ErrSchema/"sqlite: span id: %w", err/'

go_row SP-04 sp60 "TestCreate rc=0" /w/go/sqlite test -run '^TestCreate$' .
sp60_plant SP-04p "TestCreate fails on the plant" sqlite . TestCreate '' sqlite/store.go \
  's/^\tif v != 2 || mode != "wal" {$/\tif v > 2 || v < 1 || mode != "wal" {/'
sp60_plant SP-04q "TestCreate fails on the plant" sqlite . TestCreate '' sqlite/store.go \
  's/"PRAGMA user_version = 2"/"PRAGMA user_version = 1"/'

go_row SP-05 sp60 "TestMigrate rc=0" /w/go/sqlite test -run '^TestMigrate$' .
sp60_plant SP-05p "TestMigrate/v1 fails on the plant" sqlite . TestMigrate v1 sqlite/migrate.go \
  's/^PRAGMA user_version = 2;`$/PRAGMA user_version = 1;`/'
sp60_plant SP-05q "TestMigrate/current fails on the plant" sqlite . TestMigrate current sqlite/migrate.go \
  's/^\tcase v == 2:$/\tcase v == 3:/'
sp60_plant SP-05r "TestMigrate/refused fails on the plant" sqlite . TestMigrate refused sqlite/migrate.go \
  's/^\tcase mode != "wal" || v < 1 || v > 2:$/\tcase v < 1 || v > 2:/'

if sel SP-06 sp60; then
  sp60_req=$(grep -cE '^[[:space:]]+github\.com/fiberfx/ecd-runtime/go v0\.6\.0$' /w/go/sqlite/go.mod)
  sp60_rep=$(grep -cxF 'replace github.com/fiberfx/ecd-runtime/go => ../' /w/go/sqlite/go.mod)
  if [ "$sp60_req" -eq 1 ] && [ "$sp60_rep" -eq 1 ]; then
    say SP-06 "go/sqlite requires go v0.6.0 and replaces it by ../" "require 1, replace 1" PASS
  else say SP-06 "go/sqlite requires go v0.6.0 and replaces it by ../" "require $sp60_req, replace $sp60_rep" FAIL; fi
fi
