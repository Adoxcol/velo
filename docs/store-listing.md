# Store listing (Chrome Web Store and Microsoft Edge Add-ons)

Upload `dist/velo-extension-<version>.zip` (built by `tools/release.ps1`).

## Name
Velo — Download Manager

## Short description (max 132 characters)
Send downloads and videos to Velo for faster, resumable downloads. Grab HLS/DASH videos and download all links on a page.

## Detailed description
Velo is a free, open-source download manager for Windows. This extension connects your browser to the
Velo desktop app.

• Faster downloads: Velo splits files into parts and downloads them in parallel.
• Pause and resume, even after restarting your PC.
• Video downloads: when a page plays a standard HLS or DASH stream or a video file, a "Download with
  Velo" button appears on the video, with a choice of quality.
• Download all links: right-click a page to pick which files to download in one go.
• Queue, scheduler, speed limit and automatic folders by file type in the Velo app.

Requires the Velo app for Windows: https://github.com/Adoxcol/velo

Velo does not download DRM-protected or encrypted video, and does not support sites that protect their
videos, such as YouTube or streaming services.

## Category
Productivity (Chrome) / Productivity (Edge)

## Single purpose (Chrome)
Hand downloads, video streams and page links from the browser to the Velo download manager app on the
user's computer.

## Permission justifications
| Permission | Why Velo needs it |
|---|---|
| `downloads` | Pause a download the browser starts and hand it to Velo; resume it in the browser if Velo isn't available. |
| `nativeMessaging` | Talk to the installed Velo app (the only place data is sent). |
| `cookies` | Send the site's cookies with a download so files behind a sign-in still download. Only for the download's own site. |
| `webRequest` | Read response headers of the current tab to detect video streams and media files. No requests are blocked or changed. |
| `contextMenus` | "Download with Velo", "Download all links with Velo" and "Download selected links with Velo". |
| `scripting` | Collect the links on a page, only when the user chooses "Download all links". |
| `tabs` | Know which tab a detected video belongs to and use the page title to name downloaded videos. |
| `storage` | Remember the user's extension settings and the videos detected in each open tab. |
| Host access `<all_urls>` | Detect videos and capture downloads on any site the user visits. |

## Data usage disclosures
- Does not sell or transfer user data to third parties.
- Does not use data for purposes unrelated to the single purpose.
- Does not use data for credit or lending.
- Privacy policy URL: https://gist.github.com/Adoxcol/ea9009ab50bad69a5067c20ae35edd49
  (a public copy of [privacy.md](privacy.md); update both together)

## Images
- Icon: `extension/icons/icon128.png`
- Screenshots (1280×800), in `docs/store/`: `1-video-button.png`, `2-popup.png`, `3-app.png`,
  `4-batch.png`.
- Small promo tile (440×280): optional; still to make.

## After publishing
Put the store IDs in `cmd/velo-setup/main.go` (`chromeStoreID`, `edgeStoreID`) and rebuild the
installer. It will then register the store extension, and the browser shows "New extension added" on
its next start.
