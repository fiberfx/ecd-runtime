package jacquard

import (
	"bytes"
	"encoding/binary"
	"errors"
	"math/rand/v2"
	"slices"
	"testing"

	eid "github.com/fiberfx/ecd-id/go"
	"github.com/fiberfx/ecd-runtime/go/ecd"
)

// j5pSteps is the number of transactions that TestJ11 runs for each seed.
const j5pSteps = 2000

// j5pHuge is a budget that no test kind reaches.
const j5pHuge = int64(1) << 40

// j5pCIDs are the declared components that the policy tests read: the three that hold a value and the two tags.
var j5pCIDs = []ecd.CID{j3U64, j3U32, j3B3, j3TagA, j3TagB}

// j5pCatalog returns the catalog of the policy tests: the components of jcComponents, the All kinds job and evt,
// which have no budget, and the Touched kind log with the budget logBudget. The kinds are those of j1Kinds, in that
// order, so the kind indexes are 0, 1 and 2.
func j5pCatalog(logBudget int64) *ecd.Catalog {
	ks := j1Kinds()
	return &ecd.Catalog{
		Components: jcComponents(),
		Kinds: []ecd.KindDef{
			{Kind: ks[0], Residency: ecd.All},
			{Kind: ks[1], Residency: ecd.All},
			{Kind: ks[2], Residency: ecd.Touched, Budget: logBudget},
		},
	}
}

// j5pNew returns the engine that New builds for cat, as the *policy that it is.
func j5pNew(t testing.TB, cat *ecd.Catalog) *policy {
	t.Helper()
	en, err := New(cat)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	p, ok := en.(*policy)
	if !ok {
		t.Fatalf("New returns %T, want *policy", en)
	}
	return p
}

// j5pNum32 returns the 4-byte value of n.
func j5pNum32(n uint32) []byte { return binary.LittleEndian.AppendUint32(nil, n) }

// j5pSpawnOps returns the ops that spawn the entity id with a u64 of n and a u32 of n.
func j5pSpawnOps(id eid.ID, n uint64) []ecd.Op {
	return []ecd.Op{j3Spawn(id), j3Set(id, j3U64, j3Num(n)), j3Set(id, j3U32, j5pNum32(uint32(n)))}
}

// j5pSpawnAll returns one deed that spawns every entity of ids, the entity at position i with the value i.
func j5pSpawnAll(ids []eid.ID) ecd.Deed {
	var ops []ecd.Op
	for i, id := range ids {
		ops = append(ops, j5pSpawnOps(id, uint64(i))...)
	}
	return j3Deed(0, ops...)
}

// j5pClone returns a deep copy of deeds: the deeds, their ops and the value of each op.
func j5pClone(deeds []ecd.Deed) []ecd.Deed {
	out := slices.Clone(deeds)
	for i := range out {
		out[i].Ops = slices.Clone(deeds[i].Ops)
		for j := range out[i].Ops {
			out[i].Ops[j].Value = slices.Clone(out[i].Ops[j].Value)
		}
	}
	return out
}

// j5pApply applies one transaction at the next seq to en and, when store is not nil, to store, and fails the test
// on an error of either.
func j5pApply(t testing.TB, en, store ecd.Engine, deeds ...ecd.Deed) {
	t.Helper()
	seq := en.At() + 1
	forStore := j5pClone(deeds)
	if err := en.Apply(seq, deeds); err != nil {
		t.Fatalf("Apply(%d): %v", seq, err)
	}
	if store == nil {
		return
	}
	if err := store.Apply(seq, forStore); err != nil {
		t.Fatalf("store.Apply(%d): %v", seq, err)
	}
}

// j5pStat returns the Stats entry of the kind k.
func j5pStat(en ecd.Engine, k ecd.Kind) ecd.KindStats {
	for _, ks := range en.Stats().Kinds {
		if ks.Kind == k {
			return ks
		}
	}
	return ecd.KindStats{}
}

