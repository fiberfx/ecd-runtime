package gen

import (
	"cmp"
	"fmt"
	"regexp"
	"slices"
	"strings"

	eid "github.com/fiberfx/ecd-id/go"
)

// The semantic checks of format 1 run on a catalog that passed the strict pass and decoded. For each value they run
// in the order of the table of the design and stop at the first that fails, so a value has at most one refusal.

func init() { semantic = append(semantic, check) }

// maxCID is the largest component id: ecd.CID is a uint16, and 0 is refused by both engines.
const maxCID = 65535

const (
	packageGrammar = `^[a-z][a-z0-9]*$`
	nameGrammar    = `^[a-z][a-z0-9]*(_[a-z][a-z0-9]*)*$`
)

var (
	packageRE = regexp.MustCompile(packageGrammar)
	nameRE    = regexp.MustCompile(nameGrammar)
)

// reservedPackages holds the 25 Go keywords and the package names that the generated file already uses.
var reservedPackages = func() map[string]bool {
	m := make(map[string]bool)
	for _, w := range strings.Fields(`break default func interface select case defer go map struct chan else goto
		package switch const fallthrough if range type continue for import return var main ecd ecr rt eid`) {
		m[w] = true
	}
	return m
}()

// check returns every refusal of the semantic checks of format 1, unsorted.
func check(c *Catalog) []Refusal {
	var rs []Refusal
	if c.Format != 1 {
		rs = append(rs, Refusal{"$.format", fmt.Sprintf("format %d is not 1", c.Format)})
	}
	if reason := packageReason(c.Package); reason != "" {
		rs = append(rs, Refusal{"$.package", reason})
	}
	switch {
	case c.System == "" && !c.Detached:
		rs = append(rs, Refusal{"$", `want one of "system" or "detached": true`})
	case c.System != "" && c.Detached:
		rs = append(rs, Refusal{"$", `want only one of "system" or "detached"`})
	}
	if c.System != "" {
		sys, ok := eid.ParseCode(c.System)
		switch {
		case !ok:
			rs = append(rs, Refusal{"$.system", fmt.Sprintf("system %q is not a code", c.System)})
		case sys.Class() == eid.Detached:
			rs = append(rs, Refusal{"$.system", fmt.Sprintf("system code %q is detached: write \"detached\": true", c.System)})
		case sys.Class() == eid.Reserved:
			rs = append(rs, Refusal{"$.system", fmt.Sprintf("system code %q is reserved", c.System)})
		}
	}
	rs = append(rs, checkKinds(c.Kinds)...)
	crs, named, texts := checkComponents(c)
	qrs, qnamed := checkQueries(c)
	rs = append(rs, crs...)
	rs = append(rs, qrs...)
	return append(rs, collisions(c, named, qnamed, &texts)...)
}

// packageReason returns the refusal of a package name, or "" when it is fine.
func packageReason(s string) string {
	switch {
	case !packageRE.MatchString(s):
		return fmt.Sprintf("package name %q does not match %s", s, packageGrammar)
	case len(s) > MaxNameLen:
		return fmt.Sprintf("package name %q is longer than %d bytes", s, MaxNameLen)
	case reservedPackages[s]:
		return fmt.Sprintf("package name %q is reserved", s)
	}
	return ""
}

// nameReason returns the refusal of a component or field name by the grammar and the length, or "" when it is fine.
func nameReason(s string) string {
	switch {
	case !nameRE.MatchString(s):
		return fmt.Sprintf("name %q does not match %s", s, nameGrammar)
	case len(s) > MaxNameLen:
		return fmt.Sprintf("name %q is longer than %d bytes", s, MaxNameLen)
	}
	return ""
}

// docReason returns the refusal of a doc string, or "" when it is fine.
func docReason(s string) string {
	if len(s) > MaxDocLen {
		return fmt.Sprintf("doc is longer than %d bytes", MaxDocLen)
	}
	for i := 0; i < len(s); i++ {
		if s[i] < 0x20 || s[i] == 0x7f {
			return "doc holds a control character"
		}
	}
	return ""
}

// validKind reports whether code is a kind code that a catalog may declare: three lowercase letters, not sys.
func validKind(code string) bool {
	k, ok := eid.ParseCode(code)
	return ok && k.IsKind() && k != eid.Sys
}

