# Good Coding Practices & Code Quality Standards

> **Authority:** Systems Engineering Excellence for High-Durability Storage Systems

## 1. Zero-Panic Invariant & Explicit Error Handling
In a storage engine, unexpected crashes or unhandled exceptions corrupt state and violate reliability guarantees.
- **Explicit Result Types:** All fallible functions must return an explicit `Result<T, StorageError>` or equivalent domain error type.
- **No Careless Panics:** Never use raw `.unwrap()`, `.expect()`, or unhandled panic macros in the read, write, recovery, or compaction paths.
- **Granular Error Modeling:** Differentiate domain conditions cleanly:
  - `StorageError::TornRecord`
  - `StorageError::CrcMismatch`
  - `StorageError::DiskFull(String)`
  - `StorageError::KeyNotFound`
  - `StorageError::Io(std::io::Error)`
- **Recovery Resiliency:** When `open()` encounters a torn tail or invalid checksum at EOF, it must classify it as an expected uncommitted boundary and recover cleanly—never crash.

---

## 2. Deterministic File I/O & Syscall Discipline
- **Complete Write Loops:** Operating systems may perform partial writes. All write operations must loop until every byte in the slice is written (`write_all` equivalent).
- **Check Every Return:** Never ignore system call return values. Check every `read`, `write`, `fsync`, `rename`, and `truncate`.
- **Explicit Fsync Boundaries:** Never assume data is safe in OS page cache. Maintain the strict write ordering:
  `append payload` $\to$ `fsync` $\to$ `append commit marker` $\to$ `fsync` $\to$ `ACK`.
- **RAII Resource Management:** Rely on automatic destructor cleanup (e.g., Rust `Drop`, Go `defer`) so locks and file descriptors are never leaked under error conditions.

---

## 3. Memory Safety & Allocation Bounds
- **Guarded Allocations (Anti-OOM):** Never trust raw lengths from untrusted disk records. If a corrupted header claims a value length of 2GB, validating against the 1MB data limit and remaining file size prevents immediate Out-Of-Memory (OOM) crashes.
- **Index Efficiency:** Storing 1,000,000 keys in memory requires compact entries. Store key hashes or raw keys paired with 64-bit file offsets (`(u64 file_offset, u32 record_len)`) rather than duplicating full record payloads in RAM.
- **Zero-Copy Where Practical:** Use byte slices (`&[u8]`) and buffered streaming readers (`BufReader` with 64KB+ buffers) to achieve sub-2.0s recovery on 1,000,000 keys.

---

## 4. Binary Layout & Encoding Standards
- **Explicit Endianness:** Always encode integers using explicit byte orders (e.g., Little-Endian `to_le_bytes()` / `from_le_bytes()`).
- **Cryptographic/CRC Verification:** Every record header and commit marker must carry a CRC32 checksum computed over all preceding bytes.
- **Deterministic Record Boundaries:** Every record must start with a Magic Byte (`0x4B, 0x56` = "KV") and finish with a defined boundary so the recovery scanner can synchronize.

---

## 5. Architectural Modularity (Separation of Concerns)
Code must be structured into independent, easily auditable modules:
```text
src/
├── error.rs       # Domain error hierarchy
├── record.rs      # Binary record layout, opcodes, CRC32, serialization
├── log.rs         # Low-level append-only log file I/O & fsync calls
├── index.rs       # In-memory index mapping keys to file offsets
├── recovery.rs    # Startup replay, torn write truncation, validation
├── compaction.rs  # Crash-safe rewrite, temporary file swap & fsync
├── engine.rs      # High-level KVDB engine API (put, get, del, batch)
└── main.rs        # CLI wrapper implementation for ./kvdb
```

---

## 6. Testing & Verifiability
- **Unit Tests:** Every module (`record`, `index`, `log`, `recovery`) must have dedicated unit tests covering torn writes, CRC errors, empty files, and 1MB values.
- **Code Formatting:** Code must pass standard formatters and linters (`cargo fmt`, `cargo clippy`, `gofmt`) with zero warnings.
- **Judges' "Crash-Here" Readiness:** Code must be clear, readable, and self-documenting so team members can explain what is on disk at any arbitrary instruction.

