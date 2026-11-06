# Design Decision Record

## Selected stack: Go engine + Python torture tool

**Decision:** Use Go for the engine and CLI, Python 3 for the external torture tool.

**Reason:** The latest supplied Antigravity Engine Final Plan explicitly assigns Go to `src/` and Python to `torture/`. The official organiser rules permit either language. The earlier architectural blueprint recommends Rust, but that is an engineering recommendation rather than an official constraint. This repository must remain internally consistent, so Go + Python is selected.

## Selected persistence model: one authoritative append-only log

Use a 32-byte header and UABE frames. The KeyDir is in memory and rebuilt from the log. Do not add a persistent index or separate manifest.

## Selected atomicity model: one frame per logical mutation

PUT, DEL and BATCH all serialize into one UABE frame. A frame is validated completely before recovery applies any operation. The entire frame is synced before KeyDir publication and ACK.

## Selected lock model

Use one process-wide `sync.RWMutex` and one permanent sidecar lock file protected by exclusive `flock`. Simplicity and correctness are preferred over lock-free concurrency.

## Selected compaction order

Write temp → sync temp → close temp → rename temp over database → sync parent directory → swap in-memory descriptor/index while holding writer lock.

## Open decisions that must be frozen before implementation

1. Exact serve-mode BATCH grammar and whether ACK includes a durable sequence number.
2. Exact one-shot CLI encoding for binary values; serve mode is hex/binary-safe.
3. Whether `max_bytes` includes the header and whether it applies to compaction temp size.
4. Empty-batch behaviour (recommended: reject as invalid/no frame).
5. Delete-missing behaviour (officially not an error; recommended durable tombstone for simple ordering).
6. Supported OS/filesystem target for directory sync and flock.
7. Stable error-code grammar and stdout/stderr policy.

No implementation should begin in parallel until these choices are committed.
