# Group sq40 of Ecd Runtime: the module go/sqlite, an owner store in one SQLite file that a host passes to the Runtime
# as its Committer and that hydrates a fresh engine after a restart. The rows show that the module vets, that its
# requirements, the driver included, resolve and verify from the offline GOPROXY tree and that go mod tidy leaves its
# go.mod and go.sum unchanged, that Create and Open keep the settings and the schema of the store, that a committed
# history survives a close and hydrates a fresh engine equal to the live one, that the log replays to the same state,
# that a refused or retried deed leaves nothing in the file, and that the writer and the reader carry their pragmas.
# Sourced by the rows harness from /w; the harness supplies say, sel, offline, head_of, count, why_of, go_row and
# HAVE_GO.
#
#   SQ-01    go vet ./... in go/sqlite
#   SQ-02    go mod download in a copy of go/ with an empty module cache verifies every module of go/sqlite against its
#            go.sum, and the driver resolves to modernc.org/sqlite v1.37.0
#   SQ-03    go mod tidy in a copy of go/ leaves go/sqlite/go.mod and go.sum byte-equal
#   SQ-03p   plant: the driver requirement of go/sqlite/go.mod is marked indirect; go mod tidy removes the comment, so
#            the files differ
#   SQ-10    Create makes a file with the page size, the auto-vacuum mode, the journal mode and the schema of the store
#            and refuses an existing path with ErrExist; Open never creates a file and refuses a file of another schema
#            or journal mode with ErrSchema (TestCreate)
#   SQ-10p   plant: Create asks for journal mode DELETE and refuses its answer (store.go; TestCreate)
#   SQ-11    a committed history survives a close and hydrates a fresh engine equal to the live one on Get and on the
#            queries, a store of 100,000 entities hydrates from one read transaction, a catalog with no All kind still
#            brings a new engine to the seq of the store, and the log replays to the same state (TestFlushReopen,
#            TestReplayLog, TestHydrateLarge, TestHydrateTouchedOnly)
#   SQ-11p   plant: the destroy of an entity leaves its component values in the file (commit.go; TestFlushReopen)
#   SQ-11q   plant: the unset of a component leaves its value in the file (commit.go; TestFlushReopen)
#   SQ-12    a deed that the state of the store refuses is answered refused and a retried deed eid is answered conflict;
#            a refused or conflicted deed leaves no row, and a commit of refused deeds commits nothing and answers 0
#            (TestRefused, TestRefusedAtomic)
#   SQ-12p   plant: a Set or Unset on a destroyed entity is committed (commit.go; TestRefused)
#   SQ-12q   plant: a Spawn of an eid that exists replaces its row (commit.go; TestRefused)
#   SQ-12c   plant: a retried deed eid is not answered conflict, since the deed insert has no conflict clause
#            (commit.go; TestRefused)
#   SQ-12r   plant: a refused deed is not rolled back to its savepoint, so its earlier writes stay (commit.go;
#            TestRefusedAtomic)
#   SQ-12s   plant: a commit in which every deed is refused commits its empty transaction (commit.go; TestRefused)
#   SQ-14    the writer carries busy_timeout, foreign_keys and synchronous FULL, and the reader busy_timeout and
#            query_only (TestL24Conns)
#   SQ-14p   plant: the writer runs without foreign_keys (store.go; TestL24Conns)
#
# The rows SQ-01 to SQ-14 hold only with the files of module go/sqlite. Its requirements come from the module inputs,
# a GOPROXY file tree that holds the driver and its modules beside ecd-id.
#
# sq40_plant id expect test file sedexpr: copies /w/go to a temporary directory, applies the one-line sed to
# sqlite/<file> there and runs only the named test in go/sqlite of the copy. The copy is the whole of /w/go, since
# go/sqlite replaces the main module by its parent directory. KILLED: the test failed, and the observed text names its
# first failing line. SURVIVED: it passed. CNM: the sed did not change exactly one line, the file or the module was
# missing, or the test did not run.
sq40_plant() {
  sq40_id=$1 sq40_ex=$2 sq40_test=$3 sq40_file=$4 sq40_sed=$5
  sel "$sq40_id" sq40 || return 0
  if [ $HAVE_GO -eq 0 ]; then say "$sq40_id" "$sq40_ex" "go not on PATH" CNM; return 0; fi
  sq40_pd=$(mktemp -d /tmp/rows/plant.XXXXXX)
  cp -a /w/go "$sq40_pd/go"
  if [ ! -f "$sq40_pd/go/sqlite/$sq40_file" ]; then
    say "$sq40_id" "$sq40_ex" "go/sqlite/$sq40_file is absent" CNM
    rm -rf "$sq40_pd"
    return 0
  fi
  cp "$sq40_pd/go/sqlite/$sq40_file" "$sq40_pd/orig.go"
  (cd "$sq40_pd/go/sqlite" && sed -i "$sq40_sed" "$sq40_file")
  sq40_n=$(diff "$sq40_pd/orig.go" "$sq40_pd/go/sqlite/$sq40_file" | grep -c '^>')
  if [ "$sq40_n" -ne 1 ]; then
    say "$sq40_id" "$sq40_ex" "the sed changed $sq40_n lines of $sq40_file, want 1" CNM
  else
    sq40_log=/tmp/rows/$sq40_id.log
    (cd "$sq40_pd/go/sqlite" && go test -count=1 -v -run "^$sq40_test\$" . >"$sq40_log" 2>&1); sq40_rc=$?
    if offline "$sq40_log"; then say "$sq40_id" "$sq40_ex" "module not found" CNM
    elif grep -q "^--- FAIL: $sq40_test " "$sq40_log"; then
      say "$sq40_id" "$sq40_ex" "rc=$sq40_rc, $sq40_test FAIL: $(why_of "$sq40_log")" KILLED
    elif [ $sq40_rc -eq 0 ] && grep -q "^--- PASS: $sq40_test " "$sq40_log"; then
      say "$sq40_id" "$sq40_ex" "rc=0, $sq40_test PASS" SURVIVED
    else say "$sq40_id" "$sq40_ex" "rc=$sq40_rc, the test did not run" CNM; head_of "$sq40_log" 8; fi
  fi
  rm -rf "$sq40_pd"
}