// checkKinds checks $.kinds and each kind in file order.
func checkKinds(kinds []Kind) []Refusal {
	var rs []Refusal
	switch {
	case len(kinds) == 0:
		rs = append(rs, Refusal{"$.kinds", "want at least one kind"})
	case len(kinds) > MaxKinds:
		rs = append(rs, Refusal{"$.kinds", fmt.Sprintf("%d kinds, at most %d", len(kinds), MaxKinds)})
	}
	declared := make(map[string]int, len(kinds))
	for i, k := range kinds {
		p := fmt.Sprintf("$.kinds[%d]", i)
		code, ok := eid.ParseCode(k.Code)
		switch {
		case !ok || !code.IsKind():
			rs = append(rs, Refusal{p + ".code", fmt.Sprintf("kind %q is not a kind code: want three lowercase letters", k.Code)})
		case code == eid.Sys:
			rs = append(rs, Refusal{p + ".code", fmt.Sprintf("kind %q names the system itself", k.Code)})
		default:
			if j, dup := declared[k.Code]; dup {
				rs = append(rs, Refusal{p + ".code", fmt.Sprintf("kind %q is declared at $.kinds[%d]", k.Code, j)})
			} else {
				declared[k.Code] = i
			}
		}
		if k.Residency != "touched" && k.Residency != "all" {
			rs = append(rs, Refusal{p + ".residency", fmt.Sprintf("residency %q: want \"touched\" or \"all\"", k.Residency)})
		}
		if k.Budget < 0 {
			rs = append(rs, Refusal{p + ".budget", fmt.Sprintf("budget %d is below 0", k.Budget)})
		}
		if reason := docReason(k.Doc); reason != "" {
			rs = append(rs, Refusal{p + ".doc", reason})
		}
	}
	return rs
}

// checkComponents checks $.components, each component and tag in file order. It returns the refusals, the components
// whose name passed the grammar, the length and the declared-before check, and the N of every text[N] in the catalog;
// the generated identifiers are checked by collisions, once the queries are known.
func checkComponents(c *Catalog) (rs []Refusal, named []int, texts [MaxTextN + 1]bool) {
	switch n := len(c.Components); {
	case n == 0:
		rs = append(rs, Refusal{"$.components", "want at least one component or tag"})
	case n > MaxComponents:
		rs = append(rs, Refusal{"$.components", fmt.Sprintf("%d components and tags, at most %d", n, MaxComponents)})
	}
	cids := make(map[int64]int, len(c.Components))
	names := make(map[string]int, len(c.Components))
	for i := range c.Components {
		comp := &c.Components[i]
		p := fmt.Sprintf("$.components[%d]", i)

		if comp.CID < 1 || comp.CID > maxCID {
			rs = append(rs, Refusal{p + ".cid", fmt.Sprintf("cid %d is out of range 1 to %d", comp.CID, maxCID)})
		} else if j, dup := cids[comp.CID]; dup {
			rs = append(rs, Refusal{p + ".cid", fmt.Sprintf("cid %d is declared at $.components[%d]", comp.CID, j)})
		} else {
			cids[comp.CID] = i
		}

		if reason := nameReason(comp.Name); reason != "" {
			rs = append(rs, Refusal{p + ".name", reason})
		} else if j, dup := names[comp.Name]; dup {
			rs = append(rs, Refusal{p + ".name", fmt.Sprintf("name %q is declared at $.components[%d]", comp.Name, j)})
		} else {
			names[comp.Name] = i
			named = append(named, i)
		}

		isTag, isComponent := comp.Form == "tag", comp.Form == "component"
		if !isTag && !isComponent {
			rs = append(rs, Refusal{p + ".form", fmt.Sprintf("form %q: want \"component\" or \"tag\"", comp.Form)})
		}

		size := 0 // the codec bytes of the types that parsed
		if isTag {
			if comp.Query {
				rs = append(rs, Refusal{p + ".query", "a tag holds no value to iterate: declare a query in \"queries\""})
			}
			if comp.Type != "" {
				rs = append(rs, Refusal{p + ".type", "a tag holds no value"})
			}
			if comp.Fields != nil {
				rs = append(rs, Refusal{p + ".fields", "a tag holds no value"})
			}
		} else {
			if comp.Type != "" {
				if ft, err := ParseType(comp.Type); err != nil {
					rs = append(rs, Refusal{p + ".type", err.Error()})
				} else {
					size += ft.Size()
					if ft.Base == "text" {
						texts[ft.N] = true
					}
				}
			}
			if comp.Fields != nil {
				fieldRefusals, fieldSize := checkFields(p, comp.Fields, &texts)
				rs = append(rs, fieldRefusals...)
				size += fieldSize
			}
		}
		if isComponent {
			switch {
			case comp.Type == "" && comp.Fields == nil:
				rs = append(rs, Refusal{p, `want one of "type" or "fields"`})
			case comp.Type != "" && comp.Fields != nil:
				rs = append(rs, Refusal{p, `want only one of "type" or "fields"`})
			case size > MaxSize:
				rs = append(rs, Refusal{p, fmt.Sprintf("value of %d bytes, at most %d", size, MaxSize)})
			}
		}

		if reason := docReason(comp.Doc); reason != "" {
			rs = append(rs, Refusal{p + ".doc", reason})
		}
	}
	return rs, named, texts
}

