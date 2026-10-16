// smoke.mjs drives graft ui in headless Chrome through the DevTools
// Protocol, the way a person would, and checks the config file after each
// save. Four runs: edits of a copy of config.example.yaml, the path of a
// newcomer from no config at all to Save and Preview, a broken file fixed by
// hand, and a server stopped under an unsaved change. graft ui runs with a
// home of its own and none of the environment of whoever runs the smoke: its
// page reads keys. It needs Chrome and Node 22 or later, which has WebSocket
// built in.
//
// usage: node internal/ui/jstest/smoke.mjs bin/graft
// CHROME names the browser when it is not google-chrome.

import { spawn } from 'node:child_process';
import { createHash } from 'node:crypto';
import { copyFileSync, mkdirSync, mkdtempSync, readFileSync, rmSync, statSync, writeFileSync } from 'node:fs';
import http from 'node:http';
import { tmpdir } from 'node:os';
import { join, resolve } from 'node:path';
import { fileURLToPath } from 'node:url';

const graftBin = process.argv[2];
if (!graftBin) {
  console.error('usage: node smoke.mjs GRAFT_BINARY');
  process.exit(2);
}
const sleep = (ms) => new Promise((r) => setTimeout(r, ms));
const dir = mkdtempSync(join(tmpdir(), 'graft-ui-smoke-'));
const home = join(dir, 'home'); // the home of graft ui, empty
const bin = join(dir, 'bin'); // holds a claude that does nothing, for Preview to find
mkdirSync(home);
mkdirSync(bin);
writeFileSync(join(bin, 'claude'), '#!/bin/sh\nexit 0\n', { mode: 0o755 });
const procs = [];
const problems = []; // errors the page logged
let failed = 0;

function finish(code) {
  for (const p of procs) p.kill('SIGTERM');
  rmSync(dir, { recursive: true, force: true });
  process.exit(code);
}

// waitFor polls f until it returns a value other than false, null or
// undefined.
async function waitFor(what, f, ms = 5000) {
  const end = Date.now() + ms;
  for (;;) {
    const v = await f();
    if (v !== false && v !== null && v !== undefined) return v;
    if (Date.now() > end) throw new Error(`timed out waiting for ${what}`);
    await sleep(50);
  }
}

// check reports one expectation; detail shows only when it fails.
function check(name, ok, detail = '') {
  if (ok) {
    console.log(`PASS ${name}`);
    return;
  }
  failed++;
  console.log(`FAIL ${name}${detail ? `: ${detail}` : ''}`);
}

// startUI runs graft ui for the config at path and returns its address, its
// output and its process. graft opens a browser of its own, with the live
// token in its address, on macOS and on Linux with a display, unless it runs
// over SSH. Empty display variables are enough on Linux, but macOS has none,
// so SSH_CONNECTION is set too: otherwise each run would open tabs in the
// user's browser.
async function startUI(path) {
  // graft ui gets a home of the smoke's own and nothing else of the user's
  // environment: the key widget reads keys, and must never read the user's,
  // from ~/.config/graft/keys or a variable such as OLLAMA_API_KEY. The claude
  // in bin is for Preview, which only looks it up.
  const env = { PATH: `${bin}:${process.env.PATH}`, HOME: home, DISPLAY: '', WAYLAND_DISPLAY: '', SSH_CONNECTION: 'graft-ui-smoke' };
  // The child starts in home, and takes a relative path such as bin/graft from
  // there: resolve it from the directory the smoke runs in.
  const graft = spawn(resolve(graftBin), ['--config', path, 'ui'], { env, cwd: home });
  procs.push(graft);
  let out = '';
  graft.stdout.on('data', (d) => { out += d; });
  graft.stderr.on('data', (d) => { out += d; });
  const url = await waitFor('the address of graft ui', () => (out.match(/graft ui: open (\S+)/) || [])[1]);
  return { url, output: () => out, proc: graft };
}

