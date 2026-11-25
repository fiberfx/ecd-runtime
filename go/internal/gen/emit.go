package gen

import (
	"strconv"
	"strings"
)

// The sections of a generated file, but its handles (emit_handle.go). Every section starts with a blank line that
// separates it from the one before, and go/format.Source lays out the rest, so the text here carries no alignment.

// commentWidth is the width at which a comment built from a doc string wraps, the "// " included.
const commentWidth = 112

// wrapComment returns the comment lines of text, wrapped at commentWidth words at a time.
func wrapComment(text string) []string {
	var lines []string
	line := "//"
	for _, word := range strings.Fields(text) {
		if line != "//" && len(line)+1+len(word) > commentWidth {
			lines = append(lines, line)
			line = "//"
		}
		line += " " + word
	}
	return append(lines, line)
}

// cm writes one comment line for each of lines.
func (p *printer) cm(lines ...string) {
	for _, l := range lines {
		p.w("// ", l, "\n")
	}
}

// doc writes the comment of a declaration: the subject, then ": " and the doc when there is one, then a full stop
// unless the text ends with one already. A long comment wraps at commentWidth.
func (p *printer) doc(subject, doc string) {
	s := subject
	if doc != "" {
		s += ": " + doc
	}
	if !strings.HasSuffix(s, ".") {
		s += "."
	}
	for _, l := range wrapComment(s) {
		p.w(l, "\n")
	}
}

// nbytes returns "N adjbytes", with "byte" for N = 1: nbytes(16, "codec ") is "16 codec bytes".
func nbytes(n int, adj string) string {
	s := strconv.Itoa(n) + " " + adj + "byte"
	if n != 1 {
		s += "s"
	}
	return s
}

// upperFirst returns s with its first letter in upper case: a kind code "ses" gives "Ses".
func upperFirst(s string) string { return strings.ToUpper(s[:1]) + s[1:] }

// lowerFirst returns s with its first letter in lower case: the Go name "Session" gives "session".
func lowerFirst(s string) string { return strings.ToLower(s[:1]) + s[1:] }

// system writes the System of a registered catalog; a detached catalog has none.
func (p *printer) system() {
	if p.c.Detached {
		return
	}
	p.w("\n")
	p.cm("System is the system of the catalog.")
	p.w("var System = ", p.q("eid", "MustCode"), "(", strconv.Quote(p.c.System), ")\n")
}

// kindCodes writes the code of each kind, in file order.
func (p *printer) kindCodes() {
	p.w("\n")
	p.cm("The codes of the kinds of the catalog.")
	p.w("var (\n")
	for _, k := range p.c.Kinds {
		p.w("\tCode", upperFirst(k.Code), " = ", p.q("eid", "MustCode"), "(", strconv.Quote(k.Code), ")\n")
	}
	p.w(")\n")
}

// kinds writes the kinds, in file order: a var each for a registered system, a function each for a detached one.
func (p *printer) kinds() {
	p.w("\n")
	if p.c.Detached {
		for i, k := range p.c.Kinds {
			K := upperFirst(k.Code)
			if i > 0 {
				p.w("\n")
			}
			p.doc("Kind"+K+" returns the kind "+k.Code+" of the system sys, "+upperFirst(k.Residency), k.Doc)
			p.w("func Kind", K, "(sys ", p.q("eid", "Code"), ") ", p.q("ecd", "Kind"),
				" { return ", p.q("ecd", "Kind"), "{System: sys, Kind: Code", K, "} }\n")
		}
		return
	}
	p.cm("The kinds of the catalog.")
	p.w("var (\n")
	for _, k := range p.c.Kinds {
		K := upperFirst(k.Code)
		p.doc("Kind"+K+" is the kind "+k.Code+", "+upperFirst(k.Residency), k.Doc)
		p.w("Kind", K, " = ", p.q("ecd", "Kind"), "{System: System, Kind: Code", K, "}\n")
	}
	p.w(")\n")
}

// cids writes the CID of each component and tag, in CID order.
func (p *printer) cids() {
	p.w("\n")
	p.cm("The CIDs of the components and tags of the catalog.")
	p.w("const (\n")
	for i := range p.comps {
		k := &p.comps[i]
		p.w("\tCID", k.g, " ", p.q("ecd", "CID"), " = ", strconv.FormatInt(k.c.CID, 10), "\n")
	}
	p.w(")\n")
}

