package gen

import (
	"cmp"
	"encoding/json"
	"fmt"
	"slices"
	"strconv"
	"strings"
)

// Catalog is a catalog file of format 1 as Parse decodes it, after the strict pass.
type Catalog struct {
	Format     int64       `json:"format"`
	Package    string      `json:"package"`
	System     string      `json:"system"`   // a registered system code; empty when Detached
	Detached   bool        `json:"detached"` // the system is chosen at run time
	Kinds      []Kind      `json:"kinds"`
	Components []Component `json:"components"`
	Queries    []Query     `json:"queries"`
}

// Kind is one kind of a catalog.
type Kind struct {
	Code      string `json:"code"`
	Residency string `json:"residency"` // "touched" or "all"
	Budget    int64  `json:"budget"`
	Doc       string `json:"doc"`
}

// Component is one component or tag of a catalog. A component has exactly one of Type and Fields; a tag neither.
type Component struct {
	CID    int64   `json:"cid"`
	Name   string  `json:"name"`
	Form   string  `json:"form"` // "component" or "tag"
	Type   string  `json:"type"`
	Fields []Field `json:"fields"`
	Query  bool    `json:"query"` // the handle is also a query of every All kind
	Doc    string  `json:"doc"`
}

// Query is one declared query of a catalog: its kinds by code (empty: every All kind) and its components by name.
type Query struct {
	Name  string   `json:"name"`
	Kinds []string `json:"kinds"`
	All   []string `json:"all"`
	None  []string `json:"none"`
	Any   []string `json:"any"`
	Exact bool     `json:"exact"`
	Doc   string   `json:"doc"`
}

// Field is one field of a component value, in declared order.
type Field struct {
	Name string `json:"name"`
	Type string `json:"type"`
}

// Refusal is one fault of a catalog: the JSON path of the value at fault and the reason.
type Refusal struct {
	Path   string
	Reason string
}

// Refused is the error of Parse and Generate for a refused catalog: every refusal found, sorted.
type Refused struct {
	Name     string // the base name of the catalog file
	Refusals []Refusal
}

// Error returns one line per refusal, "NAME: PATH: REASON", joined by "\n".
func (e *Refused) Error() string {
	lines := make([]string, len(e.Refusals))
	for i, r := range e.Refusals {
		lines[i] = e.Name + ": " + r.Path + ": " + r.Reason
	}
	return strings.Join(lines, "\n")
}

// Limits of format 1, each the reason a later step would refuse.
const (
	MaxNameLen    = 32   // a package, component or field name
	MaxDocLen     = 200  // a doc string, in bytes
	MaxKinds      = 1024 // Jacquard's index holds 65,535; 1024 is the generator's bound
	MaxComponents = 256  // components and tags: Jacquard's mask holds 256 bits without the build tag jacquard512
	MaxFields     = 64   // fields of one component
	MaxSize       = 4096 // codec bytes of one value; also the bound of N in bytes[N]
	MaxTextN      = 255  // N of text[N]: one length byte
	MaxQueries    = 256  // declared queries and query marks of one catalog; ecd.QID holds 65,536
)

// FieldType is a parsed field type: its base and, for bytes and text, its N.
type FieldType struct {
	Base string // u8 u16 u32 u64 i8 i16 i32 i64 f32 f64 bool eid bytes text
	N    int    // the N of bytes[N] and text[N]; 0 otherwise
}

// ParseType parses a field type of format 1; the error text is the refusal's reason.
func ParseType(s string) (FieldType, error) {
	switch s {
	case "u8", "u16", "u32", "u64", "i8", "i16", "i32", "i64", "f32", "f64", "bool", "eid":
		return FieldType{Base: s}, nil
	}
	for _, c := range [...]struct {
		base string
		max  int
	}{{"bytes", MaxSize}, {"text", MaxTextN}} {
		rest, ok := strings.CutPrefix(s, c.base+"[")
		if !ok {
			continue
		}
		digits, ok := strings.CutSuffix(rest, "]")
		if !ok || !isCount(digits) {
			break
		}
		n, err := strconv.Atoi(digits)
		if err != nil || n > c.max {
			return FieldType{}, fmt.Errorf("%s[%s]: N is out of range 1 to %d", c.base, digits, c.max)
		}
		return FieldType{Base: c.base, N: n}, nil
	}
	return FieldType{}, fmt.Errorf("type %q is not a field type", s)
}

