package ecr_test

import (
	"errors"
	"fmt"
	"log"
	"os"
	"path/filepath"

	"github.com/fiberfx/ecd-runtime/go/ecr"
)

// Example_spawned names the entities that a flush spawned by their placeholders, with no read of the store's log.
func Example_spawned() {
	dir, err := os.MkdirTemp("", "ecr-spawned")
	if err != nil {
		log.Fatal(err)
	}
	defer os.RemoveAll(dir)
	h, err := ecr.Open(ecr.Options{
		Path:     filepath.Join(dir, "app.db"),
		Create:   true,
		Catalog:  appCatalog(),
		Minter:   &appMinter{},
		DeedKind: appDeed,
	})
	if err != nil {
		log.Fatal(err)
	}
	defer h.Close()

	signup, err := h.Register(0, "signup")
	if err != nil {
		log.Fatal(err)
	}
	ada := signup.Spawn(user)
	signup.Set(ada, handle, []byte("ada-lace"))
	bob := signup.Spawn(user)
	signup.Set(bob, handle, []byte("bob-byte"))
	res, err := h.Flush()
	if err != nil {
		log.Fatal(err)
	}
	for _, p := range []ecr.Ref{ada, bob} {
		id, err := res.Spawned(p) // the eid minted for the spawn of p
		if err != nil {
			log.Fatal(err)
		}
		v, err := h.Engine.Get(id, handle, nil)
		if err != nil {
			log.Fatal(err)
		}
		fmt.Printf("%s %s\n", ecr.KindOf(id).Kind, v)
	}
	_, err = res.Spawned(ecr.Ref{})
	fmt.Println("zero ref refused:", errors.Is(err, ecr.ErrRef))
	// Output:
	// usr ada-lace
	// usr bob-byte
	// zero ref refused: true
}
