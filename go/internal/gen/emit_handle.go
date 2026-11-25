package gen

// The handles of a generated file and the three helpers they share: decoder, getValue and has.

// handle writes the handle of a component or tag: a var of an unexported type whose methods read and write the
// component through an engine and a Buffer.
func (p *printer) handle(k *comp) {
	var (
		eng    = p.q("ecd", "Engine")
		id     = p.q("eid", "ID")
		buf    = "*" + p.q("rt", "Buffer")
		ref    = p.q("rt", "Ref")
		h      = lowerFirst(k.g) + "Handle"
		cid    = "CID" + k.g
		kindOf = "component"
	)
	if k.tag {
		kindOf = "tag"
	}
	p.w("\n")
	p.doc(k.g+" is the handle of the "+kindOf+" "+k.c.Name, k.c.Doc)
	p.w("var ", k.g, " ", h, "\n\n")
	if k.tag {
		p.cm(h + " reads and writes the tag " + k.c.Name + ".")
	} else {
		p.cm(h + " reads and writes the component " + k.c.Name + "; its value is " + k.g + "Value.")
	}
	p.w("type ", h, " struct{}\n\n")

	if !k.tag {
		v := k.g + "Value"
		p.cm("Get returns a copy of the "+k.c.Name+" of id. An entity or component that e reports absent answers false and nil;",
			"every other error of e, "+p.q("ecd", "ErrMiss")+" and "+p.q("ecd", "ErrCold")+" among them, is returned with false.")
		p.w("func (", h, ") Get(e ", eng, ", id ", id, ") (", v, ", bool, error) {\n",
			"\treturn getValue[", v, "](e, id, ", cid, ")\n",
			"}\n\n")
	}
	if k.tag {
		p.cm("Has reports whether id holds the tag " + k.c.Name + ". Absent answers false and nil; every other error of e is returned.")
	} else {
		p.cm("Has reports whether id holds the " + k.c.Name + ". Absent answers false and nil; every other error of e is returned.")
	}
	p.w("func (", h, ") Has(e ", eng, ", id ", id, ") (bool, error) { return has(e, id, ", cid, ") }\n\n")

	if k.tag {
		p.cm("Add records in b an add of the tag " + k.c.Name + " to r.")
		p.w("func (", h, ") Add(b ", buf, ", r ", ref, ") { b.Set(r, ", cid, ", nil) }\n\n")
		p.cm("Remove records in b a removal of the tag " + k.c.Name + " from r.")
		p.w("func (", h, ") Remove(b ", buf, ", r ", ref, ") { b.Unset(r, ", cid, ") }\n\n")
	} else {
		p.cm("Set records in b a set of the " + k.c.Name + " of r to v.")
		p.w("func (", h, ") Set(b ", buf, ", r ", ref, ", v ", k.g, "Value) {\n",
			"\tvar a [Size", k.g, "]byte\n",
			"\tb.Set(r, ", cid, ", v.Append(a[:0]))\n",
			"}\n\n")
		p.cm("Unset records in b an unset of the " + k.c.Name + " of r.")
		p.w("func (", h, ") Unset(b ", buf, ", r ", ref, ") { b.Unset(r, ", cid, ") }\n\n")
	}
	p.cm("CID returns " + cid + ".")
	p.w("func (", h, ") CID() ", p.q("ecd", "CID"), " { return ", cid, " }\n")
	if k.c.Query {
		p.iterHandle(k)
	}
}

// getters writes decoder and getValue, when a component has a value to read, and has.
func (p *printer) getters() {
	eng := p.q("ecd", "Engine")
	id := p.q("eid", "ID")
	cid := p.q("ecd", "CID")
	absent := p.q("ecd", "ErrAbsent")
	if p.need.component {
		p.w("\n")
		p.cm("decoder is the pointer type of a generated value type: Decode sets the value from its codec bytes.")
		p.w("type decoder[T any] interface {\n",
			"\t*T\n",
			"\tDecode(b []byte) error\n",
			"}\n\n")
		p.cm("getValue reads component c of id from e and decodes it. An error that matches "+absent+" answers the zero T,",
			"false and nil; every other error is returned.")
		p.w("func getValue[T any, P decoder[T]](e ", eng, ", id ", id, ", c ", cid, ") (T, bool, error) {\n",
			"\tvar v T\n",
			"\tb, err := e.Get(id, c, nil)\n",
			"\tif errors.Is(err, ", absent, ") {\n",
			"\t\treturn v, false, nil\n",
			"\t}\n",
			"\tif err != nil {\n",
			"\t\treturn v, false, err\n",
			"\t}\n",
			"\tif err := P(&v).Decode(b); err != nil {\n",
			"\t\treturn v, false, err\n",
			"\t}\n",
			"\treturn v, true, nil\n",
			"}\n")
	}
	p.w("\n")
	p.cm("has reports whether id holds component or tag c. An error that matches " + absent + " answers false and nil.")
	p.w("func has(e ", eng, ", id ", id, ", c ", cid, ") (bool, error) {\n",
		"\t_, err := e.Get(id, c, nil)\n",
		"\tswitch {\n",
		"\tcase err == nil:\n",
		"\t\treturn true, nil\n",
		"\tcase errors.Is(err, ", absent, "):\n",
		"\t\treturn false, nil\n",
		"\t}\n",
		"\treturn false, err\n",
		"}\n")
}
