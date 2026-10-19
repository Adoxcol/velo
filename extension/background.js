// Velo browser integration (MV3 service worker; an event page in Firefox).
// Firefox: use its promise-based browser.* API under the chrome name.
try { if (typeof browser === "object" && browser.runtime) globalThis.chrome = browser; } catch {}
// Translations (Firefox loads i18n.js as a background script before this one).
if (!globalThis.veloI18n && typeof importScripts === "function") importScripts("i18n.js");
const tr = (s) => (globalThis.veloI18n ? veloI18n.t(s) : s);

// Flow: pause the browser's download, hand it to the app,
// then cancel (app accepted) or resume (app unavailable / declined).

const HOST = "com.velo.host";
const MIN_SIZE = 0; // bytes; set e.g. 1_000_000 to ignore small files
// File types Velo takes over from the browser (like IDM's default list).
const DEFAULT_EXTS = [
  "zip rar 7z gz tgz tar bz2 xz zst z arj lzh cab iso img bin",
  "exe msi msix msu appx apk dmg pkg deb rpm appimage",
  "mp4 m4v mkv avi mov webm wmv flv mpg mpeg 3gp ogv ts",
  "mp3 m4a aac flac wav ogg opus wma aif aiff",
  "pdf doc docx xls xlsx ppt pptx odt ods odp epub mobi azw3 fb2 djvu cbz cbr rtf csv",
  "psd ai tif tiff torrent vhd vhdx ttf otf",
].join(" ");

async function settings() {
  const s = await chrome.storage.local.get({ enabled: true, exts: DEFAULT_EXTS, skipHosts: "" });
  return {
    enabled: s.enabled,
    exts: new Set(s.exts.toLowerCase().split(/[\s,]+/).filter(Boolean)),
    skipHosts: s.skipHosts.split(/[\s,]+/).filter(Boolean),
  };
}

async function send(msg) {
  try {
    const reply = await chrome.runtime.sendNativeMessage(HOST, msg);
    if (reply?.ext) maybeReload(reply.ext);
    return reply || { ok: false, error: "empty reply" };
  } catch (e) {
    return { ok: false, error: e?.message || String(e) };
  }
}

// Keeping an unpacked extension up to date: Velo's updater replaces the
// extension files next to the app and reports their version. If that's newer
// than what's running, reload to pick them up (once per version, so a copy
// loaded from another folder can't loop). Store installs update themselves.
function newerVersion(a, b) {
  const pa = a.split(".").map(Number), pb = b.split(".").map(Number);
  for (let i = 0; i < Math.max(pa.length, pb.length); i++) {
    if ((pa[i] || 0) !== (pb[i] || 0)) return (pa[i] || 0) > (pb[i] || 0);
  }
  return false;
}
async function maybeReload(diskVersion) {
  const running = chrome.runtime.getManifest().version;
  if (!/^\d+(\.\d+)*$/.test(diskVersion) || !newerVersion(diskVersion, running)) return;
  const self = await chrome.management.getSelf();
  if (self.installType !== "development") return;
  const { reloadedFor } = await chrome.storage.local.get("reloadedFor");
  if (reloadedFor === diskVersion) return;
  await chrome.storage.local.set({ reloadedFor: diskVersion });
  setTimeout(() => chrome.runtime.reload(), 1500); // let the current reply finish
}

async function cookieHeader(url) {
  try {
    const list = await chrome.cookies.getAll({ url });
    return list.map((c) => `${c.name}=${c.value}`).join("; ");
  } catch {
    return "";
  }
}

async function handOff({ url, referrer, filename, kind, format, title, source, size }) {
  const facebook = facebookVideoPage(url);
  if (facebook) { url = facebook; kind = "page"; filename = ""; }
  else if (partialMediaURL(url)) return { ok: false, error: "This link is only part of a video. Open the original video post and download from there." };
  return send({
    type: "download",
    url,
    referrer: referrer || "",
    filename: filename || "",
    kind: kind || "",
    format: format || "",
    title: title || "",
    source: source || "",
    size: size > 0 ? size : 0,
    cookies: await cookieHeader(url),
    userAgent: navigator.userAgent,
  });
}

