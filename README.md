# Cache-22 Server

Self-hostable server for Cache-22: streams your private PS2 ISO dumps to
Cache-22 clients over HTTP Range requests. The server never hosts or
transfers BIOS files.

## Stack

Go 1.25, Postgres, templ + htmx admin UI (Tailwind), single-binary deploy.

## Run

```sh
cp .env.example .env   # fill in IGDB creds + ADMIN_TOKEN
make db-up             # postgres
make gen css           # templ codegen + tailwind (needs ./bin/tailwindcss)
make run
```

Or `make up` for docker compose (postgres + server).

`ADMIN_TOKEN` is one-time setup only: it creates the first admin account,
then does nothing. Create further users from the admin console.

## Test

```sh
make test
```

Tests need Postgres and create scratch databases.
