// Derived from Ark (github.com/mlange-42/ark) at commit 723f2ac. Copyright (c) 2025 Martin Lange. MIT License.

//go:build !ark_tiny

package ecs

// maskTotalBits for 256 bit ECS.
const maskTotalBits = mask256TotalBits

// bitMask for 256 bit ECS.
type bitMask = bitMask256

// newMask constructor for 256 bit ECS.
var newMask = newMask256
