# Group th60 of Ecd Runtime: story 15, the generator of typed values and handles from a catalog file. The generator
# go/internal/gen reads a strict JSON catalog of format 1 and refuses an unknown or repeated key, trailing data and every
# fault of the schema, sorted; it emits a gofmt-formatted file of the catalog, value types with a big-endian packed codec
# and typed handles, in the core mode (go/internal/llm) and the facade mode (go/ecr/internal/llm); the command
# go/cmd/gen writes the file through a temporary file and writes nothing for a refused catalog. The number 88 is above
# every group file present.
# Sourced by the rows harness from /w; the harness supplies say, sel, offline, head_of, count, why_of, go_row and
# HAVE_GO.
#
#   TH-01    go test -run '^TestCatalog$' ./internal/gen/: the llm catalog and a detached catalog are read; an unknown
#            key, a repeated key, trailing data, a key queries that is not an array, a detached or reserved system code, a repeated or
#            zero cid, a reserved package name, the kind sys, an unknown field type and a name that collides are
#            refused with their path and reason, sorted
#   TH-01p   plant: the strict pass records no unknown key (internal/gen/strict.go; TestCatalog, case unknownkey)
#   TH-01q   plant: the strict pass records no repeated key (internal/gen/strict.go; TestCatalog, case repeatedkey)
#   TH-01r   plant: a detached system code is accepted (internal/gen/check.go; TestCatalog, case system)
#   TH-02    go test -run '^(TestGolden|TestRun)$' ./internal/gen/ ./cmd/gen/: the regenerated files equal the
#            committed ones byte for byte in the core, detached and ecr cases, and a refused catalog writes no file
#   TH-02p   plant: the command writes the output before it checks the refusal (cmd/gen/main.go; TestRun, case refused)
#   TH-02q   plant: Append of a u32 field is little-endian (internal/gen/emit.go; TestGolden, case core)
#   TH-02r   plant: the facade mode qualifies with eid, ecd and rt (internal/gen/gen.go; TestGolden, case ecr)
#   TH-03    go test -run '^TestCodec$' ./internal/llm/: every value encodes to its declared bytes and decodes back,
#            Size equals the codec's length, and a value of another length or with a non-canonical byte is refused
#            with ErrValue
#   TH-03p   plant: TurnValue.Append writes Seq little-endian (internal/llm/llm_gen.go; TestCodec, case turn)
#   TH-03q   plant: UsageValue.Decode accepts a longer value (internal/llm/llm_gen.go; TestCodec, case usage)
#   TH-03r   plant: decodeF64 accepts any NaN (internal/llm/llm_gen.go; TestCodec, case usage)
#   TH-03b   the codec ruling's falsifier, measured: BenchmarkCodec reports (codec - copy) / apply, minimum of 3 counts; no gate on the ratio
#   TH-04    go test -run '^TestHandles$' ./internal/llm/: a spawn with every component and the tag reads back through
#            the handles on model and jacquard, and a read of an evicted Touched entity answers ErrMiss, never absent
#   TH-04p   plant: getValue answers absent for every error (internal/llm/llm_gen.go; TestHandles, case model/miss)
#   TH-04q   plant: Pinned.Add records an unset (internal/llm/llm_gen.go; TestHandles, case model/spawn)
#   TH-04r   plant: Turn.Set records under CIDUsage (internal/llm/llm_gen.go; TestHandles, case jacquard/spawn)
#   TH-10    go test -run '^TestQueries$' ./internal/gen/ ./internal/llm/: a catalog with a marked handle and a declared
#            query generates both with QIDs from 0, and a query on a Touched kind, an undeclared kind or an undeclared
#            component is refused with its path and reason
#   TH-10p   plant: a query on a Touched kind is accepted (internal/gen/check.go; TestQueries, case touched)
#   TH-10q   plant: a query on an undeclared kind is accepted (internal/gen/check.go; TestQueries, case undeclaredkind)
#   TH-10r   plant: a query of an undeclared component is accepted (internal/gen/check.go; TestQueries, case undeclaredcomponent)
#   TH-11    go test -run '^TestIter$' ./internal/llm/: Iter of a marked handle and of a declared query yields every
#            Warm entity once with its value or row, in the engine's order, on model and jacquard; a Cold kind answers
#            ErrCold and an undeclared query ErrQuery before any yield; a flush in the loop body panics with *IterError
#   TH-11e   the same test of the facade mode: go test -run '^TestIter$' ./internal/llm/ in go/ecr
#   TH-11p   plant: Iter ignores the error of the query (internal/llm/llm_gen.go; TestIter, case model/cold)
#   TH-11q   plant: a step never sees the engine move (internal/llm/llm_gen.go; TestIter, case model/moved)
#   TH-11r   plant: the sequence runs backwards (internal/llm/llm_gen.go; TestIter, case jacquard/turn)
#
# The facade-mode package go/ecr/internal/llm is tested by FC-01 (go test ./... in go/ecr) and its imports by FC-03.
#
# th60_plant id expect pkg test case file sedexpr: copies /w/go to a temporary directory, applies the one-line sed to
# go/<file> there and runs only the named test of package pkg in the copy. KILLED: the test failed in its case, and the
# observed text names its first failing line. SURVIVED: it passed, or failed in another case only. CNM: the sed did not
# change exactly one line, the file or a module was missing, the test panicked, or the test did not run, which
# includes a mutant that does not compile.
th60_plant() {
  th60_id=$1 th60_ex=$2 th60_pkg=$3 th60_test=$4 th60_case=$5 th60_file=$6 th60_sed=$7
  sel "$th60_id" th60 || return 0
  if [ $HAVE_GO -eq 0 ]; then say "$th60_id" "$th60_ex" "go not on PATH" CNM; return 0; fi
  th60_name=$th60_test/$th60_case
  th60_pd=$(mktemp -d /tmp/rows/plant.XXXXXX)
  cp -a /w/go "$th60_pd/go"
  th60_f=$th60_pd/go/$th60_file
  if [ ! -f "$th60_f" ]; then
    say "$th60_id" "$th60_ex" "go/$th60_file is absent" CNM
    rm -rf "$th60_pd"
    return 0
  fi
  cp "$th60_f" "$th60_pd/orig.go"
  sed -i "$th60_sed" "$th60_f"
  th60_n=$(diff "$th60_pd/orig.go" "$th60_f" | grep -c '^>')
  if [ "$th60_n" -ne 1 ]; then
    say "$th60_id" "$th60_ex" "the sed changed $th60_n lines of go/$th60_file, want 1" CNM
  else
    th60_log=/tmp/rows/$th60_id.log
    (cd "$th60_pd/go" && go test -count=1 -v -run "^$th60_test\$" "$th60_pkg" >"$th60_log" 2>&1)
    th60_rc=$?
    if offline "$th60_log"; then say "$th60_id" "$th60_ex" "module not found" CNM
    elif grep -q '^panic:' "$th60_log"; then
      say "$th60_id" "$th60_ex" "rc=$th60_rc, $th60_test panicked: $(why_of "$th60_log")" CNM
    elif grep -q -E "^ *--- FAIL: $th60_name " "$th60_log"; then
      say "$th60_id" "$th60_ex" "rc=$th60_rc, $th60_name FAIL: $(why_of "$th60_log")" KILLED
    elif grep -q "^--- FAIL: $th60_test " "$th60_log"; then
      say "$th60_id" "$th60_ex" "rc=$th60_rc, $th60_test FAIL, but not in $th60_name" SURVIVED
    elif [ $th60_rc -eq 0 ] && grep -q "^--- PASS: $th60_test " "$th60_log"; then
      say "$th60_id" "$th60_ex" "rc=0, $th60_test PASS" SURVIVED
    else say "$th60_id" "$th60_ex" "rc=$th60_rc, the test did not run" CNM; head_of "$th60_log" 8; fi
  fi
  rm -rf "$th60_pd"
}

