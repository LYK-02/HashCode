# GitHub Workflow

> Status: **Specification / implementation target**  
> Owner: common team  
> Canonical filename: `08_GITHUB_WORKFLOW.md`

## 1. Purpose and scope

Rules for repository setup, branches, commits, review, and auditability.

## 2. Source of truth

Derived from the supplied official problem statement and engineering plans. The organiser's `Kill It and Prove It` specification takes precedence over proposed implementation details. The selected implementation plan is Go for the engine and Python for the torture tool.

## 3. Requirements

Create a fresh repository after the hackathon start. Keep all work in the repository, including source, tests, docs, diagrams, and the prompt log. The official rules prohibit prewritten code and require meaningful commits throughout. Do not import a database implementation or copy source from an existing storage engine.

## 4. Design and procedure

Suggested branches: `main` is always buildable; short-lived branches `engine/write-path`, `engine/recovery`, `engine/compaction`, `torture/oracle`, `torture/modes`, and `docs/proof`. Avoid long-lived divergence. Merge through small reviewed commits. Keep the file-format and protocol specification on `main` before both teams implement against it.

## 5. Edge cases and failure behaviour

Commit cadence: commit the frozen spec; initial build skeleton; each codec feature; each engine operation; recovery; torture oracle; each test mode; compaction; disk-full; docs; and final release. Commit after meaningful verified units, not after every keystroke. Never rewrite history to hide work. Preserve failed experiments in the history and describe important failures in BREAKING.md.

## 6. Verification and acceptance tests

Before merging, require: build passes; relevant unit tests pass; integration protocol unchanged or versioned; no unreviewed binary-format changes; no banned dependency; no secrets or generated DB files; and test output recorded. Use `.gitignore` for binaries, temp databases, Python caches, and local logs, but keep reproducible failure artifacts when they are needed for diagnosis. Do not ignore `PROMPTS.md`.

Additional implementation notes:

Offline release check: clone into a new directory; disable Wi-Fi; run the single documented build command; run smoke tests; run `./kvdb check`; run a short torture seed; verify all executables are available without downloads. Ensure Go modules and Python dependencies are already available or use only standard library. Do not depend on containers unless already installed and demonstrably one-command reproducible.

The submitted repository must include `.git` history if the organiser requests a repository archive. Tag the final candidate, record its commit hash, and stop feature changes after final validation except critical fixes that are retested and documented.

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
