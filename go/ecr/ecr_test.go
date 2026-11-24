package ecr_test

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	eid "github.com/fiberfx/ecd-id/go"
	"github.com/fiberfx/ecd-runtime/go/ecd"
	"github.com/fiberfx/ecd-runtime/go/ecr"
	"github.com/fiberfx/ecd-runtime/go/engine/jacquard"
	"github.com/fiberfx/ecd-runtime/go/rt"
	"github.com/fiberfx/ecd-runtime/go/sqlite"
)

// The tests name entities of system tst: kind aaa is an All kind and kind bbb a Touched kind, and component 1 holds 8
// bytes. The deeds are of kind ded, which is registered nowhere, as in the tests of go/sqlite.
var (
	sysTst    = ecr.MustCode("tst")
	kindAll   = ecr.Kind{System: sysTst, Kind: ecr.MustCode("aaa")}
	kindTouch = ecr.Kind{System: sysTst, Kind: ecr.MustCode("bbb")}
	deedKind  = ecr.MustCode("ded")
)

const (
	cidOne ecr.CID = 1

	startMs   = 1795271400000 // the time of the first eid a countMinter mints
	traceText = "4bf92f3577b34da6a3ce929d0e0e4736"
	spanText  = "00f067aa0ba902b7"
)

// catalog is the catalog of the tests: kinds aaa (All) and bbb (Touched) and component 1.
func catalog() *ecr.Catalog {
	return &ecr.Catalog{
		Components: []ecr.ComponentDef{{CID: cidOne, Name: "one", Form: ecr.Component, Size: 8}},
		Kinds: []ecr.KindDef{
			{Kind: kindAll, Residency: ecr.All},
			{Kind: kindTouch, Residency: ecr.Touched, Budget: 1 << 20},
		},
	}
}

// countMinter is a Minter whose eids are those of system tst, writer 1, a time that starts at startMs and advances by
// 1 ms every 1,000 mints, and a count from 0 to 999 within each millisecond, as in the tests of go/sqlite.
type countMinter struct{ calls int }

func (m *countMinter) Mint(kind ecr.Code) (ecr.ID, error) {
	ms := uint64(startMs) + uint64(m.calls/1000)
	n := uint32(m.calls % 1000)
	m.calls++
	return ecr.FromParts(sysTst, kind, ms, 1, n)
}

// nopCommitter is a Committer that commits nothing: it answers every call with 0 and no error.
type nopCommitter struct{}

func (nopCommitter) Commit([]ecr.Deed, []ecr.Outcome) (uint64, error) { return 0, nil }

// options returns the Options of a store file in a new temporary directory, with Create set and the default engine.
func options(t *testing.T) ecr.Options {
	t.Helper()
	return ecr.Options{Path: filepath.Join(t.TempDir(), "owner.db"), Create: true, Catalog: catalog(),
		Minter: &countMinter{}, DeedKind: deedKind}
}

// absent fails the test when anything is at path.
func absent(t *testing.T, path string) {
	t.Helper()
	if _, err := os.Lstat(path); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("Lstat(%s): %v; want nothing at the path", path, err)
	}
}

// storeOpen reports whether the store answers Log, which a closed store refuses.
func storeOpen(s *ecr.Store) bool {
	return s.Log(0, func(uint64, ecr.Deed) error { return nil }) == nil
}

// named is an engine that answers Name with its name and counts its Close calls, which call onClose when it is not nil,
// close the engine it holds and answer closeErr. Its other calls are those of the engine it holds.
type named struct {
	ecr.Engine
	name     string
	closes   int
	closeErr error
	onClose  func()
}

func (n *named) Name() string { return n.name }

func (n *named) Close() error {
	n.closes++
	if n.onClose != nil {
		n.onClose()
	}
	n.Engine.Close()
	return n.closeErr
}

// namedEngine returns an Engine func that builds Jacquard inside a named engine and keeps the named engine in *got.
func namedEngine(name string, closeErr error, got **named) func(*ecr.Catalog) (ecr.Engine, error) {
	return func(c *ecr.Catalog) (ecr.Engine, error) {
		e, err := jacquard.New(c)
		if err != nil {
			return nil, err
		}
		*got = &named{Engine: e, name: name, closeErr: closeErr}
		return *got, nil
	}
}

