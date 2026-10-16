// app.js is the page of graft ui: it loads the draft of the config, checks
// each change on the server, saves, and notices changes of the file.

import { html, render, useEffect, useRef, useState } from './vendor/htm-preact-standalone-3.1.1.module.js';
import { ApiError, call, takeToken } from './api.js';
import {
  canSave, carryKeys, carryName, isSaveKey, keepTyped, locate, messagesFor, newModel, newName, newProfile,
  own, placeErrors, renameProvider, runHint, sameDraft, takePlaces, usedBy, withKeys, writtenProfiles,
} from './draft.js';
import { DefaultsForm, ModelForm, ProfileForm, ProviderForm } from './forms.js';
import { Banner, ErrorList, Hint, Menu, Messages, NotEditable, TopBar } from './layout.js';
import { AddProvider, Welcome } from './newcomer.js';

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
  const [presets, setPresets] = useState([]); // the providers /api/presets lists; [] until it answers
  const [dialog, setDialog] = useState(null); // the dialog Add provider: {preset}, or null while it is closed
  const [pending, setPending] = useState([]); // the _k of each profile the dialogs added, until a save writes it
  const [hint, setHint] = useState(''); // how to start the profiles the last save wrote

  const dirty = draft !== null && saved !== null && !sameDraft(draft, saved);
  // The Save button and Ctrl+S follow one rule.
  const saveOn = canSave({ dirty, stale, saving, stopped, errors: check.errors.length });
  // Timers and listeners read the state through now.
  const now = useRef({});
  now.current = { file, draft, dirty, stale, saving, stopped, pending, canSave: saveOn };

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
        setDialog(null); // it adds to a draft that is gone
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

  // request is call for the parts of the forms that ask the server themselves.
  // As with trouble, no answer means the server is gone for good, and the tab
  // stops asking; the error goes on, for the part to show it at its own place.
  async function request(method, path, body) {
    try {
      return await call(method, path, body);
    } catch (e) {
      if (!(e instanceof ApiError)) setStopped(true);
      throw e;
    }
  }

  useEffect(() => {
    load();
  }, []);

  // The presets come once. The page works without them, and only the link to
  // the page for keys is missing from a provider form, so a failure is silent.
  useEffect(() => {
    (async () => {
      try {
        const res = await call('GET', '/api/presets');
        if (Array.isArray(res.providers)) setPresets(res.providers);
      } catch {
        // Nothing to tell: the list stays empty.
      }
    })();
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
        // its own doing, and so may be one that comes back while a save is
        // under way, since the save writes the file before it answers. A
        // change by someone else is not lost: the save fails with a 409,
        // which calls changed(), or the watch sees it after the save.
        // An answer that is not stale counts only while the page still holds
        // the draft and the file it was for. Two checks can overlap, and the
        // older can come back last: its answer would cover that of the
        // current draft, and leave Save off for a draft that passed, or on
        // for one that did not. A new draft or file runs this effect again,
        // so its own check is already due or under way. The page changes no
        // draft or file in place, so comparing the objects is enough.
        if (res.stale && file.version === now.current.file.version && !now.current.saving) changed();
        else if (!res.stale && draft === now.current.draft && file === now.current.file) setChecked(res);
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
      const held = keepTyped(now.current.draft, d, next);
      setDraft(held);
      setToast('Saved');
      // The profiles of the dialogs that this save wrote can be started now:
      // the hint names them as the file has them.
      const { names, left } = writtenProfiles(now.current.pending, next, held);
      if (names.length > 0) setHint(runHint(names));
      setPending(left);
    } catch (e) {
      if (e instanceof ApiError && e.status === 409) {
        changed();
      } else if (e instanceof ApiError && e.status === 422) {
        // The errors are those of the draft that went out, so they count
        // only while the page still holds it. A change during the save made
        // a new draft, and its own check, which may have answered already,
        // has the say.
        if (now.current.draft === d) setChecked({ errors: e.body.errors, profiles: {} });
      } else {
        trouble(e);
      }
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
    // A provider comes with its key, and often its models and profiles: that is
    // the dialog, not a form.
    if (section === 'providers') {
      setDialog({ preset: undefined });
      return;
    }
    const d = structuredClone(draft);
    const name = newName(d[section], BASES[section]);
    const entry = section === 'models' ? newModel(name, d.providers.length ? d.providers[0].name : '') : newProfile(name);
    d[section].push(entry);
    withKeys(d);
    setDraft(d);
    setSelected(entry._k);
    setAdded(entry._k);
  }

  // addFromDialog takes what the dialog Add provider made of its choice: the
  // draft, and the names of the profiles it added. The form of the first of them
  // opens, or the form of the provider when there is none. The marker added is
  // left out: the user has just named them all in the dialog. The profiles wait
  // for a save with those of the dialogs before, by their keys of the page,
  // which a rename keeps.
  function addFromDialog({ draft: next, profiles }) {
    const added = profiles.map((name) => next.profiles.find((p) => p.name === name));
    setDraft(next);
    setSelected((added.length > 0 ? added[0] : next.providers.at(-1))._k);
    setPending((all) => [...all, ...added.map((p) => p._k)]);
    setDialog(null);
  }

  // update replaces an entry of the draft with entry, or with what entry
  // returns when it is a function, called with the entry the draft holds by
  // then. A request that answers later changes the entry that way: the user
  // may have typed in its form meanwhile, and a copy taken before the request
  // would drop that.
  function update(section, k, entry) {
    const replace = (e) => (typeof entry === 'function' ? entry(e) : entry);
    setDraft((d) => ({ ...d, [section]: d[section].map((e) => (e._k === k ? replace(e) : e)) }));
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
  // What the parts of the forms that ask the server need of the page.
  const page = { file, presets, request, stopped, errors: check.errors.length };

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
        autoFocus=${added === entry._k} page=${page}
        errs=${(f) => messagesFor(errors, f)}
        update=${(e) => update(section, entry._k, e)}
        refs=${(taken) => takePlaces(taken, now.current.file, now.current.draft, section, entry.name)}
        rename=${(name, taken) => rename(section, entry._k, name, taken.places)}
        remove=${() => remove(section, entry._k)} />`;
    }
    const empty = draft.providers.length + draft.models.length + draft.profiles.length === 0;
    main = form || (empty
      ? html`<${Welcome} path=${file.path} exists=${file.exists} presets=${presets} onPick=${(preset) => setDialog({ preset })} />`
      : html`<p class="muted">Choose an entry on the left, or add one with +.</p>`);
  }

  const state = !file ? '' : !file.editable ? 'read only' : dirty ? 'unsaved' : file.exists ? 'saved' : 'not created yet';
  return html`
    <${TopBar} path=${file ? file.path : ''} state=${state} stale=${stale} errorCount=${check.errors.length} canSave=${saveOn}
      onSave=${save} onErrors=${() => setShowErrors(!showErrors)} />
    <${Banner} kind=${stopped ? 'stopped' : banner} onReload=${() => load('Reloaded from disk')} onKeep=${() => setBanner(null)} />
    <${Hint} text=${hint} onClose=${() => setHint('')} />
    <${Messages} items=${placed.global} />
    ${showErrors && html`<${ErrorList} errors=${check.errors} onGo=${goTo} onClose=${() => setShowErrors(false)} />`}
    <div class="layout">
      ${draft && html`<${Menu} draft=${draft} selected=${selected} counts=${placed.counts} modes=${check.profiles}
        sectionErrors=${placed.sections} onSelect=${(s, k) => setSelected(k)} onAdd=${add} />`}
      <main class="form">${main}</main>
    </div>
    ${dialog && html`<${AddProvider} preset=${dialog.preset} draft=${draft} presets=${presets} page=${page}
      onAdd=${addFromDialog} onClose=${() => setDialog(null)} />`}
    ${toast && html`<div class="toast" role="status">${toast}</div>`}`;
}

takeToken();
render(html`<${App} />`, document.getElementById('app'));
