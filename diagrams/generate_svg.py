#!/usr/bin/env python3
"""
Generates the authoritative standalone pre-rendered architecture SVG diagram
for Deliverable 5 (diagrams/architecture.svg).
Offline-compatible, self-contained, no external font or network dependencies.
"""

def generate_svg():
    width = 1600
    height = 1150
    
    svg = f"""<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 {width} {height}" width="100%" height="100%" style="background:#0b0f19; font-family:-apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, Helvetica, Arial, sans-serif;">
  <defs>
    <!-- Gradients -->
    <linearGradient id="headerGrad" x1="0%" y1="0%" x2="100%" y2="0%">
      <stop offset="0%" stop-color="#0284c7" />
      <stop offset="50%" stop-color="#6366f1" />
      <stop offset="100%" stop-color="#a855f7" />
    </linearGradient>
    <linearGradient id="clientGrad" x1="0%" y1="0%" x2="0%" y2="100%">
      <stop offset="0%" stop-color="#1e293b" />
      <stop offset="100%" stop-color="#0f172a" />
    </linearGradient>
    <linearGradient id="engineGrad" x1="0%" y1="0%" x2="0%" y2="100%">
      <stop offset="0%" stop-color="#1e1b4b" />
      <stop offset="100%" stop-color="#0f172a" />
    </linearGradient>
    <linearGradient id="diskGrad" x1="0%" y1="0%" x2="0%" y2="100%">
      <stop offset="0%" stop-color="#064e3b" />
      <stop offset="100%" stop-color="#022c22" />
    </linearGradient>
    <linearGradient id="lockGrad" x1="0%" y1="0%" x2="0%" y2="100%">
      <stop offset="0%" stop-color="#701a75" />
      <stop offset="100%" stop-color="#4a044e" />
    </linearGradient>
    <linearGradient id="cardGrad" x1="0%" y1="0%" x2="0%" y2="100%">
      <stop offset="0%" stop-color="#1e293b" />
      <stop offset="100%" stop-color="#111827" />
    </linearGradient>

    <!-- Drop Shadow Filter -->
    <filter id="shadow" x="-5%" y="-5%" width="110%" height="110%">
      <feDropShadow dx="0" dy="4" stdDeviation="6" flood-color="#000000" flood-opacity="0.6"/>
    </filter>

    <!-- Arrow Markers -->
    <marker id="arrow" viewBox="0 0 10 10" refX="8" refY="5" markerWidth="6" markerHeight="6" orient="auto-start-reverse">
      <path d="M 0 1 L 10 5 L 0 9 z" fill="#38bdf8" />
    </marker>
    <marker id="arrowGreen" viewBox="0 0 10 10" refX="8" refY="5" markerWidth="6" markerHeight="6" orient="auto-start-reverse">
      <path d="M 0 1 L 10 5 L 0 9 z" fill="#34d399" />
    </marker>
    <marker id="arrowAmber" viewBox="0 0 10 10" refX="8" refY="5" markerWidth="6" markerHeight="6" orient="auto-start-reverse">
      <path d="M 0 1 L 10 5 L 0 9 z" fill="#fbbf24" />
    </marker>
    <marker id="arrowRose" viewBox="0 0 10 10" refX="8" refY="5" markerWidth="6" markerHeight="6" orient="auto-start-reverse">
      <path d="M 0 1 L 10 5 L 0 9 z" fill="#f43f5e" />
    </marker>
  </defs>

  <!-- Title & Header Bar -->
  <rect x="0" y="0" width="{width}" height="60" fill="#0f172a" />
  <rect x="0" y="58" width="{width}" height="2" fill="url(#headerGrad)" />
  <text x="40" y="38" fill="#f8fafc" font-size="22" font-weight="800" letter-spacing="0.5">KVDB CRASH-PROOF SYSTEM ARCHITECTURE</text>
  <text x="590" y="38" fill="#94a3b8" font-size="14" font-weight="500">Go Embedded Storage Engine &amp; Python 3 Torture Test Isolation</text>
  <rect x="{width}-340" y="15" width="300" height="30" rx="6" fill="#1e293b" stroke="#334155" stroke-width="1" />
  <text x="{width}-325" y="35" fill="#38bdf8" font-size="12" font-weight="700">INVARIANT: ACK ONLY AFTER FSYNC</text>

  <!-- ========================================================================= -->
  <!-- LEFT COLUMN: SYSTEM COMPONENT & CONCURRENCY ARCHITECTURE (Width: 980)      -->
  <!-- ========================================================================= -->

  <!-- Top Layer: Client & Test Harness -->
  <g transform="translate(40, 80)">
    <rect width="940" height="110" rx="10" fill="url(#clientGrad)" stroke="#3b82f6" stroke-width="1.5" filter="url(#shadow)" />
    <rect x="15" y="-10" width="220" height="22" rx="4" fill="#1d4ed8" />
    <text x="25" y="6" fill="#ffffff" font-size="11" font-weight="800" letter-spacing="1">CLIENT &amp; HARNESS ISOLATION</text>

    <!-- Torture Harness Card -->
    <rect x="25" y="25" width="280" height="65" rx="6" fill="#111827" stroke="#334155" />
    <text x="40" y="48" fill="#f1f5f9" font-size="14" font-weight="700">Python Torture Harness</text>
    <text x="40" y="68" fill="#94a3b8" font-size="11">Independent Oracle | Random Kill Injector</text>
    <text x="40" y="80" fill="#38bdf8" font-size="10">Zero engine internals imported</text>

    <!-- CLI Driver Card -->
    <rect x="330" y="25" width="280" height="65" rx="6" fill="#111827" stroke="#334155" />
    <text x="345" y="48" fill="#f1f5f9" font-size="14" font-weight="700">Go CLI Driver (./kvdb)</text>
    <text x="345" y="68" fill="#94a3b8" font-size="11">put | get | del | batch | dump | check</text>
    <text x="345" y="80" fill="#34d399" font-size="10">8 Mandatory Hackathon Subcommands</text>

    <!-- IPC Serve Daemon Card -->
    <rect x="635" y="25" width="280" height="65" rx="6" fill="#111827" stroke="#334155" />
    <text x="650" y="48" fill="#f1f5f9" font-size="14" font-weight="700">Serve Daemon (IPC Mode)</text>
    <text x="650" y="68" fill="#94a3b8" font-size="11">Newline-delimited JSON on stdin/stdout</text>
    <text x="650" y="80" fill="#a855f7" font-size="10">Persistent long-lived process execution</text>
  </g>

  <!-- Middle Layer: Concurrency, Locking & In-Memory State -->
  <g transform="translate(40, 215)">
    <rect width="940" height="150" rx="10" fill="url(#cardGrad)" stroke="#475569" stroke-width="1.5" filter="url(#shadow)" />
    <rect x="15" y="-10" width="260" height="22" rx="4" fill="#334155" />
    <text x="25" y="6" fill="#ffffff" font-size="11" font-weight="800" letter-spacing="1">CONCURRENCY &amp; IN-MEMORY STATE</text>

    <!-- Sidecar Lock Card -->
    <rect x="25" y="25" width="280" height="105" rx="6" fill="#1e1b4b" stroke="#6366f1" />
    <text x="40" y="48" fill="#c7d2fe" font-size="13" font-weight="700">Sidecar flock (&lt;path&gt;.lock)</text>
    <text x="40" y="68" fill="#94a3b8" font-size="11">Permanent advisory lock inode</text>
    <text x="40" y="84" fill="#94a3b8" font-size="11">Held exclusively for engine lifetime</text>
    <text x="40" y="100" fill="#94a3b8" font-size="11">Acquired BEFORE open / recovery</text>
    <text x="40" y="116" fill="#818cf8" font-size="10" font-weight="600">Cross-Process Single Writer</text>

    <!-- In-Process Mutex Card -->
    <rect x="330" y="25" width="280" height="105" rx="6" fill="#1e1b4b" stroke="#6366f1" />
    <text x="345" y="48" fill="#c7d2fe" font-size="13" font-weight="700">In-Process sync.RWMutex</text>
    <text x="345" y="68" fill="#94a3b8" font-size="11">mu.Lock(): exclusive writer lock</text>
    <text x="345" y="84" fill="#94a3b8" font-size="11">mu.RLock(): concurrent readers</text>
    <text x="345" y="100" fill="#94a3b8" font-size="11">Held across Quota -&gt; Write -&gt; Sync</text>
    <text x="345" y="116" fill="#818cf8" font-size="10" font-weight="600">Zero In-Process Race Conditions</text>

    <!-- KeyDir Index Card -->
    <rect x="635" y="25" width="280" height="105" rx="6" fill="#064e3b" stroke="#10b981" />
    <text x="650" y="48" fill="#a7f3d0" font-size="13" font-weight="700">In-Memory KeyDir (Hash Index)</text>
    <text x="650" y="68" fill="#94a3b8" font-size="11">map[string]KeyDirEntry in RAM</text>
    <text x="650" y="84" fill="#94a3b8" font-size="11">Locators: Absolute Offset, ValLen, Seq</text>
    <text x="650" y="100" fill="#94a3b8" font-size="11">100% Derived: Reconstructed at Open</text>
    <text x="650" y="116" fill="#34d399" font-size="10" font-weight="600">O(1) Reads | Updated ONLY After Sync</text>
  </g>

  <!-- Lower Middle Layer: Core Persistence Engine Pipeline -->
  <g transform="translate(40, 390)">
    <rect width="940" height="175" rx="10" fill="url(#engineGrad)" stroke="#6366f1" stroke-width="1.5" filter="url(#shadow)" />
    <rect x="15" y="-10" width="250" height="22" rx="4" fill="#4338ca" />
    <text x="25" y="6" fill="#ffffff" font-size="11" font-weight="800" letter-spacing="1">CORE ENGINE WRITE &amp; COMPACT</text>

    <!-- Quota Guard & UABE Codec -->
    <rect x="25" y="25" width="210" height="130" rx="6" fill="#111827" stroke="#334155" />
    <text x="35" y="46" fill="#f8fafc" font-size="12" font-weight="700">Quota Guard &amp; Codec</text>
    <text x="35" y="65" fill="#94a3b8" font-size="10">valid_eof + len &lt;= max_bytes</text>
    <text x="35" y="80" fill="#94a3b8" font-size="10">Magic 0x55414245 ('UABE')</text>
    <text x="35" y="95" fill="#94a3b8" font-size="10">Header CRC32C over hdr</text>
    <text x="35" y="110" fill="#94a3b8" font-size="10">Payload CRC32C trailer</text>
    <text x="35" y="135" fill="#38bdf8" font-size="10" font-weight="600">Strict Memory Bounds</text>

    <!-- Log Writer & Sync Barrier -->
    <rect x="255" y="25" width="210" height="130" rx="6" fill="#111827" stroke="#334155" />
    <text x="265" y="46" fill="#f8fafc" font-size="12" font-weight="700">WriteAt &amp; fsync Barrier</text>
    <text x="265" y="65" fill="#94a3b8" font-size="10">pwrite loop at valid_eof</text>
    <text x="265" y="80" fill="#94a3b8" font-size="10">Position-independent I/O</text>
    <text x="265" y="95" fill="#94a3b8" font-size="10">os.File.Sync() hardware flush</text>
    <text x="265" y="110" fill="#94a3b8" font-size="10">Checked error propagation</text>
    <text x="265" y="135" fill="#34d399" font-size="10" font-weight="600">Physical Media Guarantee</text>

    <!-- Recovery Scanner -->
    <rect x="485" y="25" width="210" height="130" rx="6" fill="#111827" stroke="#334155" />
    <text x="495" y="46" fill="#f8fafc" font-size="12" font-weight="700">Recovery Scanner</text>
    <text x="495" y="65" fill="#94a3b8" font-size="10">Sequential scan from offset 32</text>
    <text x="495" y="80" fill="#94a3b8" font-size="10">64KB streaming BufReader</text>
    <text x="495" y="95" fill="#94a3b8" font-size="10">Validates Hdr &amp; Trailer CRC</text>
    <text x="495" y="110" fill="#94a3b8" font-size="10">Truncates torn tail &amp; fsync</text>
    <text x="495" y="135" fill="#fbbf24" font-size="10" font-weight="600">&lt;= 2.0s for 1M Keys</text>

    <!-- Copy-On-Write Compactor -->
    <rect x="715" y="25" width="200" height="130" rx="6" fill="#111827" stroke="#334155" />
    <text x="725" y="46" fill="#f8fafc" font-size="12" font-weight="700">Atomic Compactor</text>
    <text x="725" y="65" fill="#94a3b8" font-size="10">Copy live entries to temp</text>
    <text x="725" y="80" fill="#94a3b8" font-size="10">temp.Sync() flush</text>
    <text x="725" y="95" fill="#94a3b8" font-size="10">os.Rename atomic swap</text>
    <text x="725" y="110" fill="#94a3b8" font-size="10">dir.Sync() directory metadata</text>
    <text x="725" y="135" fill="#f43f5e" font-size="10" font-weight="600">Zero Write-Loss During Swap</text>
  </g>

  <!-- Bottom Layer: Physical Storage Media -->
  <g transform="translate(40, 590)">
    <rect width="940" height="110" rx="10" fill="url(#diskGrad)" stroke="#10b981" stroke-width="1.5" filter="url(#shadow)" />
    <rect x="15" y="-10" width="220" height="22" rx="4" fill="#047857" />
    <text x="25" y="6" fill="#ffffff" font-size="11" font-weight="800" letter-spacing="1">PHYSICAL STORAGE MEDIA</text>

    <!-- Authoritative DB File -->
    <rect x="25" y="25" width="380" height="65" rx="6" fill="#022c22" stroke="#059669" />
    <text x="40" y="48" fill="#ecfdf5" font-size="14" font-weight="700">Authoritative File (&lt;path&gt;)</text>
    <text x="40" y="68" fill="#a7f3d0" font-size="11">32B Fixed Header (KVDB2026) + Repeating UABE Atomic Frames</text>
    <text x="40" y="80" fill="#34d399" font-size="10">Single authoritative data inode | Zero multi-file race conditions</text>

    <!-- Compaction Temp File -->
    <rect x="430" y="25" width="250" height="65" rx="6" fill="#022c22" stroke="#059669" />
    <text x="445" y="48" fill="#ecfdf5" font-size="14" font-weight="700">Temp File (&lt;path&gt;.compact.tmp)</text>
    <text x="445" y="68" fill="#a7f3d0" font-size="11">Written &amp; fsynced before rename</text>
    <text x="445" y="80" fill="#fbbf24" font-size="10">Discarded safely on crash prior to rename</text>

    <!-- Directory Metadata -->
    <rect x="705" y="25" width="210" height="65" rx="6" fill="#022c22" stroke="#059669" />
    <text x="720" y="48" fill="#ecfdf5" font-size="14" font-weight="700">Parent Directory</text>
    <text x="720" y="68" fill="#a7f3d0" font-size="11">Directory descriptor opened</text>
    <text x="720" y="80" fill="#38bdf8" font-size="10">dir.Sync() on rename / create</text>
  </g>

  <!-- ========================================================================= -->
  <!-- RIGHT COLUMN: ON-DISK SPEC & PROTOCOLS (Width: 540, Offset X: 1020)       -->
  <!-- ========================================================================= -->

  <!-- Card 1: On-Disk Binary Format & Byte Offsets -->
  <g transform="translate(1020, 80)">
    <rect width="540" height="310" rx="10" fill="#111827" stroke="#334155" stroke-width="1.5" filter="url(#shadow)" />
    <rect x="15" y="-10" width="260" height="22" rx="4" fill="#0284c7" />
    <text x="25" y="6" fill="#ffffff" font-size="11" font-weight="800" letter-spacing="1">ON-DISK BINARY FORMAT SPEC</text>

    <!-- 32B File Header Box -->
    <rect x="20" y="25" width="500" height="80" rx="6" fill="#1e293b" stroke="#38bdf8" />
    <text x="35" y="45" fill="#38bdf8" font-size="12" font-weight="700">32-BYTE AUTHORITATIVE FILE HEADER (Offset 0..31)</text>
    <text x="35" y="65" fill="#f8fafc" font-size="11" font-family="monospace">[00..08] Magic: "KVDB2026" (0x4B56444232303236)</text>
    <text x="35" y="80" fill="#f8fafc" font-size="11" font-family="monospace">[08..12] Version: u32 LE (1)  |  [12..20] Gen: u64 LE</text>
    <text x="35" y="95" fill="#34d399" font-size="11" font-family="monospace">[20..28] Flags: u64 (0)       |  [28..32] Hdr CRC32C: u32 LE</text>

    <!-- UABE Frame Header Box -->
    <rect x="20" y="115" width="500" height="75" rx="6" fill="#1e293b" stroke="#818cf8" />
    <text x="35" y="135" fill="#818cf8" font-size="12" font-weight="700">UABE FRAME HEADER (28 Bytes per commit)</text>
    <text x="35" y="152" fill="#f8fafc" font-size="11" font-family="monospace">[00..04] Magic: 0x55414245    |  [04..08] Hdr CRC32C: u32 LE</text>
    <text x="35" y="167" fill="#f8fafc" font-size="11" font-family="monospace">[08..16] Payload Len: u64 LE  |  [16..24] Sequence No: u64 LE</text>
    <text x="35" y="182" fill="#34d399" font-size="11" font-family="monospace">[24..28] OpCount: u32 LE (1 to 10,000 atomic operations)</text>

    <!-- UABE Payload & Trailer Box -->
    <rect x="20" y="200" width="500" height="95" rx="6" fill="#1e293b" stroke="#34d399" />
    <text x="35" y="220" fill="#34d399" font-size="12" font-weight="700">PAYLOAD OPS &amp; 4-BYTE TRAILER CRC</text>
    <text x="35" y="238" fill="#f8fafc" font-size="11" font-family="monospace">Per Op: OpType:u8 (1=PUT, 2=DEL) + KeyLen:u16 + ValLen:u32</text>
    <text x="35" y="253" fill="#f8fafc" font-size="11" font-family="monospace">        + KeyBytes[KeyLen] + ValueBytes[ValLen]</text>
    <text x="35" y="270" fill="#fbbf24" font-size="11" font-family="monospace">Trailer [0..4]: Payload CRC32C (Castagnoli) covers payload body</text>
    <text x="35" y="285" fill="#a7f3d0" font-size="10">Dual CRC: Validates frame metadata independent of full payload read</text>
  </g>

  <!-- Card 2: Numbered Write Ordering Sequence -->
  <g transform="translate(1020, 410)">
    <rect width="540" height="290" rx="10" fill="#111827" stroke="#334155" stroke-width="1.5" filter="url(#shadow)" />
    <rect x="15" y="-10" width="280" height="22" rx="4" fill="#059669" />
    <text x="25" y="6" fill="#ffffff" font-size="11" font-weight="800" letter-spacing="1">NUMBERED WRITE &amp; COMMIT PROTOCOL</text>

    <!-- Steps List -->
    <g transform="translate(25, 25)" font-size="12">
      <!-- Step 1 -->
      <circle cx="12" cy="12" r="10" fill="#3b82f6" />
      <text x="12" y="16" fill="#fff" font-size="10" font-weight="800" text-anchor="middle">1</text>
      <text x="32" y="16" fill="#f8fafc" font-weight="700">Input Validation &amp; Memory Allocation</text>
      <text x="32" y="32" fill="#94a3b8" font-size="11">Check key &lt;= 1KB, val &lt;= 1MB, ops &lt;= 10,000</text>

      <!-- Step 2 -->
      <circle cx="12" cy="52" r="10" fill="#3b82f6" />
      <text x="12" y="56" fill="#fff" font-size="10" font-weight="800" text-anchor="middle">2</text>
      <text x="32" y="56" fill="#f8fafc" font-weight="700">Acquire sync.RWMutex.Lock()</text>
      <text x="32" y="72" fill="#94a3b8" font-size="11">Exclusive writer lock: blocks concurrent mutators</text>

      <!-- Step 3 -->
      <circle cx="12" cy="92" r="10" fill="#3b82f6" />
      <text x="12" y="96" fill="#fff" font-size="10" font-weight="800" text-anchor="middle">3</text>
      <text x="32" y="96" fill="#f8fafc" font-weight="700">Quota Check &amp; RAM Serialization</text>
      <text x="32" y="112" fill="#94a3b8" font-size="11">Ensure valid_eof + frame_len &lt;= max_bytes. Compute CRCs.</text>

      <!-- Step 4 -->
      <circle cx="12" cy="132" r="10" fill="#eab308" />
      <text x="12" y="136" fill="#fff" font-size="10" font-weight="800" text-anchor="middle">4</text>
      <text x="32" y="136" fill="#fbbf24" font-weight="700">pwrite / WriteAt(frame, valid_eof)</text>
      <text x="32" y="152" fill="#94a3b8" font-size="11">Full write loop at valid_eof. No shared seek offset.</text>

      <!-- Step 5 -->
      <circle cx="12" cy="172" r="10" fill="#ef4444" />
      <text x="12" y="176" fill="#fff" font-size="10" font-weight="800" text-anchor="middle">5</text>
      <text x="32" y="176" fill="#f87171" font-weight="700">HARDWARE BARRIER: os.File.Sync()</text>
      <text x="32" y="192" fill="#fca5a5" font-size="11">fsync commits frame to disk platters/NVM. Non-negotiable.</text>

      <!-- Step 6 -->
      <circle cx="12" cy="212" r="10" fill="#10b981" />
      <text x="12" y="216" fill="#fff" font-size="10" font-weight="800" text-anchor="middle">6</text>
      <text x="32" y="216" fill="#34d399" font-weight="700">Index Publication &amp; EOF Advance</text>
      <text x="32" y="232" fill="#94a3b8" font-size="11">Apply ops to KeyDir in RAM, valid_eof += len, release lock</text>

      <!-- Step 7 -->
      <circle cx="12" cy="247" r="10" fill="#10b981" />
      <text x="12" y="251" fill="#fff" font-size="10" font-weight="800" text-anchor="middle">7</text>
      <text x="32" y="251" fill="#34d399" font-weight="700">Emit "ACK &lt;op-id&gt;" to Caller</text>
    </g>
  </g>

  <!-- ========================================================================= -->
  <!-- BOTTOM PANEL: CRASH RECOVERY FLOWCHART & GUARANTEES (Y: 720, Height: 400) -->
  <!-- ========================================================================= -->
  <g transform="translate(40, 725)">
    <rect width="1520" height="385" rx="10" fill="#0f172a" stroke="#334155" stroke-width="1.5" filter="url(#shadow)" />
    <rect x="15" y="-10" width="300" height="22" rx="4" fill="#6d28d9" />
    <text x="25" y="6" fill="#ffffff" font-size="11" font-weight="800" letter-spacing="1">CRASH RECOVERY PIPELINE &amp; GUARANTEES</text>

    <!-- Pipeline Step 1 -->
    <g transform="translate(30, 30)">
      <rect width="210" height="150" rx="8" fill="#1e293b" stroke="#818cf8" stroke-width="1.2" />
      <rect x="10" y="10" width="28" height="20" rx="4" fill="#4f46e5" />
      <text x="24" y="24" fill="#fff" font-size="11" font-weight="800" text-anchor="middle">R1</text>
      <text x="45" y="25" fill="#f8fafc" font-size="12" font-weight="700">Advisory Lock</text>
      <text x="15" y="55" fill="#cbd5e1" font-size="11">Acquire flock on &lt;path&gt;.lock</text>
      <text x="15" y="75" fill="#94a3b8" font-size="10">Guarantees exclusive ownership across process death.</text>
      <text x="15" y="115" fill="#34d399" font-size="10" font-weight="600">Zero Multi-Process Races</text>
    </g>
    <path d="M 245 105 L 280 105" stroke="#38bdf8" stroke-width="2" marker-end="url(#arrow)" />

    <!-- Pipeline Step 2 -->
    <g transform="translate(285, 30)">
      <rect width="220" height="150" rx="8" fill="#1e293b" stroke="#818cf8" stroke-width="1.2" />
      <rect x="10" y="10" width="28" height="20" rx="4" fill="#4f46e5" />
      <text x="24" y="24" fill="#fff" font-size="11" font-weight="800" text-anchor="middle">R2</text>
      <text x="45" y="25" fill="#f8fafc" font-size="12" font-weight="700">Validate 32B Header</text>
      <text x="15" y="55" fill="#cbd5e1" font-size="11">Check Magic 'KVDB2026'</text>
      <text x="15" y="75" fill="#cbd5e1" font-size="11">Verify Header CRC32C</text>
      <text x="15" y="95" fill="#94a3b8" font-size="10">If file absent, init header &amp; sync</text>
      <text x="15" y="125" fill="#fbbf24" font-size="10" font-weight="600">Rejects Foreign/Corrupted Files</text>
    </g>
    <path d="M 510 105 L 545 105" stroke="#38bdf8" stroke-width="2" marker-end="url(#arrow)" />

    <!-- Pipeline Step 3 -->
    <g transform="translate(550, 30)">
      <rect width="250" height="150" rx="8" fill="#1e293b" stroke="#818cf8" stroke-width="1.2" />
      <rect x="10" y="10" width="28" height="20" rx="4" fill="#4f46e5" />
      <text x="24" y="24" fill="#fff" font-size="11" font-weight="800" text-anchor="middle">R3</text>
      <text x="45" y="25" fill="#f8fafc" font-size="12" font-weight="700">Sequential Frame Scan</text>
      <text x="15" y="55" fill="#cbd5e1" font-size="11">64KB BufReader from offset 32</text>
      <text x="15" y="75" fill="#cbd5e1" font-size="11">Verify Frame Magic &amp; Hdr CRC</text>
      <text x="15" y="95" fill="#cbd5e1" font-size="11">Verify Trailer Payload CRC</text>
      <text x="15" y="125" fill="#38bdf8" font-size="10" font-weight="600">Replays Complete Batches Only</text>
    </g>
    <path d="M 805 105 L 840 105" stroke="#38bdf8" stroke-width="2" marker-end="url(#arrow)" />

    <!-- Pipeline Step 4 -->
    <g transform="translate(845, 30)">
      <rect width="230" height="150" rx="8" fill="#1e293b" stroke="#818cf8" stroke-width="1.2" />
      <rect x="10" y="10" width="28" height="20" rx="4" fill="#4f46e5" />
      <text x="24" y="24" fill="#fff" font-size="11" font-weight="800" text-anchor="middle">R4</text>
      <text x="45" y="25" fill="#f8fafc" font-size="12" font-weight="700">Torn Tail Truncation</text>
      <text x="15" y="55" fill="#cbd5e1" font-size="11">If scan hits torn/partial byte:</text>
      <text x="15" y="75" fill="#fbbf24" font-size="11">File.Truncate(valid_eof)</text>
      <text x="15" y="95" fill="#f87171" font-size="11">File.Sync() commits truncation</text>
      <text x="15" y="125" fill="#34d399" font-size="10" font-weight="600">Self-Healing Idempotent Repair</text>
    </g>
    <path d="M 1080 105 L 1115 105" stroke="#38bdf8" stroke-width="2" marker-end="url(#arrow)" />

    <!-- Pipeline Step 5 -->
    <g transform="translate(1120, 30)">
      <rect width="210" height="150" rx="8" fill="#1e293b" stroke="#818cf8" stroke-width="1.2" />
      <rect x="10" y="10" width="28" height="20" rx="4" fill="#4f46e5" />
      <text x="24" y="24" fill="#fff" font-size="11" font-weight="800" text-anchor="middle">R5</text>
      <text x="45" y="25" fill="#f8fafc" font-size="12" font-weight="700">Orphan Prune &amp; Ready</text>
      <text x="15" y="55" fill="#cbd5e1" font-size="11">Unlink orphan .compact.tmp</text>
      <text x="15" y="75" fill="#cbd5e1" font-size="11">Publish reconstructed KeyDir</text>
      <text x="15" y="95" fill="#cbd5e1" font-size="11">Set next_seq_no</text>
      <text x="15" y="125" fill="#34d399" font-size="10" font-weight="600">Engine Online (&lt;= 2.0s)</text>
    </g>

    <!-- Bottom Proof Summary Cards -->
    <g transform="translate(30, 205)">
      <!-- Box 1 -->
      <rect width="460" height="155" rx="6" fill="#111827" stroke="#1e293b" />
      <text x="20" y="30" fill="#38bdf8" font-size="13" font-weight="700">PROVABLE GUARANTEE 1: ZERO PHANTOM WRITES</text>
      <text x="20" y="55" fill="#94a3b8" font-size="11">Every ACK is preceded by a completed File.Sync() syscall barrier.</text>
      <text x="20" y="75" fill="#94a3b8" font-size="11">If kill -9 strikes before or during write, no ACK was issued.</text>
      <text x="20" y="95" fill="#94a3b8" font-size="11">If kill -9 strikes after File.Sync(), valid dual CRC certifies durability.</text>
      <text x="20" y="125" fill="#34d399" font-size="11" font-weight="600">&#10004; 100% Durable across all crash points (Scenarios A1-A7)</text>

      <!-- Box 2 -->
      <rect x="490" y="0" width="460" height="155" rx="6" fill="#111827" stroke="#1e293b" />
      <text x="510" y="30" fill="#818cf8" font-size="13" font-weight="700">PROVABLE GUARANTEE 2: ATOMIC MULTI-OP BATCHES</text>
      <text x="510" y="55" fill="#94a3b8" font-size="11">Up to 10,000 operations serialize into ONE Universal Atomic Frame (UABE).</text>
      <text x="510" y="75" fill="#94a3b8" font-size="11">A single 4-byte CRC32C trailer seals the entirety of all batch operations.</text>
      <text x="510" y="95" fill="#94a3b8" font-size="11">A crash midway invalidates the trailer CRC; ZERO operations take effect.</text>
      <text x="510" y="125" fill="#34d399" font-size="11" font-weight="600">&#10004; Strictly All-or-Nothing batch atomicity (Scenarios B1-B6)</text>

      <!-- Box 3 -->
      <rect x="980" y="0" width="475" height="155" rx="6" fill="#111827" stroke="#1e293b" />
      <text x="1000" y="30" fill="#f43f5e" font-size="13" font-weight="700">PROVABLE GUARANTEE 3: SAFE ATOMIC COMPACTION</text>
      <text x="1000" y="55" fill="#94a3b8" font-size="11">Compaction streams live keys to a same-directory temp file.</text>
      <text x="1000" y="75" fill="#94a3b8" font-size="11">Order: temp.Sync() -&gt; close -&gt; os.Rename() -&gt; parent_dir.Sync().</text>
      <text x="1000" y="95" fill="#94a3b8" font-size="11">POS-IX rename is atomic: directory entry points to old OR new file.</text>
      <text x="1000" y="125" fill="#34d399" font-size="11" font-weight="600">&#10004; Zero data loss, zero orphaned locks (Scenarios C3-C5)</text>
    </g>
  </g>
</svg>"""

    with open("/home/laik/workspace/srinathonn/diagrams/architecture.svg", "w", encoding="utf-8") as f:
        f.write(svg)
    print("Successfully generated diagrams/architecture.svg")

if __name__ == "__main__":
    generate_svg()
