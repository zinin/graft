// fields.js holds the inputs the forms are made of.

import { html, useEffect, useRef, useState } from './vendor/htm-preact-standalone-3.1.1.module.js';
import { ALIASES } from './draft.js';

// Help shows the errors of a field, or its hint when there are none.
export function Help({ errors = [], hint }) {
  if (errors.length > 0) return html`<small class="error">${errors.join('; ')}</small>`;
  return hint ? html`<small>${hint}</small>` : null;
}

const invalid = (errors) => (errors.length > 0 ? 'true' : null);

// Field is a labelled text input.
export function Field({ label, value, onInput, hint, errors = [], placeholder = '', mono = true }) {
  return html`
    <label>
      ${label}
      <input class=${mono ? 'mono' : ''} value=${value} placeholder=${placeholder} aria-invalid=${invalid(errors)}
        onInput=${(e) => onInput(e.target.value)} />
      <${Help} errors=${errors} hint=${hint} />
    </label>`;
}

// NameField edits the name of an entry. The references to the entry that
// renames carries along are taken when the field gets the focus, while the
// name is still the entry's own. They are positions in the draft, and a
// reload replaces the draft under a field that keeps the focus: the positions
// it holds may not fit the new one. So at each input the field hands what it
// holds back to refs, which returns it while it is still good and takes new
// ones when it is not. Taking the focus away at a reload would do less: it
// puts the page's DOM into load, an input that comes with no focus event
// would still use the old places, and the user would lose the cursor.
export function NameField({ value, refs, onRename, errors = [], hint, autoFocus }) {
  const taken = useRef(null); // what refs gave last
  const input = useRef(null);
  useEffect(() => {
    if (autoFocus && input.current) {
      input.current.focus();
      input.current.select();
    }
  }, [autoFocus]);
  return html`
    <label>
      Name
      <input ref=${input} class="mono" value=${value} aria-invalid=${invalid(errors)}
        onFocus=${() => { taken.current = refs(null); }}
        onInput=${(e) => {
          taken.current = refs(taken.current);
          onRename(e.target.value, taken.current);
        }} />
      <${Help} errors=${errors} hint=${hint} />
    </label>`;
}

// Select is a labelled select. A value that is not among options, such as
// an unknown model in the file, is shown as an option of its own.
export function Select({ label, value, options, onChange, hint, errors = [] }) {
  const all = options.some((o) => o.value === value) ? options : [...options, { value, label: value }];
  return html`
    <label>
      ${label}
      <select aria-invalid=${invalid(errors)} onChange=${(e) => onChange(e.target.value)}>
        ${all.map((o) => html`<option value=${o.value} selected=${o.value === value} disabled=${o.disabled}>${o.label}</option>`)}
      </select>
      <${Help} errors=${errors} hint=${hint} />
    </label>`;
}

const CLAUDE_ID = '\u0000claude-id';

// ModelRef chooses a model: one of the config, or a Claude model of the
// subscription. claude is 'aliases' for opus, sonnet, haiku, fable and any
// claude-… ID, or 'ids' for claude-… IDs only, as aliases take. noClaude
// turns the Claude models off.
export function ModelRef({ label, value, models, claude, noClaude, empty, onChange, hint, errors = [] }) {
  const isID = value.toLowerCase().startsWith('claude-');
  const options = [{ value: '', label: empty }, ...models.map((m) => ({ value: m, label: m }))];
  if (claude === 'aliases') {
    for (const a of ALIASES) options.push({ value: a, label: `${a} (Claude)`, disabled: noClaude });
  }
  options.push({ value: CLAUDE_ID, label: 'Claude model ID…', disabled: noClaude });
  return html`
    <div class="ref">
      <${Select} label=${label} value=${isID ? CLAUDE_ID : value} options=${options} errors=${errors} hint=${hint}
        onChange=${(v) => onChange(v === CLAUDE_ID ? 'claude-' : v)} />
      ${isID && html`<input class="mono" value=${value} aria-label=${label + ' ID'} onInput=${(e) => onChange(e.target.value)} />`}
    </div>`;
}

// YamlField edits YAML text in a section that opens when it has text or
// errors, and that only the user closes.
export function YamlField({ label, value, onInput, hint, errors = [] }) {
  const box = useRef(null);
  useEffect(() => {
    if (box.current && (value !== '' || errors.length > 0)) box.current.open = true;
  }, [errors.length]);
  return html`
    <details ref=${box}>
      <summary>${label}</summary>
      <textarea class="mono" rows="6" spellcheck="false" value=${value} aria-invalid=${invalid(errors)}
        onInput=${(e) => onInput(e.target.value)}></textarea>
      <${Help} errors=${errors} hint=${hint} />
    </details>`;
}

