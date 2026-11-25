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
