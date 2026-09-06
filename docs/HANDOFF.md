# Cache-22 — Context Handoff

Date: 2026-09-06. Two public repos under `x1nx3r`. Go 1.25 server, Go client
(CLI + Wails v3 GUI). Private PS2 dumps only; server never hosts BIOS.

## Repos

- Server: `github.com/x1nx3r/cache-22-server` — module
  `github.com/x1nx3r/cache-22-server` (renamed from `github.com/cache-22/...`;
  templ `_templ.go` files need a sed pass after `templ generate` — it
  preserves stale import paths).
- Client: `github.com/x1nx3r/cache-22-client` — modules
  `github.com/x1nx3r/cache-22-client` (root) and `.../client/gui`
  (gui `replace`s parent via `../`; after renames run `go mod tidy` in gui).
- Local paths: `/home/x1nx3r/mgodonf/cache-22-server`,
  `/home/x1nx3r/mgodonf/cache-22-client`. Dev box runs Arch, has display,
  `~/go/bin` (wails3, templ), Chromium (headless verification), live
  client data at `~/.local/share/cache22`.

## Deployed state

- Server live at `https://play.x1nx3r.dev` via Coolify + cloudflared tunnel
  (no public IP). Compose file is `docker-compose.yaml` (Coolify wants
  `.yaml`). Domain MUST include container port (`...:8080`); Coolify assumes
  80 → 502 otherwise.
- Cloudflare edge caps uploads at 100MB → game uploads are chunked
  (init → 4× parallel 48MB PUTs with retry → complete). Single-shot upload
  endpoint kept for LAN.
- Postgres internal only (no published port), named volumes
  (`pgdata`, `librarydata`), server healthcheck on `/v1/health` (needs
  `curl` in image — added to Dockerfile).
- Admin: first user `root` (local compose). `ADMIN_TOKEN` is one-time setup.
- Secrets needing rotation: Twitch/IGDB secret (leaked into old container
  log), compose `ADMIN_TOKEN` default on any real deploy.

## Architecture (as built)

- Server: REST (`/v1/games`, `/v1/files/:serial` Range-served,
  `/v1/saves/:serial/:slot` GET/PUT/HEAD/DELETE per-user,
  chunked upload endpoints, IGDB cover backfill), bcrypt + DB sessions
  (30d TTL), templ/htmx admin UI (Tailwind, goth-brutalist), Postgres DDL in
  `internal/infra/db/db.go` (games, users, sessions, covers, user_saves).
- Client: sparse ISO store + 128KB block bitmap, FUSE (go-fuse v2)
  exact-range fetch + singleflight + sequential prefetch, adaptive tiers
  from link probe (128KB×8 → 2MB×64; slow-link preload head capped 256MB —
  was size/4, which blocked launch for 1GB with no UI), stock PCSX2
  AppImage sidecar + private datapath, process-tracked launch (EmuStatus,
  StopEmulator), staged Play with live progress, per-game memory cards with
  3-way cloud sync (pull pre-launch, push post-exit, LWW+backup, offline
  tolerant), keyboard + raw-gamepad (`/dev/input/js*`, pure Go) controller
  mapping into PCSX2 ini format, typed toast queue.
- GUI: React + Material Web v2.5 via `@lit/react`, Material Symbols
  (self-hosted woff2), Catppuccin Macchiato tokens in `src/style.css`
  (bundled via import, NOT public/). Icon-only 64px rail. Views: Library,
  Servers, Settings (Storage/Emulator/Servers/BIOS/Network/Controller/Saves
  cards + tabs).
- Gotchas: MWC v2.5 `headline` attribute is dead — use
  `<div slot="headline">`; `md-list-item` shadow internals don't center
  reliably in WebKitGTK (rail nav uses `md-icon-button` instead); raw SDL
  tokens (`SDL-0/JoyButton<N>`) verified against PCSX2's `ParseKeyString`;
  semantic gamecontroller names need SDL's mapping DB (don't emit them);
  gamepad capture needs `input` group.

## Verification habits that work here

- Headless Chromium + scratch vite (`--port 17891`) + CDP screenshots;
  helpers live in `/tmp/opencode` (wiped between turns — recreate).
- Never `pkill -f` with a pattern present in your own command line (kills
  your shell); kill by PID from `ss -tlnp`.
- Never probe-mount over `~/.local/share/cache22/mnt/*` (live app dirs);
  use `/tmp`.
- Manual `vite build` and `wails3 build` race on `dist/` — run sequentially.
- User runs `make dev` on this same box; don't stomp their :9245 processes.

## Open threads (not started / queued)

- Save publish UI (server `public` flag exists; need list/copy endpoints +
  client browse); save management exists client-side only.
- Live input-test flash mode (keyboard easy, gamepad needs poll loop).
- Resumable chunked uploads (protocol is order-agnostic; needs persisted
  sessions).
- Per-group pad reset; playtime history; learned preloads from
  `profile.json` heatmaps.
- Mac/Windows sparse-boot clients; P3 WireGuard/VXLAN rooms (far off).
- Server metrics (range throughput, upload success rates).

## Don't commit

`.env`, `*/data/`, ISOs, `bin/`, `node_modules/`, `frontend/dist`,
AppImages. `frontend/bindings/` IS tracked. `static/app.css` and
`*_templ.go` ARE tracked (Docker needs them).
