# Group ecm19 of Entity Component Runtime: the module go/enginecmp, which requires Ark at commit 723f2ac from the
# offline GOPROXY tree and replaces the main module by path. The rows show that the module graph of go/ never names Ark,
# that the Ark zip of the inputs matches go.sum, and that go mod tidy leaves the files of go/enginecmp unchanged.
# Sourced by the rows harness from /w; the harness supplies say, sel, offline, head_of, count, why_of, go_row and
# HAVE_GO.
#
#   EC-01   go list -mod=mod -m all in a copy of go/ names github.com/fiberfx/ecd-id/go and not github.com/mlange-42/ark
#   EC-01p  plant: the one requirement of go/go.mod is Ark instead of ecd-id; the list names Ark
#   EC-02   go mod download in a copy of go/enginecmp verifies every module against go.sum, and Ark resolves to
#           v0.8.4-0.20261110114945-723f2ac3186a
#   EC-02p  plant: the go.sum line of the Ark zip carries the hash of the Ark go.mod; go mod download refuses the zip
#           with a checksum mismatch, in an empty module cache
#   EC-03   go mod tidy in a copy of go/enginecmp leaves go.mod and go.sum byte-equal
#   EC-03p  plant: the ecd-id requirement of go/enginecmp/go.mod is marked indirect; go mod tidy removes the comment
#   EC-04   go vet ./... in go/enginecmp
#
# The rows EC-03, EC-03p and EC-04 need a package in go/enginecmp: go mod tidy keeps a requirement only when a package
# imports it, and go vet needs a package to check. Without a Go file in go/enginecmp, EC-03 and EC-03p are CNM, and
# EC-04 reports that it found no package.
#
# The rows work on copies of /w/go under /tmp/rows, so that go list, go mod download and go mod tidy do not change the
# unpacked tree.
ecm19_v=v0.8.4-0.20261110114945-723f2ac3186a
ecm19_nogo="go/enginecmp holds no Go file, so tidy would drop every requirement"

# ecm19_copy: makes the temporary directory ecm19_pd holding a copy of /w/go; 1 when it cannot.
ecm19_copy() {
  ecm19_pd=$(mktemp -d /tmp/rows/ecm.XXXXXX) || return 1
  cp -a /w/go "$ecm19_pd/go" && return 0
  rm -rf "$ecm19_pd"
  return 1
}

# ecm19_apply file sedexpr: applies the sed to the file of the copy, relative to its go directory. ecm19_n is the
# number of lines the sed changed; a plant needs exactly 1.
ecm19_apply() {
  cp "$ecm19_pd/go/$1" "$ecm19_pd/orig"
  sed -i "$2" "$ecm19_pd/go/$1"
  ecm19_n=$(diff "$ecm19_pd/orig" "$ecm19_pd/go/$1" | grep -c '^>')
}

# ecm19_list log: go list -mod=mod -m all in the copy of go/; ecm19_rc is its exit code.
ecm19_list() {
  (cd "$ecm19_pd/go" && go list -mod=mod -m all) >"$1" 2>&1
  ecm19_rc=$?
}

# ecm19_download log: go mod download in the copy of go/enginecmp with an empty module cache of its own, then the
# version of Ark that the module resolves; ecm19_rc is the exit code of go mod download.
ecm19_download() {
  (cd "$ecm19_pd/go/enginecmp" && GOMODCACHE="$ecm19_pd/mod" go mod download) >"$1" 2>&1
  ecm19_rc=$?
  ecm19_ver=$(cd "$ecm19_pd/go/enginecmp" && GOMODCACHE="$ecm19_pd/mod" go list -m -f '{{.Version}}' \
    github.com/mlange-42/ark 2>>"$1")
}

# ecm19_tidy log: go mod tidy in the copy of go/enginecmp; ecm19_rc is its exit code and ecm19_same is 1 when go.mod
# and go.sum are byte-equal to what they were before.
ecm19_tidy() {
  ecm19_e=$ecm19_pd/go/enginecmp
  cp "$ecm19_e/go.mod" "$ecm19_pd/before.mod"
  cp "$ecm19_e/go.sum" "$ecm19_pd/before.sum"
  (cd "$ecm19_e" && go mod tidy) >"$1" 2>&1
  ecm19_rc=$?
  ecm19_same=0
  if cmp -s "$ecm19_pd/before.mod" "$ecm19_e/go.mod" && cmp -s "$ecm19_pd/before.sum" "$ecm19_e/go.sum"; then
    ecm19_same=1
  fi
}

