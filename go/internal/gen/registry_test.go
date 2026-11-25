package gen_test

import (
	"errors"
	"strings"
	"testing"

	eid "github.com/fiberfx/ecd-id/go"
	"github.com/fiberfx/ecd-runtime/go/internal/gen"
)

const regSystems = "code\tname\towner\thome\tstate\tserved_by\n" +
	"tst\tTest\t-\t-\tlive\t-\n" +
	"cls\tClosed\t-\t-\tclosed\t-\n" +
	"old\tOld\t-\t-\tretired\t-\n"

const regKinds = "system\tkind\trole\tstate\tserved_by\tmeaning\n" +
	"tst\taaa\tentity\tlive\t-\tlive entity\n" +
	"tst\tbbb\tactivation\tlive\t-\tan activation\n" +
	"tst\tccc\tentity\tretired\t-\tretired entity\n" +
	"tst\tddd\tactivation\tclosed\t-\tclosed activation\n"

func registry(t *testing.T) *eid.Registry {
	t.Helper()
	reg, err := eid.ParseRegistry(regSystems, regKinds)
	if err != nil {
		t.Fatal(err)
	}
	return reg
}

// catalogOf returns a catalog source of system sys with the kinds codes.
func catalogOf(sys string, codes ...string) string {
	var ks []string
	for _, c := range codes {
		ks = append(ks, `{"code": "`+c+`", "residency": "all", "budget": 0}`)
	}
	return `{"format": 1, "package": "tst", "system": "` + sys + `", "kinds": [` + strings.Join(ks, ", ") +
		`], "components": [{"cid": 1, "name": "one", "form": "component", "type": "u64"}]}`
}

func TestCheckRegistry(t *testing.T) {
	reg := registry(t)
	cases := []struct {
		name string
		src  string
		want []string
	}{
		{"live", catalogOf("tst", "aaa"), nil},
		{"role", catalogOf("tst", "aaa", "bbb"),
			[]string{`$.kinds[1].code: kind "bbb" of system "tst" has the role activation, not entity`}},
		{"retired", catalogOf("tst", "ccc"),
			[]string{`$.kinds[0].code: kind "ccc" of system "tst" is retired in the registry`}},
		{"role and state", catalogOf("tst", "ddd"),
			[]string{
				`$.kinds[0].code: kind "ddd" of system "tst" has the role activation, not entity`,
				`$.kinds[0].code: kind "ddd" of system "tst" is closed in the registry`,
			}},
		{"unknown kind", catalogOf("tst", "zzz", "aaa"),
			[]string{`$.kinds[0].code: kind "zzz" of system "tst" is not in the registry`}},
		{"unknown system", catalogOf("xyz", "aaa"),
			[]string{`$.system: system "xyz" is not in the registry`}},
		{"closed system", catalogOf("cls", "aaa"),
			[]string{`$.system: system "cls" is closed in the registry`}},
		{"retired system", catalogOf("old", "aaa"),
			[]string{`$.system: system "old" is retired in the registry`}},
		{"detached", `{"format": 1, "package": "tst", "detached": true, "kinds": [{"code": "aaa", "residency": "all", "budget": 0}], "components": [{"cid": 1, "name": "one", "form": "component", "type": "u64"}]}`,
			[]string{`$.detached: a detached catalog has no system to check against the registry`}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			c, err := gen.Parse([]byte(tc.src), "tst.catalog.json")
			if err != nil {
				t.Fatal(err)
			}
			var got []string
			for _, r := range gen.CheckRegistry(c, reg) {
				got = append(got, r.Path+": "+r.Reason)
			}
			if strings.Join(got, "\n") != strings.Join(tc.want, "\n") {
				t.Errorf("refusals %q, want %q", got, tc.want)
			}
		})
	}
}

func TestGenerateWith(t *testing.T) {
	reg := registry(t)
	src := []byte(catalogOf("tst", "aaa"))
	plain, err := gen.Generate(src, "tst.catalog.json", gen.ModeCore)
	if err != nil {
		t.Fatal(err)
	}
	for _, r := range []*eid.Registry{nil, reg} {
		got, err := gen.GenerateWith(src, "tst.catalog.json", gen.ModeCore, r)
		if err != nil || string(got) != string(plain) {
			t.Errorf("GenerateWith(reg %v) differs from Generate (err %v)", r != nil, err)
		}
	}
	bad := []byte(catalogOf("tst", "bbb", "ccc"))
	if _, err := gen.GenerateWith(bad, "tst.catalog.json", gen.ModeCore, nil); err != nil {
		t.Errorf("a nil registry checked the catalog: %v", err)
	}
	_, err = gen.GenerateWith(bad, "tst.catalog.json", gen.ModeCore, reg)
	var refused *gen.Refused
	if !errors.As(err, &refused) || len(refused.Refusals) != 2 {
		t.Fatalf("err = %v, want a *Refused of 2 refusals", err)
	}
}
