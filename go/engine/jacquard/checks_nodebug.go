// Derived from Ark (github.com/mlange-42/ark) at commit 723f2ac. Copyright (c) 2025 Martin Lange. MIT License.

//go:build !ark_debug

package ecs

const isDebug = false

func (s *storage) checkHasComponent(_ Entity, _ ID) {}
