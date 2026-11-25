package gen

import (
	"fmt"

	eid "github.com/fiberfx/ecd-id/go"
)

// GenerateWith is Generate with one more check: a non-nil reg is the registry of ecd-id whose live entity kinds the
// catalog's system and kinds must be. The catalog's own checks run first, as Generate runs them; a catalog that passes
// them and not CheckRegistry is refused with a *Refused of its faults, sorted. A nil reg is exactly Generate.
func GenerateWith(src []byte, name string, mode Mode, reg *eid.Registry) ([]byte, error) {
	out, err := Generate(src, name, mode)
	if err != nil || reg == nil {
		return out, err
	}
	c, err := Parse(src, name)
	if err != nil {
		return nil, err
	}
	if rs := CheckRegistry(c, reg); len(rs) > 0 {
		return nil, &Refused{Name: name, Refusals: sortRefusals(rs)}
	}
	return out, nil
}

// CheckRegistry returns the refusals of catalog c against the registry reg: its system must be a live system of the
// registry and each of its kinds a live kind of that system with the role entity. A detached catalog has no system and
// is refused. It reports each fault by its path, and checks no kind of a system that is absent, closed or retired.
func CheckRegistry(c *Catalog, reg *eid.Registry) []Refusal {
	if c.Detached {
		return []Refusal{{"$.detached", "a detached catalog has no system to check against the registry"}}
	}
	sys, ok := eid.ParseCode(c.System)
	if !ok {
		return []Refusal{{"$.system", fmt.Sprintf("system %q is not in the registry", c.System)}}
	}
	srow, ok := reg.System(sys)
	if !ok {
		return []Refusal{{"$.system", fmt.Sprintf("system %q is not in the registry", c.System)}}
	}
	if srow.State != eid.Live {
		return []Refusal{{"$.system", fmt.Sprintf("system %q is %s in the registry", c.System, stateWord(srow.State))}}
	}
	entity := map[eid.Code]bool{}
	for _, k := range reg.KindsByRole(sys, eid.RoleEntity) {
		entity[k.Kind] = true
	}
	var rs []Refusal
	for i, k := range c.Kinds {
		path := fmt.Sprintf("$.kinds[%d].code", i)
		code, ok := eid.ParseCode(k.Code)
		if !ok {
			rs = append(rs, Refusal{path, fmt.Sprintf("kind %q of system %q is not in the registry", k.Code, c.System)})
			continue
		}
		row, ok := reg.Kind(sys, code)
		if !ok {
			rs = append(rs, Refusal{path, fmt.Sprintf("kind %q of system %q is not in the registry", k.Code, c.System)})
			continue
		}
		if !entity[code] {
			rs = append(rs, Refusal{path, fmt.Sprintf("kind %q of system %q has the role %s, not entity", k.Code, c.System, row.Role)})
		}
		if row.State != eid.Live {
			rs = append(rs, Refusal{path, fmt.Sprintf("kind %q of system %q is %s in the registry", k.Code, c.System, stateWord(row.State))})
		}
	}
	return rs
}

// stateWord returns the word of a registry state as the registry files spell it.
func stateWord(s eid.State) string {
	switch s {
	case eid.Live:
		return "live"
	case eid.Closed:
		return "closed"
	}
	return "retired"
}
