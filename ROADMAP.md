# Velo roadmap

Ticked items are done. Work goes roughly top to bottom.

## Before going public
- [x] Security review, first pass (0.2.0)
  - [x] File names from websites can't escape the download folder (`..`, `CON`, `NUL`, too-long names)
  - [x] Login cookies kept out of the `.part.json` files in the download folder, and dropped from history once a download finishes
  - [x] Local API: constant-time token check, request timeouts, at most 50 downloads waiting for the dialog
  - [x] Checked: only Velo's extension can talk to Velo; cookies don't follow redirects to other sites; updates must match their SHA-256
- [x] Automated releases on GitHub Actions: push a vX.Y.Z tag (.github/workflows/release.yml)
- [ ] Code signing: reapply to SignPath Foundation once Velo has more stars and downloads
- [ ] Publish on winget (each release sends a pull request to microsoft/winget-pkgs; not merged yet)
- [x] `veloctl` on Homebrew, Scoop, .deb and .rpm (0.8.4)
- [ ] Updater checks the installer's signature too (needs code signing first)
- [ ] Release builds stop allowing the dev extension ID once the store ID is in
- [x] "Copy debug info" button for bug reports (links and your user folder are hidden)

## Velo Mini
- [x] Hide until downloads finish, or never show again
- [x] Show Velo Mini toggle in the tray menu
- [x] Drag it anywhere; it reopens where you left it

## Speed and reliability
- [x] Separate HTTP/1.1 connections (HTTP/2 capped GitHub at ~300 KB/s)
- [x] 16 connections by default, more added automatically on slow servers (up to 32; stops adding when the server answers 429/503 and honours Retry-After)
- [x] Resuming checks the file hasn't changed on the server (If-Range with its ETag or Last-Modified)
- [x] Segments go straight to the redirect target
- [x] Keep Windows awake while downloading
- [x] Survive network drops: wait and resume by itself (5s, 10s, 20s, then every minute)
- [x] Check free disk space before starting
- [x] Verify a pasted SHA-256, SHA-1 or MD5 when done
- [x] Proxy: follows the Windows proxy by default, or none, or a custom http:// / socks5:// proxy

## Features IDM users expect
- [x] Right-click "Download with Velo" on links, images, video and audio, opening Velo's dialog
- [x] Download all links on a page, filtered by type (counts on each type, `*.pdf *.zip` patterns)
- [x] Refresh an expired link and continue the same file (checks it is the same size)
- [x] Queue control: start now, download next, move up and down
- [x] Named queues, each with its own hours, days and limit (0.3.2)
- [x] Login-protected downloads: username and password in the Add dialog, shown when a site asks; never sent to other hosts

## Reach
- [x] Firefox: add-on package (velo-firefox zip), native host registered for Firefox, tested end to end
- [x] Firefox add-on published on addons.mozilla.org (0.8.14)
- [x] Translations: app, dialog, Velo Mini, tray, notifications and extension in বাংলা, Deutsch, Español, Français, हिन्दी, 日本語, Português (Brasil) and 简体中文 (0.4.0)
- [ ] More languages (add tools/i18n/app/<code>.json and tools/i18n/ext/<code>.json, then node tools/i18n/build.js)
- [x] Portable zip (settings in its own data folder, connects the browser when run)

## Polish
- [x] Taskbar progress bar
- [x] Search downloads (Ctrl+K)
- [x] Keyboard shortcuts: Ctrl+N add, Ctrl+V paste a link, arrows to select, Space pause/resume, Enter open, Delete remove, ? for the list
- [x] Landing page (site/), ready for velo.adoxcol.com
- [x] Landing page live at https://velo.adoxcol.com (with changelog)
- [x] Repo public; v0.3.1 published on GitHub Releases
- [x] Real auto-update tested: an installed 0.3.1 found 0.3.2 on GitHub, verified it and updated itself in under 3 minutes

## Platform support
- [x] Portable engine and standalone `veloctl get` on Windows, Linux and macOS (0.8.0)
- [x] Linux/macOS Intel/AMD 64-bit and ARM64 release archives with checksums
- [x] Native Linux/macOS CI tests for the engine and CLI
- [ ] Linux desktop app with a portable window library
- [ ] Linux browser native-messaging registration
- [ ] AppImage and .deb release packages
