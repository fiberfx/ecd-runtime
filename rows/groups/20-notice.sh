# Group notice of Ecd Runtime: NT1 to NT6 and their plants.
# Sourced by the rows harness from /w; the harness supplies say, sel, offline, head_of, count, go_row and HAVE_GO.
#
# The checks keep the attribution of code taken from other projects in one file, go/NOTICE.txt, keep the module's
# go line and requirements as they are declared, and keep the licence text of Ecd Runtime's own code in LICENSE and
# go/LICENSE.
#
#   NT1   no path, Go file or README names the directory of a vendored copy
#   NT1p  plant: a copy of go/ with a file added below a directory of that name; NT1 must turn red
#   NT1r  plant: a copy of the tree without README.md; NT1 answers could-not-measure, which is the expected verdict
#   NT2   every "// Derived from" line of a Go file is a line of go/NOTICE.txt, in at least 12 files
#   NT2p  plant: the attribution line of engine/jacquard/pool.go names another project; NT2 must turn red
#   NT2q  plant: a copy of go/NOTICE.txt without the Ark attribution line; NT2 must turn red
#   NT3   every file that go/NOTICE.txt lists exists, carries the attribution line and names its Ark source files;
#         go/NOTICE.txt holds the Ark copyright line and the MIT permission sentence
#   NT3p  plant: go/NOTICE.txt lists engine/jacquard/absent.go; NT3 must turn red
#   NT4   go/go.mod has the line "go 1.27.4", no toolchain line, and requires exactly github.com/fiberfx/ecd-id/go v0.8.0
#   NT4p  plant: a toolchain line appended to a copy of go/go.mod; NT4 must turn red
#   NT4q  plant: the go line of a copy of go/go.mod is "go 1.27"; NT4 must turn red
#   NT5   go mod tidy in a copy of go/, offline, leaves go.mod and go.sum byte-equal
#   NT5p  plant: a copy of go.sum without the go.mod line of the requirement; tidy restores it, so the copy differs
#   NT6   LICENSE and go/LICENSE are both present, byte-equal, and begin with "MIT License" and the copyright line
#         "Copyright (c) 2026 Jonnify"; the unpacked tree holds LICENSE only when the run archives it
#   NT6p  plant: a copy of the tree without go/LICENSE; NT6 must turn red
#   NT6q  plant: the copyright line of the copy of go/LICENSE reads "Jonnify Ltd"; the files differ, NT6 must turn red
#   NT6r  plant: a copy of the tree without LICENSE; NT6 answers could-not-measure, which is the expected verdict
#
# A plant edits text files or comments only, so a planted copy still compiles. A plant that changes other than exactly
# one line, or that cannot be applied, is CNM.
nt_tp=third_
nt_tp="${nt_tp}party"
nt_root=
mkdir -p /tmp/rows

# nt_changed A B: the number of lines in which two text files differ (the larger of the removed and added sides).
nt_changed() {
  nt_d=$(diff "$1" "$2" 2>/dev/null)
  nt_nl=$(printf '%s\n' "$nt_d" | grep -c '^<')
  nt_ng=$(printf '%s\n' "$nt_d" | grep -c '^>')
  if [ "$nt_nl" -gt "$nt_ng" ]; then echo "$nt_nl"; else echo "$nt_ng"; fi
}

# nt_mk: a temporary root holding a copy of /w/go, of /w/README.md and, when it exists, of /w/LICENSE; its path is in
# nt_root.
nt_mk() {
  nt_root=$(mktemp -d /tmp/rows/plant.XXXXXX) || return 1
  cp -a /w/go "$nt_root/go" && cp /w/README.md "$nt_root/README.md" &&
    { [ ! -f /w/LICENSE ] || cp /w/LICENSE "$nt_root/LICENSE"; }
}

# nt_verdict RC: the plant verdict of a check status: 0 stayed green, 1 turned red, anything else not measured.
nt_verdict() {
  case $1 in 0) echo SURVIVED ;; 1) echo KILLED ;; *) echo CNM ;; esac
}

