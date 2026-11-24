package ecr_test

import (
	"errors"
	"fmt"
	"io/fs"
	"log"
	"os"
	"path/filepath"

	"github.com/fiberfx/ecd-runtime/go/ecr"
	"github.com/fiberfx/ecd-runtime/go/ecr/ecrtest"
)

// The examples of README.md at the root of the repository. Each Go block of the README is the code of this file:
// the quick start is Example, with main renamed, and each common task is the Example that its section names.

// The names of the examples: system app, its kind usr (a user) and component handle, 8 bytes. Deeds are of kind ded.
var (
	appSys  = ecr.MustCode("app")
	user    = ecr.Kind{System: appSys, Kind: ecr.MustCode("usr")}
	appDeed = ecr.MustCode("ded")
)

const handle ecr.CID = 1

// appCatalog declares kind usr, whose entities the engine keeps resident once written, and component handle.
func appCatalog() *ecr.Catalog {
	return &ecr.Catalog{
		Components: []ecr.ComponentDef{{CID: handle, Name: "handle", Form: ecr.Component, Size: 8}},
		Kinds:      []ecr.KindDef{{Kind: user, Residency: ecr.Touched, Budget: 1 << 20}},
	}
}

// appMinter stands in for the host's Minter: it mints the eids of system app, writer 1, at one fixed time. A real
// host holds one *eid.Minter of its lease, from github.com/fiberfx/ecd-id/go, and passes it in.
type appMinter struct{ count uint32 }

func (m *appMinter) Mint(kind ecr.Code) (ecr.ID, error) {
	m.count++
	return ecr.FromParts(appSys, kind, 1795271400000, 1, m.count)
}

// Example is the quick start of the README.
func Example() {
	dir, err := os.MkdirTemp("", "ecr-quickstart") // a real host keeps its store at a fixed path
	if err != nil {
		log.Fatal(err)
	}
	defer os.RemoveAll(dir)

	h, err := ecr.Open(ecr.Options{
		Path:     filepath.Join(dir, "app.db"),
		Create:   true, // make the file; a later start leaves Create false
		Catalog:  appCatalog(),
		Minter:   &appMinter{},
		DeedKind: appDeed,
	})
	if err != nil {
		log.Fatal(err)
	}
	defer h.Close()

	// A system records its writes in its own buffer.
	signup, err := h.Register(0, "signup")
	if err != nil {
		log.Fatal(err)
	}
	u := signup.Spawn(user) // a placeholder for the new entity
	signup.Set(u, handle, []byte("ada-lace"))

	// Flush commits the buffers to the store, then applies what committed to the engine.
	res, err := h.Flush()
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println("seq", res.Seq, "committed", res.Outcomes[0] == ecr.Committed)

	// The eid of the new entity is in the committed deed. Read its value back from the engine.
	var id ecr.ID
	err = h.Store.Log(res.Seq-1, func(seq uint64, d ecr.Deed) error {
		id = d.Ops[0].Entity // the spawn comes first
		return nil
	})
	if err != nil {
		log.Fatal(err)
	}
	v, err := h.Engine.Get(id, handle, nil)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Printf("%s %s\n", ecr.KindOf(id).Kind, v)
	// Output:
	// seq 1 committed true
	// usr ada-lace
}

// start opens the store at path. The first run makes the file; every later run first calls Migrate, which brings a
// store written by go/sqlite v0.5.x or earlier to the current schema and leaves a current store as it is.
func start(path string, m ecr.Minter) (*ecr.Host, error) {
	_, err := os.Stat(path)
	create := errors.Is(err, fs.ErrNotExist)
	if !create {
		if err := ecr.Migrate(path); err != nil {
			return nil, err // matches ecr.ErrSchema when the file is not an owner store
		}
	}
	return ecr.Open(ecr.Options{Path: path, Create: create, Catalog: appCatalog(), Minter: m, DeedKind: appDeed})
}

// ExampleMigrate runs start twice: the first run makes the store and commits a deed, the second migrates and opens it.
func ExampleMigrate() {
	dir, err := os.MkdirTemp("", "ecr-migrate")
	if err != nil {
		log.Fatal(err)
	}
	defer os.RemoveAll(dir)
	path := filepath.Join(dir, "app.db")
	m := &appMinter{} // one Minter for the life of the process

	h, err := start(path, m)
	if err != nil {
		log.Fatal(err)
	}
	signup, err := h.Register(0, "signup")
	if err != nil {
		log.Fatal(err)
	}
	signup.Set(signup.Spawn(user), handle, []byte("ada-lace"))
	if _, err := h.Flush(); err != nil {
		log.Fatal(err)
	}
	if err := h.Close(); err != nil {
		log.Fatal(err)
	}

	h, err = start(path, m)
	if err != nil {
		log.Fatal(err)
	}
	defer h.Close()
	fmt.Println("engine at seq", h.Engine.At())
	// Output:
	// engine at seq 1
}

