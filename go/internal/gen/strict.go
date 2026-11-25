package gen

import (
	"bytes"
	"encoding/json"
	"fmt"
	"slices"
	"strconv"
)

// The strict pass reads a catalog file with the token stream of encoding/json and a fixed schema, so that the
// leniency of json.Unmarshal (a key in any letter case, a repeated key, a null, trailing data) never reaches the
// generator. Every fault it finds is a Refusal with the JSON path of the value at fault.

// valueKind is the JSON kind that a key of the schema takes.
type valueKind uint8

const (
	wantInt    valueKind = iota // an integer that fits int64
	wantString                  // a string, empty or not
	wantName                    // a string that is not empty
	wantTrue                    // the literal true
	wantArray                   // an array of objects of one shape
	wantFlag                    // the literal true; leave the key out for false
	wantNames                   // an array of strings that are not empty
)

// keyDef is one key of an object shape.
type keyDef struct {
	key      string
	want     valueKind
	required bool
	elem     *shape // for wantArray: the shape of the elements
}

// shape is the keys of one object, in the order that missing keys are reported.
type shape []keyDef

var (
	fieldShape = shape{
		{"name", wantName, true, nil},
		{"type", wantName, true, nil},
	}
	kindShape = shape{
		{"code", wantName, true, nil},
		{"residency", wantName, true, nil},
		{"budget", wantInt, true, nil},
		{"doc", wantString, false, nil},
		{"components", wantArray, false, &memberShape},
	}
	memberShape = shape{
		{"name", wantName, true, nil},
		{"optional", wantFlag, false, nil},
	}
	componentShape = shape{
		{"cid", wantInt, true, nil},
		{"name", wantName, true, nil},
		{"form", wantName, true, nil},
		{"type", wantName, false, nil},
		{"fields", wantArray, false, &fieldShape},
		{"query", wantFlag, false, nil},
		{"doc", wantString, false, nil},
	}
	queryShape = shape{
		{"name", wantName, true, nil},
		{"kinds", wantNames, false, nil},
		{"all", wantNames, false, nil},
		{"none", wantNames, false, nil},
		{"any", wantNames, false, nil},
		{"exact", wantFlag, false, nil},
		{"doc", wantString, false, nil},
	}
	topShape = shape{
		{"format", wantInt, true, nil},
		{"package", wantName, true, nil},
		{"system", wantName, false, nil},
		{"detached", wantTrue, false, nil},
		{"kinds", wantArray, true, &kindShape},
		{"components", wantArray, true, &componentShape},
		{"queries", wantArray, false, &queryShape},
	}
)

// parser walks the tokens of one catalog file and collects the refusals.
type parser struct {
	dec *json.Decoder
	out []Refusal
}

// syntaxFault carries the first syntax error out of the walk: it ends the pass.
type syntaxFault struct{ err error }

// strict checks src against the schema of format 1 and returns every refusal it finds, in the order found; the
// caller sorts them. A syntax error stops the pass with that one refusal.
func strict(src []byte) (rs []Refusal) {
	p := &parser{dec: json.NewDecoder(bytes.NewReader(src))}
	p.dec.UseNumber()
	defer func() {
		switch f := recover().(type) {
		case nil:
		case syntaxFault:
			rs = []Refusal{{"$", "syntax: " + f.err.Error()}}
		default:
			panic(f)
		}
	}()

	if t := p.tok(); isDelim(t, '{') {
		p.object("$", topShape)
	} else {
		p.refuse("$", "want an object")
		p.skip(t)
	}
	if rest := src[p.dec.InputOffset():]; len(bytes.Trim(rest, " \t\r\n")) > 0 {
		p.refuse("$", "trailing data after the catalog")
	}
	return p.out
}

func (p *parser) refuse(path, reason string) {
	p.out = append(p.out, Refusal{path, reason})
}

// tok returns the next token; a syntax error ends the pass.
func (p *parser) tok() json.Token {
	t, err := p.dec.Token()
	if err != nil {
		panic(syntaxFault{err})
	}
	return t
}

// skip consumes the rest of the value that began with token t.
func (p *parser) skip(t json.Token) {
	if !isDelim(t, '{') && !isDelim(t, '[') {
		return
	}
	for depth := 1; depth > 0; {
		switch d, _ := p.tok().(json.Delim); d {
		case '{', '[':
			depth++
		case '}', ']':
			depth--
		}
	}
}

