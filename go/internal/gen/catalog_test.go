package gen_test

import (
	"errors"
	"os"
	"strings"
	"testing"

	"github.com/fiberfx/ecd-runtime/go/internal/gen"
)

const minimal = `{"format": 1, "package": "tst", "system": "tst", "kinds": [{"code": "aaa", "residency": "all", "budget": 0}], "components": [{"cid": 1, "name": "one", "form": "component", "type": "u64"}, {"cid": 2, "name": "mark", "form": "tag"}]}`

// rep replaces the first occurrence of old in s with new, and fails the test when old is absent.
func rep(t *testing.T, s, old, new string) string {
	t.Helper()
	if !strings.Contains(s, old) {
		t.Fatalf("the source holds no %q", old)
	}
	return strings.Replace(s, old, new, 1)
}

// wantRefused parses src as tst.catalog.json and requires a *gen.Refused whose refusals, as "PATH: REASON", equal want.
func wantRefused(t *testing.T, src string, want ...string) {
	t.Helper()
	c, err := gen.Parse([]byte(src), "tst.catalog.json")
	if c != nil {
		t.Errorf("Parse returned a catalog for a refused source")
	}
	var r *gen.Refused
	if !errors.As(err, &r) {
		t.Fatalf("Parse error = %v, want a *gen.Refused", err)
	}
	var got []string
	for _, f := range r.Refusals {
		got = append(got, f.Path+": "+f.Reason)
	}
	if len(got) != len(want) {
		t.Fatalf("refusals:\n got %q\nwant %q", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("refusal %d:\n got %s\nwant %s", i, got[i], want[i])
		}
	}
}

