const { test } = require('node:test');
const assert = require('node:assert/strict');
const vm = require('node:vm');
const fs = require('node:fs');
const path = require('node:path');

function miniWindow() {
  const elements = new Map();
  const element = selector => {
    const id = selector.startsWith('#') ? selector.slice(1) : selector;
    if (!elements.has(id)) {
      elements.set(id, {
        disabled: false,
        hidden: false,
        textContent: '',
        title: '',
        innerHTML: '',
        className: '',
        dataset: {},
        attributes: {},
        style: {
          setProperty(k, v) { this[k] = v; },
        },
        classList: {
          toggle(c, force) {},
          add(c) {},
          remove(c) {},
        },
        querySelectorAll() { return []; },
        setAttribute(k, v) { this.attributes[k] = String(v); },
        getAttribute(k) { return this.attributes[k]; },
        removeAttribute(k) { delete this.attributes[k]; },
        replaceChildren() {},
        addEventListener() {},
      });
    }
    return elements.get(id);
  };
  const context = vm.createContext({
    document: {
      querySelector: element,
      getElementById: id => element('#' + id),
      documentElement: { dataset: {} },
      body: { classList: { toggle() {} } },
      addEventListener() {},
    },
    window: {},
    setInterval() {},
    addEventListener() {},
    getSettings: () => Promise.resolve({ theme: 'light' }),
    appStatus: async () => ({ rows: [] }),
    pause: async () => {},
    hideMini: () => {},
    openMain: () => {},
    dragMini: () => {},
    miniMoved: () => {},
  });
  const html = fs.readFileSync(path.join(__dirname, '../../cmd/velo/mini.html'), 'utf8');
  vm.runInContext(html.match(/<script>([\s\S]*?)<\/script>/)[1], context);
  return { context, element };
}

test('mini sets contextual ARIA label on pause button when downloading', async () => {
  const m = miniWindow();
  m.context.appStatus = async () => ({
    rows: [
      {
        id: 1,
        status: 'downloading',
        file: 'test-archive.zip',
        percent: 50,
        total: 1000,
        done: 500,
        speed: 100,
        category: 'Compressed',
      },
    ],
  });
  await m.context.refresh();

  assert.equal(m.element('#pp').getAttribute('aria-label'), 'Pause test-archive.zip');
  assert.equal(m.element('#pp').title, 'Pause');
});

test('mini sets contextual ARIA label on action button when completed', async () => {
  const m = miniWindow();
  m.context.appStatus = async () => ({
    rows: [
      {
        id: 2,
        status: 'done',
        file: 'document.pdf',
        percent: 100,
        total: 2000,
        done: 2000,
        category: 'Documents',
      },
    ],
  });
  await m.context.refresh();

  assert.equal(m.element('#pp').getAttribute('aria-label'), 'Done document.pdf');
  assert.equal(m.element('#pp').title, 'Done');
});
