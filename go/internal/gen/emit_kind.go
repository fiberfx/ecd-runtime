package gen

// The whole-entity types of the kinds that list components: one struct, one typed spawn and one whole-entity read
// each.

// kindEntities writes, for each kind in file order that lists components, the type K Entity, SpawnK and GetK.
func (p *printer) kindEntities() {
	for _, k := range p.c.Kinds {
		if len(k.Components) > 0 {
			p.kindEntity(k)
		}
	}
}

// kindEntity writes the entity type, the spawn and the read of the kind k. A required member is set by every spawn and
// absent from no whole entity; an optional one has a Has field that the spawn reads and the read fills.
func (p *printer) kindEntity(k Kind) {
	var (
		K        = upperFirst(k.Code)
		ent      = K + "Entity"
		eng      = p.q("ecd", "Engine")
		id       = p.q("eid", "ID")
		buf      = "*" + p.q("rt", "Buffer")
		ref      = p.q("rt", "Ref")
		errKind  = p.q("ecd", "ErrKind")
		sysArg   = ""
		sysCall  = ""
		names    = make([]string, len(k.Components))
		required = 0
	)
	for i, m := range k.Components {
		names[i] = GoName(m.Name)
		if !m.Optional {
			required++
		}
	}
	sysIs := "id.System() != System"
	kind := "Kind" + K
	if p.c.Detached {
		sysArg = "sys " + p.q("eid", "Code") + ", "
		sysCall = "(sys)"
		sysIs = ""
	}
	p.w("\n")
	p.cm(ent+" is a whole entity of the kind "+k.Code+": the value of each component the kind lists, in its order, and for",
		"each optional one a Has field, true when the entity holds it; an absent optional component reads as its zero value.")
	p.w("type ", ent, " struct {\n")
	for i, g := range names {
		p.w("\t", g, " ", g, "Value\n")
		if k.Components[i].Optional {
			p.w("\tHas", g, " bool\n")
		}
	}
	p.w("}\n\n")

	p.cm("Spawn"+K+" records in b a spawn of an entity of the kind "+k.Code+" and then a set of each component of v, in the kind's",
		"order, an optional one only when its Has field is true, and returns the entity's placeholder.")
	p.w("func Spawn", K, "(b ", buf, ", ", sysArg, "v ", ent, ") ", ref, " {\n",
		"\tr := b.Spawn(", kind, sysCall, ")\n")
	for i, g := range names {
		if k.Components[i].Optional {
			p.w("\tif v.Has", g, " {\n",
				"\t\t", g, ".Set(b, r, v.", g, ")\n",
				"\t}\n")
		} else {
			p.w("\t", g, ".Set(b, r, v.", g, ")\n")
		}
	}
	p.w("\treturn r\n}\n\n")

	absent := "an entity that lacks a required component answers false and nil"
	if required == 0 {
		absent = "an entity that holds none of the listed components answers false and nil"
	}
	p.cm("Get"+K+" reads the whole entity id of the kind "+k.Code+" from e. An id of another kind answers an error that matches",
		p.q("ecd", "ErrKind")+"; "+absent+"; every other error of e is returned with false.")
	p.w("func Get", K, "(e ", eng, ", id ", id, ") (", ent, ", bool, error) {\n",
		"\tvar v ", ent, "\n")
	cond := "id.Kind() != Code" + K
	if sysIs != "" {
		cond = sysIs + " || " + cond
	}
	p.w("\tif ", cond, " {\n",
		"\t\treturn v, false, fmt.Errorf(\"", p.c.Package, ": Get", K, ": %v is not of the kind ", k.Code, ": %w\", id, ", errKind, ")\n",
		"\t}\n")
	if required > 0 {
		p.w("\tvar (\n\t\tok  bool\n\t\terr error\n\t)\n")
	} else {
		p.w("\tvar err error\n")
	}
	var has []string
	for i, g := range names {
		if k.Components[i].Optional {
			has = append(has, "v.Has"+g)
			p.w("\tif v.", g, ", v.Has", g, ", err = ", g, ".Get(e, id); err != nil {\n",
				"\t\treturn ", ent, "{}, false, err\n",
				"\t}\n")
		} else {
			p.w("\tif v.", g, ", ok, err = ", g, ".Get(e, id); !ok || err != nil {\n",
				"\t\treturn ", ent, "{}, false, err\n",
				"\t}\n")
		}
	}
	if required == 0 {
		cond := "!" + has[0]
		if len(has) > 1 {
			cond = "!(" + has[0]
			for _, h := range has[1:] {
				cond += " || " + h
			}
			cond += ")"
		}
		p.w("\tif ", cond, " {\n",
			"\t\treturn ", ent, "{}, false, nil\n",
			"\t}\n")
	}
	p.w("\treturn v, true, nil\n}\n")
}