// connect starts Chrome and returns send, a call of the DevTools Protocol.
async function connect() {
  const profile = join(dir, 'chrome');
  procs.push(spawn(process.env.CHROME || 'google-chrome', [
    '--headless=new', '--disable-gpu', '--no-first-run', '--no-default-browser-check',
    `--user-data-dir=${profile}`, '--remote-debugging-port=0', 'about:blank',
  ], { stdio: 'ignore' }));
  const port = await waitFor('Chrome', () => {
    try {
      return readFileSync(join(profile, 'DevToolsActivePort'), 'utf8').split('\n')[0];
    } catch {
      return null;
    }
  }, 15000);
  const target = await waitFor('a page of Chrome', async () => {
    try {
      return (await (await fetch(`http://127.0.0.1:${port}/json/list`)).json()).find((t) => t.type === 'page');
    } catch {
      return null;
    }
  });
  const ws = new WebSocket(target.webSocketDebuggerUrl);
  await new Promise((r) => ws.addEventListener('open', r));
  let lastId = 0;
  const pending = new Map();
  ws.addEventListener('message', (ev) => {
    const msg = JSON.parse(ev.data);
    if (msg.id && pending.has(msg.id)) {
      pending.get(msg.id)(msg);
      pending.delete(msg.id);
    } else if (msg.method === 'Runtime.exceptionThrown') {
      problems.push(msg.params.exceptionDetails.exception?.description || msg.params.exceptionDetails.text);
    } else if (msg.method === 'Log.entryAdded' && msg.params.entry.level === 'error') {
      problems.push(msg.params.entry.text);
    }
  });
  const send = (method, params = {}) => new Promise((resolve) => {
    const id = ++lastId;
    pending.set(id, resolve);
    ws.send(JSON.stringify({ id, method, params }));
  });
  await send('Runtime.enable');
  await send('Log.enable');
  return { send, close: () => ws.close() };
}

// page runs code in the page, with helpers to find and use its parts.
function pageOf(send) {
  return async (code) => {
    const expression = `(async () => {
      const $ = (s) => document.querySelector(s);
      const $$ = (s) => [...document.querySelectorAll(s)];
      const fire = (el, type) => el.dispatchEvent(new Event(type, { bubbles: true }));
      const click = (el) => el.dispatchEvent(new MouseEvent('click', { bubbles: true }));
      const type = (el, v) => { el.dispatchEvent(new FocusEvent('focus')); el.value = v; fire(el, 'input'); };
      const choose = (el, v) => { el.value = v; fire(el, 'change'); };
      const label = (t) => $$('label').find((l) => l.firstChild && l.firstChild.textContent.trim() === t);
      const entry = (section, name) => [...$$('nav.side ul')[['providers', 'models', 'profiles'].indexOf(section)]
        .querySelectorAll('a')].find((a) => a.querySelector('.mono').textContent === name);
      const button = (text) => $$('button').find((b) => b.textContent.trim() === text);
      const state = () => $('.state') && $('.state').textContent;
      const saveButton = () => $$('header.top button').pop();
      ${code}
    })()`;
    const r = await send('Runtime.evaluate', { expression, awaitPromise: true, returnByValue: true });
    if (r.result.exceptionDetails) throw new Error(r.result.exceptionDetails.exception?.description || r.result.exceptionDetails.text);
    return r.result.result.value;
  };
}

