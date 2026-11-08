// The Chromium extension does not support YouTube downloads. Firefox is a
// separate package with a fixed Gecko manifest; there is no settings override.
(() => {
  const firefox = !!chrome.runtime.getManifest().browser_specific_settings?.gecko;
  const domains = ["youtube.com", "youtu.be", "youtube-nocookie.com", "googlevideo.com", "ytimg.com", "youtube.googleapis.com", "youtubei.googleapis.com"];
  function blocked(raw) {
    if (firefox || !raw) return false;
    try {
      let u = new URL(raw);
      if (u.protocol === "blob:") u = new URL(u.pathname);
      const host = u.hostname.toLowerCase().replace(/\.$/, "");
      return domains.some(d => host === d || host.endsWith("." + d));
    } catch { return false; }
  }
  globalThis.veloPolicy = Object.freeze({ blocked, message: "YouTube downloads are not supported by this browser extension." });
})();