function extOf(name) {
  const m = /\.([a-z0-9]{1,6})$/i.exec(name || "");
  return m ? m[1].toLowerCase() : "";
}

// Taking over browser downloads (like IDM). Chrome/Edge: decide in
// onDeterminingFilename, before the browser's own "Save as" box can appear.
// Other browsers: pause the download as soon as it's created, then decide.
async function takeOver(item, name) {
  const url = item.finalUrl || item.url;
  if (!/^https?:/i.test(url)) return false; // blob:, data:, file:
  const s = await settings();
  if (!s.enabled) return false;
  try {
    if (s.skipHosts.some((h) => new URL(url).hostname.endsWith(h))) return false;
  } catch {
    return false;
  }
  const size = item.totalBytes > 0 ? item.totalBytes : item.fileSize;
  if (!s.exts.has(extOf(name)) || (size > 0 && size < MIN_SIZE)) return false;
  // "capture": Velo shows its download dialog.
  const reply = await handOff({ url, referrer: item.referrer, filename: name, source: "capture", size });
  if (!reply.ok) console.warn("Velo unavailable, downloading in the browser:", reply.error);
  return !!reply.ok;
}

// Chrome and Edge only (Firefox has no such event; it uses onCreated below).
const onDeterminingFilename = chrome.downloads["onDeterminingFilename"];
if (onDeterminingFilename) {
  onDeterminingFilename.addListener((item, suggest) => {
    const name = (item.filename || "").split(/[\\/]/).pop();
    takeOver(item, name).then((taken) => {
      if (taken) {
        chrome.downloads.cancel(item.id, () => {
          void chrome.runtime.lastError;
          chrome.downloads.erase({ id: item.id });
        });
      } else {
        suggest(); // keep the browser's own handling
      }
    });
    return true; // suggest() is called asynchronously
  });
} else {
  // Firefox and others: pause the download as soon as it's created, decide,
  // then cancel it (Velo took it) or let it continue. Firefox already knows
  // the file name here; other browsers report it in onChanged.
  const pending = new Set();
  const decide = async (item) => {
    if (await takeOver(item, item.filename.split(/[\\/]/).pop())) {
      await chrome.downloads.cancel(item.id).catch(() => {});
      chrome.downloads.erase({ id: item.id }).catch(() => {});
    } else {
      chrome.downloads.resume(item.id).catch(() => {});
    }
  };
  chrome.downloads.onCreated.addListener((item) => {
    if (!/^https?:/i.test(item.finalUrl || item.url) || item.state !== "in_progress") return;
    chrome.downloads.pause(item.id).catch(() => {});
    if (item.filename) return decide(item);
    pending.add(item.id);
  });
  chrome.downloads.onChanged.addListener(async (delta) => {
    if (!pending.has(delta.id) || !delta.filename) return;
    pending.delete(delta.id);
    const [item] = await chrome.downloads.search({ id: delta.id });
    if (item) decide(item);
  });
}

chrome.runtime.onInstalled.addListener(() => {
  chrome.contextMenus.removeAll().then(() => {
    chrome.contextMenus.create({ id: "velo-link", title: tr("Download with Velo"), contexts: ["link", "video", "audio", "image"] });
    chrome.contextMenus.create({ id: "velo-all", title: tr("Download all links with Velo"), contexts: ["page"] });
    chrome.contextMenus.create({ id: "velo-selection", title: tr("Download selected links with Velo"), contexts: ["selection"] });
  });
});

chrome.contextMenus.onClicked.addListener((info, tab) => {
  if (info.menuItemId === "velo-all" || info.menuItemId === "velo-selection") {
    return downloadAll(tab, info.menuItemId === "velo-selection");
  }
  menuDownload(info, tab);
});

