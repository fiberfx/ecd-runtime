// Derived from Ark (github.com/mlange-42/ark) at commit 723f2ac. Copyright (c) 2025 Martin Lange. MIT License.
// Port of ecs/column.go.

package jacquard

import (
	"reflect"
	"unsafe"
)

// colDef describes the element type of one typed column.
type colDef struct {
	typ      reflect.Type // size = typ.Size()
	pointers bool         // typ holds a pointer word: pointer, unsafe.Pointer, string, slice, map, chan, func or interface, found through struct fields and array elements
}

// defOf returns the definition of a column of element type T.
func defOf[T any]() colDef {
	t := reflect.TypeFor[T]()
	return colDef{typ: t, pointers: hasPointers(t)}
}

// hasPointers reports whether a value of type t holds a pointer word, so that the collector must scan it.
// Pointers are found through struct fields and array elements.
func hasPointers(t reflect.Type) bool {
	switch t.Kind() {
	case reflect.Pointer, reflect.UnsafePointer, reflect.Func, reflect.Interface, reflect.Map, reflect.Chan, reflect.Slice, reflect.String:
		return true
	case reflect.Array:
		return t.Len() > 0 && hasPointers(t.Elem())
	case reflect.Struct:
		for i := range t.NumField() {
			if hasPointers(t.Field(i).Type) {
				return true
			}
		}
	}
	return false
}

// copyElem copies one element of d from src to dst. A pointer-holding element is copied as a typed value
// through reflect, which respects the write barrier (JI5, J09); other elements are copied as bytes.
func copyElem(d colDef, dst, src unsafe.Pointer) {
	if d.pointers {
		reflect.NewAt(d.typ, dst).Elem().Set(reflect.NewAt(d.typ, src).Elem())
		return
	}
	n := int(d.typ.Size())
	copy(unsafe.Slice((*byte)(dst), n), unsafe.Slice((*byte)(src), n))
}

// clearElem sets one element of d to its zero value. A pointer-holding element is cleared as a typed value
// through reflect, so that the collector no longer finds what it referenced.
func clearElem(d colDef, p unsafe.Pointer) {
	if d.pointers {
		reflect.NewAt(d.typ, p).Elem().SetZero()
		return
	}
	clear(unsafe.Slice((*byte)(p), int(d.typ.Size())))
}