// sizes writes the Size of the value of each component, in CID order; a tag has none.
func (p *printer) sizes() {
	if !p.need.component {
		return
	}
	p.w("\n")
	p.cm("The sizes of the component values in the codec, in bytes.")
	p.w("const (\n")
	for i := range p.comps {
		if k := &p.comps[i]; !k.tag {
			p.w("\tSize", k.g, " = ", strconv.Itoa(k.size), "\n")
		}
	}
	p.w(")\n")
}

// catalog writes Catalog: of the system for a registered catalog, of the system sys for a detached one.
func (p *printer) catalog() {
	p.w("\n")
	if p.c.Detached {
		p.cm("Catalog returns the catalog of " + p.name + " for the system sys as a new value that the caller owns; it does not check sys.")
		p.w("func Catalog(sys ", p.q("eid", "Code"), ") *", p.q("ecd", "Catalog"), " {\n")
	} else {
		p.cm("Catalog returns the catalog of " + p.name + " as a new value that the caller owns.")
		p.w("func Catalog() *", p.q("ecd", "Catalog"), " {\n")
	}
	p.w("\treturn &", p.q("ecd", "Catalog"), "{\n\t\tComponents: []", p.q("ecd", "ComponentDef"), "{\n")
	for i := range p.comps {
		k := &p.comps[i]
		p.w("\t\t\t{CID: CID", k.g, ", Name: ", strconv.Quote(k.c.Name), ", Form: ")
		if k.tag {
			p.w(p.q("ecd", "Tag"), "},\n")
		} else {
			p.w(p.q("ecd", "Component"), ", Size: Size", k.g, "},\n")
		}
	}
	p.w("\t\t},\n\t\tKinds: []", p.q("ecd", "KindDef"), "{\n")
	for _, k := range p.c.Kinds {
		K := upperFirst(k.Code)
		kind := "Kind" + K
		if p.c.Detached {
			kind += "(sys)"
		}
		p.w("\t\t\t{Kind: ", kind, ", Residency: ", p.q("ecd", upperFirst(k.Residency)), ", Budget: ",
			strconv.FormatInt(k.Budget, 10), "},\n")
	}
	p.w("\t\t},\n")
	p.catalogQueries()
	p.w("\t}\n}\n")
}

// textType writes the type TextN: a text of at most n bytes, its constructor and its codec.
func (p *printer) textType(n int) {
	t := "Text" + strconv.Itoa(n)
	ns, codec := strconv.Itoa(n), strconv.Itoa(n+1)
	ev := p.q("ecd", "ErrValue")
	p.w("\n")
	p.cm(t+" is a text of at most "+ns+" bytes. Its codec bytes are one length byte, then "+ns+" bytes: the text, then zero",
		"bytes. The zero "+t+" is the empty text; New"+t+" makes any other.")
	p.w("type ", t, " struct {\n\tn uint8\n\tb [", ns, "]byte\n}\n\n")
	p.cm("New"+t+" returns s as a "+t+". A string of more than "+ns+" bytes is refused with an error that matches",
		ev+".")
	p.w("func New", t, "(s string) (", t, ", error) {\n",
		"\tvar t ", t, "\n",
		"\tif len(s) > len(t.b) {\n",
		"\t\treturn ", t, "{}, fmt.Errorf(\"%w: ", t, ": %d bytes, at most %d\", ", ev, ", len(s), len(t.b))\n",
		"\t}\n",
		"\tt.n = uint8(copy(t.b[:], s))\n",
		"\treturn t, nil\n",
		"}\n\n")
	p.cm("String returns the text.")
	p.w("func (t ", t, ") String() string { return string(t.b[:t.n]) }\n\n")
	p.cm("Len returns the length of the text in bytes.")
	p.w("func (t ", t, ") Len() int { return int(t.n) }\n\n")
	p.cm("Append appends the " + nbytes(n+1, "codec ") + " of t to dst and returns the extended slice.")
	p.w("func (t ", t, ") Append(dst []byte) []byte {\n",
		"\tdst = append(dst, t.n)\n",
		"\treturn append(dst, t.b[:]...)\n",
		"}\n\n")
	p.cm("Decode sets t from the "+nbytes(n+1, "codec ")+" of b. A b of another length, or with a byte that Append does not write,",
		"is refused with an error that matches "+ev+", and t is unchanged.")
	p.w("func (t *", t, ") Decode(b []byte) error { return t.decode(b, \"", t, "\") }\n\n")
	p.w("func (t *", t, ") decode(b []byte, name string) error {\n",
		"\tif len(b) != ", codec, " {\n",
		"\t\treturn errLength(name, len(b), ", codec, ")\n",
		"\t}\n",
		"\tvar x ", t, "\n",
		"\tif err := decodeText(x.b[:], &x.n, b, name); err != nil {\n",
		"\t\treturn err\n",
		"\t}\n",
		"\t*t = x\n",
		"\treturn nil\n",
		"}\n")
}

