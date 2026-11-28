// Package gen is the generator of typed values and handles from a catalog file of format 1: Parse reads and checks the
// catalog, and Generate writes the gofmt-formatted Go file of it, in one of two import modes.
package gen

import (
	"bytes"
	"cmp"
	"fmt"
	"go/format"
	"slices"
	"strconv"
	"strings"
)

// Mode is the import mode of a generated file.
type Mode uint8

const (
	ModeCore Mode = iota + 1 // imports eid, ecd and rt: for packages of the main module
	ModeECR                  // imports ecr only: for hosts and go/ecr/internal/llm
)

// ParseMode returns the Mode of "core" or "ecr"; any other string is refused.
func ParseMode(s string) (Mode, error) {
	switch s {
	case "core":
		return ModeCore, nil
	case "ecr":
		return ModeECR, nil
	}
	return 0, fmt.Errorf("gen: import mode %q: want \"core\" or \"ecr\"", s)
}

// String returns "core" or "ecr".
func (m Mode) String() string {
	switch m {
	case ModeCore:
		return "core"
	case ModeECR:
		return "ecr"
	}
	return "Mode(" + strconv.Itoa(int(m)) + ")"
}

// The import paths of a generated file are built from these parts, so that no .go file outside go/ecr holds the quoted
// path of package ecr.
const (
	module   = "github.com/fiberfx/ecd-runtime/go"
	idModule = "github.com/fiberfx/ecd-id/go"
)

// qual returns the qualifier of package pkg ("eid", "ecd" or "rt") in mode m: "ecr." in ModeECR.
func (m Mode) qual(pkg string) string {
	if m == ModeECR {
		return "ecr."
	}
	return pkg + "."
}

// Generate returns the gofmt-formatted Go file of the catalog src in mode m. name is the catalog file's base name,
// which the header and the refusals carry; a name that is empty or holds '/' or '\\' is refused with a plain error.
// A refused catalog returns nil and a *Refused. The output depends on src, name and mode only.
func Generate(src []byte, name string, mode Mode) ([]byte, error) {
	if !baseName(name) {
		return nil, fmt.Errorf("gen: catalog name %q: want a base name", name)
	}
	if mode != ModeCore && mode != ModeECR {
		return nil, fmt.Errorf("gen: import mode %s", mode)
	}
	c, err := Parse(src, name)
	if err != nil {
		return nil, err
	}
	p, err := newPrinter(c, name, mode)
	if err != nil {
		return nil, err
	}
	p.emit()
	out, err := format.Source(p.buf.Bytes())
	if err != nil {
		return nil, fmt.Errorf("gen: format: %w", err)
	}
	return out, nil
}

// baseName reports whether name can stand in the header comment of a generated file as the base name of a catalog file.
func baseName(name string) bool {
	if name == "" || strings.ContainsAny(name, "/\\") {
		return false
	}
	for i := 0; i < len(name); i++ {
		if name[i] < 0x20 || name[i] == 0x7f {
			return false
		}
	}
	return true
}

// comp is a component or tag of the catalog with the names and the layout that the emitter derives from it.
type comp struct {
	c      *Component
	g      string    // GoName of the name
	tag    bool      // a tag holds no value
	single bool      // a component of one type, not of fields
	ft     FieldType // the type of a single-type component
	fields []field   // the fields of a component of fields, in declared order
	size   int       // codec bytes of the value; 0 for a tag
}

// field is a field of a component value with its place in the codec bytes.
type field struct {
	name     string // GoName of the name
	ft       FieldType
	off, end int // the bytes [off:end) of the value
}

// needs records what the catalog makes the generated file import and declare.
type needs struct {
	binary, math bool // the imports encoding/binary and math
	component    bool // a component with a value: fmt, sync, errLength, readBuf, readBufs and getValue
	query        bool // a marked handle or a declared query: iter, fmt, the QIDs and the helpers of Iter
	anyText      bool // decodeText
	text         [MaxTextN + 1]bool
	hasBool      bool // appendBool and decodeBool
	hasF32       bool // nan32, appendF32 and decodeF32
	hasF64       bool // nan64, appendF64 and decodeF64
}

// note records that a value of type t is in the catalog.
func (n *needs) note(t FieldType) {
	switch t.Base {
	case "u16", "u32", "u64", "i16", "i32", "i64":
		n.binary = true
	case "f32":
		n.binary, n.math, n.hasF32 = true, true, true
	case "f64":
		n.binary, n.math, n.hasF64 = true, true, true
	case "bool":
		n.hasBool = true
	case "text":
		n.anyText = true
		n.text[t.N] = true
	}
}

