# Group jacquard of Ecd Loom: JQ1 and JQ1p.
# Sourced by rows/loom.rows.sh from /w; the harness supplies say, sel, offline, head_of, count, tree_of, go_row and HAVE_GO.
go_row JQ1 jacquard "go test ./engine/jacquard/... rc=0" /w/go test ./engine/jacquard/...
if sel JQ1p jacquard; then
  if [ $HAVE_GO -eq 0 ]; then say JQ1p "TestReservedEntities fails on the plant" "go not on PATH" CNM
  else
    pd=$(mktemp -d /tmp/rows/plant.XXXXXX)
    cp -a /w/go "$pd/go" && cp "$pd/go/engine/jacquard/entity.go" "$pd/entity.go.orig" &&
      sed -i 's/return e\.id == 0$/return e.id == 1/' "$pd/go/engine/jacquard/entity.go"
    if cmp -s "$pd/entity.go.orig" "$pd/go/engine/jacquard/entity.go"; then
      say JQ1p "the plant changes entity.go" "entity.go unchanged after sed" CNM
    else
      log=/tmp/rows/JQ1p.log
      (cd "$pd/go" && go test -count=1 -v -run '^TestReservedEntities$' ./engine/jacquard/ >"$log" 2>&1); rc=$?
      if offline "$log"; then say JQ1p "TestReservedEntities fails on the plant" "module not found" CNM
      elif grep -q '^--- FAIL: TestReservedEntities' "$log"; then say JQ1p "TestReservedEntities fails on the plant" "rc=$rc, TestReservedEntities FAIL" KILLED
      elif [ $rc -eq 0 ] && grep -q '^--- PASS: TestReservedEntities' "$log"; then say JQ1p "TestReservedEntities fails on the plant" "rc=0, TestReservedEntities PASS" SURVIVED
      else say JQ1p "TestReservedEntities fails on the plant" "rc=$rc, the test did not run" CNM; head_of "$log" 8; fi
    fi
    rm -rf "$pd"
  fi
fi
