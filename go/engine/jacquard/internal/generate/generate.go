// Derived from Ark (github.com/mlange-42/ark) at commit 723f2ac. Copyright (c) 2025 Martin Lange. MIT License.

package main

// Code generation for generic queries and maps.

//go:generate go run .
//go:generate go fmt ../../filter_gen.go
//go:generate go fmt ../../query_gen.go
//go:generate go fmt ../../query_debug_gen.go
//go:generate go fmt ../../query_nodebug_gen.go
//go:generate go fmt ../../query_gen_test.go
//go:generate go fmt ../../maps_gen.go
//go:generate go fmt ../../maps_gen_test.go
//go:generate go fmt ../../exchange_gen.go
//go:generate go fmt ../../exchange_gen_test.go
//go:generate go fmt ../../observers_gen.go
//go:generate go fmt ../../observers_gen_test.go
