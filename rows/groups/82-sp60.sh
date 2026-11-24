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
