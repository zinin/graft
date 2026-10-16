// newcomer.js holds the screens for someone who starts with no config: the
// first-run screen, which offers the presets of providers, and the dialog Add
// provider, which puts a provider with its key, its models and their profiles
// into the draft.

import { html, useLayoutEffect, useRef, useState } from './vendor/htm-preact-standalone-3.1.1.module.js';
import { KeyWidget, TestButton } from './checks.js';
import { addProvider, defaultKeyPath, freeName, newName, renameProvider } from './draft.js';
import { Field } from './fields.js';

// PresetCards offers the presets, and Custom… for a provider that no preset
// describes. onPick is called with the preset, or with null for Custom….
export function PresetCards({ presets, onPick }) {
  const card = (title, note, preset) => html`
    <button type="button" class="card secondary outline" onClick=${() => onPick(preset)}>
      <strong>${title}</strong>
      <small>${note}</small>
    </button>`;
  return html`
    <div class="cards">
      ${presets.map((p) => card(p.title, new URL(p.url).host, p))}
      ${card('Custom…', 'any Anthropic Messages API', null)}
    </div>`;
}

// Welcome is the screen of a config with no providers, models or profiles.
// exists says whether the file is there, as an empty one.
export function Welcome({ path, exists, presets, onPick }) {
  return html`
    <article class="welcome">
      <h2>Welcome to graft</h2>
      <p>graft starts Claude Code on the models of other providers. Choose a provider to begin:</p>
      <${PresetCards} presets=${presets} onPick=${onPick} />
      <p class="muted">${exists ? `${path} has no providers, models or profiles yet.` : `graft will create ${path} when you save.`}</p>
    </article>`;
}

// firstName is the name the dialog starts with: that of the preset when it is
// free, else the first free one with a number. Custom… has no name of its own.
function firstName(draft, preset) {
  return preset ? freeName(draft.providers, preset.name) : newName(draft.providers, 'provider');
}

// AddProvider is a modal dialog that adds a provider to draft. preset is
// undefined while the user still has to choose one, null for Custom…, or a
// preset. onAdd is called with what addProvider makes of the choice, and
// onClose once the dialog has closed itself, by Cancel or by Esc: the page then
// drops it. The dialog stays the same element from the choice to the form, and
// opens once.
export function AddProvider({ preset, draft, presets, page, onAdd, onClose }) {
  const box = useRef(null);
  const [picked, setPicked] = useState(preset);
  // A layout effect, which runs before the effects of the form: the form puts
  // the cursor into its name as it appears, and an element of a dialog that is
  // not open yet takes no focus.
  useLayoutEffect(() => {
    box.current.showModal();
  }, []);
  // Cancel closes the dialog as Esc does, and onClose follows from its event
  // close, whoever closes it.
  const cancel = () => box.current.close();
  return html`
    <dialog ref=${box} aria-labelledby="add-provider-title" onClose=${onClose}>
      <article>
        <h3 id="add-provider-title">${picked ? `Add ${picked.title}` : 'Add a provider'}</h3>
        ${picked === undefined
          ? html`
            <${PresetCards} presets=${presets} onPick=${setPicked} />
            <footer><button type="button" class="secondary" onClick=${cancel}>Cancel</button></footer>`
          : html`<${AddForm} preset=${picked} draft=${draft} page=${page} onAdd=${onAdd} onCancel=${cancel} />`}
      </article>
    </dialog>`;
}

