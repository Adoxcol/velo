// Firefox: use its promise-based browser.* API under the chrome name.
try { if (typeof browser === "object" && browser.runtime) globalThis.chrome = browser; } catch {}
if (globalThis.veloI18n) veloI18n.setup(veloI18n.STRINGS[veloI18n.lang] || {});

// Velo popup: the videos found on the current tab, quality choice, and
// "download all links on this page".
const $ = (id) => document.getElementById(id);
const svg = (d) => `<svg class="i" viewBox="0 0 24 24">${d}</svg>`;
const ICON = {
  play: '<path d="m8 5 11 7-11 7z"/>',
  bolt: '<path d="M13.5 2 4 13.5h6.5L9 22l11-12.5h-6.8z"/>',
  links: '<path d="M10 14a4 4 0 0 0 5.7 0l3-3a4 4 0 0 0-5.7-5.7l-1 1"/><path d="M14 10a4 4 0 0 0-5.7 0l-3 3a4 4 0 0 0 5.7 5.7l1-1"/>',
  video: '<rect x="3" y="5" width="18" height="14" rx="2"/><path d="m10 9 5 3-5 3z"/>',
  warn: '<path d="M12 3 2 21h20z"/><path d="M12 10v5M12 18h0"/>',
  shield: '<path d="M12 3 4 6v6c0 5 3.5 8 8 9 4.5-1 8-4 8-9V6z"/>',
};

