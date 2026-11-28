// Package gj holds the comparison benchmarks of the Jacquard engine (go/engine/jacquard) against the adapter over
// Ark (go/enginecmp/ark): the gates GJ1 to GJ4, and the values of G4 and G6, which are recorded without a target.
// The package has no code of its own; the benchmarks and TestGJSmoke are in its test file.
//
// Both engines run through ecd.Engine on the same data, built once before the sub-benchmarks: one catalog with one
// All kind, eight components of 8 bytes without pointers and one declared query, All of the first component; and
// 100,000 entities, each holding every component, so that the kind has one archetype. Every benchmark has the two
// sub-benchmarks jacquard and ark, in that order, reports allocations, and reports its value with b.ReportMetric
// under its unit:
//
//	BenchmarkGJ1Scan        ns/entity      the declared query on the Warm kind, which every entity satisfies, into
//	                                       a caller-owned Answer whose slice has room for every entity
//	BenchmarkGJ2Bytes       B/entity       KindStats.Bytes over KindStats.Entities; the timed region reads Stats
//	BenchmarkGJ3Apply       ns/deed        Apply of one deed of 8 sets, one per component, on a resident entity,
//	                                       from a ring of 1,024 deeds on distinct entities, each applied once
//	                                       before the timed region
//	BenchmarkGJ4Hydrate     entities/s     Hydrate of the kind from a slice RowSource into a new engine, which is
//	                                       built outside the timed region
//	BenchmarkG4StatsBytes   stats/heap     KindStats.Bytes over the growth of the heap objects that runtime/metrics
//	                                       reports across one hydration, both heap readings taken after a
//	                                       collection; the timed region reads Stats
//	BenchmarkG6HydratePeak  peak/resident  the growth of the heap objects at the end of a hydration that runs with
//	                                       the collector off, over the growth after a collection; the largest
//	                                       ratio of the iterations, each a hydration into a new engine
//
// Before its timed region a benchmark checks what both engines must answer alike, and fails when an engine differs:
// a hydration leaves the kind Warm with every entity, the query answers the eids of every entity in eid order, and
// after the ring of deeds a Get answers the value of the last set. KindStats.Bytes is not compared between the
// engines: Jacquard counts allocated capacity and the adapter the bytes of the held values, so GJ2 and G4 report
// it and assert nothing on it. KindStats.Nodes is not read, because the adapter reports Ark's archetypes there.
//
// The heap is read with runtime/metrics, never inside a timed region. With the collector off, the peak of G6
// counts every byte that the hydration allocates: it bounds from above the peak of a hydration with the collector
// on.
//
// The file gates_test.go holds, on Jacquard alone, the gates G3 (TestG3Allocs and BenchmarkG3Bulk) and G5
// (TestG5Scan and BenchmarkG5Scan) and the peak of G6 with the collector on (BenchmarkG6PeakGC); its header says
// what each measures. G1 is measured in go/internal/llm, through the typed path, and G2 in go/sqlite, beside the
// SQL twin of L12.
//
// Not measured here: churn, the destruction and spawn of entities between iterations; GJ3 with more than one active
// archetype, such as 256 archetypes over several kinds; Touched kinds and byte budgets, which the adapter does not
// serve. The values are the inputs of rule E9 and do not decide it alone.
package gj