// Example_span attaches the trace id and span id of a request to a deed.
func Example_span() {
	dir, err := os.MkdirTemp("", "ecr-span")
	if err != nil {
		log.Fatal(err)
	}
	defer os.RemoveAll(dir)
	h, err := ecr.Open(ecr.Options{Path: filepath.Join(dir, "app.db"), Create: true, Catalog: appCatalog(),
		Minter: &appMinter{}, DeedKind: appDeed})
	if err != nil {
		log.Fatal(err)
	}
	defer h.Close()
	signup, err := h.Register(0, "signup")
	if err != nil {
		log.Fatal(err)
	}

	// The W3C ids of the request that caused the write, as in its traceparent header.
	trace, err := ecr.ParseTraceID("4bf92f3577b34da6a3ce929d0e0e4736")
	if err != nil {
		log.Fatal(err)
	}
	span, err := ecr.ParseSpanID("00f067aa0ba902b7")
	if err != nil {
		log.Fatal(err)
	}
	signup.Span(trace, span) // the next deed of this buffer carries these ids
	signup.Set(signup.Spawn(user), handle, []byte("ada-lace"))
	if _, err := h.Flush(); err != nil {
		log.Fatal(err)
	}

	// The flush reset the ids, so this deed carries none.
	signup.Set(signup.Spawn(user), handle, []byte("bob-byte"))
	if _, err := h.Flush(); err != nil {
		log.Fatal(err)
	}

	// The store keeps each deed's ids with it; the zero values mean none.
	err = h.Store.Log(0, func(seq uint64, d ecr.Deed) error {
		fmt.Println(seq, d.TraceID, d.SpanID)
		return nil
	})
	if err != nil {
		log.Fatal(err)
	}
	// Output:
	// 1 4bf92f3577b34da6a3ce929d0e0e4736 00f067aa0ba902b7
	// 2 00000000000000000000000000000000 0000000000000000
}

// Example_model opens a Host on the reference engine of tests, as the test of a host does.
func Example_model() {
	dir, err := os.MkdirTemp("", "ecr-model") // in a test, t.TempDir()
	if err != nil {
		log.Fatal(err)
	}
	defer os.RemoveAll(dir)
	h, err := ecr.Open(ecr.Options{Path: filepath.Join(dir, "app.db"), Create: true, Catalog: appCatalog(),
		Minter: &appMinter{}, DeedKind: appDeed, Engine: ecrtest.NewModel})
	if err != nil {
		log.Fatal(err)
	}
	defer h.Close()
	signup, err := h.Register(0, "signup")
	if err != nil {
		log.Fatal(err)
	}
	signup.Set(signup.Spawn(user), handle, []byte("ada-lace"))
	if _, err := h.Flush(); err != nil {
		log.Fatal(err)
	}
	fmt.Println(h.Engine.Name(), "at seq", h.Engine.At())
	// Output:
	// model at seq 1
}

// Example_errors shows the errors of Open and Flush and the outcome of a deed that the store refuses.
func Example_errors() {
	dir, err := os.MkdirTemp("", "ecr-errors")
	if err != nil {
		log.Fatal(err)
	}
	defer os.RemoveAll(dir)
	path := filepath.Join(dir, "app.db")
	o := ecr.Options{Path: path, Create: true, Catalog: appCatalog(), Minter: &appMinter{}, DeedKind: appDeed}

	// Options that miss a required field are refused before any file is made.
	bad := o
	bad.Minter = nil
	_, err = ecr.Open(bad)
	fmt.Println("no Minter:", errors.Is(err, ecr.ErrConfig))

	h, err := ecr.Open(o)
	if err != nil {
		log.Fatal(err)
	}
	defer h.Close()

	// Create refuses a path at which anything exists.
	_, err = ecr.Open(o)
	fmt.Println("exists:", errors.Is(err, ecr.ErrExist))

	// Open refuses a file that is not an owner store of the current schema.
	empty := filepath.Join(dir, "empty.db")
	if err := os.WriteFile(empty, nil, 0o600); err != nil {
		log.Fatal(err)
	}
	_, err = ecr.Open(ecr.Options{Path: empty, Catalog: appCatalog(), Minter: &appMinter{}, DeedKind: appDeed})
	fmt.Println("not a store:", errors.Is(err, ecr.ErrSchema))

	// A deed that the store does not allow is not an error: its Outcome says so, and nothing is committed.
	rename, err := h.Register(0, "rename")
	if err != nil {
		log.Fatal(err)
	}
	nobody, err := ecr.FromParts(appSys, user.Kind, 1795271400000, 2, 1) // an eid of no entity
	if err != nil {
		log.Fatal(err)
	}
	rename.Set(ecr.Of(nobody), handle, []byte("nobody.."))
	res, err := h.Flush()
	fmt.Println("refused:", err == nil && res.Outcomes[0] == ecr.Refused, "seq", res.Seq)

	// A record that names no entity refuses the whole flush.
	rename.Set(ecr.Of(ecr.ID{}), handle, []byte("nobody.."))
	_, err = h.Flush()
	fmt.Println("no entity:", errors.Is(err, ecr.ErrRef))
	// Output:
	// no Minter: true
	// exists: true
	// not a store: true
	// refused: true seq 0
	// no entity: true
}