// checkQueries checks $.queries and each query in file order. It returns the refusals and the indexes of the queries
// whose name passed the grammar, the length and the declared-before check, in file order.
func checkQueries(c *Catalog) ([]Refusal, []int) {
	var rs []Refusal
	marks := 0
	for _, comp := range c.Components {
		if comp.Query {
			marks++
		}
	}
	if n := len(c.Queries) + marks; n > MaxQueries {
		rs = append(rs, Refusal{"$.queries", fmt.Sprintf("%d queries and query marks, at most %d", n, MaxQueries)})
	}
	kinds := make(map[string]Kind, len(c.Kinds)) // each code of $.kinds to its first Kind
	for _, k := range c.Kinds {
		if _, ok := kinds[k.Code]; !ok {
			kinds[k.Code] = k
		}
	}
	comps := make(map[string]bool, len(c.Components)) // the name of every component and tag
	for _, comp := range c.Components {
		comps[comp.Name] = true
	}
	declared := make(map[string]int, len(c.Queries))
	var qnamed []int // the queries whose name passed the grammar, the length and the declared-before check
	for i := range c.Queries {
		q := &c.Queries[i]
		p := fmt.Sprintf("$.queries[%d]", i)

		if reason := nameReason(q.Name); reason != "" {
			rs = append(rs, Refusal{p + ".name", reason})
		} else if j, dup := declared[q.Name]; dup {
			rs = append(rs, Refusal{p + ".name", fmt.Sprintf("name %q is declared at $.queries[%d]", q.Name, j)})
		} else {
			declared[q.Name] = i
			qnamed = append(qnamed, i)
		}

		seenKind := make(map[string]int, len(q.Kinds))
		for j, code := range q.Kinds {
			jp := fmt.Sprintf("%s.kinds[%d]", p, j)
			kind, kindDeclared := kinds[code]
			switch {
			case !kindDeclared:
				rs = append(rs, Refusal{jp, fmt.Sprintf("kind %q is not declared", code)})
			case kind.Residency == "touched":
				rs = append(rs, Refusal{jp, fmt.Sprintf("kind %q is a Touched kind: an engine refuses a query on it (E8)", code)})
			default:
				if k, dup := seenKind[code]; dup {
					rs = append(rs, Refusal{jp, fmt.Sprintf("kind %q is named at %s.kinds[%d]", code, p, k)})
				} else {
					seenKind[code] = j
				}
			}
		}

		type place struct {
			key string
			j   int
		}
		seenComp := make(map[string]place)
		for _, group := range []struct {
			key   string
			names []string
		}{{"all", q.All}, {"none", q.None}, {"any", q.Any}} {
			for j, name := range group.names {
				jp := fmt.Sprintf("%s.%s[%d]", p, group.key, j)
				_, compDeclared := comps[name]
				switch {
				case !compDeclared:
					rs = append(rs, Refusal{jp, fmt.Sprintf("component %q is not declared", name)})
				default:
					if at, dup := seenComp[name]; dup {
						rs = append(rs, Refusal{jp, fmt.Sprintf("component %q is named at %s.%s[%d]", name, p, at.key, at.j)})
					} else {
						seenComp[name] = place{group.key, j}
					}
				}
			}
		}

		if reason := docReason(q.Doc); reason != "" {
			rs = append(rs, Refusal{p + ".doc", reason})
		}
	}
	return rs, qnamed
}

