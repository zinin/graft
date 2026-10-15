// draft.js computes what the page needs from a draft. It touches no DOM,
// so that node --test can test it.

export const ALIASES = ['opus', 'sonnet', 'haiku', 'fable'];
export const EFFORTS = ['low', 'medium', 'high', 'xhigh', 'max'];
export const SECTIONS = ['providers', 'models', 'profiles'];

let lastKey = 0;

// withKeys gives each entry without one a key of the page, _k, that
// survives renames. The server ignores it.
export function withKeys(draft) {
  for (const s of SECTIONS) {
    for (const e of draft[s]) {
      if (!e._k) e._k = `k${++lastKey}`;
    }
  }
  return draft;
}

// carryKeys gives the entries of next the keys of the entries of prev
// with the same names, so that a saved draft keeps the page's selection.
export function carryKeys(prev, next) {
  for (const s of SECTIONS) {
    for (const e of next[s]) {
      const old = prev[s].find((o) => o.name === e.name);
      if (old) e._k = old._k;
    }
  }
  return withKeys(next);
}

// sameDraft reports whether a and b hold the same values; keys of the
// page and the order of fields do not count.
export function sameDraft(a, b) {
  return JSON.stringify(canon(a)) === JSON.stringify(canon(b));
}

function canon(v) {
  if (Array.isArray(v)) return v.map(canon);
  if (v && typeof v === 'object') {
    const out = {};
    for (const k of Object.keys(v).sort()) {
      if (k !== '_k') out[k] = canon(v[k]);
    }
    return out;
  }
  return v;
}

export function emptyDraft() {
  return { providers: [], models: [], defaults: { effort: '', settings: '', env: [], args: [] }, profiles: [] };
}

// keysDir is the directory of key files next to the config at path.
export function keysDir(path) {
  return path.replace(/[^/]*$/, '') + 'keys';
}

export function newProvider(name, configPath) {
  return { was: '', name, url: '', key: { kind: 'file', value: `${keysDir(configPath)}/${name}` } };
}

// renameProvider returns provider p named name. A key file at the default
// path of the old name, which the user has not changed, follows the name.
export function renameProvider(p, name, configPath) {
  const dir = keysDir(configPath);
  const key = p.key.kind === 'file' && p.key.value === `${dir}/${p.name}` ? { kind: 'file', value: `${dir}/${name}` } : p.key;
  return { ...p, name, key };
}

export function newModel(name, provider) {
  return { was: '', name, provider, id: '', context: '', label: '', description: '', settings: '' };
}

export function newProfile(name) {
  return {
    was: '', name, model: '', subscription: '', picker: [],
    aliases: { opus: '', sonnet: '', haiku: '', fable: '' },
    subagents: '', effort: '', settings: '', env: [], args: [],
  };
}

// newName returns base-1, base-2 or the first such name no entry has.
export function newName(entries, base) {
  const used = new Set(entries.map((e) => e.name));
  for (let n = 1; ; n++) {
    if (!used.has(`${base}-${n}`)) return `${base}-${n}`;
  }
}

// isClaude reports whether a profile's model is a model of the
// subscription: an alias or a claude-… ID.
export function isClaude(model) {
  return ALIASES.includes(model) || model.toLowerCase().startsWith('claude-');
}

// subscribed reports whether profile p uses the subscription, as graft
// decides: subscription when set, else whether the model is Claude's.
export function subscribed(p) {
  return p.subscription === '' ? isClaude(p.model) : p.subscription === 'true';
}

// refsTo returns where draft names the model or provider `name`: paths of
// keys into the draft. Models are named by the model, subagents, picker and
// aliases of profiles; providers by the provider of models. An empty name is
// an unset field, not a name, and a Claude name is Claude's model, not an
// entry of the draft: neither names an entry, so nothing is returned.
export function refsTo(draft, section, name) {
  const out = [];
  if (name === '' || (section === 'models' && isClaude(name))) return out;
  if (section === 'models') {
    draft.profiles.forEach((p, i) => {
      if (p.model === name) out.push(['profiles', i, 'model']);
      if (p.subagents === name) out.push(['profiles', i, 'subagents']);
      p.picker.forEach((m, j) => {
        if (m === name) out.push(['profiles', i, 'picker', j]);
      });
      for (const a of ALIASES) {
        if (p.aliases[a] === name) out.push(['profiles', i, 'aliases', a]);
      }
    });
  } else if (section === 'providers') {
    draft.models.forEach((m, i) => {
      if (m.provider === name) out.push(['models', i, 'provider']);
    });
  }
  return out;
}