// j5pBytes returns the Bytes of the kind k of a scratch engine that build returns for cat, after one transaction
// of deeds. A scratch engine has no policy, so no entity is evicted.
func j5pBytes(t testing.TB, cat *ecd.Catalog, k ecd.Kind, deeds ...ecd.Deed) int64 {
	t.Helper()
	en, err := build(cat)
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	if err := en.apply(1, deeds); err != nil {
		t.Fatalf("apply: %v", err)
	}
	return en.kindStats(en.idx.kinds[k]).Bytes
}

// j5pChunk returns the bytes of one chunk of the table that holds the entities of j5pSpawnOps: the Bytes of the
// kind log with one entity, less the index cost of that entity.
func j5pChunk(t testing.TB) int64 {
	t.Helper()
	log := j1Kinds()[2]
	return j5pBytes(t, j5pCatalog(0), log, j3Deed(0, j5pSpawnOps(j1ID(t, log, 1), 1)...)) - indexEntityBytes
}

// j5pRows returns the rows that store holds for the entities ids: for each entity, in the order of ids, one row for
// each declared component that it holds.
func j5pRows(t testing.TB, store ecd.Engine, ids []eid.ID) []ecd.Row {
	t.Helper()
	var rows []ecd.Row
	for _, id := range ids {
		for _, c := range j5pCIDs {
			v, err := store.Get(id, c, nil)
			if errors.Is(err, ecd.ErrAbsent) {
				continue
			}
			if err != nil {
				t.Fatalf("store.Get(%v, %d): %v", id, c, err)
			}
			rows = append(rows, ecd.Row{Entity: id, CID: c, Value: v})
		}
	}
	return rows
}

// j5pHydrate hydrates the entities ids, which are of one kind, into en from the rows that store holds for them, at
// the seq of en. It returns the error of Hydrate.
func j5pHydrate(t testing.TB, en, store ecd.Engine, ids ...eid.ID) error {
	t.Helper()
	return en.Hydrate(ecd.KindOf(ids[0]), en.At(), &jcRowList{rows: j5pRows(t, store, ids)})
}

// j5pSim is one seed of TestJ11: the engine of New and a store that holds every entity of the run.
type j5pSim struct {
	t      *testing.T
	seed   uint64
	rng    *rand.Rand
	en     ecd.Engine
	store  ecd.Engine
	kinds  []ecd.Kind
	budget int64
	next   uint64
	live   [3][]eid.ID // the entities that the store holds, by kind index
	// the numbers that the guards of TestJ11 read
	evictions int64
	within    int
	hits      int
	misses    int
}

// j5pStart returns a sim over the catalog j5pCatalog(budget), with the All kinds hydrated from the empty store.
func j5pStart(t *testing.T, seed uint64, budget int64) *j5pSim {
	t.Helper()
	s := &j5pSim{t: t, seed: seed, kinds: j1Kinds(), budget: budget}
	s.rng = rand.New(rand.NewPCG(seed, seed^0x6a35c0de))
	s.en = j5pNew(t, j5pCatalog(budget))
	s.store = jcStore(t)
	for _, k := range s.kinds[:2] {
		if err := s.en.Hydrate(k, 0, jcRows{}); err != nil {
			t.Fatalf("seed %d: Hydrate(%v): %v", seed, k, err)
		}
	}
	return s
}

// fresh returns an eid of the kind at index ki that no earlier call returned.
func (s *j5pSim) fresh(ki int) eid.ID {
	s.next++
	return j1ID(s.t, s.kinds[ki], s.next)
}

// value returns a value for a Set of c: random bytes of the component's size, nil for a tag.
func (s *j5pSim) value(c ecd.CID) []byte {
	n := j3Size[c]
	if n == 0 {
		return nil
	}
	v := make([]byte, n)
	for i := range v {
		v[i] = byte(s.rng.IntN(256))
	}
	return v
}

// pick returns a random entity of the kind at index ki that is not in used, and marks it used.
func (s *j5pSim) pick(ki int, used map[eid.ID]bool) (eid.ID, bool) {
	for try := 0; try < 4 && len(s.live[ki]) > 0; try++ {
		id := s.live[ki][s.rng.IntN(len(s.live[ki]))]
		if !used[id] {
			used[id] = true
			return id, true
		}
	}
	return eid.ID{}, false
}

