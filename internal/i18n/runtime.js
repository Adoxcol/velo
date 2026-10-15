// Velo's page translator, shared by the app's windows and the browser
// extension. Pages are written in English; this swaps visible text for the
// current language as it appears, using the English text as the key.
//
//   veloI18n.setup(dict)   start translating document (dict: {english: translated})
//   veloI18n.t(text)       translate one string
//   veloI18n.observe(root) also translate a shadow root
//
// Keys may hold placeholders: "Download {0}" matches "Download 3" and
// translates the captured part too. Text joined with " · " or " • " is
// translated piece by piece. Elements with translate="no", and the
// selectors in SKIP, are left alone (file names, links, code).
(function () {
  "use strict";
  if (globalThis.veloI18n) return;
  let dict = {};
  let patterns = [];
  const SKIP = "[translate=no], code, kbd, input, textarea, script, style, .name, .u, .url, .file .name";
  const ATTRS = ["title", "placeholder", "aria-label"];
  const esc = (s) => s.replace(/[.*+?^${}()|[\]\\]/g, "\\$&");

  function load(d) {
    dict = d || {};
    patterns = Object.keys(dict).filter((k) => /\{\d\}/.test(k)).map((k) => {
      const order = [];
      const re = new RegExp("^" + esc(k).replace(/\\\{(\d)\\\}/g, (_, n) => (order.push(+n), "(.+?)")) + "$", "s");
      return { re, order, out: dict[k], fixed: k.replace(/{d}/g, "").length };
    }).sort((a, b) => b.fixed - a.fixed); // most specific first: "Downloading {0} file" before "{0} file"
  }

  function t(s) {
    if (!s || !patterns && !dict) return s;
    const lead = s.match(/^\s*/)[0], trail = s.match(/\s*$/)[0];
    const core = s.trim();
    if (!core) return s;
    return lead + tcore(core, 0) + trail;
  }
  function tcore(s, depth) {
    if (Object.prototype.hasOwnProperty.call(dict, s)) return dict[s];
    if (depth > 3) return s;
    for (const p of patterns) {
      const m = p.re.exec(s);
      if (!m) continue;
      return p.out.replace(/\{(\d)\}/g, (_, n) => {
        const i = p.order.indexOf(+n);
        return i < 0 ? "" : tcore(m[i + 1], depth + 1);
      });
    }
    for (const sep of [" · ", " • "]) {
      if (s.includes(sep)) {
        const parts = s.split(sep);
        const tr = parts.map((x) => tcore(x, depth + 1));
        if (tr.some((x, i) => x !== parts[i])) return tr.join(sep);
      }
    }
    return s;
  }

  const done = new WeakMap(); // text node -> the English it came from
  function translateNode(node) {
    if (node.nodeType === 3) {
      const p = node.parentElement;
      if (!p || p.closest(SKIP)) return;
      const v = node.nodeValue;
      if (done.get(node) === v) return;
      const out = t(v);
      if (out !== v) { node.nodeValue = out; done.set(node, out); }
      return;
    }
    if (node.nodeType !== 1) return;
    // Inputs keep what's typed but get their placeholder and tooltip translated.
    if (/^(INPUT|TEXTAREA)$/.test(node.tagName) && !node.closest("[translate=no]")) {
      for (const a of ATTRS) {
        const v = node.getAttribute(a);
        if (v) { const out = t(v); if (out !== v) node.setAttribute(a, out); }
      }
      return;
    }
    if (node.closest(SKIP)) return;
    // Sentences with inline markup (bold, code) are translated whole, so
    // each language can order its words; the translation keeps the markup.
    if (node.hasAttribute("data-i18n-html")) {
      if (node.getAttribute("data-i18n-html") !== "done") {
        const key = node.innerHTML.replace(/\s+/g, " ").trim();
        if (Object.prototype.hasOwnProperty.call(dict, key)) {
          node.setAttribute("data-i18n-html", "done");
          node.innerHTML = dict[key];
        }
      }
      return;
    }
    for (const a of ATTRS) {
      const v = node.getAttribute(a);
      if (v) { const out = t(v); if (out !== v) node.setAttribute(a, out); }
    }
    for (const c of node.childNodes) translateNode(c);
    if (node.shadowRoot) observe(node.shadowRoot);
  }

  function observe(root) {
    for (const c of root.childNodes) translateNode(c);
    new MutationObserver((muts) => {
      for (const m of muts) {
        if (m.type === "characterData") translateNode(m.target);
        else if (m.type === "attributes") translateNode(m.target);
        else m.addedNodes.forEach(translateNode);
      }
    }).observe(root, { childList: true, subtree: true, characterData: true, attributes: true, attributeFilter: ATTRS });
  }

  function setup(d) {
    load(d);
    if (!Object.keys(dict).length) return;
    const start = () => {
      observe(document.documentElement);
      if (document.title) document.title = t(document.title);
    };
    if (document.documentElement) start(); else document.addEventListener("DOMContentLoaded", start);
    // Native dialogs don't go through the DOM.
    for (const f of ["alert", "confirm", "prompt"]) {
      const orig = globalThis[f];
      if (typeof orig === "function") globalThis[f] = (msg, ...rest) => orig.call(globalThis, t(String(msg ?? "")), ...rest);
    }
  }

  globalThis.veloI18n = { setup, t, observe, load };
})();
