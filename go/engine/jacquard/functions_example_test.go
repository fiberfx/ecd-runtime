// Derived from Ark (github.com/mlange-42/ark) at commit 723f2ac. Copyright (c) 2025 Martin Lange. MIT License.

package ecs_test

import "github.com/fiberfx/ecd-loom/go/engine/jacquard"

func ExampleAddResource() {
	world := ecs.NewWorld()

	gridResource := NewGrid(100, 100)
	ecs.AddResource(world, &gridResource)
	// Output:
}

func ExampleGetResource() {
	world := ecs.NewWorld()

	gridResource := NewGrid(100, 100)
	ecs.AddResource(world, &gridResource)

	grid := ecs.GetResource[Grid](world)
	entity := grid.Get(13, 42)
	_ = entity
	// Output:
}