# The driver line that go list -m prints for the requirement of go/sqlite.
sq40_drv='modernc.org/sqlite v1.37.0'

# sq40_copy: makes the temporary directory sq40_pd holding a copy of /w/go; 1 when it cannot.
sq40_copy() {
  sq40_pd=$(mktemp -d /tmp/rows/sq.XXXXXX) || return 1
  cp -a /w/go "$sq40_pd/go" && return 0
  rm -rf "$sq40_pd"
  return 1
}

# sq40_mod: 0 when the copy of go/sqlite holds go.mod and go.sum.
sq40_mod() { [ -f "$sq40_pd/go/sqlite/go.mod" ] && [ -f "$sq40_pd/go/sqlite/go.sum" ]; }

# sq40_gofile: 0 when the copy of go/sqlite holds a Go file.
sq40_gofile() { [ -n "$(find "$sq40_pd/go/sqlite" -name '*.go' -print -quit)" ]; }

# sq40_apply sedexpr: applies the sed to go.mod of the copy of go/sqlite. sq40_n is the number of lines the sed
# changed; a plant needs exactly 1.
sq40_apply() {
  cp "$sq40_pd/go/sqlite/go.mod" "$sq40_pd/orig"
  sed -i "$1" "$sq40_pd/go/sqlite/go.mod"
  sq40_n=$(diff "$sq40_pd/orig" "$sq40_pd/go/sqlite/go.mod" | grep -c '^>')
}

# sq40_download log: go mod download in the copy of go/sqlite with an empty module cache of its own, then the driver
# line that go list -m prints there; sq40_rc is the exit code of go mod download.
sq40_download() {
  (cd "$sq40_pd/go/sqlite" && GOMODCACHE="$sq40_pd/mod" go mod download) >"$1" 2>&1
  sq40_rc=$?
  sq40_ver=$(cd "$sq40_pd/go/sqlite" && GOMODCACHE="$sq40_pd/mod" go list -m modernc.org/sqlite 2>>"$1")
}

