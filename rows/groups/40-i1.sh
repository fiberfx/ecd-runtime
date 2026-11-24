# Group i1 of Ecd Runtime: I1-01 to I1-08 and their plants (the rows ending in p).
# The vectors of ecd-id v0.8.0 (go/ecd/testdata/eid, pinned by SHA256SUMS) run through the encodings of the eid the
# module requires: MarshalText, UnmarshalText, Value and Scan. The run's inputs folder is
# /home/jonny/.local/share/ech/mx-box/loom-inputs-080, which holds ecd-id v0.8.0 only.
# Sourced by the rows harness from /w; the harness supplies say, sel, offline, head_of, count, tree_of, go_row and HAVE_GO.
#
#   row    check
#   I1-01  every valid.tsv id: UnmarshalText gives the bytes of its hex, MarshalText its text, Value its 16 bytes, Scan of them the id; the fields agree
#   I1-02  every invalid.tsv text is refused by UnmarshalText with its reason and leaves the receiver unchanged
#   I1-03  every binary_invalid.tsv input is refused by Scan with its reason and leaves the receiver unchanged
#   I1-04  the valid ids sorted by text, by bytes and by Compare give one order
#   I1-05  stream.tsv: StreamID() text and FromStream(kind, sid) back; stream_invalid.tsv refused with its reasons
#   I1-06  SHA256SUMS lists exactly the .tsv files of testdata/eid with their digests
#   I1-07  KindRange and Window contain exactly the vector ids of their kind and window
#   I1-08  MarshalText and Value of the zero eid refuse with eid.ErrZero; an id answers its text
#
# Each plant is a one-line sed (I1-06p: one appended byte) on a temporary copy of /w/go that re-runs only the tests of
# its row. A plant is KILLED when a test fails and SURVIVED when every test passes; a plant that does not change
# exactly one line, or in which no test ran, is CNM.
#   I1-01p  the check of the bytes of an id compares against strings.ToUpper of the hex, in eid_vectors_test.go
#   I1-02p  the line that compares the reason in TestVectorsInvalidText becomes "_ = err"
#   I1-03p  the line that compares the reason in TestVectorsBinaryInvalid becomes "_ = err"
#   I1-04p  the bytes order of TestVectorsOrder compares Time() alone
#   I1-05p  FromStream takes the kind "job" for every row of stream.tsv
#   I1-06p  one byte is appended to testdata/eid/valid.tsv
#   I1-07p  Contains ignores Hi, in kind.go
#   I1-08p  "zero := eid.ID{}" becomes "zero := first", in eid_vectors_test.go

# i1_plant row file sed-expression test-pattern expectation: the plant on go/ecd/<file>.
i1_plant() {
  i1_id=$1 i1_file=$2 i1_expr=$3 i1_pat=$4 i1_what=$5
  sel "$i1_id" i1 || return 0
  if [ $HAVE_GO -eq 0 ]; then say "$i1_id" "$i1_what" "go not on PATH" CNM; return 0; fi
  i1_pd=$(mktemp -d /tmp/rows/plant.XXXXXX)
  cp -a /w/go "$i1_pd/go" && cp "$i1_pd/go/ecd/$i1_file" "$i1_pd/orig" && sed -i "$i1_expr" "$i1_pd/go/ecd/$i1_file"
  i1_del=$(diff "$i1_pd/orig" "$i1_pd/go/ecd/$i1_file" | grep -c '^<')
  i1_add=$(diff "$i1_pd/orig" "$i1_pd/go/ecd/$i1_file" | grep -c '^>')
  if [ "$i1_del" != 1 ] || [ "$i1_add" != 1 ]; then
    say "$i1_id" "$i1_what" "the sed removed $i1_del and added $i1_add lines of $i1_file (one each expected)" CNM
  else
    i1_run "$i1_id" "$i1_pd/go" "$i1_pat" "$i1_what"
  fi
  rm -rf "$i1_pd"
}

# i1_run row dir test-pattern expectation: runs the tests of the pattern in the module copy at dir and says the verdict.
i1_run() {
  i1_log=/tmp/rows/$1.log
  (cd "$2" && go test -count=1 -v -run "$3" ./ecd/ >"$i1_log" 2>&1); i1_rc=$?
  if offline "$i1_log"; then say "$1" "$4" "module not found" CNM
  elif grep -q '^--- FAIL' "$i1_log"; then say "$1" "$4" "rc=$i1_rc, a test FAIL on the plant" KILLED
  elif [ $i1_rc -eq 0 ] && grep -q '^--- PASS' "$i1_log"; then say "$1" "$4" "rc=0, every test PASS on the plant" SURVIVED
  else say "$1" "$4" "rc=$i1_rc, the tests did not run" CNM; head_of "$i1_log" 8; fi
}

