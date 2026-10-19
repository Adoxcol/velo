<p align="center"><img src="docs/velo-256.png" width="96" alt="Velo"></p>

# Velo

**Fast downloads. A smoother internet.** Velo is a free, open-source download manager:
a Windows desktop app with browser integration, and a standalone command-line
downloader for **Windows, Linux and macOS**.

[Website](https://velo.adoxcol.com/) · [Latest release](https://github.com/Adoxcol/velo/releases/latest) · [CLI installation guide](docs/cli.md)

## Download

| Platform | Download | Available today |
|---|---|---|
| Windows 10/11 | [Installer](https://github.com/Adoxcol/velo/releases/latest/download/Velo-Setup.exe) · [Portable ZIP](https://github.com/Adoxcol/velo/releases/latest/download/Velo-Portable.zip) | Desktop app, browser integration and `veloctl` |
| Linux | [Intel/AMD x64](https://github.com/Adoxcol/velo/releases/latest/download/veloctl-linux-amd64.tar.gz) · [ARM64](https://github.com/Adoxcol/velo/releases/latest/download/veloctl-linux-arm64.tar.gz) | Standalone `veloctl get` |
| macOS | [Apple Silicon](https://github.com/Adoxcol/velo/releases/latest/download/veloctl-darwin-arm64.tar.gz) · [Intel](https://github.com/Adoxcol/velo/releases/latest/download/veloctl-darwin-amd64.tar.gz) | Standalone `veloctl get` |

After extracting a Linux/macOS archive:

```sh
./veloctl get "https://example.com/file.zip"
./veloctl get "https://example.com/file.zip" "$HOME/Downloads"
```

Press **Ctrl+C** to pause; repeat the same command and destination to resume.
No desktop app is needed. See [installation and checksum verification](docs/cli.md).
A Linux desktop app and browser registration are planned.
macOS binaries are unsigned and not notarized.

## Install veloctl

| How | Command |
|---|---|
| **One-line script** (Linux, macOS) | `curl -fsSL https://velo.adoxcol.com/install.sh \| sh` |
| **Homebrew** (macOS, Linux) | `brew install adoxcol/tap/veloctl` |
| **Scoop** (Windows) | `scoop bucket add adoxcol https://github.com/Adoxcol/scoop-bucket` then `scoop install veloctl` |
| **Debian / Ubuntu** | Download `veloctl_<version>_amd64.deb` (or `arm64`) from the [latest release](https://github.com/Adoxcol/velo/releases/latest), then `sudo apt install ./veloctl_*.deb` |
| **Fedora / RHEL / openSUSE** | Download `veloctl-<version>-1.x86_64.rpm` (or `aarch64`), then `sudo dnf install ./veloctl-*.rpm` |
| **Go** 1.27+ | `go install github.com/Adoxcol/velo/cmd/veloctl@latest` |
| **Windows app** | The [installer](https://velo.adoxcol.com/download) includes `veloctl` and can add it to PATH |

The install script checks the download's SHA-256 and puts `veloctl` in
`~/.local/bin` (set `VELO_INSTALL_DIR` to choose another folder).

## Windows desktop features

- **Segmented downloads**: files are split into parts (16 by default) downloaded in parallel over
  separate connections. When a part finishes, its connection takes over half of the part with the most
  left, and servers that cap each connection (GitHub, many CDNs) automatically get more, up to 32.
- **Survives network drops**: a lost connection waits and resumes by itself; Windows is kept awake
  while downloading; free disk space is checked first.
- **Pause and resume**, even after closing Velo or restarting the PC.
- **Video grabber**: the extension finds HLS (`.m3u8`) and DASH (`.mpd`) streams and media files while
  a video plays. Velo picks the best quality and merges audio and video with ffmpeg.
- **Browser integration**: click a download link in Chrome, Edge, Brave or Firefox and Velo takes over: its download
  dialog opens (file name, size, folder, parts) instead of the browser's downloader. Can be set to
  start straight away instead. Plus a "Download with Velo" right-click item.
- **Updates**: Velo checks GitHub Releases, verifies the installer's SHA-256 and updates itself when
  idle; an unpacked extension reloads itself after an update.
- **Queue and scheduler**: downloads at once, a daily download window, and sleep or shut down when
  everything finishes.
- **Velo Mini**: a small always-on-top progress window in the corner while downloads run.
- **Download all with Velo**: right-click a page (or a selection) to send its links to Velo, then
  pick the files in a batch dialog. Cookies are only sent to links on the same site as the page.
- **Extension popup**: videos found on the tab, a quality picker for HLS streams, and a
  **Download with Velo** button over supported video players.
- **Queue control**: start now, download next, move up and down; **refresh an expired link** and
  continue the same file; optional **checksum** (SHA-256, SHA-1, MD5) checked when done.
- **Proxy** (follows Windows by default) and **password-protected downloads**.
- **Portable version**: unzip and run; settings stay next to the program.
- **Eight languages**: English, বাংলা, Deutsch, Español, Français, हिन्दी, Português (Brasil) and 简体中文, following your Windows language (the extension follows the browser's).
- **Speed limit**, **auto-categorized folders** (Video, Music, Images, Documents, Compressed, Programs),
  **history**, **notifications**, **clipboard link detection**, light and dark themes.

Velo downloads what websites serve openly. It does not bypass DRM, encryption or other protections,
so DRM-protected streaming services are not supported. Individual YouTube videos/Shorts,
Facebook video/reel pages and X/Twitter posts use the optional yt-dlp integration.

## Install on Windows

Download `Velo-Setup-<version>.exe` from the [releases page](https://github.com/Adoxcol/velo/releases/latest) and run it. It installs for your user account
only (no administrator rights), adds Velo to the Start menu and to **Settings → Apps** for uninstalling,
and connects Chrome, Edge, Brave and Firefox. Then add the browser extension:

- **Chrome, Edge and Brave:** [Velo Download Manager on the Chrome Web Store](https://chromewebstore.google.com/detail/pcpdknogjedamkhakgfbaoieehnheelk).
  After installing Velo, Chrome also offers it by itself ("New extension added").
- **Firefox:** coming to Firefox Add-ons.

Prefer no installer? `Velo-Portable-<version>.zip` runs from any folder (see its README.txt).

**Firefox**: install the Velo add-on from Firefox Add-ons once it is listed. To try it before that,
open `about:debugging` → This Firefox → Load Temporary Add-on and pick `manifest.json` from an unzipped
`velo-firefox-<version>.zip` (temporary add-ons are removed when Firefox closes).


The installer is not code-signed yet, so Windows SmartScreen may warn about an unknown publisher.

## Build a release

Tagged GitHub releases build Windows packages and Linux/macOS CLI archives.
See [the release guide](docs/releasing.md). To build the Windows packages locally:

```powershell
.\tools\release.ps1 -Version 0.1.0
```

Writes `dist\Velo-Setup-0.1.0.exe` (the installer, which embeds the app and the extension),
`dist\Velo-Portable-0.1.0.zip`, `dist\velo-firefox-0.1.0.zip` (for Firefox Add-ons) and
`dist\velo-extension-0.1.0.zip` (for the Chrome Web Store and Edge Add-ons; see
[docs/store-listing.md](docs/store-listing.md) and [docs/privacy.md](docs/privacy.md)).

## Build from source

### Standalone CLI: Windows, Linux and macOS

Requires Go 1.27+. From the repository root:

```sh
go build -o veloctl ./cmd/veloctl
go test -race ./internal/engine ./internal/ipc ./cmd/veloctl
```

On Windows, use `-o veloctl.exe`. On Linux/macOS, run `./veloctl get "<url>"`.

### Windows desktop app

Requirements: Windows 10/11, [Go](https://go.dev/) 1.27+, the WebView2 runtime (included with
Windows 11), and optionally [ffmpeg](https://ffmpeg.org/) on `PATH` for video streams.

```powershell
.\install.ps1
```

This builds `bin\velo.exe`, `bin\velo-host.exe` and `bin\veloctl.exe`, registers the native messaging
host for Chrome, Edge, Brave and Firefox (current user only) and adds Velo to the Start menu.

To change the icon, edit `tools/mkicon` and run `go run ./tools/mkicon` then `go generate ./cmd/velo`.

Then load the extension: open `chrome://extensions` or `edge://extensions`, turn on **Developer mode**,
click **Load unpacked** and choose the `extension` folder. Its ID is
`dmdcijgljocjdmafenflenjhdjhenokc`.

## Command line (veloctl)

`veloctl get <url> [folder]` and `veloctl version` run independently on all three
platforms. The other commands below control a running **Windows desktop app**;
Linux/macOS do not yet have a desktop app or background service.

On Windows, `veloctl.exe` is included in the installer and portable ZIP. The
installer can add Velo to PATH; open a new terminal afterwards. On Linux/macOS,
extract your archive and use `./veloctl`, or follow the [PATH setup guide](docs/cli.md).

| Command | What it does |
|---|---|
| `veloctl version` | Print the installed version (all platforms) |
| `veloctl list` | Show all downloads with their number (`#id`), status, progress, speed and connections |
| `veloctl add <url>` | Add a download to Velo (files, HLS, DASH and supported video pages are recognised); prints `ok <id>` |
| `veloctl pause <id>` | Pause a download |
| `veloctl resume <id>` | Resume a paused or failed download |
| `veloctl remove <id>` | Remove a download from the list (unfinished partial files are deleted) |
| `veloctl limit <KB/s>` | Set the speed limit for all downloads; `0` removes it |
| `veloctl get <url> [folder]` | Download right in the terminal, without the app. Saves to `~/Downloads/Velo` unless you give a folder. Ctrl+C pauses; run the same command again to resume |

Examples:

```powershell
veloctl add https://example.com/big-file.zip
veloctl list
veloctl pause 3
veloctl resume 3
veloctl limit 2048        # 2 MB/s
veloctl limit 0           # no limit
veloctl get https://example.com/big-file.zip D:\Downloads
```

## Video page downloads

Install [yt-dlp](https://github.com/yt-dlp/yt-dlp#installation),
[ffmpeg](https://ffmpeg.org/download.html) and
[Deno](https://github.com/yt-dlp/yt-dlp/wiki/EJS) for YouTube's JavaScript challenges.
Put the tools on `PATH` or beside `veloctl` (or `velo.exe` on Windows), then restart Velo.
For the standalone CLI, use `veloctl get "<video-page-url>"`; regular files need no extra tools.
These optional tools are not bundled in the installer. Keep yt-dlp current as sites change.
The official Windows yt-dlp executable includes the EJS components; Deno and ffmpeg
remain separate tools. If YouTube reports "The page needs to be reloaded", update
yt-dlp; the upstream fix is available in its nightly builds (`yt-dlp --update-to nightly`).

With the Windows browser extension, open an individual YouTube, Facebook video/reel, or X/Twitter post and hover over
its video, then click **Download with Velo**. You can also use the extension popup
or paste the video page URL into Velo. On feeds, the hover button uses the enclosing
post's permalink when available; otherwise open the individual video/post first.
Facebook CDN byte-range fragments are resolved to the original video page when
the link identifies it; otherwise Velo refuses the fragment. Videos requiring a
Facebook login must be sent through the browser extension so it can supply the
page cookies. Other sites retain the existing HLS, DASH and direct media detection.

Page downloads select the best available audio and video and save a merged MKV.
They support cancellation and resuming partial files. Browser cookies for the
page host are passed through a temporary cookie file and removed after extraction;
logins requiring cookies from other domains may still fail. DRM and live streams
are excluded. Availability depends on the site and yt-dlp; not every video works.
The speed limit is captured when each page download starts and applies per download,
not as Velo's shared live bandwidth budget. Quality selection is currently automatic
for page downloads; the existing HLS quality picker is unchanged.

## Translations

Velo's text is written in English and translated at runtime. To add or fix a language:

1. Run `node tools/i18n/extract.js` to see any new English text, and add it to `internal/i18n/locales/app/keys.json` (or `ext/keys.json` for the extension).
2. Add the translation at the same position in `tools/i18n/app/<code>.json` (and `tools/i18n/ext/<code>.json`). A new language also goes in `Languages` in `internal/i18n/i18n.go`.
3. Run `node tools/i18n/build.js`. It checks every line keeps its `{0}` placeholders and HTML tags, and writes the files Velo and the extension load.

## Layout

| Path | What it is |
|---|---|
| `cmd/velo/` | The desktop app: tray icon, window (WebView2, `ui.html`), queue, scheduler, settings |
| `cmd/velo-host/` | Native messaging host: the browser starts it per message; it forwards to the app |
| `cmd/velo-setup/` | Installer and uninstaller (per user); embeds the release build |
| `cmd/veloctl/` | Command-line control: `list`, `add`, `pause`, `resume`, `remove`, `limit`, `get` |
| `internal/engine/` | Download engine: segmented HTTP, HLS, DASH, speed limit, categories |
| `internal/ipc/` | How the host and CLI find the app: `%APPDATA%\Velo\endpoint.json` (port + token) |
| `extension/` | Manifest V3 extension: download capture, video detection, popup, hover button |

### How the browser talks to Velo

The extension never opens a network port. It uses the browser's **native messaging**: Chrome starts
`velo-host.exe` (allowed only for Velo's extension ID) and exchanges length-prefixed JSON over stdin and
stdout. The host forwards each message to the app over `127.0.0.1` on a random port, with a random token
from `%APPDATA%\Velo\endpoint.json`, so web pages can't add downloads.

## Development

Windows (all packages):

```powershell
go test -race ./...        # engine and app tests (local test servers, no internet needed)
go vet ./...
```

For Linux/macOS, run the portable package test command under **Build from source**;
Windows UI packages are not portable yet.

Windows desktop data lives in `%APPDATA%\Velo` (settings, history, log). Set `VELO_HOME` to use another folder.

## License

[MIT](LICENSE)
