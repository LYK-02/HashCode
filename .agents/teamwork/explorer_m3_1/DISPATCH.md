## 2026-11-06T14:30:11Z
From: 9fade4e7-7ff3-4d78-9ecd-6f9b6c00f5be
Priority: MESSAGE_PRIORITY_HIGH

You are explorer_m3_1 (role: M3 Batch Atomicity Explorer).
Your working directory: /home/laik/workspace/srinathonn/.agents/teamwork/explorer_m3_1/
Your parent: 9fade4e7-7ff3-4d78-9ecd-6f9b6c00f5be

First, read the authoritative user request at:
/home/laik/workspace/srinathonn/.agents/teamwork/ORIGINAL_REQUEST.md
Also read:
- /home/laik/workspace/srinathonn/.agents/teamwork/PROJECT.md
- /home/laik/workspace/srinathonn/docs/engine/07_BATCH_ATOMICITY.md
- /home/laik/workspace/srinathonn/docs/common/04_SHARED_DATA_FORMAT.md
- /home/laik/workspace/srinathonn/internal/engine/write.go
- /home/laik/workspace/srinathonn/internal/engine/keydir.go
- /home/laik/workspace/srinathonn/internal/format/record.go
- /home/laik/workspace/srinathonn/internal/format/uabe.go

Task:
Investigate and produce an architectural specification and implementation strategy for Phase E4 (Atomic Batch Mutations):
1. Batch Invariants & Requirements:
   - Batch size bounds: 1 to 10,000 operations per batch (enforce limits, reject empty batch or >10,000 ops with format.ErrBatchEmpty / format.ErrBatchTooLarge).
   - Single UABE Frame Envelope: All operations in a batch MUST be serialized into a single UABE frame with `op_count = len(ops)` in the 28-byte header and a single 4-byte CRC32C trailer over all combined subrecords.
   - All-or-nothing atomicity: A batch is committed if and only if the full frame is written, `file.Sync()` succeeds, and all operations are published to `KeyDir`. If a crash occurs or disk write fails, ZERO operations take effect.
2. Index publication & Locators:
   - In a multi-operation batch, how are physical locators (`ValueOffset`) computed for each operation in the batch? Inspect `format.ValueFileOffset` and subrecord layout.
   - How should `KeyDir` updates be staged and published atomically under `e.mu.Lock()`?
   - What happens when a batch contains duplicate keys or interleaves PUT and DEL for the same key? (Last-Operation-Wins within the batch).
3. CLI & Protocol Integration:
   - How does `./kvdb batch <path>` receive and parse operations (e.g. line-oriented stdin `PUT key val\nDEL key\n`)?
   - What error codes and outputs should be emitted?
4. Write handoff report to `/home/laik/workspace/srinathonn/.agents/teamwork/explorer_m3_1/handoff.md`.
Update `progress.md` with status and timestamp.
Send completion message to orchestrator (`9fade4e7-7ff3-4d78-9ecd-6f9b6c00f5be`) with report path.
