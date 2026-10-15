// layout.js holds the frame of the page: the top bar, the banners, the
// menu of entries and the screens that stand in for a form.

import { html } from './vendor/htm-preact-standalone-3.1.1.module.js';

export function TopBar({ path, state, errorCount, canSave, onSave, onErrors }) {
  return html`
    <header class="top">
      <strong>graft</strong>
      <code class="path">${path}</code>
      <span class=${'state ' + state}>${state === 'unsaved' ? '● unsaved' : state}</span>
      ${errorCount > 0 && html`
        <button type="button" class="secondary outline errors" onClick=${onErrors}>
          ${errorCount} ${errorCount === 1 ? 'error' : 'errors'}
        </button>`}
      <button type="button" disabled=${!canSave} onClick=${onSave}>Save</button>
    </header>`;
}

export function Banner({ kind, onReload, onKeep }) {
  if (kind === 'changed') {
    return html`
      <div class="banner warn" role="alert">
        <span>The file changed on disk. Reload it, or keep editing without saving, for example to copy your changes.</span>
        <button type="button" onClick=${onReload}>Reload</button>
        <button type="button" class="secondary outline" onClick=${onKeep}>Keep editing</button>
      </div>`;
  }
  if (kind === 'stopped') {
    return html`<div class="banner bad" role="alert">graft ui stopped: your unsaved changes are only in this tab.</div>`;
  }
  return null;
}

// Messages lists errors that belong to no form.
export function Messages({ items }) {
  if (items.length === 0) return null;
  return html`<div class="banner bad">${items.map((m) => html`<p>${m}</p>`)}</div>`;
}

// ErrorList lists every error; onGo(path) shows where an error is.
export function ErrorList({ errors, onGo, onClose }) {
  return html`
    <article class="error-list">
      <header>
        <strong>Errors</strong>
        <button type="button" class="secondary outline" onClick=${onClose}>Close</button>
      </header>
      <ul>
        ${errors.map((e) => html`
          <li><a href="#" onClick=${(ev) => { ev.preventDefault(); onGo(e.path); }}>
            <code>${e.path || 'config'}</code></a>: ${e.msg}</li>`)}
      </ul>
    </article>`;
}

// Menu lists the entries of the draft. counts has the number of errors by
// section, by "section.name" and for defaults; modes the mode of each valid
// profile; sectionErrors the errors of whole sections.
export function Menu({ draft, selected, counts, modes, sectionErrors, onSelect, onAdd }) {
  const section = (key, title) => html`
    <div class="sec">
      <span>${title}</span>
      ${counts[key] > 0 && html`<span class="badge">${counts[key]}</span>`}
      <button type="button" class="add" aria-label=${'Add a ' + title.toLowerCase().slice(0, -1)} onClick=${() => onAdd(key)}>+</button>
    </div>
    ${(sectionErrors[key] || []).map((m) => html`<small class="error sec-error">${m}</small>`)}
    <ul>
      ${draft[key].map((e) => html`
        <li>
          <a href="#" class=${selected === e._k ? 'sel' : ''} onClick=${(ev) => { ev.preventDefault(); onSelect(key, e._k); }}>
            <span class="mono">${e.name || '(no name)'}</span>
            ${key === 'profiles' && modes[e.name] && html`<span class=${'tag ' + modes[e.name].mode}>${modes[e.name].mode}</span>`}
            ${counts[`${key}.${e.name}`] > 0 && html`<span class="badge">${counts[`${key}.${e.name}`]}</span>`}
          </a>
        </li>`)}
    </ul>`;
  return html`
    <nav class="side">
      ${section('providers', 'Providers')}
      ${section('models', 'Models')}
      ${section('profiles', 'Profiles')}
      <div class="sec">
        <a href="#" class=${selected === 'defaults' ? 'sel' : ''} onClick=${(ev) => { ev.preventDefault(); onSelect('defaults', 'defaults'); }}>Defaults</a>
        ${counts.defaults > 0 && html`<span class="badge">${counts.defaults}</span>`}
      </div>
    </nav>`;
}

export function Empty({ path, exists, onAdd }) {
  return html`
    <article>
      <h2>${exists ? 'The config is empty' : 'No config yet'}</h2>
      <p>${exists ? path : `graft will create ${path} when you save.`} Add a provider, a model and a profile, then save.</p>
      <div class="grid">
        <button type="button" onClick=${() => onAdd('providers')}>Add a provider</button>
        <button type="button" class="secondary" onClick=${() => onAdd('models')}>Add a model</button>
        <button type="button" class="secondary" onClick=${() => onAdd('profiles')}>Add a profile</button>
      </div>
    </article>`;
}

export function NotEditable({ path, problems }) {
  const file = path.split('/').pop();
  return html`
    <article>
      <h2>The forms cannot show this file</h2>
      <ul>
        ${problems.map((p) => html`<li><code>${file}${p.line ? ':' + p.line : ''}</code>${p.path ? ' ' + p.path : ''}: ${p.msg}</li>`)}
      </ul>
      <p>Fix the file in your editor: this page reloads it when it changes.</p>
    </article>`;
}
