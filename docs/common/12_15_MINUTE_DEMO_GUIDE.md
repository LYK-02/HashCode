# 15-Minute Demo Guide

> Status: **Specification / implementation target**  
> Owner: common team  
> Canonical filename: `12_15_MINUTE_DEMO_GUIDE.md`

## 1. Purpose and scope

Minute-by-minute plan matching the official judging session.

## 2. Source of truth

Derived from the supplied official problem statement and engineering plans. The organiser's `Kill It and Prove It` specification takes precedence over proposed implementation details. The selected implementation plan is Go for the engine and Python for the torture tool.

## 3. Requirements

The judge drives the laptop; team members explain. Keep Wi-Fi off and close AI/browser tools before the session. The demo is fixed at 15 minutes. Have the repository, terminal, source editor, architecture diagram, and BREAKING.md ready. Do not improvise commands that have not been rehearsed.

## 4. Design and procedure

0–1 minute — clean offline build. Clone or use a clean clone, disable Wi-Fi, run the exact README build command. Show that the binary is created and runs. If the build fails, the judge moves on; do not spend the demo debugging.

1–4 minutes — torture run. Judge runs `./torture --iterations 200 --seed <judge-seed>`. Explain the independent oracle, random kill timing, concurrent writers, ACK semantics, and post-restart comparison. The expected result is zero invariant violations.

4–6 minutes — targeted modes. Judge runs compaction and recovery-crash modes with judge-selected seeds. Show the temp-file sync/rename/directory-sync order and idempotent recovery. Do not claim modes pass unless the current binary demonstrates it.

## 5. Edge cases and failure behaviour

6–8 minutes — manual serve kill. Start `./kvdb serve <path>`, send a batch, kill the process 5–10 times at judge-selected moments, then run `check` and `dump`. Explain ACK only after fsync and why an in-flight frame can be present or absent but not partial.

8–12 minutes — crash-here questions. Judge points to source lines. Answer in this order: exact instruction; what may have reached the file/page cache; what has been synced; whether ACK has been emitted; how next `open` scans/truncates/replays; what the oracle expects. Use precise offsets and function names from the current source. Every member should be able to explain recovery.

## 6. Verification and acceptance tests

12–15 minutes — TTL/expiry design extension. Do not code. Explain a format-version change, expiry field semantics, lazy read visibility, replay behaviour, compaction filtering, clock rollback policy, and zombie resurrection. Critical trap: an expired newer value must act as a tombstone during replay; simply skipping it could resurrect an older immortal value. Explain how batches with expiry and overwrite order behave.

Additional implementation notes:

Rules: may inspect own code and BREAKING.md; may not use AI or internet. Any member may answer, but judge may direct a question to one person. If a tool crashes, those points are lost. If a defect is found, diagnose honestly and explain exact failure rather than denying it.

Prepare three rehearsed crash-here locations: between append and sync; in the middle of temp-file write before sync; and after recovery truncation before sync. Keep line numbers updated immediately before release. Never present a stale architecture diagram.

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
