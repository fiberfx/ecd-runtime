package rt

import (
	"errors"
	"strings"
	"testing"

	eid "github.com/fiberfx/ecd-id/go"
	"github.com/fiberfx/ecd-runtime/go/ecd"
)

const (
	cidWord = ecd.CID(1) // a component of 8 bytes
	cidMark = ecd.CID(2) // a tag
	cidText = ecd.CID(3) // a component with pointers, of any length
	cidNone = ecd.CID(9) // not declared
)

// checkBase is the time of the first test eid, in milliseconds.
const checkBase = 1795271400000

var (
	checkSys   = eid.MustCode("tst")
	checkKind  = ecd.Kind{System: checkSys, Kind: eid.MustCode("aaa")}
	checkOther = ecd.Kind{System: checkSys, Kind: eid.MustCode("bbb")} // not declared
)

// checkCatalog declares one kind, a component of 8 bytes, a tag and a component with pointers.
func checkCatalog() *ecd.Catalog {
	return &ecd.Catalog{
		Components: []ecd.ComponentDef{
			{CID: cidWord, Name: "word", Form: ecd.Component, Size: 8},
			{CID: cidMark, Name: "mark", Form: ecd.Tag},
			{CID: cidText, Name: "text", Form: ecd.Component, Size: 16, Pointers: true},
		},
		Kinds: []ecd.KindDef{{Kind: checkKind, Residency: ecd.All}},
	}
}

// checkID returns the eid of kind k that carries n in its time, so that eids with different n differ.
func checkID(t *testing.T, k ecd.Kind, n uint64) eid.ID {
	t.Helper()
	id, err := eid.FromParts(k.System, k.Kind, checkBase+n, 1, 1)
	if err != nil {
		t.Fatalf("FromParts(%v, %d): %v", k, n, err)
	}
	return id
}

