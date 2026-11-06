# The 32 Scenarios Matrix (Groups A–F)

> **Authority:** "Kill It and Prove It" Hackathon Guide (Section 5, Section 9A)  
> Every scenario below must survive `kill -9` and be proven in `ARCHITECTURE.md`.

## Group A: Crash During a Single Write
- **A1:** Kill before any bytes of a `put` reach the file. $\to$ Key absent (or holds previous value).
- **A2:** Kill after some bytes of record written (torn write). $\to$ Key absent or previous value; torn bytes ignored, never returned.
- **A3:** Kill after record is fully written but before `fsync`. $\to$ Either outcome acceptable (success was not returned); no corruption.
- **A4:** Kill after `fsync` returns but before `put` returns success to caller. $\to$ Key present with new value (durable even if caller never heard back).
- **A5:** Kill immediately after `put` returns success. $\to$ Key present with new value; zero exceptions.
- **A6:** Kill during a `delete`. $\to$ Key either still present with old value or absent. Never corrupted.
- **A7:** Kill during `put` of a 1 MB value. $\to$ Same rules as A1–A5 across multi-block boundaries.

## Group B: Crash During a Batch
- **B1:** Kill after 3 of 10 operations in a batch written. $\to$ None of the 10 visible.
- **B2:** Kill after all 10 operations written but commit marker is not. $\to$ None of the 10 visible.
- **B3:** Kill after commit marker written but before `fsync`. $\to$ Either all 10 visible or none visible. Never some.
- **B4:** Kill after batch returns success. $\to$ All 10 visible.
- **B5:** A batch that puts key X and deletes key X in same batch. $\to$ Defined ordering in `README.md` honoured after crash.
- **B6:** A batch of 10,000 operations killed halfway. $\to$ None visible; recovery completes in under 2 seconds.

## Group C: Crash During Recovery and Maintenance
- **C1:** Kill during `open()` while replaying the log. $\to$ Next `open()` succeeds and reaches same correct state.
- **C2:** Kill during `open()` while truncating or repairing torn tail. $\to$ Next `open()` succeeds.
- **C3:** Kill during compaction before new file replaces old. $\to$ Old file authoritative, all data intact.
- **C4:** Kill during compaction in middle of rename/swap. $\to$ Exactly one of old or new is authoritative, both complete, nothing lost.
- **C5:** Kill during compaction after swap before cleanup of temp file. $\to$ Data intact; temp file cleaned/ignored on next `open()`.
- **C6:** Kill during `close()`. $\to$ Same as kill at any other time. `close()` is not special.

## Group D: Concurrency
- **D1:** Two writers putting different keys, killed at random. $\to$ Every acknowledged put from either writer is present.
- **D2:** Two writers putting same key, killed at random. $\to$ Key holds value that one writer actually wrote; never a mix of two values.
- **D3:** One writer in long batch, another doing single puts, killed at random. $\to$ Batch is all-or-nothing; single puts acknowledged are present.
- **D4:** Reader calling `get` during a batch on same keys (no crash). $\to$ Reader sees state before batch or after batch, never in between.

## Group E: Disk Full
- **E1:** File size cap reached during single `put`. $\to$ `put` returns clean error; database remains readable; previous data intact.
- **E2:** Cap reached halfway through a batch. $\to$ Batch returns clean error; none of its operations visible.
- **E3:** Cap reached, then process killed. $\to$ Restart succeeds; all previously acknowledged data intact.
- **E4:** Cap reached during compaction. $\to$ Compaction fails or completes cleanly; no data lost; database opens.
- **E5:** After deletes free up space, compaction runs. $\to$ Database usable again without manual intervention.

## Group F: Repeated Abuse
- **F1:** 500 consecutive random kills and restarts. $\to$ Invariant holds after every single iteration.
- **F2:** Kill, restart, kill during recovery, restart, kill during recovery again. $\to$ Invariant holds; recovery is idempotent.
- **F3:** 1,000,000 keys written, process killed, `open()` called. $\to$ Recovery completes in under 2.0 seconds; all ACKed keys present.
- **F4:** Database file truncated by 1 byte at end by hand (simulates lost tail). $\to$ `open()` succeeds; at most last unacknowledged record lost.