function size(n) {
  if (!(n > 0)) return "";
  const u = ["B", "KB", "MB", "GB"];
  let i = 0;
  while (n >= 1024 && i < u.length - 1) { n /= 1024; i++; }
  return n.toFixed(i ? 1 : 0) + " " + u[i];
}
const mbps = (bw) => (bw >= 1e6 ? (bw / 1e6).toFixed(1) + " Mbps" : Math.round(bw / 1e3) + " kbps");
function fileName(url) {
  try {
    const p = new URL(url).pathname.split("/").filter(Boolean);
    return decodeURIComponent(p[p.length - 1] || url);
  } catch { return url; }
}
const esc = (s) => String(s).replace(/[&<>"']/g, (c) => ({ "&": "&amp;", "<": "&lt;", ">": "&gt;", '"': "&quot;", "'": "&#39;" }[c]));
const NOTE = { encrypted: "DRM-protected", live: "Live stream", unsupported: "Not supported yet" };
const KIND = { hls: "Stream (HLS)", dash: "Stream (DASH)", file: "File", page: "Best audio + video" };

function say(text, cls) { $("msg").textContent = text; $("msg").className = cls || ""; }

const PAGE_FORMATS = [
  { format: "", label: "Best quality", meta: "Best audio + video" },
  { format: "1080p", label: "Video · 1080p", meta: "Full HD" },
  { format: "720p", label: "Video · 720p", meta: "HD" },
  { format: "480p", label: "Video · 480p", meta: "SD" },
  { format: "audio", label: "Audio only (MP3)", meta: "MP3 audio" },
];

// Each choice is { item, variant?, format?, label, meta, note }.
function choicesFor(items) {
  const out = [];
  for (const item of items) {
    if (item.options?.length && !item.note) {
      for (const o of item.options) {
        out.push({ item, variant: o.url, label: o.height ? `Video · ${o.height}p` : "Video", meta: [KIND[item.kind], o.bandwidth ? mbps(o.bandwidth) : ""].filter(Boolean).join(" · ") });
      }
      out.push({ item, format: "audio", label: "Audio only (MP3)", meta: `${KIND[item.kind]} · MP3 audio` });
    } else if ((item.kind === "page" || item.kind === "dash") && !item.note) {
      for (const f of PAGE_FORMATS) {
        out.push({ item, format: f.format, label: f.label, meta: item.kind === "dash" ? `${KIND.dash} · ${f.meta}` : f.meta });
      }
    } else {
      const stream = item.kind !== "file";
      out.push({ item, label: stream ? `Video${item.quality ? " · " + item.quality : ""}` : fileName(item.url),
        meta: [KIND[item.kind], stream ? "" : size(item.size)].filter(Boolean).join(" · "), note: item.note });
      if (item.kind === "hls" && !item.note) {
        out.push({ item, format: "audio", label: "Audio only (MP3)", meta: `${KIND.hls} · MP3 audio` });
      }
    }
  }
  return out;
}

(async () => {
  const [tab] = await chrome.tabs.query({ active: true, currentWindow: true });
  $("openApp").onclick = () => chrome.runtime.sendMessage({ type: "show" });

  // Firefox doesn't grant host access at install: ask for it here.
  const ALL = { origins: ["<all_urls>"] };
  if (!(await chrome.permissions.contains(ALL))) {
    $("access").hidden = false;
    $("grant").onclick = async () => {
      if (await chrome.permissions.request(ALL)) { $("access").hidden = true; chrome.tabs.reload(tab.id); }
    };
  }

  // Is Velo reachable? (The host starts Velo if it isn't running.)
  chrome.runtime.sendMessage({ type: "ping" }).then((r) => {
    if (r?.ok) return;
    const err = r?.error || "No reply from Velo.";
    const advice = /not found/i.test(err) ? "Install or reinstall Velo (or run the portable velo.exe), then restart the browser."
      : /forbidden/i.test(err) ? `This copy of the extension (ID ${chrome.runtime.id}) isn't the one Velo allows. Load it from Velo's install folder, or reinstall Velo.`
      : /exited|communicating/i.test(err) ? "Velo's connector stopped unexpectedly. Restart Velo, then reopen this popup."
      : "";
    $("banner").hidden = false;
    veloSetHTML($("banner"), svg(ICON.warn) + `<div><b>Velo isn't connected.</b> ${esc(advice)}
      <div style="opacity:.8;margin-top:4px;font-size:11.5px">Browser said: ${esc(err)}</div></div>`);
  });

  const media = await chrome.runtime.sendMessage({ type: "media", tabId: tab.id });
  const body = $("body");
  let host = "";
  try { host = new URL(tab.url).hostname; } catch {}
  const pageCard = (sub) => `<div class="page"><div class="thumb">${svg(ICON.video)}</div><div>
    <div class="t">${esc(tab.title || host)}</div><div class="s">${esc(sub)}</div></div></div>`;
  const allBtn = `<button class="btn ghost" id="all">${svg(ICON.links)}Download all links on this page</button>`;

  if (media.unsupported) {
    veloSetHTML(body, `<div class="empty"><div class="ill">${svg(ICON.shield)}</div><b>This site isn't supported</b>
      It streams video in a protected format that Velo doesn't download.</div>` + allBtn);
  } else if (!media.items.length) {
    veloSetHTML(body, `<div class="empty"><div class="ill">${svg(ICON.play)}</div><b>No videos detected yet</b>
      Start playing the video, then open this again.</div>` + allBtn);
  } else {
    const choices = choicesFor(media.items);
    const firstOk = choices.findIndex((c) => !c.note);
    veloSetHTML(body, pageCard(host + (choices.length > 1 ? ` · ${choices.length} options` : "")) +
      `<button class="btn" id="go"${firstOk < 0 ? " disabled" : ""}>${svg(ICON.bolt)}Download with Velo</button>` +
      `<h4>Download as</h4><div class="opts">` + choices.map((c, i) => `
        <label class="opt${c.note ? " bad" : ""}" title="${esc(c.item.url)}">
          <input type="radio" name="c" value="${i}"${i === firstOk ? " checked" : ""}${c.note ? " disabled" : ""}>
          <span class="n">${esc(c.label)}</span>
          <span class="m">${c.note ? `<span class="tag">${NOTE[c.note]}</span>` : esc(c.meta)}</span></label>`).join("") +
      `</div><div class="row">${allBtn}</div>`);
    $("go").onclick = async () => {
      const c = choices[+document.querySelector('input[name="c"]:checked').value];
      $("go").disabled = true;
      const r = await chrome.runtime.sendMessage({ type: "grab", tabId: tab.id, url: c.item.url, variant: c.variant, format: c.format });
      if (r.ok) { say("Sent to Velo ✓", "ok"); setTimeout(() => window.close(), 700); }
      else { say(r.error, "err"); $("go").disabled = false; }
    };
  }
  $("all").onclick = async () => {
    $("all").disabled = true;
    say("Collecting links…");
    await chrome.runtime.sendMessage({ type: "all", tabId: tab.id });
    say("Sent to Velo — choose the files in its window.", "ok");
    setTimeout(() => window.close(), 1200);
  };
})();
