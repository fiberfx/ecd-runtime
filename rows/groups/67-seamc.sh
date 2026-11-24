# Group seamc of Ecd Runtime: the decided text of the comments in the seam, go/ecd/engine.go.
# Sourced by the rows harness from /w; the harness supplies say, sel, offline, head_of, count, go_row and HAVE_GO.
#
# TestSeamDecidedText reads engine.go and requires the words of the KindDef Budget comment, of the Op comment on
# committed deeds, of the Hydrate comment, of the Query comment and of the ErrOp comment. The plants of this group
# edit one line of engine.go each and run that test.
#
#   SC-01    TestSeamDecidedText passes on engine.go
#   SC-01p   plant: the Budget comment ends "and 0 means no residency", not "and 0 keeps none resident" (engine.go)
#   SC-01q   plant: the Op comment drops the reference "(E1)" (engine.go)
#   SC-01r   plant: the Hydrate comment does not state that a call is all or nothing (engine.go)
#   SC-01s   plant: the ErrOp comment does not name a Hydrate row (engine.go)
#   SC-01t   plant: the Query comment drops the rule for a non-empty Any (engine.go)
#   SC-01u   plant: the Budget comment drops the empty-kind case (engine.go)
#
# seamc_plant id expect sedexpr: copies /w/go to a temporary directory, applies the one-line sed to ecd/engine.go and
# runs only TestSeamDecidedText there. KILLED: the test failed. SURVIVED: it passed.
# CNM: the sed did not change exactly one line, the module was missing, or the test did not run.
seamc_plant() {
  seamc_id=$1 seamc_ex=$2 seamc_sed=$3 seamc_test=TestSeamDecidedText
  sel "$seamc_id" seamc || return 0
  if [ $HAVE_GO -eq 0 ]; then say "$seamc_id" "$seamc_ex" "go not on PATH" CNM; return 0; fi
  seamc_pd=$(mktemp -d /tmp/rows/plant.XXXXXX)
  cp -a /w/go "$seamc_pd/go" && cp "$seamc_pd/go/ecd/engine.go" "$seamc_pd/orig.go"
  (cd "$seamc_pd/go/ecd" && sed -i "$seamc_sed" engine.go)
  seamc_n=$(diff "$seamc_pd/orig.go" "$seamc_pd/go/ecd/engine.go" | grep -c '^>')
  if [ "$seamc_n" -ne 1 ]; then
    say "$seamc_id" "$seamc_ex" "the sed changed $seamc_n lines of engine.go, want 1" CNM
  else
    seamc_log=/tmp/rows/$seamc_id.log
    (cd "$seamc_pd/go" && go test -count=1 -v -run "^$seamc_test\$" ./ecd/ >"$seamc_log" 2>&1); seamc_rc=$?
    if offline "$seamc_log"; then say "$seamc_id" "$seamc_ex" "module not found" CNM
    elif grep -q "^--- FAIL: $seamc_test" "$seamc_log"; then
      seamc_why=$(grep -m1 -E '_test\.go:[0-9]+:|^panic:' "$seamc_log" | cut -c1-100)
      say "$seamc_id" "$seamc_ex" "rc=$seamc_rc, $seamc_test FAIL: $seamc_why" KILLED
    elif [ $seamc_rc -eq 0 ] && grep -q "^--- PASS: $seamc_test" "$seamc_log"; then
      say "$seamc_id" "$seamc_ex" "rc=0, $seamc_test PASS" SURVIVED
    else say "$seamc_id" "$seamc_ex" "rc=$seamc_rc, the test did not run" CNM; head_of "$seamc_log" 8; fi
  fi
  rm -rf "$seamc_pd"
}

go_row SC-01 seamc "TestSeamDecidedText rc=0" /w/go test -run '^TestSeamDecidedText$' ./ecd/
seamc_plant SC-01p "TestSeamDecidedText fails on the plant" 's/, and 0 keeps none resident$/, and 0 means no residency/'
seamc_plant SC-01q "TestSeamDecidedText fails on the plant" 's/ (E1)\.$/./'
seamc_plant SC-01r "TestSeamDecidedText fails on the plant" 's/Hydrate is all or nothing per call:/Hydrate loads rows:/'
seamc_plant SC-01s "TestSeamDecidedText fails on the plant" \
  's/, or a Hydrate row: an undeclared CID/: an undeclared CID/'
seamc_plant SC-01t "TestSeamDecidedText fails on the plant" 's/and, when Any is not empty, one$/and one/'
seamc_plant SC-01u "TestSeamDecidedText fails on the plant" 's/within it, or holds no$/within it, and holds no/'
