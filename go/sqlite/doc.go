// Package sqlite is an owner store of Ecd Runtime in one SQLite file. A host passes a *Store to the Runtime as
// its rt.Committer, so that committed deeds outlive the process. After a restart, Hydrate brings a fresh engine to
// the store's seq with every All kind as it was committed, and Log yields the committed deeds for replay.
//
// # Tables
//
//   - entity: one row per spawned entity, with the seq of its last write and, after a destroy, that seq in gone.
//     The row of a destroyed entity is a tombstone that is never deleted, so its eid is never spawned again.
//   - state: one row per component value of a live entity, of every kind, keyed by eid and CID. A value of length
//     0, a tag or a nil value, is stored as NULL.
//   - deed: one row per committed deed: its eid, the seq of its transaction, its Ord and the commit time in Unix
//     milliseconds, which is kept for audit only.
//   - deed_body: the ops of each committed deed, for replay: for each op in list order, its code (1 byte), its
//     entity (16 bytes), its CID (2 bytes) and its value's length (4 bytes), both big-endian, then the value.
//
// The store's seq is the highest deed.seq, or 0 for a store with no deed. Only a transaction that commits at least
// one deed advances it, by one.
//
// # Commit
//
// Commit writes the deeds of one flush in one SQLite transaction, begun BEGIN IMMEDIATE:
//
//  1. Read the store's seq; the transaction's seq is the next one.
//  2. For each deed in slice order, in a savepoint of its own: insert its deed row and its body, then write its
//     ops in list order. A spawn inserts the entity. A set or an unset requires a live entity and writes or
//     deletes the value. A destroy requires a live entity, marks it gone and deletes its values.
//  3. Commit when at least one deed committed, and return the transaction's seq. Otherwise roll back and
//     return 0.
//
// Each deed is answered one rt.Outcome. Conflict and Refused are read from a statement that changed no row, never
// from the text of an error:
//
//   - Committed: every op of the deed is written.
//   - Conflict: the deed's eid is committed already, as on a retry.
//   - Refused: an op that the store's state contradicts: a spawn of an eid that exists, alive or destroyed; a set,
//     an unset or a destroy of an entity that is not alive; a code that is not an op.
//   - Unknown: a SQL error. The transaction is rolled back, every deed is answered Unknown, and Commit returns 0
//     and the error.
//
// A deed answered Conflict or Refused is rolled back to its savepoint, and nothing of it stays.
//
// # Hydration and replay
//
// Hydrate reads the store's seq and then, for each All kind of the catalog in catalog order, the values in the
// kind's eid range in eid order, all in one read transaction; the engine loads each kind at that seq. The values
// of Touched kinds are written by Commit and not read back. Log yields the deeds above a seq, in seq order, then
// Ord order, from one read transaction.
//
// # Settings
//
// Create refuses a path where anything exists. It sets page_size 4096 and auto_vacuum INCREMENTAL before the first
// table, then journal_mode WAL, and writes the schema and user_version 1 in one transaction. Open never creates a
// file, and refuses a file whose user_version is not 1 or whose journal_mode is not wal. The writer is one
// connection with busy_timeout 2000 ms, foreign_keys on, synchronous FULL and, on darwin, fullfsync and
// checkpoint_fullfsync; each of its transactions begins BEGIN IMMEDIATE. The reader is one connection with
// busy_timeout 2000 ms and query_only. wal_autocheckpoint keeps SQLite's default.
//
// # Limits
//
//   - No outbox, no intents, no feed, no store identity row, no lineage, no replication and no lake projection.
//   - One state table of opaque values: no typed column, no store-routed query and no index per declared query.
//     A value is stored whole, whatever its size.
//   - An entity with no component value is alive in the store, but no hydration can name it.
//   - Conflict is answered only for a deed whose eid is committed; the first seq and Ord of that deed are not
//     returned.
//   - No file lock: one owner process per file. Two processes keep the seq consistent, since it is read inside
//     BEGIN IMMEDIATE, but each engine misses the other's deeds.
package sqlite
