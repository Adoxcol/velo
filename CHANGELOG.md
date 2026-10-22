# Changelog

All notable changes to Velo. The newest version is first.

## 0.8.14 — 2026-10-23

### New
- **Velo is on Firefox Add-ons.** The Browser Extension page and the welcome guide now have an **Add to Firefox** button next to **Add to Chrome**, and Firefox installs connect to Velo straight away.

## 0.8.13 — 2026-10-22

### New
- A **What's new** dialog shows the installed version's changelog after each update. Notes work offline and appear once per version after dismissal.
- Release notes wait for other dialogs to close and skip fresh-install onboarding.

## 0.8.12 — 2026-10-22

### Improved
- Faster joining of HLS and DASH video segments at the end of a video download.
- Clearer labels for screen readers on the Settings switches and icon buttons.
- Every way of adding a download now accepts only web links (http and https).

## 0.8.11 — 2026-10-22

### Changed
- **"Offer to download copied links" is now off by default**, so copying a link no longer pops up a Velo notification. Turn it on in **Settings → General** if you want it. This also switches it off once for existing installs.

### Improved
- The search box has a clear button, and Escape clears it.
- Faster setup of DASH video downloads.
- Velo's local connection only accepts web links (http and https).

## 0.8.10 — 2026-10-20

### Fixed
- **Rename videos before downloading:** Extension video downloads now open the browser capture dialog when “Ask on capture” is enabled, preserving the selected quality or format.
- **Correct video filenames:** Video page downloads resolve the actual video title instead of using a stale browser tab or search title. Custom filenames remain supported.

### New
- **MP4 video downloads:** Choose **MP4 video** in the extension popup, on-video menu, right-click menu, or browser capture dialog. Velo prefers MP4 sources and converts other sources when needed.

## 0.8.9 — 2026-10-20

### New
- **Browser extension quality & audio-only selector:** Pick **Best quality**, **1080p Full HD**, **720p HD**, **480p SD**, or **Audio only (MP3)** directly from the browser extension toolbar popup, the on-video **Download with Velo** (`▾`) menu, and the right-click context menu on videos.
- **YouTube playlist expansion into Batch Download:** Pasting a YouTube playlist URL (`youtube.com/playlist?list=...`) in **Add Download** (or copying it to the clipboard) automatically lists all videos in the playlist inside Velo's **Batch Download** dialog so you can choose which ones to download.
- **Embedded video subtitles:** Video downloads (`Best quality`, `1080p`, `720p`, `480p`) automatically embed available subtitles/closed captions into the `.mkv` container.
- **Open when finished:** Right-click any active, queued, or paused download and choose **Open when finished** to launch the file automatically the moment it completes.
- **Export download links (`.txt`):** Added **Export all links (.txt)…** to the right-click context menu to save your download URLs via the native Windows Save dialog.
- **CLI (`veloctl`) quality & batch flags:** Added `-format 1080p|720p|480p|audio` and `-i links.txt` to `veloctl get` and `veloctl add`, plus a live terminal progress bar showing transfer speed, size, and ETA.

### Improved
- **Custom themed UI dropdowns:** Replaced native OS `<select>` menus across Velo and the browser capture dialog with Velo's custom themed dropdown menu.

## 0.8.8 — 2026-10-19

### New
- **One-click Video Tools Manager:** **Settings → Video Grabber** now shows the live status and path of `yt-dlp`, `ffmpeg`, and `deno`, with a one-click **Install / Update video tools** button (and a **Fix video tools** button directly on failed video cards) that downloads or updates them into Velo's folder.
- **Video quality and audio-only selector:** Pick **Best quality**, **1080p**, **720p**, **480p**, or **Audio only (MP3)** directly from the browser extension popup, the on-video **Download with Velo** menu, or the Add Download and Browser Capture dialogs for video pages, HLS, and DASH streams. Audio-only downloads extract `.mp3` and categorize under **Music**.
- **Per-segment progress bar:** Multi-connection downloads now display live IDM-style parallel segment progress chunks inside each download card's progress bar.
- **Drop a `.txt` link list to batch-download:** Dragging and dropping a `.txt` file (or multiple links) onto the Velo window opens the **Batch Download** dialog with all links pre-selected.
- **Delete file from disk:** Added **Delete file from disk…** (with confirmation) to the context menu of completed downloads alongside **Remove from list**.

## 0.8.7 — 2026-10-19

