# Group kr60 of Ecd Runtime: the generator half of story 26. gen -registry DIR refuses a catalog whose system and kinds
# are not live entity kinds of the ecd-id registry, each fault named; without the flag gen is as it was. The number 92
# is above every group file present when car g1 lands, after 91-kb60.sh.
# Sourced by the rows harness from /w; the harness supplies say, sel, offline, head_of, count, why_of, go_row and
# HAVE_GO.
#
#   KR-01    go test -run '^TestRunRegistry$/^good$' ./cmd/gen/: with -registry a catalog of live entity kinds generates
#            the committed bytes of internal/llm/llm_gen.go; TestGenerateWith (internal/gen) holds that a nil registry
#            is exactly Generate
#   KR-01p   plant: the flag is ignored (cmd/gen/main.go; TestRunRegistry, case violating)
#   KR-02    go test -run '^(TestRunRegistry|TestCheckRegistry)$' ./cmd/gen/ ./internal/gen/: a catalog of an unknown
#            system, a closed system, a detached catalog, an unknown kind, a kind of another role or a retired kind is
#            refused with exit 1, each fault named at its path, and nothing is written; a registry that cannot be read
#            or parsed is exit 3
#   KR-02p   plant: the role of a kind is not compared (internal/gen/registry.go; TestCheckRegistry, case role)
#   KR-02q   plant: the state of a kind is not checked (internal/gen/registry.go; TestCheckRegistry, case retired)
#
kr60_plant() {
  kr60_id=$1 kr60_ex=$2 kr60_mod=$3 kr60_pkg=$4 kr60_test=$5 kr60_case=$6 kr60_file=$7 kr60_sed=$8
  sel "$kr60_id" kr60 || return 0
  if [ $HAVE_GO -eq 0 ]; then say "$kr60_id" "$kr60_ex" "go not on PATH" CNM; return 0; fi
  kr60_name=$kr60_test
  if [ -n "$kr60_case" ]; then kr60_name=$kr60_test/$kr60_case; fi
  kr60_pd=$(mktemp -d /tmp/rows/plant.XXXXXX)
  cp -a /w/go "$kr60_pd/go"
  kr60_f=$kr60_pd/go/$kr60_file
  if [ ! -f "$kr60_f" ]; then
    say "$kr60_id" "$kr60_ex" "go/$kr60_file is absent" CNM
    rm -rf "$kr60_pd"
    return 0
  fi
  cp "$kr60_f" "$kr60_pd/orig.go"
  sed -i "$kr60_sed" "$kr60_f"
  kr60_n=$(diff "$kr60_pd/orig.go" "$kr60_f" | grep -c '^>')
  if [ "$kr60_n" -ne 1 ]; then
    say "$kr60_id" "$kr60_ex" "the sed changed $kr60_n lines of go/$kr60_file, want 1" CNM
  else
    kr60_log=/tmp/rows/$kr60_id.log
    (cd "$kr60_pd/go/$kr60_mod" && go test -count=1 -v -run "^$kr60_test\$" "$kr60_pkg" >"$kr60_log" 2>&1)
    kr60_rc=$?
    if offline "$kr60_log"; then say "$kr60_id" "$kr60_ex" "module not found" CNM
    elif grep -q '^panic:' "$kr60_log"; then
      say "$kr60_id" "$kr60_ex" "rc=$kr60_rc, $kr60_test panicked: $(why_of "$kr60_log")" CNM
    elif grep -q -E "^ *--- FAIL: $kr60_name " "$kr60_log"; then
      say "$kr60_id" "$kr60_ex" "rc=$kr60_rc, $kr60_name FAIL: $(why_of "$kr60_log")" KILLED
    elif grep -q "^--- FAIL: $kr60_test " "$kr60_log"; then
      say "$kr60_id" "$kr60_ex" "rc=$kr60_rc, $kr60_test FAIL, but not in $kr60_name" SURVIVED
    elif [ $kr60_rc -eq 0 ] && grep -q "^--- PASS: $kr60_test " "$kr60_log"; then
      say "$kr60_id" "$kr60_ex" "rc=0, $kr60_test PASS" SURVIVED
    else say "$kr60_id" "$kr60_ex" "rc=$kr60_rc, the test did not run" CNM; head_of "$kr60_log" 8; fi
  fi
  rm -rf "$kr60_pd"
}


go_row KR-01 kr60 "TestRunRegistry/good rc=0" /w/go test -run '^TestRunRegistry$/^good$' ./cmd/gen/
go_row KR-01g kr60 "TestGenerateWith rc=0" /w/go test -run '^TestGenerateWith$' ./internal/gen/
kr60_plant KR-01p "TestRunRegistry/violating fails on the plant" . ./cmd/gen/ TestRunRegistry violating cmd/gen/main.go \
  's/if \*regDir != "" {/if false \&\& *regDir != "" {/'

go_row KR-02 kr60 "TestRunRegistry and TestCheckRegistry rc=0" /w/go test -run '^(TestRunRegistry|TestCheckRegistry)$' ./cmd/gen/ ./internal/gen/
kr60_plant KR-02p "TestCheckRegistry/role fails on the plant" . ./internal/gen/ TestCheckRegistry role internal/gen/registry.go \
  's/if !entity\[code\] {/if false \&\& !entity[code] {/'
kr60_plant KR-02q "TestCheckRegistry/retired fails on the plant" . ./internal/gen/ TestCheckRegistry retired internal/gen/registry.go \
  's/if row\.State != eid\.Live {/if false \&\& row.State != eid.Live {/'
