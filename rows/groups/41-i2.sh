# Group i2 of Entity Component Runtime: L06, I2-01 to I2-06 and their plants (the rows ending in p).
# Package ecd (go/ecd): the id types over eid, the trace and span ids, and the engine seam. Rules E1 to E9 bind the
# engines that implement the seam; this group checks the seam's types, constants and sentinels.
# Sourced by the rows harness from /w; the harness supplies say, sel, offline, head_of, count, why_of, go_row and
# HAVE_GO.
#
#   row    check
#   L06    the TraceID and SpanID declarations refuse every conversion with eid.ID and with an array (a probe package
#          fails to build with six "cannot convert" errors and no other error); eid.ParseString refuses 32 and 16 hex
#          characters, ParseTraceID and ParseSpanID refuse an eid text; MarshalText and Value of the zero eid refuse with eid.ErrZero
#   I2-01  KindRange bytes, the ids inside and the ids of the neighbouring kinds and systems outside; Window(k, 0, TimeMax+1) equals KindRange(k)
#   I2-02  Window bounds at from-1, from, to-1 and to; Contains(Hi) is false; from > to and to > TimeMax+1 answer eid.ErrRange
#   I2-03  Day at 0, 86,399,999 and 86,400,000 ms
#   I2-04  trace and span id: parse, String, Bytes, IsZero, Value, Scan; uppercase, 0x, UUID text, all zero, wrong length and a Scan of a string refused
#   I2-05  the seam: a stub satisfies ecd.Engine, the constant values, the error texts, the sentinels never match each other
#   I2-06  go/go.mod says go 1.27.4 and requires only ecd-id v0.11.0, go/go.sum holds its two lines, go version reads go1.27; a go.mod with another require is refused
#
# Each plant is a one-line sed on a temporary copy of /w/go that re-runs only the tests of its row. A plant is KILLED
# when a test fails and SURVIVED when every test passes; a plant that does not change exactly one line, or in which no
# test ran, is CNM.
#   I2-01p  "hi := p + 1" becomes "hi := p + 4" in kind.go
#   I2-02p  Contains compares "<= 0" with Hi in kind.go
#   I2-03p  86_400_000 becomes 86_400 in kind.go
#   I2-04p  ParseTraceID lowercases its input first, in trace.go
#   I2-05p  OpSet takes the value 3 (iota + 2) in engine.go
#   I2-06p  "go 1.27.4" becomes "go 1.24" in a temporary go.mod
#   L06p    "type TraceID struct{ b [16]byte }" becomes "type TraceID [16]byte" in the declaration lines the probe extracts

# i2_plant row file sed-expression test-pattern expectation: the plant on go/ecd/<file>.
i2_plant() {
  i2_id=$1 i2_file=$2 i2_expr=$3 i2_pat=$4 i2_what=$5
  sel "$i2_id" i2 || return 0
  if [ $HAVE_GO -eq 0 ]; then say "$i2_id" "$i2_what" "go not on PATH" CNM; return 0; fi
  i2_pd=$(mktemp -d /tmp/rows/plant.XXXXXX)
  cp -a /w/go "$i2_pd/go" && cp "$i2_pd/go/ecd/$i2_file" "$i2_pd/orig" && sed -i "$i2_expr" "$i2_pd/go/ecd/$i2_file"
  i2_del=$(diff "$i2_pd/orig" "$i2_pd/go/ecd/$i2_file" | grep -c '^<')
  i2_add=$(diff "$i2_pd/orig" "$i2_pd/go/ecd/$i2_file" | grep -c '^>')
  if [ "$i2_del" != 1 ] || [ "$i2_add" != 1 ]; then
    say "$i2_id" "$i2_what" "the sed removed $i2_del and added $i2_add lines of $i2_file (one each expected)" CNM
  else
    i2_log=/tmp/rows/$i2_id.log
    (cd "$i2_pd/go" && go test -count=1 -v -run "$i2_pat" ./ecd/ >"$i2_log" 2>&1); i2_rc=$?
    if offline "$i2_log"; then say "$i2_id" "$i2_what" "module not found" CNM
    elif grep -q '^--- FAIL' "$i2_log"; then
      say "$i2_id" "$i2_what" "rc=$i2_rc, a test FAIL on the plant: $(why_of "$i2_log")" KILLED
    elif [ $i2_rc -eq 0 ] && grep -q '^--- PASS' "$i2_log"; then say "$i2_id" "$i2_what" "rc=0, every test PASS on the plant" SURVIVED
    else say "$i2_id" "$i2_what" "rc=$i2_rc, the tests did not run" CNM; head_of "$i2_log" 8; fi
  fi
  rm -rf "$i2_pd"
}