// TestAliases holds that every type of ecr is the type of its package, every sentinel the error of its package,
// every constant the constant of its package, with its type and value, and every function the answer of the function
// it names, Migrate included: it leaves an owner store as it is, refuses a file that is not an owner store with
// ErrSchema, and refuses a missing path without making a file.
func TestAliases(t *testing.T) {
	t.Run("types", func(t *testing.T) {
		for _, c := range []struct {
			name      string
			got, want reflect.Type
		}{
			{"ID", reflect.TypeFor[ecr.ID](), reflect.TypeFor[eid.ID]()},
			{"Code", reflect.TypeFor[ecr.Code](), reflect.TypeFor[eid.Code]()},
			{"TraceID", reflect.TypeFor[ecr.TraceID](), reflect.TypeFor[ecd.TraceID]()},
			{"SpanID", reflect.TypeFor[ecr.SpanID](), reflect.TypeFor[ecd.SpanID]()},
			{"Deed", reflect.TypeFor[ecr.Deed](), reflect.TypeFor[ecd.Deed]()},
			{"Op", reflect.TypeFor[ecr.Op](), reflect.TypeFor[ecd.Op]()},
			{"OpCode", reflect.TypeFor[ecr.OpCode](), reflect.TypeFor[ecd.OpCode]()},
			{"Kind", reflect.TypeFor[ecr.Kind](), reflect.TypeFor[ecd.Kind]()},
			{"Range", reflect.TypeFor[ecr.Range](), reflect.TypeFor[ecd.Range]()},
			{"CID", reflect.TypeFor[ecr.CID](), reflect.TypeFor[ecd.CID]()},
			{"QID", reflect.TypeFor[ecr.QID](), reflect.TypeFor[ecd.QID]()},
			{"Catalog", reflect.TypeFor[ecr.Catalog](), reflect.TypeFor[ecd.Catalog]()},
			{"KindDef", reflect.TypeFor[ecr.KindDef](), reflect.TypeFor[ecd.KindDef]()},
			{"ComponentDef", reflect.TypeFor[ecr.ComponentDef](), reflect.TypeFor[ecd.ComponentDef]()},
			{"Form", reflect.TypeFor[ecr.Form](), reflect.TypeFor[ecd.Form]()},
			{"QueryDef", reflect.TypeFor[ecr.QueryDef](), reflect.TypeFor[ecd.QueryDef]()},
			{"Residency", reflect.TypeFor[ecr.Residency](), reflect.TypeFor[ecd.Residency]()},
			{"State", reflect.TypeFor[ecr.State](), reflect.TypeFor[ecd.State]()},
			{"Engine", reflect.TypeFor[ecr.Engine](), reflect.TypeFor[ecd.Engine]()},
			{"Minter", reflect.TypeFor[ecr.Minter](), reflect.TypeFor[ecd.Minter]()},
			{"Answer", reflect.TypeFor[ecr.Answer](), reflect.TypeFor[ecd.Answer]()},
			{"Row", reflect.TypeFor[ecr.Row](), reflect.TypeFor[ecd.Row]()},
			{"RowSource", reflect.TypeFor[ecr.RowSource](), reflect.TypeFor[ecd.RowSource]()},
			{"Stats", reflect.TypeFor[ecr.Stats](), reflect.TypeFor[ecd.Stats]()},
			{"KindStats", reflect.TypeFor[ecr.KindStats](), reflect.TypeFor[ecd.KindStats]()},
			{"Runtime", reflect.TypeFor[ecr.Runtime](), reflect.TypeFor[rt.Runtime]()},
			{"Buffer", reflect.TypeFor[ecr.Buffer](), reflect.TypeFor[rt.Buffer]()},
			{"Ref", reflect.TypeFor[ecr.Ref](), reflect.TypeFor[rt.Ref]()},
			{"Result", reflect.TypeFor[ecr.Result](), reflect.TypeFor[rt.Result]()},
			{"Outcome", reflect.TypeFor[ecr.Outcome](), reflect.TypeFor[rt.Outcome]()},
			{"Committer", reflect.TypeFor[ecr.Committer](), reflect.TypeFor[rt.Committer]()},
			{"Config", reflect.TypeFor[ecr.Config](), reflect.TypeFor[rt.Config]()},
			{"Store", reflect.TypeFor[ecr.Store](), reflect.TypeFor[sqlite.Store]()},
		} {
			if c.got != c.want {
				t.Errorf("ecr.%s is %v; want %v", c.name, c.got, c.want)
			}
		}
		// The violating half: two types of the seam with the same underlying type differ, so the comparison tells an
		// alias from another type.
		if reflect.TypeFor[ecr.QID]() == reflect.TypeFor[ecd.CID]() {
			t.Error("ecr.QID is ecd.CID; want distinct types")
		}
	})

	t.Run("sentinels", func(t *testing.T) {
		for _, c := range []struct {
			name      string
			got, want error
		}{
			{"ErrTraceID", ecr.ErrTraceID, ecd.ErrTraceID},
			{"ErrSpanID", ecr.ErrSpanID, ecd.ErrSpanID},
			{"ErrGap", ecr.ErrGap, ecd.ErrGap},
			{"ErrStale", ecr.ErrStale, ecd.ErrStale},
			{"ErrCold", ecr.ErrCold, ecd.ErrCold},
			{"ErrMiss", ecr.ErrMiss, ecd.ErrMiss},
			{"ErrAbsent", ecr.ErrAbsent, ecd.ErrAbsent},
			{"ErrKind", ecr.ErrKind, ecd.ErrKind},
			{"ErrOp", ecr.ErrOp, ecd.ErrOp},
			{"ErrBudget", ecr.ErrBudget, ecd.ErrBudget},
			{"ErrQuery", ecr.ErrQuery, ecd.ErrQuery},
			{"ErrOwner", ecr.ErrOwner, ecd.ErrOwner},
			{"ErrConfig", ecr.ErrConfig, rt.ErrConfig},
			{"ErrRef", ecr.ErrRef, rt.ErrRef},
			{"ErrExist", ecr.ErrExist, sqlite.ErrExist},
			{"ErrSchema", ecr.ErrSchema, sqlite.ErrSchema},
		} {
			if c.got != c.want {
				t.Errorf("ecr.%s is %v; want the error of its package, %v", c.name, c.got, c.want)
			}
			if err := fmt.Errorf("wrapped: %w", c.want); !errors.Is(err, c.got) {
				t.Errorf("an error that wraps the error of ecr.%s's package does not match ecr.%s", c.name, c.name)
			}
		}
		// The violating half: a new error with the text of rt.ErrRef does not match ecr.ErrRef.
		if errors.Is(errors.New(rt.ErrRef.Error()), ecr.ErrRef) {
			t.Error("a new error with the text of rt.ErrRef matches ecr.ErrRef; want no match")
		}
	})

	t.Run("constants", func(t *testing.T) {
		for _, c := range []struct {
			name      string
			got, want any
		}{
			{"Touched", ecr.Touched, ecd.Touched},
			{"All", ecr.All, ecd.All},
			{"Cold", ecr.Cold, ecd.Cold},
			{"Warm", ecr.Warm, ecd.Warm},
			{"Stale", ecr.Stale, ecd.Stale},
			{"Component", ecr.Component, ecd.Component},
			{"Tag", ecr.Tag, ecd.Tag},
			{"Edge", ecr.Edge, ecd.Edge},
			{"OpSpawn", ecr.OpSpawn, ecd.OpSpawn},
			{"OpSet", ecr.OpSet, ecd.OpSet},
			{"OpUnset", ecr.OpUnset, ecd.OpUnset},
			{"OpDestroy", ecr.OpDestroy, ecd.OpDestroy},
			{"Committed", ecr.Committed, rt.Committed},
			{"Conflict", ecr.Conflict, rt.Conflict},
			{"Refused", ecr.Refused, rt.Refused},
			{"Unknown", ecr.Unknown, rt.Unknown},
		} {
			if c.got != c.want {
				t.Errorf("ecr.%s is %T %v; want %T %v", c.name, c.got, c.got, c.want, c.want)
			}
		}
		// The violating half: constants of one value and two types differ, so the comparison checks the type.
		if any(ecr.Touched) == any(ecr.Committed) {
			t.Error("ecr.Touched equals ecr.Committed; want them distinct by type")
		}
	})

	t.Run("functions", func(t *testing.T) {
		id, err := eid.FromParts(sysTst, kindAll.Kind, startMs, 1, 0)
		if err != nil {
			t.Fatalf("eid.FromParts: %v", err)
		}
		text := id.Encode()
		tr, err := ecd.ParseTraceID(traceText)
		if err != nil {
			t.Fatalf("ecd.ParseTraceID: %v", err)
		}
		sp, err := ecd.ParseSpanID(spanText)
		if err != nil {
			t.Fatalf("ecd.ParseSpanID: %v", err)
		}
		window, err := ecd.Window(kindAll, startMs, startMs+1)
		if err != nil {
			t.Fatalf("ecd.Window: %v", err)
		}
		check := func(name string, got any, err error, want any) {
			t.Helper()
			if err != nil || got != want {
				t.Errorf("ecr.%s: %v, %v; want %v", name, got, err, want)
			}
		}
		v, err := ecr.Parse(text[:])
		check("Parse", v, err, id)
		v, err = ecr.ParseString(string(text[:]))
		check("ParseString", v, err, id)
		v, err = ecr.FromBytes(id[:])
		check("FromBytes", v, err, id)
		v, err = ecr.FromParts(sysTst, kindAll.Kind, startMs, 1, 0)
		check("FromParts", v, err, id)
		code, ok := ecr.ParseCode("tst")
		check("ParseCode", code, nil, sysTst)
		check("ParseCode ok", ok, nil, true)
		check("KindOf", ecr.KindOf(id), nil, kindAll)
		check("KindRange", ecr.KindRange(kindAll), nil, ecd.KindRange(kindAll))
		w, err := ecr.Window(kindAll, startMs, startMs+1)
		check("Window", w, err, window)
		check("Day", ecr.Day(id), nil, ecd.Day(id))
		gt, err := ecr.ParseTraceID(traceText)
		check("ParseTraceID", gt, err, tr)
		gs, err := ecr.ParseSpanID(spanText)
		check("ParseSpanID", gs, err, sp)
		gt, err = ecr.TraceIDFromBytes(tr.Bytes())
		check("TraceIDFromBytes", gt, err, tr)
		gs, err = ecr.SpanIDFromBytes(sp.Bytes())
		check("SpanIDFromBytes", gs, err, sp)
		check("Of", ecr.Of(id), nil, rt.Of(id))
		en, err := jacquard.New(catalog())
		if err != nil {
			t.Fatalf("jacquard.New: %v", err)
		}
		cfg := ecr.Config{Catalog: catalog(), Engine: en, Minter: &countMinter{}, Committer: nopCommitter{},
			DeedKind: deedKind}
		if r, err := ecr.New(cfg); r == nil || err != nil {
			t.Errorf("ecr.New of a full Config: %v, %v; want a Runtime", r, err)
		}
		// The violating half: each function refuses what the function it names refuses, with the same error.
		if r, err := ecr.New(ecr.Config{}); r != nil || !errors.Is(err, ecr.ErrConfig) {
			t.Errorf("ecr.New of the zero Config: %v, %v; want no Runtime and ErrConfig", r, err)
		}
		bad := text
		bad[3] = '-'
		if _, err := ecr.Parse(bad[:]); !errors.Is(err, eid.ErrSeparator) {
			t.Errorf("ecr.Parse of a text without its separator: %v; want eid.ErrSeparator", err)
		}
		if _, ok := ecr.ParseCode("TST"); ok {
			t.Error("ecr.ParseCode(\"TST\") is ok; want it refused")
		}
		if _, err := ecr.Window(kindAll, 2, 1); !errors.Is(err, eid.ErrRange) {
			t.Errorf("ecr.Window(aaa, 2, 1): %v; want eid.ErrRange", err)
		}
		if _, err := ecr.ParseTraceID(strings.Repeat("0", 32)); !errors.Is(err, ecr.ErrTraceID) {
			t.Errorf("ecr.ParseTraceID of zeros: %v; want ErrTraceID", err)
		}
		if _, err := ecr.SpanIDFromBytes(make([]byte, 7)); !errors.Is(err, ecr.ErrSpanID) {
			t.Errorf("ecr.SpanIDFromBytes of 7 bytes: %v; want ErrSpanID", err)
		}
		// Migrate is sqlite.Migrate: it accepts an owner store and refuses what sqlite.Migrate refuses, with its error.
		o := options(t)
		h, err := ecr.Open(o)
		if err != nil {
			t.Fatalf("ecr.Open with Create: %v", err)
		}
		if err := h.Close(); err != nil {
			t.Fatalf("Close: %v", err)
		}
		if err := ecr.Migrate(o.Path); err != nil {
			t.Errorf("ecr.Migrate of an owner store: %v; want nil", err)
		}
		other := filepath.Join(t.TempDir(), "other.db")
		if err := os.WriteFile(other, nil, 0o600); err != nil {
			t.Fatalf("WriteFile: %v", err)
		}
		if err := ecr.Migrate(other); !errors.Is(err, ecr.ErrSchema) {
			t.Errorf("ecr.Migrate of an empty file: %v; want an error that matches ErrSchema", err)
		}
		missing := filepath.Join(t.TempDir(), "missing.db")
		if err := ecr.Migrate(missing); err == nil {
			t.Error("ecr.Migrate of a missing path: no error; want an error")
		}
		absent(t, missing)
	})
}