// editExample edits a copy of config.example.yaml.
async function editExample(send, page) {
  const config = join(dir, 'example.yaml');
  copyFileSync(fileURLToPath(new URL('../../../config.example.yaml', import.meta.url)), config);
  const file = () => readFileSync(config, 'utf8');
  const { url } = await startUI(config);
  await send('Page.navigate', { url });
  await waitFor('the menu', () => page(`return $$('nav.side li').length > 0 || null`));

  check('the token leaves the address bar', !(await page(`return location.hash.includes('token')`)));
  const profiles = await page(`return $$('nav.side ul')[2].querySelectorAll('a').length`);
  check('the menu lists the five profiles', profiles === 5, String(profiles));
  check('profiles show their mode', await waitFor('modes', () => page(`return $$('nav.side .tag.router').length === 3 || null`)));
  check('the file reads as saved', (await page(`return state()`)) === 'saved');

  // The + of Providers opens the dialog Add a provider, which offers the presets
  // and Custom; Cancel closes it. Custom is there before the presets arrive, so
  // the wait is for a preset.
  await page(`click($$('nav.side .sec .add')[0])`);
  const offered = await waitFor('the dialog with the presets', () => page(
    `const c = $$('dialog[open] .cards button.card strong').map((s) => s.textContent); return c.includes('Ollama Cloud') ? c : null`));
  await page(`click(button('Cancel'))`);
  const closed = await waitFor('the dialog to close', () => page(`return $('dialog') === null || null`));
  check('the + of providers opens the dialog with the presets', closed && offered.includes('Ollama Cloud') && offered.includes('Custom…'), offered.join(', '));

  // Change the effort of profile sol and save with Ctrl+S.
  await page(`click(entry('profiles', 'sol'))`);
  await waitFor('the form of sol', () => page(`return label('Effort') ? true : null`));
  await page(`choose(label('Effort').querySelector('select'), 'high')`);
  check('a change reads as unsaved', (await page(`return state()`)) === '● unsaved');
  await waitFor('Save to turn on', () => page(`return !saveButton().disabled || null`));
  await page(`window.dispatchEvent(new KeyboardEvent('keydown', { key: 's', ctrlKey: true }))`);
  await waitFor('the save', () => (file().includes('sol: {model: sol, effort: high}') ? true : null));
  check('Ctrl+S saves the change', true);
  check('the comments stay', file().includes('# The whole session on one external model: graft sol'));
  check('the page reads as saved again', await waitFor('saved', () => page(`return state() === 'saved' || null`)));

  // The key of provider cliproxy is ~/.config/graft/keys/cliproxy: under the
  // home of the smoke, where there is no such file. It is not keys/cliproxy
  // next to the config either, so the widget gives the command for a terminal
  // and no field to paste the key in.
  await page(`click(entry('providers', 'cliproxy'))`);
  await waitFor('the state of the key', () => page(`return $('.key-state') && $('.key-state').textContent === 'No key file yet.' ? true : null`));
  check('the key widget says that the key file is missing', true);
  const field = await page(`return $('input[aria-label="Key to save"]') !== null`);
  const command = await page(`return $('code.key-command') && $('code.key-command').textContent`);
  check('a key file away from the config gets the command, not Save key',
    !field && command === '(umask 077; mkdir -p ~/.config/graft/keys && rm -f ~/.config/graft/keys/cliproxy && cat > ~/.config/graft/keys/cliproxy)',
    `${field ? 'Save key is there; ' : ''}${command}`);

  // Preview shows what graft --dry-run prints for profile sol, with the key as
  // its source: reading a key is not what Preview is for.
  await page(`click(entry('profiles', 'sol'))`);
  await waitFor('the tabs of sol', () => page(`return button('Preview') ? true : null`));
  await page(`click(button('Preview'))`);
  const plan = await waitFor('the preview', () => page(`return $('pre.plan') && $('pre.plan').textContent`));
  check('Preview shows what --dry-run prints, with the key as its source',
    plan.includes('profile sol: direct, provider cliproxy') && plan.includes('ANTHROPIC_AUTH_TOKEN=<file ~/.config/graft/keys/cliproxy>') &&
      plan.includes(`exec   ${join(bin, 'claude')}`), plan);
  await page(`click(button('Edit'))`);

  // Rename model kimi: the profiles that use it follow.
  await page(`click(entry('models', 'kimi'))`);
  await waitFor('the form of kimi', () => page(`return label('Name') ? true : null`));
  await page(`type(label('Name').querySelector('input'), 'kimi-k3')`);
  await waitFor('the check of the rename', () => page(`return !saveButton().disabled || null`));
  // The page checks a change 300 ms after the last edit, so until the answer
  // comes the error button and Save still show the draft before the rename.
  await sleep(700);
  check('the rename keeps the config valid', await page(`return $('header.top .errors') === null`));
  await page(`click(saveButton())`);
  await waitFor('the save of the rename', () => (file().includes('kimi-k3:') ? true : null));
  check('the rename reaches the profiles', /picker: \[sol, glm, kimi-k3\]/.test(file()) && /picker: \[sol, kimi-k3\]/.test(file()));

  // A wrong value: an error at its field, and no saving.
  await page(`click(entry('models', 'kimi-k3'))`);
  await waitFor('the form of kimi-k3', () => page(`return label('Context') ? true : null`));
  await page(`type(label('Context').querySelector('input'), '5')`);
  const error = await waitFor('the error', () => page(`const s = label('Context').querySelector('small.error'); return s && s.textContent`));
  check('the error shows at its field', error.includes('at least 100000'), error);
  check('Save is off while there are errors', await page(`return saveButton().disabled`));

  // Ctrl+S follows the button. Sent anyway, a save would draw a 422 from the
  // server, which Chrome logs as an error, so a silent console shows that the
  // page asked nothing.
  const before = file();
  const logged = problems.length;
  await page(`window.dispatchEvent(new KeyboardEvent('keydown', { key: 's', ctrlKey: true }))`);
  await sleep(700);
  check('Ctrl+S saves nothing while there are errors', file() === before && problems.length === logged,
    problems.slice(logged).join(' | ') || 'the file changed');

  // The file changes on disk under unsaved changes: the banner.
  writeFileSync(config, file() + '# edited by hand\n');
  await waitFor('the banner', () => page(`return $('.banner.warn') ? true : null`), 6000);
  check('a change on disk shows the banner', true);
  await page(`click(button('Reload'))`);
  check('Reload drops the unsaved change', await waitFor('saved', () => page(`return state() === 'saved' || null`)));
}

