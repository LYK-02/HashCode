## 2026-11-06T11:50:45Z
MANDATORY FIRST STEP: Read `/home/laik/workspace/srinathonn/.agents/teamwork/ORIGINAL_REQUEST.md` before starting work.

Context: Milestone M1 (Core Foundation & Codecs: Phases E0 and E1).
Scope Document: `/home/laik/workspace/srinathonn/.agents/teamwork/PROJECT.md`
Prior Survey Reports:
- `/home/laik/workspace/srinathonn/.agents/teamwork/spec_miner_survey_1/handoff.md`
- `/home/laik/workspace/srinathonn/.agents/teamwork/spec_miner_survey_2/handoff.md`

Working directory: `/home/laik/workspace/srinathonn/.agents/teamwork/spec_miner_m1_3/`
Identity: `spec_miner_m1_3`

Task:
Mine and specify the exhaustive test plan and golden test fixtures for Milestone M1 (Phases E0 & E1):
1. Exact byte-for-byte golden fixtures: Provide concrete hex bytes for 32-byte DB Header, 28-byte UABE Frame Header, PUT subrecords, DEL subrecords, and 4-byte CRC32C trailer.
2. Boundary test cases:
   - Key length 0 (rejected), 1 byte (accepted), 1024 bytes (accepted), 1025 bytes (rejected).
   - Value length 0 (accepted), 1 byte (accepted), 1,048,576 bytes (accepted), 1,048,577 bytes (rejected).
   - Batch op count 0 (rejected), 1 (accepted), 10,000 (accepted), 10,001 (rejected).
   - DEL operation with value length > 0 (rejected).
3. Corruption test vectors:
   - Bit-flip in DB header magic / version / CRC.
   - Bit-flip in UABE magic `0x55414245` / header CRC.
   - Bit-flip in payload bytes / trailer CRC.
   - Short headers (<32 bytes, <28 bytes, truncated payload).
4. Concrete test function signatures and assertions for `internal/format/codec_test.go`.

Deliverable:
Write a comprehensive test specification report to `/home/laik/workspace/srinathonn/.agents/teamwork/spec_miner_m1_3/handoff.md`.
Do NOT implement code directly. Send a message to your caller upon completion with the path to your handoff report.
