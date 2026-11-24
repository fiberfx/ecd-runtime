// Derived from Ark (github.com/mlange-42/ark) at commit 723f2ac. Copyright (c) 2025 Martin Lange. MIT License.
// Port of ecs/entity.go of the cut in go/third_party/ark/.

package jacquard

// Entity is the process-local handle of one entity: the index of its record and the generation that the
// record had when the entity was created. It is 8 bytes and holds no pointer, so a component or a table
// column can carry it by value.
//
// An Entity is valid while the generation of its record equals gen and the record is alive. Freeing an entity
// advances the generation, so a handle that outlives its entity is refused and never names the entity that
// reuses the index (JI1). Entity{} is the null handle: record 0 is reserved and never alive.
//
// An Entity never crosses the ecd.Engine seam and never marshals: the eid is the durable key at every
// boundary, and an Entity is meaningful only inside the process that created it. The type implements no
// JSON, text or binary encoding.
type Entity struct{ index, gen uint32 }
