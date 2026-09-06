# cache-22-server

Self-hosted game server for your own PS2 rips. It streams ISOs to Cache-22
clients over HTTP Range requests, serves cover art, holds your memory cards,
and runs an admin UI. It never hosts or transfers BIOS files — dump your own
like an adult.

No discs were harmed. Okay, one disc drive died during testing. It knew what
it signed up for.

## Why this exists

Ripping a PS2 collection gives you a folder of 4GB ISOs and a new problem:
every machine that wants to play needs the whole file. Sneakernetting USB
sticks around the house gets old by the third time. This server holds the
library once, and clients stream only the blocks the emulator actually reads
— boot a 4GB game after fetching ~64MB.

Uploads go through in 48MB chunks, which exists for exactly one reason:
the author has no public IP and lives behind a Cloudflare tunnel, and
Cloudflare's edge 413s anything over 100MB. The chunked endpoint doesn't
care about your reverse proxy's feelings.

## Stack

Go 1.25, Postgres, templ + htmx admin UI (Tailwind), single binary in Docker.

## Run

```sh
cp .env.example .env   # IGDB creds, ADMIN_TOKEN, the works
make db-up             # postgres
make gen css           # templ codegen + tailwind (needs ./bin/tailwindcss, 41MB, hence gitignored)
make run
```

Or the lazy way: `make up` (compose: postgres + server).

`ADMIN_TOKEN` is one-time setup only. It mints the first admin account and
then becomes a decorative string. Make real users in the admin console.

## Deploy (Coolify, no public IP and all)

The compose file is Coolify-ready: no published DB port, named volumes,
healthcheck on `/v1/health`. In Coolify, attach your domain to the `server`
service **with the container port** — `https://your.host:8080` — because
Coolify only assumes port 80 and will 502 you otherwise. Ask me how I know.

## What's here

- `POST /v1/games/upload` — single-shot upload (LAN use)
- `POST /v1/games/upload/init` → `PUT .../chunk` → `POST .../complete` — chunked upload
- `GET /v1/files/:serial` — Range-served ISO bytes
- `GET/PUT/DELETE /v1/saves/:serial/:slot` — per-user, per-game memory cards with backups
- `POST /v1/admin/covers/backfill` — IGDB cover art (bring your own Twitch creds)

## Test

```sh
make test
```

Needs Postgres; tests spin up scratch databases and throw them away.
