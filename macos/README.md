# DockerView.app

A tiny native macOS wrapper around the `dkrv` TUI: one window containing a
[SwiftTerm](https://github.com/migueldeicaza/SwiftTerm) terminal that runs the
bundled `dkrv` binary, themed to match Ghostty (JetBrains Mono 13, light/dark
palettes following the system appearance). Closing the window or quitting
`dkrv` quits the app.

## Build & install

Requires Go (`brew install go`) and the Xcode command-line tools
(`xcode-select --install`). No Xcode project needed.

```sh
./macos/build.sh
```

This builds `dkrv` and the wrapper, assembles `build/DockerView.app`, installs
it to `~/Applications/DockerView.app` (findable via Spotlight), and symlinks
`~/.local/bin/dkrv` to the bundled binary.

## Update

```sh
git pull && ./macos/build.sh
```

## Signing / Gatekeeper

Nothing is distributed prebuilt: the app is built on your own machine and
ad-hoc signed locally, so there is no notarization and no Gatekeeper prompt.