go_row I2-01 i2 "KindRange bytes, ids inside and outside, Window over all time equals KindRange" /w/go test -run '^(TestKindOf|TestKindRange|TestKindRangeEids|TestKindRangeFullWindow)$' ./ecd/
i2_plant I2-01p kind.go 's/hi := p + 1$/hi := p + 4/' '^(TestKindRange|TestKindRangeFullWindow)$' "the Hi computation adds 4 and the I2-01 tests fail"

go_row I2-02 i2 "Window bounds at from-1, from, to-1 and to; Contains(Hi) false; refusals answer eid.ErrRange" /w/go test -run '^(TestWindow|TestWindowRefusals)$' ./ecd/
i2_plant I2-02p kind.go 's/bytes.Compare(id\[:\], r.Hi\[:\]) < 0/bytes.Compare(id[:], r.Hi[:]) <= 0/' '^(TestWindow|TestWindowRefusals)$' "Contains compares <= 0 with Hi and the I2-02 tests fail"

go_row I2-03 i2 "Day at 0, 86,399,999 and 86,400,000 ms" /w/go test -run '^TestDay$' ./ecd/
i2_plant I2-03p kind.go 's/86_400_000/86_400/' '^TestDay$' "the day length is 86_400 and TestDay fails"

go_row I2-04 i2 "trace and span ids: the good forms accepted, the violating forms refused" /w/go test -run '^Test(Trace|Span)ID' ./ecd/
i2_plant I2-04p trace.go 's/parseHex(t\.b\[:\], s, ErrTraceID)/parseHex(t.b[:], strings.ToLower(s), ErrTraceID)/' '^Test(Trace|Span)ID' "ParseTraceID lowercases its input and the I2-04 tests fail"

go_row I2-05 i2 "a stub satisfies ecd.Engine, constants, error texts, distinct sentinels" /w/go test -run '^TestSeam' ./ecd/
i2_plant I2-05p engine.go 's/^\tOpSet$/\tOpSet OpCode = iota + 2/' '^TestSeam' "OpSet takes the value 3 and the I2-05 tests fail"

# i2_gomod_ok file: 0 when the file says "go 1.27.4" and its only requirement is ecd-id v0.11.0.
i2_gomod_ok() {
  grep -qx 'go 1\.27\.4' "$1" || return 1
  i2_req=$(awk '/^require[ \t]*\(/{b=1;next} b&&/^\)/{b=0;next} b{print;next} /^require[ \t]/{sub(/^require[ \t]+/,"");print}' "$1" | sed 's/^[ \t]*//;s/[ \t]*$//')
  [ "$i2_req" = "github.com/fiberfx/ecd-id/go v0.11.0" ]
}

