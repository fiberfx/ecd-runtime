// Package jacquard is Loom's archetype engine.
//
// Engine is the central type of the package. An entity is known by two names: the eid is its durable key and
// crosses every boundary, and Entity is the process-local handle that the hot paths use and that never leaves
// the process.
//
// Entities of one kind and one component set share an archetype. An archetype stores its rows in fixed-size
// chunks, and column 0 of every chunk holds the eid of each row (JI1). A component set is a mask, and the
// width of a mask is 256 bits, or 512 bits under the build tag jacquard512.
//
// Files that port a file of Ark name their source file in their header; the cut in go/third_party/ark/ecs is
// the source.
package jacquard