go_row TH-01 th60 "TestCatalog rc=0" /w/go test -run '^TestCatalog$' ./internal/gen/
th60_plant TH-01p "TestCatalog/unknownkey fails on the plant" ./internal/gen/ TestCatalog unknownkey \
  internal/gen/strict.go 's/p\.refuse(kpath, "unknown key")/_ = kpath/'
th60_plant TH-01q "TestCatalog/repeatedkey fails on the plant" ./internal/gen/ TestCatalog repeatedkey \
  internal/gen/strict.go 's/p\.refuse(kpath, "repeated key")/_ = kpath/'
th60_plant TH-01r "TestCatalog/system fails on the plant" ./internal/gen/ TestCatalog system \
  internal/gen/check.go 's/case sys\.Class() == eid\.Detached:/case false:/'

go_row TH-02 th60 "TestGolden and TestRun rc=0" /w/go test -run '^(TestGolden|TestRun)$' ./internal/gen/ ./cmd/gen/
th60_plant TH-02p "TestRun/refused fails on the plant" ./cmd/gen/ TestRun refused cmd/gen/main.go \
  's/gen\.Generate(src, filepath\.Base(\*in), mode)$/gen.Generate(src, filepath.Base(*in), mode); _ = write(*out, got)/'
th60_plant TH-02q "TestGolden/core fails on the plant" ./internal/gen/ TestGolden core internal/gen/emit.go \
  's/return "dst = binary\.BigEndian\.AppendUint32(dst, " + x + ")"/return "dst = binary.LittleEndian.AppendUint32(dst, " + x + ")"/'
