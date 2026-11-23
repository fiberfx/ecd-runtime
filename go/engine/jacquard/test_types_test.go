// Derived from Ark (github.com/mlange-42/ark) at commit 723f2ac. Copyright (c) 2025 Martin Lange. MIT License.

package ecs_test

type Position struct {
	X, Y float64
}

type Velocity struct {
	X, Y float64
}

type Altitude struct {
	Z float64
}

type Grid struct {
	Width  int
	Height int
}