// "Download with Velo" on a link, image, video or audio. Velo shows its
// download dialog, as for a captured download.
async function menuDownload(info, tab) {
  let url = info.mediaType === "video" || info.mediaType === "audio" ? info.srcUrl || info.linkUrl : info.linkUrl || info.srcUrl;
  let reply;
  const page = videoPage(info.linkUrl) || ((info.mediaType === "video") && videoPage(info.frameUrl || tab?.url));
  if (page) {
    reply = await handOff({ url: page, kind: "page", title: tab?.title, referrer: tab?.url });
  } else if (url && /^https?:/i.test(url)) {
    reply = await handOff({ url, referrer: info.frameUrl || tab?.url, filename: "", source: "capture" });
  } else if (info.mediaType === "video" || info.mediaType === "audio") {
    // Streamed players use blob: URLs; download the stream Velo detected instead.
    if (tab && unsupportedSite(tab.url)) return notifyTab(tab.id, "This site uses a protected streaming format that Velo doesn't support.");
    const entry = tab?.id != null && best((await getMedia(tab.id)).items);
    if (!entry) return notifyTab(tab.id, "Velo hasn't found this video's stream yet. Play it for a moment and try again.");
    reply = await grab(tab.id, entry);
  } else {
    return tab?.id != null && notifyTab(tab.id, "Velo can't download this kind of link.");
  }
  if (!reply.ok && tab?.id != null) notifyTab(tab.id, reply.error && /not running|native host|exited/i.test(reply.error)
    ? "Velo isn't running. Start Velo and try again." : "Velo: " + (reply.error || "couldn't start the download."));
}

// Runs in the page: collects the links on it (or in the selection).
function collectLinks(selectionOnly) {
  let anchors = [...document.querySelectorAll("a[href]")];
  const sel = getSelection();
  if (selectionOnly && sel && sel.rangeCount) {
    anchors = anchors.filter((a) => [...Array(sel.rangeCount).keys()].some((i) => sel.getRangeAt(i).intersectsNode(a)));
  }
  const seen = new Set();
  return anchors
    .map((a) => ({ url: a.href, text: (a.textContent || "").trim().slice(0, 200) }))
    .filter((l) => /^https?:/i.test(l.url) && !seen.has(l.url) && seen.add(l.url));
}

// "Download all with Velo": send the page's links to Velo, which shows a
// dialog to pick which to download (files are pre-selected).
async function downloadAll(tab, selectionOnly) {
  if (!tab?.id) return;
  let links = [];
  try {
    const results = await chrome.scripting.executeScript({ target: { tabId: tab.id }, func: collectLinks, args: [selectionOnly] });
    links = results[0]?.result || [];
  } catch (e) {
    return notifyTab(tab.id, "Velo can't read links on this page.");
  }
  if (!links.length) return notifyTab(tab.id, selectionOnly ? "No links in the selection." : "No links found on this page.");
  const reply = await send({
    type: "batch",
    page: tab.url,
    referrer: tab.url,
    cookies: await cookieHeader(tab.url),
    userAgent: navigator.userAgent,
    links,
  });
  if (!reply.ok) notifyTab(tab.id, "Couldn't reach Velo: " + reply.error);
}

function notifyTab(tabId, text) {
  chrome.tabs.sendMessage(tabId, { type: "toast", text }, { frameId: 0 }).catch(() => {});
}

// ---------------------------------------------------------------------------
// Video grabber: watch each tab's network
// traffic for HLS playlists and direct media files.

const MEDIA_EXT = /\.(mp4|m4v|webm|mkv|mov|flv|mp3|m4a|ogg|opus|wav|flac)(\?|$)/i;
const SEGMENT_EXT = /\.(ts|m4s|aac|vtt|key)(\?|$)/i;

function header(details, name) {
  const h = details.responseHeaders?.find((x) => x.name.toLowerCase() === name);
  return h ? h.value : "";
}

function classify(details) {
  const url = details.url;
  const type = header(details, "content-type").toLowerCase().split(";")[0].trim();
  if (facebookVideoPage(url)) return "page";
  if (partialMediaURL(url)) return null;
  if (/\.m3u8(\?|$)/i.test(url) || type === "application/vnd.apple.mpegurl" || type === "application/x-mpegurl") return "hls";
  if (/\.mpd(\?|$)/i.test(url) || type === "application/dash+xml") return "dash";
  if (SEGMENT_EXT.test(url) || type === "video/mp2t" || type === "video/iso.segment") return null;
  if (type.startsWith("video/") || type.startsWith("audio/") || MEDIA_EXT.test(url)) return "file";
  return null;
}