// kindIndex returns the kind index of the eid id.
func (s *j5pSim) kindIndex(id eid.ID) int { return slices.Index(s.kinds, ecd.KindOf(id)) }

// forget removes id from the entities that the store holds.
func (s *j5pSim) forget(id eid.ID) {
	ki := s.kindIndex(id)
	if i := slices.Index(s.live[ki], id); i >= 0 {
		s.live[ki] = slices.Delete(s.live[ki], i, i+1)
	}
}

// step applies one random transaction of one to three changes to the engine and to the store, reads three
// Touched entities, and hydrates each one that was a miss. The invariants are checked after the transaction and
// after each hydration.
func (s *j5pSim) step(n int) {
	t := s.t
	t.Helper()
	used := map[eid.ID]bool{}
	var ops []ecd.Op
	var spawned, changed, gone []eid.ID
	for i, count := 0, 1+s.rng.IntN(3); i < count; i++ {
		switch r := s.rng.IntN(100); {
		case r < 40:
			id := s.fresh(2)
			ops = append(ops, j5pSpawnOps(id, s.next)...)
			spawned = append(spawned, id)
		case r < 60:
			id := s.fresh(s.rng.IntN(2))
			ops = append(ops, j3Spawn(id), j3Set(id, j3U64, s.value(j3U64)))
			if s.rng.IntN(2) == 0 {
				c := j5pCIDs[1+s.rng.IntN(len(j5pCIDs)-1)]
				ops = append(ops, j3Set(id, c, s.value(c)))
			}
			spawned = append(spawned, id)
		case r < 80:
			if id, ok := s.pick(s.rng.IntN(2), used); ok {
				c := j5pCIDs[s.rng.IntN(len(j5pCIDs))]
				ops = append(ops, j3Set(id, c, s.value(c)))
				changed = append(changed, id)
			}
		case r < 90:
			if id, ok := s.pick(2, used); ok {
				ops = append(ops, j3Set(id, j3U64, s.value(j3U64)))
				changed = append(changed, id)
			}
		case r < 95:
			if id, ok := s.pick(s.rng.IntN(2), used); ok {
				ops = append(ops, j3Destroy(id))
				gone = append(gone, id)
			}
		default:
			if id, ok := s.pick(2, used); ok {
				ops = append(ops, j3Destroy(id))
				gone = append(gone, id)
			}
		}
	}
	if len(ops) > 0 {
		j5pApply(t, s.en, s.store, j3Deed(0, ops...))
	}
	for _, id := range spawned {
		s.live[s.kindIndex(id)] = append(s.live[s.kindIndex(id)], id)
	}
	for _, id := range gone {
		s.forget(id)
	}
	s.invariant(n)
	for _, id := range slices.Concat(spawned, changed, gone) {
		s.same(n, id)
	}
	for ki := range 3 {
		if len(s.live[ki]) > 0 {
			s.same(n, s.live[ki][s.rng.IntN(len(s.live[ki]))])
		}
	}
	for range 3 {
		if len(s.live[2]) == 0 {
			break
		}
		id := s.live[2][s.rng.IntN(len(s.live[2]))]
		got, err := s.en.Get(id, j3U64, nil)
		switch {
		case err == nil:
			s.hits++
			if want, _ := s.store.Get(id, j3U64, nil); !bytes.Equal(got, want) {
				t.Fatalf("seed %d step %d: Get(%v) = %x, the store holds %x", s.seed, n, id, got, want)
			}
		case errors.Is(err, ecd.ErrMiss):
			s.misses++
			if err := j5pHydrate(t, s.en, s.store, id); err != nil {
				t.Fatalf("seed %d step %d: Hydrate(%v): %v", s.seed, n, id, err)
			}
			s.invariant(n)
		default:
			t.Fatalf("seed %d step %d: Get(%v): %v", s.seed, n, id, err)
		}
	}
}

