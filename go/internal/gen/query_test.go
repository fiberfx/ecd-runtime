package gen_test

import (
	"regexp"
	"testing"

	"github.com/fiberfx/ecd-runtime/go/internal/gen"
)

const qbase = `{"format": 1, "package": "tst", "system": "tst", "kinds": [{"code": "aaa", "residency": "all", "budget": 0}, {"code": "bbb", "residency": "touched", "budget": 0}], "components": [{"cid": 1, "name": "one", "form": "component", "type": "u64", "query": true}, {"cid": 2, "name": "mark", "form": "tag"}], "queries": [{"name": "both", "kinds": ["aaa"], "all": ["one"], "none": ["mark"]}]}`

func TestQueries(t *testing.T) {
	t.Run("good", func(t *testing.T) {
		c, err := gen.Parse([]byte(qbase), "tst.catalog.json")
		if err != nil {
			t.Fatalf("Parse: %v", err)
		}
		if !c.Components[0].Query {
			t.Errorf("Components[0].Query = false, want true")
		}
		if len(c.Queries) != 1 {
			t.Fatalf("len(Queries) = %d, want 1", len(c.Queries))
		}
		q := c.Queries[0]
		if q.Name != "both" || len(q.Kinds) != 1 || q.Kinds[0] != "aaa" ||
			len(q.All) != 1 || q.All[0] != "one" || len(q.None) != 1 || q.None[0] != "mark" {
			t.Errorf("Queries[0] = %+v, want both: kinds [aaa] all [one] none [mark]", q)
		}
		out, err := gen.Generate([]byte(qbase), "tst.catalog.json", gen.ModeCore)
		if err != nil {
			t.Fatalf("Generate: %v", err)
		}
		for _, re := range []string{`(?m)^\tQIDOne +ecd\.QID = 0$`, `(?m)^\tQIDBoth +ecd\.QID = 1$`} {
			if !regexp.MustCompile(re).Match(out) {
				t.Errorf("the generated text does not match %s", re)
			}
		}
	})

	t.Run("touched", func(t *testing.T) {
		wantRefused(t, rep(t, qbase, `"kinds": ["aaa"]`, `"kinds": ["bbb"]`),
			`$.queries[0].kinds[0]: kind "bbb" is a Touched kind: an engine refuses a query on it (E8)`)
	})

	t.Run("undeclaredkind", func(t *testing.T) {
		wantRefused(t, rep(t, qbase, `"kinds": ["aaa"]`, `"kinds": ["ccc"]`),
			`$.queries[0].kinds[0]: kind "ccc" is not declared`)
	})

	t.Run("undeclaredcomponent", func(t *testing.T) {
		wantRefused(t, rep(t, qbase, `"all": ["one"]`, `"all": ["two"]`),
			`$.queries[0].all[0]: component "two" is not declared`)
	})

	t.Run("repeat", func(t *testing.T) {
		wantRefused(t, rep(t, qbase, `"none": ["mark"]`, `"none": ["one"]`),
			`$.queries[0].none[0]: component "one" is named at $.queries[0].all[0]`)
	})

	t.Run("tagmark", func(t *testing.T) {
		wantRefused(t, rep(t, qbase, `"form": "tag"}`, `"form": "tag", "query": true}`),
			`$.components[1].query: a tag holds no value to iterate: declare a query in "queries"`)
	})

	t.Run("name", func(t *testing.T) {
		wantRefused(t, rep(t, qbase, `"name": "both"`, `"name": "Both"`),
			`$.queries[0].name: name "Both" does not match ^[a-z][a-z0-9]*(_[a-z][a-z0-9]*)*$`)
	})

	t.Run("collision", func(t *testing.T) {
		wantRefused(t, rep(t, qbase, `"name": "both"`, `"name": "one"`),
			`$.queries[0].name: name "one" makes the identifier QIDOne, which another declaration of the file holds`)
	})

	t.Run("exactfalse", func(t *testing.T) {
		wantRefused(t, rep(t, qbase, `"none": ["mark"]`, `"none": ["mark"], "exact": false`),
			`$.queries[0].exact: want true; leave the key out for false`)
	})

	t.Run("kindsstring", func(t *testing.T) {
		wantRefused(t, rep(t, qbase, `"kinds": ["aaa"]`, `"kinds": "aaa"`),
			`$.queries[0].kinds: want an array of strings`)
	})
}
