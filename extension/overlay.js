// "Download video" button shown when hovering a <video>.
(() => {
  if (window.__veloOverlay) return;
  // No button on sites Velo doesn't support (their own protected players).
  if (/(^|\.)(youtube\.com|youtube-nocookie\.com|netflix\.com|disneyplus\.com|primevideo\.com|hulu\.com|max\.com|spotify\.com)$/i.test(location.hostname)) return;
  window.__veloOverlay = true;

  const host = document.createElement("div");
  host.style.cssText = "position:fixed;z-index:2147483647;display:none;top:0;left:0;";
  const shadow = host.attachShadow({ mode: "closed" }); // page CSS can't restyle it
  shadow.innerHTML = `
    <style>
      button {
        all: initial; cursor: pointer; display: flex; align-items: center; gap: 6px;
        padding: 6px 10px; border-radius: 6px; background: rgba(37,99,235,.95); color: #fff;
        font: 600 12px "Segoe UI", system-ui, sans-serif; box-shadow: 0 2px 8px rgba(0,0,0,.35);
      }
      button:hover { background: #1d4ed8; }
      .msg { margin-top: 4px; padding: 5px 8px; border-radius: 6px; background: rgba(0,0,0,.8); color: #fff;
             font: 12px "Segoe UI", system-ui, sans-serif; max-width: 260px; display: none; }
    </style>
    <button><svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.5"
      stroke-linecap="round" stroke-linejoin="round"><path d="M12 4v11M6 10l6 6 6-6M5 20h14"/></svg>Download video</button>
    <div class="msg"></div>`;
  const btn = shadow.querySelector("button");
  const msg = shadow.querySelector(".msg");
  let current = null, hideTimer = null;

  function place(video) {
    const r = video.getBoundingClientRect();
    if (r.width < 200 || r.height < 120) return hide();
    host.style.top = Math.max(r.top + 8, 0) + "px";
    host.style.left = Math.max(r.right - 150, 0) + "px";
    host.style.display = "block";
  }

  function hide() {
    host.style.display = "none";
    msg.style.display = "none";
    current = null;
  }

  function show(text, ms) {
    msg.textContent = text;
    msg.style.display = "block";
    clearTimeout(hideTimer);
    hideTimer = setTimeout(hide, ms);
  }

  document.addEventListener("mouseover", (e) => {
    const v = e.target.closest?.("video") || (e.target === host ? current : null);
    if (v) {
      clearTimeout(hideTimer);
      current = v;
      if (!host.isConnected) document.documentElement.append(host);
      place(v);
    }
  }, true);

  document.addEventListener("mouseout", (e) => {
    if (current && !host.contains(e.relatedTarget) && e.relatedTarget !== host && !current.contains(e.relatedTarget)) {
      clearTimeout(hideTimer);
      hideTimer = setTimeout(hide, 600);
    }
  }, true);
  host.addEventListener("mouseenter", () => clearTimeout(hideTimer));
  host.addEventListener("mouseleave", () => { hideTimer = setTimeout(hide, 600); });
  window.addEventListener("scroll", () => current && place(current), { passive: true });

  btn.addEventListener("click", async (e) => {
    e.preventDefault();
    e.stopPropagation();
    let r;
    try {
      r = await chrome.runtime.sendMessage({ type: "grab" });
    } catch {
      r = { ok: false, error: "Velo extension was reloaded. Refresh this page." };
    }
    show(r.ok ? "Sent to Velo ✓" : r.error, r.ok ? 1500 : 4000);
  });

  // ---- "Link detected" card and notices (only in the top frame) ----
  if (window !== window.top) return;
  const cardHost = document.createElement("div");
  cardHost.style.cssText = "position:fixed;z-index:2147483647;right:20px;bottom:20px;";
  const cs = cardHost.attachShadow({ mode: "closed" });
  cs.innerHTML = `
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
    <div class="card" id="card"></div>`;
  const cardEl = cs.getElementById("card");
  let cardTimer;
  const bolt = '<svg viewBox="0 0 24 24"><path d="M13.5 2 4 13.5h6.5L9 22l11-12.5h-6.8z" fill="#fff"/></svg>';
  const play = '<svg viewBox="0 0 24 24"><path d="m8 5 11 7-11 7z" fill="#fff"/></svg>';
  const esc = (s) => String(s).replace(/[&<>"']/g, (c) => ({ "&": "&amp;", "<": "&lt;", ">": "&gt;", '"': "&quot;", "'": "&#39;" }[c]));
  function hideCard() {
    cardEl.classList.remove("on");
    clearTimeout(cardTimer);
    setTimeout(() => cardHost.remove(), 300);
  }
  function showCard(html, ms) {
    cardEl.innerHTML = html;
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
    if (msg.type !== "detected") return;
    cardEl.style.background = "";
    const it = msg.item;
    const kind = { hls: "Video stream", dash: "Video stream", file: "File" }[it.kind] || "File";
    const sizeText = it.size > 0 ? (it.size / 1048576).toFixed(1) + " MB" : "";
    const meta = [kind, it.quality, sizeText].filter(Boolean).join(" • ");
    showCard(`
      <div class="top"><div class="logo">${bolt}</div>
        <div><div class="h">Video detected</div><div class="sub">Velo found a downloadable video on this page.</div></div>
        <button class="x" id="x" title="Close">×</button></div>
      <div class="file"><div class="thumb">${play}</div><div><div class="name">${esc(msg.title || document.title)}</div><div class="sub">${esc(meta)}</div></div></div>
      <button class="go" id="go">Download</button>`, 12000);
    cs.getElementById("x").onclick = hideCard;
    cs.getElementById("go").onclick = async () => {
      let r;
      try { r = await chrome.runtime.sendMessage({ type: "grab", url: it.url }); }
      catch { r = { ok: false, error: "Velo extension was reloaded. Refresh this page." }; }
      showCard(`<div class="note">${esc(r.ok ? "Sent to Velo ✓" : r.error)}</div>`, r.ok ? 2000 : 4500);
      cardEl.style.background = "none";
    };
  });
})();
