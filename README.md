# my-child-at-school-cli

A command-line client for the [My Child At School](https://www.mychildatschool.com)
(MCAS) parent portal, ported from the
[ha-mychildatschool-mcas](https://github.com/robbrad/ha-mychildatschool-mcas)
Home Assistant integration. It supports a single pupil per login, matching
that reference integration's scope.

## Prerequisites

- Go 1.23 or later
- An MCAS parent account (email + password) with an active pupil

## Build

```sh
go build -o mcas .
```

Or run directly without producing a binary:

```sh
go run . <command> [flags]
```

## Configuring credentials for local testing

The CLI needs your MCAS email and password. There's no OAuth or API key -
MCAS only supports the same username/password login you'd use on the
website. Credentials are resolved in this order:

1. `--email` / `--password` flags (discouraged - they show up in shell
   history; useful for one-off debugging)
2. `MCAS_EMAIL` / `MCAS_PASSWORD` environment variables (handy for CI or
   scripted/agent use)
3. The OS keychain entry written by `mcas login`

For everyday interactive use, sign in once and let the CLI store your
password in the OS keychain (via [go-keyring](https://github.com/zalando/go-keyring)):

```sh
$ go run . login
Email: parent@example.com
Password:
Signed in as Alex Smith (Example High School). Credentials saved to the OS keychain.
```

Subsequent commands use the stored credentials automatically. To remove
them:

```sh
go run . logout
```

For local testing without touching the keychain, environment variables are
often more convenient:

```sh
export MCAS_EMAIL=parent@example.com
export MCAS_PASSWORD=your-password
go run . attendance
```

## Commands

| Command       | Description                                                          |
|---------------|-----------------------------------------------------------------------|
| `login`       | Sign in and store credentials in the OS keychain                      |
| `logout`      | Remove stored credentials from the OS keychain                        |
| `attendance`  | Registration marks for a day (`--date YYYY-MM-DD`, default: today)    |
| `behaviour`   | Behaviour points/events for the academic year, or a day with `--date`; `--limit N` keeps the newest N |
| `timetable`   | This week's timetable                                                 |
| `dinner`      | Dinner money credit balance                                           |
| `detentions`  | Recorded detentions                                                   |
| `reports`     | Published school reports                                              |
| `clubs`       | Clubs and trips the pupil is enrolled on                              |
| `messages`    | Every message, newest first (`--limit`, `--from`, `--unread`, `--since`) |
| `messages <message-id>` | A single message in full                                    |
| `messages attachment <message-id> <attachment-id>` | Download an attachment |

`behaviour` lists each event with its type, points, subject or class, teacher,
description and outcome. The year call only has type and points, so the rest
is fetched per event day (only for the events shown) and cached; a day that
can't be loaded or matched is left without those fields, with a warning on
stderr. Aggregate negative totals (`negative`, `all_time_negative`) are
absolute values; each event's `points` is signed. `--date` returns a JSON
array of events (`[]` when there are none).

Global flags (available on every command):

- `--output text|json` - output format (default `text`)
- `--force-refresh` - bypass the local cache and fetch fresh data from MCAS
- `--email` / `--password` - override stored/env credentials for one call

## Output formats

### Human-readable (default)

```sh
$ go run . attendance
2026-09-27: Present
Period 1: Present, Period 2: Present
```

### Machine-readable JSON

Pass `--output json` for a stable envelope intended for scripts and AI
agents to parse without per-command schema knowledge:

```sh
$ go run . attendance --output json
{
  "schema_version": "1",
  "command": "attendance",
  "generated_at": "2026-09-27T08:00:00Z",
  "data": {
    "day": "2026-09-27T00:00:00Z",
    "periods": [
      {
        "period_name": "Period 1",
        "mark_sign": "P",
        "mark_meaning": "Present",
        "mark_description": "",
        "subject_name": "Maths"
      }
    ]
  },
  "error": null
}
```

On failure, `data` is `null` and `error` is populated with a stable `type`
(`auth_error` or `api_error`) so a script can branch on it without
string-matching messages:

```sh
$ go run . attendance --output json
{
  "schema_version": "1",
  "command": "attendance",
  "generated_at": "2026-09-27T08:00:00Z",
  "data": null,
  "error": {
    "type": "auth_error",
    "message": "no MCAS credentials found - run 'mcas login', set MCAS_EMAIL/MCAS_PASSWORD, or pass --email/--password"
  }
}
```

The process exits non-zero on error regardless of `--output` mode.

## Caching

Data-fetching commands cache their result locally for a short TTL (15
minutes by default, configurable via `cache_ttl_minutes` in the config
file) and serve repeated calls from that cache instead of hitting MCAS on
every invocation - useful since a script or AI agent has no natural pacing
the way a human clicking through the website does. Pass `--force-refresh`
to bypass the cache and fetch live data.

The config file lives at `$XDG_CONFIG_HOME/my-child-at-school-cli/config.yaml`
(or `~/.config/my-child-at-school-cli/config.yaml`); the cache lives under
`$XDG_CACHE_HOME/my-child-at-school-cli` (or `~/.cache/my-child-at-school-cli`).

## Messages

`messages` addresses by message, not by thread: it lists every message from
every sender in one flat list, always newest first (ties on date go to the
higher message ID). `messages <message-id>` opens one message in full,
including any links found in the body and attachment metadata; `messages
attachment <message-id> <attachment-id>` downloads one attachment.

The list can be filtered and combined:

```sh
$ mcas messages --limit 3
$ mcas messages --from 508 --since 2026-09-01
$ mcas messages --unread
```

| Flag | Default | Meaning |
|---|---|---|
| `--limit N` | `0` (all) | Show at most N messages, applied after filtering |
| `--from <recipient-id>` | (none) | Only messages from that sender |
| `--unread` | `false` | Only unread messages |
| `--since YYYY-MM-DD` | (none) | Messages on or after this date, local time |

These list flags can't be combined with a message ID - open a single
message, then filter separately to browse a sender's other messages.

MCAS returns the whole inbox in a single call - there is no pagination to
page through, and reading messages never marks them read on the server
(that's a separate action this CLI never takes).

Downloaded attachments are saved under their own name (e.g.
`Lockdown Procedure Practice.pdf`) in the current directory, `0o600`
permissioned, the way a browser download would be. If a file with that name
already exists there, the next free name is used instead - `Lockdown
Procedure Practice (1).pdf`, `(2).pdf`, and so on - so nothing already on
disk is ever silently overwritten. Pass `-o`/`--out` to `messages attachment`
to save somewhere else: an existing directory saves the attachment inside it
with the same de-duplication, while any other path is treated as an exact
file name and is always (over)written.

## Known limitations

Ported from the reference integration, and inherited here:

- Single pupil per login - multi-pupil accounts aren't supported.
- No CAPTCHA or 2FA support - if your account ever requires either, login
  will fail with a generic error.
- `detentions`, `reports` and `clubs` return loosely-typed rows rather than
  a fixed schema, since the reference integration never destructures them
  beyond a count. See [issue #3](../../issues/3) for details.

## Development

```sh
go build ./...
go vet ./...
go test ./...
```
