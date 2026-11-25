package main

import (
	"bytes"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

// names returns the sorted entry names of dir.
func names(t *testing.T, dir string) []string {
	t.Helper()
	es, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	var ns []string
	for _, e := range es {
		ns = append(ns, e.Name())
	}
	sort.Strings(ns)
	return ns
}

func equal(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func TestRun(t *testing.T) {
	catalog, err := os.ReadFile("../../internal/llm/llm.catalog.json")
	if err != nil {
		t.Fatal(err)
	}
	committed, err := os.ReadFile("../../internal/llm/llm_gen.go")
	if err != nil {
		t.Fatal(err)
	}
	badCatalog := bytes.Replace(catalog, []byte(`"package"`), []byte(`"Package"`), 1)
	if bytes.Equal(badCatalog, catalog) {
		t.Fatal(`the catalog has no "package" key to rename`)
	}

	t.Run("good", func(t *testing.T) {
		dir := t.TempDir()
		in, out := filepath.Join(dir, "llm.catalog.json"), filepath.Join(dir, "llm_gen.go")
		if err := os.WriteFile(in, catalog, 0o644); err != nil {
			t.Fatal(err)
		}
		var stderr bytes.Buffer
		if rc := run([]string{"-import", "core", "-in", in, "-out", out}, &stderr); rc != 0 {
			t.Fatalf("rc = %d, want 0; stderr %q", rc, stderr.String())
		}
		got, err := os.ReadFile(out)
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(got, committed) {
			t.Errorf("the out file differs from internal/llm/llm_gen.go (%d bytes against %d)", len(got), len(committed))
		}
		if ns, want := names(t, dir), []string{"llm.catalog.json", "llm_gen.go"}; !equal(ns, want) {
			t.Errorf("the directory holds %v, want %v", ns, want)
		}
	})

	t.Run("refused", func(t *testing.T) {
		dir := t.TempDir()
		in, out := filepath.Join(dir, "llm.catalog.json"), filepath.Join(dir, "llm_gen.go")
		if err := os.WriteFile(in, badCatalog, 0o644); err != nil {
			t.Fatal(err)
		}
		var stderr bytes.Buffer
		if rc := run([]string{"-import", "core", "-in", in, "-out", out}, &stderr); rc != 1 {
			t.Fatalf("rc = %d, want 1; stderr %q", rc, stderr.String())
		}
		if _, err := os.Stat(out); err == nil {
			t.Error("the out file exists for a refused catalog")
		}
		if ns, want := names(t, dir), []string{"llm.catalog.json"}; !equal(ns, want) {
			t.Errorf("the directory holds %v, want %v", ns, want)
		}
		want := "llm.catalog.json: $: missing key \"package\"\n" +
			"llm.catalog.json: $.Package: unknown key\n"
		if stderr.String() != want {
			t.Errorf("stderr = %q, want %q", stderr.String(), want)
		}
	})

	t.Run("kept", func(t *testing.T) {
		dir := t.TempDir()
		in, out := filepath.Join(dir, "llm.catalog.json"), filepath.Join(dir, "llm_gen.go")
		if err := os.WriteFile(in, badCatalog, 0o644); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(out, []byte("old\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		var stderr bytes.Buffer
		if rc := run([]string{"-import", "core", "-in", in, "-out", out}, &stderr); rc != 1 {
			t.Fatalf("rc = %d, want 1; stderr %q", rc, stderr.String())
		}
		got, err := os.ReadFile(out)
		if err != nil {
			t.Fatal(err)
		}
		if string(got) != "old\n" {
			t.Errorf("the out file holds %q, want %q", got, "old\n")
		}
	})

	t.Run("usage", func(t *testing.T) {
		dir := t.TempDir()
		out := filepath.Join(dir, "llm_gen.go")
		var stderr bytes.Buffer
		if rc := run([]string{"-import", "core", "-out", out}, &stderr); rc != 2 {
			t.Fatalf("rc = %d, want 2; stderr %q", rc, stderr.String())
		}
		if _, err := os.Stat(out); err == nil {
			t.Error("the out file exists after a usage error")
		}
		if !strings.Contains(stderr.String(), "-in") {
			t.Errorf("stderr %q does not name -in", stderr.String())
		}
	})
}

const regSystems = "code\tname\towner\thome\tstate\tserved_by\n" +
	"llm\tLLM\t-\t-\tlive\t-\n" +
	"cls\tClosed\t-\t-\tclosed\t-\n"

// regKinds holds, for system llm, the kinds of the catalog of internal/llm: with fault false all three are live
// entities, with fault true cnv is an activation and arc is a retired entity (and ses is live).
func regKinds(fault bool) string {
	h := "system\tkind\trole\tstate\tserved_by\tmeaning\n"
	if !fault {
		return h + "llm\tses\tentity\tlive\t-\ta\nllm\tcnv\tentity\tlive\t-\tb\nllm\tarc\tentity\tlive\t-\tc\n"
	}
	return h + "llm\tses\tentity\tlive\t-\ta\nllm\tcnv\tactivation\tlive\t-\tb\nllm\tarc\tentity\tretired\t-\tc\n"
}

func writeRegistry(t *testing.T, kinds string) string {
	t.Helper()
	dir := t.TempDir()
	for name, body := range map[string]string{"systems.tsv": regSystems, "kinds.tsv": kinds} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

func TestRunRegistry(t *testing.T) {
	catalog, err := os.ReadFile("../../internal/llm/llm.catalog.json")
	if err != nil {
		t.Fatal(err)
	}
	committed, err := os.ReadFile("../../internal/llm/llm_gen.go")
	if err != nil {
		t.Fatal(err)
	}
	detached, err := os.ReadFile("../../internal/gen/testdata/detached.catalog.json")
	if err != nil {
		t.Fatal(err)
	}
	// try runs gen on src with the extra args and returns the exit code, stderr and the directory.
	try := func(t *testing.T, src []byte, extra ...string) (int, string, string) {
		t.Helper()
		dir := t.TempDir()
		in, out := filepath.Join(dir, "llm.catalog.json"), filepath.Join(dir, "llm_gen.go")
		if err := os.WriteFile(in, src, 0o644); err != nil {
			t.Fatal(err)
		}
		var stderr bytes.Buffer
		rc := run(append([]string{"-import", "core", "-in", in, "-out", out}, extra...), &stderr)
		return rc, stderr.String(), dir
	}
	noOut := func(t *testing.T, dir string) {
		t.Helper()
		if ns, want := names(t, dir), []string{"llm.catalog.json"}; !equal(ns, want) {
			t.Errorf("the directory holds %v, want %v", ns, want)
		}
	}

	t.Run("good", func(t *testing.T) {
		reg := writeRegistry(t, regKinds(false))
		rc, stderr, dir := try(t, catalog, "-registry", reg)
		if rc != 0 {
			t.Fatalf("rc = %d, want 0; stderr %q", rc, stderr)
		}
		got, err := os.ReadFile(filepath.Join(dir, "llm_gen.go"))
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(got, committed) {
			t.Errorf("the out file differs from internal/llm/llm_gen.go with -registry")
		}
	})

	t.Run("violating", func(t *testing.T) {
		reg := writeRegistry(t, regKinds(true))
		unknown := bytes.Replace(catalog, []byte(`"code": "ses"`), []byte(`"code": "zzz"`), 1)
		rc, stderr, dir := try(t, unknown, "-registry", reg)
		if rc != 1 {
			t.Fatalf("rc = %d, want 1; stderr %q", rc, stderr)
		}
		noOut(t, dir)
		for _, want := range []string{
			`llm.catalog.json: $.kinds[0].code: kind "zzz" of system "llm" is not in the registry`,
			`llm.catalog.json: $.kinds[1].code: kind "cnv" of system "llm" has the role activation, not entity`,
			`llm.catalog.json: $.kinds[2].code: kind "arc" of system "llm" is retired in the registry`,
		} {
			if !strings.Contains(stderr, want+"\n") {
				t.Errorf("stderr %q does not hold %q", stderr, want)
			}
		}
		// the same catalog without the flag is not checked
		if rc, stderr, _ := try(t, unknown); rc != 0 {
			t.Errorf("without -registry rc = %d, want 0; stderr %q", rc, stderr)
		}
	})

	t.Run("system", func(t *testing.T) {
		reg := writeRegistry(t, regKinds(false))
		for _, tc := range []struct{ src, want string }{
			{string(bytes.Replace(catalog, []byte(`"system": "llm"`), []byte(`"system": "xyz"`), 1)),
				`llm.catalog.json: $.system: system "xyz" is not in the registry` + "\n"},
			{string(bytes.Replace(catalog, []byte(`"system": "llm"`), []byte(`"system": "cls"`), 1)),
				`llm.catalog.json: $.system: system "cls" is closed in the registry` + "\n"},
			{string(detached),
				`detached.catalog.json: $.detached: a detached catalog has no system to check against the registry` + "\n"},
		} {
			rc, stderr, dir := try(t, []byte(tc.src), "-registry", reg)
			if rc != 1 {
				t.Fatalf("rc = %d, want 1; stderr %q", rc, stderr)
			}
			noOut(t, dir)
			if want := strings.Replace(tc.want, "detached.catalog.json", "llm.catalog.json", 1); stderr != want {
				t.Errorf("stderr = %q, want %q", stderr, want)
			}
		}
	})

	t.Run("bad registry", func(t *testing.T) {
		missing := filepath.Join(t.TempDir(), "absent")
		malformed := writeRegistry(t, "not a header\n")
		for _, reg := range []string{missing, malformed} {
			rc, stderr, dir := try(t, catalog, "-registry", reg)
			if rc != 3 {
				t.Fatalf("rc = %d, want 3; stderr %q", rc, stderr)
			}
			if !strings.HasPrefix(stderr, "gen: registry: ") {
				t.Errorf("stderr = %q, want the prefix \"gen: registry: \"", stderr)
			}
			noOut(t, dir)
		}
	})
}
