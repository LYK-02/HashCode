# Torture Tool Overview

> Status: **Specification / implementation target**  
> Owner: torture team  
> Canonical filename: `01_TORTURE_OVERVIEW.md`

## 1. Purpose and scope

Defines the independent adversarial tester and its pass/fail contract.

## 2. Source of truth

Derived from the supplied official problem statement and engineering plans. The organiser's `Kill It and Prove It` specification takes precedence over proposed implementation details. The selected implementation plan is Go for the engine and Python for the torture tool.

## 3. Requirements

The torture tool is an independent Python 3 program that attacks the Go `kvdb` process through the public CLI/serve interface. It is a required deliverable, not a unit-test wrapper. Its purpose is to prove that acknowledged operations survive random process death and that unacknowledged operations are never partially applied.

## 4. Design and procedure

Default invocation: `./torture --iterations 500 --seed 12345`. Required targeted examples include `./torture --iterations 100 --seed 999 --mode large-values`, `--mode compaction`, and `--mode recovery-crash`; add explicit large-batches and disk-full modes. The `--seed` flag is mandatory. Judge will use a seed not used by the team.

## 5. Edge cases and failure behaviour

Each iteration: create/reuse isolated DB; start serve child; start at least two concurrent writers; generate PUT/DEL/BATCH operations; persist independent START records; record ACKs only when received; wait a genuinely random millisecond delay; send SIGKILL; wait for process; restart; run `check`; run `dump`; parse actual state; compare to oracle; print one-line iteration result; continue. On first failure stop and print expected vs found plus operation IDs and preserve artifacts.

## 6. Verification and acceptance tests

The oracle must not import engine code, inspect KeyDir, parse private memory, or construct expectations from the database dump. It stores the independent history of requests and observed ACKs. The dump is only the actual state being checked. For in-flight operations with no observed ACK, allow wholly present or wholly absent outcomes; never allow partial batch or invalid value.

Additional implementation notes:

A successful run requires zero invariant violations, all checks completed, no harness crash, and reproducible metadata. A harness that kills only between operations, uses SIGTERM, lacks concurrent writers, skips batches, or trusts engine output is weak and may be disqualified if rigged.

Failure bundle: seed, iteration, mode, exact command, engine commit hash, harness commit hash, START/ACK/FAIL event log, stderr/stdout, expected model, actual dump, database copy, and reproduction command. Never overwrite the first failure while trying to continue.

## 7. Implementation notes

- Keep this document synchronized with the actual code and tests.
- Do not claim a behaviour is implemented or verified until evidence exists.
- For persistence changes, update the shared format, architecture diagram, scenario matrix, and torture parser in the same change.
- Record the exact command, seed, commit hash, and result for every crash test.

## 8. Related documents

- `16_TORTURE_TESTING.md`
- `17_CRITERIA_AND_IMPROVEMENTS.md`
- `PHASES.md`
- `../common/02_OFFICIAL_REQUIREMENTS.md`
- `../common/04_SHARED_DATA_FORMAT.md`
- `../common/05_CLI_AND_PROTOCOL_SPEC.md`
- `../common/10_OFFICIAL_32_SCENARIOS.md`
