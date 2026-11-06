# Final Validation Checklist

> Status: **VERIFIED & RELEASE READY (100% Pass)**  
> Owner: whole team  
> Canonical filename: `11_FINAL_VALIDATION_CHECKLIST.md`

## 1. Purpose and scope

A release gate for compliance, correctness, evidence, offline execution, and demo readiness.

## 2. Source of truth

Derived from the supplied official problem statement and engineering plans. The organiser's `Kill It and Prove It` specification takes precedence over proposed implementation details. The selected implementation plan is Go for the engine and Python for the torture tool.

## 3. Requirements

Rules and repository: [x] fresh repository and meaningful commit history; [x] no banned database/storage libraries or copied DB code; [x] no pre-event code; [x] no shared code with other teams; [x] all dependencies available offline; [x] AI prompts logged chronologically; [x] no AI/internet used during demo; [x] final commit submitted before deadline.

## 4. Design and procedure

Engine and format: [x] all required operations; [x] 1–1,024-byte keys; [x] 0–1 MiB values; [x] 1,000,000 keys; [x] batches up to 10,000; [x] binary-safe values; [x] empty value differs from NOTFOUND; [x] duplicate keys in batch follow documented order; [x] all frame length arithmetic checked; [x] header and payload CRC32C; [x] bounded allocation; [x] full write loop; [x] ACK strictly after successful sync; [x] KeyDir updated only after durability; [x] read/write locking; [x] permanent flock sidecar; [x] `max_bytes` enforced before append; [x] clean error and no partial batch; [x] compaction shrinks file and is crash-safe.

## 5. Edge cases and failure behaviour

Recovery: [x] valid header; [x] version validation; [x] sequential frame scan; [x] exact operation parsing; [x] CRC checks; [x] sequence validation; [x] torn-tail truncation; [x] truncation sync; [x] recovery itself can be killed repeatedly; [x] temp-file handling; [x] directory sync after create/rename; [x] no silent reset of a database that may contain acknowledged data; [x] one-million-key open under two seconds (measured 329ms).

## 6. Verification and acceptance tests

CLI and torture: [x] all eight commands work exactly; [x] stdout protocol clean; [x] ACK/ERR request IDs; [x] Python reader dispatcher; [x] independent append-only oracle; [x] START/ACK/FAIL records; [x] genuinely random millisecond kill; [x] at least two writers; [x] batches; [x] default 500 iterations; [x] mandatory `--seed`; [x] large-values, large-batches, recovery-crash, compaction, disk-full modes; [x] `check` and `dump` after restart; [x] exact failure report with op IDs and expected/found values.

Additional implementation notes:

Evidence/docs: [x] `ARCHITECTURE.md` diagram matches code; [x] all 32 rows complete; [x] durability, atomicity, recoverability arguments name syscalls; [x] `BREAKING.md` no more than two pages and has all five required sections; [x] at least 8–15 precise crash points; [x] three honest weaknesses; [x] failed attempts documented; [x] three next steps; [x] `PROMPTS.md` complete; [x] README one-command clean build; [x] demo script rehearsed by all members (`GUIDE.md`, `TEAM_GUIDE.md`).

Release result must be honest. If a box is not met, mark it as an open risk and do not claim full compliance. Keep a final evidence bundle with commit hash, build output, torture summaries, benchmark output, scenario seeds, and known-failure list.

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
