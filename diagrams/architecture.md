# KVDB System Architecture Diagrams (Deliverable 5)

> **Hackathon:** Kill It and Prove It - 24-Hour Engineering Hackathon  
> **Target:** Go Engine (`./kvdb`) & Python Torture Harness (`./torture`)  
> **Source of Truth:** Section 9A / `docs/common/03_SYSTEM_ARCHITECTURE.md`

---

## 1. Component Map, Concurrency & Ownership Boundaries

```mermaid
flowchart TD
    subgraph ClientAndHarness ["Client and Test Harness Isolation"]
        Harness["Python Torture Harness ./torture"]
        CLI["CLI Driver: put, get, del, batch, dump, check"]
        Serve["Serve Daemon: JSON-Lines IPC Protocol"]
    end

    subgraph ProcessIsolation ["OS Process and Locking Isolation"]
        SidecarLock["Sidecar Lock Inode: data.db.lock<br/>Exclusive flock LOCK_EX"]
        ProcMutex["In-Process Concurrency Control<br/>sync.RWMutex: One Exclusive Writer"]
    end

    subgraph MemoryState ["Volatile In-Memory Engine State (Derived Only)"]
        KeyDir["KeyDir Hash Index<br/>map string KeyDirEntry<br/>Offset, ValueLen, SeqNo, Generation"]
        QuotaGuard["Quota Guard<br/>valid_eof + frame_size &le; max_bytes"]
        SeqCounter["Global Monotonic Sequence Counter<br/>next_seq_no"]
    end

    subgraph CoreEnginePipeline ["Core Engine Pipeline"]
        Encoder["UABE Frame Codec<br/>Magic 0x55414245 + HdrCRC + Ops + PayloadCRC"]
        LogWriter["Position-Independent Log Writer<br/>os.File.WriteAt / pwrite"]
        SyncBarrier["Hardware Durability Barrier<br/>os.File.Sync / fsync"]
        Scanner["Streaming Recovery Scanner<br/>64KB BufReader + Dual CRC32C Validator"]
        Compactor["Copy-on-Write Compactor<br/>Temp File -&gt; Sync -&gt; Rename -&gt; DirSync"]
    end

    subgraph PersistentStorage ["Physical Storage Media"]
        ActiveDB[("Authoritative Log File<br/>data.db<br/>32B Header + Repeating UABE Frames")]
        TempDB[("Compaction Temp File<br/>data.db.compact.tmp")]
        ParentDir[("Parent Directory Metadata<br/>Directory Descriptor + fsync")]
    end

    %% Client Interactions
    Harness -->|"Subprocess CLI / Stdin-Stdout IPC"| Serve
    CLI --> ProcMutex
    Serve --> ProcMutex

    %% Concurrency & Locking
    ProcMutex --> SidecarLock
    ProcMutex --> QuotaGuard
    ProcMutex --> KeyDir
    ProcMutex --> SeqCounter

    %% Write Pipeline
    QuotaGuard -->|"Quota OK"| Encoder
    Encoder --> LogWriter
    LogWriter -->|"1. WriteAt frame to valid_eof"| ActiveDB
    LogWriter -->|"2. File.Sync physical barrier"| SyncBarrier
    SyncBarrier -->|"3. Durable barrier return"| KeyDir
    KeyDir -->|"4. Publish mutation and advance EOF"| Serve
    Serve -->|"5. Emit ACK to client"| Harness

    %% Recovery Pipeline
    SidecarLock -->|"Acquired before recovery"| Scanner
    Scanner -->|"Stream and validate CRC32C"| ActiveDB
    Scanner -->|"Reconstruct valid state"| KeyDir
    Scanner -->|"Clean orphan temp"| TempDB

    %% Compaction Pipeline
    ProcMutex -->|"Exclusive writer lock held"| Compactor
    KeyDir -->|"Iterate live keys"| Compactor
    Compactor -->|"1. Write live frames"| TempDB
    Compactor -->|"2. Temp file sync"| SyncBarrier
    Compactor -->|"3. Atomic rename temp over target"| ActiveDB
    Compactor -->|"4. Parent directory sync"| ParentDir
    Compactor -->|"5. Swap descriptors and rebuild KeyDir"| KeyDir
```

