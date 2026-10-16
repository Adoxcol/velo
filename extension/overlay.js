// Firefox: use its promise-based browser.* API under the chrome name.
try { if (typeof browser === "object" && browser.runtime) globalThis.chrome = browser; } catch {}

// "Download with Velo" button on videos (top-right of the video while the
// pointer is over it), with a menu of every quality/file found on the page.
(() => {
  if (window.__veloOverlay) return;
  // No button on sites Velo doesn't support (their own protected players).
  if (/(^|\.)(youtube\.com|youtube-nocookie\.com|netflix\.com|disneyplus\.com|primevideo\.com|hulu\.com|max\.com|spotify\.com)$/i.test(location.hostname)) return;
  window.__veloOverlay = true;

  const BOLT = '<svg viewBox="0 0 24 24" width="14" height="14"><path d="M13.5 2 4 13.5h6.5L9 22l11-12.5h-6.8z" fill="currentColor"/></svg>';
  const CARET = '<svg viewBox="0 0 24 24" width="14" height="14" fill="none" stroke="currentColor" stroke-width="2.6" stroke-linecap="round" stroke-linejoin="round"><path d="m6 9 6 6 6-6"/></svg>';
  const esc = (s) => String(s).replace(/[&<>"']/g, (c) => ({ "&": "&amp;", "<": "&lt;", ">": "&gt;", '"': "&quot;", "'": "&#39;" }[c]));
  const host = document.createElement("div");
  host.style.cssText = "position:fixed;z-index:2147483647;display:none;top:0;left:0;";
  const shadow = host.attachShadow({ mode: "closed" }); // page CSS can't restyle it
  if (globalThis.veloI18n) veloI18n.observe(shadow);
  veloSetHTML(shadow, `
    <style>
      * { box-sizing: border-box; }
      .wrap { all: initial; display: flex; flex-direction: column; align-items: flex-end; gap: 6px; font: 13px/1.35 "Segoe UI", system-ui, sans-serif; }
      .pill { display: flex; border-radius: 9px; overflow: hidden; box-shadow: 0 4px 14px rgba(0,0,0,.35); }
      .pill button { all: unset; cursor: pointer; display: flex; align-items: center; gap: 7px; padding: 7px 11px;
        background: rgba(22,119,255,.96); color: #fff; font: 600 12.5px "Segoe UI", system-ui, sans-serif; }
      .pill button:hover { background: #0f63dd; }
      .pill .more { padding: 7px 8px; border-left: 1px solid rgba(255,255,255,.28); }
      .pill.idle button { background: rgba(13,23,40,.82); }
      .pill.idle button:hover { background: rgba(13,23,40,.95); }
      .menu { display: none; min-width: 250px; max-width: 340px; max-height: 300px; overflow: auto; padding: 6px; border-radius: 12px;
        background: #fff; color: #1b2433; box-shadow: 0 12px 36px rgba(13,23,40,.3); }
      .menu.on { display: block; }
      .menu .h { padding: 6px 10px 4px; color: #64748b; font-size: 11.5px; font-weight: 600; }
      .item { all: unset; box-sizing: border-box; display: flex; width: 100%; gap: 10px; align-items: center; padding: 8px 10px; border-radius: 8px; cursor: pointer; }
      .item:hover { background: #f1f5f9; }
      .item .n { flex: 1; min-width: 0; font-weight: 600; color: #0d1728; white-space: nowrap; overflow: hidden; text-overflow: ellipsis; }
      .item .m { color: #64748b; font-size: 12px; white-space: nowrap; }
      .item.bad { cursor: default; opacity: .55; }
      .item.bad:hover { background: none; }
      .sep { height: 1px; background: #e6ebf2; margin: 5px 4px; }
      .item.quiet .n { font-weight: 400; color: #64748b; }
      .msg { display: none; max-width: 280px; padding: 7px 11px; border-radius: 9px; background: rgba(13,23,40,.9); color: #fff; font-size: 12.5px; }
      .msg.on { display: block; }
    </style>
    <div class="wrap">
      <div class="pill" id="pill"><button id="go"></button><button class="more" id="more" title="Choose quality">${CARET}</button></div>
      <div class="menu" id="menu"></div>
      <div class="msg" id="msg"></div>
    </div>`);
  const $ = (id) => shadow.getElementById(id);
  let current = null, hideTimer = null, msgTimer = null, items = [], hidden = false;

  const size = (n) => (n > 0 ? (n >= 1073741824 ? (n / 1073741824).toFixed(2) + " GB" : (n / 1048576).toFixed(1) + " MB") : "");
  const mbps = (bw) => (bw >= 1e6 ? (bw / 1e6).toFixed(1) + " Mbps" : Math.round(bw / 1e3) + " kbps");
  const NOTE = { encrypted: "DRM-protected", live: "Live", unsupported: "Not supported" };

  // Every downloadable choice on the page: HLS qualities, DASH, files.
  function choices() {
    const out = [];
    for (const it of items) {
      if (it.options?.length && !it.note) {
        for (const o of it.options) out.push({ url: it.url, variant: o.url, label: o.height ? `${o.height}p` : "Video", meta: ["HLS", o.bandwidth ? mbps(o.bandwidth) : ""].filter(Boolean).join(" · ") });
      } else {
        let name = it.quality || "Video";
        if (it.kind === "file") { try { name = decodeURIComponent(new URL(it.url).pathname.split("/").pop()) || "Video file"; } catch {} }
        out.push({ url: it.url, label: name, meta: it.note ? NOTE[it.note] : [{ hls: "HLS", dash: "DASH", file: "File" }[it.kind], size(it.size)].filter(Boolean).join(" · "), bad: !!it.note });
      }
    }
    return out;
  }

  async function loadItems() {
    try { items = (await chrome.runtime.sendMessage({ type: "media" }))?.items || []; } catch { items = []; }
    const ok = choices().filter((c) => !c.bad);
    $("pill").classList.toggle("idle", !ok.length);
    veloSetHTML($("go"), BOLT + (ok.length ? "Download with Velo" : "Play to enable download"));
    $("more").style.display = ok.length > 1 || choices().some((c) => c.bad) ? "" : "none";
  }

  function place(video) {
    const r = video.getBoundingClientRect();
    if (hidden || r.width < 200 || r.height < 120) return hide();
    const fs = document.fullscreenElement;
    const parent = fs && fs.contains(video) ? fs : document.documentElement;
    if (host.parentNode !== parent) parent.append(host); // stay visible in fullscreen
    host.style.display = "block";
    host.style.top = Math.max(r.top + 10, 4) + "px";
    // Anchor to the video's right edge so opening the menu doesn't move the button.
    host.style.left = "auto";
    host.style.right = Math.max(document.documentElement.clientWidth - r.right + 10, 4) + "px";
  }

  function hide() {
    host.style.display = "none";
    $("menu").classList.remove("on");
    current = null;
  }

  function say(text, ms) {
    $("msg").textContent = text;
    $("msg").classList.add("on");
    clearTimeout(msgTimer);
    msgTimer = setTimeout(() => $("msg").classList.remove("on"), ms);
  }

  async function grab(c) {
    let r;
    try { r = await chrome.runtime.sendMessage(c ? { type: "grab", url: c.url, variant: c.variant } : { type: "grab" }); }
    catch { r = { ok: false, error: "Velo extension was reloaded. Refresh this page." }; }
    $("menu").classList.remove("on");
    say(r.ok ? "Sent to Velo ✓" : r.error, r.ok ? 1800 : 4500);
  }

  function openMenu() {
    const list = choices();
    veloSetHTML($("menu"), `<div class="h">Download as</div>` +
      list.map((c, i) => `<button class="item${c.bad ? " bad" : ""}" data-i="${i}"><span class="n">${esc(c.label)}</span><span class="m">${esc(c.meta)}</span></button>`).join("") +
      `<div class="sep"></div><button class="item quiet" data-hide="1"><span class="n">Hide on this page</span></button>`);
    $("menu").classList.add("on");
    $("menu").onclick = (e) => {
      const b = e.target.closest(".item");
      if (!b) return;
      if (b.dataset.hide) { hidden = true; return hide(); }
      const c = list[+b.dataset.i];
      if (!c.bad) grab(c);
    };
  }

  document.addEventListener("mouseover", (e) => {
    if (e.composedPath().includes(host)) { clearTimeout(hideTimer); return; }
    const v = e.target.closest?.("video");
    if (v && !hidden) {
      clearTimeout(hideTimer);
      if (current !== v) { current = v; loadItems().then(() => current && place(current)); }
      place(v);
    }
  }, true);
  document.addEventListener("mouseout", (e) => {
    if (!current || e.composedPath().includes(host)) return;
    const to = e.relatedTarget;
    if (to && (current.contains(to) || to === host)) return;
    clearTimeout(hideTimer);
    hideTimer = setTimeout(() => { if (!$("menu").classList.contains("on")) hide(); }, 900);
  }, true);
  host.addEventListener("mouseenter", () => clearTimeout(hideTimer));
  host.addEventListener("mouseleave", () => { hideTimer = setTimeout(hide, 900); });
  window.addEventListener("scroll", () => current && place(current), { passive: true });
  window.addEventListener("resize", () => current && place(current));
  document.addEventListener("fullscreenchange", () => current && place(current));
  document.addEventListener("click", (e) => { if (!e.composedPath().includes(host)) $("menu").classList.remove("on"); }, true);

  $("go").addEventListener("click", (e) => {
    e.preventDefault(); e.stopPropagation();
    const ok = choices().filter((c) => !c.bad);
    if (!ok.length) return say("Start playing the video first, then try again.", 3500);
    grab(null);
  });
  $("more").addEventListener("click", (e) => {
    e.preventDefault(); e.stopPropagation();
    $("menu").classList.contains("on") ? $("menu").classList.remove("on") : openMenu();
  });
  // Refresh the button when the page plays something new.
  chrome.runtime.onMessage.addListener((m) => { if (m.type === "detected" && current) loadItems(); });

  // ---- notices (only in the top frame) ----
  if (window !== window.top) return;
  const cardHost = document.createElement("div");
  cardHost.style.cssText = "position:fixed;z-index:2147483647;right:20px;bottom:20px;";
  const cs = cardHost.attachShadow({ mode: "closed" });
  if (globalThis.veloI18n) veloI18n.observe(cs);
  veloSetHTML(cs, `
    <style>
      .card { all: initial; display: block; width: 300px; background: #fff; color: #1b2433; border-radius: 14px;
        box-shadow: 0 12px 40px rgba(13,23,40,.25); font: 13px/1.4 "Segoe UI", system-ui, sans-serif; overflow: hidden;
        transform: translateY(16px); opacity: 0; transition: all .25s ease; }
      .card.on { transform: none; opacity: 1; }
      .top { display: flex; gap: 10px; align-items: flex-start; padding: 14px 14px 0; }
      .logo { width: 30px; height: 30px; border-radius: 8px; background: #1677ff; display: grid; place-items: center; flex: none; }
      .logo svg { width: 18px; height: 18px; }
      .h { font-weight: 700; color: #0d1728; font-size: 13.5px; }
      .sub { color: #64748b; font-size: 12px; }
      .x { all: initial; margin-left: auto; cursor: pointer; color: #64748b; font: 18px/1 sans-serif; padding: 2px 4px; border-radius: 6px; }
      .x:hover { background: #f1f5f9; color: #0d1728; }
      .file { display: flex; gap: 10px; align-items: center; margin: 12px 14px; padding: 10px; border-radius: 10px; background: #f5f8fc; }
      .thumb { width: 54px; height: 38px; border-radius: 7px; flex: none; background: linear-gradient(135deg,#14325f,#1677ff); display: grid; place-items: center; }
      .thumb svg { width: 18px; height: 18px; }
      .name { font-weight: 600; color: #0d1728; white-space: nowrap; overflow: hidden; text-overflow: ellipsis; }
      .file > div { min-width: 0; }
      .go { all: initial; display: block; margin: 0 14px 14px; padding: 10px; border-radius: 10px; background: #1677ff; color: #fff;
        font: 600 13.5px "Segoe UI", system-ui, sans-serif; text-align: center; cursor: pointer; }
      .go:hover { background: #0f63dd; }
      .note { all: initial; display: block; padding: 10px 14px; border-radius: 10px; background: #0d1728; color: #fff;
        font: 13px "Segoe UI", system-ui, sans-serif; box-shadow: 0 8px 24px rgba(0,0,0,.3); }
    </style>
    <div class="card" id="card"></div>`);
  const cardEl = cs.getElementById("card");
  let cardTimer;
  function hideCard() {
    cardEl.classList.remove("on");
    clearTimeout(cardTimer);
    setTimeout(() => cardHost.remove(), 300);
  }
  function showCard(html, ms) {
    veloSetHTML(cardEl, html);
    if (!cardHost.isConnected) document.documentElement.append(cardHost);
    requestAnimationFrame(() => cardEl.classList.add("on"));
    clearTimeout(cardTimer);
    cardTimer = setTimeout(hideCard, ms);
    cardEl.onmouseenter = () => clearTimeout(cardTimer);
    cardEl.onmouseleave = () => (cardTimer = setTimeout(hideCard, 4000));
  }
  chrome.runtime.onMessage.addListener((msg) => {
    if (msg.type === "toast") {
      showCard(`<div class="note">${esc(msg.text)}</div>`, 3500);
      cardEl.style.background = "none";
      return;
    }
  });
})();
