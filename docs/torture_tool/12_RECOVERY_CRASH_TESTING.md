# Recovery Crash Testing

> Status: **Specification / implementation target**  
> Owner: torture team  
> Canonical filename: `12_RECOVERY_CRASH_TESTING.md`

## 1. Purpose and scope

Defines repeated kills during open, replay, and tail repair.

## 2. Source of truth

Derived from the supplied official problem statement and engineering plans. The organiser's `Kill It and Prove It` specification takes precedence over proposed implementation details. The selected implementation plan is Go for the engine and Python for the torture tool.

## 3. Requirements

Recovery-crash mode must create a database large enough that open/replay has a meaningful time window. Start `kvdb open <path>` as a child process, then use the seeded random delay scheduler to SIGKILL it while recovery is active. Restart and repeat. The next open must reach the same logical state and complete successfully.

## 4. Design and procedure

Test both replay and repair. For replay, create many valid frames and kill during scanner progress. For repair, create a known torn tail and kill during truncate/sync. The recovery algorithm is idempotent: scanning valid frames reconstructs the same KeyDir; truncating to the same last-valid offset can be repeated safely. Do not advance recovery metadata or expose the engine before repair sync completes.

## 5. Edge cases and failure behaviour

To ensure kills overlap recovery, measure open duration and select delays within that window. Record whether kill occurred before process exit and whether the child had entered recovery (use stderr trace in test build or a readiness marker that does not alter durable state). A random delay alone that always kills before process starts or after recovery completes is not meaningful coverage.

## 6. Verification and acceptance tests

After each kill, restart, run check and dump, and compare to the oracle. The database may contain any valid acknowledged frames and wholly present in-flight frames. It must not contain partial frames. Repeat kill-recovery-kill at least several times to exercise F2.

Additional implementation notes:

Tests: kill during valid frame replay; kill during KeyDir build; kill during tail ftruncate; kill during truncate sync; repeated restarts; one-byte tail loss; no-tail recovery; one-million-key recovery; and timing under two seconds.

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
