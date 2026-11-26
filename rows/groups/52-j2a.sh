# Group j2a of Entity Component Runtime: masks, chunk geometry and vocabulary of package jacquard.
# Sourced by the rows harness from /w; the harness supplies say, sel, offline, head_of, count, why_of, go_row and
# HAVE_GO.
#
#   J13      chunkShift gives the largest power-of-two row count, 64 to 4096, that fits 16 KiB
#   J13p     plant: chunkShift returns 6 for every row width; TestChunkShift must fail
#   J2A-01   mask operations equal a []bool reference on random masks; a mask lacking a bit never contains it
#   J2A-01p  plant: contains combines words with | instead of &; TestMaskOps must fail
#   J2A-02   maskBits is 256, and 512 under -tags jacquard512 (two invocations of TestMaskBits)
#   J2A-02p  plant: maskWords = 4 in mask_w8.go, run under -tags jacquard512; TestMaskBits must fail
#   J2A-03   no Go file under engine/jacquard and ecd has a banned word or the package clause of the copied package,
#            and a file that ports an Ark file carries the attribution line (TestVocabulary)
#   J2A-03p  plant: every package clause in engine/jacquard becomes "package ecs"; TestVocabulary must fail
#
# j2a_plant id expect test files sedexpr [flags]: copies /w/go to a temporary directory, applies sedexpr to the
# files (a glob in engine/jacquard) and runs only the named test there. KILLED: the test failed. SURVIVED: it passed.
# CNM: the sed changed nothing, the module was missing, or the test did not run.
j2a_plant() {
  pid=$1 pex=$2 ptest=$3 pfiles=$4 psed=$5 pflags=${6:-}
  sel "$pid" j2a || return 0
  if [ $HAVE_GO -eq 0 ]; then say "$pid" "$pex" "go not on PATH" CNM; return 0; fi
  pd=$(mktemp -d /tmp/rows/plant.XXXXXX)
  cp -a /w/go "$pd/go" && cp -a "$pd/go/engine/jacquard" "$pd/orig"
  (cd "$pd/go/engine/jacquard" && sed -i "$psed" $pfiles)
  if diff -rq "$pd/orig" "$pd/go/engine/jacquard" >/dev/null 2>&1; then
    say "$pid" "$pex" "the sed changed nothing" CNM
  else
    plog=/tmp/rows/$pid.log
    (cd "$pd/go" && go test -count=1 -v $pflags -run "^$ptest\$" ./engine/jacquard/ >"$plog" 2>&1); prc=$?
    if offline "$plog"; then say "$pid" "$pex" "module not found" CNM
    elif grep -q "^--- FAIL: $ptest" "$plog"; then
      say "$pid" "$pex" "rc=$prc, $ptest FAIL: $(why_of "$plog")" KILLED
    elif [ $prc -eq 0 ] && grep -q "^--- PASS: $ptest" "$plog"; then say "$pid" "$pex" "rc=0, $ptest PASS" SURVIVED
    else say "$pid" "$pex" "rc=$prc, the test did not run" CNM; head_of "$plog" 8; fi
  fi
  rm -rf "$pd"
}

go_row J13 j2a "TestChunkShift rc=0" /w/go test -run '^TestChunkShift$' ./engine/jacquard/
j2a_plant J13p "TestChunkShift fails on the plant" TestChunkShift geometry.go 's/return uint8(bits\.Len(uint(n)) - 1)/return 6/'

go_row J2A-01 j2a "TestMaskOps rc=0" /w/go test -run '^TestMaskOps$' ./engine/jacquard/
j2a_plant J2A-01p "TestMaskOps fails on the plant" TestMaskOps mask.go 's/if m\[i\]&o\[i\] != o\[i\] {/if m[i]|o[i] != o[i] {/'

if sel J2A-02 j2a; then
  x2="TestMaskBits rc=0 without the tag (256) and with -tags jacquard512 (512)"
  if [ $HAVE_GO -eq 0 ]; then say J2A-02 "$x2" "go not on PATH" CNM
  else
    l1=/tmp/rows/J2A-02.log l2=/tmp/rows/J2A-02-512.log
    (cd /w/go && go test -count=1 -v -run '^TestMaskBits$' ./engine/jacquard/ >"$l1" 2>&1); r1=$?
    (cd /w/go && go test -count=1 -v -tags jacquard512 -run '^TestMaskBits$' ./engine/jacquard/ >"$l2" 2>&1); r2=$?
    p1=$(count '^--- PASS: TestMaskBits' "$l1"); p2=$(count '^--- PASS: TestMaskBits' "$l2")
    if offline "$l1" || offline "$l2"; then say J2A-02 "$x2" "module not found" CNM
    elif [ $r1 -ne 0 ] || [ $r2 -ne 0 ]; then say J2A-02 "$x2" "rc=$r1 without the tag, rc=$r2 with it" FAIL; head_of "$l1" 12; head_of "$l2" 12
    elif [ "$p1" -ne 1 ] || [ "$p2" -ne 1 ]; then say J2A-02 "$x2" "rc=0 but TestMaskBits passed $p1 and $p2 times" CNM
    else say J2A-02 "$x2" "rc=0 and rc=0, TestMaskBits PASS twice" PASS; fi
  fi
fi
j2a_plant J2A-02p "TestMaskBits fails on the plant under -tags jacquard512" TestMaskBits mask_w8.go 's/^const maskWords = 8$/const maskWords = 4/' "-tags jacquard512"

go_row J2A-03 j2a "TestVocabulary rc=0" /w/go test -run '^TestVocabulary$' ./engine/jacquard/
j2a_plant J2A-03p "TestVocabulary fails on the plant" TestVocabulary '*.go' 's/^package jacquard$/package ecs/'