// goType returns the Go type of a field type in the generated file.
func (p *printer) goType(t FieldType) string {
	switch t.Base {
	case "u8", "u16", "u32", "u64":
		return "uint" + t.Base[1:]
	case "i8", "i16", "i32", "i64":
		return "int" + t.Base[1:]
	case "f32", "f64":
		return "float" + t.Base[1:]
	case "bool":
		return "bool"
	case "eid":
		return p.q("eid", "ID")
	case "bytes":
		return "[" + strconv.Itoa(t.N) + "]byte"
	case "text":
		return "Text" + strconv.Itoa(t.N)
	}
	panic("gen: no Go type for the field type " + t.Base)
}

// singleX returns the expression that the codec statements use for the receiver v of a single-type value: v converted
// to its base Go type, and v itself for eid and bytes, whose statement slices it.
func (p *printer) singleX(t FieldType) string {
	switch t.Base {
	case "eid", "bytes":
		return "v"
	}
	return p.goType(t) + "(v)"
}

// appendStmt returns the statement of Append that writes the codec bytes of x, a value of type t, to dst.
func appendStmt(t FieldType, x string) string {
	switch t.Base {
	case "u8":
		return "dst = append(dst, " + x + ")"
	case "i8":
		return "dst = append(dst, uint8(" + x + "))"
	case "bool":
		return "dst = appendBool(dst, " + x + ")"
	case "u16":
		return "dst = binary.BigEndian.AppendUint16(dst, " + x + ")"
	case "u32":
		return "dst = binary.BigEndian.AppendUint32(dst, " + x + ")"
	case "u64":
		return "dst = binary.BigEndian.AppendUint64(dst, " + x + ")"
	case "i16":
		return "dst = binary.BigEndian.AppendUint16(dst, uint16(" + x + "))"
	case "i32":
		return "dst = binary.BigEndian.AppendUint32(dst, uint32(" + x + "))"
	case "i64":
		return "dst = binary.BigEndian.AppendUint64(dst, uint64(" + x + "))"
	case "f32":
		return "dst = appendF32(dst, " + x + ")"
	case "f64":
		return "dst = appendF64(dst, " + x + ")"
	case "eid", "bytes":
		return "dst = append(dst, " + x + "[:]...)"
	case "text":
		return "dst = " + x + ".Append(dst)"
	}
	panic("gen: no Append statement for the field type " + t.Base)
}

// decodeStmt returns the statements of Decode that read a value of type t from the bytes [off:end) of b into target,
// which is x for a single type and x.Field for a field; name is what a refusal calls the value.
func decodeStmt(t FieldType, target string, off, end int, name string) string {
	s := "b[" + strconv.Itoa(off) + "]"
	if end-off != 1 {
		s = "b[" + strconv.Itoa(off) + ":" + strconv.Itoa(end) + "]"
	}
	checked := func(call, arg string) string {
		return "if err := " + call + "(" + arg + ", " + s + ", " + strconv.Quote(name) + "); err != nil {\n\treturn err\n}\n"
	}
	w := t.Base[1:]
	switch t.Base {
	case "u8":
		return target + " = " + s + "\n"
	case "i8":
		return target + " = int8(" + s + ")\n"
	case "u16", "u32", "u64":
		return target + " = binary.BigEndian.Uint" + w + "(" + s + ")\n"
	case "i16", "i32", "i64":
		return target + " = int" + w + "(binary.BigEndian.Uint" + w + "(" + s + "))\n"
	case "bool":
		return checked("decodeBool", "&"+target)
	case "f32", "f64":
		return checked("decodeF"+w, "&"+target)
	case "eid", "bytes":
		return "copy(" + target + "[:], " + s + ")\n"
	case "text":
		// the text's own decode takes the bytes first
		return "if err := " + target + ".decode(" + s + ", " + strconv.Quote(name) + "); err != nil {\n\treturn err\n}\n"
	}
	panic("gen: no Decode statement for the field type " + t.Base)
}

