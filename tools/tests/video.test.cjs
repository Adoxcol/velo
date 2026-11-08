const { test } = require('node:test');
const assert = require('node:assert/strict');
const vm = require('node:vm');
const fs = require('node:fs');
const path = require('node:path');

function worker(firefox = true) {
  let listener, tab = { id: 1, url: 'https://www.youtube.com/watch?v=first', title: 'Test video' };
  const sent = [];
  let cached = [];
  const event = { addListener() {} };
  const chrome = {
    runtime: { getManifest: () => firefox ? { browser_specific_settings: { gecko: { id: 'velo@test' } } } : {}, onInstalled: event, onMessage: { addListener(fn) { listener = fn; } },
      async sendNativeMessage(host, message) { sent.push(message); return { ok: true }; } },
    downloads: { onDeterminingFilename: event }, contextMenus: { onClicked: event },
    webRequest: { onHeadersReceived: event },
    tabs: { async get() { return tab; }, async sendMessage() {}, onUpdated: event, onRemoved: event },
    cookies: { async getAll() { return []; } },
    storage: { session: { async get() { return { "media:1": { items: cached } }; } } },
  };
  const context = vm.createContext({ chrome, URL, atob, navigator: { userAgent: 'test' }, console });
  vm.runInContext(fs.readFileSync(path.join(__dirname, '../../extension/policy.js'), 'utf8'), context);
  if (firefox) vm.runInContext(fs.readFileSync(path.join(__dirname, '../../extension/youtube.js'), 'utf8'), context); // Firefox build only
  vm.runInContext(fs.readFileSync(path.join(__dirname, '../../extension/background.js'), 'utf8'), context);
  return {
    sent,
    send: msg => context.send(msg),
    menu: (info, tab) => context.menuDownload(info, tab),
    cache(items) { cached = items; },
    classify(url) { return context.classify({ url }); },
    navigate(url) { tab = { ...tab, url }; },
    message(msg, content = false) { return new Promise(resolve => listener(msg, content ? { tab, url: tab.url } : {}, resolve)); },
  };
}

test('Chrome rejects YouTube pages, embeds, media URLs and old cached entries', async () => {
  const w = worker(false);
  for (const url of ['https://www.youtube.com/watch?v=abc', 'https://youtu.be/abc', 'https://www.youtube-nocookie.com/embed/abc', 'https://rr1.googlevideo.com/videoplayback?id=abc']) {
    w.navigate(url);
    const media = await w.message({ type: 'media', tabId: 1 });
    assert.equal(media.unsupported, true);
    assert.equal(media.items.length, 0);
    assert.equal((await w.message({ type: 'grab', tabId: 1, url })).ok, false);
    assert.equal((await w.send({ type: 'download', url })).ok, false);
    assert.equal(w.classify(url), null);
  }
  w.navigate('https://example.com/article');
  w.cache([{ url: 'https://rr1.googlevideo.com/videoplayback', kind: 'file' }]);
  assert.equal((await w.message({ type: 'media', tabId: 1 })).items.length, 0);
  assert.equal(w.sent.length, 0);
});

test('Chrome filters batch links and blocks YouTube referrers at native boundary', async () => {
  const w = worker(false);
  const file = { url: 'https://example.com/file.zip', text: 'file' };
  await w.send({ type: 'batch', page: 'https://example.com', links: [file, { url: 'https://youtube.com/watch?v=abc' }] });
  assert.equal(w.sent.length, 1);
  assert.equal(w.sent[0].links.length, 1);
  assert.equal(w.sent[0].links[0].url, file.url);
  assert.equal((await w.send({ type: 'download', url: file.url, referrer: 'https://youtube.com/watch?v=abc' })).ok, false);
  assert.equal(w.sent.length, 1);
});

test('Chrome context menus reject YouTube links on other sites', async () => {
  const w = worker(false);
  await w.menu({ menuItemId: 'velo-link', linkUrl: 'https://youtu.be/abc' }, { id: 1, url: 'https://example.com' });
  await w.menu({ menuItemId: 'velo-video', mediaType: 'video', srcUrl: 'https://rr1.googlevideo.com/video.mp4' }, { id: 1, url: 'https://example.com' });
  assert.equal(w.sent.length, 0);
});

