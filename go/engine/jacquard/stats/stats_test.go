// Derived from Ark (github.com/mlange-42/ark) at commit 723f2ac. Copyright (c) 2025 Martin Lange. MIT License.

package stats

import (
	"fmt"
	"reflect"
	"testing"
)

func TestStats(t *testing.T) {
	stats := World{
		Entities:           Entities{},
		ComponentTypes:     []reflect.Type{reflect.TypeOf(1)},
		ComponentTypeNames: []string{"int"},
		Locked:             false,
		Archetypes: []Archetype{
			{
				Size:               1,
				Capacity:           128,
				ComponentIDs:       []uint8{0},
				ComponentTypes:     []reflect.Type{reflect.TypeOf(1)},
				ComponentTypeNames: []string{"int"},
			},
			{
				Size:               1,
				Capacity:           128,
				ComponentIDs:       []uint8{0},
				ComponentTypes:     []reflect.Type{reflect.TypeOf(1)},
				ComponentTypeNames: []string{"int"},
			},
		},
	}
	fmt.Println(stats.String())

	table := Table{
		Size:     16,
		Capacity: 64,
	}
	fmt.Println(table.String())
}
