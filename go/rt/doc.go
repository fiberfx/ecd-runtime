// Package rt is the Ecd Runtime: one command buffer per registered system, and the flush that turns the buffers
// into deeds, commits them in the owner store and applies the committed ones to the engine.
//
// A system records its writes in its Buffer: Spawn, Set, Unset and Destroy. A record names its entity by a Ref:
// the eid of an entity that exists (Of), or the placeholder that a Spawn of the same buffer returned since that
// buffer's last flush. No record method returns an error, mints an eid or touches an engine.
//
// Flush runs on the owner goroutine with no system running, and takes these steps in this order:
//
//  1. Collect the buffers that hold a record, in flush order: stage ascending, then registration order within a
//     stage. With none, Flush mints, commits and applies nothing.
//  2. Resolve, before any mint or change: every record names an entity by a non-zero eid, or by a placeholder of
//     its own buffer issued since that buffer's last flush. Any other Ref refuses the flush with ErrRef.
//  3. Mint from the host's Minter, in flush order: for each buffer the deed's eid of Config.DeedKind, then one eid
//     per spawn in record order. A spawn eid of another kind than the spawn's refuses the flush with ecd.ErrKind.
//  4. Build one ecd.Deed per buffer: Ord is the buffer's index among the collected buffers, and the ops follow
//     record order. A spawn is one OpSpawn, and the Set records that name its placeholder follow as OpSet ops.
//  5. Check the deeds against the catalog. A refusal matches ecd.ErrKind or ecd.ErrOp, and nothing is committed.
//  6. Commit the deeds in one store transaction through the Committer, which answers one Outcome per deed.
//  7. Apply the deeds answered Committed, with their Ord unchanged, in one Apply at the transaction's seq. With
//     none committed there is no Apply.
//
// Every buffer collected in step 1 is emptied before Flush returns, on every path, and the placeholders issued
// before the flush are refused afterwards. A refusal in step 2, 3, 5 or 6 drops the records of every collected
// buffer; its error names each stage that held a record with the number of records dropped, and matches what its
// cause matches. An error of Apply is returned as the engine gives it, beside the Result, since its deeds are
// committed. A deed answered Conflict, Refused or Unknown returns no error; Result.Outcomes names it by Ord.
//
// The order of a flush depends only on the stages, the registration order and the record order, so two flushes
// of the same records with the same Minter answers build the same deeds, byte for byte, with the same Ord.
//
// A placeholder is valid only in the buffer that issued it, and one buffer with a record is one deed. A spawn and
// every record that names its placeholder are therefore in one deed and share one outcome, and no deed names an
// entity spawned by another deed of the same flush.
//
// Limits. The package runs no scheduler and no system: the host calls Flush at its sync points. The store sits
// behind Committer and is not an engine. The Runtime builds no Minter and reads no environment variable; the host
// passes the one Minter of its lease in Config. A record names an entity only by its eid or by a placeholder of
// its own buffer.
package rt