// OrderedList edits a list of strings in order: each item has ↑, ↓ and ×.
// With choices, new items come from a select of them; without, a new
// item is an empty text input. errorsAt(i) returns the errors of item i.
export function OrderedList({ label, items, onChange, choices, addLabel, hint, errorsAt = () => [] }) {
  const move = (i, j) => {
    const next = [...items];
    [next[i], next[j]] = [next[j], next[i]];
    onChange(next);
  };
  const [pick, setPick] = useState('');
  const left = choices ? choices.filter((c) => !items.includes(c)) : [];
  return html`
    <fieldset>
      <legend>${label}</legend>
      ${items.map((item, i) => html`
        <div class="row">
          ${choices
            ? html`<span class="mono item" aria-invalid=${invalid(errorsAt(i))}>${item}</span>`
            : html`<input class="mono" value=${item} aria-invalid=${invalid(errorsAt(i))}
                onInput=${(e) => onChange(items.map((x, k) => (k === i ? e.target.value : x)))} />`}
          <button type="button" class="secondary outline" disabled=${i === 0} aria-label="Up" onClick=${() => move(i, i - 1)}>↑</button>
          <button type="button" class="secondary outline" disabled=${i === items.length - 1} aria-label="Down" onClick=${() => move(i, i + 1)}>↓</button>
          <button type="button" class="secondary outline" aria-label="Remove" onClick=${() => onChange(items.filter((_, k) => k !== i))}>×</button>
        </div>
        <${Help} errors=${errorsAt(i)} />`)}
      ${choices
        ? html`<div class="row">
            <select value=${pick} onChange=${(e) => setPick(e.target.value)}>
              <option value="">${left.length ? '— add a model —' : '— no more models —'}</option>
              ${left.map((c) => html`<option value=${c}>${c}</option>`)}
            </select>
            <button type="button" class="secondary" disabled=${pick === ''} onClick=${() => { onChange([...items, pick]); setPick(''); }}>Add</button>
          </div>`
        : html`<button type="button" class="secondary outline" onClick=${() => onChange([...items, ''])}>${addLabel}</button>`}
      <${Help} hint=${hint} />
    </fieldset>`;
}

// EnvEditor edits variables: a name and a value per row. errorsOf(name)
// returns the errors of a variable.
export function EnvEditor({ pairs, onChange, errorsOf }) {
  const set = (i, j, v) => onChange(pairs.map((p, k) => (k !== i ? p : j === 0 ? [v, p[1]] : [p[0], v])));
  return html`
    <fieldset>
      <legend>Env</legend>
      ${pairs.map((p, i) => html`
        <div class="row">
          <input class="mono" placeholder="NAME" value=${p[0]} aria-invalid=${invalid(errorsOf(p[0]))} onInput=${(e) => set(i, 0, e.target.value)} />
          <input class="mono" placeholder="value" value=${p[1]} onInput=${(e) => set(i, 1, e.target.value)} />
          <button type="button" class="secondary outline" aria-label="Remove" onClick=${() => onChange(pairs.filter((_, k) => k !== i))}>×</button>
        </div>
        <${Help} errors=${errorsOf(p[0])} />`)}
      <button type="button" class="secondary outline" onClick=${() => onChange([...pairs, ['', '']])}>+ Variable</button>
      <${Help} hint="Values are strings; an empty value is an empty string." />
    </fieldset>`;
}

const KIND_HINTS = {
  env: 'a variable of the shell that runs graft',
  file: 'a file that holds only the key; only you may read it (chmod 600)',
  command: 'a command that prints the key; it may ask for a password in the terminal',
};
const KIND_PLACEHOLDERS = { env: 'OLLAMA_API_KEY', file: '~/.config/graft/keys/ollama', command: 'pass show ollama' };

// KeySource chooses where graft reads a provider's key. The key itself
// never goes into the config.
export function KeySource({ value, onChange, errors = [] }) {
  return html`
    <fieldset>
      <legend>Key</legend>
      <div role="group">
        ${['env', 'file', 'command'].map((k) => html`
          <button type="button" class=${value.kind === k ? '' : 'secondary outline'} aria-pressed=${value.kind === k}
            onClick=${() => onChange({ kind: k, value: value.kind === k ? value.value : '' })}>${k}</button>`)}
      </div>
      <input class="mono" value=${value.value} placeholder=${KIND_PLACEHOLDERS[value.kind] || ''} disabled=${!value.kind}
        aria-label="Key source" aria-invalid=${invalid(errors)} onInput=${(e) => onChange({ kind: value.kind, value: e.target.value })} />
      <${Help} errors=${errors} hint=${value.kind ? KIND_HINTS[value.kind] : 'Choose where graft reads the key.'} />
    </fieldset>`;
}
