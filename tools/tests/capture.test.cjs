const { test } = require('node:test');
const assert = require('node:assert/strict');
const vm = require('node:vm');
const fs = require('node:fs');
const path = require('node:path');

function worker(firefox, reply = { ok: true }) {
  const listeners = {}, sent = [], actions = [];
  const event = name => ({ addListener(fn) { listeners[name] = fn; } });
  const downloads = {
    onCreated: event('created'), onChanged: event('changed'),
    async pause(id) { actions.push(['pause', id]); },
    async resume(id) { actions.push(['resume', id]); },
    async cancel(id, callback) { actions.push(['cancel', id]); callback?.(); },
    async erase(item) { actions.push(['erase', item.id]); },
  };
  if (!firefox) downloads.onDeterminingFilename = event('filename');
  const chrome = {
    downloads,
    runtime: {
      onInstalled: event('installed'), onMessage: event('message'),
      async sendNativeMessage(host, message) { sent.push(message); return reply; },
    },
    storage: { local: { async get(defaults) { return defaults; } } },
    cookies: { async getAll() { return []; } },
    contextMenus: { onClicked: event('menu') },
    webRequest: { onHeadersReceived: event('headers') },
    tabs: { onUpdated: event('updated'), onRemoved: event('removed') },
  };
  const context = vm.createContext({ chrome, URL, navigator: { userAgent: 'test' }, console: { warn() {} } });
  vm.runInContext(fs.readFileSync(path.join(__dirname, '../../extension/background.js'), 'utf8'), context);
  return {
    sent, actions,
    async capture(filename, url = 'https://example.com/download?id=1') {
      const item = { id: 7, filename, url, state: 'in_progress', totalBytes: 1024 };
      if (firefox) await listeners.created(item);
      else listeners.filename(item, () => actions.push(['suggest']));
      // Drain async settings, cookie lookup and native-host replies.
      await new Promise(resolve => setImmediate(resolve));
    },
  };
}

for (const firefox of [true, false]) {
  const browser = firefox ? 'Firefox' : 'Chrome';
  for (const filename of ['linux.torrent', 'LINUX.TORRENT', 'Tool.AppImage', 'archive.zip']) {
    test(`${browser} captures ${filename} and requests the Velo dialog`, async () => {
      const w = worker(firefox);
      await w.capture(filename);
      assert.equal(w.sent.length, 1);
      assert.equal(w.sent[0].filename, filename);
      assert.equal(w.sent[0].source, 'capture');
      assert.equal(w.sent[0].url, 'https://example.com/download?id=1');
      assert.ok(w.actions.some(([action]) => action === 'cancel'));
      assert.ok(w.actions.some(([action]) => action === 'erase'));
      assert.ok(!w.actions.some(([action]) => action === 'resume' || action === 'suggest'));
    });
  }
  test(`${browser} keeps unsupported extensions in the browser`, async () => {
    const w = worker(firefox);
    await w.capture('file.unsupportedextension');
    assert.equal(w.sent.length, 0);
    assert.ok(w.actions.some(([action]) => action === (firefox ? 'resume' : 'suggest')));
  });
  test(`${browser} keeps the torrent when Velo is unavailable`, async () => {
    const w = worker(firefox, { ok: false, error: 'not running' });
    await w.capture('linux.torrent');
    assert.equal(w.sent.length, 1);
    assert.ok(!w.actions.some(([action]) => action === 'cancel'));
    assert.ok(w.actions.some(([action]) => action === (firefox ? 'resume' : 'suggest')));
  });
}
