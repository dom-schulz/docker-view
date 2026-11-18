# docker-view (`dkrv`) — execution plan

A Go TUI that works like lazydocker but groups containers by compose project the way Docker Desktop does. Arrow or number through every project on the machine, read merged logs, run compose actions, open a shell in a project's folder, and clean up stopped projects with one key. It runs on the Lenovo (NixOS, GNOME) and the Mac (Docker Desktop), and opens from GNOME Super search and macOS Spotlight.

- Repo: `dom-schulz/docker-view` (public)
- Command: `dkrv`
- Mac app: `DockerView.app`

The work is split into five phases. Each phase ends with something usable and a checklist that says when it's done. The design decisions the phases rely on are in [Reference](#reference) at the end.

## Scope

**In v1**

- Every compose project on the machine in one list, running or stopped, with a running/total count
- The selected project's services, with merged logs, per-service logs and stats
- Up, down, restart and stop for a project or a service; a shell into a service
- A shell in the project's folder (`t`) or a new Ghostty window there (`T`)
- Clean up: `down` every project with nothing running, after one confirmation
- Launchers with their own dock identity on both machines

**Not in v1**

- Images, volumes and networks panels (lazydocker stays installed for those)
- Remembering projects after `down` (they drop off the list; fine for a dev tool)
- Remote hosts, `docker context` switching, theme settings

## Prerequisites

