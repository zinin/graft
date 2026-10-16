// checks.js holds the parts of the forms that ask the server themselves: the
// key widget of a provider, Test of a model and Preview of a profile. Each
// asks through page.request, and once page.stopped says that the server is
// gone it asks nothing and has its buttons off. An answer counts only while
// the part still holds the input it was asked for: an old answer can come
// last, as with the checks of the page.

import { html, useEffect, useRef, useState } from './vendor/htm-preact-standalone-3.1.1.module.js';
import { ApiError } from './api.js';
import { defaultKeyPath, keyCommand } from './draft.js';
import { KeySource } from './fields.js';

// whyFailed tells what a part shows of a failed request: the words of the
// server, or that there was no server to answer.
function whyFailed(e) {
  return e instanceof ApiError ? e.message : 'graft ui is not running.';
}

// KeyState tells what an answer of /api/key/check says of the key source it
// was asked for. name is the variable of an env source.
function KeyState({ answer, name }) {
  const text = {
    missing: 'No key file yet.',
    unset: `${name} is not set in the environment of graft ui. Export it in the shell that runs graft, then restart graft ui.`,
    'not run': 'Not run yet.',
    ok: `Key ok: sha256:${answer.fingerprint}`,
  }[answer.state];
  return html`
    <p class=${answer.state === 'ok' ? 'key-state ok' : 'key-state'}>
      ${text}
      ${answer.error && html`<small class="error">${answer.error}</small>`}
      ${answer.warning && html`<small class="warn">${answer.warning}</small>`}
    </p>`;
}

// KeyPaste is the field for a key to paste, and Save key, which sends it to
// the server to be written to the key file of provider. The pasted key lives
// only in the field and goes out only in the body of that write: it is in no
// draft and no state of the page, so the field is not a controlled one, and
// the state knows only whether it holds something. onWritten is called once
// the file is written.
//
// The field is a text field that CSS masks (see .masked in style.css), not a
// password field: once a request succeeds after a password field was typed
// into, Chrome offers to keep what it held, which autocomplete="off" does not
// stop, and one click on its bubble would put the key of the provider into the
// browser's password store, synced to the user's account. On a text field,
// autocomplete="off" does count: it keeps the key out of the state that the
// browser saves of the page, which would fill the field in again after Back
// and is written to the files of its session on disk. The data- attributes
// keep the common password-manager extensions away. spellcheck and
// autocorrect are boolean properties of the element, which Preact sets as
// properties: the strings "false" and "off" would turn them on.
function KeyPaste({ provider, page, onWritten }) {
  const field = useRef(null);
  const busy = useRef(false); // two clicks in a row see no render between them
  const [filled, setFilled] = useState(false);
  const [saving, setSaving] = useState(false);
  const [saved, setSaved] = useState(''); // where the last Save key wrote
  const [failure, setFailure] = useState('');

  async function save() {
    const key = field.current ? field.current.value : '';
    if (busy.current || key === '') return;
    busy.current = true;
    setSaving(true);
    setSaved('');
    setFailure('');
    try {
      const write = (replace) => page.request('POST', '/api/key/write', { provider, key, replace });
      let res;
      try {
        res = await write(false);
      } catch (e) {
        if (!(e instanceof ApiError && e.status === 409)) throw e;
        // The file holds a key already, and its fingerprint tells which one.
        const fingerprint = e.body && e.body.fingerprint;
        if (!window.confirm(`${e.message}${fingerprint ? ` (sha256:${fingerprint})` : ''}. Replace it with the pasted key?`)) return;
        res = await write(true);
      }
      // The field is gone when the form moved on while the key was on its way.
      if (field.current) field.current.value = '';
      setFilled(false);
      setSaved(res.path);
      onWritten();
    } catch (e) {
      setFailure(whyFailed(e));
    } finally {
      busy.current = false;
      setSaving(false);
    }
  }

  return html`
    <div class="row">
      <input type="text" class="masked" ref=${field} aria-label="Key to save" placeholder="Paste the key"
        autocomplete="off" spellcheck=${false} autocapitalize="off" autocorrect=${false}
        data-1p-ignore data-lpignore="true" data-bwignore
        disabled=${page.stopped} onInput=${(e) => setFilled(e.target.value !== '')} />
      <button type="button" disabled=${page.stopped || saving || !filled} onClick=${save}>Save key</button>
    </div>
    ${saved && html`<small class="ok">Saved to ${saved}.</small>`}
    ${failure && html`<small class="error">${failure}</small>`}`;
}