---

## 2. Numbered Write & Commit Sequence (Zero Phantom ACK Guarantee)

```mermaid
sequenceDiagram
    autonumber
    actor Client as Client / Torture Tool
    participant IPC as Serve Protocol / CLI
    participant Engine as Engine State (Go)
    participant Disk as Physical Storage File
    participant KeyDir as In-Memory Index

    Client->>IPC: {"op": "put", "key": "k1", "value": "v1"}
    IPC->>Engine: Put("k1", "v1")
    Note over Engine: Step 1: Input Validation (Key <= 1KB, Val <= 1MB)
    Engine->>Engine: Acquire sync.RWMutex.Lock() [Exclusive]
    Note over Engine: Step 2: Encode UABE Frame in RAM (Header + Payload + CRC32C Trailer)
    Note over Engine: Step 3: Quota Guard Check (valid_eof + frame_len <= max_bytes)
    
    Engine->>Disk: Step 4: WriteAt(frame_bytes, valid_eof) [Full write loop]
    Engine->>Disk: Step 5: File.Sync() [fsync durability barrier]
    Disk-->>Engine: fsync returned successfully (Bytes durable on physical medium)
    
    Engine->>KeyDir: Step 6: Apply operation to KeyDir (Set k1 -> offset, len, seq)
    Engine->>Engine: Step 7: Advance valid_eof += frame_len and next_seq_no++
    Engine->>Engine: Release sync.RWMutex.Unlock()
    
    Engine-->>IPC: Operation Committed (op_id, seq)
    IPC-->>Client: ACK <op-id> (Acknowledged to caller)

    Note over Client,Disk: INVARIANT: If kill -9 strikes anywhere in Steps 1-5, NO ACK was emitted.<br/>If kill -9 strikes in Steps 6-7, recovery scanner finds valid CRC and recovers the key.
```

---

## 3. Compaction Lifecycle & Atomic Swap

```mermaid
sequenceDiagram
    autonumber
    participant Worker as Compaction Worker
    participant Engine as Engine State (Write Lock)
    participant Temp as Temp File (data.db.compact.tmp)
    participant Disk as Authoritative File (data.db)
    participant Dir as Parent Directory

    Note over Worker: Trigger: dead_bytes > 50% && total_size > 10MB
    Worker->>Engine: Acquire Exclusive mu.Lock() (Block all concurrent writes)
    Worker->>Temp: Create temp file with Header (Gen = old_gen + 1)
    
    loop For each live entry in KeyDir
        Worker->>Disk: Read live value at recorded offset
        Worker->>Temp: Append live UABE frame
    end
    
    Worker->>Temp: File.Sync() [Ensure all compacted frames reach physical storage]
    Worker->>Temp: Close temp file descriptor
    
    Worker->>Disk: os.Rename(temp_path, authoritative_path) [POSIX Atomic Rename]
    Worker->>Dir: Open parent directory & call dir.Sync() [Persist rename metadata]
    
    Worker->>Engine: Re-open authoritative file descriptor & rebuild KeyDir
    Worker->>Engine: Release mu.Unlock()
    Note over Worker,Disk: If killed before Rename: old file intact, orphan temp pruned on open.<br/>If killed after Rename: new file complete, header gen verified.
```

---

## 4. Crash Recovery Pipeline Flowchart

