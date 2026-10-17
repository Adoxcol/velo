// Prints the GitHub release notes for a version: its CHANGELOG.md section
// plus the download table. Used by .github/workflows/release.yml.
//
//   node tools/release-notes.js 0.4.1 > notes.md
const fs = require("fs");

const version = process.argv[2];
if (!/^\d+\.\d+\.\d+$/.test(version || "")) {
  console.error("usage: node tools/release-notes.js <version>");
  process.exit(2);
}
const changelog = fs.readFileSync("CHANGELOG.md", "utf8");
const start = changelog.search(new RegExp(`^## ${version.replace(/\./g, "\\.")}\\b.*$`, "m"));
if (start < 0) {
  console.error(`CHANGELOG.md has no "## ${version}" section`);
  process.exit(1);
}
const rest = changelog.slice(start);
const next = rest.slice(3).search(/^## /m);
const section = (next < 0 ? rest : rest.slice(0, next + 3))
  .replace(/^## .*\r?\n/, "") // the heading is the release title
  .replace(/^### /gm, "#### ")
  .trim();

process.stdout.write(`## What's new in ${version}

${section}

Already using Velo? It updates itself when nothing is downloading, or click **Check for updates** in Settings → Advanced.

## Download

| File | What it is |
|---|---|
| **Velo-Setup-${version}.exe** | Installer (no administrator rights). Connects Chrome, Edge, Brave and Firefox. |
| **Velo-Portable-${version}.zip** | No install: unzip anywhere and run \`velo.exe\`. |
| velo-extension-${version}.zip | Extension package for the Chrome Web Store and Edge Add-ons. |
| velo-firefox-${version}.zip | Add-on package for Firefox Add-ons. |
| veloctl-${version}-linux-amd64.tar.gz / linux-arm64.tar.gz | Standalone Linux command-line downloader. |
| veloctl-${version}-darwin-amd64.tar.gz / darwin-arm64.tar.gz | Standalone macOS command-line downloader (Intel / Apple Silicon). |

Checksums are in \`SHA256SUMS-${version}.txt\` and \`SHA256SUMS-veloctl-linux.txt\` / \`SHA256SUMS-veloctl-darwin.txt\`. Full history: https://velo.adoxcol.com/changelog
`);