function mediaSize(details) {
  const range = header(details, "content-range"); // "bytes 0-1023/123456"
  if (range.includes("/")) return parseInt(range.split("/")[1], 10) || 0;
  return parseInt(header(details, "content-length"), 10) || 0;
}

// Per-tab state lives in session storage so it survives the service worker
// being suspended.
const key = (tabId) => `media:${tabId}`;
async function getMedia(tabId) {
  return (await chrome.storage.session.get(key(tabId)))[key(tabId)] || { items: [], variants: [] };
}
async function setMedia(tabId, m) {
  await chrome.storage.session.set({ [key(tabId)]: m });
  const n = m.items.length;
  chrome.action.setBadgeText({ tabId, text: n ? String(n) : "" });
  chrome.action.setBadgeBackgroundColor({ tabId, color: "#2563eb" });
}

// Reads an HLS playlist to tell master playlists (several qualities) from
// the per-quality media playlists they point to, so only one entry shows.
async function inspectPlaylist(url) {
  try {
    const text = await (await fetch(url, { credentials: "include" })).text();
    if (!text.startsWith("#EXTM3U")) return null;
    const variants = [];
    const options = []; // qualities to offer: { url, height, bandwidth }
    let best = 0;
    const lines = text.split(/\r?\n/);
    lines.forEach((line, i) => {
      if (line.startsWith("#EXT-X-STREAM-INF")) {
        const res = /RESOLUTION=\d+x(\d+)/.exec(line);
        const bw = /[:,]BANDWIDTH=(\d+)/.exec(line);
        if (res) best = Math.max(best, +res[1]);
        const next = lines.slice(i + 1).find((l) => l.trim() && !l.startsWith("#"));
        if (next) {
          const href = new URL(next.trim(), url).href;
          variants.push(href);
          options.push({ url: href, height: res ? +res[1] : 0, bandwidth: bw ? +bw[1] : 0 });
        }
      }
    });
    // Best first; one entry per resolution.
    options.sort((a, b) => b.height - a.height || b.bandwidth - a.bandwidth);
    const unique = options.filter((o, i) => !o.height || options.findIndex((x) => x.height === o.height) === i);
    const encrypted = /#EXT-X-KEY:METHOD=(?!NONE)/.test(text);
    const live = !variants.length && !text.includes("#EXT-X-ENDLIST");
    return { master: variants.length > 0, variants, options: unique, quality: best ? best + "p" : "", encrypted, live };
  } catch {
    return null;
  }
}

// Reads a DASH manifest: refuses DRM, live and multi-period ones, and finds
// the best video height.
async function inspectManifest(url) {
  try {
    const text = await (await fetch(url, { credentials: "include" })).text();
    if (!/<MPD[\s>]/.test(text)) return null;
    const heights = [...text.matchAll(/\bheight="(\d+)"/g)].map((m) => +m[1]);
    return {
      encrypted: /<(\w+:)?ContentProtection\b/.test(text),
      live: /\btype="dynamic"/.test(text),
      multiPeriod: (text.match(/<(\w+:)?Period\b/g) || []).length > 1,
      quality: heights.length ? Math.max(...heights) + "p" : "",
    };
  } catch {
    return null;
  }
}

const seen = new Set(); // urls handled in this worker lifetime

chrome.webRequest.onHeadersReceived.addListener(
  (details) => {
    if (details.tabId < 0 || !/^https?:/i.test(details.url)) return;
    const kind = classify(details);
    if (!kind) return;
    const id = details.tabId + " " + details.url;
    if (seen.has(id)) return;
    seen.add(id);
    recordMedia(details, kind);
  },
  { urls: ["<all_urls>"], types: ["media", "xmlhttprequest", "other", "object"] },
  ["responseHeaders"]
);

