# Group ark19 of Ecd Runtime: the adapter of package ark in go/enginecmp, an ecd.Engine over Ark for catalogs of All
# kinds, and its refusals. The module go/enginecmp holds the adapter and the comparison tests; its go.mod requires Ark,
# which the inputs tree provides.
# Sourced by the rows harness from /w; the harness supplies say, sel, offline, head_of, count, why_of, go_row and
# HAVE_GO.
#
#   AR-01    go build ./... in go/enginecmp, which builds package ark
#   AR-02    TestArkNew, TestArkApply, TestArkHydrate, TestArkQuery and TestArkStats pass
#   AR-02p   plant: New does not refuse a Budget below 0 (ark.go; TestArkNew)
#   AR-02q   plant: New does not refuse a Touched kind (ark.go; TestArkNew)
#   AR-02r   plant: the check pass does not refuse a value of the wrong length (apply.go; TestArkApply)
#   AR-03p   plant: an Exact query is not exclusive (read.go; TestArkCompare)
#   AR-03q   plant: an Unset removes nothing (apply.go; TestArkCompare)
#
# The rows AR-03p and AR-03q hold only with the test TestArkCompare of package ark, in the file compare_test.go; a
# run without that test finds no test and is CNM.
#
# ark19_plant id expect test file sedexpr: copies /w/go to a temporary directory, applies the one-line sed to
# enginecmp/ark/<file> and runs only the named test there. KILLED: the test failed, and the observed text names its
# first failing line. SURVIVED: it passed. CNM: the sed did not change exactly one line, the module was missing, or the
# test did not run.
ark19_plant() {
  ark19_id=$1 ark19_ex=$2 ark19_test=$3 ark19_file=$4 ark19_sed=$5
  sel "$ark19_id" ark19 || return 0
  if [ $HAVE_GO -eq 0 ]; then say "$ark19_id" "$ark19_ex" "go not on PATH" CNM; return 0; fi
  ark19_pd=$(mktemp -d /tmp/rows/plant.XXXXXX)
  cp -a /w/go "$ark19_pd/go" && cp "$ark19_pd/go/enginecmp/ark/$ark19_file" "$ark19_pd/orig.go"
  (cd "$ark19_pd/go/enginecmp/ark" && sed -i "$ark19_sed" "$ark19_file")
  ark19_n=$(diff "$ark19_pd/orig.go" "$ark19_pd/go/enginecmp/ark/$ark19_file" | grep -c '^>')
  if [ "$ark19_n" -ne 1 ]; then
    say "$ark19_id" "$ark19_ex" "the sed changed $ark19_n lines of $ark19_file, want 1" CNM
  else
    ark19_log=/tmp/rows/$ark19_id.log
    (cd "$ark19_pd/go/enginecmp" && go test -count=1 -v -run "^$ark19_test\$" ./ark/ >"$ark19_log" 2>&1); ark19_rc=$?
    if offline "$ark19_log"; then say "$ark19_id" "$ark19_ex" "module not found" CNM
    elif grep -q "^--- FAIL: $ark19_test" "$ark19_log"; then
      say "$ark19_id" "$ark19_ex" "rc=$ark19_rc, $ark19_test FAIL: $(why_of "$ark19_log")" KILLED
    elif [ $ark19_rc -eq 0 ] && grep -q "^--- PASS: $ark19_test" "$ark19_log"; then
      say "$ark19_id" "$ark19_ex" "rc=0, $ark19_test PASS" SURVIVED
    else say "$ark19_id" "$ark19_ex" "rc=$ark19_rc, the test did not run" CNM; head_of "$ark19_log" 8; fi
  fi
  rm -rf "$ark19_pd"
}

go_row AR-01 ark19 "go build rc=0" /w/go/enginecmp build ./...
go_row AR-02 ark19 "TestArk New, Apply, Hydrate, Query, Stats rc=0" /w/go/enginecmp test \
  -run '^TestArk(New|Apply|Hydrate|Query|Stats)$' ./ark/
ark19_plant AR-02p "TestArkNew fails on the plant" TestArkNew ark.go 's/if k\.Budget < 0 {/if false {/'
ark19_plant AR-02q "TestArkNew fails on the plant" TestArkNew ark.go 's/if k\.Residency == ecd\.Touched {/if false {/'
ark19_plant AR-02r "TestArkApply fails on the plant" TestArkApply apply.go \
  's/case def\.Form != ecd\.Tag && !def\.Pointers && len(op\.Value) != def\.Size:/case false:/'
ark19_plant AR-03p "TestArkCompare fails on the plant" TestArkCompare read.go 's/f = f\.Exclusive()/_ = f/'
ark19_plant AR-03q "TestArkCompare fails on the plant" TestArkCompare apply.go 's/u\.Remove(ent, id)/_ = u/'