th60_plant TH-02r "TestGolden/ecr fails on the plant" ./internal/gen/ TestGolden ecr internal/gen/gen.go \
  's/^\tif m == ModeECR {$/\tif false {/'

go_row TH-03 th60 "TestCodec rc=0" /w/go test -run '^TestCodec$' ./internal/llm/
th60_plant TH-03p "TestCodec/turn fails on the plant" ./internal/llm/ TestCodec turn internal/llm/llm_gen.go \
  's/dst = binary\.BigEndian\.AppendUint32(dst, v\.Seq)/dst = binary.LittleEndian.AppendUint32(dst, v.Seq)/'
th60_plant TH-03q "TestCodec/usage fails on the plant" ./internal/llm/ TestCodec usage internal/llm/llm_gen.go \
  's/if len(b) != SizeUsage {/if len(b) < SizeUsage {/'
th60_plant TH-03r "TestCodec/usage fails on the plant" ./internal/llm/ TestCodec usage internal/llm/llm_gen.go \
  's/if math\.IsNaN(f) && bits != nan64 {/if false {/'

# TH-03b: the minimum ns/op of each sub-benchmark over 3 counts. The row passes when the benchmark runs and the
# ratio is reported; the ratio is a measurement for the acceptance owner (falsifier: above about 0.05), not a gate.
th60_ex="BenchmarkCodec reports (codec - copy) / apply for the llm values"
if sel TH-03b th60; then
  if [ $HAVE_GO -eq 0 ]; then say TH-03b "$th60_ex" "go not on PATH" CNM
  else
    th60_log=/tmp/rows/TH-03b.log
    (cd /w/go && go test -count=3 -run '^$' -bench '^BenchmarkCodec$' -benchtime 1s ./internal/llm/ >"$th60_log" 2>&1)
    th60_rc=$?
    th60_obs=$(awk '$1 ~ /^BenchmarkCodec\/(codec|copy|apply)(-[0-9]+)?$/ {
        split($1, p, "/"); s = p[2]; sub(/-[0-9]+$/, "", s); v = $3 + 0
        if (!(s in m) || v < m[s]) m[s] = v
      }
      END {
        if (("codec" in m) && ("copy" in m) && ("apply" in m) && m["apply"] > 0)
          printf "codec %.1f copy %.1f apply %.1f ns/op ratio %.4f %s\n", m["codec"], m["copy"], m["apply"],
            (m["codec"] - m["copy"]) / m["apply"], ((m["codec"] - m["copy"]) / m["apply"] <= 0.05) ? "within 0.05" : "over 0.05"
      }' "$th60_log")
    if offline "$th60_log"; then say TH-03b "$th60_ex" "module not found" CNM
    elif [ $th60_rc -ne 0 ]; then say TH-03b "$th60_ex" "rc=$th60_rc" FAIL; head_of "$th60_log" 20
    elif [ -z "$th60_obs" ]; then say TH-03b "$th60_ex" "rc=0, no three benchmark lines" CNM; head_of "$th60_log" 8
    else
      say TH-03b "$th60_ex" "$th60_obs" PASS
    fi
  fi