// object walks the members of an object whose opening brace has been read, then reports its missing keys.
func (p *parser) object(path string, s shape) {
	seen := make(map[string]bool)
	for p.dec.More() {
		key, _ := p.tok().(string)
		kpath := path + keyPath(key)
		if seen[key] {
			p.refuse(kpath, "repeated key")
			p.skip(p.tok())
			continue
		}
		seen[key] = true
		i := slices.IndexFunc(s, func(d keyDef) bool { return d.key == key })
		if i < 0 {
			p.refuse(kpath, "unknown key")
			p.skip(p.tok())
			continue
		}
		p.value(kpath, s[i])
	}
	p.tok() // the closing brace
	for _, d := range s {
		if d.required && !seen[d.key] {
			p.refuse(path, fmt.Sprintf("missing key %q", d.key))
		}
	}
}

// array walks the elements of an array of objects whose opening bracket has been read.
func (p *parser) array(path string, elem *shape) {
	for i := 0; p.dec.More(); i++ {
		epath := path + "[" + strconv.Itoa(i) + "]"
		if t := p.tok(); isDelim(t, '{') {
			p.object(epath, *elem)
		} else {
			p.refuse(epath, "want an object")
			p.skip(t)
		}
	}
	p.tok() // the closing bracket
}

// value reads the value of the key d at path and refuses it when it is not of the kind the key takes.
func (p *parser) value(path string, d keyDef) {
	t := p.tok()
	switch d.want {
	case wantInt:
		n, ok := t.(json.Number)
		switch {
		case !ok || !isInteger(string(n)):
			p.refuse(path, "want an integer")
			p.skip(t)
		default:
			if _, err := strconv.ParseInt(string(n), 10, 64); err != nil {
				p.refuse(path, "integer out of range")
			}
		}
	case wantString, wantName:
		s, ok := t.(string)
		switch {
		case !ok:
			p.refuse(path, "want a string")
			p.skip(t)
		case s == "" && d.want == wantName:
			p.refuse(path, "want a string that is not empty")
		}
	case wantTrue:
		if b, ok := t.(bool); !ok || !b {
			p.refuse(path, "want true; leave the key out for a registered system")
			p.skip(t)
		}
	case wantFlag:
		if b, ok := t.(bool); !ok || !b {
			p.refuse(path, "want true; leave the key out for false")
			p.skip(t)
		}
	case wantNames:
		if !isDelim(t, '[') {
			p.refuse(path, "want an array of strings")
			p.skip(t)
			break
		}
		for i := 0; p.dec.More(); i++ {
			p.value(path+"["+strconv.Itoa(i)+"]", keyDef{want: wantName})
		}
		p.tok() // the closing bracket
	case wantArray:
		if isDelim(t, '[') {
			p.array(path, d.elem)
		} else {
			p.refuse(path, "want an array")
			p.skip(t)
		}
	}
}

// isDelim reports whether t is the delimiter d.
func isDelim(t json.Token, d json.Delim) bool {
	got, ok := t.(json.Delim)
	return ok && got == d
}

// isInteger reports whether s matches ^-?(0|[1-9][0-9]*)$.
func isInteger(s string) bool {
	if len(s) > 0 && s[0] == '-' {
		s = s[1:]
	}
	if s == "" || (s[0] == '0' && len(s) > 1) {
		return false
	}
	for i := 0; i < len(s); i++ {
		if s[i] < '0' || s[i] > '9' {
			return false
		}
	}
	return true
}

// keyPath returns the path segment of an object key: ".key", or ["key"] quoted when the key does not match
// ^[A-Za-z_][A-Za-z0-9_]*$.
func keyPath(key string) string {
	ident := key != ""
	for i := 0; i < len(key); i++ {
		c := key[i]
		letter := 'a' <= c && c <= 'z' || 'A' <= c && c <= 'Z' || c == '_'
		if !letter && (i == 0 || c < '0' || c > '9') {
			ident = false
			break
		}
	}
	if ident {
		return "." + key
	}
	return "[" + strconv.Quote(key) + "]"
}
