# Changelog

All notable changes to Velo. The newest version is first.

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