```mermaid
flowchart TD
    Start(["Engine Open Request (path, max_bytes)"]) --> LockSidecar["1. Open & acquire exclusive flock on <path>.lock"]
    
    LockSidecar --> CheckExists{"File exists & size >= 32?"}
    CheckExists -- No --> InitHeader["Create new file & write 32B Header (KVDB2026, Gen 0) & fsync"]
    InitHeader --> PruneTemp["Prune orphan .compact.tmp if present"]
    PruneTemp --> Ready(["Engine Online (Empty KeyDir, EOF=32)"])
    
    CheckExists -- Yes --> ReadHeader["2. Read 32-byte header & verify Magic + CRC32C"]
    ReadHeader --> HeaderValid{"Header Magic == 'KVDB2026' & Header CRC valid?"}
    HeaderValid -- No --> FatalPanic["Fatal Error: Unrecoverable header corruption"]
    
    HeaderValid -- Yes --> ScanLoop["3. Initialize Streaming BufReader at offset 32<br/>Set scan_offset = 32, valid_eof = 32"]
    ScanLoop --> ReadFrameHdr["Read 28-byte UABE Frame Header"]
    
    ReadFrameHdr --> FrameHdrValid{"Bytes read == 28 &&<br/>Magic == 0x55414245 &&<br/>HdrCRC32C valid?"}
    FrameHdrValid -- No (EOF or Torn) --> HaltScan["Stop scanning: torn/partial frame or clean EOF encountered"]
    
    FrameHdrValid -- Yes --> CheckPayloadBounds{"PayloadLen <= 64MB &&<br/>OpCount <= 10,000?"}
    CheckPayloadBounds -- No --> HaltScan
    
    CheckPayloadBounds -- Yes --> ReadPayload["Read PayloadLen bytes + 4-byte Trailer CRC"]
    ReadPayload --> TrailerValid{"Trailer CRC32C == CRC(Payload)?"}
    TrailerValid -- No (Torn Payload) --> HaltScan
    
    TrailerValid -- Yes --> ApplyOps["Apply all operations in frame to KeyDir in payload order"]
    ApplyOps --> AdvanceOffset["valid_eof = scan_offset + 28 + PayloadLen + 4<br/>scan_offset = valid_eof<br/>next_seq_no = max(next_seq_no, frame.seq + 1)"]
    AdvanceOffset --> ScanLoop
    
    HaltScan --> TruncateCheck{"valid_eof < actual_file_size?"}
    TruncateCheck -- Yes --> TruncateTail["4. Truncate file to valid_eof & call File.Sync()"]
    TruncateCheck -- No --> PruneTemp
    TruncateTail --> PruneTemp
```

---

## 5. On-Disk Binary Specification (Exact Byte Offsets)

```mermaid
classDiagram
    class FileHeader32B {
        +00..08: Magic Bytes [8B] = 'KVDB2026' (0x4B56444232303236)
        +08..12: Version [4B] = 1 (u32 LE)
        +12..20: Compaction Generation [8B] (u64 LE)
        +20..28: Reserved Flags [8B] = 0 (u64 LE)
        +28..32: Header CRC32C [4B] (Checksum over bytes 0..28)
    }

    class UABEFrameHeader28B {
        +00..04: Frame Magic [4B] = 0x55414245 ('UABE')
        +04..08: Header CRC32C [4B] (Checksum over bytes 8..28)
        +08..16: Payload Length [8B] (u64 LE, total payload bytes)
        +16..24: Mutation Sequence [8B] (u64 LE, monotonic global)
        +24..28: Operation Count [4B] (u32 LE, 1 to 10,000 ops)
    }

    class OperationRecord {
        +00..01: OpType [1B] (0x01 = PUT, 0x02 = DEL)
        +01..03: Key Length [2B] (u16 LE, 1 to 1,024 bytes)
        +03..07: Value Length [4B] (u32 LE, 0 to 1,048,576 bytes; 0 for DEL)
        +07..KeyLen: Raw Key Bytes
        +KeyLen..ValLen: Raw Value Bytes
    }

    class UABEFrameTrailer4B {
        +00..04: Payload CRC32C [4B] (Castagnoli checksum over entire payload body)
    }

    FileHeader32B "1" --> "0..N" UABEFrameHeader28B : followed by
    UABEFrameHeader28B "1" --> "1..N" OperationRecord : contains payload of
    OperationRecord "1..N" --> "1" UABEFrameTrailer4B : sealed by
```

