# Group rtp30 of Ecd Runtime: package rt (go/rt) with the store as the record of the flush: the transactions that the
# store committed, replayed in seq order into a new engine of the same catalog, give the state of the engine that the
# flushes applied.
# Sourced by the rows harness from /w; the harness supplies say, sel, offline, head_of, count, why_of, go_row and
# HAVE_GO.
#
#   RT-33    over a seeded history of flushes with conflicts, destroys and spawns with a tag, the deeds of the store's
#            committed transactions, applied in seq order to a new engine, give every query answer and every Get of
#            every minted eid that the live engine gives, on the reference model and on Jacquard (TestL31Replay)
#   RT-33p   plant: a flush applies every committed deed but the last (flush.go; TestL31Replay)
#
# The rows hold only with the files of package rt, go/rt.
#
# rtp30_plant id expect test file sedexpr: copies /w/go to a temporary directory, applies the one-line sed to
# rt/<file> and runs only the named test there. KILLED: the test failed, and the observed text names its first
# failing line. SURVIVED: it passed. CNM: the sed did not change exactly one line, the module was missing, or the test
# did not run.
rtp30_plant() {
  rtp30_id=$1 rtp30_ex=$2 rtp30_test=$3 rtp30_file=$4 rtp30_sed=$5
  sel "$rtp30_id" rtp30 || return 0
  if [ $HAVE_GO -eq 0 ]; then say "$rtp30_id" "$rtp30_ex" "go not on PATH" CNM; return 0; fi
  rtp30_pd=$(mktemp -d /tmp/rows/plant.XXXXXX)
  cp -a /w/go "$rtp30_pd/go" && cp "$rtp30_pd/go/rt/$rtp30_file" "$rtp30_pd/orig.go"
  (cd "$rtp30_pd/go/rt" && sed -i "$rtp30_sed" "$rtp30_file")
  rtp30_n=$(diff "$rtp30_pd/orig.go" "$rtp30_pd/go/rt/$rtp30_file" | grep -c '^>')
  if [ "$rtp30_n" -ne 1 ]; then
    say "$rtp30_id" "$rtp30_ex" "the sed changed $rtp30_n lines of $rtp30_file, want 1" CNM
  else
    rtp30_log=/tmp/rows/$rtp30_id.log
    (cd "$rtp30_pd/go" && go test -count=1 -v -run "^$rtp30_test\$" ./rt/ >"$rtp30_log" 2>&1); rtp30_rc=$?
    if offline "$rtp30_log"; then say "$rtp30_id" "$rtp30_ex" "module not found" CNM
    elif grep -q "^--- FAIL: $rtp30_test" "$rtp30_log"; then
      say "$rtp30_id" "$rtp30_ex" "rc=$rtp30_rc, $rtp30_test FAIL: $(why_of "$rtp30_log")" KILLED
    elif [ $rtp30_rc -eq 0 ] && grep -q "^--- PASS: $rtp30_test" "$rtp30_log"; then
      say "$rtp30_id" "$rtp30_ex" "rc=0, $rtp30_test PASS" SURVIVED
    else say "$rtp30_id" "$rtp30_ex" "rc=$rtp30_rc, the test did not run" CNM; head_of "$rtp30_log" 8; fi
  fi
  rm -rf "$rtp30_pd"
}

go_row RT-33 rtp30 "TestL31Replay rc=0" /w/go test -run '^TestL31Replay$' ./rt/
rtp30_plant RT-33p "TestL31Replay fails on the plant" TestL31Replay flush.go \
  's/err = r\.engine\.Apply(seq, committed)/err = r.engine.Apply(seq, committed[:len(committed)-1])/'