// invariant checks the Stats of the engine: the Touched kind holds at most its budget or no entity, and its
// evictions have not fallen; each All kind is Warm and holds as many entities as the store.
func (s *j5pSim) invariant(step int) {
	t := s.t
	t.Helper()
	log := j5pStat(s.en, s.kinds[2])
	switch {
	case log.Entities == 0:
	case log.Bytes <= s.budget:
		s.within++
	default:
		t.Fatalf("seed %d step %d: the Touched kind holds %d bytes in %d entities, budget %d",
			s.seed, step, log.Bytes, log.Entities, s.budget)
	}
	if log.Evictions < s.evictions {
		t.Fatalf("seed %d step %d: evictions fell from %d to %d", s.seed, step, s.evictions, log.Evictions)
	}
	s.evictions = log.Evictions
	for _, k := range s.kinds[:2] {
		e, m := j5pStat(s.en, k), j5pStat(s.store, k)
		if e.State != ecd.Warm || e.Entities != m.Entities {
			t.Fatalf("seed %d step %d: kind %v is %v with %d entities, the store holds %d",
				s.seed, step, k, e.State, e.Entities, m.Entities)
		}
	}
}

// same compares every Get of the entity id on the engine with the store. A Touched entity that is not resident
// answers ErrMiss and is not compared.
func (s *j5pSim) same(step int, id eid.ID) {
	t := s.t
	t.Helper()
	for _, c := range j5pCIDs {
		ge, errE := s.en.Get(id, c, nil)
		gs, errS := s.store.Get(id, c, nil)
		if errors.Is(errE, ecd.ErrMiss) && ecd.KindOf(id) == s.kinds[2] {
			continue
		}
		if (errE == nil) != (errS == nil) || errors.Is(errE, ecd.ErrAbsent) != errors.Is(errS, ecd.ErrAbsent) ||
			!bytes.Equal(ge, gs) {
			t.Fatalf("seed %d step %d: Get(%v, %d): engine %x (%v), store %x (%v)", s.seed, step, id, c, ge, errE, gs, errS)
		}
	}
}

// TestJ11 (J11): four seeds of 2,000 random transactions run through the engine of New and a store. The catalog has
// two All kinds with no budget and one Touched kind with a budget of three chunks, which the kind exceeds many times
// over; the transactions spawn, set and destroy entities of every kind, and three reads of a Touched entity follow
// each transaction, a miss being hydrated from the store. After every Apply and every Hydrate the Touched kind holds
// at most its budget or no entity, and its evictions have not fallen. Each All kind holds as many entities as the
// store, and every Get of an entity that the transaction or a random draw names equals the store's; the whole set is
// compared every 250 transactions and at the end. The test fails for a seed in which no step ended within the budget
// with a resident entity, or in which nothing was evicted, hit or missed.
func TestJ11(t *testing.T) {
	budget := 3 * j5pChunk(t)
	for seed := uint64(1); seed <= 4; seed++ {
		s := j5pStart(t, seed, budget)
		for n := 1; n <= j5pSteps; n++ {
			s.step(n)
			if n%250 == 0 || n == j5pSteps {
				for ki := range 3 {
					for _, id := range s.live[ki] {
						s.same(n, id)
					}
				}
			}
		}
		t.Logf("seed %d: %d steps within budget with residents, %d evictions, %d hits, %d misses",
			seed, s.within, s.evictions, s.hits, s.misses)
		if s.within == 0 {
			t.Errorf("seed %d: no step ended within the budget with a resident entity", seed)
		}
		if s.evictions == 0 {
			t.Errorf("seed %d: no entity was evicted", seed)
		}
		if s.hits == 0 || s.misses == 0 {
			t.Errorf("seed %d: %d hits and %d misses, want both above zero", seed, s.hits, s.misses)
		}
	}
}

