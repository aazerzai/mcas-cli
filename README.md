# my-child-at-school-cli

A command-line client for the [My Child At School](https://www.mychildatschool.com)
(MCAS) parent portal, ported from the
[ha-mychildatschool-mcas](https://github.com/robbrad/ha-mychildatschool-mcas)
Home Assistant integration. It supports a single pupil per login, matching
that reference integration's scope.

## Disclaimer

This is an **unofficial, independent project**. It is **not affiliated with,
endorsed by or supported by** My Child At School or its owners.

- "My Child At School" and "MCAS" are trademarks of their respective owners
  and are used here only to describe compatibility.
- There is no official MCAS API. The CLI signs in to the parent portal with
  your own credentials and reads data the way a browser would. The portal may
  change or block access at any time, which may break this tool.
- Use at your own risk. You are responsible for complying with the portal's
  terms of service, including any rules on automated access.
- The software is provided "as is", without warranty of any kind, as set out
  in the [MIT licence](LICENSE).
- Your credentials are stored only in your OS keychain (or read from flags /
  environment variables you supply) and are never sent anywhere except the
  MCAS portal.

Please don't include real pupil data or credentials in issues, tests, fixtures
or pull requests. Redact or invent values instead.

## Attribution

Ported from [ha-mychildatschool-mcas](https://github.com/robbrad/ha-mychildatschool-mcas)
by Robert Bradley, which is released under the MIT licence (copyright (c) 2026
Robert Bradley). A copy of its licence is kept in
[`reference/ha-mychildatschool-mcas/LICENSE`](reference/ha-mychildatschool-mcas/LICENSE).

## Install

```sh
npm install -g @aazerzai/mcas-cli
# or, for one-off use:
npx @aazerzai/mcas-cli
```

Verify the install:

```sh
mcas --version
```

## Uninstall

First remove the credentials stored in your OS keychain by `mcas login`:

```sh
mcas logout
```

Then remove the package:

```sh
npm uninstall -g @aazerzai/mcas-cli
```

## Prerequisites (building from source)

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

## CLI Commands

| Command       | Description                                                          |
|---------------|-----------------------------------------------------------------------|
| `login`       | Sign in and store credentials in the OS keychain                      |
| `logout`      | Remove stored credentials from the OS keychain                        |
| `attendance`  | Registration marks for a day (`--date YYYY-MM-DD`, default: today)    |
| `behaviour`   | Behaviour points/events for the academic year, or a day with `--date` |
| `calendar`    | School academic calendar summary, or a day's type with `--date`       |
| `timetable`   | This week's timetable                                                 |
| `dinner`      | Dinner money credit balance                                           |
| `detentions`  | Recorded detentions                                                   |
| `reports`     | Published school reports                                              |
| `clubs`       | Clubs and trips the pupil is enrolled on                              |
| `messages`    | Every message, newest first (`--limit`, `--from`, `--unread`, `--since`) |
| `messages <message-id>` | A single message in full                                    |
| `messages <message-id> --attachments [<attachment-id>...]` | Download all (or the listed) attachments of a message |

`calendar` is sourced from the behaviour module's data (MCAS only exposes the
academic calendar there), so it reports an error if the school hasn't enabled
the Behaviour module. `behaviour --output json` no longer includes a
`calendar` key; use `calendar --output json` (a sorted `days` array) instead.

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
<message-id> --attachments` downloads all of its attachments, or only those
whose ids follow it (`messages <message-id> --attachments 481 482`).

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
disk is ever silently overwritten. Pass `-o`/`--out` (only valid with
`--attachments`) to save somewhere else. With one file, an existing directory
saves it inside with the same de-duplication, while any other path is treated
as an exact file name and is always (over)written. With more than one file,
`-o` must be a directory: a path that doesn't exist yet is created, and a
path to an existing file is an error.

All requested ids are checked against the message before anything is
downloaded, so a typo never leaves a partial result. If a download fails
part-way, the command stops with an `api_error` and files already saved are
kept. Text output prints one `Saved ... to ...` line per file; `--output
json` returns an array of `{message_id, attachment_id, file_name, path}`.

MCAS stores attachment names with stray whitespace (runs of spaces, spaces
before the extension) that the website hides. The CLI normalises names once
when reading them: whitespace runs collapse to a single space, the ends are
trimmed, and spaces before the final extension are removed
(`Year 10 Induction Evening    .pdf` becomes `Year 10 Induction Evening.pdf`).
Everything else in the name is left as is.

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

Contributing: never commit real pupil data or credentials in issues, tests or
fixtures - use made-up data only.

### Releasing

Pushing a tag of the form `vX.Y.Z` (for example `v1.2.3`) runs the release
workflow: GoReleaser builds binaries for darwin/linux/windows (amd64, arm64) and
attaches them to the GitHub Release, then the npm packages are published with the version taken from the tag
(without the `v`): one binary package per platform
(`@aazerzai/mcas-cli-<os>-<cpu>`, generated by `npm/scripts/build-platform-packages.js`)
and the `@aazerzai/mcas-cli` wrapper that depends on them. No install scripts are
used, so installs work under npm's default script blocking.
Tags that aren't `vX.Y.Z` fail and do not publish.

Maintainer setup:

- An npm account that owns the `@aazerzai` scope, with 2FA enabled
  (Authorization and Publishing).
- A repository secret `NPM_TOKEN`: a granular token with read/write on
  `@aazerzai` that can publish from CI without a one-time code. Note its expiry
  date and renew it before it lapses.
- First release: `git tag v0.1.0 && git push origin v0.1.0`.
- Optionally switch to npm trusted publishing afterwards and drop the token.

Dry run before the first tag: `goreleaser release --snapshot --clean`, then
`cd npm && node scripts/build-platform-packages.js 0.0.0-test && npm pack --dry-run`
(and `git checkout npm/package.json` afterwards).

## License

This project is released under the [MIT licence](LICENSE). It is a Go port of
[ha-mychildatschool-mcas](https://github.com/robbrad/ha-mychildatschool-mcas)
by Robert Bradley (also MIT), whose copyright notice is retained in
[`LICENSE`](LICENSE).