### New
- **Video thumbnails, image previews and native file/app icons:** Download cards, Velo Mini, the browser capture dialog and the batch download dialog now display real previews—instant YouTube/Shorts thumbnails while queued or downloading, extracted frame thumbnails (with a play badge) for completed videos, cropped previews for images, and real Windows file-type and embedded `.exe` application icons. Extracted thumbnails are cached on disk across restarts.

### Improved
- **Cleaner sidebar navigation:** Merged the separate **Video/Audio** sidebar tab into filter chips on **Home**, **Downloads** and **Completed**, and added a one-click **Clear completed** button directly on the **Completed** page.

## 0.8.6 — 2026-10-19

### Fixed
- **Video tools (`ffmpeg`, `yt-dlp`, `deno`) are found reliably even when missing from the process `PATH`:** Velo now reads the live Windows Registry `Path` (`HKCU` and `HKLM`) and checks standard package-manager directories (WinGet Links and Packages, Scoop, Chocolatey, Homebrew, `~/.local/bin`, `~/.deno/bin`, Snap) for both video page downloads and HLS/DASH streams. Tools installed while Velo is already running work immediately without restarting Velo or rebooting.
- **Prevent Windows from dropping the User `PATH` when `PATH` exceeds 4,095 characters:** When adding Velo to `PATH` would push `System PATH + User PATH` past Windows's 4,095-character `WM_SETTINGCHANGE` buffer limit, the installer now deduplicates the User `PATH`, removes entries already present in System `PATH`, and compacts `%LOCALAPPDATA%` and `%USERPROFILE%` prefixes.


## 0.8.5 — 2026-10-19

### Fixed
- `go install github.com/Adoxcol/velo/cmd/veloctl@latest` works: the repository no longer contains two folders whose names differ only in capital letters, which Go refuses to download.


## 0.8.4 — 2026-10-19

### New
- **A proper setup wizard.** Installing, updating and uninstalling Velo now happen in one Velo-styled window instead of a series of pop-ups: choose a desktop shortcut and whether to add `veloctl` to PATH, watch the progress, then add the browser extension and open Velo from the last page. Uninstalling asks once, with an option to also delete your settings and history. Automatic updates stay silent.
- **Install `veloctl` with a package manager:** `brew install adoxcol/tap/veloctl` on macOS and Linux, `scoop install veloctl` on Windows, `.deb` and `.rpm` packages for Debian, Ubuntu and Fedora, `go install github.com/Adoxcol/velo/cmd/veloctl@latest`, or one line: `curl -fsSL https://velo.adoxcol.com/install.sh | sh`.
- A standalone `veloctl` for Windows (x64 and ARM64), without the desktop app.

### Improved
- The website's Download buttons start the download straight away and confirm it.

## 0.8.3 — 2026-10-18

### New
- **The browser extension is on the Chrome Web Store** as **Velo Download Manager**, for Chrome, Edge and Brave. No more Developer mode: add it in one click.
- Installing Velo now makes Chrome offer the extension by itself ("New extension added"), and the setup's last step opens the store page for Edge and Brave.
- The Browser Extension page and the welcome guide have an **Add to Chrome** button. Loading the unpacked extension is still there for developers.

### Improved
- Clearer labels for screen readers on icon buttons, and visible focus outlines when using the keyboard.
- Faster file-name checks for downloads.

## 0.8.2 — 2026-10-17

### Fixed
- Updates no longer fail with "context deadline exceeded" on slow or unsteady connections. Velo used to give the whole update download 5 minutes; now it continues a partial download from where it stopped and only gives up if no data arrives for a minute, trying again up to six times.

## 0.8.1 — 2026-10-17

### Security
- Video page downloads run yt-dlp with `--ignore-config`, `--no-config-locations` and `--no-exec`, so a yt-dlp config file on the computer can't change what Velo runs.
- Velo's update folder is now private to your user account, and the local connection between the extension and the app has extra tests for its access token.

### Improved
- Faster handling of long download lists: downloads are looked up by number instead of searching the whole list, settings are cached in memory and the progress loop does less work.
- Internal clean-up of the download engine, the video page extractor and the tray menu, with more tests for checksums, proxies and pending browser downloads.

## 0.8.0 — 2026-10-17

### New
- **Standalone command-line downloads on Linux and macOS:** `veloctl get <url>` now uses the portable download engine without requiring the Windows desktop app. Release archives cover Intel/AMD 64-bit and ARM64, including Apple Silicon.
- Linux and macOS downloads check free disk space, use standard proxy environment variables and cancel external video tools together with their child processes.
- `veloctl version` reports the installed version. CLI downloads now recognize supported video page URLs as well as files, HLS and DASH.