// CopyCommand shows a command for a terminal, and a button that copies it.
function CopyCommand({ command }) {
  const [copied, setCopied] = useState(false);
  const timer = useRef(0);
  useEffect(() => () => clearTimeout(timer.current), []);

  async function copy() {
    try {
      await navigator.clipboard.writeText(command);
    } catch {
      return; // the browser refused: the command stays on the page, to select by hand
    }
    setCopied(true);
    clearTimeout(timer.current);
    timer.current = setTimeout(() => setCopied(false), 2000);
  }

  return html`
    <small>Or in a terminal:</small>
    <div class="row">
      <code class="key-command">${command}</code>
      <button type="button" class="secondary outline" onClick=${copy}>${copied ? 'Copied' : 'Copy'}</button>
    </div>`;
}

// KeyWidget is the KeySource of a provider with what the server says of the
// key it names: the state of the key, Run for a key command, a field to paste a
// key into when the key file is the one Save key writes (next to the config,
// under the name of the provider), and the command that writes a key file in a
// terminal. provider is the name of the provider; keysLink is the page where
// its maker issues keys, '' when there is none. onSaved is called once Save key
// has written the key file.
export function KeyWidget({ value, onChange, errors = [], provider, keysLink, page, onSaved = () => {} }) {
  const [got, setGot] = useState(null); // the last answer and the source it is for: {kind, value, res}
  const [running, setRunning] = useState(false);
  const runLock = useRef(false); // two clicks in a row see no render between them
  const now = useRef(value); // the key source the form holds now
  now.current = value;

  // ask puts the key source to the server. run says whether a key command may
  // run now: it may ask for a password in the terminal of graft ui, so only Run
  // sets it. A failed request is an error of the key source it was for.
  async function ask(run) {
    const { kind, value: text } = now.current;
    if (!kind || text.trim() === '') return;
    let res;
    try {
      res = await page.request('POST', '/api/key/check', { kind, value: text, run });
    } catch (e) {
      res = { state: 'error', error: whyFailed(e) };
    }
    if (now.current.kind === kind && now.current.value === text) setGot({ kind, value: text, res });
  }

  // Ask 300 ms after the last edit of the source, and again each time the user
  // comes back to the page: the key file may have been written meanwhile, as
  // by the command for a terminal below. Neither runs a key command, and a Run
  // under way, which may be what the user comes back from, brings its own
  // answer.
  useEffect(() => {
    if (!value.kind || value.value.trim() === '' || page.stopped) return undefined;
    const t = setTimeout(() => ask(false), 300);
    const back = () => {
      if (document.visibilityState === 'visible' && !runLock.current) ask(false);
    };
    window.addEventListener('focus', back);
    document.addEventListener('visibilitychange', back);
    return () => {
      clearTimeout(t);
      window.removeEventListener('focus', back);
      document.removeEventListener('visibilitychange', back);
    };
  }, [value.kind, value.value, page.stopped]);

  async function run() {
    if (runLock.current) return;
    runLock.current = true;
    setRunning(true);
    try {
      await ask(true);
    } finally {
      runLock.current = false;
      setRunning(false);
    }
  }

  const answer = got && got.kind === value.kind && got.value === value.value ? got.res : null;
  const path = (answer && answer.path) || ''; // the key file graft reads, set for a file source
  // Save key writes the key file of the provider next to the config, and only
  // that one; a provider with no name has none.
  const defaultFile = provider !== '' ? defaultKeyPath(page.file.path, provider) : '';
  const paste = path !== '' && path === defaultFile;
  return html`
    <${KeySource} value=${value} onChange=${onChange} errors=${errors} defaultFile=${defaultFile}>
      <div class="key-widget">
        ${answer && html`<${KeyState} answer=${answer} name=${value.value} />`}
        ${value.kind === 'command' && html`
          <div class="row">
            <button type="button" class="secondary outline" disabled=${page.stopped || running || value.value.trim() === ''}
              aria-busy=${running ? 'true' : null} onClick=${run}>Run</button>
            <small>Runs the command now. It may ask for a password in the terminal of graft ui.</small>
          </div>`}
        ${paste && html`<${KeyPaste} provider=${provider} page=${page} onWritten=${() => { ask(false); onSaved(); }} />`}
        ${path !== '' && html`<${CopyCommand} command=${keyCommand(path)} />`}
        ${keysLink && html`<small>Get a key at <a href=${keysLink} target="_blank" rel="noopener noreferrer">${keysLink}</a></small>`}
      </div>
    <//>`;
}

// sameTarget reports whether two targets of Test send the same request.
function sameTarget(a, b) {
  return a !== null && b !== null && a.url === b.url && a.key.kind === b.key.kind && a.key.value === b.key.value && a.id === b.id;
}