# sq40_tidy log: go mod tidy in the copy of go/sqlite; sq40_rc is its exit code and sq40_same is 1 when go.mod and
# go.sum are byte-equal to what they were before.
sq40_tidy() {
  sq40_e=$sq40_pd/go/sqlite
  cp "$sq40_e/go.mod" "$sq40_pd/before.mod"
  cp "$sq40_e/go.sum" "$sq40_pd/before.sum"
  (cd "$sq40_e" && go mod tidy) >"$1" 2>&1
  sq40_rc=$?
  sq40_same=0
  if cmp -s "$sq40_pd/before.mod" "$sq40_e/go.mod" && cmp -s "$sq40_pd/before.sum" "$sq40_e/go.sum"; then
    sq40_same=1
  fi
}

go_row SQ-01 sq40 "go vet rc=0" /w/go/sqlite vet ./...

# SQ-02
sq40_ex="go mod download rc=0 and the driver resolves to $sq40_drv"
if sel SQ-02 sq40; then
  if [ $HAVE_GO -eq 0 ]; then say SQ-02 "$sq40_ex" "go not on PATH" CNM
  elif ! sq40_copy; then say SQ-02 "$sq40_ex" "the copy failed" CNM
  elif ! sq40_mod; then say SQ-02 "$sq40_ex" "go/sqlite holds no go.mod or go.sum" CNM; rm -rf "$sq40_pd"
  else
    sq40_log=/tmp/rows/SQ-02.log
    sq40_download "$sq40_log"
    if offline "$sq40_log"; then say SQ-02 "$sq40_ex" "module not found" CNM; head_of "$sq40_log" 8
    elif [ $sq40_rc -ne 0 ]; then say SQ-02 "$sq40_ex" "go mod download rc=$sq40_rc" FAIL; head_of "$sq40_log" 8
    elif [ "$sq40_ver" != "$sq40_drv" ]; then say SQ-02 "$sq40_ex" "rc=0, go list -m prints '$sq40_ver'" FAIL
    else say SQ-02 "$sq40_ex" "rc=0, go list -m prints $sq40_ver" PASS; fi
    rm -rf "$sq40_pd"
  fi
fi

# SQ-03
sq40_ex="go mod tidy leaves go.mod and go.sum of go/sqlite byte-equal"
if sel SQ-03 sq40; then
  if [ $HAVE_GO -eq 0 ]; then say SQ-03 "$sq40_ex" "go not on PATH" CNM
  elif ! sq40_copy; then say SQ-03 "$sq40_ex" "the copy failed" CNM
  elif ! sq40_mod; then say SQ-03 "$sq40_ex" "go/sqlite holds no go.mod or go.sum" CNM; rm -rf "$sq40_pd"
  elif ! sq40_gofile; then say SQ-03 "$sq40_ex" "go/sqlite holds no Go file, so tidy would drop every requirement" CNM
    rm -rf "$sq40_pd"
  else
    sq40_log=/tmp/rows/SQ-03.log
    sq40_tidy "$sq40_log"
    if offline "$sq40_log"; then say SQ-03 "$sq40_ex" "module not found" CNM; head_of "$sq40_log" 8
    elif [ $sq40_rc -ne 0 ]; then say SQ-03 "$sq40_ex" "go mod tidy rc=$sq40_rc" FAIL; head_of "$sq40_log" 8
    elif [ $sq40_same -ne 1 ]; then say SQ-03 "$sq40_ex" "go mod tidy rc=0, the files changed" FAIL
    else say SQ-03 "$sq40_ex" "go mod tidy rc=0, go.mod and go.sum unchanged" PASS; fi
    rm -rf "$sq40_pd"
  fi
fi

