# Running locally without Docker

`make dev` / `make up` require Docker, but Docker is only used for four infra
services. The application itself (Go backend/consumer/worker + React
dashboard/admin) runs natively. This machine runs those four natively instead.

## Infra (native)

| Service  | Port          | Provided by                              |
|----------|---------------|------------------------------------------|
| Postgres | 5432          | Homebrew `postgresql@16` (already installed) |
| Redis    | 6379          | Homebrew `redis` (already installed)     |
| NATS     | 4222 (+8222)  | Homebrew `nats-server` — **needs `-js`** |
| Mailpit  | 11025 / 18025 | Homebrew `mailpit`                       |

The compose file publishes Postgres on 15432 and Redis on 16379. The Makefile
now takes `PG_PORT` / `REDIS_PORT` overrides (defaults unchanged) so the native
services on their standard ports can be targeted.

### Start infra

```bash
brew services start postgresql@16
brew services start redis
nats-server -js -sd /tmp/natsdata -m 8222 -p 4222 &
mailpit --smtp 0.0.0.0:11025 --listen 0.0.0.0:18025 &
```

NOTE: JetStream (`-js`) is required. Without it the backend crashes at boot in
`eventbus.NATSBus.ensureStream`.

### One-time database setup

```bash
psql -d postgres -c "CREATE ROLE dakiya WITH LOGIN PASSWORD 'dakiya' SUPERUSER;"
psql -d dakiya_dev -c "CREATE DATABASE dakiya_dev OWNER dakiya TEMPLATE postgres;"
```

NOTE: `TEMPLATE postgres` is deliberate. On this machine both `template0` and
`template1` are missing seven system-catalog files, so the default
`CREATE DATABASE` fails with `could not open file "base/1/...": No such file or
directory`. The `postgres` database is intact and empty, so it works as the
template. See "Known issue: damaged Postgres templates" below.

## Run the app

Every target needs the port overrides:

```bash
make migrate PG_PORT=5432 REDIS_PORT=6379
SEED_RICH=true SEED_FULL=true make seed PG_PORT=5432 REDIS_PORT=6379
```

Then start each service in its own terminal (`make dev` can't be used — it
shells out to docker compose):

```bash
make backend  PG_PORT=5432 REDIS_PORT=6379   # :8080
make consumer PG_PORT=5432 REDIS_PORT=6379
make worker   PG_PORT=5432 REDIS_PORT=6379
make web                                      # :5173
make admin                                    # :5174
```

## Not running

`realtime` (Elixir) and `tracking` (Rust) normally run as containers. They are
not needed for the core dashboard and are skipped here. To run them natively,
install Elixir / cargo and use `make realtime` / `make tracking`.

## Logging in

Dashboard http://localhost:5173 — `dev@dakiyamail.com` / `password123`.
`AUTH_LOGIN_CODE=always`, so a 6-digit code is emailed; read it in Mailpit at
http://localhost:18025.


## Known issue: damaged Postgres templates

Both template databases on this machine are missing the same 7 system-catalog
files (including `pg_attribute`, relfilenode 1249):

```
base/1  (template1) : 292 files, 1249 + 2838 MISSING
base/4  (template0) : 292 files, 1249 + 2838 MISSING
base/5  (postgres)  : 299 files, intact
```

Every real database (`dakiya_dev`, `nutriflo_db`, `open-wearables`) is intact
and healthy — only the templates are affected. The practical symptom is that a
plain `CREATE DATABASE foo;` fails; `CREATE DATABASE foo TEMPLATE postgres;`
works.

To repair the templates (optional — needs the server briefly in a state where
nothing else is creating databases):

```bash
psql -d postgres -c "UPDATE pg_database SET datallowconn = true WHERE datname = 'template1';"
psql -d postgres -c "DROP DATABASE template1;"
psql -d postgres -c "CREATE DATABASE template1 TEMPLATE postgres IS_TEMPLATE true;"
```

`template0` is normally left alone (it is the pristine fallback); repairing it
the same way loses that guarantee, so prefer leaving it and passing an explicit
`TEMPLATE` when creating databases.
