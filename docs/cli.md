# Velo command-line downloads

Download the archive matching your system and processor from GitHub Releases.
Linux uses `linux`; macOS uses `darwin`. Choose `arm64` for Apple Silicon or
ARM64 Linux, and `amd64` for Intel/AMD 64-bit systems. Extract the archive and
run `./veloctl get "https://example.com/file.zip"`. No desktop app is required.

Optionally place `veloctl` in a directory on your PATH. On Windows, veloctl.exe
is included in the installer and portable package.

```
veloctl get "https://example.com/file.zip"
veloctl get "https://example.com/file.zip" /path/to/downloads
```

Ctrl+C pauses the download. Repeat the same command and destination to resume.
The default folder is `~/Downloads/Velo`. Files, HLS, DASH and supported video
page URLs use the same engine. Video pages need yt-dlp, ffmpeg and (for YouTube)
Deno; install them separately on PATH or alongside veloctl.

Linux and macOS use HTTP_PROXY, HTTPS_PROXY and NO_PROXY environment variables.
Windows also supports the Windows proxy settings. macOS binaries are unsigned
and not notarized. Linux binaries do not require a desktop environment.

`list`, `add`, `pause`, `resume`, `remove` and `limit` control a running Velo
application. This release provides standalone `get` on Linux and macOS; it does
not yet provide a desktop app or background service on those platforms.
