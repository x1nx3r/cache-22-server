# Cache-22 Server — Linux Phases

Scope: Linux server and Linux client only. Mac and Windows use sparse-file boot at this time.

Source plan: `./IMPLEMENTATION_PLAN.md`.

## S0: Scan and Game List

Goal: Scan a folder and show the game list in the client.

Work to do:
- Make a Go service as a single binary.
- Use Structured Query Language Lite (SQLite) for game data.
- Scan folders for ISO, CSO, ZSO, and Compressed Hunks of Data (CHD) files.
- Read the serial, size, and Redump hash for each file.
- Serve the game list through a Representational State Transfer (REST) interface.

Endpoints:
- `GET /v1/games` returns the game list.
- `GET /v1/games/:serial` returns game detail and the manifest link.
- `POST /v1/scan` starts a new scan.

Data:
- `Game{serial, redump_hash, title, size, flags}`.

Acceptance:
- Scan 50 discs in less than 2 minutes.
- Start the full stack with one Docker Compose command.
- Get the game list with `curl`.

Do not make artwork lookup, user login, or Postgres at this time.

## S1: Presence

Goal: Show who is online and what each user plays.

Work to do:
- Add a WebSocket (WS) hub at `/v1/presence`.
- Keep state in memory and record state changes in SQLite.
- Send a heartbeat each 15 seconds and remove stale entries after 45 seconds.

Message:
- `Presence{user_id, game_id, state, room_id, updated_at}`.

Acceptance:
- Connect two clients and see presence updates in less than 1 second.
- Reconnect a client and get full state again.

Do not add Redis, Network Attached Storage (NATS), or cross-server sync at this time.

## S2: Chunk Server and Manifest

Goal: Serve byte ranges over HyperText Transfer Protocol (HTTP) Range.

Work to do:
- Serve bytes from local disk or S3 storage. Do not transcode data.
- Support `Range` requests and return `206` with `Accept-Ranges` and `ETag`.
- Serve any byte range. There is no fixed chunk size.
- Ship one `manifest.json` file per game serial with boot ranges and priority.
- Protect chunk links with a bearer token that has a short life.

Endpoints:
- `GET /v1/files/:serial` supports `Range` reads.
- `GET /v1/games/:serial/manifest.json` returns the manifest.

Acceptance:
- Get a 1 MB range with `curl` and receive `206`.
- Reach more than 100 MB per second on a local area network.
- Reject a scrubbed image when the hash does not match.
- Boot one Dual Layer Digital Versatile Disc (DVD9) title from a pinned boot set.

Cache stays in the client. The server holds no game cache at this time.

## S3: Room Coordinator

Goal: Connect two Linux hosts in one Virtual Extensible Local Area Network (VXLAN) room.

Work to do:
- Assign one Virtual Network Identifier (VNI) per room.
- Use WireGuard for transport and VXLAN for broadcast traffic.
- Use the kernel WireGuard module and `ip link` for VXLAN.
- Set Maximum Transmission Unit (MTU) to 1380 and keepalive to 25 seconds.
- Relay all traffic through the server first. Add peer-to-peer later.

Endpoints:
- `POST /v1/rooms {game_id}` makes a room.
- `POST /v1/rooms/:id/join` joins a room.
- `GET /v1/rooms/:id/peers` returns WireGuard peers and the VNI.

Data:
- `Room{id, game_id, vni, members[], wg_peers[]}`.

Acceptance:
- Join two Linux hosts to one room.
- Send a broadcast ping through the VXLAN link.
- Show the peer in the PlayStation 2 DEV9 TAP bridge.

Do not make hole punch, hosted relay billing, or File System in Userspace (FUSE) changes in this phase.

## Build Order

Do the phases in this order:
1. S0 scan and game list.
2. S1 presence.
3. S2 chunk server and one hand-made manifest.
4. S2 client sparse boot without FUSE.
5. S3 room coordinator for Linux.

Move to Postgres, TinyLFU cache, profiler, and paid relay only after S2 boots a full game.
