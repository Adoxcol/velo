#!/bin/sh
# Installs veloctl, Velo's command-line downloader, on Linux or macOS:
#
#   curl -fsSL https://velo.adoxcol.com/install.sh | sh
#
# It downloads the latest release for this system from GitHub, checks its
# SHA-256 and puts veloctl in ~/.local/bin (or $VELO_INSTALL_DIR).
set -eu

repo="https://github.com/Adoxcol/velo/releases/latest/download"
dir="${VELO_INSTALL_DIR:-$HOME/.local/bin}"

say() { printf '%s\n' "$*"; }
fail() { say "veloctl install: $*" >&2; exit 1; }

case "$(uname -s)" in
  Linux) os=linux ;;
  Darwin) os=darwin ;;
  *) fail "unsupported system $(uname -s). On Windows, use the installer from https://velo.adoxcol.com or: scoop install veloctl" ;;
esac
case "$(uname -m)" in
  x86_64 | amd64) arch=amd64 ;;
  aarch64 | arm64) arch=arm64 ;;
  *) fail "unsupported processor $(uname -m)" ;;
esac

if command -v curl >/dev/null 2>&1; then
  get() { curl -fsSL "$1" -o "$2"; }
elif command -v wget >/dev/null 2>&1; then
  get() { wget -q "$1" -O "$2"; }
else
  fail "needs curl or wget"
fi
if command -v sha256sum >/dev/null 2>&1; then
  sha() { sha256sum "$1" | cut -d' ' -f1; }
elif command -v shasum >/dev/null 2>&1; then
  sha() { shasum -a 256 "$1" | cut -d' ' -f1; }
else
  fail "needs sha256sum or shasum to check the download"
fi

tmp="$(mktemp -d)"
trap 'rm -rf "$tmp"' EXIT INT TERM
name="veloctl-$os-$arch.tar.gz"

say "Downloading veloctl for $os/$arch…"
get "$repo/$name" "$tmp/$name" || fail "download failed"
get "$repo/SHA256SUMS-veloctl-$os.txt" "$tmp/sums" || fail "couldn't get the checksums"
want="$(awk -v n="$name" '$2 == n { print $1 }' "$tmp/sums")"
[ -n "$want" ] || fail "no checksum listed for $name"
[ "$(sha "$tmp/$name")" = "$want" ] || fail "checksum mismatch; the download was not installed"

tar -xzf "$tmp/$name" -C "$tmp" veloctl
mkdir -p "$dir"
mv "$tmp/veloctl" "$dir/veloctl"
chmod 755 "$dir/veloctl"
if [ "$os" = darwin ]; then
  xattr -d com.apple.quarantine "$dir/veloctl" 2>/dev/null || true
fi

say "Installed $("$dir/veloctl" version) to $dir/veloctl"
case ":$PATH:" in
  *":$dir:"*) say "Try it:  veloctl get https://example.com/file.zip" ;;
  *)
    say ""
    say "$dir is not on your PATH. Add this line to your shell profile (~/.bashrc, ~/.zshrc):"
    say "  export PATH=\"$dir:\$PATH\""
    ;;
esac
