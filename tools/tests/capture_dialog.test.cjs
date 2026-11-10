const { test } = require('node:test');
const assert = require('node:assert/strict');
const vm = require('node:vm');
const fs = require('node:fs');
const path = require('node:path');

function captureDialog() {
  const elements = new Map();
  const element = id => {
    if (!elements.has(id)) {
      elements.set(id, {
        disabled: false,
        hidden: false,
        value: '',
        placeholder: '',
        textContent: '',
        dataset: {},
        attributes: {},
        setAttribute(k, v) { this.attributes[k] = String(v); },
        getAttribute(k) { return this.attributes[k]; },
        removeAttribute(k) { delete this.attributes[k]; },
        focus() {},
        parentElement: { title: '' },
        querySelector() { return { hidden: false }; },
      });
    }
    return elements.get(id);
  };
  const context = vm.createContext({
    document: {
      getElementById: element,
      documentElement: { dataset: {} },
      addEventListener() {},
    },
    window: {},
    addEventListener() {},
    getSettings: () => Promise.resolve({ theme: 'light' }),
    nextPending: async () => null,
    pendingCount: async () => 0,
    folderFor: async () => 'C:\\Downloads',
    categoryOf: async () => 'Other',
    closeDialog: () => {},
  });
  const html = fs.readFileSync(path.join(__dirname, '../../cmd/velo/capture.html'), 'utf8');
  vm.runInContext(html.match(/<script>([\s\S]*?)<\/script>/)[1], context);
  return { context, element };
}

test('capture dialog sets contextual ARIA labels on action buttons for files', async () => {
  const d = captureDialog();
  d.context.nextPending = async () => ({
    id: 1,
    url: 'https://example.com/release-v2.0.zip',
    filename: 'release-v2.0.zip',
    size: 50000000,
  });
  await d.context.load();

  assert.equal(d.element('start').getAttribute('aria-label'), 'Start download for release-v2.0.zip');
  assert.equal(d.element('later').getAttribute('aria-label'), 'Download release-v2.0.zip later');
  assert.equal(d.element('cancel').getAttribute('aria-label'), 'Cancel download for release-v2.0.zip');
});

test('capture dialog updates contextual ARIA labels when renaming file input', async () => {
  const d = captureDialog();
  d.context.nextPending = async () => ({
    id: 2,
    url: 'https://example.com/video.mp4',
    filename: 'video.mp4',
    size: 2000000,
  });
  await d.context.load();

  assert.equal(d.element('start').getAttribute('aria-label'), 'Start download for video.mp4');

  d.element('name').value = 'my-vacation-video.mp4';
  d.element('name').oninput();
  await new Promise(resolve => setImmediate(resolve));

  assert.equal(d.element('start').getAttribute('aria-label'), 'Start download for my-vacation-video.mp4');
  assert.equal(d.element('later').getAttribute('aria-label'), 'Download my-vacation-video.mp4 later');
  assert.equal(d.element('cancel').getAttribute('aria-label'), 'Cancel download for my-vacation-video.mp4');
});

test('capture dialog uses fallback label when name is empty for page captures', async () => {
  const d = captureDialog();
  d.context.nextPending = async () => ({
    id: 3,
    url: 'https://example.com/watch?v=123',
    kind: 'page',
    size: 0,
  });
  await d.context.load();

  assert.equal(d.element('start').getAttribute('aria-label'), 'Start download for video');
  assert.equal(d.element('later').getAttribute('aria-label'), 'Download video later');
  assert.equal(d.element('cancel').getAttribute('aria-label'), 'Cancel download for video');
});