if sel I2-06 i2 || sel I2-06p i2; then
  i2_pd=$(mktemp -d /tmp/rows/plant.XXXXXX)
  if sel I2-06 i2; then
    i2_what="go.mod: go 1.27.4 and only ecd-id v0.11.0; go.sum: its two lines; go version go1.27; a go.mod with another require is refused"
    if [ $HAVE_GO -eq 0 ]; then say I2-06 "$i2_what" "go not on PATH" CNM
    else
      i2_obs=""
      i2_gomod_ok /w/go/go.mod || i2_obs="$i2_obs go.mod is not go 1.27.4 with only ecd-id v0.11.0;"
      cp /w/go/go.mod "$i2_pd/other.mod" && printf '\nrequire example.com/other v1.0.0\n' >>"$i2_pd/other.mod"
      ! i2_gomod_ok "$i2_pd/other.mod" || i2_obs="$i2_obs a go.mod with another require line was accepted;"
      i2_sum=$(printf '%s\n%s\n' 'github.com/fiberfx/ecd-id/go v0.11.0 h1:M8D12NQ8ARnQDZanpFV23/uwC7r7mzzfFNxtRLDl+oE=' 'github.com/fiberfx/ecd-id/go v0.11.0/go.mod h1:M5YOPf1cdIPU87e2ZFpmC9OdYXCGRRl24BgWeRGrdOY=')
      [ "$(cat /w/go/go.sum)" = "$i2_sum" ] || i2_obs="$i2_obs go.sum is not the two ecd-id lines;"
      i2_gv=$(go version 2>&1)
      case $i2_gv in *' go1.27'*) ;; *) i2_obs="$i2_obs go version reads: $i2_gv;" ;; esac
      if [ -z "$i2_obs" ]; then say I2-06 "$i2_what" "go.mod and go.sum as expected; $i2_gv; the other require refused" PASS
      else say I2-06 "$i2_what" "$i2_obs" FAIL; fi
    fi
  fi
  if sel I2-06p i2; then
    i2_what="go.mod says go 1.24 and the go.mod check turns red"
    cp /w/go/go.mod "$i2_pd/plant.mod" && sed -i 's/^go 1\.27\.4$/go 1.24/' "$i2_pd/plant.mod"
    if cmp -s /w/go/go.mod "$i2_pd/plant.mod"; then say I2-06p "$i2_what" "go.mod unchanged after sed" CNM
    elif i2_gomod_ok "$i2_pd/plant.mod"; then say I2-06p "$i2_what" "the check still accepts the plant" SURVIVED
    else say I2-06p "$i2_what" "the check refuses go 1.24" KILLED; fi
  fi
  rm -rf "$i2_pd"
fi

# i2_probe dir: in the module copy at dir, builds a package that holds the TraceID and SpanID declaration lines of
# ecd/trace.go and six conversions: eid.ID(t), TraceID(id), eid.ID(s), SpanID(id), [8]byte(s) and SpanID(a8).
# Returns 0 when the build fails with six "cannot convert" lines and no other error, 1 when it does not, 2 when it
# measured nothing. i2_obs holds the observation.
i2_probe() {
  i2_dir=$1
  mkdir -p "$i2_dir/probe" || return 2
  {
    printf 'package probe\n\nimport eid "github.com/fiberfx/ecd-id/go"\n\n'
    grep -E '^type (TraceID|SpanID) ' "$i2_dir/ecd/trace.go"
    cat <<'PROBE'

var (
	t  TraceID
	s  SpanID
	id eid.ID
	a8 [8]byte
)

var (
	c1 = eid.ID(t)
	c2 = TraceID(id)
	c3 = eid.ID(s)
	c4 = SpanID(id)
	c5 = [8]byte(s)
	c6 = SpanID(a8)
)
PROBE
  } >"$i2_dir/probe/probe.go"
  i2_nd=$(grep -cE '^type (TraceID|SpanID) ' "$i2_dir/probe/probe.go")
  if [ "$i2_nd" != 2 ]; then i2_obs="found $i2_nd declaration lines of TraceID and SpanID in trace.go, two expected"; return 2; fi
  i2_plog=/tmp/rows/L06.probe.log
  (cd "$i2_dir" && go build ./probe/ >"$i2_plog" 2>&1); i2_prc=$?
  if offline "$i2_plog"; then i2_obs="module not found"; return 2; fi
  i2_nconv=$(grep -c 'cannot convert' "$i2_plog")
  i2_nerr=$(grep -v '^#' "$i2_plog" | grep -c .)
  i2_obs="probe build rc=$i2_prc, $i2_nconv cannot-convert lines of $i2_nerr error lines"
  [ $i2_prc -ne 0 ] && [ "$i2_nconv" = 6 ] && [ "$i2_nerr" = 6 ]
}

