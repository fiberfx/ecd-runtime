package ecd_test

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"strings"
	"testing"

	eid "github.com/fiberfx/ecd-id/go"
	"github.com/fiberfx/ecd-runtime/go/ecd"
)

// stub is the smallest type that satisfies ecd.Engine.
type stub struct{ at uint64 }

func (s *stub) Name() string                                           { return "stub" }
func (s *stub) At() uint64                                             { return s.at }
func (s *stub) State(k ecd.Kind) ecd.State                             { return ecd.Cold }
func (s *stub) Hydrate(k ecd.Kind, at uint64, src ecd.RowSource) error { return ecd.ErrCold }
func (s *stub) Evict(k ecd.Kind, ids []eid.ID) error                   { return ecd.ErrMiss }
func (s *stub) Stats() ecd.Stats                                       { return ecd.Stats{} }
func (s *stub) Close() error                                           { return nil }

func (s *stub) Apply(seq uint64, deeds []ecd.Deed) error {
	if seq != s.at+1 {
		return fmt.Errorf("apply %d after %d: %w", seq, s.at, ecd.ErrGap)
	}
	s.at = seq
	return nil
}

func (s *stub) Get(id eid.ID, c ecd.CID, dst []byte) ([]byte, error) {
	return dst, fmt.Errorf("get %v: %w", c, ecd.ErrAbsent)
}

func (s *stub) Query(q ecd.QID, arg []byte, out *ecd.Answer) error {
	out.IDs = append(out.IDs, eid.ID{})
	out.Count++
	return nil
}

var _ ecd.Engine = (*stub)(nil)

// I2-05: a stub satisfies the seam, and its answers carry the sentinels through errors.Is.
func TestSeamStub(t *testing.T) {
	var e ecd.Engine = &stub{}
	if e.Name() != "stub" || e.At() != 0 || e.State(ecd.Kind{}) != ecd.Cold {
		t.Fatalf("Name, At or State of the stub: %q %d %v", e.Name(), e.At(), e.State(ecd.Kind{}))
	}
	if err := e.Apply(2, nil); !errors.Is(err, ecd.ErrGap) {
		t.Errorf("Apply(2) on a fresh engine: err = %v, want ErrGap", err)
	}
	deed := ecd.Deed{Ord: 1, Ops: []ecd.Op{{Code: ecd.OpSpawn}, {Code: ecd.OpSet, CID: 1, Value: []byte{1}}}}
	if err := e.Apply(1, []ecd.Deed{deed}); err != nil || e.At() != 1 {
		t.Errorf("Apply(1): err = %v, At = %d; want nil, 1", err, e.At())
	}
	if _, err := e.Get(eid.ID{}, 1, nil); !errors.Is(err, ecd.ErrAbsent) {
		t.Errorf("Get: err = %v, want ErrAbsent", err)
	}
	var out ecd.Answer
	if err := e.Query(0, nil, &out); err != nil || out.Count != 1 || len(out.IDs) != 1 {
		t.Errorf("Query: err = %v, answer %+v", err, out)
	}
}

// I2-05: the values of the seam's constants.
func TestSeamConstants(t *testing.T) {
	for _, c := range []struct {
		name      string
		got, want uint8
	}{
		{"Touched", uint8(ecd.Touched), 1},
		{"All", uint8(ecd.All), 2},
		{"Cold", uint8(ecd.Cold), 0},
		{"Warm", uint8(ecd.Warm), 1},
		{"Stale", uint8(ecd.Stale), 2},
		{"Component", uint8(ecd.Component), 1},
		{"Tag", uint8(ecd.Tag), 2},
		{"Edge", uint8(ecd.Edge), 3},
		{"OpSpawn", uint8(ecd.OpSpawn), 1},
		{"OpSet", uint8(ecd.OpSet), 2},
		{"OpUnset", uint8(ecd.OpUnset), 3},
		{"OpDestroy", uint8(ecd.OpDestroy), 4},
	} {
		if c.got != c.want {
			t.Errorf("%s = %d, want %d", c.name, c.got, c.want)
		}
	}
}

var seamErrors = []struct {
	name string
	err  error
	text string
}{
	{"ErrGap", ecd.ErrGap, "ecd: apply out of order"},
	{"ErrStale", ecd.ErrStale, "ecd: engine stale"},
	{"ErrCold", ecd.ErrCold, "ecd: kind cold"},
	{"ErrMiss", ecd.ErrMiss, "ecd: not resident"},
	{"ErrAbsent", ecd.ErrAbsent, "ecd: absent"},
	{"ErrKind", ecd.ErrKind, "ecd: kind not in catalog"},
	{"ErrOp", ecd.ErrOp, "ecd: op refused"},
	{"ErrBudget", ecd.ErrBudget, "ecd: over budget"},
	{"ErrQuery", ecd.ErrQuery, "ecd: query refused"},
	{"ErrOwner", ecd.ErrOwner, "ecd: call off the owner goroutine"},
}