// AddForm is the form of the dialog, for preset, which is null for Custom….
function AddForm({ preset, draft, page, onAdd, onCancel }) {
  const presetModels = preset ? preset.models : [];
  const [first] = useState(() => firstName(draft, preset));
  const [name, setName] = useState(first);
  const [url, setUrl] = useState(preset ? preset.url : '');
  const [key, setKey] = useState({ kind: 'file', value: defaultKeyPath(page.file.path, first) });
  const [keySaved, setKeySaved] = useState(false); // Save key has written the key file
  const [checked, setChecked] = useState(() => presetModels.map((_, i) => i === 0)); // which models of the preset to add
  const [typed, setTyped] = useState([]); // the models typed in: {k, name, id, context}
  const [perModel, setPerModel] = useState(true);
  const [mix, setMix] = useState(false);
  const lastRow = useRef(0); // the last k given to a typed model: a row keeps its k, and so its Test, when another is removed

  const taken = draft.providers.some((p) => p.name === name);
  const models = [
    ...presetModels.filter((_, i) => checked[i]),
    ...typed.map((t) => ({ name: t.name, id: t.id, context: t.context, label: '' })),
  ];
  // The checkbox of mix is disabled, and so reads as off, while there is no
  // model for its picker.
  const choice = { name, url, key, models, perModel, mix: mix && models.length > 0, keySaved };
  const added = addProvider(draft, choice);
  // A model of the preset that is checked lies in the draft under the name that
  // addProvider gave it, which is numbered when another model has its name:
  // each row shows that one. The models that are not checked lie nowhere, and
  // show their own. A model typed in lies there too, after the checked ones.
  const names = presetModels.map((m, i) => (checked[i] ? added.models[checked.slice(0, i).filter(Boolean).length] : m.name));
  const typedNames = added.models.slice(added.models.length - typed.length);
  const canAdd = !taken && name.trim() !== '' && url.trim() !== '' && typed.every((t) => t.name.trim() !== '');

  function rename(to) {
    // Until Save key has written the key file, its path follows the name, as
    // for a new provider in its form.
    setKey((cur) => renameProvider({ was: '', name, key: cur, _keySaved: keySaved }, to, page.file.path).key);
    setName(to);
  }

  const target = (id) => (id === '' ? null : { url, key, id });
  const setRow = (k, field, value) => setTyped((all) => all.map((t) => (t.k === k ? { ...t, [field]: value } : t)));
  function addRow() {
    const k = ++lastRow.current;
    setTyped((all) => [...all, { k, name: '', id: '', context: '' }]);
  }

  const presetRow = (m, i) => html`
    <div class="model-row">
      <label>
        <input type="checkbox" checked=${checked[i]} onChange=${(e) => setChecked((all) => all.map((c, j) => (j === i ? e.target.checked : c)))} />
        <strong class="mono">${names[i]}</strong>
        <small class="mono">${m.id}</small>
        <small>${m.context.toLocaleString('en-US')} tokens</small>
      </label>
      <${TestButton} target=${target(m.id)} page=${page} />
    </div>`;
  // A row whose name another model has shows the name under which the model is
  // added. A row with no name yet shows none: Add to config is off until it
  // has one.
  const typedRow = (t, i) => html`
    <div class="model-row" key=${t.k}>
      <div class="row">
        <input class="mono" aria-label="Model name" placeholder="name" value=${t.name} onInput=${(e) => setRow(t.k, 'name', e.target.value)} />
        <input class="mono" aria-label="Model ID" placeholder="ID, if not the name" value=${t.id}
          onInput=${(e) => setRow(t.k, 'id', e.target.value)} />
        <input class="mono" aria-label="Context window" placeholder="context window" value=${t.context}
          onInput=${(e) => setRow(t.k, 'context', e.target.value)} />
        <button type="button" class="secondary outline" aria-label="Remove" onClick=${() => setTyped((all) => all.filter((x) => x.k !== t.k))}>×</button>
      </div>
      ${t.name.trim() !== '' && typedNames[i] !== t.name && html`
        <small>Another model has this name, so this one is added as <strong class="mono">${typedNames[i]}</strong>.</small>`}
      <${TestButton} target=${target(t.id || t.name)} page=${page} />
    </div>`;

  return html`
    ${preset && preset.docs && html`
      <p class="muted"><a href=${preset.docs} target="_blank" rel="noopener noreferrer">How ${preset.title} works with Claude Code</a></p>`}
    <${Field} label="Name" value=${name} onInput=${rename} errors=${taken ? ['another provider has this name'] : []}
      hint="lowercase letters, digits, '-' and '_'" autoFocus />
    <${Field} label="URL" value=${url} placeholder="https://ollama.com" onInput=${setUrl}
      hint="the base URL of the provider's Anthropic Messages API; Claude Code appends /v1/messages" />
    <${KeyWidget} value=${key} onChange=${setKey} provider=${taken ? '' : name} keysLink=${preset ? preset.keys : ''} page=${page}
      onSaved=${() => setKeySaved(true)} />
    <fieldset>
      <legend>Models</legend>
      ${presetModels.map(presetRow)}
      ${typed.map(typedRow)}
      <div class="row">
        <button type="button" class="secondary outline" onClick=${addRow}>+ Model</button>
        ${preset && preset.catalog && html`
          <small>Find the models at <a href=${preset.catalog} target="_blank" rel="noopener noreferrer">${preset.catalog}</a></small>`}
      </div>
    </fieldset>
    <fieldset>
      <legend>Profiles</legend>
      <label>
        <input type="checkbox" checked=${perModel} onChange=${(e) => setPerModel(e.target.checked)} />
        One profile per model
      </label>
      <label>
        <input type="checkbox" checked=${choice.mix} disabled=${models.length === 0} onChange=${(e) => setMix(e.target.checked)} />
        mix: Opus on your Claude subscription, with these models in /model
      </label>
    </fieldset>
    <footer>
      <button type="button" class="secondary" onClick=${onCancel}>Cancel</button>
      <button type="button" disabled=${!canAdd} onClick=${() => onAdd(added)}>Add to config</button>
    </footer>`;
}