go_row I1-01 i1 "every valid id both ways through the encodings, the fields agree" /w/go test -run '^TestVectorsValid$' ./ecd/
i1_plant I1-01p eid_vectors_test.go 's/^\t\tif hex.EncodeToString(id\[:\]) != r\[1\] {$/\t\tif hex.EncodeToString(id[:]) != strings.ToUpper(r[1]) {/' '^TestVectorsValid$' "the bytes are compared against the upper-case hex and the I1-01 test fails"

go_row I1-02 i1 "every invalid text refused by UnmarshalText with its reason, the receiver unchanged" /w/go test -run '^TestVectorsInvalidText$' ./ecd/
i1_plant I1-02p eid_vectors_test.go '/^func TestVectorsInvalidText/,/^}/s/^\t\tcompared += vecRefused(t, r\[0\], err, r\[2\])$/\t\t_ = err/' '^TestVectorsInvalidText$' "the reason comparison is skipped and the I1-02 test fails"

go_row I1-03 i1 "every invalid 16-byte input refused by Scan with its reason, the receiver unchanged" /w/go test -run '^TestVectorsBinaryInvalid$' ./ecd/
i1_plant I1-03p eid_vectors_test.go '/^func TestVectorsBinaryInvalid/,/^}/s/^\t\tcompared += vecRefused(t, r\[0\], err, r\[2\])$/\t\t_ = err/' '^TestVectorsBinaryInvalid$' "the reason comparison is skipped and the I1-03 test fails"

go_row I1-04 i1 "the valid ids sorted by text, by bytes and by Compare have one order" /w/go test -run '^TestVectorsOrder$' ./ecd/
i1_plant I1-04p eid_vectors_test.go 's/^\t\treturn bytes.Compare(a\[:\], b\[:\])$/\t\treturn cmp.Compare(a.Time(), b.Time())/' '^TestVectorsOrder$' "the bytes order compares Time() alone and the I1-04 test fails"

go_row I1-05 i1 "stream ids and their inverse from the kind; the invalid stream ids refused" /w/go test -run '^TestVectorsStream' ./ecd/
i1_plant I1-05p eid_vectors_test.go 's/^\t\tback, err := eid.FromStream(r\[1\], sid)$/\t\tback, err := eid.FromStream("job", sid)/' '^TestVectorsStream' "FromStream takes the kind job for every row and the I1-05 test fails"

go_row I1-06 i1 "SHA256SUMS matches testdata/eid/*.tsv" /w/go test -run '^TestVectorsSums$' ./ecd/

if sel I1-06p i1; then
  i1_what="one byte appended to valid.tsv and the I1-06 test fails"
  if [ $HAVE_GO -eq 0 ]; then say I1-06p "$i1_what" "go not on PATH" CNM
  else
    i1_pd=$(mktemp -d /tmp/rows/plant.XXXXXX)
    cp -a /w/go "$i1_pd/go" && i1_n0=$(wc -c <"$i1_pd/go/ecd/testdata/eid/valid.tsv") && printf '\n' >>"$i1_pd/go/ecd/testdata/eid/valid.tsv"
    i1_n1=$(wc -c <"$i1_pd/go/ecd/testdata/eid/valid.tsv")
    if [ $((i1_n1 - i1_n0)) -ne 1 ]; then say I1-06p "$i1_what" "valid.tsv grew by $((i1_n1 - i1_n0)) bytes (one expected)" CNM
    else i1_run I1-06p "$i1_pd/go" '^TestVectorsSums$' "$i1_what"; fi
    rm -rf "$i1_pd"
  fi
fi

go_row I1-07 i1 "KindRange and Window contain exactly the vector ids of their kind and window" /w/go test -run '^TestVectorsRanges$' ./ecd/
i1_plant I1-07p kind.go 's/ \&\& bytes.Compare(id\[:\], r.Hi\[:\]) < 0//' '^TestVectorsRanges$' "Contains ignores Hi and the I1-07 test fails"

go_row I1-08 i1 "MarshalText and Value of the zero eid refuse with eid.ErrZero; an id answers its text" /w/go test -run '^TestZeroIDEncodingRefused$' ./ecd/
i1_plant I1-08p eid_vectors_test.go 's/^\tzero := eid.ID{}$/\tzero := first/' '^TestZeroIDEncodingRefused$' "the zero eid is replaced by a valid id and the I1-08 test fails"
