# In-Memory Index

> Status: **Specification / implementation target**  
> Owner: engine team  
> Canonical filename: `13_IN_MEMORY_INDEX.md`

## 1. Purpose and scope

Defines KeyDir entries, reconstruction, locators, memory strategy, and consistency.

## 2. Source of truth

Derived from the supplied official problem statement and engineering plans. The organiser's `Kill It and Prove It` specification takes precedence over proposed implementation details. The selected implementation plan is Go for the engine and Python for the torture tool.

## 3. Requirements

The KeyDir is a derived in-memory hash map from key bytes to a locator for the current value in `data.db`. It is not persisted separately. A locator contains value offset, value length, and sequence/generation metadata. The map contains only live keys; delete removes an entry. A value length of zero is valid and still represents a present key.

## 4. Design and procedure

Recovery builds the map by scanning valid frames in sequence. For each PUT, overwrite the key's locator; for each DEL, remove it. Duplicate keys in a batch are applied in payload order. Only a fully validated frame is applied. The KeyDir must not point to bytes in an invalid tail frame. After tail truncation, every locator must remain within the validated file prefix.

## 5. Edge cases and failure behaviour

Use Go `map[string]KeyDirEntry` for arbitrary byte keys represented as strings, or a byte-key wrapper with stable hashing. Ensure conversion copies/owns bytes as required. Preallocate capacity for one million keys to reduce map growth during recovery, but measure memory and runtime. The Rust blueprint's FxHashMap/SIMD advice is not directly applicable to this Go implementation; use Go profiling before changing hash strategy.

## 6. Verification and acceptance tests

Index update is atomic from readers' perspective because the writer holds the exclusive mutex across all updates. A batch can update many entries; no reader may observe a prefix of those changes. `Get` obtains read lock, finds locator, reads exact bytes with ReadAt, verifies read length, and returns a copy.

Additional implementation notes:

Compaction creates a new file with only live values. Build a new KeyDir whose offsets refer to the new file; do not mutate the active KeyDir to new offsets before the rename is committed. After replacement and directory sync, atomically swap descriptor and index while holding the writer lock. Ensure old descriptors are closed only after no reader can use them.

Tests: one million unique keys; overwrites; deletes; zero-length values; random binary keys; locator boundary checks; index rebuild equals live-state oracle; no stale offsets after compaction; no partial batch visibility; and recovery memory/time benchmark.

## 7. Implementation notes

- Keep this document synchronized with the actual code and tests.
- Do not claim a behaviour is implemented or verified until evidence exists.
- For persistence changes, update the shared format, architecture diagram, scenario matrix, and torture parser in the same change.
- Record the exact command, seed, commit hash, and result for every crash test.

## 8. Related documents

- `../common/02_OFFICIAL_REQUIREMENTS.md`
- `../common/04_SHARED_DATA_FORMAT.md`
- `../common/05_CLI_AND_PROTOCOL_SPEC.md`
- `../common/10_OFFICIAL_32_SCENARIOS.md`
