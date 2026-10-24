---
name: dakiya-ops
description: Administer and recover a self-hosted Dakiya Mail instance with dakiyactl's operator commands - instance status and health checks, creating accounts, resetting passwords, granting or revoking platform admin, disabling 2FA, claiming a fresh instance, exporting or importing a workspace. Use when the task is about the instance itself (accounts, access, health, migration) rather than campaigns or contacts.
---

# Operating a Dakiya Mail instance with dakiyactl

The operator commands talk to the database directly, so they work when
signing in does not. They run wherever `PRIMARY_DB` is set, which in practice
means inside the backend container:

```bash
docker compose -p dakiya exec backend dakiyactl <command>
make cli ARGS="<command>"          # same thing on a compose install
```

## Start every investigation with status

```bash
docker compose -p dakiya exec backend dakiyactl status        # prose + exit 1 on errors
docker compose -p dakiya exec backend dakiyactl status --json # stable keys, always exits 0
make doctor                                                     # the same, wrapped
```

`--json` keys are a contract (only ever appended to); read `.summary.error`
for the verdict and `.next_steps` for the exact commands the situation calls
for. Prefer parsing that over reasoning from the prose.

## The commands

| Task | Command |
|---|---|
| Claim a fresh instance | `dakiyactl setup-link` (refuses once accounts exist) |
| Create an account | `dakiyactl user create --email you@example.com [--admin]` |
| List accounts / find admins | `dakiyactl user list [--admin]` |
| Reset a password | `dakiyactl user reset-password --email ...` (prints a link) |
| Grant platform admin | `dakiyactl user grant-admin --email ... --role super\|support\|ops\|analyst` |
| Revoke platform admin | `dakiyactl user revoke-admin --email ...` |
| Clear a lost authenticator | `dakiyactl user disable-2fa --email ...` |
| Hash for unattended bootstrap | `dakiyactl hash-password` |
| List workspaces | `dakiyactl org list` |
| Move a workspace out | `dakiyactl org export --org <id\|slug\|owner-email> --out file.zip` |
| Move a workspace in | `dakiyactl org import --org ... --file file.zip --dry-run` first |

## Mechanics that bite

- Prompts need a TTY; pipes need `-T`. `docker compose exec` allocates a TTY
  unless you pass `-T`, so:
  - prompting: `docker compose -p dakiya exec backend dakiyactl user create --email ...`
  - piping: `printf '%s' "$PW" | docker compose -p dakiya exec -T backend dakiyactl user create --email ... --password-stdin`
- Passwords are 8-128 characters, the dashboard's own rule.
- `--admin` opens the admin panel on `ADMIN_URL` (port 5174 by default), not
  the dashboard on `APP_URL`.
- Redis down: `setup-link` and link-minting `reset-password` fail; use
  `user reset-password --password-stdin` instead. Everything else degrades to
  a warning and keeps working.
- `org import` always with `--dry-run` first; the report is free and names
  every member whose rows would be reassigned to the owner.
- `org export --with-credentials` produces the most sensitive file the
  product has (every mailbox password and refresh token, sealed only by the
  passphrase you supply). Never write it anywhere world-readable, and never
  echo the passphrase.

## Interacting with the product itself

Campaigns, contacts, mailboxes, the inbox and settings go through the API
half of dakiyactl with an API key. That is the `dakiya-api` skill.
