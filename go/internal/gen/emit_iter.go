package gen

import (
	"strconv"
	"strings"
)

// The queries of a generated file: the QIDs, the queries of Catalog, Iter of a marked handle and of a declared query,
// and the helpers that Iter shares. A catalog with no query and no mark writes none of it.

// query is a query of the generated file: a marked handle or a declared query. Its QID is its index in printer.queries.
type query struct {
	g    string // GoName of the name
	name string // the name in the catalog
	mark *comp  // the marked component, nil for a declared query
	decl *Query // the declared query, nil for a marked handle
}

// qids writes the QID of each query: the marked handles in CID order, then the declared queries in file order.
func (p *printer) qids() {
	if len(p.queries) == 0 {
		return
	}
	p.w("\n")
	p.cm("The QIDs of the queries of the catalog: the marked handles in CID order, then the declared queries in file order.")
	p.w("const (\n")
	for i := range p.queries {
		p.w("\tQID", p.queries[i].g, " ", p.q("ecd", "QID"), " = ", strconv.Itoa(i), "\n")
	}
	p.w(")\n")
}

// catalogQueries writes the Queries field of Catalog: each query with the fields that it sets, as ecd.QueryDef.
func (p *printer) catalogQueries() {
	if len(p.queries) == 0 {
		return
	}
	p.w("\t\tQueries: []", p.q("ecd", "QueryDef"), "{\n")
	for i := range p.queries {
		q := &p.queries[i]
		p.w("\t\t\t{QID: QID", q.g, ", Name: ", strconv.Quote(q.name))
		if q.mark != nil {
			p.w(", All: []", p.q("ecd", "CID"), "{CID", q.g, "}")
		} else {
			d := q.decl
			if len(d.Kinds) > 0 {
				kinds := make([]string, len(d.Kinds))
				for j, code := range d.Kinds {
					kinds[j] = "Kind" + upperFirst(code)
					if p.c.Detached {
						kinds[j] += "(sys)"
					}
				}
				p.w(", Kinds: []", p.q("ecd", "Kind"), "{", strings.Join(kinds, ", "), "}")
			}
			p.cidField("All", d.All)
			p.cidField("None", d.None)
			p.cidField("Any", d.Any)
			if d.Exact {
				p.w(", Exact: true")
			}
		}
		p.w("},\n")
	}
	p.w("\t\t},\n")
}

// cidField writes ", label: []CID{...}" for the components of names, or nothing for no name.
func (p *printer) cidField(label string, names []string) {
	if len(names) == 0 {
		return
	}
	cids := make([]string, len(names))
	for i, name := range names {
		cids[i] = "CID" + GoName(name)
	}
	p.w(", ", label, ": []", p.q("ecd", "CID"), "{", strings.Join(cids, ", "), "}")
}

// iterWidth is the width at which the comment of Iter wraps, the "// " included: wider than commentWidth, as the text of
// q3 is.
const iterWidth = 118

// iterComment writes the comment of Iter: text wrapped at iterWidth words at a time.
func (p *printer) iterComment(text string) {
	line := "//"
	for _, word := range strings.Fields(text) {
		if line != "//" && len(line)+1+len(word) > iterWidth {
			p.w(line, "\n")
			line = "//"
		}
		line += " " + word
	}
	p.w(line, "\n")
}

// iterDoc is the common ending of the comment of Iter.
func (p *printer) iterDoc() string {
	return "The query runs before Iter returns, so an error of e, " + p.q("ecd", "ErrCold") + " and " +
		p.q("ecd", "ErrQuery") + " among them, comes before any yield. A loop body that moves e panics with an *IterError" +
		" at the next step."
}

// iterHandle writes Iter and QID of the marked handle k, after its CID method.
func (p *printer) iterHandle(k *comp) {
	h := lowerFirst(k.g) + "Handle"
	v := k.g + "Value"
	qid := "QID" + k.g
	name := k.c.Name
	p.w("\n")
	p.iterComment("Iter runs the query of the handle " + name + ", every entity of an All kind that holds the " + name +
		", and returns its answer as a sequence of each entity and a copy of its " + name +
		", in the order of the engine's answer. " + p.iterDoc())
	p.w("func (", h, ") Iter(e ", p.q("ecd", "Engine"), ") (iter.Seq2[", p.q("eid", "ID"), ", ", v, "], error) {\n",
		"\treturn iterValues[", v, "](e, ", qid, ", CID", k.g, ")\n",
		"}\n\n")
	p.cm("QID returns " + qid + ".")
	p.w("func (", h, ") QID() ", p.q("ecd", "QID"), " { return ", qid, " }\n")
	p.scanHandle(k)
}

// rowFields returns the components of the "all" of d that hold a value, in the order of "all".
func (p *printer) rowFields(d *Query) []*comp {
	var out []*comp
	for _, name := range d.All {
		for i := range p.comps {
			if k := &p.comps[i]; k.c.Name == name {
				if !k.tag {
					out = append(out, k)
				}
				break
			}
		}
	}
	return out
}