// printer holds the catalog, the mode and the text of the generated file before it is formatted.
type printer struct {
	c     *Catalog
	name  string
	mode  Mode
	buf   bytes.Buffer
	comps []comp // components and tags in CID order
	texts []int  // the N of every text[N], ascending
	// queries are the marked handles in CID order, then the declared queries in file order; the QID is the index.
	queries []query
	need    needs
}

// newPrinter derives the order, the layout and the needs of the catalog c, which Parse has accepted.
func newPrinter(c *Catalog, name string, mode Mode) (*printer, error) {
	p := &printer{c: c, name: name, mode: mode}
	sorted := slices.Clone(c.Components)
	slices.SortFunc(sorted, func(a, b Component) int { return cmp.Compare(a.CID, b.CID) })
	p.comps = make([]comp, len(sorted))
	for i := range sorted {
		x := &sorted[i]
		k := comp{c: x, g: GoName(x.Name), tag: x.Form == "tag"}
		switch {
		case k.tag:
		case x.Type != "":
			ft, err := ParseType(x.Type)
			if err != nil {
				return nil, fmt.Errorf("gen: component %q: %w", x.Name, err)
			}
			k.single, k.ft, k.size = true, ft, ft.Size()
			p.need.note(ft)
		default:
			for _, f := range x.Fields {
				ft, err := ParseType(f.Type)
				if err != nil {
					return nil, fmt.Errorf("gen: component %q field %q: %w", x.Name, f.Name, err)
				}
				k.fields = append(k.fields, field{name: GoName(f.Name), ft: ft, off: k.size, end: k.size + ft.Size()})
				k.size += ft.Size()
				p.need.note(ft)
			}
		}
		if !k.tag {
			p.need.component = true
		}
		p.comps[i] = k
	}
	for n, used := range p.need.text {
		if used {
			p.texts = append(p.texts, n)
		}
	}
	for i := range p.comps {
		if k := &p.comps[i]; k.c.Query {
			p.queries = append(p.queries, query{g: k.g, name: k.c.Name, mark: k})
		}
	}
	for i := range c.Queries {
		d := &c.Queries[i]
		p.queries = append(p.queries, query{g: GoName(d.Name), name: d.Name, decl: d})
	}
	p.need.query = len(p.queries) > 0
	return p, nil
}

// emit writes the sections of the file in their order.
func (p *printer) emit() {
	p.header()
	p.imports()
	p.system()
	p.kindCodes()
	p.kinds()
	p.cids()
	p.sizes()
	p.qids()
	p.catalog()
	for _, n := range p.texts {
		p.textType(n)
	}
	for i := range p.comps {
		if !p.comps[i].tag {
			p.valueType(&p.comps[i])
		}
	}
	p.helpers()
	for i := range p.comps {
		p.handle(&p.comps[i])
	}
	p.kindEntities()
	for i := range p.queries {
		if p.queries[i].decl != nil {
			p.query(&p.queries[i])
		}
	}
	p.getters()
	p.iterHelpers()
}

// w writes the parts of s to the file as they stand.
func (p *printer) w(s ...string) {
	for _, x := range s {
		p.buf.WriteString(x)
	}
}

// q returns the name of package pkg ("eid", "ecd" or "rt") qualified in the mode of the file. Every qualified name of
// the generated file comes from here.
func (p *printer) q(pkg, name string) string { return p.mode.qual(pkg) + name }

// header writes the header line and the package clause.
func (p *printer) header() {
	p.w("// Code generated by ecd-runtime/go/cmd/gen from ", p.name, "; DO NOT EDIT.\n\npackage ", p.c.Package, "\n\n")
}

// imports writes the import block: the standard library, then the packages of the mode.
func (p *printer) imports() {
	p.w("import (\n")
	if p.need.binary {
		p.w("\t\"encoding/binary\"\n")
	}
	p.w("\t\"errors\"\n")
	if p.need.component || p.need.query {
		p.w("\t\"fmt\"\n")
	}
	if p.need.query {
		p.w("\t\"iter\"\n")
	}
	if p.need.math {
		p.w("\t\"math\"\n")
	}
	if p.need.component {
		p.w("\t\"sync\"\n")
	}
	p.w("\n")
	switch p.mode {
	case ModeECR:
		p.w("\t", strconv.Quote(module+"/"+"ecr"), "\n")
	default:
		p.w("\teid ", strconv.Quote(idModule), "\n")
		p.w("\t", strconv.Quote(module+"/"+"ecd"), "\n")
		p.w("\t", strconv.Quote(module+"/"+"rt"), "\n")
	}
	p.w(")\n")
}
