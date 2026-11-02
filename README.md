# juggler-sandbox

Run Juggler sessions on macOS, each in its own [nono](https://nono.sh) sandbox.

- One sandbox per project session.
- One login per provider (Claude, Codex, GitHub) shared by all sessions.
- Sessions hold phantom tokens only. nono keeps the real ones and swaps them in on allowed routes.

## How it works

1. `juggler-sandbox start` runs the Juggler machine server on the host, outside nono.
2. The machine server starts each session through `juggler-sandbox`, which runs it inside nono.
3. Every session shares one identity in `<data>/identity/`.

| Path | Default | Override |
|---|---|---|
| `<data>` | `~/.local/share/juggler-sandbox` | `$JUGGLER_SANDBOX_DATA_DIR` |
| `<state>` | `~/.local/state/juggler-sandbox` | `$JUGGLER_SANDBOX_STATE_DIR` |

`<data>` holds logins and settings. `<state>` holds the machine server state (`machine/`)
and one sandbox per session (`sessions/<id>/`). The program recreates `<state>` as needed.

## Requirements

- macOS, Go 1.27+ (to build)
- nono 0.81.0 (checked on every run)
- Juggler (`juggler` server, a release after 0.7.9) and Node 22+
- Claude Code, Codex CLI, Git, and GitHub CLI (`gh`)

`start` records the path of each tool in `<state>/machine/tools.json`.
Run `start` again after you upgrade a tool.

## Install

```sh
go install github.com/justenwalker/juggler-sandbox/cmd/juggler-sandbox@latest
```

Keep the binary and any clone outside every project you open in Juggler.

## Use

Log in once per provider:

```sh
juggler-sandbox login claude
juggler-sandbox login codex
juggler-sandbox login github   # paste a dedicated fine-grained token
```

Open the printed URL in your host browser. Running sessions restart to pick up new credentials.

Start the machine server:

```sh
juggler-sandbox start            # then open the printed URL and pick a project folder
juggler-sandbox start ~/src/app  # also open this folder
```

| Flag | Meaning |
|---|---|
| `--port` | UI port (default 3939) |
| `--allow-host NAME` | Also accept requests for `NAME` (a reverse proxy name). Repeat for more. |
| `--extends PROFILE` | Every session also inherits this nono profile. Repeat for more. See below. |

Example with Tailscale:

```sh
tailscale serve --bg 3939
juggler-sandbox start --allow-host mac.tailnet-name.ts.net
```

Callers through an allowed host count as remote and need the session token.
Pass `--allow-host` on every `start`.

Press Ctrl-C to stop the machine server and all sessions.
A second `start` attaches to the running machine server.

To use a development build of Juggler for one run:

```sh
JUGGLER_SANDBOX_JUGGLER=~/src/juggler/bin/juggler juggler-sandbox start
```

### Extra permissions

The session profile inherits nono's `default` profile. To grant more, name other nono
profiles (built-in, package, or your own in `~/.config/nono/profiles`):

```sh
juggler-sandbox start --extends go-dev --extends my-tools
```

nono merges them left to right after `default`; the launcher's own rules come last.
Lists such as filesystem paths and allowed domains add up. nono cannot remove an inherited
path, so an extended profile can only widen the sandbox. That includes the network: for
example, `go-dev` sets `network_profile: developer`, which the session inherits. Name only
profiles you trust.
Use a profile name, not a file path. Run `nono profile show NAME` first to see what it grants.
Like `--allow-host`, the flag applies only when `start` launches the machine server.

### GitHub token

Create a dedicated fine-grained token. Select only the repositories you open here, with
contents and pull-request write access. All sessions share it, but nono sends it only on the
routes of each session's own repository (the project's `origin` remote).
A project without a GitHub `origin` gets Claude and Codex only.

## Security model

A session can write:

- its project folder
- its own `sessions/<id>/data/`
- the shared `identity/claude`, `identity/codex`, and `identity/juggler`

nono denies everything else that matters: other sessions, `identity/control/`,
`<state>/machine/`, and the host's Claude, Codex, Juggler, GitHub, SSH, cloud, and Docker
state. Keychain stays denied. By default, outbound HTTP is limited to provider and GitHub
hosts. Profiles you add with `--extends` can widen this.

The machine server's control token stays outside the sandbox. Sessions get a proxy that
allows read-only status calls (server, LAN, tunnel, launch). Change LAN access and tunnels
from the machine server's own page or terminal.

`juggler-sandbox` also refuses a project that is `/`, your home folder, a link, or inside
`<data>` or `<state>`.

**Limits:**

- A malicious command can use the brokered routes, or send readable data to allowed hosts.
  Use a dedicated token and server-side branch protection. Grant no admin permissions.
- Path rules cannot see which branch a push updates. Branch protection is the control.
- GraphQL cannot be scoped by path, so the token's own scope is the limit.
- Sessions can read each other's Claude and Codex history.
- The agent writes straight into your project folder. Keep backups. Rollback snapshots are off.
- Local Git config is writable, so an agent can turn Git hooks back on.
- Every session can read `/tmp`. Keep projects under your home folder.
- On macOS, nono cannot enforce a loopback-only inbound boundary. Use a VM if you need one.
- Package downloads, web research, Docker, Kubernetes, and deployment tools are outside this
  profile. Do not attach unsandboxed MCP services that do these for the agent.

## Token brokering

nono intercepts the OAuth token endpoints for Claude and Codex and hands out phantom tokens.
Refreshes go through nono, which rotates the phantoms in the shared identity.

Each nono loads phantoms at start. After a refresh, login, or new GitHub token, the other
sessions restart within seconds. A restart ends the turn in progress; the conversation stays.

nono keeps captured tokens for up to 90 days (4096 phantoms max), shared by all your nono
profiles. A failed capture fails closed.

## Troubleshooting

nono lists denials after each run. If a session needs a path, review it outside the sandbox
and add a grant in `BuildProfile`. Never grant the whole home directory for one file.

Debug logs for each run are in `<state>/sessions/<id>/control/logs/`.
Login logs are in `<data>/identity/login/control/logs/`.

## Development

```sh
go test ./...
golangci-lint run ./...
go test ./internal/sandbox -run TestBuildProfile -update   # rewrite the golden profile
```

Tests use temporary directories, a fake process runner, and a fake machine server.

## License

[MIT](LICENSE)
