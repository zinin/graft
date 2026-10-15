// app.js is the page of graft ui: it loads the draft of the config, checks
// each change on the server, saves, and notices changes of the file.

import { html, render, useEffect, useRef, useState } from './vendor/htm-preact-standalone-3.1.1.module.js';
import { ApiError, call, takeToken } from './api.js';
import {
  canSave, carryKeys, carryName, isSaveKey, keepTyped, locate, messagesFor, newModel, newName, newProfile, newProvider,
  own, placeErrors, renameProvider, sameDraft, takePlaces, usedBy, withKeys,
} from './draft.js';
import { DefaultsForm, ModelForm, ProfileForm, ProviderForm } from './forms.js';
import { Banner, Empty, ErrorList, Menu, Messages, NotEditable, TopBar } from './layout.js';

const BASES = { providers: 'provider', models: 'model', profiles: 'profile' };
const FORMS = { providers: ProviderForm, models: ModelForm, profiles: ProfileForm };

function App() {
  const [file, setFile] = useState(null); // the config file as /api/config gave it
  const [saved, setSaved] = useState(null); // the draft of the file on disk
  const [draft, setDraft] = useState(null); // the draft the forms edit
  const [check, setCheck] = useState({ errors: [], profiles: {} });
  const [selected, setSelected] = useState(null); // an entry's _k, or 'defaults'
  const [added, setAdded] = useState(null); // the _k of the entry just added
  const [stale, setStale] = useState(false); // the file changed under unsaved edits
  const [banner, setBanner] = useState(null); // 'changed' or null
  const [stopped, setStopped] = useState(false); // the server is gone for good
  const [showErrors, setShowErrors] = useState(false);
  const [toast, setToast] = useState('');
  const [failure, setFailure] = useState('');
  const [saving, setSaving] = useState(false);

  const dirty = draft !== null && saved !== null && !sameDraft(draft, saved);
  // The Save button and Ctrl+S follow one rule.
  const saveOn = canSave({ dirty, stale, saving, stopped, errors: check.errors.length });
  // Timers and listeners read the state through now.
  const now = useRef({});
  now.current = { file, draft, dirty, stale, saving, stopped, canSave: saveOn };

  // setChecked keeps the result of a check. When it holds no error, the list
  // of errors closes: it would be empty, and would open again at the next error.
  function setChecked(res) {
    setCheck(res);
    if (res.errors.length === 0) setShowErrors(false);
  }

  async function load(note, auto = false) {
    try {
      const st = await call('GET', '/api/config');
      // The draft changed while the file was being read, and reloading would
      // drop those changes: the file changed under unsaved changes instead.
      if (auto && now.current.dirty) {
        changed();
        return;
      }
      setFile(st);
      setStale(false);
      setBanner(null);
      // An earlier failure and the errors of the earlier draft are over; the
      // check of the file comes with its draft.
      setFailure('');
      setChecked({ errors: [], profiles: {} });
      if (st.editable) {
        // The entries keep the keys of the page by name, as after a save, so
        // that the form that is open stays open.
        const cur = now.current.draft;
        const d = cur ? carryKeys(cur, st.draft) : withKeys(st.draft);
        setSaved(structuredClone(d));
        setDraft(d);
      } else {
        setSaved(null);
        setDraft(null);
      }
      if (note) setToast(note);
    } catch (e) {
      setFailure(e instanceof ApiError ? e.message : 'graft ui is not running: start it again and open the address it prints.');
    }
  }

  // changed handles a new version of the file on disk.
  function changed() {
    if (!now.current.dirty) {
      load('Reloaded from disk', true);
    } else if (!now.current.stale) {
      setStale(true);
      setBanner('changed');
    }
  }

  // trouble shows a failed request. An answer of the server is a toast. No
  // answer means the server is gone for good: the next graft ui has another
  // port and token, so this tab stops asking and Save goes off.
  function trouble(e) {
    if (e instanceof ApiError) {
      setToast(e.message);
    } else {
      setStopped(true);
    }
  }

  useEffect(() => {
    load();
  }, []);

  // Check the draft 300 ms after its last change.
  useEffect(() => {
    if (!draft || !file) return undefined;
    const t = setTimeout(async () => {
      // The forms stay editable once the server has stopped, to copy the
      // changes out, but nothing is left to check them.
      if (now.current.stopped) return;
      try {
        const res = await call('POST', '/api/validate', { version: file.version, draft });
        // A check of a version this page has since saved over is stale by
        // its own doing.
        if (res.stale && file.version === now.current.file.version) changed();
        else if (!res.stale) setChecked(res);
      } catch (e) {
        trouble(e);
      }
    }, 300);
    return () => clearTimeout(t);
  }, [draft, file]);

  // Watch the file: every 2 s, ask for its version, unless the server has stopped.
  useEffect(() => {
    const t = setInterval(async () => {
      const f = now.current.file;
      if (!f || now.current.saving || now.current.stopped) return;
      try {
        const { version } = await call('GET', '/api/version');
        // The page's own save may have changed the version meanwhile.
        if (version !== now.current.file.version && !now.current.saving) changed();
      } catch (e) {
        trouble(e);
      }
    }, 2000);
    return () => clearInterval(t);
  }, []);

  // The marker of the entry just added is for its form as it opens, which
  // puts the cursor into the name; reopened later, the form is like any other.
  useEffect(() => {
    if (added !== null) setAdded(null);
  }, [added]);

  useEffect(() => {
    if (!toast) return undefined;
    const t = setTimeout(() => setToast(''), 3000);
    return () => clearTimeout(t);
  }, [toast]);

  async function save() {
    // Ctrl+S comes here as well, and does what the button does: nothing when
    // the button is off.
    if (!now.current.canSave) return;
    const { file: f, draft: d } = now.current;
    setSaving(true);
    try {
      const res = await call('POST', '/api/save', { version: f.version, draft: d });
      const next = carryKeys(d, res.draft);
      const saved = { ...f, exists: true, version: res.version };
      now.current.file = saved; // before the next render, for the watch
      setFile(saved);
      setStale(false);
      setBanner(null);
      setSaved(structuredClone(next));
      // What was typed while the request was under way stays, unsaved.
      setDraft(keepTyped(now.current.draft, d, next));
      setToast('Saved');
    } catch (e) {
      if (e instanceof ApiError && e.status === 409) changed();
      else if (e instanceof ApiError && e.status === 422) setChecked({ errors: e.body.errors, profiles: {} });
      else trouble(e);
    } finally {
      setSaving(false);
    }
  }
  const saveRef = useRef(save);
  saveRef.current = save;

  useEffect(() => {
    const onKey = (e) => {
      if (isSaveKey(e)) {
        e.preventDefault();
        saveRef.current();
      }
    };
    const onLeave = (e) => {
      if (now.current.dirty) {
        e.preventDefault();
        e.returnValue = '';
      }
    };
    window.addEventListener('keydown', onKey);
    window.addEventListener('beforeunload', onLeave);
    return () => {
      window.removeEventListener('keydown', onKey);
      window.removeEventListener('beforeunload', onLeave);
    };
  }, []);

  function add(section) {
    const d = structuredClone(draft);
    const name = newName(d[section], BASES[section]);
    const entry = section === 'providers' ? newProvider(name, file.path)
      : section === 'models' ? newModel(name, d.providers.length ? d.providers[0].name : '')
        : newProfile(name);
    d[section].push(entry);
    withKeys(d);
    setDraft(d);
    setSelected(entry._k);
    setAdded(entry._k);
  }

  function update(section, k, entry) {
    setDraft((d) => ({ ...d, [section]: d[section].map((e) => (e._k === k ? entry : e)) }));
  }

  function rename(section, k, name, refs) {
    setDraft((d) => {
      const next = carryName(d, section, refs, name);
      next[section] = next[section].map((e) => {
        if (e._k !== k) return e;
        return section === 'providers' ? renameProvider(e, name, file.path) : { ...e, name };
      });
      return next;
    });
  }

  function remove(section, k) {
    const entry = draft[section].find((e) => e._k === k);
    const users = usedBy(draft, section, entry.name);
    const what = `${BASES[section]} ${entry.name}`;
    const note = users.length ? `\n\nUsed by ${users.join(', ')}: they will show errors until you change them.` : '';
    if (!window.confirm(`Delete ${what}?${note}`)) return;
    setDraft({ ...draft, [section]: draft[section].filter((e) => e._k !== k) });
    setSelected(null);
  }

  // goTo selects the entry an error path is about.
  function goTo(path) {
    if (!draft) return; // the file is not shown any more
    const at = locate(draft, path);
    if (at && at.section === 'defaults') setSelected('defaults');
    else if (at && at.name !== null) setSelected(draft[at.section].find((e) => e.name === at.name)._k);
    setShowErrors(false);
  }

  const placed = draft ? placeErrors(draft, check.errors) : { global: [], sections: {}, entries: {}, counts: {} };

  let main;
  if (failure) {
    main = html`<article class="error">${failure}</article>`;
  } else if (!file) {
    main = html`<p aria-busy="true">Loading…</p>`;
  } else if (!file.editable) {
    main = html`<${NotEditable} path=${file.path} problems=${file.problems} />`;
  } else if (selected === 'defaults') {
    const errors = placed.entries.defaults || [];
    main = html`<${DefaultsForm} entry=${draft.defaults} errs=${(f) => messagesFor(errors, f)}
      update=${(d) => setDraft((cur) => ({ ...cur, defaults: d }))} />`;
  } else {
    let form = null;
    for (const section of ['providers', 'models', 'profiles']) {
      const entry = draft[section].find((e) => e._k === selected);
      if (!entry) continue;
      const errors = placed.entries[`${section}.${entry.name}`] || [];
      const Form = FORMS[section];
      form = html`<${Form} key=${entry._k} entry=${entry} draft=${draft} info=${own(check.profiles, entry.name)}
        autoFocus=${added === entry._k}
        errs=${(f) => messagesFor(errors, f)}
        update=${(e) => update(section, entry._k, e)}
        refs=${(taken) => takePlaces(taken, now.current.file, now.current.draft, section, entry.name)}
        rename=${(name, taken) => rename(section, entry._k, name, taken.places)}
        remove=${() => remove(section, entry._k)} />`;
    }
    const empty = draft.providers.length + draft.models.length + draft.profiles.length === 0;
    main = form || (empty
      ? html`<${Empty} path=${file.path} exists=${file.exists} onAdd=${add} />`
      : html`<p class="muted">Choose an entry on the left, or add one with +.</p>`);
  }

  const state = !file ? '' : !file.editable ? 'read only' : dirty ? 'unsaved' : file.exists ? 'saved' : 'not created yet';
  return html`
    <${TopBar} path=${file ? file.path : ''} state=${state} stale=${stale} errorCount=${check.errors.length} canSave=${saveOn}
      onSave=${save} onErrors=${() => setShowErrors(!showErrors)} />
    <${Banner} kind=${stopped ? 'stopped' : banner} onReload=${() => load('Reloaded from disk')} onKeep=${() => setBanner(null)} />
    <${Messages} items=${placed.global} />
    ${showErrors && html`<${ErrorList} errors=${check.errors} onGo=${goTo} onClose=${() => setShowErrors(false)} />`}
    <div class="layout">
      ${draft && html`<${Menu} draft=${draft} selected=${selected} counts=${placed.counts} modes=${check.profiles}
        sectionErrors=${placed.sections} onSelect=${(s, k) => setSelected(k)} onAdd=${add} />`}
      <main class="form">${main}</main>
    </div>
    ${toast && html`<div class="toast" role="status">${toast}</div>`}`;
}

takeToken();
render(html`<${App} />`, document.getElementById('app'));