// TestButton sends a request to the model of target, {url, key, id} or null,
// and shows what the provider answered. It spends a few tokens, so it asks
// only when pressed.
export function TestButton({ target, page }) {
  const [result, setResult] = useState(null); // {answer} of the provider, or {problem} for no answer
  const [busy, setBusy] = useState(false);
  const lock = useRef(false); // two clicks in a row see no render between them
  const now = useRef(target); // the target the form holds now
  now.current = target;

  // A result is of the target it was asked for: it goes when the target changes.
  useEffect(() => {
    setResult(null);
  }, [target?.url, target?.key.kind, target?.key.value, target?.id]);

  async function test() {
    if (lock.current || target === null) return;
    lock.current = true;
    const sent = target;
    setBusy(true);
    setResult(null);
    let next;
    try {
      next = { answer: await page.request('POST', '/api/test', { url: sent.url, key: { kind: sent.key.kind, value: sent.key.value }, id: sent.id }) };
    } catch (e) {
      next = { problem: whyFailed(e) };
    }
    lock.current = false;
    setBusy(false);
    if (sameTarget(sent, now.current)) setResult(next);
  }

  let line = null; // the result in a line
  let body = ''; // the body of an answer that is not a 2xx
  if (result && result.problem) {
    line = html`<small class="test-result error">${result.problem}</small>`;
  } else if (result) {
    const { status, ms, text } = result.answer;
    const ok = Math.floor(status / 100) === 2;
    const head = `${status} in ${ms} ms`;
    line = html`<small class=${ok ? 'test-result ok' : 'test-result error'}>${ok ? `${head}: ${text || 'no text in 16 tokens'}` : head}</small>`;
    if (!ok) body = result.answer.body;
  }
  const off = target === null || target.url === '' || target.key.kind === '' || page.stopped || busy;
  return html`
    <div class="test">
      <div class="row">
        <button type="button" class="secondary outline" disabled=${off} aria-busy=${busy ? 'true' : null} onClick=${test}>Test (spends a few tokens)</button>
        ${line}
      </div>
      ${body && html`<pre class="plan">${body}</pre>`}
    </div>`;
}

// Preview shows what graft would start for profile of draft: the text of
// graft --dry-run, with each key as its source. It asks 300 ms after it
// appears and after each change of the draft or the file.
export function Preview({ draft, profile, page }) {
  const [shown, setShown] = useState(null); // the last answer that counted
  const [problem, setProblem] = useState(''); // why the last request failed
  const [staleOf, setStaleOf] = useState(null); // the file that the last answer found changed on disk, or null
  const now = useRef(null); // the draft, file and profile the part holds now
  now.current = { draft, file: page.file, profile };
  const hasErrors = page.errors > 0;

  useEffect(() => {
    if (hasErrors || page.stopped) return undefined;
    const t = setTimeout(async () => {
      let res = null;
      let why = '';
      try {
        res = await page.request('POST', '/api/preview', { version: page.file.version, draft, profile });
      } catch (e) {
        why = whyFailed(e);
      }
      // The answer counts only while the part still holds the draft and the
      // file it was for, which the page replaces and never changes in place.
      // A stale one is of a file that has changed, and shows nothing new.
      if (draft !== now.current.draft || page.file !== now.current.file || profile !== now.current.profile) return;
      if (why) {
        setProblem(why);
      } else if (res.stale) {
        setStaleOf(page.file);
      } else {
        setProblem('');
        setStaleOf(null);
        setShown(res);
      }
    }, 300);
    return () => clearTimeout(t);
  }, [draft, page.file, profile, hasErrors, page.stopped]);

  if (hasErrors || (shown && shown.errors.length > 0)) return html`<p class="muted">Fix the errors to see the preview.</p>`;
  if (problem) return html`<p class="error">${problem}</p>`;
  if (!shown) {
    if (page.stopped) return null;
    // Each answer is stale until the page has the file again, which it does
    // not take over unsaved changes by itself: say so instead of loading for
    // ever. A file the page has taken since asks anew.
    if (staleOf === page.file) return html`<p class="muted">The file changed on disk: reload it to see the preview.</p>`;
    return html`<p aria-busy="true">Loading…</p>`;
  }
  return html`
    ${shown.warnings.map((w) => html`<p class="warn">${w}</p>`)}
    ${shown.failure && html`<pre class="plan error">${shown.failure}</pre>`}
    ${shown.text && html`<pre class="plan">${shown.text}</pre>`}`;
}