# SQ-03p
sq40_ex="go mod tidy changes go.mod of go/sqlite on the plant"
if sel SQ-03p sq40; then
  if [ $HAVE_GO -eq 0 ]; then say SQ-03p "$sq40_ex" "go not on PATH" CNM
  elif ! sq40_copy; then say SQ-03p "$sq40_ex" "the copy failed" CNM
  elif ! sq40_mod; then say SQ-03p "$sq40_ex" "go/sqlite holds no go.mod or go.sum" CNM; rm -rf "$sq40_pd"
  elif ! sq40_gofile; then say SQ-03p "$sq40_ex" "go/sqlite holds no Go file, so tidy would drop every requirement" CNM
    rm -rf "$sq40_pd"
  else
    sq40_apply 's|^\tmodernc\.org/sqlite v1\.37\.0$|\tmodernc.org/sqlite v1.37.0 // indirect|'
    if [ "$sq40_n" -ne 1 ]; then
      say SQ-03p "$sq40_ex" "the sed changed $sq40_n lines of go.mod, want 1" CNM
    else
      sq40_log=/tmp/rows/SQ-03p.log
      sq40_tidy "$sq40_log"
      if offline "$sq40_log"; then say SQ-03p "$sq40_ex" "module not found" CNM; head_of "$sq40_log" 8
      elif [ $sq40_rc -ne 0 ]; then say SQ-03p "$sq40_ex" "go mod tidy rc=$sq40_rc" CNM; head_of "$sq40_log" 8
      elif [ $sq40_same -ne 1 ]; then say SQ-03p "$sq40_ex" "go mod tidy rc=0, the files changed" KILLED
      else say SQ-03p "$sq40_ex" "go mod tidy rc=0, the files are unchanged" SURVIVED; fi
    fi
    rm -rf "$sq40_pd"
  fi
fi

go_row SQ-10 sq40 "TestCreate rc=0" /w/go/sqlite test -run '^TestCreate$' .
sq40_plant SQ-10p "TestCreate fails on the plant" TestCreate store.go \
  's/^const sqlWAL = `PRAGMA journal_mode = WAL`$/const sqlWAL = `PRAGMA journal_mode = DELETE`/'

go_row SQ-11 sq40 "TestFlushReopen, TestReplayLog, TestHydrateLarge, TestHydrateTouchedOnly rc=0" /w/go/sqlite test -run '^(TestFlushReopen|TestReplayLog|TestHydrateLarge|TestHydrateTouchedOnly)$' .
sq40_plant SQ-11p "TestFlushReopen fails on the plant" TestFlushReopen commit.go \
  's/^const sqlClear = `DELETE FROM state WHERE id = ?1`$/const sqlClear = `DELETE FROM state WHERE id = ?1 AND 0`/'
sq40_plant SQ-11q "TestFlushReopen fails on the plant" TestFlushReopen commit.go \
  's/^const sqlUnset = `DELETE FROM state WHERE id = ?1 AND cid = ?2`$/const sqlUnset = `DELETE FROM state WHERE id = ?1 AND cid = ?2 AND 0`/'

go_row SQ-12 sq40 "TestRefused, TestRefusedAtomic rc=0" /w/go/sqlite test -run '^(TestRefused|TestRefusedAtomic)$' .
sq40_plant SQ-12p "TestRefused fails on the plant" TestRefused commit.go \
  's/^const sqlTouch = `UPDATE entity SET seq = ?2 WHERE id = ?1 AND gone IS NULL`$/const sqlTouch = `UPDATE entity SET seq = ?2 WHERE id = ?1`/'
sq40_plant SQ-12q "TestRefused fails on the plant" TestRefused commit.go \
  's/^const sqlSpawn = `INSERT INTO entity (id, seq) VALUES (?1, ?2) ON CONFLICT DO NOTHING`$/const sqlSpawn = `INSERT OR REPLACE INTO entity (id, seq) VALUES (?1, ?2)`/'
sq40_plant SQ-12c "TestRefused fails on the plant" TestRefused commit.go \
  's/^const sqlDeed = `INSERT INTO deed (id, seq, ord, at) VALUES (?1, ?2, ?3, ?4) ON CONFLICT (id) DO NOTHING`$/const sqlDeed = `INSERT INTO deed (id, seq, ord, at) VALUES (?1, ?2, ?3, ?4)`/'
sq40_plant SQ-12r "TestRefusedAtomic fails on the plant" TestRefusedAtomic commit.go \
  's/if _, err := tx\.Exec("ROLLBACK TO deed"); err != nil {/if _, err := tx.Exec("SELECT 1"); err != nil {/'
sq40_plant SQ-12s "TestRefused fails on the plant" TestRefused commit.go 's/if committed == 0 {/if false {/'

go_row SQ-14 sq40 "TestL24Conns rc=0" /w/go/sqlite test -run '^TestL24Conns$' .
sq40_plant SQ-14p "TestL24Conns fails on the plant" TestL24Conns store.go \
  's/p := \[\]string{"busy_timeout(2000)", "foreign_keys(1)", "synchronous(FULL)"}/p := []string{"busy_timeout(2000)", "synchronous(FULL)"}/'
