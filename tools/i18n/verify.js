// Comprehensive localization verification script for Velo.
// Run from repo root: node tools/i18n/verify.js
const fs = require("fs");
const path = require("path");

const tags = (s) => (s.match(/<\/?[a-z]+[^>]*>/g) || []).map((t) => t.replace(/\s.*>/, ">")).sort().join("");
const holes = (s) => (s.match(/\{\d\}/g) || []).sort().join("");
let failed = false;

function verifyGroup(group) {
  const keysFile = `internal/i18n/locales/${group}/keys.json`;
  if (!fs.existsSync(keysFile)) {
    console.error(`Missing keys file: ${keysFile}`);
    failed = true;
    return;
  }
  const keys = JSON.parse(fs.readFileSync(keysFile, "utf8"));
  const srcDir = `tools/i18n/${group}`;
  const files = fs.readdirSync(srcDir).filter((f) => f.endsWith(".json"));

  console.log(`Verifying ${group} (${keys.length} keys across ${files.length} languages)...`);

  for (const f of files) {
    const lang = f.replace(/\.json$/, "");
    const filePath = path.join(srcDir, f);
    const list = JSON.parse(fs.readFileSync(filePath, "utf8"));

    if (list.length !== keys.length) {
      console.error(`[FAIL] ${group}/${lang}: length is ${list.length}, expected ${keys.length}`);
      failed = true;
      continue;
    }

    list.forEach((v, i) => {
      const k = keys[i];
      if (typeof v !== "string" || !v.trim()) {
        console.error(`[FAIL] ${group}/${lang} #${i + 1} empty: ${k}`);
        failed = true;
        return;
      }
      if (holes(k) !== holes(v)) {
        console.error(`[FAIL] ${group}/${lang} #${i + 1} placeholders differ:\n  Source: ${k}\n  Target: ${v}`);
        failed = true;
      }
      if (tags(k) !== tags(v)) {
        console.error(`[FAIL] ${group}/${lang} #${i + 1} HTML tags differ:\n  Source: ${k}\n  Target: ${v}`);
        failed = true;
      }
    });
  }
}

verifyGroup("app");
verifyGroup("ext");

// Check language parity between app and ext groups
const appLangs = fs.readdirSync("tools/i18n/app").filter(f => f.endsWith(".json")).map(f => f.replace(".json", "")).sort();
const extLangs = fs.readdirSync("tools/i18n/ext").filter(f => f.endsWith(".json")).map(f => f.replace(".json", "")).sort();

if (JSON.stringify(appLangs) !== JSON.stringify(extLangs)) {
  console.error(`[FAIL] Mismatch between app languages [${appLangs.join(", ")}] and ext languages [${extLangs.join(", ")}]`);
  failed = true;
}

// Check Go i18n Languages list parity
const i18nGo = fs.readFileSync("internal/i18n/i18n.go", "utf8");
appLangs.forEach(lang => {
  if (lang !== "en" && !i18nGo.includes(`"${lang}"`)) {
    console.error(`[FAIL] Language "${lang}" is in tools/i18n/app but not registered in internal/i18n/i18n.go`);
    failed = true;
  }
});

// Check built locale json files in internal/i18n/locales
["app", "ext"].forEach(group => {
  const keys = JSON.parse(fs.readFileSync(`internal/i18n/locales/${group}/keys.json`, "utf8"));
  const langs = group === "app" ? appLangs : extLangs;
  langs.forEach(lang => {
    const builtFile = `internal/i18n/locales/${group}/${lang}.json`;
    if (!fs.existsSync(builtFile)) {
      console.error(`[FAIL] Missing compiled locale file: ${builtFile}`);
      failed = true;
      return;
    }
    const dict = JSON.parse(fs.readFileSync(builtFile, "utf8"));
    if (Object.keys(dict).length !== keys.length) {
      console.error(`[FAIL] ${builtFile} compiled count (${Object.keys(dict).length}) does not match key count (${keys.length})`);
      failed = true;
    }
  });
});

// Check built extension i18n bundle
if (!fs.existsSync("extension/i18n.js")) {
  console.error("[FAIL] Missing extension/i18n.js bundle");
  failed = true;
}

if (failed) {
  console.error("Localization verification failed.");
  process.exitCode = 1;
} else {
  console.log("All localization checks passed successfully.");
}