# ecm19_gofile: 0 when the copy of go/enginecmp holds a Go file.
ecm19_gofile() { [ -n "$(find "$ecm19_pd/go/enginecmp" -name '*.go' -print -quit)" ]; }

# EC-01
ecm19_ex="the list names github.com/fiberfx/ecd-id/go and no line of github.com/mlange-42/ark"
if sel EC-01 ecm19; then
  if [ $HAVE_GO -eq 0 ]; then say EC-01 "$ecm19_ex" "go not on PATH" CNM
  elif ! ecm19_copy; then say EC-01 "$ecm19_ex" "the copy failed" CNM
  else
    ecm19_log=/tmp/rows/EC-01.log
    ecm19_list "$ecm19_log"
    ecm19_na=$(count 'mlange-42/ark' "$ecm19_log")
    ecm19_ne=$(count '^github.com/fiberfx/ecd-id/go ' "$ecm19_log")
    if offline "$ecm19_log"; then say EC-01 "$ecm19_ex" "module not found" CNM; head_of "$ecm19_log" 8
    elif [ "$ecm19_na" -ne 0 ]; then say EC-01 "$ecm19_ex" "rc=$ecm19_rc, $ecm19_na lines name Ark" FAIL
    elif [ $ecm19_rc -ne 0 ]; then say EC-01 "$ecm19_ex" "go list rc=$ecm19_rc" CNM; head_of "$ecm19_log" 8
    elif [ "$ecm19_ne" -eq 0 ]; then say EC-01 "$ecm19_ex" "rc=0, the list does not name ecd-id" CNM
    else say EC-01 "$ecm19_ex" "rc=0, ecd-id listed, 0 lines name Ark" PASS; fi
    rm -rf "$ecm19_pd"
  fi
fi

# EC-01p
ecm19_ex="the list names Ark on the plant"
if sel EC-01p ecm19; then
  if [ $HAVE_GO -eq 0 ]; then say EC-01p "$ecm19_ex" "go not on PATH" CNM
  elif ! ecm19_copy; then say EC-01p "$ecm19_ex" "the copy failed" CNM
  else
    ecm19_apply go.mod \
      "s|^require github\.com/fiberfx/ecd-id/go v0\.11\.0\$|require github.com/mlange-42/ark $ecm19_v|"
    if [ "$ecm19_n" -ne 1 ]; then
      say EC-01p "$ecm19_ex" "the sed changed $ecm19_n lines of go.mod, want 1" CNM
    else
      ecm19_log=/tmp/rows/EC-01p.log
      ecm19_list "$ecm19_log"
      ecm19_na=$(count 'mlange-42/ark' "$ecm19_log")
      if offline "$ecm19_log"; then say EC-01p "$ecm19_ex" "module not found" CNM; head_of "$ecm19_log" 8
      elif [ "$ecm19_na" -ne 0 ]; then say EC-01p "$ecm19_ex" "rc=$ecm19_rc, $ecm19_na lines name Ark" KILLED
      elif [ $ecm19_rc -ne 0 ]; then say EC-01p "$ecm19_ex" "go list rc=$ecm19_rc" CNM; head_of "$ecm19_log" 8
      else say EC-01p "$ecm19_ex" "rc=0, no line names Ark" SURVIVED; fi
    fi
    rm -rf "$ecm19_pd"
  fi
fi

# EC-02
ecm19_ex="go mod download rc=0 and Ark resolves to $ecm19_v"
if sel EC-02 ecm19; then
  if [ $HAVE_GO -eq 0 ]; then say EC-02 "$ecm19_ex" "go not on PATH" CNM
  elif ! ecm19_copy; then say EC-02 "$ecm19_ex" "the copy failed" CNM
  else
    ecm19_log=/tmp/rows/EC-02.log
    ecm19_download "$ecm19_log"
    if offline "$ecm19_log"; then say EC-02 "$ecm19_ex" "module not found" CNM; head_of "$ecm19_log" 8
    elif [ $ecm19_rc -ne 0 ]; then say EC-02 "$ecm19_ex" "go mod download rc=$ecm19_rc" FAIL; head_of "$ecm19_log" 8
    elif [ "$ecm19_ver" != "$ecm19_v" ]; then say EC-02 "$ecm19_ex" "rc=0, Ark resolves to '$ecm19_ver'" FAIL
    else say EC-02 "$ecm19_ex" "rc=0, Ark resolves to $ecm19_ver" PASS; fi
    rm -rf "$ecm19_pd"
  fi
fi

