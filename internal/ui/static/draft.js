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

// renameProvider returns provider p named name. For a provider the file does
// not have yet (was is ''), a key file still at the default path of the old
// name follows the name. A saved provider keeps its key path, since renaming
// it does not move the key file.
export function renameProvider(p, name, configPath) {
  const dir = keysDir(configPath);
  const key = p.was === '' && p.key.kind === 'file' && p.key.value === `${dir}/${p.name}` ? { kind: 'file', value: `${dir}/${name}` } : p.key;
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

// carryName returns a copy of draft with name at each of paths, the
// references to an entry being renamed. A name no reference can mean, ''
// or a Claude name for a model, leaves them at their old value: they show
// as errors until a real name arrives, instead of being lost.
export function carryName(draft, section, paths, name) {
  if (name === '' || (section === 'models' && isClaude(name))) return structuredClone(draft);
  return setAt(draft, paths, name);
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

// within reports whether path is f or lies under it, as key.env lies under
// key and picker[1] under picker.
function within(path, f) {
  return path === f || path.startsWith(f + '.') || path.startsWith(f + '[');
}

// FIELDS are the fields each form has an input for. Besides them, the forms
// of profiles and defaults have an input for each item of args, for each
// item of picker (profiles only) and for each variable of env.
const FIELDS = {
  providers: ['url', 'key'],
  models: ['provider', 'id', 'context', 'label', 'description', 'settings'],
  profiles: ['model', 'subscription', 'subagents', 'effort', 'settings', ...ALIASES.map((a) => `aliases.${a}`)],
  defaults: ['effort', 'settings'],
};

// hasInput reports whether the form of section has an input that shows an
// error at field. A whole env, picker, args or aliases has none, nor has a
// field the form does not know.
function hasInput(section, field) {
  if (FIELDS[section].some((f) => within(field, f))) return true;
  if (section === 'providers' || section === 'models') return false;
  const items = section === 'profiles' ? /^(picker|args)\[\d+\]/ : /^args\[\d+\]/;
  return items.test(field) || field.startsWith('env.');
}

// placeErrors sorts errors by where the page shows them: global (no
// section), sections (by section), entries (by "section.name" or
// "defaults", as {field, msg}), and counts by the same keys. An error at a
// field its form has no input for goes to the head of the form, which is
// field '': its message names the field, since no input does.
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
      const key = at.section === 'defaults' ? 'defaults' : `${at.section}.${at.name}`;
      const noInput = at.field !== '' && !hasInput(at.section, at.field);
      add(out.entries, key, noInput ? { field: '', msg: `${at.field}: ${e.msg}` } : { field: at.field, msg: e.msg });
    }
  }
  return out;
}

// messagesFor returns the messages of an entry's errors about field: the
// field itself and anything under it, such as picker[1] under picker.
// field '' means the entry itself.
export function messagesFor(errors, field) {
  return errors.filter((e) => (field === '' ? e.field === '' : within(e.field, field))).map((e) => e.msg);
}

// isSaveKey reports whether keydown event e asks to save: Ctrl or Cmd with S.
// S is the letter s, in capitals too. On a layout with no Latin letters, such
// as the Russian one, where that key types ы, it is the key in the place of S.
// A Latin layout that has put another letter there keeps it: Ctrl+O on Dvorak
// and Ctrl+R on Colemak are not saves. Alt rules it out, since AltGr, which
// browsers report as Ctrl+Alt, types letters such as ś.
export function isSaveKey({ key, code, ctrlKey, metaKey, altKey }) {
  if (!(ctrlKey || metaKey) || altKey) return false;
  return /^[a-z]$/i.test(key) ? key.toLowerCase() === 's' : code === 'KeyS';
}
