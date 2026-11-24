//go:build jacquard512

// Derived from Ark (github.com/mlange-42/ark) at commit 723f2ac. Copyright (c) 2025 Martin Lange. MIT License.
// Port of ecs/mask_tiny.go of the cut in go/third_party/ark/.

package jacquard

// maskWords is the number of 64-bit words of a mask: 8 words give 512 component bits.
const maskWords = 8

// wideMask is true when the mask has 512 bits (build tag jacquard512).
const wideMask = true
