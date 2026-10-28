#!/usr/bin/env bash
# Speed benchmark: Velo against aria2 and curl on the same files.
#
#   tools/bench/bench.sh [runs] [url...]
#
# Builds veloctl from this checkout, downloads aria2 1.37.0 (official
# release) into tools/bench/.cache the first time, then downloads each URL
# with every tool, rotating the order each round so no tool always goes
# first. Prints one CSV row per download and the average per tool.
#
# The website's speed test (site/index.html, #compare) used the defaults:
# 3 runs of Velo's own installer from GitHub Releases. Be gentle with
# public test servers: many connections in a row can get you blocked.
set -euo pipefail
here="$(cd "$(dirname "$0")" && pwd)"
root="$(cd "$here/../.." && pwd)"
cache="$here/.cache"
runs="${1:-3}"; shift || true
urls=("$@")
[ ${#urls[@]} -gt 0 ] || urls=("https://github.com/Adoxcol/velo/releases/download/v0.8.26/Velo-Setup-0.8.26.exe")

mkdir -p "$cache"
exe=""; case "$(uname -s)" in MINGW*|MSYS*|CYGWIN*) exe=".exe";; esac
velo="$cache/veloctl$exe"
(cd "$root" && go build -o "$velo" ./cmd/veloctl)
aria2="$(command -v aria2c || true)"
if [ -z "$aria2" ] && [ -n "$exe" ]; then
  aria2="$cache/aria2-1.37.0-win-64bit-build1/aria2c.exe"
  if [ ! -x "$aria2" ]; then
    curl -sSL -o "$cache/aria2.zip" https://github.com/aria2/aria2/releases/download/release-1.37.0/aria2-1.37.0-win-64bit-build1.zip
    (cd "$cache" && unzip -qo aria2.zip)
  fi
fi
[ -n "$aria2" ] || { echo "aria2c not found; install it (apt/brew install aria2)" >&2; exit 1; }

run() { # tool url dir
  case $1 in
    velo)     "$velo" get "$2" "$3" >/dev/null 2>&1 ;;
    aria2-16) "$aria2" -q -x16 -s16 -k1M --file-allocation=none --disable-ipv6 -d "$3" "$2" ;;
    aria2-1)  "$aria2" -q -x1 -s1 --file-allocation=none --disable-ipv6 -d "$3" "$2" ;;
    curl)     (cd "$3" && curl -sSL -O "$2") ;;
  esac
}
tools=(velo aria2-16 aria2-1 curl)
out="$cache/out"; csv="$cache/results.csv"
echo "url,tool,run,seconds,bytes,MBps" | tee "$csv"
for r in $(seq 1 "$runs"); do
  for u in "${urls[@]}"; do
    for i in "${!tools[@]}"; do
      t=${tools[$(( (i + r) % ${#tools[@]} ))]}
      d="$out/$t"; rm -rf "$d"; mkdir -p "$d"
      s=$(date +%s.%N); run "$t" "$u" "$d" || true; e=$(date +%s.%N)
      b=$(find "$d" -type f ! -name '*.part*' ! -name '*.aria2' -printf '%s\n' | sort -n | tail -1)
      awk -v s="$s" -v e="$e" -v b="${b:-0}" -v u="$u" -v t="$t" -v r="$r" \
        'BEGIN{d=e-s; printf "%s,%s,%d,%.2f,%d,%.2f\n",u,t,r,d,b,b/d/1048576}' | tee -a "$csv"
    done
  done
done
rm -rf "$out"
echo
echo "Average of complete downloads (MB/s, seconds):"
awk -F, 'NR>1 && $5>0 {n[$2]++; m[$2]+=$6; t[$2]+=$4} NR>1 && $5==0 {f[$2]++}
  END {for (k in n) printf "  %-9s %6.2f MB/s %7.1f s%s\n", k, m[k]/n[k], t[k]/n[k], f[k] ? "  (" f[k] " failed)" : ""}' "$csv" | sort -k2 -nr
