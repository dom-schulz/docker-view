#!/usr/bin/env bash
# Builds dkrv + the DockerView.app wrapper and installs both locally.
# Usage: ./macos/build.sh   (works from any directory)
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT"

APP_NAME="DockerView"
BIN_NAME="dkrv"                       # name of the Go binary inside Contents/MacOS
BUILD_DIR="$ROOT/build"
APP="$BUILD_DIR/$APP_NAME.app"
INSTALL_DIR="$HOME/Applications"
INSTALLED_APP="$INSTALL_DIR/$APP_NAME.app"
LINK_DIR="$HOME/.local/bin"
LSREGISTER="/System/Library/Frameworks/CoreServices.framework/Frameworks/LaunchServices.framework/Support/lsregister"

info() { printf '==> %s\n' "$*"; }
warn() { printf 'warning: %s\n' "$*" >&2; }
die()  { printf 'error: %s\n' "$*" >&2; exit 1; }

[[ "$(uname -s)" == "Darwin" ]] || die "this script builds a macOS app and must run on macOS"
command -v go >/dev/null 2>&1    || die "'go' not found. Install it with: brew install go"
command -v swift >/dev/null 2>&1 || die "'swift' not found. Install the Xcode command-line tools with: xcode-select --install"

# 1) Go binary
VERSION="$(git describe --tags --always --dirty 2>/dev/null || echo dev)"
info "Building $BIN_NAME ($VERSION)"
mkdir -p "$BUILD_DIR"
go build -ldflags "-X main.version=$VERSION" -o "$BUILD_DIR/$BIN_NAME" ./cmd/dkrv

# 2) Swift wrapper
info "Building $APP_NAME wrapper (swift build -c release)"
swift build -c release --package-path macos
SWIFT_BIN_DIR="$(swift build -c release --package-path macos --show-bin-path)"

# 3) Assemble the bundle
info "Assembling $APP"
rm -rf "$APP"
mkdir -p "$APP/Contents/MacOS" "$APP/Contents/Resources"
cp "$SWIFT_BIN_DIR/$APP_NAME" "$APP/Contents/MacOS/$APP_NAME"
cp "$BUILD_DIR/$BIN_NAME" "$APP/Contents/MacOS/$BIN_NAME"
cp "$ROOT/macos/Info.plist" "$APP/Contents/Info.plist"

# 4) Icon: SVG -> PNG (Quick Look) -> iconset (sips) -> .icns (iconutil)
make_icon() {
  local svg="$ROOT/assets/dkrv.svg" tmp png iconset
  [[ -f "$svg" ]] || { warn "$svg not found"; return 1; }
  tmp="$(mktemp -d)"
  qlmanage -t -s 1024 -o "$tmp" "$svg" >/dev/null 2>&1 || { rm -rf "$tmp"; return 1; }
  png="$tmp/$(basename "$svg").png"
  [[ -f "$png" ]] || { rm -rf "$tmp"; return 1; }
  iconset="$tmp/dkrv.iconset"
  mkdir -p "$iconset"
  local s
  for s in 16 32 128 256 512; do
    sips -z "$s" "$s" "$png" --out "$iconset/icon_${s}x${s}.png" >/dev/null || { rm -rf "$tmp"; return 1; }
    sips -z $((s * 2)) $((s * 2)) "$png" --out "$iconset/icon_${s}x${s}@2x.png" >/dev/null || { rm -rf "$tmp"; return 1; }
  done
  iconutil -c icns "$iconset" -o "$APP/Contents/Resources/dkrv.icns" || { rm -rf "$tmp"; return 1; }
  rm -rf "$tmp"
}
info "Rendering icon"
if ! make_icon; then
  warn "could not render assets/dkrv.svg to an icon; continuing without one"
fi

# 5) Ad-hoc signature (required to run on Apple silicon; local use only)
info "Signing (ad-hoc)"
codesign --force --deep -s - "$APP"

# 6) Install into ~/Applications
info "Installing to $INSTALLED_APP"
mkdir -p "$INSTALL_DIR"
rm -rf "$INSTALLED_APP"
ditto "$APP" "$INSTALLED_APP"

# 7) CLI symlink
mkdir -p "$LINK_DIR"
ln -sf "$INSTALLED_APP/Contents/MacOS/$BIN_NAME" "$LINK_DIR/$BIN_NAME"

# 8) Make Spotlight / Launch Services notice the (new) app
touch "$INSTALLED_APP"
if [[ -x "$LSREGISTER" ]]; then
  "$LSREGISTER" -f "$INSTALLED_APP" || warn "lsregister failed; Spotlight may take a moment to pick up the app"
fi

echo
info "Done ($VERSION)"
echo "    App:  $INSTALLED_APP   (open via Spotlight: \"$APP_NAME\")"
echo "    CLI:  $LINK_DIR/$BIN_NAME -> $INSTALLED_APP/Contents/MacOS/$BIN_NAME"
case ":$PATH:" in
  *":$LINK_DIR:"*) ;;
  *) echo "    Note: $LINK_DIR is not on your PATH; add it to use '$BIN_NAME' from a shell." ;;
esac