// TestL15 (L15-jacquard): the Touched entities are spawned in one transaction, so the kind holds its full bytes before
// the first trim, and the budget is a tenth of those bytes. After the transaction the kind is within the budget and
// has lost the entities that do not fit. 100,000 random reads follow: every Get equals the store or answers ErrMiss,
// and on ErrMiss the entity is hydrated from the store and read again, which equals the store or answers ErrMiss
// when the hydration's own trim evicted it. The test fails when no read hit or no read missed.
func TestL15(t *testing.T) {
	const entities, reads = 8000, 100_000
	log := j1Kinds()[2]
	ids := make([]eid.ID, entities)
	for i := range ids {
		ids[i] = j1ID(t, log, uint64(i+1))
	}
	full := j5pBytes(t, j5pCatalog(0), log, j5pSpawnAll(ids))
	budget := full / 10
	p := j5pNew(t, j5pCatalog(budget))
	store := jcStore(t)
	j5pApply(t, p, store, j5pSpawnAll(ids))
	st := j5pStat(p, log)
	if st.Bytes > budget || st.Entities == 0 || st.Entities == entities || st.Evictions != entities-st.Entities {
		t.Fatalf("after the spawns: %d bytes, %d entities, %d evictions, budget %d of %d bytes",
			st.Bytes, st.Entities, st.Evictions, budget, full)
	}
	rng := rand.New(rand.NewPCG(15, 0x15c10c4))
	var hits, misses int
	for n := 0; n < reads; n++ {
		id := ids[rng.IntN(entities)]
		want, err := store.Get(id, j3U64, nil)
		if err != nil {
			t.Fatalf("store.Get(%v): %v", id, err)
		}
		got, err := p.Get(id, j3U64, nil)
		switch {
		case err == nil:
			hits++
		case errors.Is(err, ecd.ErrMiss):
			misses++
			if err := j5pHydrate(t, p, store, id); err != nil {
				t.Fatalf("read %d: Hydrate(%v): %v", n, id, err)
			}
			if got, err = p.Get(id, j3U64, nil); err != nil && !errors.Is(err, ecd.ErrMiss) {
				t.Fatalf("read %d: Get(%v) after the hydration: %v", n, id, err)
			}
		default:
			t.Fatalf("read %d: Get(%v): %v", n, id, err)
		}
		if err == nil && !bytes.Equal(got, want) {
			t.Fatalf("read %d: Get(%v) = %x, the store holds %x", n, id, got, want)
		}
	}
	if st := j5pStat(p, log); st.Bytes > budget {
		t.Errorf("after the reads: %d bytes, budget %d", st.Bytes, budget)
	}
	t.Logf("%d hits, %d misses, budget %d of %d bytes", hits, misses, budget, full)
	if hits == 0 || misses == 0 {
		t.Errorf("%d hits and %d misses, want both above zero", hits, misses)
	}
}

