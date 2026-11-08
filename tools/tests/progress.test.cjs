const { test } = require('node:test');
const assert = require('node:assert/strict');
const vm = require('node:vm');
const fs = require('node:fs');
const path = require('node:path');

function progressWindow() {
  const elements = new Map();
  const element = id => {
    if (!elements.has(id)) {
      elements.set(id, {
        disabled: false,
        hidden: false,
        textContent: '',
        title: '',
        className: '',
        dataset: {},
        attributes: {},
        style: {},
        classList: {
          add(c) {},
          remove(c) {},
        },
        querySelectorAll() { return []; },
        setAttribute(k, v) { this.attributes[k] = String(v); },
        getAttribute(k) { return this.attributes[k]; },
        removeAttribute(k) { delete this.attributes[k]; },
      });
    }
    return elements.get(id);
  };
  const context = vm.createContext({
    document: {
      getElementById: element,
      title: '',
      documentElement: { dataset: {} },
      addEventListener() {},
    },
    window: {},
    setInterval() {},
    setTimeout: (fn, ms) => setTimeout(fn, ms),
    clearTimeout: id => clearTimeout(id),
    getSettings: () => new Promise(() => {}),
    progressInfo: async () => null,
    fileTypeIcon: async () => ({}),
    veloSetHTML: () => {},
    closeProgress: () => {},
  });
  const html = fs.readFileSync(path.join(__dirname, '../../cmd/velo/progress.html'), 'utf8');
  vm.runInContext(html.match(/<script>([\s\S]*?)<\/script>/)[1], context);
  return { context, element };
}

test('progress sets contextual ARIA labels on pause, cancel, and hide buttons', async () => {
  const p = progressWindow();
  p.context.progressInfo = async () => ({
    status: 'downloading',
    file: 'archive.tar.gz',
    url: 'https://example.com/archive.tar.gz',
    percent: 42,
    total: 1000,
    done: 420,
    speed: 50,
  });
  await p.context.tick();

  assert.equal(p.element('pause').getAttribute('aria-label'), 'Pause archive.tar.gz');
  assert.equal(p.element('cancel').getAttribute('aria-label'), 'Cancel archive.tar.gz');
  assert.equal(p.element('hide').getAttribute('aria-label'), 'Hide progress for archive.tar.gz');
});

test('progress updates pause button ARIA label when paused or in error', async () => {
  const p = progressWindow();
  p.context.progressInfo = async () => ({
    status: 'paused',
    file: 'video.mp4',
    url: 'https://example.com/video.mp4',
    percent: 10,
    total: 2000,
    done: 200,
  });
  await p.context.tick();
  assert.equal(p.element('pause').getAttribute('aria-label'), 'Resume video.mp4');

  p.context.progressInfo = async () => ({
    status: 'error',
    file: 'video.mp4',
    url: 'https://example.com/video.mp4',
    error: 'network error',
    percent: 10,
    total: 2000,
    done: 200,
  });
  await p.context.tick();
  assert.equal(p.element('pause').getAttribute('aria-label'), 'Retry video.mp4');
});

test('cancel button includes file context in ARIA label during confirmation', async () => {
  const p = progressWindow();
  p.context.progressInfo = async () => ({
    status: 'downloading',
    file: 'setup.exe',
    url: 'https://example.com/setup.exe',
    percent: 75,
    total: 1000,
    done: 750,
  });
  await p.context.tick();

  assert.equal(p.element('cancel').getAttribute('aria-label'), 'Cancel setup.exe');

  // Trigger cancel confirmation
  await p.element('cancel').onclick();
  assert.equal(
    p.element('cancel').getAttribute('aria-label'),
    'Delete partial file for setup.exe? Click again to confirm'
  );
});
