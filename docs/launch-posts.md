# Launch posts

Ready to paste. Post from your own account, reply to comments in the first few hours, and don't
post everywhere on the same day: Reddit communities notice cross-posting spam.

Links: https://velo.adoxcol.com · https://github.com/Adoxcol/velo

---

## Reddit — r/software, r/opensource, r/windows

Check each subreddit's rules first (some want self-promotion flair or only allow it on certain days).

**Title:** I made Velo, a free and open-source download manager for Windows (an IDM alternative)

**Body:**

I got tired of paying for a download manager and of the free ones looking like they're from 2008, so I built my own.

**Velo** splits each file into up to 32 parts that download at the same time, resumes after network drops or restarts, and takes over downloads from Chrome, Edge, Brave and Firefox with a small IDM-style dialog.

What it does:

- 16 connections per file by default, more added automatically when a server throttles each one
- Pause/resume, even after rebooting; survives Wi-Fi drops
- Browser extension: clicked downloads open in Velo, plus a "Download with Velo" button on HLS/DASH videos
- Download all links on a page, queues with schedules, speed limit
- Checksum check, proxy support, files sorted into folders by type
- Portable version, light/dark theme, 8 languages
- Installs without admin rights, updates itself

It's MIT-licensed with no ads, no telemetry and no account. It downloads what sites openly serve, so no YouTube/DRM.

Download: https://velo.adoxcol.com
Source: https://github.com/Adoxcol/velo

It's new and not code-signed yet, so Windows SmartScreen will warn you ("More info" → "Run anyway"). Every release lists its SHA-256. Feedback and bug reports are very welcome.

---

## Hacker News — Show HN

**Title:** Show HN: Velo – an open-source segmented download manager for Windows

**URL:** https://github.com/Adoxcol/velo

**First comment (post it yourself right after submitting):**

Hi HN, I built Velo because I wanted an IDM-like download manager that is free and open source.

It's written in Go with a WebView2 UI. Some things I learned along the way:

- GitHub and many CDNs were stuck at ~0.3 MB/s at first. Go's HTTP client negotiated HTTP/2, so all 16 "connections" were multiplexed over one TCP connection that the server rate-limits as a whole. Forcing HTTP/1.1 per segment took the same download to ~5 MB/s.
- When a server throttles each connection, Velo adds connections (up to 32) while the per-connection speed stays low and stops when it no longer helps.
- Segments go straight to the final redirect URL, and cookies/auth are dropped if the redirect crosses hosts.
- The browser extension (MV3, Chrome/Edge/Brave/Firefox) talks to the app over native messaging. It never touches DRM or YouTube.

It installs per-user without admin and auto-updates from GitHub Releases with SHA-256 verification. It isn't code-signed yet, so SmartScreen will complain.

Happy to answer questions, and I'd love feedback on the engine.

---

## AlternativeTo

Add Velo at https://alternativeto.net/software/new/ and list it as an alternative to **Internet Download Manager**, Free Download Manager, JDownloader and Xtreme Download Manager.

- **Name:** Velo
- **Website:** https://velo.adoxcol.com
- **License:** Free, Open Source (MIT)
- **Platforms:** Windows, Chrome, Firefox, Microsoft Edge, Brave
- **Short description:** Fast, free and open-source download manager for Windows with browser integration.
- **Description:** Velo splits each file into up to 32 parts that download at once, resumes after network drops and restarts, and takes over downloads from Chrome, Edge, Brave and Firefox. It includes a video grabber for HLS/DASH streams, page link grabber, queues and scheduler, speed limit, checksum verification, proxy support, a portable version and 8 languages. No ads, no telemetry, no account.
- **Tags:** download-manager, download-accelerator, segmented-downloads, video-downloader, portable

---

## Product Hunt

**Name:** Velo
**Tagline:** The free, open-source download manager for Windows
**Topics:** Windows, Open Source, Productivity, Developer Tools

**Description:**
Velo makes downloads faster and dependable. It splits each file into parts that download at once, resumes after network drops or restarts, and catches downloads from Chrome, Edge, Brave and Firefox in a small dialog. Video grabber, queues, scheduler and portable mode included. Free, MIT-licensed, no ads or telemetry.

**Maker's first comment:**
Hi everyone! I built Velo because the good download managers cost money and the free ones feel dated. It's open source, installs without admin rights and updates itself. I'd love to hear what's missing for you.

Use the promo images in `docs/store/promo/` and screenshots from the website.

---

## X / Bluesky / Mastodon

Velo is out: a free, open-source download manager for Windows ⚡

Up to 32 connections per file, resume after drops, browser integration for Chrome/Edge/Brave/Firefox, video grabber, scheduler. No ads, no tracking.

https://velo.adoxcol.com

#opensource #windows
