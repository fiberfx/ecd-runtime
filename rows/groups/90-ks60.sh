# Group ks60 of Ecd Runtime: story 24, a typed scan of a kind from the owner store. Store.Scan of go/sqlite answers
# the stored values of one component of the entities of one kind, in eid order, from one read transaction, after a
# given eid; the generated Scan of a marked handle decodes them through ecd.Scanner and stops at a value that does not
# decode, with ErrValue, and at an error of its f. The number 90 is above every group file present.
# Sourced by the rows harness from /w; the harness supplies say, sel, offline, head_of, count, why_of, go_row and
# HAVE_GO.
#
#   KS-01    go test -run '^TestScan$' . in go/sqlite: a scan of a kind and a component yields every live entity of the
#            kind that holds it once, in eid order, with its value, at the store's seq; no destroyed entity, no entity
#            of another kind and none without the component; it resumes after an eid; an after of another kind is
#            refused before f
#   KS-01p   plant: the scan reads every component from c up (sqlite/scan.go; TestScan, case excluded)
#   KS-01q   plant: the scan reads past the kind's range (sqlite/scan.go; TestScan, case excluded)
#   KS-01r   plant: a resumed scan yields its after again (sqlite/scan.go; TestScan, case resume)
#   KS-02    go test -run '^TestScan$' ./internal/llm/: Turn.Scan of cnv and of arc yields what Turn.Iter yields on a
#            hydrated engine, model and jacquard, kind by kind in eid order; a Turn of the wrong length answers
#            ErrValue and stops the scan; an error of f stops it and is returned; a resumed scan starts after its eid
#   KS-02e   go test -run '^TestScanHost$' ./internal/llm/ in go/ecr: on a host that ecr.Open opened, Turn.Iter answers
#            ErrCold and Turn.Scan of the store yields every cnv with its flushed value
#   KS-02p   plant: Scan ignores a value that does not decode (internal/llm/llm_gen.go; TestScan, case length)
#   KS-02q   plant: Scan ignores the error of f (internal/llm/llm_gen.go; TestScan, case stop)
#   KS-02r   plant: Turn.Scan reads the component usage (internal/llm/llm_gen.go; TestScan, case model)
#
# ks60_plant id expect mod pkg test case file sedexpr: copies /w/go to a temporary directory, applies the one-line sed
# to go/<file> there and runs only the named test of package pkg in module directory go/<mod> of the copy. The copy is
# the whole of /w/go, since the nested modules replace the main module by a parent directory. KILLED: the test failed
# in its case (the whole test when case is empty), and the observed text names its first failing line. SURVIVED: it
# passed, or failed in another case only. CNM: the sed did not change exactly one line, the file or a module was
# missing, the test panicked, or the test did not run, which includes a mutant that does not compile.
ks60_plant() {
  ks60_id=$1 ks60_ex=$2 ks60_mod=$3 ks60_pkg=$4 ks60_test=$5 ks60_case=$6 ks60_file=$7 ks60_sed=$8
  sel "$ks60_id" ks60 || return 0
  if [ $HAVE_GO -eq 0 ]; then say "$ks60_id" "$ks60_ex" "go not on PATH" CNM; return 0; fi
  ks60_name=$ks60_test
  if [ -n "$ks60_case" ]; then ks60_name=$ks60_test/$ks60_case; fi
  ks60_pd=$(mktemp -d /tmp/rows/plant.XXXXXX)
  cp -a /w/go "$ks60_pd/go"
  ks60_f=$ks60_pd/go/$ks60_file
  if [ ! -f "$ks60_f" ]; then
    say "$ks60_id" "$ks60_ex" "go/$ks60_file is absent" CNM
    rm -rf "$ks60_pd"
    return 0
  fi
  cp "$ks60_f" "$ks60_pd/orig.go"
  sed -i "$ks60_sed" "$ks60_f"
  ks60_n=$(diff "$ks60_pd/orig.go" "$ks60_f" | grep -c '^>')
  if [ "$ks60_n" -ne 1 ]; then
    say "$ks60_id" "$ks60_ex" "the sed changed $ks60_n lines of go/$ks60_file, want 1" CNM
  else
    ks60_log=/tmp/rows/$ks60_id.log
    (cd "$ks60_pd/go/$ks60_mod" && go test -count=1 -v -run "^$ks60_test\$" "$ks60_pkg" >"$ks60_log" 2>&1)
    ks60_rc=$?
    if offline "$ks60_log"; then say "$ks60_id" "$ks60_ex" "module not found" CNM
    elif grep -q '^panic:' "$ks60_log"; then
      say "$ks60_id" "$ks60_ex" "rc=$ks60_rc, $ks60_test panicked: $(why_of "$ks60_log")" CNM
    elif grep -q -E "^ *--- FAIL: $ks60_name " "$ks60_log"; then
      say "$ks60_id" "$ks60_ex" "rc=$ks60_rc, $ks60_name FAIL: $(why_of "$ks60_log")" KILLED
    elif grep -q "^--- FAIL: $ks60_test " "$ks60_log"; then
      say "$ks60_id" "$ks60_ex" "rc=$ks60_rc, $ks60_test FAIL, but not in $ks60_name" SURVIVED
    elif [ $ks60_rc -eq 0 ] && grep -q "^--- PASS: $ks60_test " "$ks60_log"; then
      say "$ks60_id" "$ks60_ex" "rc=0, $ks60_test PASS" SURVIVED
    else say "$ks60_id" "$ks60_ex" "rc=$ks60_rc, the test did not run" CNM; head_of "$ks60_log" 8; fi
  fi
  rm -rf "$ks60_pd"
}

go_row KS-01 ks60 "TestScan rc=0" /w/go/sqlite test -run '^TestScan$' .
ks60_plant KS-01p "TestScan/excluded fails on the plant" sqlite . TestScan excluded sqlite/scan.go \
  's/WHERE cid = ?1 AND/WHERE cid >= ?1 AND/'
ks60_plant KS-01q "TestScan/excluded fails on the plant" sqlite . TestScan excluded sqlite/scan.go \
  's/r\.Lo\[:\], r\.Hi\[:\], after/r.Lo[:], []byte{0xff}, after/'
ks60_plant KS-01r "TestScan/resume fails on the plant" sqlite . TestScan resume sqlite/scan.go \
  's/AND id > ?4 ORDER/AND id >= ?4 ORDER/'

go_row KS-02 ks60 "TestScan rc=0" /w/go test -run '^TestScan$' ./internal/llm/
go_row KS-02e ks60 "TestScanHost of the facade mode rc=0" /w/go/ecr test -run '^TestScanHost$' ./internal/llm/
ks60_plant KS-02p "TestScan/length fails on the plant" . ./internal/llm/ TestScan length internal/llm/llm_gen.go \
  's/if err := P(&v)\.Decode(r\.Value); err != nil {/if err := P(\&v).Decode(r.Value); false \&\& err != nil {/'
ks60_plant KS-02q "TestScan/stop fails on the plant" . ./internal/llm/ TestScan stop internal/llm/llm_gen.go \
  's/if err := f(r\.Entity, v); err != nil {/if err := f(r.Entity, v); false \&\& err != nil {/'
ks60_plant KS-02r "TestScan/model fails on the plant" . ./internal/llm/ TestScan model internal/llm/llm_gen.go \
  's/return scanValues\[TurnValue\](s, k, CIDTurn, after, f)/return scanValues[TurnValue](s, k, CIDUsage, after, f)/'
