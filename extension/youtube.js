// YouTube video pages, for the Firefox build and the personal Chrome build
// (tools/packext -browser personal). Not in the Chrome Web Store package.
globalThis.veloYouTube = Object.freeze({
  videoPage(u) {
    const on = (domain) => u.hostname === domain || u.hostname.endsWith("." + domain);
    const p = u.pathname.split("/").filter(Boolean);
    if (on("youtu.be") && p.length === 1) return true;
    return (on("youtube.com") || on("youtube-nocookie.com")) &&
      ((u.pathname === "/watch" && !!u.searchParams.get("v")) ||
       (p.length === 2 && ["shorts", "embed", "live"].includes(p[0])));
  },
});
