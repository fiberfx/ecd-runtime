package ecd_test

import (
	"os"
	"reflect"
	"strings"
	"testing"

	"github.com/fiberfx/ecd-runtime/go/ecd"
)

// SP-01: a Deed carries the trace id and span id of the span that made it, as ecd.TraceID and ecd.SpanID, and the
// seam states that their zero values mean no trace and no span and that no engine reads them.
func TestDeedSpanFields(t *testing.T) {
	t.Run("types", func(t *testing.T) {
		typ := reflect.TypeOf(ecd.Deed{})
		for _, f := range []struct {
			name string
			want reflect.Type
		}{
			{"TraceID", reflect.TypeOf(ecd.TraceID{})},
			{"SpanID", reflect.TypeOf(ecd.SpanID{})},
		} {
			got, ok := typ.FieldByName(f.name)
			if !ok || got.Type != f.want {
				t.Errorf("Deed.%s: present %v, type %v; want type %v", f.name, ok, got.Type, f.want)
			}
		}
	})
	t.Run("comment", func(t *testing.T) {
		src, err := os.ReadFile("engine.go")
		if err != nil {
			t.Fatal(err)
		}
		for _, want := range []string{
			"the zero values mean no trace and no span.",
			"No engine reads them.",
		} {
			if !strings.Contains(string(src), want) {
				t.Errorf("engine.go lacks %q", want)
			}
		}
	})
}
