// Derived from Ark (github.com/mlange-42/ark) at commit 723f2ac. Copyright (c) 2025 Martin Lange. MIT License.

package ecs

import "fmt"

func (s *storage) checkRelationComponent(id ID) {
	if !s.registry.IsRelation[id.id] {
		panic(fmt.Sprintf("component with ID %d is not a relation component", id.id))
	}
}

func (s *storage) checkRelationTarget(target Entity) {
	if !target.IsZero() && !s.entityPool.Alive(target) {
		panic("can't use a dead entity as relation target, except for the zero entity")
	}
}
