# Velo privacy policy

_Last updated: 15 October 2026_

Velo is an open-source download manager for Windows and a companion browser extension for Chrome and
Edge. This policy covers both.

## The short version

Velo does not collect, sell or send your data anywhere. There are no accounts, analytics, ads or
trackers. Everything stays on your computer.

## What the extension reads, and why

- **Downloads you start in the browser**: the link, file name, referring page and your browser's user
  agent are passed to the Velo app on your computer so it can download the file instead of the browser.
- **Cookies for the site of a download**: sent to the Velo app with that download only, so files that
  need you to be signed in (for example on a course or file-hosting site) still download. For
  "Download all links", cookies are only attached to links on the same site as the page.
- **Network requests of the tab you're on**: the extension looks at response headers (content type and
  size) to spot video streams and media files, so it can offer to download them. It reads playlist
  files (`.m3u8`, `.mpd`) for those streams to list the available qualities.
- **Links on a page**: only when you choose "Download all links with Velo", so you can pick which to
  download.

## Where it goes

All of this is sent only to the Velo app on the same computer, through the browser's native messaging
(a local channel between the browser and an installed program). Velo then downloads the files you
asked for directly from the websites that host them. Nothing is sent to the Velo developers or any
third party.

## What the app stores

The Velo app keeps your settings and download history in `%APPDATA%\Velo` on your computer. You can
clear finished downloads from the list at any time; uninstalling Velo offers to delete this folder.

## Contact

Questions or concerns: open an issue at https://github.com/Adoxcol/velo/issues.