// checkFields checks $.components[I].fields and each field; p is the path of the component. It returns the refusals
// and the codec bytes of the fields whose type parsed.
func checkFields(p string, fields []Field, texts *[MaxTextN + 1]bool) ([]Refusal, int) {
	var rs []Refusal
	fp := p + ".fields"
	switch {
	case len(fields) == 0:
		rs = append(rs, Refusal{fp, "want at least one field"})
	case len(fields) > MaxFields:
		rs = append(rs, Refusal{fp, fmt.Sprintf("%d fields, at most %d", len(fields), MaxFields)})
	}
	declared := make(map[string]int, len(fields))
	size := 0
	for j, f := range fields {
		jp := fmt.Sprintf("%s[%d]", fp, j)
		if reason := nameReason(f.Name); reason != "" {
			rs = append(rs, Refusal{jp + ".name", reason})
		} else if k, dup := declared[f.Name]; dup {
			rs = append(rs, Refusal{jp + ".name", fmt.Sprintf("name %q is declared at %s[%d]", f.Name, fp, k)})
		} else {
			declared[f.Name] = j
			if f.Name == "append" || f.Name == "decode" {
				rs = append(rs, Refusal{jp + ".name", fmt.Sprintf("field name %q makes the identifier %s, a method of the value", f.Name, GoName(f.Name))})
			}
		}
		if ft, err := ParseType(f.Type); err != nil {
			rs = append(rs, Refusal{jp + ".type", err.Error()})
		} else {
			size += ft.Size()
			if ft.Base == "text" {
				texts[ft.N] = true
			}
		}
	}
	return rs, size
}

// collisions registers the identifiers that the generated file declares at package level, in the order of the design,
// the first holder winning, and refuses the name of each component and query whose identifier is already held. named
// and qnamed list the components and queries with a name that passed the earlier checks; texts marks the N of every
// text[N] in the catalog.
func collisions(c *Catalog, named, qnamed []int, texts *[MaxTextN + 1]bool) []Refusal {
	held := make(map[string]bool)
	hold := func(id string) bool { // reports whether id was free
		if held[id] {
			return false
		}
		held[id] = true
		return true
	}
	hold("Catalog")
	if len(c.Queries) > 0 || slices.ContainsFunc(c.Components, func(x Component) bool { return x.Query }) {
		hold("IterError")
	}
	if c.System != "" {
		hold("System")
	}
	for _, k := range c.Kinds {
		if validKind(k.Code) {
			upper := strings.ToUpper(k.Code[:1]) + k.Code[1:]
			hold("Code" + upper)
			hold("Kind" + upper)
		}
	}
	for n, used := range texts {
		if used {
			hold(fmt.Sprintf("Text%d", n))
			hold(fmt.Sprintf("NewText%d", n))
		}
	}

	order := slices.Clone(named)
	slices.SortStableFunc(order, func(a, b int) int {
		return cmp.Compare(c.Components[a].CID, c.Components[b].CID)
	})
	var rs []Refusal
	refused := make(map[int]bool) // the components whose name is refused: one refusal for a value
	for _, i := range order {
		comp := &c.Components[i]
		g := GoName(comp.Name)
		ids := []string{"CID" + g, g}
		if comp.Form == "component" {
			ids = append(ids, "Size"+g, g+"Value")
		}
		clash := ""
		for _, id := range ids {
			if !hold(id) && clash == "" {
				clash = id
			}
		}
		if clash != "" {
			refused[i] = true
			rs = append(rs, Refusal{fmt.Sprintf("$.components[%d].name", i),
				fmt.Sprintf("name %q makes the identifier %s, which another declaration of the file holds", comp.Name, clash)})
		}
	}
	for _, i := range order {
		if comp := &c.Components[i]; comp.Query && comp.Form == "component" {
			id := "QID" + GoName(comp.Name)
			if !hold(id) && !refused[i] {
				rs = append(rs, Refusal{fmt.Sprintf("$.components[%d].name", i),
					fmt.Sprintf("name %q makes the identifier %s, which another declaration of the file holds", comp.Name, id)})
			}
		}
	}
	for _, i := range qnamed {
		q := &c.Queries[i]
		g := GoName(q.Name)
		clash := ""
		for _, id := range []string{"QID" + g, g, g + "Row"} {
			if !hold(id) && clash == "" {
				clash = id
			}
		}
		if clash != "" {
			rs = append(rs, Refusal{fmt.Sprintf("$.queries[%d].name", i),
				fmt.Sprintf("name %q makes the identifier %s, which another declaration of the file holds", q.Name, clash)})
		}
	}
	return rs
}
