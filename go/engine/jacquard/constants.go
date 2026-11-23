// Derived from Ark (github.com/mlange-42/ark) at commit 723f2ac. Copyright (c) 2025 Martin Lange. MIT License.

package ecs

// Reserved Entities.
// Add this to initial capacities of entity pool and lists,
// to avoid unexpected allocations.
const reservedEntities = 2