// startProvider starts a fake provider of the Messages API on 127.0.0.1. It
// records each request it gets, with the body parsed, and answers every one
// with a model that says pong.
async function startProvider() {
  const requests = [];
  const server = http.createServer((req, res) => {
    let data = '';
    req.on('data', (d) => { data += d; });
    req.on('end', () => {
      let body = null;
      try {
        body = JSON.parse(data);
      } catch {
        // Not JSON: the check of the request finds no body.
      }
      requests.push({ method: req.method, path: req.url, headers: req.headers, body });
      res.writeHead(200, { 'content-type': 'application/json' });
      res.end(JSON.stringify({ type: 'message', content: [{ type: 'text', text: 'pong' }] }));
    });
  });
  await new Promise((r) => server.listen(0, '127.0.0.1', r));
  return { url: `http://127.0.0.1:${server.address().port}`, requests };
}

// regexpOf returns the regular expression that matches text as it is.
const regexpOf = (text) => text.replace(/[.*+?^${}()|[\]\\]/g, '\\$&');

// newcomer walks the path of someone with no config at all, up to Save and
// Preview: Custom, a key pasted and saved, a model tested against the fake
// provider, the dialog Add provider, Save, and the plan of the new profile. The
// key is a fake one, and it must reach neither the page nor the config.
async function newcomer(send, page) {
  const provider = await startProvider();
  const config = join(dir, 'new', 'config.yaml');
  const key = 'sk-smoke-key';
  const fingerprint = createHash('sha256').update(key).digest('hex').slice(0, 12);
  const keyPath = join(dir, 'new', 'keys', 'local');
  const { url, output } = await startUI(config);
  await send('Page.navigate', { url });

  const presets = await waitFor('the first-run screen', () => page(
    `const c = $$('article.welcome .cards button.card strong').map((s) => s.textContent); return c.includes('Ollama Cloud') ? c : null`));
  // The page checks the draft 300 ms after it loads, and the check finds that
  // a profile is missing: the first-run screen shows no error for it, and Save
  // stays off.
  await sleep(700);
  const calm = await page(`return { button: $('header.top .errors') !== null, badge: $$('nav.side .sec')[2].querySelector('.badge') !== null,
    line: $$('nav.side .sec-error').length > 0, saveOff: saveButton().disabled }`);
  check('a missing config shows the presets and no error', presets.includes('Custom…') && !calm.button && !calm.badge && !calm.line && calm.saveOff,
    `${presets.join(', ')}; ${JSON.stringify(calm)}`);

  await page(`click($$('article.welcome .cards button.card').find((b) => b.querySelector('strong').textContent === 'Custom…'))`);
  const first = await waitFor('the form of Custom', () => page(`return $('dialog[open]') && label('Name') && label('Name').querySelector('input').value`));
  await page(`type(label('Name').querySelector('input'), 'local')`);
  const source = await waitFor('the key file to follow the name', () => page(
    `const v = $('input[aria-label="Key source"]').value; return v.endsWith('/new/keys/local') ? v : null`));
  check('Custom starts with a free name, and the key file follows the name', first === 'provider-1', `${first}, ${source}`);

  await page(`type(label('URL').querySelector('input'), ${JSON.stringify(provider.url)})`);
  const missing = await waitFor('the state of the key', () => page(`const s = $('dialog[open] .key-state'); return s && s.textContent.trim()`));
  const paste = await page(`const f = $('input[aria-label="Key to save"]'); return f !== null && f.getClientRects().length > 0`);
  check('the key widget says that there is no key file yet', missing === 'No key file yet.' && paste, `${missing}; the field is ${paste ? 'there' : 'missing'}`);

  // The field is a text field that CSS masks, not a password field: Chrome
  // offers to keep what a password field held in its password store. The
  // attributes keep spell checks and password-manager extensions away; a
  // Chrome without the property autocorrect has no autocorrection to turn off.
  const masked = await page(`const f = $('input[aria-label="Key to save"]');
    return { type: f.type, mask: getComputedStyle(f).webkitTextSecurity, passwords: $$('input[type="password"]').length,
      spellcheck: f.spellcheck, autocorrect: !('autocorrect' in f) || f.autocorrect === false,
      attributes: ['autocomplete', 'autocapitalize', 'data-1p-ignore', 'data-lpignore', 'data-bwignore'].map((a) => f.getAttribute(a)).join(' ') }`);
  check('the field for the key is masked text, and the page has no password field',
    masked.type === 'text' && masked.mask === 'disc' && masked.passwords === 0 && masked.spellcheck === false && masked.autocorrect &&
      masked.attributes === 'off off true true true', JSON.stringify(masked));

  await page(`type($('input[aria-label="Key to save"]'), ${JSON.stringify(key)})`);
  await waitFor('Save key to turn on', () => page(`return !button('Save key').disabled || null`));
  await page(`click(button('Save key'))`);
  const written = await waitFor('the key file', () => {
    try {
      return readFileSync(keyPath, 'utf8');
    } catch {
      return null;
    }
  });
  const mode = statSync(keyPath).mode & 0o777;
  check('Save key writes the key, readable only by its owner', written === `${key}\n` && mode === 0o600, `mode ${mode.toString(8)}`);

  await waitFor('the line of the key file', () => (output().includes(`graft ui: wrote ${keyPath} (sha256:${fingerprint})`) ? true : null));
  check('graft ui prints the key file with its fingerprint, never the key', !output().includes(key));
  check('the widget shows the fingerprint of the saved key', await waitFor('the fingerprint', () => page(
    `const s = $('dialog[open] .key-state'); return (s && s.textContent.trim() === ${JSON.stringify(`Key ok: sha256:${fingerprint}`)}) || null`)));

  await page(`click(button('+ Model'))`);
  await waitFor('the row of the model', () => page(`return $('input[aria-label="Model name"]') ? true : null`));
  await page(`type($('input[aria-label="Model name"]'), 'fake')`);
  await page(`type($('input[aria-label="Model ID"]'), 'fake-1')`);
  await page(`type($('input[aria-label="Context window"]'), '200000')`);
  // Test forgets an old result in an effect, a moment after its model changed:
  // let that pass, so that it does not take the answer to come.
  await sleep(300);
  const row = `$('input[aria-label="Model name"]').closest('.model-row')`;
  await waitFor('Test to turn on', () => page(`return !${row}.querySelector('.test button').disabled || null`));
  await page(`click(${row}.querySelector('.test button'))`);
  const hit = await waitFor('the request of Test', () => provider.requests[0] || null);
  check('Test sends the key and the model to the provider',
    hit.method === 'POST' && hit.path === '/v1/messages' && hit.headers.authorization === `Bearer ${key}` &&
      hit.headers['anthropic-version'] === '2023-06-01' && hit.body?.model === 'fake-1' && hit.body?.max_tokens === 16,
    `${hit.method} ${hit.path}, model ${hit.body?.model}, max_tokens ${hit.body?.max_tokens}`);
  const answer = await waitFor('the answer of Test', () => page(`const r = ${row}.querySelector('.test-result'); return r && r.textContent`));
  check('Test shows the answer', /^200 in \d+ ms: pong$/.test(answer), answer);

  // A click that a script sends reaches a button that is off, which a person's does not.
  await waitFor('Add to config to turn on', () => page(`return !button('Add to config').disabled || null`));
  await page(`click(button('Add to config'))`);
  check('Add to config fills the menu', await waitFor('the menu', () => page(
    `return $('dialog') === null && entry('providers', 'local') && entry('models', 'fake') && entry('profiles', 'fake') ? true : null`)));

  await waitFor('Save to turn on', () => page(`return !saveButton().disabled || null`));
  await page(`click(saveButton())`);
  const file = await waitFor('the new file', () => {
    try {
      return readFileSync(config, 'utf8');
    } catch {
      return null;
    }
  });
  check('Save creates the config',
    new RegExp(`local:\\s*\\n\\s*url: ${regexpOf(provider.url)}\\s*\\n\\s*key: \\{file: ${regexpOf(keyPath)}\\}`).test(file) &&
      /fake:\s*\n\s*provider: local\s*\n\s*id: fake-1\s*\n\s*context: 200000/.test(file) && /fake:\s*\n\s*model: fake/.test(file), file);
  check('graft ui says it created the file', await waitFor('the line', () => output().includes('graft ui: created ') || null));

  const hint = await waitFor('the hint', () => page(`return $('.banner.info') && $('.banner.info').textContent`));
  check('the page says how to start the new profile', hint.includes('Run graft fake in a terminal.'), hint);

  // The form of the first new profile is open: its Preview shows the key file.
  const form = await page(`return $('main .form-head code') && $('main .form-head code').textContent`);
  await page(`click(button('Preview'))`);
  const plan = await waitFor('the preview', () => page(`return $('pre.plan') && $('pre.plan').textContent`));
  check('Preview of the new profile shows its key file',
    form === 'fake' && plan.includes('profile fake: direct, provider local') && plan.includes(`<file ${keyPath}>`), `${form}: ${plan}`);

  // Nothing of the page holds the key: not its text, not what its fields hold.
  const seen = await page(`return document.body.textContent + '\\n' + $$('input, textarea').map((i) => i.value).join('\\n')`);
  check('the key reaches neither the page nor the config', !seen.includes(key) && !readFileSync(config, 'utf8').includes(key));
}