# Each check takes a root directory (the unpacked tree, or a temporary copy of it), prints what it observed on one
# line, and returns 0 (as expected), 1 (not as expected) or 2 (not measurable).

nt1_check() {
  nt_p=$(find "$1" -name "*$nt_tp*" 2>/dev/null | head -3 | tr '\n' ' ')
  nt_f=$(grep -rlF --include='*.go' -- "$nt_tp" "$1/go" 2>/dev/null | head -3 | tr '\n' ' ')
  nt_m=
  if [ -f "$1/README.md" ] && grep -qF -- "$nt_tp" "$1/README.md"; then nt_m="$1/README.md"; fi
  if [ -n "$nt_p$nt_f$nt_m" ]; then echo "names $nt_tp: $nt_p$nt_f$nt_m"; return 1; fi
  if [ ! -f "$1/README.md" ]; then echo "README.md is not in the tree under test"; return 2; fi
  echo "no path, Go file or README names $nt_tp"
}

nt2_check() {
  nt_n="$1/go/NOTICE.txt"
  if [ ! -f "$nt_n" ]; then echo "go/NOTICE.txt is absent"; return 1; fi
  nt_l=$(mktemp /tmp/rows/nt2.XXXXXX)
  grep -rH --include='*.go' '^// Derived from ' "$1/go" >"$nt_l" 2>/dev/null
  nt_files=$(cut -d: -f1 "$nt_l" | sort -u | wc -l | tr -d ' ')
  nt_miss=0
  nt_first=
  while IFS= read -r nt_line; do
    nt_t=${nt_line#*:}
    nt_t=${nt_t#// }
    if ! grep -Fxq -- "$nt_t" "$nt_n" && ! grep -Fxq -- "Attribution: $nt_t" "$nt_n"; then
      nt_miss=$((nt_miss + 1))
      [ -z "$nt_first" ] && nt_first=${nt_line%%:*}
    fi
  done <"$nt_l"
  rm -f "$nt_l"
  if [ "$nt_files" -eq 0 ]; then echo "no Go file has a Derived from line"; return 2; fi
  if [ "$nt_miss" -gt 0 ]; then echo "files=$nt_files, $nt_miss lines absent from go/NOTICE.txt, first in $nt_first"; return 1; fi
  if [ "$nt_files" -lt 12 ]; then echo "files=$nt_files, fewer than 12"; return 1; fi
  echo "files=$nt_files, every line is in go/NOTICE.txt"
}

nt3_check() {
  nt_n="$1/go/NOTICE.txt"
  if [ ! -f "$nt_n" ]; then echo "go/NOTICE.txt is absent"; return 1; fi
  nt_att=$(sed -n 's/^Attribution: //p' "$nt_n" | head -1)
  if [ -z "$nt_att" ]; then echo "go/NOTICE.txt has no Attribution line"; return 1; fi
  nt_l=$(mktemp /tmp/rows/nt3.XXXXXX)
  grep -E '^  [a-z][A-Za-z0-9_/.-]*\.go  +ecs/' "$nt_n" >"$nt_l"
  nt_cnt=0
  nt_bad=0
  nt_why=
  while IFS= read -r nt_line; do
    nt_cnt=$((nt_cnt + 1))
    nt_p=$(printf '%s\n' "$nt_line" | awk '{print $1}')
    nt_file="$1/go/$nt_p"
    if [ ! -f "$nt_file" ]; then nt_bad=$((nt_bad + 1)); nt_why="$nt_why $nt_p absent;"; continue; fi
    if ! grep -Fxq -- "// $nt_att" "$nt_file"; then nt_bad=$((nt_bad + 1)); nt_why="$nt_why $nt_p lacks the attribution line;"; continue; fi
    for nt_tok in $(printf '%s\n' "$nt_line" | grep -o 'ecs/[A-Za-z0-9_]*\.go'); do
      if ! grep -Fq -- "$nt_tok" "$nt_file"; then nt_bad=$((nt_bad + 1)); nt_why="$nt_why $nt_p does not name $nt_tok;"; fi
    done
  done <"$nt_l"
  rm -f "$nt_l"
  if ! grep -Fq 'Copyright (c) 2025 Martin Lange' "$nt_n"; then nt_bad=$((nt_bad + 1)); nt_why="$nt_why the Ark copyright line is absent;"; fi
  if ! grep -Fq 'Permission is hereby granted, free of charge' "$nt_n"; then nt_bad=$((nt_bad + 1)); nt_why="$nt_why the MIT permission sentence is absent;"; fi
  if [ "$nt_cnt" -eq 0 ]; then echo "go/NOTICE.txt lists no file"; return 1; fi
  if [ "$nt_bad" -gt 0 ]; then echo "listed=$nt_cnt, problems:$nt_why"; return 1; fi
  echo "listed=$nt_cnt, each exists, carries the attribution line and names its source files"
}

nt4_check() {
  nt_m="$1/go/go.mod"
  if [ ! -f "$nt_m" ]; then echo "go/go.mod is absent"; return 1; fi
  nt_golines=$(grep -c '^go ' "$nt_m")
  nt_tc=$(grep -c '^toolchain' "$nt_m")
  nt_req=$(awk '/^require \(/ {b = 1; next} b && /^\)/ {b = 0; next} b {print $1 " " $2; next} /^require / {print $2 " " $3}' "$nt_m" | tr '\n' ';')
  if [ "$nt_golines" -ne 1 ] || ! grep -Fxq 'go 1.27.4' "$nt_m"; then echo "the go line is not exactly go 1.27.4"; return 1; fi
  if [ "$nt_tc" -ne 0 ]; then echo "a toolchain line is present"; return 1; fi
  if [ "$nt_req" != "github.com/fiberfx/ecd-id/go v0.8.0;" ]; then echo "requirements: $nt_req"; return 1; fi
  echo "go 1.27.4, no toolchain line, requires only github.com/fiberfx/ecd-id/go v0.8.0"
}

# nt6_check ROOT: LICENSE and go/LICENSE are present and byte-equal, and begin with the MIT License title and the
# copyright line of Ecd Runtime. A tree without go/LICENSE is red; a tree without LICENSE could not be measured, as the
# run's archive holds LICENSE only when its paths name it.
nt6_check() {
  if [ ! -f "$1/go/LICENSE" ]; then echo "go/LICENSE is absent"; return 1; fi
  if [ ! -f "$1/LICENSE" ]; then echo "LICENSE is not in the tree under test"; return 2; fi
  if ! cmp -s "$1/LICENSE" "$1/go/LICENSE"; then echo "LICENSE and go/LICENSE differ"; return 1; fi
  if [ "$(sed -n 1p "$1/LICENSE")" != "MIT License" ]; then echo "line 1 of LICENSE is not MIT License"; return 1; fi
  if [ "$(sed -n 3p "$1/LICENSE")" != "Copyright (c) 2026 Jonnify" ]; then
    echo "line 3 of LICENSE is not the copyright line of Ecd Runtime"; return 1
  fi
  echo "LICENSE and go/LICENSE are byte-equal, MIT License, Copyright (c) 2026 Jonnify"
}

# nt5_tidy GODIR LOG: go mod tidy, offline, in a copy of GODIR; 0 when go.mod and go.sum stay byte-equal.
nt5_tidy() {
  nt_t=$(mktemp -d /tmp/rows/tidy.XXXXXX) || return 2
  cp -a "$1" "$nt_t/go" || { rm -rf "$nt_t"; echo "the copy failed"; return 2; }
  (cd "$nt_t/go" && go mod tidy) >"$2" 2>&1
  nt_rc=$?
  if offline "$2"; then rm -rf "$nt_t"; echo "module not found"; return 2; fi
  if [ $nt_rc -ne 0 ]; then rm -rf "$nt_t"; echo "go mod tidy rc=$nt_rc"; return 1; fi
  nt_diff=
  cmp -s "$1/go.mod" "$nt_t/go/go.mod" || nt_diff="$nt_diff go.mod"
  cmp -s "$1/go.sum" "$nt_t/go/go.sum" || nt_diff="$nt_diff go.sum"
  rm -rf "$nt_t"
  if [ -n "$nt_diff" ]; then echo "go mod tidy changed:$nt_diff"; return 1; fi
  echo "go mod tidy rc=0, go.mod and go.sum unchanged"
}

# NT1
if sel NT1 notice; then
  nt_o=$(nt1_check /w); nt_r=$?
  say NT1 "no path, Go file or README names $nt_tp" "$nt_o" "$([ $nt_r -eq 0 ] && echo PASS || { [ $nt_r -eq 1 ] && echo FAIL || echo CNM; })"
fi
if sel NT1p notice; then
  if nt_mk; then
    mkdir -p "$nt_root/go/$nt_tp/ark" && printf 'x\n' >"$nt_root/go/$nt_tp/ark/SOURCE.txt"
    nt_o=$(nt1_check "$nt_root"); nt_r=$?
    say NT1p "a file added below go/$nt_tp turns NT1 red" "$nt_o" "$(nt_verdict $nt_r)"
  else
    say NT1p "a file added below go/$nt_tp turns NT1 red" "the copy failed" CNM
  fi
  rm -rf "$nt_root"
fi
if sel NT1r notice; then
  if nt_mk; then
    rm -f "$nt_root/README.md"
    nt_o=$(nt1_check "$nt_root"); nt_r=$?
    say NT1r "a tree without README.md makes NT1 answer could-not-measure" "$nt_o" "$([ $nt_r -eq 2 ] && echo KILLED || { [ $nt_r -eq 0 ] && echo SURVIVED || echo CNM; })"
  else
    say NT1r "a tree without README.md makes NT1 answer could-not-measure" "the copy failed" CNM
  fi
  rm -rf "$nt_root"
fi

# NT2
if sel NT2 notice; then
  nt_o=$(nt2_check /w); nt_r=$?
  say NT2 "every Derived from line of a Go file is in go/NOTICE.txt, in at least 12 files" "$nt_o" "$([ $nt_r -eq 0 ] && echo PASS || { [ $nt_r -eq 1 ] && echo FAIL || echo CNM; })"
fi
if sel NT2p notice; then
  if nt_mk; then
    nt_f="$nt_root/go/engine/jacquard/pool.go"
    cp "$nt_f" "$nt_root/pool.orig"
    sed -i '1s#^// Derived from Ark.*$#// Derived from Volt (github.com/akmonengine/volt) at commit d6236fe. Copyright 2024, Agoune Ramine. Apache License 2.0.#' "$nt_f"
    nt_n=$(nt_changed "$nt_root/pool.orig" "$nt_f")
    if [ "$nt_n" -ne 1 ]; then
      say NT2p "the attribution line of pool.go names another project, NT2 turns red" "the plant changed $nt_n lines" CNM
    else
      nt_o=$(nt2_check "$nt_root"); nt_r=$?
      say NT2p "the attribution line of pool.go names another project, NT2 turns red" "$nt_o" "$(nt_verdict $nt_r)"
    fi
  else
    say NT2p "the attribution line of pool.go names another project, NT2 turns red" "the copy failed" CNM
  fi
  rm -rf "$nt_root"
fi
if sel NT2q notice; then
  if nt_mk; then
    cp "$nt_root/go/NOTICE.txt" "$nt_root/notice.orig"
    grep -v '^Attribution: Derived from Ark' "$nt_root/notice.orig" >"$nt_root/go/NOTICE.txt"
    nt_n=$(nt_changed "$nt_root/notice.orig" "$nt_root/go/NOTICE.txt")
    if [ "$nt_n" -ne 1 ]; then
      say NT2q "a go/NOTICE.txt without the Ark attribution line turns NT2 red" "the plant changed $nt_n lines" CNM
    else
      nt_o=$(nt2_check "$nt_root"); nt_r=$?
      say NT2q "a go/NOTICE.txt without the Ark attribution line turns NT2 red" "$nt_o" "$(nt_verdict $nt_r)"
    fi
  else
    say NT2q "a go/NOTICE.txt without the Ark attribution line turns NT2 red" "the copy failed" CNM
  fi
  rm -rf "$nt_root"
fi

# NT3
if sel NT3 notice; then
  nt_o=$(nt3_check /w); nt_r=$?
  say NT3 "every file listed in go/NOTICE.txt exists and carries its attribution; the Ark copyright line and the MIT permission sentence are present" "$nt_o" "$([ $nt_r -eq 0 ] && echo PASS || { [ $nt_r -eq 1 ] && echo FAIL || echo CNM; })"
fi
if sel NT3p notice; then
  if nt_mk; then
    cp "$nt_root/go/NOTICE.txt" "$nt_root/notice.orig"
    sed -i 's#^  engine/jacquard/table\.go  #  engine/jacquard/absent.go #' "$nt_root/go/NOTICE.txt"
    nt_n=$(nt_changed "$nt_root/notice.orig" "$nt_root/go/NOTICE.txt")
    if [ "$nt_n" -ne 1 ]; then
      say NT3p "a go/NOTICE.txt that lists engine/jacquard/absent.go turns NT3 red" "the plant changed $nt_n lines" CNM
    else
      nt_o=$(nt3_check "$nt_root"); nt_r=$?
      say NT3p "a go/NOTICE.txt that lists engine/jacquard/absent.go turns NT3 red" "$nt_o" "$(nt_verdict $nt_r)"
    fi
  else
    say NT3p "a go/NOTICE.txt that lists engine/jacquard/absent.go turns NT3 red" "the copy failed" CNM
  fi
  rm -rf "$nt_root"
fi

# NT4
if sel NT4 notice; then
  nt_o=$(nt4_check /w); nt_r=$?
  say NT4 "go.mod has go 1.27.4, no toolchain line and the one requirement ecd-id v0.8.0" "$nt_o" "$([ $nt_r -eq 0 ] && echo PASS || { [ $nt_r -eq 1 ] && echo FAIL || echo CNM; })"
fi
if sel NT4p notice; then
  if nt_mk; then
    cp "$nt_root/go/go.mod" "$nt_root/gomod.orig"
    printf 'toolchain go1.27.4\n' >>"$nt_root/go/go.mod"
    nt_n=$(nt_changed "$nt_root/gomod.orig" "$nt_root/go/go.mod")
    if [ "$nt_n" -ne 1 ]; then
      say NT4p "a toolchain line in go.mod turns NT4 red" "the plant changed $nt_n lines" CNM
    else
      nt_o=$(nt4_check "$nt_root"); nt_r=$?
      say NT4p "a toolchain line in go.mod turns NT4 red" "$nt_o" "$(nt_verdict $nt_r)"
    fi
  else
    say NT4p "a toolchain line in go.mod turns NT4 red" "the copy failed" CNM
  fi
  rm -rf "$nt_root"
fi
if sel NT4q notice; then
  if nt_mk; then
    cp "$nt_root/go/go.mod" "$nt_root/gomod.orig"
    sed -i 's/^go 1\.27\.4$/go 1.27/' "$nt_root/go/go.mod"
    nt_n=$(nt_changed "$nt_root/gomod.orig" "$nt_root/go/go.mod")
    if [ "$nt_n" -ne 1 ]; then
      say NT4q "the go line go 1.27 in go.mod turns NT4 red" "the plant changed $nt_n lines" CNM
    else
      nt_o=$(nt4_check "$nt_root"); nt_r=$?
      say NT4q "the go line go 1.27 in go.mod turns NT4 red" "$nt_o" "$(nt_verdict $nt_r)"
    fi
  else
    say NT4q "the go line go 1.27 in go.mod turns NT4 red" "the copy failed" CNM
  fi
  rm -rf "$nt_root"
fi

# NT5
if sel NT5 notice; then
  if [ $HAVE_GO -eq 0 ]; then
    say NT5 "go mod tidy leaves go.mod and go.sum byte-equal" "go not on PATH" CNM
  else
    nt_o=$(nt5_tidy /w/go /tmp/rows/NT5.log); nt_r=$?
    say NT5 "go mod tidy leaves go.mod and go.sum byte-equal" "$nt_o" "$([ $nt_r -eq 0 ] && echo PASS || { [ $nt_r -eq 1 ] && echo FAIL || echo CNM; })"
    [ $nt_r -ne 0 ] && head_of /tmp/rows/NT5.log 8
  fi
fi
if sel NT5p notice; then
  if [ $HAVE_GO -eq 0 ]; then
    say NT5p "a go.sum without the go.mod line of the requirement differs after go mod tidy" "go not on PATH" CNM
  elif nt_mk; then
    cp "$nt_root/go/go.sum" "$nt_root/gosum.orig"
    sed -i '/\/go\.mod h1:/d' "$nt_root/go/go.sum"
    nt_n=$(nt_changed "$nt_root/gosum.orig" "$nt_root/go/go.sum")
    if [ "$nt_n" -ne 1 ]; then
      say NT5p "a go.sum without the go.mod line of the requirement differs after go mod tidy" "the plant changed $nt_n lines" CNM
    else
      nt_o=$(nt5_tidy "$nt_root/go" /tmp/rows/NT5p.log); nt_r=$?
      say NT5p "a go.sum without the go.mod line of the requirement differs after go mod tidy" "$nt_o" "$(nt_verdict $nt_r)"
    fi
  else
    say NT5p "a go.sum without the go.mod line of the requirement differs after go mod tidy" "the copy failed" CNM
  fi
  rm -rf "$nt_root"
fi

# NT6
if sel NT6 notice; then
  nt_o=$(nt6_check /w); nt_r=$?
  say NT6 "LICENSE and go/LICENSE are byte-equal and carry the MIT License and the copyright line of Ecd Runtime" "$nt_o" "$([ $nt_r -eq 0 ] && echo PASS || { [ $nt_r -eq 1 ] && echo FAIL || echo CNM; })"
fi
if sel NT6p notice; then
  if nt_mk; then
    rm -f "$nt_root/go/LICENSE"
    nt_o=$(nt6_check "$nt_root"); nt_r=$?
    say NT6p "a tree without go/LICENSE turns NT6 red" "$nt_o" "$(nt_verdict $nt_r)"
  else
    say NT6p "a tree without go/LICENSE turns NT6 red" "the copy failed" CNM
  fi
  rm -rf "$nt_root"
fi
if sel NT6q notice; then
  if nt_mk; then
    cp "$nt_root/go/LICENSE" "$nt_root/license.orig"
    sed -i '3s/Jonnify/Jonnify Ltd/' "$nt_root/go/LICENSE"
    nt_n=$(nt_changed "$nt_root/license.orig" "$nt_root/go/LICENSE")
    if [ "$nt_n" -ne 1 ]; then
      say NT6q "a copyright line of go/LICENSE that differs from LICENSE turns NT6 red" "the plant changed $nt_n lines" CNM
    else
      nt_o=$(nt6_check "$nt_root"); nt_r=$?
      say NT6q "a copyright line of go/LICENSE that differs from LICENSE turns NT6 red" "$nt_o" "$(nt_verdict $nt_r)"
    fi
  else
    say NT6q "a copyright line of go/LICENSE that differs from LICENSE turns NT6 red" "the copy failed" CNM
  fi
  rm -rf "$nt_root"
fi
if sel NT6r notice; then
  if nt_mk; then
    rm -f "$nt_root/LICENSE"
    nt_o=$(nt6_check "$nt_root"); nt_r=$?
    say NT6r "a tree without LICENSE makes NT6 answer could-not-measure" "$nt_o" "$([ $nt_r -eq 2 ] && echo KILLED || { [ $nt_r -eq 0 ] && echo SURVIVED || echo CNM; })"
  else
    say NT6r "a tree without LICENSE makes NT6 answer could-not-measure" "the copy failed" CNM
  fi
  rm -rf "$nt_root"
fi
