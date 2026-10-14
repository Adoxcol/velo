// Velo browser integration (MV3 service worker).
// Flow: pause the browser's download, hand it to the app,
// then cancel (app accepted) or resume (app unavailable / declined).

const HOST = "com.velo.host";
const MIN_SIZE = 0; // bytes; set e.g. 1_000_000 to ignore small files
const DEFAULT_EXTS = "zip rar 7z gz tar iso exe msi mp4 mkv avi mov mp3 flac pdf apk dmg";

async function settings() {
  const s = await chrome.storage.local.get({ enabled: true, exts: DEFAULT_EXTS, skipHosts: "" });
  return {
    enabled: s.enabled,
    exts: new Set(s.exts.toLowerCase().split(/[\s,]+/).filter(Boolean)),
    skipHosts: s.skipHosts.split(/[\s,]+/).filter(Boolean),
  };
}

function send(msg) {
  return new Promise((resolve) => {
    chrome.runtime.sendNativeMessage(HOST, msg, (reply) => {
      if (chrome.runtime.lastError) resolve({ ok: false, error: chrome.runtime.lastError.message });
      else resolve(reply || { ok: false, error: "empty reply" });
    });
  });
}

async function cookieHeader(url) {
  try {
    const list = await chrome.cookies.getAll({ url });
    return list.map((c) => `${c.name}=${c.value}`).join("; ");
  } catch {
    return "";
  }
}

async function handOff({ url, referrer, filename, kind, title }) {
  return send({
    type: "download",
    url,
    referrer: referrer || "",
    filename: filename || "",
    kind: kind || "",
    title: title || "",
    cookies: await cookieHeader(url),
    userAgent: navigator.userAgent,
  });
}

function extOf(name) {
  const m = /\.([a-z0-9]{1,6})$/i.exec(name || "");
  return m ? m[1].toLowerCase() : "";
}

const pending = new Set();

chrome.downloads.onCreated.addListener(async (item) => {
  const url = item.finalUrl || item.url;
  if (!/^https?:/i.test(url)) return; // skip blob:, data:, file:
  const s = await settings();
  if (!s.enabled) return;
  try {
    if (s.skipHosts.some((h) => new URL(url).hostname.endsWith(h))) return;
  } catch {
    return;
  }
  // Pause immediately; filename may not be known yet, decide once it is.
  pending.add(item.id);
  chrome.downloads.pause(item.id, () => void chrome.runtime.lastError);
});

chrome.downloads.onChanged.addListener(async (delta) => {
  if (!pending.has(delta.id) || !delta.filename) return;
  pending.delete(delta.id);
  const [item] = await chrome.downloads.search({ id: delta.id });
  if (!item) return;
  const s = await settings();
  const name = item.filename.split(/[\\/]/).pop();
  const wanted = s.exts.has(extOf(name)) && (item.totalBytes <= 0 || item.totalBytes >= MIN_SIZE);
  if (!wanted) return chrome.downloads.resume(item.id);

  const reply = await handOff({ url: item.finalUrl || item.url, referrer: item.referrer, filename: name });
  if (reply.ok) {
    chrome.downloads.cancel(item.id);
    chrome.downloads.erase({ id: item.id });
  } else {
    console.warn("Velo unavailable, resuming in browser:", reply.error);
    chrome.downloads.resume(item.id);
  }
});

chrome.runtime.onInstalled.addListener(() => {
  chrome.contextMenus.removeAll(() => {
    chrome.contextMenus.create({ id: "velo-link", title: "Download with Velo", contexts: ["link", "video", "audio", "image"] });
    chrome.contextMenus.create({ id: "velo-all", title: "Download all links with Velo", contexts: ["page"] });
    chrome.contextMenus.create({ id: "velo-selection", title: "Download selected links with Velo", contexts: ["selection"] });
  });
});

chrome.contextMenus.onClicked.addListener((info, tab) => {
  if (info.menuItemId === "velo-all" || info.menuItemId === "velo-selection") {
    return downloadAll(tab, info.menuItemId === "velo-selection");
  }
  const url = info.linkUrl || info.srcUrl;
  if (url) handOff({ url, referrer: tab?.url, filename: "" });
});

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
  const entry = { url: details.url, kind, size: mediaSize(details), quality: "", note: "" };
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

// "Link detected": tell the page's overlay about the first playable video
// found on it, so it can show the card. Once per page; can be turned off.
async function announce(tabId, entry) {
  if (entry.note) return;
  const { detectCard } = await chrome.storage.local.get({ detectCard: true });
  if (!detectCard) return;
  const m = await getMedia(tabId);
  if (m.announced) return;
  m.announced = true;
  await chrome.storage.session.set({ [key(tabId)]: m });
  const tab = await chrome.tabs.get(tabId).catch(() => null);
  if (!tab || unsupportedSite(tab.url)) return;
  chrome.tabs.sendMessage(tabId, { type: "detected", item: entry, title: tab.title }, { frameId: 0 }).catch(() => {});
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
const UNSUPPORTED = /(^|\.)(youtube\.com|youtu\.be|youtube-nocookie\.com|netflix\.com|disneyplus\.com|primevideo\.com|hulu\.com|max\.com|spotify\.com)$/i;

function unsupportedSite(url) {
  try {
    return UNSUPPORTED.test(new URL(url).hostname);
  } catch {
    return false;
  }
}

// The best candidate: a playable stream (HLS, then DASH) first, then the largest file.
function best(items) {
  const ok = items.filter((x) => !x.note);
  return ok.find((x) => x.kind === "hls") || ok.find((x) => x.kind === "dash") || ok.sort((a, b) => b.size - a.size)[0];
}

async function grab(tabId, entry) {
  const tab = await chrome.tabs.get(tabId);
  if (entry.note === "encrypted") return { ok: false, error: "This video is DRM-protected and can't be downloaded." };
  if (entry.note === "live") return { ok: false, error: "Live streams aren't supported." };
  if (entry.note === "unsupported") return { ok: false, error: "This video's format (multi-part DASH) isn't supported yet." };
  const stream = entry.kind === "hls" || entry.kind === "dash";
  return handOff({
    url: entry.url,
    referrer: tab.url,
    kind: stream ? entry.kind : "",
    title: stream ? tab.title : "",
    filename: "",
  });
}

chrome.runtime.onMessage.addListener((msg, sender, reply) => {
  (async () => {
    const tabId = msg.tabId ?? sender.tab?.id;
    const tab = tabId != null ? await chrome.tabs.get(tabId).catch(() => null) : null;
    const unsupported = tab && unsupportedSite(tab.url);
    if (msg.type === "media") return reply({ ...(await getMedia(tabId)), unsupported });
    if (msg.type === "grab") {
      if (unsupported) {
        return reply({ ok: false, error: "This site uses a protected streaming format that Velo doesn't support." });
      }
      const m = await getMedia(tabId);
      const entry = msg.url ? m.items.find((x) => x.url === msg.url) : best(m.items);
      if (!entry) return reply({ ok: false, error: "No downloadable video found on this page yet. Start playing it first." });
      // A chosen quality must be one the page offered.
      const variant = msg.variant && entry.options?.some((o) => o.url === msg.variant) ? msg.variant : null;
      return reply(await grab(tabId, variant ? { ...entry, url: variant } : entry));
    }
    if (msg.type === "ping" || msg.type === "show") return reply(await send({ type: msg.type }));
    if (msg.type === "all") {
      await downloadAll(tab, false);
      return reply({ ok: true });
    }
    reply({ ok: false, error: "unknown message" });
  })();
  return true; // async reply
});