// isCount reports whether s matches [1-9][0-9]*: the grammar of the N of bytes[N] and text[N].
func isCount(s string) bool {
	if s == "" || s[0] < '1' || s[0] > '9' {
		return false
	}
	for i := 1; i < len(s); i++ {
		if s[i] < '0' || s[i] > '9' {
			return false
		}
	}
	return true
}

// Size returns the codec bytes of the type: 1 for u8 i8 bool, 2 for u16 i16, 4 for u32 i32 f32, 8 for u64 i64
// f64, 16 for eid, N for bytes[N] and N+1 for text[N].
func (t FieldType) Size() int {
	switch t.Base {
	case "u8", "i8", "bool":
		return 1
	case "u16", "i16":
		return 2
	case "u32", "i32", "f32":
		return 4
	case "u64", "i64", "f64":
		return 8
	case "eid":
		return 16
	case "bytes":
		return t.N
	case "text":
		return t.N + 1
	}
	return 0
}

// GoName returns the Go identifier of a catalog name: each part between underscores with its first letter in upper
// case, "latency_ms" -> "LatencyMs".
func GoName(name string) string {
	var b strings.Builder
	for _, part := range strings.Split(name, "_") {
		if part == "" {
			continue
		}
		if c := part[0]; 'a' <= c && c <= 'z' {
			b.WriteByte(c - 'a' + 'A')
			part = part[1:]
		}
		b.WriteString(part)
	}
	return b.String()
}

// semantic holds the checks that run after the strict pass and the decode; check.go adds the checks of format 1.
var semantic []func(*Catalog) []Refusal

// Parse reads a catalog file of format 1: the strict pass over src, json.Unmarshal, then the semantic checks. It
// returns the catalog, or nil and a *Refused listing every refusal of the first phase that found one.
func Parse(src []byte, name string) (*Catalog, error) {
	rs := strict(src)
	if len(rs) == 0 {
		var c Catalog
		if err := json.Unmarshal(src, &c); err != nil {
			rs = []Refusal{{"$", "decode: " + err.Error()}}
		} else {
			for _, fn := range semantic {
				rs = append(rs, fn(&c)...)
			}
			if len(rs) == 0 {
				return &c, nil
			}
		}
	}
	return nil, &Refused{Name: name, Refusals: sortRefusals(rs)}
}

// sortRefusals orders refusals by (sortKey(Path), Reason) and drops exact duplicates; it reorders rs in place.
func sortRefusals(rs []Refusal) []Refusal {
	slices.SortStableFunc(rs, func(a, b Refusal) int {
		return cmp.Or(
			strings.Compare(sortKey(a.Path), sortKey(b.Path)),
			strings.Compare(a.Reason, b.Reason),
			strings.Compare(a.Path, b.Path),
		)
	})
	return slices.Compact(rs)
}

// sortKey returns path with each index [n] written as [ + n zero-padded to 10 digits + ], so that $.components[2]
// sorts before $.components[10]. A quoted key, ["..."], is copied as it stands.
func sortKey(path string) string {
	var b strings.Builder
	for i := 0; i < len(path); {
		if path[i] != '[' {
			b.WriteByte(path[i])
			i++
			continue
		}
		if strings.HasPrefix(path[i:], `["`) {
			// a quoted key: copy it through its closing quote
			j := i + 2
			for j < len(path) && path[j] != '"' {
				if path[j] == '\\' {
					j++
				}
				j++
			}
			j = min(j+1, len(path))
			b.WriteString(path[i:j])
			i = j
			continue
		}
		j := i + 1
		for j < len(path) && '0' <= path[j] && path[j] <= '9' {
			j++
		}
		if j == i+1 || j == len(path) || path[j] != ']' {
			b.WriteByte('[') // not an index
			i++
			continue
		}
		b.WriteString("[" + strings.Repeat("0", max(0, 10-(j-i-1))) + path[i+1:j] + "]")
		i = j + 1
	}
	return b.String()
}