### Verification and scope
- CI runs engine and CLI tests natively on Linux, macOS and Windows, including an actual command-line download against a local server. Linux and macOS release archives include checksums.
- This is the command-line portability milestone. Linux desktop windows, browser registration and AppImage/.deb packages are next; remote-control commands still require a running desktop app. macOS binaries are unsigned and not notarized.
- yt-dlp, ffmpeg and Deno remain separate optional dependencies for video page downloads.

## 0.7.2 — 2026-10-17

### New
- The installer offers to **add Velo to PATH**, so the `veloctl` command-line tool works from any terminal. Updates keep your choice, and uninstalling removes it.
- The README explains every `veloctl` command, with examples.

## 0.7.1 — 2026-10-17

### Fixed
- Facebook CDN links that contain only a byte-range fragment are no longer offered as complete MP4 files. When the link identifies the original video, Velo sends its Facebook page to yt-dlp to resolve the audio and video.
- Previously detected Facebook fragments are normalized too, so stale extension entries do not keep producing broken files. Fragments without a recoverable video page are rejected.

### Setup and verification
- Updated the video setup guide: yt-dlp, ffmpeg and a supported JavaScript runtime such as Deno remain separate dependencies, not bundled in this release. YouTube's "page needs to be reloaded" error may require a newer nightly yt-dlp build.
- YouTube downloading was verified with a complete 1080p video and audio. Facebook fragment-routing regression tests pass; a complete signed-in Facebook download still needs verification. Videos requiring login must be sent through the browser extension.
- Existing incomplete Facebook files are not repaired automatically; download them again after updating and refreshing the video page.

## 0.7.0 — 2026-10-17

### New
- Video page downloads for individual YouTube videos and Shorts, Facebook videos and reels, and X/Twitter posts through optional yt-dlp integration.
- The video hover button and extension popup can send supported page URLs to Velo, which selects the best available audio and video and merges them into an MKV file.
- Page downloads retain partial files for resuming and respect the selected download folder and file name.

### Fixed
- The video button is no longer disabled on YouTube. Page navigation refreshes the selected video, and supported feed layouts use the hovered post's permalink.
- Video detection updates now reach embedded players as well as the main page.

### Setup and limitations
- Install yt-dlp and ffmpeg on PATH or beside Velo; YouTube also needs a supported JavaScript runtime such as Deno. These tools are not bundled with this release.
- Page downloads use automatic best-quality selection. DRM and live streams are excluded; availability depends on the site and extractor. Cross-domain login flows may require additional support.
- The speed limit for page downloads applies per download and is captured when it starts.
- Automated tests cover URL routing, cookie scoping, progress, resume state and a local yt-dlp/ffmpeg download. Live YouTube, Facebook and X browser downloads have not yet been verified.

## 0.4.4 — 2026-10-17

### Fixed
- The **Download with Velo** button now appears on video players that cover the video with their own controls or keep it inside a web component, like the Internet Archive's.

### Improved
- Cancelling a download or deleting a queue now asks in Velo's own dialog, centred in the window, instead of a plain Windows box.

## 0.4.3 — 2026-10-17

### Changed
- The browser extension no longer pops up a "Video detected" card on pages that play video. The **Download with Velo** button on the video itself and the extension's popup still list every video Velo finds.
- The website's download buttons now download Velo directly.

## 0.4.2 — 2026-10-16

### Fixed
- **Copy debug info**, **Copy link** and pasting a link with Ctrl+V failed with "Cannot read properties of undefined". Velo now uses the Windows clipboard directly.

## 0.4.1 — 2026-10-16

### Improved
- The browser extension builds its popup, video button and cards with a safer method that never runs scripts or event handlers from markup. Mozilla's add-on checker now reports no warnings at all.
- Releases are now built, tested and published automatically on GitHub, ready for code signing.

## 0.4.0 — 2026-10-16

### New
- **Velo speaks your language.** The app, the download dialog, Velo Mini, the tray menu, notifications and the browser extension are translated into বাংলা (Bengali), Deutsch, Español, Français, हिन्दी (Hindi), Português (Brasil) and 简体中文 (Simplified Chinese). Velo follows your Windows language by default; pick another in **Settings → Appearance → Language**, and it switches straight away. The extension follows your browser's language.

### Fixed
- Velo now reads its settings file even if it was saved with a byte-order mark (for example after editing it in Notepad), instead of falling back to defaults.
- The Browser Extension page no longer mentions the old `install.ps1` setup script.

## 0.3.4 — 2026-10-16

