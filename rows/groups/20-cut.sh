# Group cut of Ecd Loom: AK1, AK2 and AK2p.
# Sourced by rows/loom.rows.sh from /w; the harness supplies say, sel, offline, head_of, count, tree_of, go_row and HAVE_GO.
go_row AK1 cut "go test ./... rc=0 in third_party/ark" /w/go/third_party/ark test ./...
if sel AK2 cut || sel AK2p cut; then
  want=$(sed -n 's/^Tree of ecs\/: *\([0-9a-f]\{40\}\).*/\1/p' /w/go/third_party/ark/SOURCE.txt)
  if [ -z "$want" ]; then
    sel AK2 cut && say AK2 "the tree hash of ecs/ from SOURCE.txt" "no hash in third_party/ark/SOURCE.txt" CNM
    sel AK2p cut && say AK2p "the hash comparison turns red on a changed file" "no hash in third_party/ark/SOURCE.txt" CNM
  else
    if sel AK2 cut; then
      got=$(tree_of /w/go/third_party/ark/ecs 2>/tmp/rows/AK2.err)
      if [ -z "$got" ]; then say AK2 "tree hash of third_party/ark/ecs = $want" "git could not compute the hash" CNM; head_of /tmp/rows/AK2.err 6
      elif [ "$got" = "$want" ]; then say AK2 "tree hash of third_party/ark/ecs = $want (SOURCE.txt)" "$got" PASS
      else say AK2 "tree hash of third_party/ark/ecs = $want (SOURCE.txt)" "$got" FAIL; fi
    fi
    if sel AK2p cut; then
      pd=$(mktemp -d /tmp/rows/plant.XXXXXX)
      cp -a /w/go/third_party/ark/ecs "$pd/ecs" && printf 'x' >>"$pd/ecs/entity.go"
      got=$(tree_of "$pd/ecs" 2>/tmp/rows/AK2p.err)
      rm -rf "$pd"
      if [ -z "$got" ]; then say AK2p "a byte appended to ecs/entity.go changes the hash" "git could not compute the hash" CNM
      elif [ "$got" = "$want" ]; then say AK2p "a byte appended to ecs/entity.go changes the hash" "hash unchanged: $got" SURVIVED
      else say AK2p "a byte appended to ecs/entity.go changes the hash" "hash $got differs from $want" KILLED; fi
    fi
  fi
fi