func TestCatalog(t *testing.T) {
	t.Run("good", func(t *testing.T) {
		src, err := os.ReadFile("../llm/llm.catalog.json")
		if err != nil {
			t.Fatal(err)
		}
		c, err := gen.Parse(src, "llm.catalog.json")
		if err != nil {
			t.Fatalf("Parse: %v", err)
		}
		if c.Package != "llm" || c.System != "llm" || c.Detached {
			t.Errorf("Package %q System %q Detached %v, want llm llm false", c.Package, c.System, c.Detached)
		}
		var codes []string
		for _, k := range c.Kinds {
			codes = append(codes, k.Code)
		}
		if got := strings.Join(codes, " "); got != "ses cnv arc" {
			t.Errorf("kind codes %q, want %q", got, "ses cnv arc")
		}
		if len(c.Components) != 7 {
			t.Fatalf("%d components, want 7", len(c.Components))
		}
		if f := c.Components[6].Form; f != "tag" {
			t.Errorf("component 7 form %q, want tag", f)
		}
		if n := len(c.Components[2].Fields); n != 7 {
			t.Errorf("usage has %d fields, want 7", n)
		}
	})

	t.Run("detached", func(t *testing.T) {
		src, err := os.ReadFile("testdata/detached.catalog.json")
		if err != nil {
			t.Fatal(err)
		}
		c, err := gen.Parse(src, "detached.catalog.json")
		if err != nil {
			t.Fatalf("Parse: %v", err)
		}
		if !c.Detached || c.System != "" {
			t.Errorf("Detached %v System %q, want true and empty", c.Detached, c.System)
		}
	})

	t.Run("minimal", func(t *testing.T) {
		if _, err := gen.Parse([]byte(minimal), "tst.catalog.json"); err != nil {
			t.Fatalf("Parse: %v", err)
		}
	})

	t.Run("unknownkey", func(t *testing.T) {
		wantRefused(t, rep(t, minimal, `"package"`, `"Package"`),
			`$: missing key "package"`,
			`$.Package: unknown key`)
	})

	t.Run("repeatedkey", func(t *testing.T) {
		wantRefused(t, rep(t, minimal, `"cid": 1,`, `"cid": 1, "cid": 3,`),
			`$.components[0].cid: repeated key`)
	})

	t.Run("trailing", func(t *testing.T) {
		wantRefused(t, minimal+" {}",
			`$: trailing data after the catalog`)
	})

	t.Run("queries", func(t *testing.T) {
		wantRefused(t, minimal[:len(minimal)-1]+", \"queries\": []}",
			`$.queries: unknown key`)
	})

	t.Run("system", func(t *testing.T) {
		wantRefused(t, rep(t, minimal, `"system": "tst"`, `"system": "3ab"`),
			`$.system: system code "3ab" is detached: write "detached": true`)
		wantRefused(t, rep(t, minimal, `"system": "tst"`, `"system": "2ab"`),
			`$.system: system code "2ab" is reserved`)
		wantRefused(t, rep(t, minimal, `"system": "tst"`, `"system": "TST"`),
			`$.system: system "TST" is not a code`)
	})

	t.Run("detachedfalse", func(t *testing.T) {
		wantRefused(t, rep(t, minimal, `"system": "tst"`, `"detached": false`),
			`$.detached: want true; leave the key out for a registered system`)
	})

	t.Run("both", func(t *testing.T) {
		wantRefused(t, rep(t, minimal, `"system": "tst"`, `"system": "tst", "detached": true`),
			`$: want only one of "system" or "detached"`)
	})

	t.Run("neither", func(t *testing.T) {
		wantRefused(t, rep(t, minimal, `"system": "tst", `, ``),
			`$: want one of "system" or "detached": true`)
	})

	t.Run("cid", func(t *testing.T) {
		wantRefused(t, rep(t, minimal, `"cid": 2`, `"cid": 1`),
			`$.components[1].cid: cid 1 is declared at $.components[0]`)
		wantRefused(t, rep(t, minimal, `"cid": 1`, `"cid": 0`),
			`$.components[0].cid: cid 0 is out of range 1 to 65535`)
	})

	t.Run("package", func(t *testing.T) {
		wantRefused(t, rep(t, minimal, `"package": "tst"`, `"package": "ecd"`),
			`$.package: package name "ecd" is reserved`)
	})

	t.Run("kind", func(t *testing.T) {
		wantRefused(t, rep(t, minimal, `"code": "aaa"`, `"code": "sys"`),
			`$.kinds[0].code: kind "sys" names the system itself`)
	})

	t.Run("type", func(t *testing.T) {
		wantRefused(t, rep(t, minimal, `"type": "u64"`, `"type": "u128"`),
			`$.components[0].type: type "u128" is not a field type`)
		wantRefused(t, rep(t, minimal, `"type": "u64"`, `"type": "text[0]"`),
			`$.components[0].type: type "text[0]" is not a field type`)
		wantRefused(t, rep(t, minimal, `"type": "u64"`, `"type": "text[256]"`),
			`$.components[0].type: text[256]: N is out of range 1 to 255`)
	})

	t.Run("collision", func(t *testing.T) {
		wantRefused(t, rep(t, minimal, `"name": "one"`, `"name": "kind_aaa"`),
			`$.components[0].name: name "kind_aaa" makes the identifier KindAaa, which another declaration of the file holds`)
	})

	t.Run("sorted", func(t *testing.T) {
		src := rep(t, minimal, `"package": "tst"`, `"package": "ecd"`)
		src = rep(t, src, `"code": "aaa"`, `"code": "sys"`)
		src = rep(t, src, `"type": "u64"`, `"type": "u128"`)
		wantRefused(t, src,
			`$.components[0].type: type "u128" is not a field type`,
			`$.kinds[0].code: kind "sys" names the system itself`,
			`$.package: package name "ecd" is reserved`)
		_, err := gen.Parse([]byte(src), "tst.catalog.json")
		want := strings.Join([]string{
			`tst.catalog.json: $.components[0].type: type "u128" is not a field type`,
			`tst.catalog.json: $.kinds[0].code: kind "sys" names the system itself`,
			`tst.catalog.json: $.package: package name "ecd" is reserved`,
		}, "\n")
		if err == nil || err.Error() != want {
			t.Errorf("Error():\n got %v\nwant %s", err, want)
		}
	})
}