### New
- **More categories:** Torrents, Disk images (`.iso`, `.img`, `.vhd`), E-books (`.epub`, `.mobi`, `.azw3`, comics) and Fonts each get their own folder. Velo also recognises more types in the existing categories, like `.m2ts`, `.ape`, camera RAW photos and `.msixbundle`.
- **Cleaner file icons.** Soft tinted tiles with a crisp icon in each category's colour, the same in the main window, the download dialog, Velo Mini and the link picker.
- The download dialog is wider and shorter, with tighter spacing.

### Fixed
- Downloads that fail before starting now show their file name instead of "Starting…".

## 0.3.3 — 2026-10-16

### New
- **A small download dialog, like IDM's.** Clicking a download in the browser now opens a compact "Download file" window on its own, instead of the whole Velo window. It shows the file, its size and type, the link and the folder it goes to, with **Start download**, **Download later** and **Cancel**. Rename the file in place, choose another folder, press Enter to start or Esc to cancel. Several downloads at once are shown one after another.

### Improved
- The extension's "Velo isn't connected" message now shows the browser's exact error, and Velo keeps a small connection log to help track down problems.
- The Firefox add-on now requires Firefox 142 or newer.

## 0.3.2 — 2026-10-16

### New
- **Named queues.** Make extra queues on the Scheduler page, each with its own download hours, days and limit, like a "Night" queue for big files. Pick a queue in the download dialog or use **Move to queue** in a download's menu, and filter the Queue page by queue.

## 0.3.1 — 2026-10-15

### New
- **Firefox support.** The Velo add-on now works in Firefox as well as Chrome, Edge and Brave. Clicked downloads open in Velo, and the installer connects Firefox automatically.
- In Firefox, the popup offers **Allow on all sites** if Velo's site access has been turned off.

### Fixed
- The "Velo isn't connected" message no longer points to an old setup script.

## 0.3.0 — 2026-10-15

### New
- **Portable version.** Unzip and run `velo.exe` from any folder, even a USB stick. Settings and history stay in a `data` folder next to it.
- **Taskbar progress.** Velo's taskbar button fills with the progress of everything downloading.
- **Keyboard shortcuts.** Arrows to select, Space to pause or resume, Enter to open, Delete to remove, Ctrl+V to download a copied link, and ? to list them all.
- **Queue control.** Waiting downloads can Start now, Download next, Move up or Move down, and the Queue page lists them in the order they will run.
- **Refresh link.** Give a paused or failed download a fresh link and it continues the same file, after Velo checks it is the same size.
- **Checksum check.** Paste a SHA-256, SHA-1 or MD5 when adding a download and Velo verifies the finished file.
- **Proxy settings.** Velo follows the Windows proxy, like your browser, or you can turn it off or set your own.
- **Password-protected downloads.** Username and password fields appear when a site asks for a login.
- **Copy debug info** in Settings → Advanced, for bug reports. Link details and your user folder are left out.
- **Download all links**: each type shows a count, and the filter accepts patterns like `*.pdf *.zip`.
- **Velo Mini** can be hidden until downloads finish or turned off for good, has a tray menu switch, and can be dragged anywhere.

## 0.2.0 — 2026-10-15

### Faster
- **Much faster downloads from GitHub and many CDNs.** Each part now uses its own connection; before, they shared one and were capped together. A GitHub test went from 0.3 MB/s to 5.1 MB/s.
- 16 connections per download by default, with more added automatically (up to 32) when a server slows each connection down.
- Parts go straight to where a link redirects, and the last part of a file no longer crawls.

### New
- **Survives network drops.** A lost connection waits and resumes by itself instead of failing.
- **Keeps Windows awake** while anything is downloading.
- **Checks free disk space** before starting.
- **Right-click → Download with Velo** opens Velo's download dialog, works on streamed videos, and says when Velo isn't running.

### Security
- File names from websites can no longer escape the download folder or use reserved Windows names.
- Login cookies are no longer saved next to unfinished downloads, and are removed from history once a download finishes.
- Hardened the local connection between the extension and the app.

## 0.1.0 — 2026-10-15

The first version of Velo.

- Segmented downloads with pause and resume, even after a restart.
- Browser extension for Chrome, Edge and Brave: clicked downloads open in Velo's download dialog.
- Video grabber for HLS and DASH streams, with a "Download with Velo" button and quality menu on videos.
- Download all links on a page, queue and scheduler, speed limit, Velo Mini, notifications, clipboard link detection, and folders sorted by type.
- Installer for your account only (no administrator rights) and automatic, verified updates.