async function recordMedia(details, kind) {
  const tabId = details.tabId;
  const entry = { url: kind === "page" ? facebookVideoPage(details.url) : details.url, kind, size: kind === "page" ? 0 : mediaSize(details), quality: kind === "page" ? "Best quality" : "", note: "" };
  if (kind === "file" && entry.size && entry.size < 300 * 1024) return; // ignore tiny clips/sounds
  if (kind === "hls") {
    const info = await inspectPlaylist(details.url);
    if (!info) return;
    if (info.encrypted) entry.note = "encrypted";
    if (info.live) entry.note = "live";
    entry.quality = info.quality;
    if (info.options.length > 1) entry.options = info.options;
    const m = await getMedia(tabId);
    if (info.master) m.variants.push(...info.variants);
    else if (m.variants.includes(details.url)) return; // a quality of a master we already have
    m.items = m.items.filter((x) => !(x.kind === "hls" && info.variants.includes(x.url)));
    if (m.items.some((x) => x.url === entry.url)) return;
    m.items.push(entry);
    await setMedia(tabId, m);
    return announce(tabId, entry);
  }
  if (kind === "dash") {
    const info = await inspectManifest(details.url);
    if (!info) return;
    if (info.encrypted) entry.note = "encrypted";
    else if (info.live) entry.note = "live";
    else if (info.multiPeriod) entry.note = "unsupported";
    entry.quality = info.quality;
  }
  const m = await getMedia(tabId);
  if (m.items.some((x) => x.url === entry.url)) return;
  m.items.push(entry);
  await setMedia(tabId, m);
  await announce(tabId, entry);
}

// Tell the page's video button that the list of videos changed.
async function announce(tabId, entry) {
  if (entry.note) return;
  chrome.tabs.sendMessage(tabId, { type: "detected" }).catch(() => {});
}

// New page in the tab: forget what the old page played.
chrome.tabs.onUpdated.addListener((tabId, info) => {
  if (info.status === "loading" && info.url) {
    chrome.storage.session.remove(key(tabId));
    chrome.action.setBadgeText({ tabId, text: "" });
  }
});
chrome.tabs.onRemoved.addListener((tabId) => chrome.storage.session.remove(key(tabId)));

// Sites that stream through their own protected players instead of
// standard HLS or files. Velo deliberately doesn't support them.
const UNSUPPORTED = /(^|\.)(netflix\.com|disneyplus\.com|primevideo\.com|hulu\.com|max\.com|spotify\.com)$/i;

function unsupportedSite(url) {
  try {
    return UNSUPPORTED.test(new URL(url).hostname);
  } catch {
    return false;
  }
}

// The best candidate: a playable stream (HLS, then DASH) first, then the largest file.
function best(items) {
  const ok = items.filter((x) => !x.note && !partialMediaURL(x.url));
  return ok.find((x) => x.kind === "page") || ok.find((x) => x.kind === "hls") || ok.find((x) => x.kind === "dash") || ok.sort((a, b) => b.size - a.size)[0];
}

async function grab(tabId, entry, format) {
  const tab = await chrome.tabs.get(tabId);
  if (entry.note === "encrypted") return { ok: false, error: "This video is DRM-protected and can't be downloaded." };
  if (entry.note === "live") return { ok: false, error: "Live streams aren't supported." };
  if (entry.note === "unsupported") return { ok: false, error: "This video's format (multi-part DASH) isn't supported yet." };
  const stream = entry.kind !== "file";
  const fmt = stream && ["1080p", "720p", "480p", "audio"].includes(format) ? format : "";
  return handOff({
    url: entry.url,
    referrer: tab.url,
    kind: stream ? entry.kind : "",
    format: fmt,
    title: stream ? tab.title : "",
    filename: "",
  });
}

