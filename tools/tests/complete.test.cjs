const { test } = require('node:test');
const assert = require('node:assert/strict');
const vm = require('node:vm');
const fs = require('node:fs');
const path = require('node:path');

function dialog() {
  const elements = new Map();
  const calls = [];
  const element = id => {
    if (!elements.has(id)) elements.set(id, { disabled: false, checked: false, textContent: '' });
    return elements.get(id);
  };
  const context = vm.createContext({
    document: { getElementById: element, addEventListener() {} }, window: {},
    getSettings: () => new Promise(() => {}),
    closeComplete: async never => { calls.push(['close', never]); },
  });
  const html = fs.readFileSync(path.join(__dirname, '../../cmd/velo/complete.html'), 'utf8');
  vm.runInContext(html.match(/<script>([\s\S]*?)<\/script>/)[1], context);
  vm.runInContext('cur = { id: 41 };', context);
  return { context, calls, element };
}

test('completion waits for action acceptance before destroying its WebView', async () => {
  for (const action of ['open', 'with', 'folder']) {
    const d = dialog();
    let accept;
    const launch = id => {
      d.calls.push(['launch', id]);
      return new Promise(resolve => { accept = resolve; });
    };
    d.context.openDone = d.context.openDoneWith = d.context.showDoneInFolder = launch;
    d.element('never').checked = true;
    const pending = d.element(action).onclick();
    await d.element(action).onclick(); // ignore a second click while pending
    assert.deepEqual(d.calls, [['launch', 41]]);
    assert.equal(d.element(action).disabled, true);
    accept();
    await pending;
    assert.deepEqual(d.calls, [['launch', 41], ['close', true]]);
  }
});

test('failed completion actions keep the dialog usable and display the error', async () => {
  const d = dialog();
  d.element('open').disabled = true;
  d.context.showDoneInFolder = async () => { throw new Error('no such download'); };
  await d.element('folder').onclick();
  assert.deepEqual(d.calls, []);
  assert.equal(d.element('err').textContent, 'no such download');
  assert.equal(d.element('folder').disabled, false);
  assert.equal(d.element('open').disabled, true);
  d.context.showDoneInFolder = async () => {};
  await d.element('folder').onclick();
  assert.deepEqual(d.calls, [['close', false]]);
});

test('an older action cannot dismiss a newer completed download', async () => {
  const d = dialog();
  let accept;
  d.context.openDone = () => new Promise(resolve => { accept = resolve; });
  const pending = d.element('open').onclick();
  vm.runInContext('++loadVersion; cur = {id: 42, exists: false};', d.context);
  accept();
  await pending;
  assert.deepEqual(d.calls, []);
  assert.equal(d.element('open').disabled, true);
  assert.equal(d.element('folder').disabled, false);
});

test('Close displays a settings save failure instead of silently failing', async () => {
  const d = dialog();
  d.context.closeComplete = async () => { throw new Error('settings could not be saved'); };
  await d.element('close').onclick();
  assert.equal(d.element('err').textContent, 'settings could not be saved');
});