// TestL15Budget (L15v): an All kind whose budget is below the bytes of its rows refuses the hydration with an error
// that matches ecd.ErrBudget, and stays Cold with no entity; At and the seq rule are as before the call, so the engine,
// which has loaded nothing, still takes the seq of its next hydration. The same rows hydrate into a kind whose budget
// equals their bytes, and the kind is Warm.
func TestL15Budget(t *testing.T) {
	ks := j1Kinds()
	job := ks[0]
	ids := make([]eid.ID, 100)
	for i := range ids {
		ids[i] = j1ID(t, job, uint64(i+1))
	}
	store := jcStore(t)
	if err := store.Apply(1, []ecd.Deed{j5pSpawnAll(ids)}); err != nil {
		t.Fatalf("store.Apply: %v", err)
	}
	rows := j5pRows(t, store, ids)
	scratch, err := build(j5pCatalog(0))
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	if err := scratch.hydrate(job, 1, &jcRowList{rows: rows}); err != nil {
		t.Fatalf("scratch hydrate: %v", err)
	}
	bytesOf := scratch.kindStats(0).Bytes

	over := j5pCatalog(0)
	over.Kinds[0].Budget = bytesOf - 1
	p := j5pNew(t, over)
	err = p.Hydrate(job, 1, &jcRowList{rows: rows})
	if !errors.Is(err, ecd.ErrBudget) {
		t.Fatalf("Hydrate over the budget answers %v, want ErrBudget", err)
	}
	if st := p.State(job); st != ecd.Cold {
		t.Errorf("State after the refusal is %v, want Cold", st)
	}
	if st := j5pStat(p, job); st.Entities != 0 || st.State != ecd.Cold {
		t.Errorf("Stats after the refusal: %d entities, state %v, want 0 and Cold", st.Entities, st.State)
	}
	if at := p.At(); at != 0 {
		t.Errorf("At after the refusal is %d, want 0", at)
	}
	if err := p.Hydrate(ks[1], 7, jcRows{}); err != nil || p.At() != 7 {
		t.Errorf("a hydration at 7 after the refusal answers %v and At is %d, want nil and 7", err, p.At())
	}

	fits := j5pCatalog(0)
	fits.Kinds[0].Budget = bytesOf
	q := j5pNew(t, fits)
	if err := q.Hydrate(job, 1, &jcRowList{rows: rows}); err != nil {
		t.Fatalf("Hydrate within the budget: %v", err)
	}
	if st := j5pStat(q, job); st.State != ecd.Warm || st.Entities != int64(len(ids)) || st.Bytes != bytesOf {
		t.Errorf("Stats within the budget: state %v, %d entities, %d bytes, want Warm, %d and %d",
			st.State, st.Entities, st.Bytes, len(ids), bytesOf)
	}
	if q.At() != 1 {
		t.Errorf("At within the budget is %d, want 1", q.At())
	}
}

// TestL35Record (L35-jacquard): 1,000 Get hits on one Touched entity leave the entity's record equal to what it was,
// and set the entity's bit in the clock of its kind, while the bit of an entity that was not read stays clear.
func TestL35Record(t *testing.T) {
	log := j1Kinds()[2]
	read, other := j1ID(t, log, 1), j1ID(t, log, 2)
	p := j5pNew(t, j5pCatalog(j5pHuge))
	j5pApply(t, p, nil, j3Deed(0, append(j5pSpawnOps(read, 7), j5pSpawnOps(other, 8)...)...))
	e, ok := p.idx.entity(read)
	o, ok2 := p.idx.entity(other)
	if !ok || !ok2 {
		t.Fatalf("the spawned entities are not resident")
	}
	ki := p.idx.kinds[log]
	before := p.idx.pool.records[e.index]
	if p.clocks[ki].marked(e.index) {
		t.Fatalf("the entity is marked before any read")
	}
	for i := 0; i < 1000; i++ {
		got, err := p.Get(read, j3U64, nil)
		if err != nil || !bytes.Equal(got, j3Num(7)) {
			t.Fatalf("Get %d = %x, %v, want %x", i, got, err, j3Num(7))
		}
	}
	if after := p.idx.pool.records[e.index]; after != before {
		t.Errorf("the record changed from %+v to %+v", before, after)
	}
	if !p.clocks[ki].marked(e.index) {
		t.Errorf("the entity is not marked after 1,000 reads")
	}
	if p.clocks[ki].marked(o.index) {
		t.Errorf("the entity that was not read is marked")
	}
}

