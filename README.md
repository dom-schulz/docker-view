# docker-view (`dkrv`)

A terminal UI for Docker that groups containers by compose project, the way
Docker Desktop does. Arrow or number through every project on the machine, read
merged logs, run compose actions, open a shell in a project's folder, and clean
up stopped projects with one key.

Not affiliated with or endorsed by Docker, Inc.

```
┌ Projects ─────────────────┐┌ Logs ─ Stats ─ Config ─────────────────────┐
│ 1 ● demo-webapp      2/2  ││ frontend │ VITE v6.4.6 ready in 128 ms     │
│ 2 ● api-feature-x    3/3  ││ backend  │ listening on :8000              │
│ 3 ○ api-main         0/3  ││ frontend │ ➜ Local: http://localhost:5173  │
│ 4 ○ old-spike        0/1  ││ ...                                        │
├ Containers: demo-webapp ──┤│                                            │
│ ● backend    8000->8000   ││                                            │
│ ● frontend   5173->5173   ││                                            │
└───────────────────────────┘└────────────────────────────────────────────┘
 1-9 jump · ↑↓ move · tab focus · u up · d down · C clean up · ? help
```

## Install

**NixOS / Nix**: `nix run github:dom-schulz/docker-view`, or add the flake as
an input and use `packages.<system>.default`.

**macOS**: `./macos/build.sh` builds `dkrv` and `DockerView.app` (a small
SwiftTerm wrapper with its own Dock icon), installs the app to
`~/Applications` and links `~/.local/bin/dkrv`. Needs Go and the Xcode
command-line tools. Update with `git pull && ./macos/build.sh`. See
[macos/README.md](macos/README.md).

**Anywhere with Go 1.24+**: `go install github.com/dom-schulz/docker-view/cmd/dkrv@latest`.

dkrv talks to the daemon at `DOCKER_HOST`, falling back to
`/var/run/docker.sock` (and `~/.docker/run/docker.sock` for Docker Desktop).
Compose actions run `docker compose` (or `docker-compose` if the plugin is
missing).

## Keys

| Key | Where | Does |
| --- | --- | --- |
| `↑` `↓` or `j` `k` | any list | move the selection |
| `1`–`9` | anywhere | jump to project 1–9 |
| `Tab` / `Shift+Tab` | anywhere | move focus: projects, containers, right pane |
| `[` `]` | anywhere | switch tab: Logs, Stats, Config |
| `Enter` | containers | logs for that container only (`Esc` goes back to all) |
| `/` | projects | filter projects by name |
| `a` | projects | show or hide stopped projects |
| `t` | project | shell in the project's folder, in this window; `exit` returns |
| `T` | project | new Ghostty window in the project's folder; dkrv stays open |
| `u` / `d` | project or container | compose up -d / down (down asks first) |
| `r` / `s` | project or container | restart / stop |
| `e` | containers | shell into the container |
| `C` | anywhere | clean up every project with nothing running |
| `PgUp` `PgDn` / `G` | logs | scroll / jump back to following |
| `?` / `q` | anywhere | help / quit |

Actions apply to the selected container when the containers list has focus, and
to the whole project otherwise.

### How compose commands are built

dkrv never `cd`s. It rebuilds each command from the labels compose puts on
containers:

```
docker compose -p <project> --project-directory <working_dir> -f <config_files…> <verb>
```

so every worktree acts on its own project and picks up its own `.env`. Flags
such as `--profile` aren't recorded in labels, so they aren't reproduced.

`down` removes a project's containers, so it drops off the list along with
its logs. `stop` keeps them, so the project stays listed and its logs stay
readable.

### Project shell (`t`)

`t` hands the window to `$SHELL -l` in the project's working directory, prints
a banner, sets the window title to `<project> — shell`, and exports
`DKRV_PROJECT=<project>` for that shell only (a prompt can show it). `exit`
brings dkrv back on the same project.

## Config

`~/.config/dkrv/config.toml` is optional:

```toml
[cleanup]
volumes = "ask"   # ask (default): y = down, v = down -v, n = cancel
                  # keep: always down; remove: always down -v
```

## Development

```
nix develop           # go, gopls
go test ./...
go run ./cmd/dkrv
nix build             # the Linux package
```

The plan this was built from is in [PLAN.md](PLAN.md).

## License

MIT