# EC-02p
ecm19_ex="go mod download refuses the Ark zip with a checksum mismatch on the plant"
if sel EC-02p ecm19; then
  if [ $HAVE_GO -eq 0 ]; then say EC-02p "$ecm19_ex" "go not on PATH" CNM
  elif ! ecm19_copy; then say EC-02p "$ecm19_ex" "the copy failed" CNM
  else
    ecm19_apply enginecmp/go.sum \
      's|^\(github\.com/mlange-42/ark v[^ /]*\) h1:.*$|\1 h1:gkS9cuklENPTmSjL2z4DcJgJsIVqF1yNwFlx48Hz/Sw=|'
    if [ "$ecm19_n" -ne 1 ]; then
      say EC-02p "$ecm19_ex" "the sed changed $ecm19_n lines of go.sum, want 1" CNM
    else
      ecm19_log=/tmp/rows/EC-02p.log
      ecm19_download "$ecm19_log"
      if grep -q -E 'checksum mismatch|SECURITY ERROR' "$ecm19_log"; then
        say EC-02p "$ecm19_ex" "go mod download rc=$ecm19_rc, checksum mismatch" KILLED
      elif offline "$ecm19_log"; then say EC-02p "$ecm19_ex" "module not found" CNM; head_of "$ecm19_log" 8
      elif [ $ecm19_rc -eq 0 ]; then say EC-02p "$ecm19_ex" "go mod download rc=0" SURVIVED
      else say EC-02p "$ecm19_ex" "go mod download rc=$ecm19_rc, no checksum mismatch" CNM; head_of "$ecm19_log" 8; fi
    fi
    rm -rf "$ecm19_pd"
  fi
fi

# EC-03
ecm19_ex="go mod tidy leaves go.mod and go.sum of go/enginecmp byte-equal"
if sel EC-03 ecm19; then
  if [ $HAVE_GO -eq 0 ]; then say EC-03 "$ecm19_ex" "go not on PATH" CNM
  elif ! ecm19_copy; then say EC-03 "$ecm19_ex" "the copy failed" CNM
  elif ! ecm19_gofile; then say EC-03 "$ecm19_ex" "$ecm19_nogo" CNM; rm -rf "$ecm19_pd"
  else
    ecm19_log=/tmp/rows/EC-03.log
    ecm19_tidy "$ecm19_log"
    if offline "$ecm19_log"; then say EC-03 "$ecm19_ex" "module not found" CNM; head_of "$ecm19_log" 8
    elif [ $ecm19_rc -ne 0 ]; then say EC-03 "$ecm19_ex" "go mod tidy rc=$ecm19_rc" FAIL; head_of "$ecm19_log" 8
    elif [ $ecm19_same -ne 1 ]; then say EC-03 "$ecm19_ex" "go mod tidy rc=0, the files changed" FAIL
    else say EC-03 "$ecm19_ex" "go mod tidy rc=0, go.mod and go.sum unchanged" PASS; fi
    rm -rf "$ecm19_pd"
  fi
fi

# EC-03p
ecm19_ex="go mod tidy changes go.mod of go/enginecmp on the plant"
if sel EC-03p ecm19; then
  if [ $HAVE_GO -eq 0 ]; then say EC-03p "$ecm19_ex" "go not on PATH" CNM
  elif ! ecm19_copy; then say EC-03p "$ecm19_ex" "the copy failed" CNM
  elif ! ecm19_gofile; then say EC-03p "$ecm19_ex" "$ecm19_nogo" CNM; rm -rf "$ecm19_pd"
  else
    ecm19_apply enginecmp/go.mod \
      's|^\tgithub\.com/fiberfx/ecd-id/go v0\.11\.0$|\tgithub.com/fiberfx/ecd-id/go v0.11.0 // indirect|'
    if [ "$ecm19_n" -ne 1 ]; then
      say EC-03p "$ecm19_ex" "the sed changed $ecm19_n lines of go.mod, want 1" CNM
    else
      ecm19_log=/tmp/rows/EC-03p.log
      ecm19_tidy "$ecm19_log"
      if offline "$ecm19_log"; then say EC-03p "$ecm19_ex" "module not found" CNM; head_of "$ecm19_log" 8
      elif [ $ecm19_rc -ne 0 ]; then say EC-03p "$ecm19_ex" "go mod tidy rc=$ecm19_rc" CNM; head_of "$ecm19_log" 8
      elif [ $ecm19_same -ne 1 ]; then say EC-03p "$ecm19_ex" "go mod tidy rc=0, the files changed" KILLED
      else say EC-03p "$ecm19_ex" "go mod tidy rc=0, the files are unchanged" SURVIVED; fi
    fi
    rm -rf "$ecm19_pd"
  fi
fi

go_row EC-04 ecm19 "go vet rc=0" /w/go/enginecmp vet ./...