// renameRefs returns the references to rename with entry `name` of section:
// none when another entry has the same name, since then they are not only
// this entry's.
export function renameRefs(draft, section, name) {
  if (draft[section].filter((e) => e.name === name).length !== 1) return [];
  return refsTo(draft, section, name);
}

// setAt returns a copy of draft with value at each of paths.
export function setAt(draft, paths, value) {
  const d = structuredClone(draft);
  for (const path of paths) {
    let o = d;
    for (const k of path.slice(0, -1)) o = o[k];
    o[path[path.length - 1]] = value;
  }
  return d;
}

// usedBy names the entries that refer to entry `name` of section. As in
// refsTo, an empty name or a Claude name refers to none.
export function usedBy(draft, section, name) {
  if (name === '' || (section === 'models' && isClaude(name))) return [];
  if (section === 'providers') {
    return draft.models.filter((m) => m.provider === name).map((m) => `model ${m.name}`);
  }
  if (section === 'models') {
    return draft.profiles
      .filter((p) => p.model === name || p.subagents === name || p.picker.includes(name) || ALIASES.some((a) => p.aliases[a] === name))
      .map((p) => `profile ${p.name}`);
  }
  return [];
}

// locate finds what an error path is about: {section, name, field}. name is
// the entry ('' for defaults, null for the section itself) and field the
// rest of the path ('' for the entry itself). It returns null for a path of
// no section. Names may hold dots, so the longest name that fits wins.
export function locate(draft, path) {
  if (path === 'defaults' || path.startsWith('defaults.')) {
    return { section: 'defaults', name: '', field: path.slice('defaults.'.length) };
  }
  for (const s of SECTIONS) {
    if (path === s) return { section: s, name: null, field: '' };
    if (!path.startsWith(s + '.')) continue;
    const rest = path.slice(s.length + 1);
    let best = null;
    for (const e of draft[s]) {
      const fits = rest === e.name || rest.startsWith(e.name + '.') || rest.startsWith(e.name + '[');
      if (fits && (best === null || e.name.length > best.length)) best = e.name;
    }
    if (best === null) return { section: s, name: null, field: '' };
    return { section: s, name: best, field: rest.slice(best.length).replace(/^\./, '') };
  }
  return null;
}

// placeErrors sorts errors by where the page shows them: global (no
// section), sections (by section), entries (by "section.name" or
// "defaults", as {field, msg}), and counts by the same keys.
export function placeErrors(draft, errors) {
  const out = { global: [], sections: {}, entries: {}, counts: {} };
  const add = (map, key, v) => {
    (map[key] = map[key] || []).push(v);
    out.counts[key] = (out.counts[key] || 0) + 1;
  };
  for (const e of errors) {
    const at = locate(draft, e.path);
    if (at === null) {
      out.global.push(e.path ? `${e.path}: ${e.msg}` : e.msg);
    } else if (at.name === null) {
      add(out.sections, at.section, e.path === at.section ? e.msg : `${e.path}: ${e.msg}`);
    } else {
      add(out.entries, at.section === 'defaults' ? 'defaults' : `${at.section}.${at.name}`, { field: at.field, msg: e.msg });
    }
  }
  return out;
}

// messagesFor returns the messages of an entry's errors about field: the
// field itself and anything under it, such as picker[1] under picker.
// field '' means the entry itself.
export function messagesFor(errors, field) {
  return errors
    .filter((e) => (field === '' ? e.field === '' : e.field === field || e.field.startsWith(field + '.') || e.field.startsWith(field + '[')))
    .map((e) => e.msg);
}
