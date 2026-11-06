# Final Validation Checklist

> Status: **Specification / implementation target**  
> Owner: common team  
> Canonical filename: `11_FINAL_VALIDATION_CHECKLIST.md`

## 1. Purpose and scope

A release gate for compliance, correctness, evidence, offline execution, and demo readiness.

## 2. Source of truth

Derived from the supplied official problem statement and engineering plans. The organiser's `Kill It and Prove It` specification takes precedence over proposed implementation details. The selected implementation plan is Go for the engine and Python for the torture tool.

## 3. Requirements

Rules and repository: [ ] fresh repository and meaningful commit history; [ ] no banned database/storage libraries or copied DB code; [ ] no pre-event code; [ ] no shared code with other teams; [ ] all dependencies available offline; [ ] AI prompts logged chronologically; [ ] no AI/internet used during demo; [ ] final commit submitted before deadline.

## 4. Design and procedure

Engine and format: [ ] all required operations; [ ] 1–1,024-byte keys; [ ] 0–1 MiB values; [ ] 1,000,000 keys; [ ] batches up to 10,000; [ ] binary-safe values; [ ] empty value differs from NOTFOUND; [ ] duplicate keys in batch follow documented order; [ ] all frame length arithmetic checked; [ ] header and payload CRC32C; [ ] bounded allocation; [ ] full write loop; [ ] ACK strictly after successful sync; [ ] KeyDir updated only after durability; [ ] read/write locking; [ ] permanent flock sidecar; [ ] `max_bytes` enforced before append; [ ] clean error and no partial batch; [ ] compaction shrinks file and is crash-safe.

## 5. Edge cases and failure behaviour

Recovery: [ ] valid header; [ ] version validation; [ ] sequential frame scan; [ ] exact operation parsing; [ ] CRC checks; [ ] sequence validation; [ ] torn-tail truncation; [ ] truncation sync; [ ] recovery itself can be killed repeatedly; [ ] temp-file handling; [ ] directory sync after create/rename; [ ] no silent reset of a database that may contain acknowledged data; [ ] one-million-key open under two seconds.

## 6. Verification and acceptance tests

CLI and torture: [ ] all eight commands work exactly; [ ] stdout protocol clean; [ ] ACK/ERR request IDs; [ ] Python reader dispatcher; [ ] independent append-only oracle; [ ] START/ACK/FAIL records; [ ] genuinely random millisecond kill; [ ] at least two writers; [ ] batches; [ ] default 500 iterations; [ ] mandatory `--seed`; [ ] large-values, large-batches, recovery-crash, compaction, disk-full modes; [ ] `check` and `dump` after restart; [ ] exact failure report with op IDs and expected/found values.

Additional implementation notes:

Evidence/docs: [ ] `ARCHITECTURE.md` diagram matches code; [ ] all 32 rows complete; [ ] durability, atomicity, recoverability arguments name syscalls; [ ] `BREAKING.md` no more than two pages and has all five required sections; [ ] at least 8–15 precise crash points; [ ] three honest weaknesses; [ ] failed attempts documented; [ ] three next steps; [ ] `PROMPTS.md` complete; [ ] README one-command clean build; [ ] demo script rehearsed by all members.

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
