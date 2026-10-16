const { test } = require('node:test');
const assert = require('node:assert/strict');
const vm = require('node:vm');
const fs = require('node:fs');
const path = require('node:path');

function worker() {
  let listener, tab = { id: 1, url: 'https://www.youtube.com/watch?v=first', title: 'Test video' };
  const sent = [];
  const event = { addListener() {} };
  const chrome = {
    runtime: { onInstalled: event, onMessage: { addListener(fn) { listener = fn; } },
      async sendNativeMessage(host, message) { sent.push(message); return { ok: true }; } },
    downloads: { onDeterminingFilename: event }, contextMenus: { onClicked: event },
    webRequest: { onHeadersReceived: event },
    tabs: { async get() { return tab; }, onUpdated: event, onRemoved: event },
    cookies: { async getAll() { return []; } },
    storage: { session: { async get() { return {}; } } },
  };
  vm.runInNewContext(fs.readFileSync(path.join(__dirname, '../../extension/background.js'), 'utf8'),
    { chrome, URL, navigator: { userAgent: 'test' }, console });
  return {
    sent,
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