// TestOpen holds the steps of Open: a Host made with Create commits a deed with its trace id and span id, which a Host
// opened later reads back through the store with its engine at the seq of the store and the All kind Cold; Open of an
// absent path without Create fails, makes no file and closes the engine it built; Open with Create of an existing
// store fails with ErrExist and closes the engine; an Engine func of the Options builds the engine, and one that
// returns no engine and no error is refused with ErrConfig before a file is made; and Options without a Path, a
// Catalog, a Minter or a DeedKind that is a kind are refused with ErrConfig before a file is made.
func TestOpen(t *testing.T) {
	t.Run("create", func(t *testing.T) {
		o := options(t)
		h, err := ecr.Open(o)
		if err != nil {
			t.Fatalf("Open with Create: %v", err)
		}
		buf, err := h.Register(0, "tx-0001")
		if err != nil {
			t.Fatalf("Register: %v", err)
		}
		tr, err := ecr.ParseTraceID(traceText)
		if err != nil {
			t.Fatalf("ParseTraceID: %v", err)
		}
		sp, err := ecr.ParseSpanID(spanText)
		if err != nil {
			t.Fatalf("ParseSpanID: %v", err)
		}
		e := buf.Spawn(kindAll)
		buf.Set(e, cidOne, []byte{1, 2, 3, 4, 5, 6, 7, 8})
		buf.Span(tr, sp)
		res, err := h.Flush()
		if err != nil || res.Seq != 1 || len(res.Deeds) != 1 || len(res.Outcomes) != 1 ||
			res.Outcomes[0] != ecr.Committed {
			t.Fatalf("Flush: %+v, %v; want one deed committed at seq 1", res, err)
		}
		// The violating half: a record that names no entity refuses the flush with ErrRef and commits nothing.
		buf.Set(ecr.Of(ecr.ID{}), cidOne, make([]byte, 8))
		if r, err := h.Flush(); !errors.Is(err, ecr.ErrRef) || r.Seq != 0 {
			t.Errorf("Flush of a record that names the zero eid: %+v, %v; want seq 0 and an error that matches ErrRef",
				r, err)
		}
		if err := h.Close(); err != nil {
			t.Fatalf("Close: %v", err)
		}

		o.Create = false
		h, err = ecr.Open(o)
		if err != nil {
			t.Fatalf("Open of the store without Create: %v", err)
		}
		defer h.Close()
		if at, st := h.Engine.At(), h.Engine.State(kindAll); at != 1 || st != ecr.Cold {
			t.Errorf("the engine after Open: At %d, State(aaa) %v; want At 1 and the All kind Cold (%v)", at, st,
				ecr.Cold)
		}
		var got []ecr.Deed
		err = h.Store.Log(0, func(seq uint64, d ecr.Deed) error {
			got = append(got, d)
			return nil
		})
		if err != nil || len(got) != 1 {
			t.Fatalf("Log(0): %d deeds, %v; want 1", len(got), err)
		}
		if d := got[0]; d.ID != res.Deeds[0] || d.TraceID != tr || d.SpanID != sp {
			t.Errorf("Log(0): deed %v, trace %v, span %v; want %v, %v, %v", d.ID, d.TraceID, d.SpanID, res.Deeds[0],
				tr, sp)
		}
	})

	t.Run("missing", func(t *testing.T) {
		o := options(t)
		o.Create = false
		var got *named
		o.Engine = namedEngine("named", nil, &got)
		if h, err := ecr.Open(o); err == nil {
			h.Close()
			t.Fatal("Open of an absent path without Create: no error; want an error")
		}
		absent(t, o.Path)
		if got == nil || got.closes != 1 {
			t.Errorf("the engine that the failed Open built: %v; want it closed once", got)
		}
		// The violating half: with Create the same path opens.
		o.Create = true
		h, err := ecr.Open(o)
		if err != nil {
			t.Fatalf("Open with Create: %v", err)
		}
		if err := h.Close(); err != nil {
			t.Errorf("Close: %v", err)
		}
	})

	t.Run("exists", func(t *testing.T) {
		o := options(t)
		h, err := ecr.Open(o)
		if err != nil {
			t.Fatalf("Open with Create: %v", err)
		}
		if err := h.Close(); err != nil {
			t.Fatalf("Close: %v", err)
		}
		var got *named
		o.Engine = namedEngine("named", nil, &got)
		_, err = ecr.Open(o)
		if !errors.Is(err, ecr.ErrExist) || !strings.HasPrefix(fmt.Sprint(err), "ecr: open: ") {
			t.Errorf("Open with Create of an existing store: %v; want an error that matches ErrExist and begins with "+
				"\"ecr: open: \"", err)
		}
		if got == nil || got.closes != 1 {
			t.Errorf("the engine that the failed Open built: %v; want it closed once", got)
		}
		// The violating half: without Create the same store opens.
		o.Create = false
		h, err = ecr.Open(o)
		if err != nil {
			t.Fatalf("Open of the store without Create: %v", err)
		}
		if err := h.Close(); err != nil {
			t.Errorf("Close: %v", err)
		}
	})

	t.Run("engine", func(t *testing.T) {
		o := options(t)
		var got *named
		o.Engine = namedEngine("named", nil, &got)
		h, err := ecr.Open(o)
		if err != nil {
			t.Fatalf("Open with an Engine func: %v", err)
		}
		if n := h.Engine.Name(); n != "named" || got == nil || h.Engine != ecr.Engine(got) {
			t.Errorf("the engine of the Host answers Name %q; want the engine of the Engine func, \"named\"", n)
		}
		if err := h.Close(); err != nil {
			t.Errorf("Close: %v", err)
		}
		// The violating half: an Engine func that refuses makes Open fail with its error and make no file, and
		// Options without an Engine func build Jacquard.
		refusal := errors.New("refused by the test")
		o = options(t)
		o.Engine = func(*ecr.Catalog) (ecr.Engine, error) { return nil, refusal }
		if _, err := ecr.Open(o); !errors.Is(err, refusal) {
			t.Errorf("Open with a refusing Engine func: %v; want its error", err)
		}
		absent(t, o.Path)
		h, err = ecr.Open(options(t))
		if err != nil {
			t.Fatalf("Open without an Engine func: %v", err)
		}
		if n := h.Engine.Name(); n != "jacquard" {
			t.Errorf("the default engine answers Name %q; want \"jacquard\"", n)
		}
		h.Close()
	})

	t.Run("nilengine", func(t *testing.T) {
		o := options(t)
		o.Engine = func(*ecr.Catalog) (ecr.Engine, error) { return nil, nil }
		h, err := ecr.Open(o)
		if err == nil {
			h.Close()
		}
		if !errors.Is(err, ecr.ErrConfig) {
			t.Errorf("Open with an Engine func that returns no engine and no error: %v; want an error that matches "+
				"ErrConfig", err)
		}
		absent(t, o.Path)
		// The violating half: an Engine func that returns an engine opens at the same path.
		o.Engine = jacquard.New
		h, err = ecr.Open(o)
		if err != nil {
			t.Fatalf("Open with an Engine func that returns an engine: %v", err)
		}
		if err := h.Close(); err != nil {
			t.Errorf("Close: %v", err)
		}
	})

	t.Run("nilcatalog", func(t *testing.T) {
		for _, c := range []struct {
			name string
			edit func(*ecr.Options)
		}{
			{"no Path", func(o *ecr.Options) { o.Path = "" }},
			{"no Path without Create", func(o *ecr.Options) { o.Path, o.Create = "", false }},
			{"no Catalog", func(o *ecr.Options) { o.Catalog = nil }},
			{"no Minter", func(o *ecr.Options) { o.Minter = nil }},
			{"DeedKind 0", func(o *ecr.Options) { o.DeedKind = 0 }},
		} {
			o := options(t)
			c.edit(&o)
			if h, err := ecr.Open(o); !errors.Is(err, ecr.ErrConfig) {
				if err == nil {
					h.Close()
				}
				t.Errorf("Open with %s: %v; want an error that matches ErrConfig", c.name, err)
			}
			absent(t, o.Path)
		}
		// The violating half: the Options with all three open.
		h, err := ecr.Open(options(t))
		if err != nil {
			t.Fatalf("Open: %v", err)
		}
		h.Close()
	})
}

