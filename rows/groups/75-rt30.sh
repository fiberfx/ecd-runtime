# Group rt30 of Entity Component Runtime: package rt (go/rt), which records the writes of systems in buffers and commits
# them as one flush: a Runtime that needs a Minter, the order and the deeds of a flush, the refusal of a record before
# anything is minted, committed or applied, the pre-commit check of the deeds, and the direction of the dependency
# between the engine and the runtime.
# Sourced by the rows harness from /w; the harness supplies say, sel, offline, head_of, count, why_of, go_row and
# HAVE_GO.
#
#   RT-30    a Runtime without a Minter is refused, a flush mints one deed eid per buffer with a record, and the
#            environment of a system holds only its own buffer (TestL30*)
#   RT-30p   plant: New accepts a Config without a Minter (rt.go; TestL30NoMinter)
#   RT-30q   plant: a flush takes no deed eid from the Minter (flush.go; TestL30Count)
#   RT-31    a flush runs the systems in stage order, then registration order, and returns one deed per buffer with a
#            record; a deed answered conflict or refused is left out of Apply; a Ref resolves in the buffer that issued
#            it; a buffer copies the value it records (TestL31*, TestBufferCopy)
#   RT-31p   plant: a flush visits the buffers in map order (flush.go; TestL31Order)
#   RT-31q   plant: a conflicted deed is applied (flush.go; TestL31Conflict)
#   RT-31r   plant: a Ref is not resolved against its buffer (flush.go; TestL31Refs)
#   RT-31s   plant: a buffer keeps the caller's slice for a recorded value (buffer.go; TestBufferCopy)
#   RT-31c   a record that names another buffer's placeholder is refused before any mint, commit or apply
#            (TestL31Cascade)
#   RT-31cp  plant: the resolve pass does not compare the buffer of a Ref (flush.go; TestL31Cascade)
#   RT-32    the pre-commit check refuses a deed that the engine would refuse, a refused flush drops the records of
#            every buffer and its error names the stage and the record count of each stage (TestCheck, TestFlush*)
#   RT-32p   plant: the check accepts a record on a destroyed entity (check.go; TestCheck)
#   RT-32q   plant: the check accepts a value of the wrong size (check.go; TestCheck)
#   RT-32r   plant: the error of a refused flush counts buffers, not records (flush.go; TestFlushDropped)
#   RT-37    go list -deps ./engine/... names no package of go/rt: the engine does not depend on the runtime
#   RT-37p   plant: package jacquard imports go/rt (engine/jacquard/doc.go; the measure of RT-37)
#
# The rows RT-30 to RT-32 hold only with the files of package rt, go/rt.
#
# rt30_plant id expect test file sedexpr: copies /w/go to a temporary directory, applies the one-line sed to rt/<file>
# and runs only the named test there. KILLED: the test failed, and the observed text names its first failing line.
# SURVIVED: it passed. CNM: the sed did not change exactly one line, the module was missing, or the test did not run.
rt30_plant() {
  rt30_id=$1 rt30_ex=$2 rt30_test=$3 rt30_file=$4 rt30_sed=$5
  sel "$rt30_id" rt30 || return 0
  if [ $HAVE_GO -eq 0 ]; then say "$rt30_id" "$rt30_ex" "go not on PATH" CNM; return 0; fi
  rt30_pd=$(mktemp -d /tmp/rows/plant.XXXXXX)
  cp -a /w/go "$rt30_pd/go" && cp "$rt30_pd/go/rt/$rt30_file" "$rt30_pd/orig.go"
  (cd "$rt30_pd/go/rt" && sed -i "$rt30_sed" "$rt30_file")
  rt30_n=$(diff "$rt30_pd/orig.go" "$rt30_pd/go/rt/$rt30_file" | grep -c '^>')
  if [ "$rt30_n" -ne 1 ]; then
    say "$rt30_id" "$rt30_ex" "the sed changed $rt30_n lines of $rt30_file, want 1" CNM
  else
    rt30_log=/tmp/rows/$rt30_id.log
    (cd "$rt30_pd/go" && go test -count=1 -v -run "^$rt30_test\$" ./rt/ >"$rt30_log" 2>&1); rt30_rc=$?
    if offline "$rt30_log"; then say "$rt30_id" "$rt30_ex" "module not found" CNM
    elif grep -q "^--- FAIL: $rt30_test" "$rt30_log"; then
      say "$rt30_id" "$rt30_ex" "rc=$rt30_rc, $rt30_test FAIL: $(why_of "$rt30_log")" KILLED
    elif [ $rt30_rc -eq 0 ] && grep -q "^--- PASS: $rt30_test" "$rt30_log"; then
      say "$rt30_id" "$rt30_ex" "rc=0, $rt30_test PASS" SURVIVED
    else say "$rt30_id" "$rt30_ex" "rc=$rt30_rc, the test did not run" CNM; head_of "$rt30_log" 8; fi
  fi
  rm -rf "$rt30_pd"
}

