# Deliverable 5: Architecture Diagrams (`diagrams/`)

This directory contains the authoritative architecture diagrams for the **KVDB** crash-proof embedded database, fulfilling **Deliverable 5** of the *"Kill It and Prove It"* Hackathon.

---

## Files in this Directory

| File | Format | Description |
| :--- | :--- | :--- |
| [`architecture.mermaid`](file:///home/laik/workspace/srinathonn/diagrams/architecture.mermaid) | Mermaid Source | Complete 5-part Mermaid specifications (Component Map, Concurrency Boundaries, Write Ordering, Compaction, Recovery Pipeline, and On-Disk Binary Format). |
| [`architecture.svg`](file:///home/laik/workspace/srinathonn/diagrams/architecture.svg) | Standalone Vector Graphic | Pre-rendered, high-resolution SVG diagram styled with dark theme for 100% offline inspection during the 15-minute live demo. |
| [`generate_svg.py`](file:///home/laik/workspace/srinathonn/diagrams/generate_svg.py) | Python 3 Script | Deterministic generator script that compiles `architecture.svg` without external packages. |

---

## 15-Minute Live Demo Quick Reference

During the offline live judging session:
1. To view the architecture diagram:
   Open [`ARCHITECTURE.md`](file:///home/laik/workspace/srinathonn/ARCHITECTURE.md) or [`architecture.svg`](file:///home/laik/workspace/srinathonn/diagrams/architecture.svg).
2. To inspect the raw Mermaid diagrams and markdown proofs:
   Open [`ARCHITECTURE.md`](file:///home/laik/workspace/srinathonn/ARCHITECTURE.md).

