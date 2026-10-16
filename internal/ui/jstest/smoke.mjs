// smoke.mjs drives graft ui in headless Chrome through the DevTools
// Protocol, the way a person would, and checks the config file after each
// save. Four runs: edits of a copy of config.example.yaml, a config built
// from nothing, a broken file fixed by hand, and a server stopped under an
// unsaved change. graft ui runs with a home of its own and none of the
// environment of whoever runs the smoke: its page reads keys. It needs Chrome
// and Node 22 or later, which has WebSocket built in.
//
// usage: node internal/ui/jstest/smoke.mjs bin/graft
// CHROME names the browser when it is not google-chrome.

import { spawn } from 'node:child_process';
import { copyFileSync, mkdirSync, mkdtempSync, readFileSync, rmSync, writeFileSync } from 'node:fs';
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
    !field && command === 'mkdir -p ~/.config/graft/keys && (umask 077; cat > ~/.config/graft/keys/cliproxy)',
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

// buildFromNothing starts without a config and builds one.
async function buildFromNothing(send, page) {
  const config = join(dir, 'new', 'config.yaml');
  const { url, output } = await startUI(config);
  await send('Page.navigate', { url });
  const title = await waitFor('the empty screen', () => page(`return $('main article h2') && $('main article h2').textContent`));
  check('a missing config shows the empty screen', title === 'No config yet', title);

  await page(`click(button('Add a provider'))`);
  const focused = await waitFor('the new form', () => page(`return document.activeElement && document.activeElement.value`));
  check('a new provider gets a name and the focus', focused === 'provider-1', String(focused));
  await page(`type(label('Name').querySelector('input'), 'ollama')`);
  await page(`type(label('URL').querySelector('input'), 'https://ollama.com')`);
  const keyPath = await page(`return $('input[aria-label="Key source"]').value`);
  check('the default key file follows the name', keyPath.endsWith('/keys/ollama'), keyPath);

  await page(`click($$('nav.side .sec .add')[1])`);
  await waitFor('the model form', () => page(`return label('Context') ? true : null`));
  await page(`type(label('Name').querySelector('input'), 'glm')`);
  await page(`type(label('Context').querySelector('input'), '1048576')`);
  await page(`click($$('nav.side .sec .add')[2])`);
  await waitFor('the profile form', () => page(`return label('Model') ? true : null`));
  await page(`choose(label('Model').querySelector('select'), 'glm')`);
  await waitFor('a valid config', () => page(`return !saveButton().disabled || null`));
  await page(`click(saveButton())`);
  const file = await waitFor('the new file', () => {
    try {
      return readFileSync(config, 'utf8');
    } catch {
      return null;
    }
  });
  check('Save creates the config', /ollama:\s*\n\s*url: https:\/\/ollama\.com\s*\n\s*key: \{file: .*\/keys\/ollama\}/.test(file) &&
    /glm:\s*\n\s*provider: ollama\s*\n\s*context: 1048576/.test(file) && /profile-1:\s*\n\s*model: glm/.test(file), file);
  check('graft ui says it created the file', await waitFor('the line', () => output().includes('graft ui: created ') || null));
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
  await buildFromNothing(send, page);
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