// I2-05, I2B-03: the ten error texts.
func TestSeamErrorTexts(t *testing.T) {
	if len(seamErrors) != 10 {
		t.Fatalf("the seam lists %d error texts, want 10", len(seamErrors))
	}
	for _, c := range seamErrors {
		if c.err.Error() != c.text {
			t.Errorf("%s = %q, want %q", c.name, c.err.Error(), c.text)
		}
	}
}

// I2-05: no two sentinels match each other under errors.Is, and each matches itself when wrapped.
func TestSeamSentinelsDistinct(t *testing.T) {
	for i, a := range seamErrors {
		wrapped := fmt.Errorf("context: %w", a.err)
		for j, b := range seamErrors {
			if got := errors.Is(wrapped, b.err); got != (i == j) {
				t.Errorf("errors.Is(wrapped %s, %s) = %v, want %v", a.name, b.name, got, i == j)
			}
		}
	}
}

// I2B-03: ErrOp matches itself, wrapped, and none of the nine other sentinels.
func TestSeamErrOpDistinct(t *testing.T) {
	wrapped := fmt.Errorf("check pass: %w", ecd.ErrOp)
	if !errors.Is(wrapped, ecd.ErrOp) {
		t.Errorf("errors.Is(wrapped ErrOp, ErrOp) = false")
	}
	for _, c := range seamErrors {
		if c.err != ecd.ErrOp && errors.Is(ecd.ErrOp, c.err) {
			t.Errorf("errors.Is(ErrOp, %s) = true, want false", c.name)
		}
	}
}

// I2B-03: the comment above Apply in engine.go states the two passes and the stale rule.
func TestSeamApplyText(t *testing.T) {
	src, err := os.ReadFile("engine.go")
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(string(src), "\n")
	at := -1
	for i, l := range lines {
		if strings.HasPrefix(l, "\tApply(seq uint64, deeds []Deed) error") {
			at = i
			break
		}
	}
	if at < 0 {
		t.Fatal("engine.go declares no Apply method in Engine")
	}
	var doc []string
	for i := at - 1; i >= 0 && strings.HasPrefix(lines[i], "\t//"); i-- {
		doc = append(doc, lines[i])
	}
	text := strings.Join(doc, "\n")
	for _, want := range []string{"all or nothing", "check pass", "makes the engine Stale"} {
		if !strings.Contains(text, want) {
			t.Errorf("the Apply comment lacks %q", want)
		}
	}
}

// I2B-01: QueryDef.Any is a []CID that sits between None and Exact.
func TestSeamQueryAny(t *testing.T) {
	typ := reflect.TypeOf(ecd.QueryDef{})
	anyF, ok := typ.FieldByName("Any")
	if !ok {
		t.Fatal("QueryDef has no field Any")
	}
	if anyF.Type != reflect.TypeOf([]ecd.CID(nil)) {
		t.Errorf("QueryDef.Any has type %v, want []ecd.CID", anyF.Type)
	}
	none, _ := typ.FieldByName("None")
	exact, _ := typ.FieldByName("Exact")
	if anyF.Index[0] != none.Index[0]+1 || exact.Index[0] != anyF.Index[0]+1 {
		t.Errorf("field order: None %d, Any %d, Exact %d; want consecutive", none.Index[0], anyF.Index[0], exact.Index[0])
	}
}

// I2B-02: *eid.Minter satisfies the host's Minter.
var _ ecd.Minter = (*eid.Minter)(nil)

// sourceFiles lists the non-test Go files under go/ecd and go/engine, and fails when either holds none.
func sourceFiles(t *testing.T) []string {
	t.Helper()
	var all []string
	for _, root := range []string{".", filepath.Join("..", "engine")} {
		n := 0
		err := filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if !d.IsDir() && strings.HasSuffix(p, ".go") && !strings.HasSuffix(p, "_test.go") {
				all = append(all, p)
				n++
			}
			return nil
		})
		if err != nil {
			t.Fatalf("scan %s: %v", root, err)
		}
		if n == 0 {
			t.Fatalf("scan %s: no non-test Go file found", root)
		}
	}
	return all
}

// I2B-02: no engine or seam file builds a Minter or clones one; the host's Minter is the only one.
func TestNoSecondMinter(t *testing.T) {
	re := regexp.MustCompile(`eid\.NewMinter\b|\.Clone\(\)`)
	for _, p := range sourceFiles(t) {
		src, err := os.ReadFile(p)
		if err != nil {
			t.Fatal(err)
		}
		if m := re.Find(src); m != nil {
			t.Errorf("%s builds or clones a Minter: %q", p, m)
		}
	}
}

// I2B-04: no source file under go/ecd and go/engine carries the retired product name, in any case.
func TestNoLoomName(t *testing.T) {
	retired := "lo" + "om"
	for _, p := range sourceFiles(t) {
		src, err := os.ReadFile(p)
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(strings.ToLower(string(src)), retired) {
			t.Errorf("%s contains the retired name", p)
		}
	}
}
