package gen_test

import (
	"strings"
	"testing"

	"github.com/fiberfx/ecd-runtime/go/internal/gen"
)

const kbase = `{"format": 1, "package": "tst", "system": "tst", "kinds": [{"code": "aaa", "residency": "all", "budget": 0, "components": [{"name": "one"}]}], "components": [{"cid": 1, "name": "one", "form": "component", "type": "u64"}, {"cid": 2, "name": "mark", "form": "tag"}, {"cid": 3, "name": "has_one", "form": "component", "type": "u8"}]}`

// TestKindComponents is KB-02: the components that a kind lists are declared, hold a value and are listed once, the
// fields of its entity type are distinct, and the identifiers that the kind makes are free.
func TestKindComponents(t *testing.T) {
	t.Run("good", func(t *testing.T) {
		out, err := gen.Generate([]byte(kbase), "tst.catalog.json", gen.ModeCore)
		if err != nil {
			t.Fatalf("Generate: %v", err)
		}
		for _, want := range []string{"type AaaEntity struct", "func SpawnAaa(b *rt.Buffer, v AaaEntity) rt.Ref", "func GetAaa(e ecd.Engine, id eid.ID) (AaaEntity, bool, error)"} {
			if !strings.Contains(string(out), want) {
				t.Errorf("the generated text does not hold %q", want)
			}
		}
	})
	t.Run("optional", func(t *testing.T) {
		src := rep(t, kbase, `"components": [{"name": "one"}]`, `"components": [{"name": "one", "optional": true}]`)
		out, err := gen.Generate([]byte(src), "tst.catalog.json", gen.ModeCore)
		if err != nil {
			t.Fatalf("Generate: %v", err)
		}
		for _, want := range []string{"\tHasOne bool\n", "\tif v.HasOne {\n", "if v.One, v.HasOne, err = One.Get(e, id); err != nil {", "\tif !v.HasOne {\n"} {
			if !strings.Contains(string(out), want) {
				t.Errorf("the generated text does not hold %q", want)
			}
		}
	})
	t.Run("undeclared", func(t *testing.T) {
		wantRefused(t, rep(t, kbase, `"components": [{"name": "one"}]`, `"components": [{"name": "two"}]`),
			`$.kinds[0].components[0].name: component "two" is not declared`)
	})
	t.Run("tag", func(t *testing.T) {
		wantRefused(t, rep(t, kbase, `"components": [{"name": "one"}]`, `"components": [{"name": "mark"}]`),
			`$.kinds[0].components[0].name: "mark" is a tag: a kind lists components that hold a value`)
	})
	t.Run("repeat", func(t *testing.T) {
		wantRefused(t, rep(t, kbase, `"components": [{"name": "one"}]`, `"components": [{"name": "one"}, {"name": "one", "optional": true}]`),
			`$.kinds[0].components[1].name: component "one" is listed at $.kinds[0].components[0]`)
	})
	t.Run("field", func(t *testing.T) {
		wantRefused(t, rep(t, kbase, `"components": [{"name": "one"}]`, `"components": [{"name": "one", "optional": true}, {"name": "has_one"}]`),
			`$.kinds[0].components[1].name: component "has_one" makes the field HasOne, which component "one" makes`)
	})
	t.Run("member", func(t *testing.T) {
		wantRefused(t, rep(t, kbase, `"components": [{"name": "one"}]`, `"components": ["one"]`),
			`$.kinds[0].components[0]: want an object`)
	})
	t.Run("clash", func(t *testing.T) {
		src := rep(t, kbase, `"budget": 0, "components": [{"name": "one"}]}]`, `"budget": 0, "components": [{"name": "one"}]}, {"code": "bbb", "residency": "all", "budget": 0, "components": [{"name": "one"}]}]`)
		src = rep(t, src, `"name": "mark"`, `"name": "spawn_bbb"`)
		wantRefused(t, src,
			`$.kinds[1].components: kind "bbb" makes the identifier SpawnBbb, which another declaration of the file holds`)
	})
}

// TestKindDetached holds that a detached catalog spawns with the system and reads by the kind code alone.
func TestKindDetached(t *testing.T) {
	src := strings.Replace(kbase, `"system": "tst"`, `"detached": true`, 1)
	out, err := gen.Generate([]byte(src), "tst.catalog.json", gen.ModeCore)
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	for _, want := range []string{
		"func SpawnAaa(b *rt.Buffer, sys eid.Code, v AaaEntity) rt.Ref",
		"r := b.Spawn(KindAaa(sys))",
		"func GetAaa(e ecd.Engine, id eid.ID) (AaaEntity, bool, error)",
		"if id.Kind() != CodeAaa {",
	} {
		if !strings.Contains(string(out), want) {
			t.Errorf("the generated text does not hold %q", want)
		}
	}
}
