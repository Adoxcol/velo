// Lists the user-visible English text in Velo's pages, to keep the
// translation files complete. Run from the repo root:
//
//   node tools/i18n/extract.js            -> prints strings not yet in internal/i18n/locales/<group>/keys.json
//   node tools/i18n/extract.js --all      -> prints every string found
//
// It reads text between tags, title/placeholder/aria-label attributes, and
// string and template literals in scripts, keeps what looks like prose, and
// turns ${...} inside template literals into {0}, {1}... placeholders.
const fs = require("fs");
const path = require("path");

const SOURCES = {
  app: ["cmd/velo/ui.html", "cmd/velo/capture.html", "cmd/velo/mini.html"],
  ext: ["extension/popup.html", "extension/popup.js", "extension/overlay.js", "extension/background.js"],
};

const looksLikeText = (s) => {
  s = s.trim();
  if (s.length < 2 || !/[A-Za-z]{2}/.test(s)) return false;
  if (/^[a-z][A-Za-z0-9]*$/.test(s)) return false;                 // identifiers: "queued", "hidden"
  if (/^[\w-]+(\.[\w-]+)+$/.test(s)) return false;                  // file.ext, a.b.c
  if (/^(https?:|chrome|edge|about:|#|\.|\/|\$|%|--|<|\[)/.test(s)) return false;
  if (/[{};=]|=>|\(\)|\b(function|const|let|return|var)\b/.test(s)) return false;
  if (/^[\w-]+:\s/.test(s) && /;/.test(s)) return false;            // css
  if (/^(rgba?|linear-gradient|translate|scale|M\d|m\d)/.test(s)) return false;
  if (/^[A-Z_]+$/.test(s)) return false;                            // CONSTANTS
  if (/^(application|text|image|video|audio)\//.test(s)) return false;
  if (!/\s/.test(s) && /[a-z][A-Z]/.test(s)) return false;          // camelCase
  return true;
};

function fromHTML(html, out) {
  html = html.replace(/<style[\s\S]*?<\/style>/g, "").replace(/<svg[\s\S]*?<\/svg>/g, "");
  for (const m of html.matchAll(/(?:title|placeholder|aria-label)="([^"$]+)"/g)) out.add(m[1].trim());
  for (const m of html.matchAll(/>([^<>]+)</g)) {
    const t = m[1].replace(/\s+/g, " ").trim();
    if (t && !t.includes("${")) out.add(t);
  }
}

function fromJS(js, out) {
  // template literals: split into HTML-ish pieces with ${} -> placeholders
  for (const m of js.matchAll(/`((?:[^`\\]|\\.)*)`/g)) {
    const tpl = m[1];
    if (/<[a-z]/i.test(tpl)) {
      // markup: take text between tags and attributes
      let i = 0;
      const withPh = tpl.replace(/\$\{(?:[^{}]|\{[^{}]*\})*\}/g, () => `\u0001${i++}\u0002`);
      for (const mm of withPh.matchAll(/(?:title|placeholder|aria-label)="([^"]+)"/g)) add(mm[1]);
      for (const mm of withPh.matchAll(/>([^<>]+)</g)) add(mm[1]);
    } else {
      let i = 0;
      add(tpl.replace(/\$\{(?:[^{}]|\{[^{}]*\})*\}/g, () => `\u0001${i++}\u0002`));
    }
  }
  for (const m of js.matchAll(/"((?:[^"\\\n]|\\.)*)"|'((?:[^'\\\n]|\\.)*)'/g)) {
    const s = (m[1] ?? m[2]).replace(/\\(.)/g, "$1");
    if (/<[a-z]/i.test(s)) fromHTML(s, out); else add(s);
  }
  function add(s) {
    s = s.replace(/\s+/g, " ").trim();
    // renumber placeholders from {0}
    let n = 0;
    const map = {};
    s = s.replace(/\u0001(\d+)\u0002/g, (_, k) => `{${(map[k] ??= n++)}}`);
    if (looksLikeText(s.replace(/\{\d+\}/g, "").trim() || "")) out.add(s);
  }
}

function extract(files) {
  const out = new Set();
  for (const f of files) {
    const src = fs.readFileSync(f, "utf8");
    if (f.endsWith(".js")) fromJS(src, out);
    else {
      fromHTML(src.replace(/<script>[\s\S]*?<\/script>/g, ""), out);
      for (const m of src.matchAll(/<script>([\s\S]*?)<\/script>/g)) fromJS(m[1], out);
    }
  }
  // a string that is only placeholders/punctuation isn't translatable
  return [...out].filter((s) => /[A-Za-z]{2}/.test(s.replace(/\{\d+\}/g, ""))).sort();
}

if (require.main === module) {
  const all = process.argv.includes("--all");
  for (const [group, files] of Object.entries(SOURCES)) {
    const strings = extract(files);
    const keysFile = path.join("internal/i18n/locales", group, "keys.json");
    const keys = new Set(fs.existsSync(keysFile) ? JSON.parse(fs.readFileSync(keysFile, "utf8")) : []);
    const missing = all ? strings : strings.filter((s) => !keys.has(s));
    console.log(`## ${group}: ${missing.length} ${all ? "found" : "not in keys.json (code fragments are expected here)"}`);
    missing.forEach((s) => console.log("  " + s));
  }
}
module.exports = { extract, SOURCES };