// fixBrokenFile opens a file the forms cannot show, then fixes it on disk.
async function fixBrokenFile(send, page) {
  const config = join(dir, 'broken.yaml');
  writeFileSync(config, 'providers: [a]\nprofiles:\n  x: { model: opus }\n');
  const { url } = await startUI(config);
  await send('Page.navigate', { url });
  const text = await waitFor('the problem', () => page(`return $('main article') && $('main article').textContent`));
  check('a broken file shows its problem with the line', text.includes('broken.yaml:1') && text.includes('providers: must be a mapping'), text);
  writeFileSync(config, 'profiles:\n  x: { model: opus }\n');
  check('fixing the file brings the forms back', await waitFor('the menu', () => page(`return $$('nav.side li').length === 1 || null`), 6000));
}

// stopServer stops graft ui under a page with an unsaved change. A new graft ui
// has another port and token, so this tab never gets its server back: the page
// says so, asks the server nothing more, and does not let Save or Ctrl+S try.
async function stopServer(send, page) {
  const config = join(dir, 'stopped.yaml');
  copyFileSync(fileURLToPath(new URL('../../../config.example.yaml', import.meta.url)), config);
  const { url, proc } = await startUI(config);
  await send('Page.navigate', { url });
  await waitFor('the menu', () => page(`return $$('nav.side li').length > 0 || null`));
  await page(`click(entry('profiles', 'sol'))`);
  await waitFor('the form of sol', () => page(`return label('Effort') ? true : null`));
  await page(`choose(label('Effort').querySelector('select'), 'high')`);
  await waitFor('Save to turn on', () => page(`return !saveButton().disabled || null`));
  // The page checks a change 300 ms after the last edit: wait for the answer,
  // so that the stop comes with no request under way.
  await sleep(700);

  proc.kill('SIGINT');
  await waitFor('the banner', () => page(`return $('.banner.bad') ? true : null`), 6000);
  check('a stopped server shows the banner', true);

  // The request that told the page is logged as an error, and so would be each
  // poll of the version after it, every 2 s: wait for two. The forms stay
  // editable, and a change is checked 300 ms later: that must not ask either.
  // Nor must Ctrl+S, which would send a save if Save were on.
  const logged = problems.length;
  await page(`choose(label('Effort').querySelector('select'), 'low')`);
  await page(`window.dispatchEvent(new KeyboardEvent('keydown', { key: 's', ctrlKey: true }))`);
  await sleep(4500);
  const saveOff = await page(`return saveButton().disabled`);
  check('the page stops asking a stopped server, and Save and Ctrl+S are off', problems.length === logged && saveOff,
    `Save is ${saveOff ? 'off' : 'on'}; ${problems.slice(logged).join(' | ')}`);
}

try {
  const { send, close } = await connect();
  const page = pageOf(send);
  await editExample(send, page);
  await newcomer(send, page);
  await fixBrokenFile(send, page);
  check('the page logged no errors', problems.length === 0, problems.join(' | '));
  // The request by which the page learns that the server is gone is logged as
  // an error too, so this cannot come before the check above.
  await stopServer(send, page);
  close();
} catch (e) {
  failed++;
  console.log(`FAIL ${e.message}`);
}
finish(failed ? 1 : 0);
