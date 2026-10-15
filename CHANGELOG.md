# Changelog

All notable changes to Velo. The newest version is first.

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