if sel L06 i2; then
  i2_what="the probe fails with six cannot-convert errors; eid.ParseString refuses 32 and 16 hex characters; ParseTraceID and ParseSpanID refuse an eid text; the zero eid refuses MarshalText and Value"
  if [ $HAVE_GO -eq 0 ]; then say L06 "$i2_what" "go not on PATH" CNM
  else
    i2_pd=$(mktemp -d /tmp/rows/plant.XXXXXX)
    cp -a /w/go "$i2_pd/go"
    i2_obs=""
    i2_probe "$i2_pd/go"; i2_prc=$?
    i2_pobs=$i2_obs
    rm -rf "$i2_pd"
    i2_tlog=/tmp/rows/L06.log
    (cd /w/go && go test -count=1 -v -run '^(TestTextFormsRefuseEachOther|TestZeroIDEncodingRefused)$' ./ecd/ >"$i2_tlog" 2>&1); i2_trc=$?
    if [ $i2_prc -eq 2 ] || offline "$i2_tlog"; then say L06 "$i2_what" "measured nothing: $i2_pobs" CNM
    elif [ $i2_prc -ne 0 ]; then say L06 "$i2_what" "$i2_pobs" FAIL; head_of /tmp/rows/L06.probe.log 12
    elif [ $i2_trc -ne 0 ] || ! grep -q '^--- PASS: TestTextFormsRefuseEachOther' "$i2_tlog" || ! grep -q '^--- PASS: TestZeroIDEncodingRefused' "$i2_tlog"; then say L06 "$i2_what" "$i2_pobs; the text refusals or the zero eid refusals did not pass (rc=$i2_trc)" FAIL; head_of "$i2_tlog" 12
    else say L06 "$i2_what" "$i2_pobs; the text refusals and the zero eid refusals PASS" PASS; fi
  fi
fi

if sel L06p i2; then
  i2_what="TraceID declared as [16]byte turns the L06 probe red"
  if [ $HAVE_GO -eq 0 ]; then say L06p "$i2_what" "go not on PATH" CNM
  else
    i2_pd=$(mktemp -d /tmp/rows/plant.XXXXXX)
    cp -a /w/go "$i2_pd/go" && cp "$i2_pd/go/ecd/trace.go" "$i2_pd/orig" &&
      sed -i 's/^type TraceID struct{ b \[16\]byte }$/type TraceID [16]byte/' "$i2_pd/go/ecd/trace.go"
    i2_del=$(diff "$i2_pd/orig" "$i2_pd/go/ecd/trace.go" | grep -c '^<')
    i2_add=$(diff "$i2_pd/orig" "$i2_pd/go/ecd/trace.go" | grep -c '^>')
    if [ "$i2_del" != 1 ] || [ "$i2_add" != 1 ]; then
      say L06p "$i2_what" "the sed removed $i2_del and added $i2_add lines of trace.go (one each expected)" CNM
    else
      i2_obs=""
      i2_probe "$i2_pd/go"; i2_prc=$?
      case $i2_prc in
        0) say L06p "$i2_what" "$i2_obs: the probe stayed green" SURVIVED ;;
        1) say L06p "$i2_what" "$i2_obs" KILLED ;;
        *) say L06p "$i2_what" "measured nothing: $i2_obs" CNM ;;
      esac
    fi
    rm -rf "$i2_pd"
  fi
fi