chrome.runtime.onMessage.addListener((msg, sender, reply) => {
  (async () => {
    const tabId = msg.tabId ?? sender.tab?.id;
    const tab = tabId != null ? await chrome.tabs.get(tabId).catch(() => null) : null;
    const unsupported = tab && unsupportedSite(tab.url);
    const media = tab ? await getMedia(tabId) : { items: [] };
    // Normalize cached entries from older extension versions too.
    media.items = media.items.map((item) => {
      const page = facebookVideoPage(item.url);
      return page ? { url: page, kind: "page", quality: "Best quality", note: "", size: 0 } : item;
    }).filter((item, i, all) => !partialMediaURL(item.url) && all.findIndex((x) => x.url === item.url) === i);
    // Resolve the page on demand: SPA navigation must never reuse the previous video.
    let page = videoPage(tab?.url);
    // Content scripts can identify the hovered post on a feed. Keep the URL
    // on the sender's origin; a web page cannot ask us to extract arbitrary sites.
    const candidate = videoPage(msg.pageUrl || msg.url);
    if (sender.tab && candidate) {
      try { if (new URL(candidate).origin === new URL(sender.url).origin) page = candidate; } catch {}
    }
    if (page) media.items = [{ url: page, kind: "page", quality: "Best quality", note: "" }];
    if (msg.type === "media") return reply({ ...media, unsupported });
    if (msg.type === "grab") {
      if (unsupported) {
        return reply({ ok: false, error: "This site uses a protected streaming format that Velo doesn't support." });
      }
      const m = media;
      const entry = msg.url ? m.items.find((x) => x.url === msg.url) : best(m.items);
      if (!entry) return reply({ ok: false, error: "No downloadable video found on this page yet. Start playing it first." });
      // A chosen quality must be one the page offered.
      const variant = msg.variant && entry.options?.some((o) => o.url === msg.variant) ? msg.variant : null;
      return reply(await grab(tabId, variant ? { ...entry, url: variant } : entry, msg.format));
    }
    if (msg.type === "ping" || msg.type === "show") return reply(await send({ type: msg.type }));
    if (msg.type === "all") {
      await downloadAll(tab, false);
      return reply({ ok: true });
    }
    reply({ ok: false, error: "unknown message" });
  })().catch((error) => reply({ ok: false, error: error.message || String(error) }));
  return true; // async reply
});

// Only individual video/post URLs, never a home feed or playlist.
function videoPage(raw) {
  try {
    const u = new URL(raw);
    if (!/^https?:$/.test(u.protocol)) return null;
    const on = (domain) => u.hostname === domain || u.hostname.endsWith("." + domain);
    const p = u.pathname.split("/").filter(Boolean);
    if (on("youtu.be") && p.length === 1) return u.href;
    if ((on("youtube.com") || on("youtube-nocookie.com")) &&
        ((u.pathname === "/watch" && u.searchParams.get("v")) ||
         (p.length === 2 && ["shorts", "embed", "live"].includes(p[0])))) return u.href;
    if ((on("x.com") || on("twitter.com")) && p.length >= 3 && p[1] === "status") return u.href;
    if (on("facebook.com") && ((["/watch", "/watch/", "/video.php"].includes(u.pathname) && u.searchParams.get("v")) ||
        (p.length === 2 && p[0] === "reel") || (p.length >= 3 && p[1] === "videos"))) return u.href;
  } catch {}
  return null;
}

// Facebook's CDN URLs may be a range of a single DASH track, not a movie.
// Its efg metadata identifies the original video without stripping signatures
// or guessing that a complete video track also contains audio.
function facebookVideoPage(raw) {
  try {
    const u = new URL(raw);
    if (!/^https?:$/.test(u.protocol) || !(u.hostname === "fbcdn.net" || u.hostname.endsWith(".fbcdn.net"))) return null;
    const encoded = u.searchParams.get("efg");
    if (!encoded || encoded.length > 16384) return null;
    const meta = JSON.parse(atob(encoded.replace(/-/g, "+").replace(/_/g, "/")));
    const id = meta.video_id;
    if (typeof id === "number" && !Number.isSafeInteger(id)) return null;
    if (!/^[1-9][0-9]{0,19}$/.test(String(id))) return null;
    return "https://www.facebook.com/watch/?v=" + id;
  } catch { return null; }
}

function partialMediaURL(raw) {
  try {
    const u = new URL(raw);
    return u.searchParams.has("bytestart") || u.searchParams.has("byteend");
  } catch { return false; }
}
