# Cache-22: The Stacks — Implementation Plan

Self-hostable client-server pair for PS2 ISO streaming + PCSX2 wrapper + LAN rooms.
Private dumps only. Server never hosts BIOS. Content stays user-owned.

## 0. MVP Phases

- P0: Library + launcher + presence (SMB/file path boot, no chunks)
- P1: Chunk server + sparse file + RAM/disk cache
- P2: Per-game hot-set optimizer + manifests
- P3: WireGuard + VXLAN LAN rooms

## ADRs

### ADR-001: No PCSX2 Fork
Decision: Wrap stock PCSX2, boot via ISO path / FUSE file. No engine patches.
Rationale: Keeps GPL boundary clean, tracks upstream nightlies, avoids netplay determinism hell.
Consequence: All streaming smarts live in client filesystem layer + emulated CDVD timing does the rest.

### ADR-002: HTTP Range Chunks, Not SMB/NFS for WAN
Decision: Serve 1MB LBA-aligned chunks over plain HTTP Range. SMB/NFS LAN-only for P0.
Rationale: CDN-friendly, firewall-safe, supports priority + cache control. SMB is chatty and leaks.
Consequence: Need client sparse-file assembler from day P1.

### ADR-003: Client Exposes Sparse File via FUSE / WinFSP
Decision: Linux FUSE, Windows WinFSP. Single sparse `.iso` that fills on demand.
Rationale: PCSX2 sees a normal file. No plugin API to maintain.
Consequence: Must handle random reads, FMV scans, dual-layer offsets.

### ADR-004: 1MB Chunk Size, LBA-Aligned
Decision: 1MB = 512 sectors @2048B. Key by `(redump_hash, lba_start)`.
Rationale: Matches DVD 16-sector reads, good HTTP balance. Preserves logical layout order.
Consequence: Manifests keyed by hash; scrubbed/repacked ISOs rejected.

### ADR-005: Two-Tier Cache — RAM LRU + SSD TinyLFU
Decision: RAM 256-512MB LRU for hot/in-flight. SSD 10GB TinyLFU/ARC for cold. Pin boot set.
Rationale: FMV scans poison pure LRU. TinyLFU gives scan resistance.
Consequence: Need prefetch of 2-8MB sequential neighbors.

### ADR-006: Per-Game Manifest with File->LBA Map
Decision: Parse ISO9660/UDF extents. Ship `manifest.json` per serial: chunk priority 0-3 + prefetch neighbors.
Rationale: Devs optimized for sequential LBA. Mirror LBAs, tag hot LBAs from CDVD logs.
Consequence: Profiler logs `SeekToSector+size` from PCSX2 runs to build heatmaps.

### ADR-007: Preserve CDVD Timing, Don't Outrun It
Decision: Let PCSX2 throttle to 4x DVD (~5.28MB/s) / 24x CD (~3.6MB/s), CAV/CLV + 30/100ms seeks. Client only ensures prefetch drains faster.
Rationale: Timing-sensitive games (SH2, Shadowman) break if starved or rushed.
Consequence: Sustained net need ~80-100Mbps per client headroom, plus jitter buffer. `Fast CDVD` flag per-game in DB.

### ADR-008: BIOS Strictly Client-Only
Decision: Server never stores/transfers BIOS. Client validates local dump at boot.
Rationale: Legal + technical. Reduces Sony IP exposure.
Consequence: Onboarding must guide user dump.

### ADR-009: Library + Presence API
Decision: Go server. REST `/games`, `/rooms`, WS `/presence`. Postgres for metadata. IGDB/ScreenScraper for art.
Rationale: Feishin pattern: register server, login, browse, see who is on.
Consequence: Heartbeat `user, game-id, state, room-id`. No game bytes in API.

### ADR-010: WG Transport + VXLAN L2 Overlay
Decision: WireGuard for transport, VXLAN per-room VNI for L2 broadcast. Server = coordinator + relay fallback, P2P when possible.
Rationale: WG is L3 only, drops PS2 LAN discovery broadcasts. VXLAN restores L2. Matches PCSX2 DEV9 TAP/PCAP Bridged.
Consequence: MTU 1380-1420. `PersistentKeepalive=25`. Relay-all first, then P2P hole-punch.

### ADR-011: BYO-Storage, No Central Game Hosting
Decision: PaaS hosts signaling/relay/manifests only. Game bytes stay on user NAS/S3 or E2E private vault.
Rationale: 200-400GB/user hosting + egress kills margin. Sharing dumps = distribution liability. Zero-knowledge can't dedup + optimize anyway.
Consequence: Opt-in telemetry only. No public links. Rooms share play, not files — each player uses own dump.

### ADR-012: Wails Client
Decision: Wails (Go backend + web frontend). Go side: WG, VXLAN/TAP, cache, PCSX2 launcher. React for library UI.
Rationale: Single Go codebase across server + client, native WG/netlink, lighter than Electron, clean multi-server support like Feishin.
Consequence: Maintain FUSE/WinFSP backends via Go.

### ADR-013: Go Server + Postgres
Decision: Go for chunk + coordination. Postgres + object files. Redis/NATS for presence pub/sub.
Rationale: Simple cross-compile, good `wireguard-go`, static binaries for self-host.
Consequence: Docker Compose single-command deploy.

### ADR-014: Open Core, Paid Relay
Decision: AGPL/GPL core free self-host. Paid: hosted relay, TURN-like fallback, hosted lobby, metrics.
Rationale: Coolify/Grafana model. Can't charge for emulator itself, charge for ops.
Consequence: Keep PCSX2-adjacent code open from day one.

## Data Model (minimal)

- Game{serial, redump_hash, title, size, manifest_url, flags{fast_cdvd, docs}}
- Chunk{hash, lba, prio, neighbors[]}
- User{id, name}
- Presence{user_id, game_id, state, room_id, updated_at}
- Room{id, game_id, vni, members[], wg_peers[]}

## Next Build Order

1. Server scan + `/games` + WS presence
2. Wails launcher boot PCSX2 from path
3. HTTP chunk + sparse FUSE file + RAM LRU
4. Profiler -> manifest -> SSD cache
5. WG+VXLAN rooms for LAN titles
