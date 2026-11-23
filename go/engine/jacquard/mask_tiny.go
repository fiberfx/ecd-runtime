// Derived from Ark (github.com/mlange-42/ark) at commit 723f2ac. Copyright (c) 2025 Martin Lange. MIT License.

//go:build ark_tiny

package ecs

// maskTotalBits for 64 bit tiny ECS.
const maskTotalBits = mask64TotalBits

// bitMask for 64 bit tiny ECS.
type bitMask = bitMask64

// newMask constructor for 64 bit tiny ECS.
var newMask = newMask64
