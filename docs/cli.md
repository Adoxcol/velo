# Velo command-line downloads

`veloctl get <url> [folder]` runs on Windows, Linux and macOS without a desktop app.
Regular file downloads need no additional runtime. Use `veloctl version` to check
the installed version.

## Choose a download

| System | Processor | Archive |
|---|---|---|
| Linux | Intel/AMD 64-bit (`x86_64`) | [veloctl-linux-amd64.tar.gz](https://github.com/Adoxcol/velo/releases/latest/download/veloctl-linux-amd64.tar.gz) |
| Linux | ARM64 (`aarch64` or `arm64`) | [veloctl-linux-arm64.tar.gz](https://github.com/Adoxcol/velo/releases/latest/download/veloctl-linux-arm64.tar.gz) |
| macOS | Apple Silicon, M-series (`arm64`) | [veloctl-darwin-arm64.tar.gz](https://github.com/Adoxcol/velo/releases/latest/download/veloctl-darwin-arm64.tar.gz) |
| macOS | Intel (`x86_64`) | [veloctl-darwin-amd64.tar.gz](https://github.com/Adoxcol/velo/releases/latest/download/veloctl-darwin-amd64.tar.gz) |
| Windows 10/11 | Intel/AMD 64-bit | `veloctl.exe` is included in the [installer](https://github.com/Adoxcol/velo/releases/latest/download/Velo-Setup.exe) and [portable ZIP](https://github.com/Adoxcol/velo/releases/latest/download/Velo-Portable.zip). |

Run `uname -m` on Linux/macOS to check your architecture. `darwin` means macOS;
`amd64` also covers Intel x64 processors. macOS binaries are unsigned and not notarized.

## Install on Linux or macOS

1. Download your archive and the matching checksum file from the
   [latest release](https://github.com/Adoxcol/velo/releases/latest):
   [Linux checksums](https://github.com/Adoxcol/velo/releases/latest/download/SHA256SUMS-veloctl-linux.txt) or
   [macOS checksums](https://github.com/Adoxcol/velo/releases/latest/download/SHA256SUMS-veloctl-darwin.txt).
2. In the download folder, calculate the archive's SHA-256 and compare it with
   the entry for that exact filename in the checksum file:

   ```sh
   # Linux x64 example
   sha256sum veloctl-linux-amd64.tar.gz
   # macOS Apple Silicon example
   shasum -a 256 veloctl-darwin-arm64.tar.gz
   ```

3. Extract into a new folder (replace the filename with your download):

   ```sh
   mkdir velo-cli
   tar -xzf veloctl-linux-amd64.tar.gz -C velo-cli
   cd velo-cli
   ./veloctl version
   ./veloctl get "https://example.com/file.zip"
   ```

4. Optional: put the executable on your personal PATH:

   ```sh
   mkdir -p "$HOME/.local/bin"
   install -m 755 veloctl "$HOME/.local/bin/veloctl"
   export PATH="$HOME/.local/bin:$PATH"
   ```

   Add that `export PATH` line to your shell's startup file to keep it in new
   terminals (for example `~/.bashrc` for Bash or `~/.zshrc` for Zsh).

On Windows, the installer offers to add Velo to PATH. Open a new terminal after
installation. Otherwise run `veloctl.exe` from its installed or portable folder.

## Download and resume

```sh
veloctl get "https://example.com/file.zip"
veloctl get "https://example.com/file.zip" "$HOME/Downloads"
```

The default folder is `~/Downloads/Velo`. Ctrl+C pauses the download. Repeat the
same URL and destination to resume. Keep its `.part` and `.part.json` files;
stream downloads may also use a `.parts` folder. Resume depends on server support.

Linux and macOS use `HTTP_PROXY`, `HTTPS_PROXY` and `NO_PROXY` environment variables.
Windows also supports the Windows proxy settings.

## Video downloads

Files, HLS, DASH and supported video page URLs use the same engine. Page extraction
needs [yt-dlp](https://github.com/yt-dlp/yt-dlp#installation),
[ffmpeg](https://ffmpeg.org/download.html) and, for full YouTube support,
[Deno or another supported JavaScript runtime](https://github.com/yt-dlp/yt-dlp/wiki/EJS).
Install these separately on PATH or alongside veloctl. Keep yt-dlp and its required
components current. Page downloads select the best available quality and merge to MKV.

The standalone CLI does not import your browser login. Videos requiring browser
cookies may fail; browser integration is currently available with the Windows
app only. DRM and live streams are unsupported.

## Desktop-control commands

`list`, `add`, `pause`, `resume`, `remove` and `limit` control a running Windows Velo
app. They do not start a Linux/macOS service. The Linux desktop app, browser
registration and AppImage/.deb packaging are planned separately.

## Update or uninstall

CLI archives do not update themselves. Download the latest archive, verify its
checksum and replace the executable after any running download has stopped.
To uninstall, remove the executable you installed. Your downloads and partial
files stay in their destination folder. Video tools have their own updates.
