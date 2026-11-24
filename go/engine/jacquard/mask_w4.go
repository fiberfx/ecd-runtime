//go:build !jacquard512

// Derived from Ark (github.com/mlange-42/ark) at commit 723f2ac. Copyright (c) 2025 Martin Lange. MIT License.
// Port of ecs/mask_notiny.go.

package jacquard

// maskWords is the number of 64-bit words of a mask: 4 words give 256 component bits.
const maskWords = 4

// wideMask is true when the mask has 512 bits (build tag jacquard512).
const wideMask = false