- Go 1.23+ on both machines (`brew install go` on the Mac; the flake's dev shell on the Lenovo)
- Docker running: Docker Engine on the Lenovo, Docker Desktop on the Mac
- Xcode command-line tools on the Mac (phase 5 only)
- At least two compose projects running for testing (e.g. two worktrees of the work repo, or two small sample stacks)

## Phase 0 — Repo setup

Tasks

- [ ] Create `dom-schulz/docker-view` with the layout below, a README (with "Not affiliated with or endorsed by Docker, Inc."), an MIT license and this plan.
- [ ] `go mod init github.com/dom-schulz/docker-view`; add Bubble Tea, Bubbles, Lip Gloss, the Docker SDK and a TOML parser.
- [ ] `flake.nix` with a dev shell (go, gopls) and a `buildGoModule` package.
- [ ] CI: `go vet`, `go test ./...` and `nix build` on push.

Done when: `go run ./cmd/dkrv --version` prints a version, and CI is green.

```
docker-view/
  cmd/dkrv/main.go         entry point, flags (--version)
  internal/docker/         SDK client, listing, grouping, events, logs, stats
  internal/compose/        builds and runs docker compose commands
  internal/config/         reads ~/.config/dkrv/config.toml
  internal/ui/             Bubble Tea model, panes, keymap, styles
  macos/                   Swift wrapper app (SwiftTerm) + build.sh
  assets/dkrv.svg          icon
  flake.nix                Linux package + dev shell
  .github/workflows/ci.yml go vet, go test, nix build
```

## Phase 1 — Browse

Tasks

- [ ] Docker client from `DOCKER_HOST`, falling back to `/var/run/docker.sock`. A clear error screen if the daemon isn't reachable.
- [ ] `ContainerList(all: true)` and group containers into projects by the compose labels (see [Reference](#compose-labels)). Containers without a project label go in an `Other` group.
- [ ] Sort: running projects first, then by name. Show running/total per project.
- [ ] Layout: projects in the tall left column, the selected project's services under it, an empty right pane with Logs / Stats / Config tabs, a key-hint line at the bottom.
- [ ] Keys: `↑↓`/`jk`, `1`–`9`, `Tab`/`Shift+Tab`, `/` filter, `a` show/hide stopped, `?` help, `q` quit.
- [ ] Live updates: subscribe to the event stream filtered to container events; re-list on start, stop, create, destroy, debounced by about 200 ms. Reconnect with backoff if the stream drops, and re-list every 30 s as a safety net.
- [ ] Colors: only the terminal's 16 ANSI colors, no hex values.
- [ ] Unit tests for grouping and sorting, using fixture container lists.

Done when: starting or stopping a stack in another terminal updates the list within a second, and `1`–`9` jumps between projects.

## Phase 2 — Logs and stats

Tasks

- [ ] Selecting a project follows logs from all its services: tail the last ~500 lines, then follow. Each line is prefixed with the service name, colored per service.
- [ ] `stdcopy` to split stdout and stderr for containers without a TTY.
- [ ] Keep a capped buffer per project (e.g. 5,000 lines) and drop the oldest lines beyond it.
- [ ] `Enter` on a service narrows logs to that service; `Esc` goes back to all.
- [ ] Scrollback with `PgUp`/`PgDn`; `G` jumps back to following. Show a "following" indicator.
- [ ] Switching projects cancels the old log streams.
- [ ] Stats tab: CPU and memory per service, streamed only for the selected project.

Done when: logs from three services interleave in order, switching projects is instant, and leaving dkrv open for an hour doesn't grow memory without limit. At this point it should already beat lazydocker for reading logs across projects.

## Phase 3 — Actions

Tasks

- [ ] Build compose commands from labels, never `cd`: `docker compose -p <project> --project-directory <working_dir> -f <config_files…> <verb>`. Unit-test the command builder.
- [ ] `u` up -d, `d` down (asks first), `r` restart, `s` stop, for the selected project or service. Show the command's output or error in a status line.
- [ ] `e` runs `docker exec -it <container> sh` and returns to dkrv on exit (Bubble Tea `ExecProcess`).
- [ ] `t` project shell (see [Reference](#project-shell)): `$SHELL -l` in the working directory, with the banner, `DKRV_PROJECT`, and the window title set and restored.
- [ ] `T` opens a new Ghostty window in the working directory: `ghostty --working-directory=<dir>` on Linux, `open -na Ghostty --args --working-directory=<dir>` on the Mac.
- [ ] `C` clean up: list every project with nothing running, then `y` down, `v` down -v, `n` cancel (per `cleanup.volumes`).
- [ ] Config: read the optional `~/.config/dkrv/config.toml` at start; v1 has only `cleanup.volumes` (`ask` default, `keep`, `remove`).
- [ ] my-shell PR: a few lines in the zsh config that show `[$DKRV_PROJECT]` in the prompt when it's set.

Done when: every key in the [key table](#keys) works on both machines, `t` then `exit` lands back on the same project, and clean up removes only stopped projects.

## Phase 4 — Ship on the Lenovo

Tasks

- [ ] Flake package builds with `nix build`; tag `v0.1.0`.
- [ ] nixos-config PR: add `docker-view` as a flake input next to `my-shell`; add `home/dominicks/docker-view.nix` with the package and an `xdg.desktopEntries` entry (Name "Docker View", Keywords dkrv;docker;compose, Exec `ghostty --class=dev.dkrv -e dkrv`, `StartupWMClass=dev.dkrv`, Icon from `assets/dkrv.svg`); import it in `home.nix`.
- [ ] Delete `lazydocker.nix`'s desktop entry in the same PR (keep the lazydocker package).
- [ ] Rebuild with `sudo nixos-rebuild switch --flake .#lenovo`.

Done when: typing "docker", "compose" or "dkrv" in Super search opens dkrv in a Ghostty window with its own dock icon. Updates are `nix flake update docker-view` and a rebuild.

## Phase 5 — Mac app

Tasks

- [ ] `macos/`: a ~100-line Swift app using SwiftTerm whose terminal view runs the bundled `dkrv` binary. Set PATH to include `/opt/homebrew/bin` and `/usr/local/bin` so `docker` is found when launched from Spotlight.
- [ ] Give the app's terminal view the same theme and font as the Mac's Ghostty config, so dkrv and the `t` shell look the same as in Ghostty.
- [ ] Icon: `assets/dkrv.svg` converted to `.icns` at build time.
- [ ] `macos/build.sh`: build the Go binary, build the Swift app, put the binary inside `DockerView.app`, copy the app to `~/Applications`, and link `~/.local/bin/dkrv` to the binary.
- [ ] my-shell PR: delete `macos/make-lazydocker-app.sh`; remove the Lazydocker app from `~/Applications`.

Done when: Spotlight finds DockerView, it has its own Dock icon and Cmd+Tab entry, and `t`/`T` work. Updates are `git pull && ./macos/build.sh`. Nothing is downloaded prebuilt, so there's no signing, tap or Gatekeeper prompt. Until this phase is done, run `dkrv` from a terminal on the Mac.

## Reference

### Compose labels

| Label | Used for |
| --- | --- |
| `com.docker.compose.project` | the project name, the grouping key |
| `com.docker.compose.project.working_dir` | where compose actions run from |
| `com.docker.compose.project.config_files` | which compose files to pass with `-f` |
| `com.docker.compose.service` | the service name inside the project |

Each worktree is its own project with its own directory, so a command rebuilt from these labels always lands in the right one, and `.env` files there are picked up as if run from that folder. Flags such as `--profile` aren't recorded in labels, so they aren't reproduced.

### Down vs stop

`docker compose down` removes a project's containers, so the project drops off the list right away and its logs go with it. A stopped project keeps its containers, so it stays listed and its logs stay readable.

### Project shell

`t` hands the window to `$SHELL`, started as a login shell in the project's working directory, so PATH, prompt and aliases load as in a normal tab. `exit` brings dkrv back on the same project. While the shell is open:

- a banner prints on entry: `── api-feature-x · ~/projects/api-feature-x · type exit to return ──`
- `DKRV_PROJECT=api-feature-x` is set for that shell only; my-shell's zsh config shows it as `[api-feature-x]` in the prompt, so regular terminals are unchanged
- the window title is `api-feature-x — shell`, restored on return

### Layout

```
┌ Projects ─────────────────┐┌ Logs ─ Stats ─ Config ─────────────────────┐
│ 1 ● demo-webapp      2/2  ││ frontend  | VITE v6.4.6 ready in 128 ms    │
│ 2 ● api-feature-x    3/3  ││ backend   | listening on :8000             │
│ 3 ○ api-main         0/3  ││ frontend  | ➜ Local: http://localhost:5173 │
│ 4 ○ old-spike        0/1  ││ ...                                        │
│                           ││                                            │
├ Services: demo-webapp ────┤│                                            │
│ ● backend    8000->8000   ││                                            │
│ ● frontend   5173->5173   ││                                            │
└───────────────────────────┘└────────────────────────────────────────────┘
 1-9 jump · ↑↓ move · tab focus · u up · d down · C clean up · ? help
```

All keys are plain keys, so nothing clashes with keyd's Cmd/Option mappings on the Lenovo.

### Keys

| Key | Where | Does |
| --- | --- | --- |
| `↑` `↓` or `j` `k` | any list | move the selection |
| `1`–`9` | anywhere | jump to project 1–9 |
| `Tab` / `Shift+Tab` | anywhere | move focus: projects, services, right pane |
| `[` `]` | anywhere | switch tab: Logs, Stats, Config |
| `Enter` | services | logs for that service only (`Esc` goes back to all) |
| `/` | projects | filter projects by name |
| `a` | projects | show or hide stopped projects |
| `t` | project | shell in the project's folder, in this window; `exit` returns |
| `T` | project | new Ghostty window in the project's folder; dkrv stays open |
| `u` / `d` | project or service | compose up -d / down (down asks first) |
| `r` / `s` | project or service | restart / stop |
| `e` | services | shell into the service's container |
| `C` | anywhere | clean up every project with nothing running |
| `PgUp` `PgDn` / `G` | logs | scroll / jump back to following |
| `?` / `q` | anywhere | help / quit |

### Config

`~/.config/dkrv/config.toml` is optional and uses the same path on both machines, so it can live in my-shell.

| Setting | Default | Options |
| --- | --- | --- |
| `cleanup.volumes` | `ask` | `keep`, `remove`, `ask` |

With `ask`, `C` offers `y` (down), `v` (down -v, also deletes volumes) and `n` (cancel). `keep` and `remove` skip the choice.

### Launchers

| | Lenovo (GNOME) | Mac |
| --- | --- | --- |
| Binary on PATH | Nix package from the flake | `~/.local/bin/dkrv`, linked into `DockerView.app` |
| Search entry | `xdg.desktopEntries` in `home/dominicks/docker-view.nix` | `DockerView.app` in `~/Applications` |
| Launcher runs | `ghostty --class=dev.dkrv -e dkrv` | the app's terminal view runs the bundled `dkrv` |
| Found by typing | Docker View, dkrv, docker, compose | DockerView |
| Own dock identity | `StartupWMClass=dev.dkrv` | its own bundle ID, Dock icon, Cmd+Tab entry |
| Icon | `assets/dkrv.svg` | the same art as `.icns` |
