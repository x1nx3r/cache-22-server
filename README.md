# cache-22-server

Self-hosted game server for your own PS2 rips. It streams ISOs to Cache-22
clients over HTTP Range requests, serves cover art, holds your memory cards,
and runs an admin UI. It never hosts or transfers BIOS files — dump your own
like an adult.

No discs were harmed. Okay, one disc drive died during testing. It knew what
it signed up for.

## How it came to be

It started with a ripped collection and a stupid problem: a folder of 4GB
ISOs that every machine in the house needed a full copy of. USB sticks got
a workout for about a week before that got old. The PS2 itself solved this
in 2000 — the disc stays in one place, the console reads what it needs.
So the server is the disc shelf: it holds each game once and hands out
byte ranges on demand.

Then came deployment reality: no public IP, so the whole thing lives behind
a Cloudflare tunnel. Which is fine until you upload a 4.7GB ISO and
Cloudflare's edge 413s anything over 100MB with zero sympathy. That's why
uploads go up in 48MB chunks — the chunked endpoint doesn't care about your
reverse proxy's feelings, and as a bonus a dropped tunnel costs you one
chunk retry instead of the whole file.

## The theory

Nothing about this is clever, which is the point:

- **The library is just files.** ISOs sit in a directory. The database holds
  metadata (serial, title, hashes, sizes), never game bytes. If Postgres
  evaporates, you rescan the folder and you're back.
- **Manifests describe, files serve.** Each game gets a manifest v2: total
  size, 128KB block size, exact byte ranges, and boot ranges (the first
  ~64MB the emulator always wants). Clients fetch the manifest, then pull
  byte ranges with plain `Range` requests. No custom streaming protocol to
  debug at 2AM.
- **Stateless bytes.** File serving is auth + `http.ServeContent` semantics.
  Ten clients reading ten offsets is ten independent range requests. The
  server keeps no per-client streaming state.
- **Small things live in Postgres.** Users (bcrypt), sessions (DB-backed,
  30-day TTL), covers metadata, per-user memory cards. Game bytes never
  touch the database.
- **Cover art is someone else's problem.** IGDB via Twitch
  client-credentials, backfilled on scan and upload. Bring your own creds.

## In practice

```sh
cp .env.example .env   # IGDB creds, ADMIN_TOKEN, the works
make db-up             # postgres
make gen css           # templ codegen + tailwind (needs ./bin/tailwindcss, 41MB, hence gitignored)
make run
```

Or the lazy way: `make up` (compose: postgres + server). Yes, `make up`.
It deploys and it apologizes for nothing.

`ADMIN_TOKEN` is one-time setup only. It mints the first admin account and
then becomes a decorative string. Make real users in the admin console.

Uploads: the admin library page has a + tile. On a LAN it single-shots the
file; through a capped proxy it transparently runs init → 4 parallel 48MB
PUTs with per-chunk retry → complete. Same tile, same progress bar.

### Deploy (Coolify, no public IP and all)

The compose file is Coolify-ready: no published DB port, named volumes,
healthcheck on `/v1/health`. Two things Coolify will not do for you:

1. It assumes port 80. Our server listens on 8080, so attach your domain
   **with the container port** — `https://your.host:8080` — or enjoy a
   502 with a perfectly healthy container.
2. It won't invent secrets. Set `ENV=prod`, a real `ADMIN_TOKEN`, a real
   `POSTGRES_PASSWORD`, and your IGDB creds in the Coolify env UI.

## What's here

- `POST /v1/games/upload` — single-shot upload (LAN use)
- `POST /v1/games/upload/init` → `PUT .../chunk` → `POST .../complete` — chunked upload
- `GET /v1/files/:serial` — Range-served ISO bytes
- `GET/PUT/HEAD/DELETE /v1/saves/:serial/:slot` — per-user, per-game memory
  cards with server-side backup rotation
- `POST /v1/admin/covers/backfill` — IGDB cover art
- Admin UI: library, game detail, users, setup flow (templ + htmx)

## Roadmap

- [ ] **Public saves.** The `public` flag already sits on every save row.
  Remaining: list/browse/copy endpoints plus a moderation story (report +
  takedown, because public uploads are public uploads).
- [ ] **Resumable chunked uploads.** Chunks are order-agnostic already; what's
  missing is server-persisted session resume, so a dead browser can pick up
  where it died instead of restarting.
- [ ] **Rooms plumbing (P3).** The far-off multiplayer dream is WireGuard +
  VXLAN LAN rooms. Server side that means room membership,PSK distribution,
  and presence beyond the current heartbeat scaffold. Not started, not soon.
- [ ] **Metrics worth looking at.** Range throughput per game, cache
  efficiency (bytes served vs bytes stored), upload success rates. Currently
  flying blind past the container logs.
- [ ] **Read-only share links.** Time-boxed links for a single game to a
  friend's client, without a full account. Needs the auth model to learn
  about scoped tokens.

## Test

```sh
make test
```

Needs Postgres; tests spin up scratch databases and throw them away.
