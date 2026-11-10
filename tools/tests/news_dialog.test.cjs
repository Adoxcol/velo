const { test } = require('node:test');
const assert = require('node:assert/strict');
const vm = require('node:vm');
const fs = require('node:fs');
const path = require('node:path');

function uiWindow() {
  const elements = new Map();
  const getOrCreate = key => {
    if (!elements.has(key)) {
      elements.set(key, {
        disabled: false,
        hidden: false,
        value: '',
        textContent: '',
        title: '',
        innerHTML: '',
        className: '',
        dataset: { s: 'autostart' },
        attributes: {},
        style: {
          display: 'block',
          setProperty(k, v) { this[k] = v; },
        },
        classList: {
          toggle() {},
          add() {},
          remove() {},
        },
        querySelectorAll() { return []; },
        querySelector() { return null; },
        setAttribute(k, v) { this.attributes[k] = String(v); },
        getAttribute(k) { return this.attributes[k]; },
        removeAttribute(k) { delete this.attributes[k]; },
        closest() { return null; },
        addEventListener() {},
        contains() { return false; },
        close() { this.open = false; },
        showModal() { this.open = true; },
        focus() {},
        open: false,
      });
    }
    return elements.get(key);
  };

  const context = vm.createContext({
    document: {
      querySelector: sel => getOrCreate(sel),
      querySelectorAll: sel => (sel === "[data-s]" ? [] : [getOrCreate(sel)]),
      getElementById: id => getOrCreate('#' + id),
      documentElement: { dataset: {} },
      body: { classList: { toggle() {} } },
      addEventListener() {},
    },
    window: {
      addEventListener() {},
    },
    matchMedia: () => ({ matches: false, addEventListener() {} }),
    setInterval() {},
    setTimeout() {},
    clearTimeout() {},
    addEventListener() {},
    getSettings: () => Promise.resolve({ theme: 'light', onboarded: true, schedule: { days: [true] } }),
    defaultDir: async () => 'C:\\Downloads',
    languages: async () => ({ list: [] }),
    appStatus: async () => ({ rows: [], speed: [] }),
    appVersion: async () => '1.0.0',
    pendingReleaseNotes: async () => null,
    dismissReleaseNotes: async () => true,
    updateInfo: async () => ({ current: '1.0.0', status: 'Up to date' }),
    takeBatch: async () => null,
  });

  const html = fs.readFileSync(path.join(__dirname, '../../cmd/velo/ui.html'), 'utf8');
  vm.runInContext(html.match(/<script>([\s\S]*?)<\/script>/)[1], context);
  return { context, element: getOrCreate };
}

test('closeNews closes #newsDlg immediately before awaiting dismissReleaseNotes', async () => {
  const ui = uiWindow();
  let dismissOrder = -1;
  let closeOrder = -1;
  let step = 0;

  const newsDlg = ui.element('#newsDlg');
  newsDlg.open = true;
  newsDlg.close = () => {
    closeOrder = ++step;
    newsDlg.open = false;
  };

  ui.context.dismissReleaseNotes = async () => {
    // Artificial delay to simulate IPC / disk I/O
    await new Promise(r => setTimeout(r, 20));
    dismissOrder = ++step;
    return true;
  };

  await ui.context.closeNews();

  assert.equal(closeOrder, 1, 'dialog should close first before async dismiss');
  assert.equal(dismissOrder, 2, 'dismissReleaseNotes should resolve after dialog closes');
  assert.equal(newsDlg.open, false, 'dialog should be closed');
});
