<p align="center"><img src="docs/velo-256.png" width="96" alt="Velo"></p>

# Velo

**Fast downloads. A smoother internet.** Velo is an open-source download manager for Windows with a
browser extension for Chrome and Edge.

- **Segmented downloads**: files are split into parts (8 by default) downloaded in parallel. When a part
  finishes, its connection takes over half of the part with the most left, so all connections stay busy.
- **Pause and resume**, even after closing Velo or restarting the PC.
- **Video grabber**: the extension finds HLS (`.m3u8`) and DASH (`.mpd`) streams and media files while
  a video plays. Velo picks the best quality and merges audio and video with ffmpeg.
- **Browser integration**: downloads in Chrome and Edge are handed to Velo, plus a
  "Download with Velo" right-click item.
- **Queue and scheduler**: downloads at once, a daily download window, and sleep or shut down when
  everything finishes.
- **Velo Mini**: a small always-on-top progress window in the corner while downloads run.
- **Download all with Velo**: right-click a page (or a selection) to send its links to Velo, then
  pick the files in a batch dialog. Cookies are only sent to links on the same site as the page.
- **Extension popup**: the videos found on the tab, a quality picker for HLS streams, and a
  "Link detected" card on pages that play a video (can be turned off).
- **Speed limit**, **auto-categorized folders** (Video, Music, Images, Documents, Compressed, Programs),
  **history**, **notifications**, **clipboard link detection**, light and dark themes.

Velo downloads what websites serve openly. It does not bypass DRM, encryption or other protections,
so protected streaming services and sites like YouTube are not supported.

## Install (from source)

Requirements: Windows 10/11, [Go](https://go.dev/) 1.27+, the WebView2 runtime (included with
Windows 11), and optionally [ffmpeg](https://ffmpeg.org/) on `PATH` for video streams.

```powershell
.\install.ps1
```

This builds `bin\velo.exe`, `bin\velo-host.exe` and `bin\veloctl.exe`, registers the native messaging
host for Chrome and Edge (current user only) and adds Velo to the Start menu.

To change the icon, edit `tools/mkicon` and run `go run ./tools/mkicon` then `go generate ./cmd/velo`.

Then load the extension: open `chrome://extensions` or `edge://extensions`, turn on **Developer mode**,
click **Load unpacked** and choose the `extension` folder. Its ID is
`dmdcijgljocjdmafenflenjhdjhenokc`.

## Layout

| Path | What it is |
|---|---|
| `cmd/velo/` | The desktop app: tray icon, window (WebView2, `ui.html`), queue, scheduler, settings |
| `cmd/velo-host/` | Native messaging host: the browser starts it per message; it forwards to the app |
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

```powershell
go test -race ./...        # engine and app tests (local test servers, no internet needed)
go vet ./...
```

Data lives in `%APPDATA%\Velo` (settings, history, log). Set `VELO_HOME` to use another folder.

## License

[MIT](LICENSE)
