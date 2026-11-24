# Group i2b of Ecd Runtime: I2B-01 to I2B-04 and their plants (the rows ending in p).
# Package ecd (go/ecd): the seam's hardening. Apply is all or nothing, in a check pass and a commit pass; QueryDef holds
# an Any term; ErrOp is the tenth sentinel; the host's one Minter is the only minter; the catalog names are ecd_*.
# Sourced by the rows harness from /w; the harness supplies say, sel, offline, head_of, count, why_of, go_row and
# HAVE_GO.
#
#   row     check
#   I2B-01  QueryDef has a field Any of type []ecd.CID, between None and Exact (TestSeamQueryAny)
#   I2B-02  *eid.Minter satisfies ecd.Minter (the package compiles with the assertion) and no non-test Go file under
#           go/ecd and go/engine builds or clones a Minter: eid.NewMinter and .Clone() occur nowhere (TestNoSecondMinter)
#   I2B-03  the seam lists ten error texts, ErrOp matches none of the nine others, and the comment of Apply in engine.go
#           holds "all or nothing", "check pass" and "makes the engine Stale" (TestSeamErrorTexts, TestSeamErrOpDistinct,
#           TestSeamSentinelsDistinct, TestSeamApplyText); every one of the four tests must pass
#   I2B-04  no non-test Go file under go/ecd and go/engine contains the retired product name, in any case (TestNoRetiredName)
#
# Each plant is one sed expression on engine.go of a temporary copy of /w/go that re-runs only the test of its row.
# KILLED: a test failed. SURVIVED: every test passed. CNM: the sed did not change the expected number of lines, the
# module was missing, or no test ran.
#   I2B-01p  the field "Any []CID" is renamed "AnyOf []CID"
#   I2B-02p  the line "var _ = eid.NewMinter" is appended
#   I2B-03p  the Apply comment's "all or nothing, in two passes" is dropped
#   I2B-04p  "ecd_component.cid" in the CID comment becomes the retired name with the same suffix

# i2b_good id expectation tests...: runs the named tests, which are all required to pass.
i2b_good() {
  i2b_id=$1 i2b_what=$2 i2b_names=$3
  sel "$i2b_id" i2b || return 0
  if [ $HAVE_GO -eq 0 ]; then say "$i2b_id" "$i2b_what" "go not on PATH" CNM; return 0; fi
  i2b_log=/tmp/rows/$i2b_id.log
  (cd /w/go && go test -count=1 -v -timeout 900s -run "^($i2b_names)\$" ./ecd/ >"$i2b_log" 2>&1); i2b_rc=$?
  i2b_miss=""
  for i2b_t in $(printf '%s' "$i2b_names" | tr '|' ' '); do
    grep -q "^--- PASS: $i2b_t " "$i2b_log" || i2b_miss="$i2b_miss $i2b_t"
  done
  if offline "$i2b_log"; then say "$i2b_id" "$i2b_what" "module not found" CNM; head_of "$i2b_log" 8
  elif grep -q '^--- FAIL' "$i2b_log" || [ $i2b_rc -ne 0 ]; then say "$i2b_id" "$i2b_what" "rc=$i2b_rc, a test failed or the package did not build" FAIL; head_of "$i2b_log" 20
  elif [ -n "$i2b_miss" ]; then say "$i2b_id" "$i2b_what" "rc=0 but no PASS line for:$i2b_miss" CNM
  else say "$i2b_id" "$i2b_what" "rc=0, every named test PASS" PASS; fi
}

# i2b_plant id expectation test-names sed-expression removed added: the plant on go/ecd/engine.go.
i2b_plant() {
  i2b_id=$1 i2b_what=$2 i2b_names=$3 i2b_expr=$4 i2b_nd=$5 i2b_na=$6
  sel "$i2b_id" i2b || return 0
  if [ $HAVE_GO -eq 0 ]; then say "$i2b_id" "$i2b_what" "go not on PATH" CNM; return 0; fi
  i2b_pd=$(mktemp -d /tmp/rows/plant.XXXXXX)
  cp -a /w/go "$i2b_pd/go" && cp "$i2b_pd/go/ecd/engine.go" "$i2b_pd/orig" && sed -i "$i2b_expr" "$i2b_pd/go/ecd/engine.go"
  i2b_del=$(diff "$i2b_pd/orig" "$i2b_pd/go/ecd/engine.go" | grep -c '^<')
  i2b_add=$(diff "$i2b_pd/orig" "$i2b_pd/go/ecd/engine.go" | grep -c '^>')
  if [ "$i2b_del" != "$i2b_nd" ] || [ "$i2b_add" != "$i2b_na" ]; then
    say "$i2b_id" "$i2b_what" "the sed removed $i2b_del and added $i2b_add lines of engine.go ($i2b_nd and $i2b_na expected)" CNM
  else
    i2b_log=/tmp/rows/$i2b_id.log
    (cd "$i2b_pd/go" && go test -count=1 -v -run "^($i2b_names)\$" ./ecd/ >"$i2b_log" 2>&1); i2b_rc=$?
    if offline "$i2b_log"; then say "$i2b_id" "$i2b_what" "module not found" CNM
    elif grep -q '^--- FAIL' "$i2b_log"; then
      say "$i2b_id" "$i2b_what" "rc=$i2b_rc, a test FAIL on the plant: $(why_of "$i2b_log")" KILLED
    elif [ $i2b_rc -eq 0 ] && grep -q '^--- PASS' "$i2b_log"; then say "$i2b_id" "$i2b_what" "rc=0, every test PASS on the plant" SURVIVED
    else say "$i2b_id" "$i2b_what" "rc=$i2b_rc, the tests did not run" CNM; head_of "$i2b_log" 8; fi
  fi
  rm -rf "$i2b_pd"
}

i2b_good I2B-01 "QueryDef.Any is a []ecd.CID between None and Exact" 'TestSeamQueryAny'
i2b_plant I2B-01p "the field is renamed AnyOf and TestSeamQueryAny fails" 'TestSeamQueryAny' 's/^\tAny\( *\)\[\]CID/\tAnyOf []CID/' 1 1

i2b_good I2B-02 "*eid.Minter satisfies ecd.Minter; no non-test file under go/ecd and go/engine builds or clones a Minter" 'TestNoSecondMinter'
i2b_plant I2B-02p "a file that builds a Minter is appended and TestNoSecondMinter fails" 'TestNoSecondMinter' '$a var _ = eid.NewMinter' 0 1

i2b_good I2B-03 "ten error texts, ErrOp distinct from the nine others, and the Apply comment states the two passes and the stale rule" 'TestSeamErrorTexts|TestSeamErrOpDistinct|TestSeamSentinelsDistinct|TestSeamApplyText'
i2b_plant I2B-03p "the Apply comment drops all or nothing and TestSeamApplyText fails" 'TestSeamApplyText' 's/Apply applies one committed transaction, all or nothing, in two passes\./Apply applies one committed transaction./' 1 1

i2b_good I2B-04 "no non-test Go file under go/ecd and go/engine contains the retired product name" 'TestNoRetiredName'
i2b_plant I2B-04p "a catalog name returns to the retired prefix and TestNoRetiredName fails" 'TestNoRetiredName' 's/ecd_component\.cid/loom_component.cid/' 1 1
