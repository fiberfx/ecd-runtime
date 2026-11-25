package gen

// The scan of a generated file: Scan of a marked handle, which reads the owner store through ecd.Scanner, and the
// helper scanValues that it calls. A catalog with no marked handle writes none of it.

// scanHandle writes Scan of the marked handle k, after its Iter and QID.
func (p *printer) scanHandle(k *comp) {
	h := lowerFirst(k.g) + "Handle"
	v := k.g + "Value"
	id := p.q("eid", "ID")
	name := k.c.Name
	p.w("\n")
	p.iterComment("Scan calls f with each entity of kind k in s whose eid is above after and that holds the " + name +
		", and a copy of its " + name + ", in eid order, from one read transaction of the store; the zero after starts" +
		" at the first entity of k. It reads the store and no engine, so it answers a kind that an engine holds Cold." +
		" A stored value that does not decode (" + p.q("ecd", "ErrValue") + "), an error of s and an error of f stop" +
		" the scan and are returned; f must not call s.")
	p.w("func (", h, ") Scan(s ", p.q("ecd", "Scanner"), ", k ", p.q("ecd", "Kind"), ", after ", id,
		", f func(id ", id, ", v ", v, ") error) error {\n",
		"\treturn scanValues[", v, "](s, k, CID", k.g, ", after, f)\n",
		"}\n")
}

// scanHelper writes scanValues, which Scan of every marked handle calls.
func (p *printer) scanHelper() {
	id := p.q("eid", "ID")
	p.w("\n")
	p.cm("scanValues calls f with each entity of kind k above after in s that holds component c, and a copy of its c, in",
		"eid order. A value that does not decode stops the scan with an error that names the entity; an error of s or of",
		"f is returned unchanged.")
	p.w("func scanValues[T any, P decoder[T]](s ", p.q("ecd", "Scanner"), ", k ", p.q("ecd", "Kind"), ", c ",
		p.q("ecd", "CID"), ", after ", id, ", f func(id ", id, ", v T) error) error {\n",
		"\treturn s.Scan(k, c, after, func(_ uint64, src ", p.q("ecd", "RowSource"), ") error {\n",
		"\t\tvar r ", p.q("ecd", "Row"), "\n",
		"\t\tfor {\n",
		"\t\t\tok, err := src.Next(&r)\n",
		"\t\t\tif err != nil || !ok {\n",
		"\t\t\t\treturn err\n",
		"\t\t\t}\n",
		"\t\t\tvar v T\n",
		"\t\t\tif err := P(&v).Decode(r.Value); err != nil {\n",
		"\t\t\t\treturn fmt.Errorf(\"", p.c.Package, ": scan of %v: %w\", r.Entity, err)\n",
		"\t\t\t}\n",
		"\t\t\tif err := f(r.Entity, v); err != nil {\n",
		"\t\t\t\treturn err\n",
		"\t\t\t}\n",
		"\t\t}\n",
		"\t})\n",
		"}\n")
}