# rt30_deps dir log: lists the packages that ./engine/... depends on in dir. Sets rt30_state to listed when the list
# names go/rt, to clear when it has at least one line and none names go/rt, and to cnm when go list failed, listed
# nothing or found no module; rt30_obs holds the observed text.
rt30_deps() {
  rt30_dd=$1 rt30_dl=$2
  (cd "$rt30_dd" && go list -deps ./engine/... >"$rt30_dl" 2>"$rt30_dl.err"); rt30_drc=$?
  rt30_dn=$(wc -l <"$rt30_dl")
  if [ $rt30_drc -ne 0 ] || [ "$rt30_dn" -eq 0 ] || offline "$rt30_dl.err"; then
    rt30_state=cnm rt30_obs="go list rc=$rt30_drc, $rt30_dn packages"
  elif grep -qx 'github.com/fiberfx/ecd-runtime/go/rt' "$rt30_dl"; then
    rt30_state=listed rt30_obs="go/rt is among $rt30_dn packages"
  else
    rt30_state=clear rt30_obs="go/rt is not among $rt30_dn packages"
  fi
}

go_row RT-30 rt30 "TestL30 rc=0" /w/go test -run '^TestL30' ./rt/
rt30_plant RT-30p "TestL30NoMinter fails on the plant" TestL30NoMinter rt.go 's/if cfg\.Minter == nil {/if false {/'
rt30_plant RT-30q "TestL30Count fails on the plant" TestL30Count flush.go \
  's/id, err := r\.minter\.Mint(rec\.kind\.Kind)/id, err := deedID, error(nil)/'

go_row RT-31 rt30 "TestL31 and TestBufferCopy rc=0" /w/go test -run '^(TestL31|TestBufferCopy)' ./rt/
rt30_plant RT-31p "TestL31Order fails on the plant" TestL31Order flush.go \
  's/for _, name := range r\.order {/for name := range r.byName {/'
rt30_plant RT-31q "TestL31Conflict fails on the plant" TestL31Conflict flush.go \
  's/if out\[i\] == Committed {/if true {/'
rt30_plant RT-31r "TestL31Refs fails on the plant" TestL31Refs flush.go \
  's/if rec\.ref\.buf != b || rec\.ref\.epoch != b\.epoch {/if false {/'
rt30_plant RT-31s "TestBufferCopy fails on the plant" TestBufferCopy buffer.go \
  's/val := append(\[\]byte(nil), v\.\.\.)/val := v/'

go_row RT-31c rt30 "TestL31Cascade rc=0" /w/go test -run '^TestL31Cascade$' ./rt/
rt30_plant RT-31cp "TestL31Cascade fails on the plant" TestL31Cascade flush.go \
  's/if rec\.ref\.buf != b || rec\.ref\.epoch != b\.epoch {/if rec.ref.epoch != b.epoch {/'

go_row RT-32 rt30 "TestCheck and TestFlush rc=0" /w/go test -run '^(TestCheck|TestFlush)' ./rt/
rt30_plant RT-32p "TestCheck fails on the plant" TestCheck check.go 's/if st\.destroyed {/if false {/'
rt30_plant RT-32q "TestCheck fails on the plant" TestCheck check.go \
  's/case def\.Form != ecd\.Tag && !def\.Pointers && len(op\.Value) != def\.Size:/case false:/'
rt30_plant RT-32r "TestFlushDropped fails on the plant" TestFlushDropped flush.go 's/n += bufs\[i\]\.Len()/n++/'

# RT-37: the engine packages do not depend on package rt. PASS when the list has a line and none is that package.
rt30_e37='go/rt not in go list -deps ./engine/...'
rt30_e37p='go/rt in go list -deps ./engine/... on the plant'
rt30_s37='s|^package jacquard$|package jacquard; import _ "github.com/fiberfx/ecd-runtime/go/rt"|'
if sel RT-37 rt30; then
  if [ $HAVE_GO -eq 0 ]; then say RT-37 "$rt30_e37" "go not on PATH" CNM
  else
    rt30_deps /w/go /tmp/rows/RT-37.log
    case $rt30_state in
      clear) say RT-37 "$rt30_e37" "$rt30_obs" PASS ;;
      listed) say RT-37 "$rt30_e37" "$rt30_obs" FAIL ;;
      *) say RT-37 "$rt30_e37" "$rt30_obs" CNM; head_of /tmp/rows/RT-37.log.err 8 ;;
    esac
  fi
fi

# RT-37p: the same measure on a copy whose package jacquard imports go/rt. KILLED when the list names go/rt.
if sel RT-37p rt30; then
  if [ $HAVE_GO -eq 0 ]; then say RT-37p "$rt30_e37p" "go not on PATH" CNM
  else
    rt30_pd=$(mktemp -d /tmp/rows/plant.XXXXXX)
    cp -a /w/go "$rt30_pd/go" && cp "$rt30_pd/go/engine/jacquard/doc.go" "$rt30_pd/orig.go"
    (cd "$rt30_pd/go/engine/jacquard" && sed -i "$rt30_s37" doc.go)
    rt30_n=$(diff "$rt30_pd/orig.go" "$rt30_pd/go/engine/jacquard/doc.go" | grep -c '^>')
    if [ "$rt30_n" -ne 1 ]; then
      say RT-37p "$rt30_e37p" "the sed changed $rt30_n lines of doc.go, want 1" CNM
    else
      rt30_deps "$rt30_pd/go" /tmp/rows/RT-37p.log
      case $rt30_state in
        listed) say RT-37p "$rt30_e37p" "$rt30_obs" KILLED ;;
        clear) say RT-37p "$rt30_e37p" "$rt30_obs" SURVIVED ;;
        *) say RT-37p "$rt30_e37p" "$rt30_obs" CNM
           head_of /tmp/rows/RT-37p.log.err 8 ;;
      esac
    fi
    rm -rf "$rt30_pd"
  fi
fi