// valueType writes the value type of a component, with its Append and Decode.
func (p *printer) valueType(k *comp) {
	v := k.g + "Value"
	ev := p.q("ecd", "ErrValue")
	subject := v + " is the value of the component " + k.c.Name + ", "
	if k.single && k.ft.Base == "text" {
		subject += "a " + p.goType(k.ft) + " of "
	}
	p.w("\n")
	p.doc(subject+nbytes(k.size, "")+" in the codec", k.c.Doc)
	if k.single && k.ft.Base == "text" {
		p.w("type ", v, " = ", p.goType(k.ft), "\n") // the text type has the codec already
		return
	}
	if k.single {
		p.w("type ", v, " ", p.goType(k.ft), "\n\n")
	} else {
		p.w("type ", v, " struct {\n")
		for _, f := range k.fields {
			p.w("\t", f.name, " ", p.goType(f.ft), "\n")
		}
		p.w("}\n\n")
	}
	p.cm("Append appends the " + nbytes(k.size, "codec ") + " of v to dst and returns the extended slice.")
	p.w("func (v ", v, ") Append(dst []byte) []byte {\n")
	if k.single {
		p.w("\t", appendStmt(k.ft, p.singleX(k.ft)), "\n")
	} else {
		for _, f := range k.fields {
			p.w("\t", appendStmt(f.ft, "v."+f.name), "\n")
		}
	}
	p.w("\treturn dst\n}\n\n")
	p.cm("Decode sets v from the "+nbytes(k.size, "codec ")+" of b. A b of another length, or with a byte that Append does not write,",
		"is refused with an error that matches "+ev+", and v is unchanged.")
	p.w("func (v *", v, ") Decode(b []byte) error {\n",
		"\tif len(b) != Size", k.g, " {\n",
		"\t\treturn errLength(", strconv.Quote(v), ", len(b), Size", k.g, ")\n",
		"\t}\n")
	if k.single {
		p.w("\tvar x ", p.goType(k.ft), "\n")
		p.w(decodeStmt(k.ft, "x", 0, k.size, v))
		p.w("\t*v = ", v, "(x)\n")
	} else {
		p.w("\tvar x ", v, "\n")
		for _, f := range k.fields {
			p.w(decodeStmt(f.ft, "x."+f.name, f.off, f.end, v+"."+f.name))
		}
		p.w("\t*v = x\n")
	}
	p.w("\treturn nil\n}\n")
}