func TestCheck(t *testing.T) {
	x := newIndex(checkCatalog())
	a := checkID(t, checkKind, 1)
	b := checkID(t, checkKind, 2)
	c := checkID(t, checkKind, 3)
	out := checkID(t, checkOther, 4)
	word := make([]byte, 8)

	spawn := func(e eid.ID) ecd.Op { return ecd.Op{Code: ecd.OpSpawn, Entity: e} }
	set := func(e eid.ID, cid ecd.CID, v []byte) ecd.Op {
		return ecd.Op{Code: ecd.OpSet, Entity: e, CID: cid, Value: v}
	}
	unset := func(e eid.ID, cid ecd.CID) ecd.Op { return ecd.Op{Code: ecd.OpUnset, Entity: e, CID: cid} }
	destroy := func(e eid.ID) ecd.Op { return ecd.Op{Code: ecd.OpDestroy, Entity: e} }
	deed := func(ord uint32, ops ...ecd.Op) ecd.Deed { return ecd.Deed{ID: c, Ord: ord, Ops: ops} }

	good := []struct {
		name  string
		deeds []ecd.Deed
	}{
		{"no deed", nil},
		{"a deed without an op", []ecd.Deed{deed(0)}},
		{"every op code", []ecd.Deed{
			deed(0, spawn(a), set(a, cidWord, word), set(a, cidMark, nil), set(a, cidText, []byte("abc"))),
			deed(1, unset(a, cidMark), set(b, cidWord, word), set(b, cidText, nil), unset(b, cidWord), destroy(c)),
		}},
		{"an empty value for a tag", []ecd.Deed{deed(0, set(a, cidMark, []byte{}))}},
		{"a set of an eid across deeds", []ecd.Deed{deed(0, set(a, cidWord, word)), deed(1, set(a, cidWord, word))}},
	}
	for _, g := range good {
		if err := x.check(g.deeds); err != nil {
			t.Errorf("%s: check = %v, want nil", g.name, err)
		}
	}

	bad := []struct {
		name  string
		deeds []ecd.Deed
		want  error
	}{
		{"undeclared kind, spawn", []ecd.Deed{deed(0, spawn(out))}, ecd.ErrKind},
		{"undeclared kind, set", []ecd.Deed{deed(0, set(out, cidWord, word))}, ecd.ErrKind},
		{"undeclared kind before undeclared cid", []ecd.Deed{deed(0, set(out, cidNone, word))}, ecd.ErrKind},
		{"unknown op code 0", []ecd.Deed{deed(0, ecd.Op{Code: 0, Entity: a})}, ecd.ErrOp},
		{"unknown op code 5", []ecd.Deed{deed(0, ecd.Op{Code: ecd.OpDestroy + 1, Entity: a})}, ecd.ErrOp},
		{"undeclared cid, set", []ecd.Deed{deed(0, set(a, cidNone, word))}, ecd.ErrOp},
		{"undeclared cid, unset", []ecd.Deed{deed(0, unset(a, cidNone))}, ecd.ErrOp},
		{"tag with a value", []ecd.Deed{deed(0, set(a, cidMark, []byte{1}))}, ecd.ErrOp},
		{"value too short", []ecd.Deed{deed(0, set(a, cidWord, word[:7]))}, ecd.ErrOp},
		{"value too long", []ecd.Deed{deed(0, set(a, cidWord, append(word, 0)))}, ecd.ErrOp},
		{"component without a value", []ecd.Deed{deed(0, set(a, cidWord, nil))}, ecd.ErrOp},
		{"spawn then destroy", []ecd.Deed{deed(0, spawn(a), destroy(a))}, ecd.ErrOp},
		{"spawn then destroy, two deeds", []ecd.Deed{deed(0, spawn(a)), deed(1, destroy(a))}, ecd.ErrOp},
		{"destroy then spawn", []ecd.Deed{deed(0, destroy(a), spawn(a))}, ecd.ErrOp},
		{"spawn after a set", []ecd.Deed{deed(0, set(a, cidWord, word), spawn(a))}, ecd.ErrOp},
		{"spawn twice", []ecd.Deed{deed(0, spawn(a)), deed(1, spawn(a))}, ecd.ErrOp},
		{"set after destroy", []ecd.Deed{deed(0, destroy(a), set(a, cidWord, word))}, ecd.ErrOp},
		{"set after destroy, two deeds", []ecd.Deed{deed(0, destroy(a)), deed(1, set(a, cidWord, word))}, ecd.ErrOp},
		{"unset after destroy", []ecd.Deed{deed(0, destroy(a), unset(a, cidWord))}, ecd.ErrOp},
		{"destroy after destroy", []ecd.Deed{deed(0, destroy(a), destroy(a))}, ecd.ErrOp},
	}
	for _, g := range bad {
		err := x.check(g.deeds)
		if !errors.Is(err, g.want) {
			t.Errorf("%s: check = %v, want an error that matches %v", g.name, err, g.want)
			continue
		}
		if !strings.HasPrefix(err.Error(), "rt: ") {
			t.Errorf("%s: error text %q does not begin with %q", g.name, err.Error(), "rt: ")
		}
		if g.want == ecd.ErrKind && errors.Is(err, ecd.ErrOp) {
			t.Errorf("%s: error matches ecd.ErrOp as well as ecd.ErrKind", g.name)
		}
	}

	// The first fault in commit order is the one returned.
	if err := x.check([]ecd.Deed{deed(0, set(a, cidNone, word), spawn(out))}); !errors.Is(err, ecd.ErrOp) {
		t.Errorf("an undeclared cid before an undeclared kind: check = %v, want ecd.ErrOp", err)
	}
	if err := x.check([]ecd.Deed{deed(0, spawn(out)), deed(1, set(a, cidNone, word))}); !errors.Is(err, ecd.ErrKind) {
		t.Errorf("an undeclared kind before an undeclared cid: check = %v, want ecd.ErrKind", err)
	}

	// A refusal leaves nothing behind: the same index answers the next transaction as if it were the first.
	if err := x.check([]ecd.Deed{deed(0, destroy(a))}); err != nil {
		t.Errorf("a destroy after a refused transaction: check = %v, want nil", err)
	}
	if err := x.check([]ecd.Deed{deed(0, spawn(a), set(a, cidWord, word))}); err != nil {
		t.Errorf("a spawn after a destroy in an earlier transaction: check = %v, want nil", err)
	}
}