fi

go_row TH-04 th60 "TestHandles rc=0" /w/go test -run '^TestHandles$' ./internal/llm/
th60_plant TH-04p "TestHandles/model/miss fails on the plant" ./internal/llm/ TestHandles model/miss \
  internal/llm/llm_gen.go 's/^\tif errors\.Is(err, ecd\.ErrAbsent) {$/\tif err != nil {/'
th60_plant TH-04q "TestHandles/model/spawn fails on the plant" ./internal/llm/ TestHandles model/spawn \
  internal/llm/llm_gen.go 's/{ b\.Set(r, CIDPinned, nil) }$/{ b.Unset(r, CIDPinned) }/'
th60_plant TH-04r "TestHandles/jacquard/spawn fails on the plant" ./internal/llm/ TestHandles jacquard/spawn \
  internal/llm/llm_gen.go 's/b\.Set(r, CIDTurn, v\.Append(a\[:0\]))/b.Set(r, CIDUsage, v.Append(a[:0]))/'

go_row TH-10 th60 "TestQueries rc=0" /w/go test -run '^TestQueries$' ./internal/gen/ ./internal/llm/
th60_plant TH-10p "TestQueries/touched fails on the plant" ./internal/gen/ TestQueries touched \
  internal/gen/check.go 's/case kind\.Residency == "touched":/case kind.Residency == "Touched":/'
th60_plant TH-10q "TestQueries/undeclaredkind fails on the plant" ./internal/gen/ TestQueries undeclaredkind \
  internal/gen/check.go 's/case !kindDeclared:/case !kindDeclared \&\& false:/'
th60_plant TH-10r "TestQueries/undeclaredcomponent fails on the plant" ./internal/gen/ TestQueries undeclaredcomponent \
  internal/gen/check.go 's/case !compDeclared:/case !compDeclared \&\& false:/'

go_row TH-11 th60 "TestIter rc=0" /w/go test -run '^TestIter$' ./internal/llm/
go_row TH-11e th60 "TestIter of the facade mode rc=0" /w/go/ecr test -run '^TestIter$' ./internal/llm/
th60_plant TH-11p "TestIter/model/cold fails on the plant" ./internal/llm/ TestIter model/cold \
  internal/llm/llm_gen.go 's/if err := e\.Query(q, nil, &a); err != nil {/if err := e.Query(q, nil, \&a); false \&\& err != nil {/'
th60_plant TH-11q "TestIter/model/moved fails on the plant" ./internal/llm/ TestIter model/moved \
  internal/llm/llm_gen.go 's/if now := e\.At(); now != at {/if now := e.At(); now < at {/'
th60_plant TH-11r "TestIter/jacquard/turn fails on the plant" ./internal/llm/ TestIter jacquard/turn \
  internal/llm/llm_gen.go 's/for _, id := range a\.IDs {/for i := range a.IDs { id := a.IDs[len(a.IDs)-1-i]/'