// TestJ5PClock (J5P-01): the budget holds three entities of the Touched kind. After a Get of the first entity, a
// fourth spawn is over the budget and the hand takes the second entity, which has not been read, and leaves the
// first. Then a Get and an Evict of the third entity free its index, the next spawn takes that index, and the spawn
// after it is over the budget again: the victim is the entity at the reused index, which does not inherit the mark
// of the entity that held the index before.
func TestJ5PClock(t *testing.T) {
	log := j1Kinds()[2]
	id := func(n uint64) eid.ID { return j1ID(t, log, n) }
	spawn := func(ns ...uint64) ecd.Deed {
		var ops []ecd.Op
		for _, n := range ns {
			ops = append(ops, j5pSpawnOps(id(n), n)...)
		}
		return j3Deed(0, ops...)
	}
	three := j5pBytes(t, j5pCatalog(0), log, spawn(1, 2, 3))
	p := j5pNew(t, j5pCatalog(three))
	resident := func(n uint64) bool { _, ok := p.idx.entity(id(n)); return ok }
	expect := func(step string, in ...uint64) {
		t.Helper()
		for n := uint64(1); n <= 6; n++ {
			if got, want := resident(n), slices.Contains(in, n); got != want {
				t.Errorf("%s: entity %d resident is %v, want %v", step, n, got, want)
			}
		}
	}

	j5pApply(t, p, nil, spawn(1, 2, 3))
	expect("three entities at the budget", 1, 2, 3)
	if _, err := p.Get(id(1), j3U64, nil); err != nil {
		t.Fatalf("Get(1): %v", err)
	}
	j5pApply(t, p, nil, spawn(4))
	expect("the fourth spawn", 1, 3, 4)
	if ev := j5pStat(p, log).Evictions; ev != 1 {
		t.Errorf("evictions after the fourth spawn: %d, want 1", ev)
	}

	if _, err := p.Get(id(3), j3U64, nil); err != nil {
		t.Fatalf("Get(3): %v", err)
	}
	if err := p.Evict(log, []eid.ID{id(3)}); err != nil {
		t.Fatalf("Evict(3): %v", err)
	}
	j5pApply(t, p, nil, spawn(5))
	if e, ok := p.idx.entity(id(5)); !ok || e.index != 3 {
		t.Fatalf("the fifth entity is at %+v, want the index 3 that the eviction freed", e)
	}
	expect("the fifth spawn", 1, 4, 5)
	j5pApply(t, p, nil, spawn(6))
	expect("the sixth spawn", 1, 4, 6)
	if ev := j5pStat(p, log).Evictions; ev != 3 {
		t.Errorf("evictions after the sixth spawn: %d, want 3", ev)
	}
}

// TestJ5PZero (J5P-02): a Touched kind with Budget 0 keeps no entity. After a transaction that spawns three
// entities, and after a hydration of two of them from the store, every Get answers ErrMiss, the kind holds no entity,
// and Evictions counts each entity that was dropped. A nil catalog, and a catalog that declares a CID twice, answer an
// error that matches the catalog refusal, with a nil engine.
func TestJ5PZero(t *testing.T) {
	log := j1Kinds()[2]
	ids := []eid.ID{j1ID(t, log, 1), j1ID(t, log, 2), j1ID(t, log, 3)}
	p := j5pNew(t, j5pCatalog(0))
	store := jcStore(t)
	miss := func(step string) {
		t.Helper()
		for _, id := range ids {
			if _, err := p.Get(id, j3U64, nil); !errors.Is(err, ecd.ErrMiss) {
				t.Errorf("%s: Get(%v) answers %v, want ErrMiss", step, id, err)
			}
		}
	}

	j5pApply(t, p, store, j5pSpawnAll(ids))
	miss("after the spawns")
	if st := j5pStat(p, log); st.Entities != 0 || st.Evictions != 3 {
		t.Errorf("after the spawns: %d entities and %d evictions, want 0 and 3", st.Entities, st.Evictions)
	}
	if err := j5pHydrate(t, p, store, ids[:2]...); err != nil {
		t.Fatalf("Hydrate: %v", err)
	}
	miss("after the hydration")
	if st := j5pStat(p, log); st.Entities != 0 || st.Evictions != 5 {
		t.Errorf("after the hydration: %d entities and %d evictions, want 0 and 5", st.Entities, st.Evictions)
	}

	dup := j5pCatalog(0)
	dup.Components = append(dup.Components, ecd.ComponentDef{CID: j3U64, Name: "again", Form: ecd.Component, Size: 8})
	for name, cat := range map[string]*ecd.Catalog{"nil catalog": nil, "duplicate CID": dup} {
		en, err := New(cat)
		if !errors.Is(err, errCatalog) || en != nil {
			t.Errorf("New with a %s answers %v and %v, want an error that matches errCatalog and a nil engine", name, en, err)
		}
	}
}
