const { test } = require('node:test');
const assert = require('node:assert/strict');
const vm = require('node:vm');
const fs = require('node:fs');
const path = require('node:path');

function worker() {
  let listener, tab = { id: 1, url: 'https://www.youtube.com/watch?v=first', title: 'Test video' };
  const sent = [];
  let cached = [];
  const event = { addListener() {} };
  const chrome = {
    runtime: { onInstalled: event, onMessage: { addListener(fn) { listener = fn; } },
      async sendNativeMessage(host, message) { sent.push(message); return { ok: true }; } },
    downloads: { onDeterminingFilename: event }, contextMenus: { onClicked: event },
    webRequest: { onHeadersReceived: event },
    tabs: { async get() { return tab; }, onUpdated: event, onRemoved: event },
    cookies: { async getAll() { return []; } },
    storage: { session: { async get() { return { "media:1": { items: cached } }; } } },
  };
  const context = vm.createContext({ chrome, URL, atob, navigator: { userAgent: 'test' }, console });
  vm.runInContext(fs.readFileSync(path.join(__dirname, '../../extension/background.js'), 'utf8'), context);
  return {
    sent,
    cache(items) { cached = items; },
    classify(url) { return context.classify({ url }); },
    navigate(url) { tab = { ...tab, url }; },
    message(msg, content = false) { return new Promise(resolve => listener(msg, content ? { tab, url: tab.url } : {}, resolve)); },
  };
}

test('YouTube page extraction replaces intercepted fragments and follows SPA navigation', async () => {
  const w = worker();
  const first = await w.message({ type: 'media', tabId: 1 });
  assert.equal(first.unsupported, false);
  assert.equal(first.items[0].kind, 'page');
  w.navigate('https://www.youtube.com/shorts/second');
  const stale = await w.message({ type: 'grab', tabId: 1, url: first.items[0].url });
  assert.equal(stale.ok, false);
  const result = await w.message({ type: 'grab', tabId: 1 });
  assert.equal(result.ok, true);
  assert.equal(w.sent[0].url, 'https://www.youtube.com/shorts/second');
  assert.equal(w.sent[0].kind, 'page');
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
