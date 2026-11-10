// Dependency-free checks for async feedback and metric calculations.
const assert = require('node:assert/strict');
const fs = require('node:fs');
const path = require('node:path');
const vm = require('node:vm');

class Element {
  constructor(tag = 'div') {
    this.tagName = tag.toUpperCase(); this.childNodes = []; this.listeners = {};
    this.attributes = {}; this.dataset = {}; this.style = {}; this.disabled = false;
    this.isConnected = true; this.className = ''; this.parentNode = null;
    this.nodeType = 1;
    this.classList = { add: (c) => { this.className += ' ' + c; }, remove: (c) => { this.className = this.className.split(' ').filter((x) => x !== c).join(' '); } };
  }
  set innerHTML(text) { this.childNodes = [{ nodeType: 3, textContent: text }]; }
  get innerHTML() { return this.textContent; }
  get textContent() { return this.childNodes.map((c) => c.textContent || '').join(''); }
  set textContent(value) { this.childNodes = [{ nodeType: 3, textContent: String(value) }]; }
  append(...children) { for (let child of children) { if (typeof child === 'string') child = { nodeType: 3, textContent: child }; child.parentNode = this; this.childNodes.push(child); } }
  appendChild(c) { this.append(c); }
  replaceChildren(...children) { this.childNodes = []; this.append(...children); }
  setAttribute(k, v) { this.attributes[k] = v; }
  removeAttribute(k) { delete this.attributes[k]; }
  hasAttribute(k) { return k in this.attributes; }
  getAttribute(k) { return this.attributes[k]; }
  addEventListener(k, fn) { this.listeners[k] = fn; }
  insertAdjacentHTML(_, text) { this.append({ nodeType: 3, textContent: text }); }
  closest() { return null; }
  querySelector() { return null; }
  after() {}
  remove() { this.isConnected = false; }
  focus() { this.focused = true; }
}

const root = new Element();
const document = {
  createElement: (tag) => new Element(tag), createTextNode: (text) => ({ nodeType: 3, textContent: String(text) }),
  getElementById: () => root, body: root, documentElement: root,
};
const context = vm.createContext({ document, console, setTimeout, clearTimeout, setInterval, clearInterval,
  window: { devicePixelRatio: 1 }, Intl, URL, FormData, Promise, WeakMap,
  requestAnimationFrame: (callback) => callback(), location: { hash: '#/' },
});
for (const file of ['ui.js', 'app.js']) {
  const source = fs.readFileSync(path.join(__dirname, '../../web/js', file), 'utf8').replace(/boot\(\);\s*$/, '');
  vm.runInContext(source, context, { filename: file });
}
vm.runInContext('toast = (message) => { globalThis.lastToast = message; };', context);
const { h, chart, metricRates, runAction, renderMemory, githubActionsWorkflow } = vm.runInContext('({ h, chart, metricRates, runAction, renderMemory, githubActionsWorkflow })', context);

