package gen_test

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/fiberfx/ecd-runtime/go/internal/gen"
)

// golden is one case of TestGolden: a catalog, its name, a mode and the committed file that Generate must equal.
type golden struct {
	name, catalog, mode, file string
}

var goldens = []golden{
	{"core", "../llm/llm.catalog.json", "core", "../llm/llm_gen.go"},
	{"detached", "testdata/detached.catalog.json", "core", "testdata/detached_gen.golden"},
}

// TestGolden generates the file of each case twice and requires both outputs to equal each other and the committed file.
func TestGolden(t *testing.T) {
	for _, g := range goldens {
		t.Run(g.name, func(t *testing.T) {
			src, err := os.ReadFile(g.catalog)
			if err != nil {
				t.Fatal(err)
			}
			mode, err := gen.ParseMode(g.mode)
			if err != nil {
				t.Fatal(err)
			}
			first, err := gen.Generate(src, filepath.Base(g.catalog), mode)
			if err != nil {
				t.Fatalf("Generate %s: %v", g.catalog, err)
			}
			second, err := gen.Generate(src, filepath.Base(g.catalog), mode)
			if err != nil {
				t.Fatalf("Generate %s, second time: %v", g.catalog, err)
			}
			if !bytes.Equal(first, second) {
				t.Fatalf("Generate %s is not deterministic: %s", g.catalog, firstDiff(first, second))
			}
			want, err := os.ReadFile(g.file)
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(first, want) {
				t.Errorf("Generate %s differs from %s: %s", g.catalog, g.file, firstDiff(want, first))
			}
		})
	}
}

// firstDiff describes the first line at which want and got differ, with both lines.
func firstDiff(want, got []byte) string {
	w := strings.Split(string(want), "\n")
	g := strings.Split(string(got), "\n")
	for i := 0; i < len(w) || i < len(g); i++ {
		var wl, gl string
		wok, gok := i < len(w), i < len(g)
		if wok {
			wl = w[i]
		}
		if gok {
			gl = g[i]
		}
		if wok && gok && wl == gl {
			continue
		}
		return fmt.Sprintf("line %d: want %s, got %s", i+1, lineText(wl, wok), lineText(gl, gok))
	}
	return "no differing line"
}

// lineText quotes a line, or says that the file has no such line.
func lineText(l string, ok bool) string {
	if !ok {
		return "no line"
	}
	return fmt.Sprintf("%q", l)
}
