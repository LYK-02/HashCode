Audit the existing `torture_tool/` implementation against `docs/torture_tool/`, produce a phased improvement plan, and execute each phase systematically to make the torture tool robust and reliable for database resilience evaluation and testing.

Working directory: /home/ashwin/HashCode
Integrity mode: development

## Requirements

### R1. Audit and Improvement Plan
Study `docs/torture_tool/` documentation and `torture_tool/` source code thoroughly to understand every plan, specification, module, dependency, test, and existing implementation. Audit the current implementation to identify bugs, missing functionality, weaknesses, and testing gaps. Produce a detailed, phased improvement plan covering errors, the independent oracle, concurrency, crash injection, ACK verification, recovery testing, reproducibility, reporting, and torture modes. Do not start implementation until the plan is presented and approved by the user.

### R2. Systematic Phase Execution and Hardening
After plan approval, execute each phase systematically within `torture_tool/`: fix existing errors, strengthen the independent oracle, concurrency, crash injection, ACK verification, recovery testing, reproducibility, reporting, and every torture mode. Add comprehensive tests, run stress tests, and verify actual results. Preserve existing functionality unless a documented improvement requires changing it. Work phase by phase, maintain a clear progress checklist, report changed files and actual test evidence, fix failures before proceeding, and never claim unverified success.

### R3. Strict File Boundaries and Teammate Isolation
Read `docs/torture_tool/` as the technical reference. Modify, create, or delete files ONLY inside `torture_tool/`. Never modify `docs/`, `engine/`, root files, or shared configurations. Do not modify or interfere with teammates' work.

## Verification Resources
- Use existing tests located in `torture_tool/tests/`.
- Verify torture tool execution against the compiled `./kvdb` binary.

## Acceptance Criteria

### Audit & Approval
- [ ] Detailed audit and phased improvement plan is documented covering bugs, gaps, oracle, concurrency, crash injection, ACK verification, recovery testing, reproducibility, reporting, and torture modes before any implementation begins.
- [ ] Implementation begins only after explicit user approval of the improvement plan.

### Functionality & Verification
- [ ] All unit and integration tests in `torture_tool/tests/` pass.
- [ ] All torture test modes run against `./kvdb` without unhandled internal exceptions for standard verification runs.
- [ ] Progress checklist is updated phase by phase with reported changed files and test evidence.
- [ ] Existing functionality is preserved unless documented as an improvement in the plan.

### Boundary Compliance
- [ ] All modified, created, or deleted files are strictly confined to `torture_tool/`.
- [ ] No files in `docs/`, `engine/`, root, or shared configurations are modified.