async function main() {
  let resolve; let calls = 0;
  const pending = new Promise((r) => { resolve = r; });
  const button = h('button', { onclick: async (event) => { calls++; await pending; assert.equal(event.currentTarget, button); } }, 'Сохранить');
  const event = { currentTarget: button, target: button, preventDefault() {} };
  const action = button.listeners.click(event);
  assert.equal(button.disabled, true);
  assert.equal(button.getAttribute('aria-busy'), 'true');
  assert.match(button.textContent, /Сохраняем/);
  button.listeners.click(event);
  assert.equal(calls, 1, 'double click started another operation');
  event.currentTarget = null; // Native events clear currentTarget after dispatch.
  resolve(); await action;
  assert.equal(button.disabled, false);
  assert.equal(button.textContent, 'Сохранить');
  assert.equal(button.hasAttribute('aria-busy'), false);
  await runAction(button, async () => { throw new Error('network error'); });
  assert.equal(button.disabled, false);
  assert.equal(context.lastToast, 'network error');
  const workflow = githubActionsWorkflow(10);
  assert.match(workflow, /FASTHOST_PROJECT_ID: '10'/);
  assert.match(workflow, /FASTHOST_API_KEY: \$\{\{ secrets.FASTHOST_API_KEY \}\}/);
  assert.match(workflow, /api\/deploy\/\$FASTHOST_PROJECT_ID/);
  assert.match(workflow, /php-version: '8.4'/);
  assert.match(workflow, /xargs -0 -r -n1 php -l/);
  assert.match(workflow, /"\.env" "\.env\.\*"/);
  assert.match(workflow, /"\*\.sqlite"/);
  assert.match(githubActionsWorkflow('10; echo unsafe'), /<ID_ПРОЕКТА>/);
  assert.ok(workflow.split('\n').every((line) => !/\\\\$/.test(line)), 'shell continuation contains two backslashes');

  const checkbox = h('input', { type: 'checkbox', onchange: async () => Promise.resolve() });
  await checkbox.listeners.change({ target: checkbox, preventDefault() {} });
  assert.equal(checkbox.disabled, false);

  const rates = metricRates([{ timestamp_ms: 0, requests_cum: 5 }, { timestamp_ms: 30000, requests_cum: 11 }, { timestamp_ms: 150000, requests_cum: 15 }]);
  assert.deepEqual(Array.from(rates), [12, 2]);
  assert.equal(metricRates([{ timestamp_ms: 0, requests_cum: 10 }, { timestamp_ms: 0, requests_cum: 11 }])[0], null);
  assert.equal(metricRates([{ timestamp_ms: 0, requests_cum: 10 }, { timestamp_ms: 1000, requests_cum: 1 }])[0], null);
  let dots = 0;
  const drawing = new Proxy({ arc() { dots++; } }, { get: (target, key) => key in target ? target[key] : () => {} });
  chart({ style: {}, clientWidth: 400, getContext: () => drawing }, ['12:00'], [[512]], {});
  assert.equal(dots, 1, 'single sample must be visible');
  const memory = new Element();
  renderMemory(memory, { available: true, total_bytes: 1024, used_bytes: 512, available_bytes: 512, swap_used_bytes: 0, swap_total_bytes: 0, sampled_at: '2026-11-10T12:00:00Z' });
  assert.match(memory.textContent, /RAM/);
  renderMemory(memory, { available: false, error: 'no data' });
  assert.equal(memory.textContent, 'no data');
  vm.runInContext(fs.readFileSync(path.join(__dirname, '../../web/js/api.js'), 'utf8'), context);
  const api = vm.runInContext('api', context);
  context.fetch = async () => ({ status: 500, ok: false, json: async () => ({ error: 'server failure' }) });
  await assert.rejects(api.get('/failure'), /server failure/);
  assert.equal(vm.runInContext('activeRequests', context), 0, 'failed request left loading indicator active');
  context.fetch = async () => ({ status: 200, ok: true, json: async () => ({ ready: true }) });
  assert.equal((await api.get('/success')).ready, true);
  assert.equal(vm.runInContext('activeRequests', context), 0);
  const settingsPage = new Element();
  const walk = (node) => [node, ...(node.childNodes || []).flatMap(walk)];
  document.getElementById = (id) => walk(settingsPage).find((node) => node.attributes && node.attributes.id === id);
  api.get = async (url) => url === '/api/projects'
    ? { projects: [{ id: 10, name: 'Example', domain: 'example.test' }] }
    : { api_key: 'test-private-key' };
  let clipboard = '';
  context.navigator = { clipboard: { writeText: async (text) => { clipboard = text; } } };
  await vm.runInContext('renderSettings', context)(settingsPage);
  assert.match(settingsPage.textContent, /Автоматический деплой/);
  assert.match(settingsPage.textContent, /Безопасность аккаунта/);
  const select = document.getElementById('workflow-project');
  assert.equal(select.disabled, false);
  select.value = '10'; select.listeners.change();
  assert.match(document.getElementById('workflow-code').textContent, /FASTHOST_PROJECT_ID: '10'/);
  const copy = walk(settingsPage).find((node) => node.tagName === 'BUTTON' && node.textContent.includes('Копировать YAML'));
  await copy.listeners.click({ target: copy, preventDefault() {} });
  assert.match(clipboard, /secrets.FASTHOST_API_KEY/);
  assert.ok(!clipboard.includes('test-private-key'), 'workflow leaked the actual API key');
  let passwordWrites = 0;
  api.put = async () => { passwordWrites++; return {}; };
  const currentPassword = document.getElementById('pw-cur'); currentPassword.value = 'current';
  const newPassword = document.getElementById('pw-new'); newPassword.value = 'new-password';
  const confirmation = document.getElementById('pw-confirm'); confirmation.value = 'wrong-password';
  const savePassword = walk(settingsPage).find((node) => node.tagName === 'BUTTON' && node.textContent === 'Сменить пароль');
  await savePassword.listeners.click({ target: savePassword, preventDefault() {} });
  assert.equal(passwordWrites, 0);
  confirmation.value = newPassword.value;
  await savePassword.listeners.click({ target: savePassword, preventDefault() {} });
  assert.equal(passwordWrites, 1);
  assert.equal(newPassword.value, '');
  assert.equal(confirmation.value, '');
  console.log('PASS: async spinner/restore, duplicate click, stable event, errors, checkbox, interval rates, one-point chart, RAM states');
}
main().catch((error) => { console.error(error); process.exitCode = 1; });
