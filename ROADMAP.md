# Velo roadmap

Ticked items are done. Work goes roughly top to bottom.

## Before going public
- [x] Security review, first pass (0.2.0)
  - [x] File names from websites can't escape the download folder (`..`, `CON`, `NUL`, too-long names)
  - [x] Login cookies kept out of the `.part.json` files in the download folder, and dropped from history once a download finishes
  - [x] Local API: constant-time token check, request timeouts, at most 50 downloads waiting for the dialog
  - [x] Checked: only Velo's extension can talk to Velo; cookies don't follow redirects to other sites; updates must match their SHA-256
- [ ] Code signing (SignPath for open source, or a certificate)
- [ ] Updater checks the installer's signature too (needs code signing first)
- [ ] Release builds stop allowing the dev extension ID once the store ID is in
- [x] "Copy debug info" button for bug reports (links and your user folder are hidden)

## Velo Mini
- [x] Hide until downloads finish, or never show again
- [x] Show Velo Mini toggle in the tray menu
- [x] Drag it anywhere; it reopens where you left it

## Speed and reliability
- [x] Separate HTTP/1.1 connections (HTTP/2 capped GitHub at ~300 KB/s)
- [x] 16 connections by default, more added automatically on slow servers
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
- [ ] Queue control: reorder, download next, several queues
- [x] Login-protected downloads: username and password in the Add dialog, shown when a site asks; never sent to other hosts

## Reach
- [ ] Firefox
- [ ] Translations
- [ ] Portable zip

## Polish
- [ ] Taskbar progress bar
- [ ] Keyboard shortcuts and search
- [ ] Landing page for launch
