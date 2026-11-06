# Official 32 Scenarios

> Status: **Specification / implementation target**  
> Owner: common team  
> Canonical filename: `10_OFFICIAL_32_SCENARIOS.md`

## 1. Purpose and scope

The canonical A1–F4 scenario register required by the architecture proof.

## 2. Source of truth

Derived from the supplied official problem statement and engineering plans. The organiser's `Kill It and Prove It` specification takes precedence over proposed implementation details. The selected implementation plan is Go for the engine and Python for the torture tool.

## 3. Requirements

This register contains all 32 official scenarios in Groups A–F. It is a planning index; `ARCHITECTURE.md` must include the full required row columns: disk state at kill, mechanism, source function, and verified-by evidence. A scenario is not marked PASS until it has actually been run against the current code.

## 4. Design and procedure

Group A — single writes: A1 kill before any put bytes; A2 partial record/torn write; A3 full record written before sync; A4 sync returns before put returns; A5 immediately after put success; A6 during delete; A7 1 MiB value. Expected: A1/A2 preserve old state or absence; A3 may be old or new but no corruption; A4/A5 new value present; A6 old key or absent; A7 same rules as ordinary put.

## 5. Edge cases and failure behaviour

Group B — batches: B1 kill after some of ten operations written; B2 all operations written but no commit marker in marker-based design; B3 commit marker written before sync; B4 after batch success; B5 same key put and delete in one batch; B6 10,000-op batch killed halfway. For the selected UABE design there is no separate commit marker: the whole batch is one frame, and incomplete/invalid frame is dropped. B1/B2/B3/B6 must never expose a partial batch; B4 all operations visible; B5 payload order defines final state.

## 6. Verification and acceptance tests

Group C — recovery/maintenance: C1 kill while replaying; C2 kill while truncating/repairing tail; C3 kill during compaction before replacement; C4 kill during rename/swap; C5 kill after swap before temp cleanup; C6 kill during close. Recovery must be repeatable; old or new complete file authoritative; orphan temp ignored/removed; close is not a special crash guarantee.

Additional implementation notes:

Group D — concurrency: D1 two writers, distinct keys, random kill; D2 same key writers; D3 long batch plus single puts; D4 reader during batch without crash. All acknowledged writes survive; same key is one complete value from an actual writer; batch atomic; reader sees before or after state.

Group E — disk full: E1 cap during put; E2 cap during batch; E3 cap then process kill; E4 cap during compaction; E5 deletes followed by compaction restore write capacity. Fail cleanly; never corrupt prior data; batch remains all-or-nothing; compaction may fail or complete but cannot lose data.

Group F — repeated abuse: F1 500 random kill/restart iterations; F2 repeated kill during recovery; F3 one million keys then kill and recover under two seconds; F4 manually truncate final byte. Recovery must be correct and idempotent; F4 may lose at most the final unacknowledged record.

Maintain a status ledger: `NOT STARTED`, `IMPLEMENTED`, `TESTED`, `PASS`, `FAIL`, `NOT VERIFIED`. Include seed and artifact path for every pass. Never mark an untested scenario as passed because its mechanism seems obvious.

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