// helpers writes the codec helpers that the values of the catalog use, in a fixed order.
func (p *printer) helpers() {
	ev := p.q("ecd", "ErrValue")
	if p.need.component {
		p.w("\n")
		p.cm("errLength is the refusal of a value of n bytes where the codec takes want.")
		p.w("func errLength(name string, n, want int) error {\n",
			"\treturn fmt.Errorf(\"%w: %s: %d bytes, want %d\", ", ev, ", name, n, want)\n",
			"}\n")
	}
	if p.need.anyText {
		p.w("\n")
		p.cm("decodeText reads a text of the codec from b into dst and n: one length byte of at most len(dst), then len(dst)",
			"bytes whose bytes after the text are zero. b holds 1+len(dst) bytes.")
		p.w("func decodeText(dst []byte, n *uint8, b []byte, name string) error {\n",
			"\tl := int(b[0])\n",
			"\tif l > len(dst) {\n",
			"\t\treturn fmt.Errorf(\"%w: %s: length %d, at most %d\", ", ev, ", name, l, len(dst))\n",
			"\t}\n",
			"\tfor i := 1 + l; i < len(b); i++ {\n",
			"\t\tif b[i] != 0 {\n",
			"\t\t\treturn fmt.Errorf(\"%w: %s: byte %d after the text is not zero\", ", ev, ", name, i)\n",
			"\t\t}\n",
			"\t}\n",
			"\tcopy(dst, b[1:])\n",
			"\t*n = uint8(l)\n",
			"\treturn nil\n",
			"}\n")
	}
	if p.need.hasBool {
		p.w("\n")
		p.cm("appendBool appends the codec byte of v: 1 for true, 0 for false.")
		p.w("func appendBool(dst []byte, v bool) []byte {\n",
			"\tif v {\n",
			"\t\treturn append(dst, 1)\n",
			"\t}\n",
			"\treturn append(dst, 0)\n",
			"}\n\n")
		p.cm("decodeBool reads the codec byte c of a bool into v: 0 or 1; any other byte is refused.")
		p.w("func decodeBool(v *bool, c byte, name string) error {\n",
			"\tif c > 1 {\n",
			"\t\treturn fmt.Errorf(\"%w: %s: byte 0x%02x, want 0 or 1\", ", ev, ", name, c)\n",
			"\t}\n",
			"\t*v = c == 1\n",
			"\treturn nil\n",
			"}\n")
	}
	if p.need.hasF32 || p.need.hasF64 {
		p.w("\n")
		p.cm("The canonical NaNs of the codec: the only NaN bits that Append writes and Decode accepts.")
		p.w("const (\n")
		if p.need.hasF32 {
			p.w("\tnan32 = 0x7fc00000\n")
		}
		if p.need.hasF64 {
			p.w("\tnan64 = 0x7ff8000000000000\n")
		}
		p.w(")\n")
	}
	if p.need.hasF32 {
		p.w("\n")
		p.cm("appendF32 appends the codec bytes of v: IEEE 754 binary32, big-endian, every NaN as nan32 and -0 as 0.")
		p.w("func appendF32(dst []byte, v float32) []byte {\n",
			"\tbits := math.Float32bits(v)\n",
			"\tswitch {\n",
			"\tcase math.IsNaN(float64(v)):\n",
			"\t\tbits = nan32\n",
			"\tcase v == 0:\n",
			"\t\tbits = 0\n",
			"\t}\n",
			"\treturn binary.BigEndian.AppendUint32(dst, bits)\n",
			"}\n\n")
		p.cm("decodeF32 reads the 4 codec bytes of b into v. A NaN other than nan32 and a negative zero are refused.")
		p.w("func decodeF32(v *float32, b []byte, name string) error {\n",
			"\tbits := binary.BigEndian.Uint32(b)\n",
			"\tf := math.Float32frombits(bits)\n",
			"\tif math.IsNaN(float64(f)) && bits != nan32 {\n",
			"\t\treturn fmt.Errorf(\"%w: %s: NaN 0x%08x, want 0x%08x\", ", ev, ", name, bits, nan32)\n",
			"\t}\n",
			"\tif bits == 1<<31 {\n",
			"\t\treturn fmt.Errorf(\"%w: %s: negative zero\", ", ev, ", name)\n",
			"\t}\n",
			"\t*v = f\n",
			"\treturn nil\n",
			"}\n")
	}
	if p.need.hasF64 {
		p.w("\n")
		p.cm("appendF64 appends the codec bytes of v: IEEE 754 binary64, big-endian, every NaN as nan64 and -0 as 0.")
		p.w("func appendF64(dst []byte, v float64) []byte {\n",
			"\tbits := math.Float64bits(v)\n",
			"\tswitch {\n",
			"\tcase math.IsNaN(v):\n",
			"\t\tbits = nan64\n",
			"\tcase v == 0:\n",
			"\t\tbits = 0\n",
			"\t}\n",
			"\treturn binary.BigEndian.AppendUint64(dst, bits)\n",
			"}\n\n")
		p.cm("decodeF64 reads the 8 codec bytes of b into v. A NaN other than nan64 and a negative zero are refused.")
		p.w("func decodeF64(v *float64, b []byte, name string) error {\n",
			"\tbits := binary.BigEndian.Uint64(b)\n",
			"\tf := math.Float64frombits(bits)\n",
			"\tif math.IsNaN(f) && bits != nan64 {\n",
			"\t\treturn fmt.Errorf(\"%w: %s: NaN 0x%016x, want 0x%016x\", ", ev, ", name, bits, uint64(nan64))\n",
			"\t}\n",
			"\tif bits == 1<<63 {\n",
			"\t\treturn fmt.Errorf(\"%w: %s: negative zero\", ", ev, ", name)\n",
			"\t}\n",
			"\t*v = f\n",
			"\treturn nil\n",
			"}\n")
	}
}