// TestClose holds that Close closes the engine and the store, that a second Close returns nil, that the store is
// closed when the engine's Close fails, whose error Close returns, and that the engine is closed first, while the
// store is open.
func TestClose(t *testing.T) {
	t.Run("both", func(t *testing.T) {
		h, err := ecr.Open(options(t))
		if err != nil {
			t.Fatalf("Open: %v", err)
		}
		// The violating half: before Close the engine answers and the store is open.
		if st := h.Engine.State(kindTouch); st != ecr.Warm || !storeOpen(h.Store) {
			t.Fatalf("before Close: State(bbb) %v, store open %v; want Warm and open", st, storeOpen(h.Store))
		}
		if err := h.Close(); err != nil {
			t.Fatalf("Close: %v", err)
		}
		if st := h.Engine.State(kindTouch); st != ecr.Stale {
			t.Errorf("after Close: State(bbb) %v; want Stale (%v)", st, ecr.Stale)
		}
		if storeOpen(h.Store) {
			t.Error("after Close the store answers Log; want it closed")
		}
		if err := h.Close(); err != nil {
			t.Errorf("a second Close: %v; want nil", err)
		}
	})

	t.Run("error", func(t *testing.T) {
		o := options(t)
		boom := errors.New("close refused by the test")
		var got *named
		o.Engine = namedEngine("named", boom, &got)
		h, err := ecr.Open(o)
		if err != nil {
			t.Fatalf("Open: %v", err)
		}
		if err := h.Close(); !errors.Is(err, boom) {
			t.Errorf("Close with an engine whose Close fails: %v; want its error", err)
		}
		if got.closes != 1 || storeOpen(h.Store) {
			t.Errorf("after Close: %d engine closes, store open %v; want 1 and closed", got.closes, storeOpen(h.Store))
		}
	})

	t.Run("order", func(t *testing.T) {
		o := options(t)
		var got *named
		o.Engine = namedEngine("named", nil, &got)
		h, err := ecr.Open(o)
		if err != nil {
			t.Fatalf("Open: %v", err)
		}
		// The engine records whether the store answers when its Close begins.
		calls, open := 0, false
		got.onClose = func() {
			calls++
			open = storeOpen(h.Store)
		}
		if err := h.Close(); err != nil {
			t.Fatalf("Close: %v", err)
		}
		if calls != 1 || !open {
			t.Errorf("the engine's Close began %d times, with the store open %v; want once, with the store open", calls,
				open)
		}
		// The violating half: the probe tells the states apart, since the store answers no Log after Close.
		if storeOpen(h.Store) {
			t.Error("after Close the store answers Log; want it closed")
		}
	})
}