test('Chrome still routes supported non-YouTube video pages', async () => {
  const w = worker(false);
  w.navigate('https://www.facebook.com/reel/123');
  const media = await w.message({ type: 'media', tabId: 1 });
  assert.equal(media.items[0].kind, 'page');
  assert.equal((await w.message({ type: 'grab', tabId: 1 })).ok, true);
  assert.equal(w.sent.length, 1);
});

test('YouTube page extraction replaces intercepted fragments and follows SPA navigation', async () => {
  const w = worker();
  const first = await w.message({ type: 'media', tabId: 1 });
  assert.equal(first.unsupported, false);
  assert.equal(first.items[0].kind, 'page');
  w.navigate('https://www.youtube.com/shorts/second');
  const stale = await w.message({ type: 'grab', tabId: 1, url: first.items[0].url });
  assert.equal(stale.ok, false);
  const result = await w.message({ type: 'grab', tabId: 1, format: '720p' });
  assert.equal(result.ok, true);
  assert.equal(w.sent[0].url, 'https://www.youtube.com/shorts/second');
  assert.equal(w.sent[0].kind, 'page');
  assert.equal(w.sent[0].format, '720p');
  assert.equal(w.sent[0].source, 'capture');
  assert.equal(w.sent[0].title, '');
});

test('MP4 selection reaches the desktop rename dialog without a stale tab title', async () => {
  const w = worker();
  assert.equal((await w.message({ type: 'grab', tabId: 1, format: 'mp4' })).ok, true);
  assert.equal(w.sent[0].format, 'mp4');
  assert.equal(w.sent[0].source, 'capture');
  assert.equal(w.sent[0].filename, '');
  assert.equal(w.sent[0].title, '');
});

test('feed hover selects a same-origin post and rejects unrelated extraction URLs', async () => {
  const w = worker();
  w.navigate('https://x.com/home');
  const url = 'https://x.com/person/status/123';
  const media = await w.message({ type: 'media', pageUrl: url }, true);
  assert.equal(media.items[0].url, url);
  assert.equal((await w.message({ type: 'grab', url }, true)).ok, true);
  assert.equal(w.sent[0].url, url);
  assert.equal((await w.message({ type: 'grab', url: 'https://youtube.com/watch?v=bad' }, true)).ok, false);
  w.navigate('https://youtube.com.evil.test/watch?v=bad');
  assert.equal((await w.message({ type: 'media', tabId: 1 })).items.length, 0);
});

test('Facebook CDN byte fragments resolve to video pages, including old cached entries', async () => {
  const w = worker();
  w.navigate('https://www.facebook.com/');
  const efg = Buffer.from(JSON.stringify({ video_id: 1407376671570396 })).toString('base64');
  const fragment = 'https://scontent.example.fbcdn.net/video.mp4?bytestart=169116&byteend=832276&efg=' + encodeURIComponent(efg);
  assert.equal(w.classify(fragment), 'page');
  assert.equal(w.classify('https://cdn.example/video.mp4?bytestart=100'), null);
  assert.equal(w.classify('https://fbcdn.net.evil.test/video.mp4?bytestart=100&efg=' + efg), null);
  assert.equal(w.classify('https://cdn.example/video.mp4'), 'file');
  w.cache([{ url: fragment, kind: 'file' }, { url: fragment, kind: 'file' }]);
  const media = await w.message({ type: 'media', tabId: 1 });
  assert.equal(media.items.length, 1);
  assert.equal(media.items[0].url, 'https://www.facebook.com/watch/?v=1407376671570396');
  assert.equal(media.items[0].kind, 'page');
  assert.equal((await w.message({ type: 'grab', tabId: 1 })).ok, true);
  assert.equal(w.sent[0].kind, 'page');
  assert.equal(w.sent[0].url, media.items[0].url);
});