// query writes a declared query: its var, its type, its row and its Iter and QID.
func (p *printer) query(q *query) {
	d := q.decl
	t := lowerFirst(q.g) + "Query"
	row := q.g + "Row"
	qid := "QID" + q.g
	fields := p.rowFields(d)
	id := p.q("eid", "ID")
	p.w("\n")
	p.doc(q.g+" is the declared query "+d.Name, d.Doc)
	p.w("var ", q.g, " ", t, "\n\n")
	p.doc(t+" runs the query "+d.Name+"; its row is "+row, "")
	p.w("type ", t, " struct{}\n\n")
	p.doc(row+" is a row of the query "+d.Name, `a copy of each component of its "all" that holds a value`)
	if len(fields) == 0 {
		p.w("type ", row, " struct{}\n\n")
	} else {
		p.w("type ", row, " struct {\n")
		for _, f := range fields {
			p.w("\t", f.g, " ", f.g, "Value\n")
		}
		p.w("}\n\n")
	}
	p.iterComment("Iter runs the query " + d.Name + " and returns its answer as a sequence of each entity and its row, in the" +
		" order of the engine's answer. " + p.iterDoc())
	p.w("func (", t, ") Iter(e ", p.q("ecd", "Engine"), ") (iter.Seq2[", id, ", ", row, "], error) {\n",
		"\treturn iterRows(e, ", qid, ", func(id ", id, ", r *", row, ") error {\n")
	for i, f := range fields {
		call := "readValue(e, id, CID" + f.g + ", &r." + f.g + ")"
		if i == len(fields)-1 {
			p.w("\t\treturn ", call, "\n")
			break
		}
		p.w("\t\tif err := ", call, "; err != nil {\n",
			"\t\t\treturn err\n",
			"\t\t}\n")
	}
	if len(fields) == 0 {
		p.w("\t\treturn nil\n")
	}
	p.w("\t})\n",
		"}\n\n")
	p.cm("QID returns " + qid + ".")
	p.w("func (", t, ") QID() ", p.q("ecd", "QID"), " { return ", qid, " }\n")
}

// iterHelpers writes IterError and the helpers of Iter: iterRows always, iterValues when a handle is marked and
// readValue when a handle is marked or a row has a field.
func (p *printer) iterHelpers() {
	if len(p.queries) == 0 {
		return
	}
	marked, read := false, false
	for i := range p.queries {
		q := &p.queries[i]
		if q.mark != nil {
			marked, read = true, true
		} else if len(p.rowFields(q.decl)) > 0 {
			read = true
		}
	}
	var (
		eng = p.q("ecd", "Engine")
		id  = p.q("eid", "ID")
		qid = p.q("ecd", "QID")
		cid = p.q("ecd", "CID")
		ans = p.q("ecd", "Answer")
		pkg = p.c.Package
	)
	p.w("\n")
	p.cm("IterError is the panic value of a sequence of Iter whose engine moved or failed during the loop. At is the engine's",
		"At() when the query ran and Now its At() at the failing step; Err is the error of the step's read, or nil when the",
		"engine moved.")
	p.w("type IterError struct {\n",
		"\tQID ", qid, "\n",
		"\tAt  uint64\n",
		"\tNow uint64\n",
		"\tErr error\n",
		"}\n\n")
	p.cm("Error describes the failed step.")
	p.w("func (e *IterError) Error() string {\n",
		"\tif e.Err != nil {\n",
		"\t\treturn fmt.Sprintf(\"", pkg, ": iteration of query %d at seq %d: %v\", e.QID, e.At, e.Err)\n",
		"\t}\n",
		"\treturn fmt.Sprintf(\"", pkg, ": iteration of query %d at seq %d: the engine moved to seq %d\", e.QID, e.At, e.Now)\n",
		"}\n\n")
	p.cm("Unwrap returns Err.")
	p.w("func (e *IterError) Unwrap() error { return e.Err }\n\n")

	p.cm("iterRows runs query q on e and returns a sequence of each entity of its answer and the row that read fills. The",
		"query runs before iterRows returns; each step panics with an *IterError when e has moved or read fails.")
	p.w("func iterRows[R any](e ", eng, ", q ", qid, ", read func(id ", id, ", r *R) error) (iter.Seq2[", id, ", R], error) {\n",
		"\tat := e.At()\n",
		"\tvar a ", ans, "\n",
		"\tif err := e.Query(q, nil, &a); err != nil {\n",
		"\t\treturn nil, err\n",
		"\t}\n",
		"\treturn func(yield func(", id, ", R) bool) {\n",
		"\t\tfor _, id := range a.IDs {\n",
		"\t\t\tif now := e.At(); now != at {\n",
		"\t\t\t\tpanic(&IterError{QID: q, At: at, Now: now})\n",
		"\t\t\t}\n",
		"\t\t\tvar r R\n",
		"\t\t\tif err := read(id, &r); err != nil {\n",
		"\t\t\t\tpanic(&IterError{QID: q, At: at, Now: at, Err: err})\n",
		"\t\t\t}\n",
		"\t\t\tif !yield(id, r) {\n",
		"\t\t\t\treturn\n",
		"\t\t\t}\n",
		"\t\t}\n",
		"\t}, nil\n",
		"}\n")
	if marked {
		p.w("\n")
		p.cm("iterValues runs query q on e and returns a sequence of each entity of its answer and a copy of its component c.")
		p.w("func iterValues[T any, P decoder[T]](e ", eng, ", q ", qid, ", c ", cid, ") (iter.Seq2[", id, ", T], error) {\n",
			"\treturn iterRows(e, q, func(id ", id, ", v *T) error { return readValue[T, P](e, id, c, v) })\n",
			"}\n")
		p.scanHelper()
	}
	if read {
		p.w("\n")
		p.cm("readValue reads component c of id from e into v. Every error of e, " + p.q("ecd", "ErrAbsent") + " among them, is returned.")
		p.w("func readValue[T any, P decoder[T]](e ", eng, ", id ", id, ", c ", cid, ", v *T) error {\n",
			"\tb, err := e.Get(id, c, nil)\n",
			"\tif err != nil {\n",
			"\t\treturn err\n",
			"\t}\n",
			"\treturn P(v).Decode(b)\n",
			"}\n")
	}
}
