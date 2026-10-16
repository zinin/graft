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

// sameDraft reports whether a and b hold the same values; the order of fields
// and the fields of the page do not count. Those are the fields whose names
// start with _: the key of an entry, _k, and the flag _keySaved of a provider
// (see renameProvider).
export function sameDraft(a, b) {
  return JSON.stringify(canon(a)) === JSON.stringify(canon(b));
}

function canon(v) {
  if (Array.isArray(v)) return v.map(canon);
  if (v && typeof v === 'object') {
    const out = {};
    for (const k of Object.keys(v).sort()) {
      if (!k.startsWith('_')) out[k] = canon(v[k]);
    }
    return out;
  }
  return v;
}

// keepTyped returns the draft the page takes once the server has saved sent
// and answered with next, whose keys carryKeys took from sent or made anew.
// current is the page's draft by then, null when the page shows none. When
// there is none, or it holds what sent holds, the page takes next. Otherwise
// the user typed during the save, and next would drop that, so the page keeps
// current: it reads as unsaved until the next save sends it. The server
// matches the entries of that save to the file by was, which this save may
// have changed by renaming or adding entries. So each entry of current takes
// the was of the entry of next with its _k: the same entry, since a rename
// keeps the _k. An entry added during the save has a _k that next lacks, and
// keeps its empty was.
export function keepTyped(current, sent, next) {
  if (!current || sameDraft(current, sent)) return next;
  const kept = structuredClone(current);
  for (const s of SECTIONS) {
    for (const e of kept[s]) {
      const written = next[s].find((n) => n._k === e._k);
      if (written) e.was = written.was;
    }
  }
  return kept;
}

export function emptyDraft() {
  return { providers: [], models: [], defaults: { effort: '', settings: '', env: [], args: [] }, profiles: [] };
}

// hasEntry reports whether draft has a provider, a model or a profile. A draft
// with none, as a missing or empty file gives, is that of the first-run screen.
export function hasEntry(draft) {
  return SECTIONS.some((s) => draft[s].length > 0);
}

// keysDir is the directory of key files next to the config at path.
export function keysDir(path) {
  return path.replace(/[^/]*$/, '') + 'keys';
}

// defaultKeyPath is the key file of provider name next to the config at
// configPath: the file Save key writes.
export function defaultKeyPath(configPath, name) {
  return `${keysDir(configPath)}/${name}`;
}

export function newProvider(name, configPath) {
  return { was: '', name, url: '', key: { kind: 'file', value: defaultKeyPath(configPath, name) } };
}

// renameProvider returns provider p named name. For a provider the file does
// not have yet (was is ''), a key file still at the default path of the old
// name follows the name. A saved provider keeps its key path, since renaming
// it does not move the key file. A provider with _keySaved, which the page
// sets once Save key has written its key file, keeps its key path too: the
// file stays where Save key wrote it, so only the name changes.
export function renameProvider(p, name, configPath) {
  const follows = p.was === '' && !p._keySaved && p.key.kind === 'file' && p.key.value === defaultKeyPath(configPath, p.name);
  return { ...p, name, key: follows ? { kind: 'file', value: defaultKeyPath(configPath, name) } : p.key };
}

// pickKind returns what key source becomes after a click on kind: the same
// source when it has that kind, else an empty one, since the value of one kind
// means nothing to another. A file left empty takes defaultFile, the key file
// that Save key writes for the provider, '' when the provider has no name: a
// newcomer who tries env and comes back to file gets that path back, and Save
// key with it.
export function pickKind(source, kind, defaultFile) {
  const value = source.kind === kind ? source.value : '';
  return { kind, value: kind === 'file' && value === '' ? defaultFile : value };
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

// freeName returns name when no entry has it, else name-2, name-3 or the
// first such name that no entry has.
export function freeName(entries, name) {
  const used = new Set(entries.map((e) => e.name));
  let free = name;
  for (let n = 2; used.has(free); n++) free = `${name}-${n}`;
  return free;
}

// The names that package config takes for providers and models, and those it
// keeps for itself.
const PROVIDER_NAME = /^[a-z0-9_-]+$/;
const RESERVED_PROVIDERS = ['anthropic', 'graft'];
const MODEL_NAME = /^[A-Za-z0-9][A-Za-z0-9._-]*$/;
const RESERVED_MODELS = ['default', 'best', 'opus', 'sonnet', 'haiku', 'fable', 'opusplan', 'inherit'];

// providerNameError says what is wrong with name as the name of a provider,
// by the rules of package config (see config.CheckProviderName), and returns
// '' when the name is fine. An empty name gets '' too: the dialog Add provider
// keeps Add to config off for it, with no message.
export function providerNameError(name) {
  if (name === '') return '';
  if (!PROVIDER_NAME.test(name)) return "a provider name may contain lowercase letters, digits, '-' and '_'";
  if (RESERVED_PROVIDERS.includes(name)) return `"${name}" is reserved`;
  return '';
}

// typedModelError says what is wrong with a model typed into the dialog Add
// provider, by the rules of package config for models (see config.Parse): the
// first rule that its name breaks, else that its context window breaks, each
// taken without white space at the ends. It returns '' when the model is fine.
// An empty name or context window gets no message: the dialog keeps Add to
// config off for it.
export function typedModelError({ name, context }) {
  const n = name.trim();
  const c = context.trim();
  if (n !== '') {
    const lower = n.toLowerCase();
    if (!MODEL_NAME.test(n)) {
      return "a model name may contain letters, digits, '.', '_' and '-', and starts with a letter or digit; " +
        'a model ID such as vendor/model goes into the field ID';
    }
    if (lower.startsWith('claude')) return 'a model name must not start with "claude": such names belong to the subscription';
    if (RESERVED_MODELS.includes(lower)) return `"${n}" is a Claude Code alias`;
  }
  if (c !== '' && !(/^[0-9]+$/.test(c) && Number(c) >= 100000)) {
    return 'the context window must be a whole number of tokens, 100000 or more';
  }
  return '';
}

// addProvider returns what the dialog Add provider adds to draft: a copy of
// draft with the provider of choice (name, url and key), a model of it for
// each of choice.models ({name, id, context, label}), and the profiles: with
// choice.perModel one on each model, named after it; with choice.mix, and a
// model at least, mix, with opus as its model and the models in its picker.
// A model or a profile whose name is taken gets the first free one (see
// freeName). A model with no id that so gets another name takes the name it
// was given as its id, so that the config asks the provider for the model
// that the dialog tested. It returns the draft and the names of the models
// and of the profiles it added. choice.keySaved, which the dialog sets once
// its Save key has succeeded, gives the provider the flag _keySaved (see
// renameProvider); without it the provider has no such field.
export function addProvider(draft, choice) {
  const next = structuredClone(draft);
  const provider = { was: '', name: choice.name, url: choice.url, key: { ...choice.key } };
  if (choice.keySaved) provider._keySaved = true;
  next.providers.push(provider);
  // next holds what this call has added so far, so the names that freeName
  // takes are free among those, too: two models of one name get two names.
  const models = [];
  for (const m of choice.models) {
    const name = freeName(next.models, m.name);
    const id = m.id || (name !== m.name ? m.name : '');
    next.models.push({ ...newModel(name, choice.name), id, context: String(m.context ?? ''), label: m.label ?? '' });
    models.push(name);
  }
  const profiles = [];
  const addProfile = (name, fields) => {
    const free = freeName(next.profiles, name);
    next.profiles.push({ ...newProfile(free), ...fields });
    profiles.push(free);
  };
  // A profile refers to its model by the name the model was added under.
  if (choice.perModel) {
    for (const name of models) addProfile(name, { model: name });
  }
  if (choice.mix && models.length > 0) addProfile('mix', { model: 'opus', picker: [...models] });
  return { draft: withKeys(next), models, profiles };
}

// presetFor returns the preset whose URL is url, a slash at the end aside,
// or null: the provider form takes the page for keys from it.
export function presetFor(presets, url) {
  if (url === '') return null;
  const bare = (u) => u.replace(/\/$/, '');
  return presets.find((p) => bare(p.url) === bare(url)) ?? null;
}

// testTarget returns what the Test of model m sends: the URL and the key of
// its provider in draft, and its id, which defaults to its name; null while
// draft has no such provider.
export function testTarget(draft, m) {
  const p = draft.providers.find((q) => q.name === m.provider);
  return p ? { url: p.url, key: { ...p.key }, id: m.id || m.name } : null;
}

// runHint tells how to start the profiles names once a save has written
// them. config is the path of the file when graft ui took it from --config,
// else '': graft then needs --config too to read that file, and each command
// names it. With no names there is nothing to tell, and it returns ''.
export function runHint(names, config = '') {
  if (names.length === 0) return '';
  const graft = config === '' ? 'graft' : `graft --config ${shellWord(config)}`;
  const commands = names.map((n) => `${graft} ${n}`);
  const list = commands.length === 1 ? commands[0] : `${commands.slice(0, -1).join(', ')} or ${commands.at(-1)}`;
  return `Run ${list} in a terminal.`;
}

// writtenProfiles sorts pending, the keys of the page (_k) of the profiles that
// the dialogs Add provider added and no save has written yet, after a save.
// written is the draft that the save wrote, keyed by carryKeys from the draft
// sent, and held the draft that the page keeps after it (see keepTyped). names
// are the names that written gives the pending profiles it holds: a profile
// renamed before the save kept its _k, and the hint must name the command that
// starts it now. left are the keys of those that still wait: held has them but
// written does not, since the dialog added them while the save was under way.
// A profile deleted before the save is in neither, and drops out.
export function writtenProfiles(pending, written, held) {
  const names = [];
  const left = [];
  for (const k of pending) {
    const p = written.profiles.find((e) => e._k === k);
    if (p) names.push(p.name);
    else if (held.profiles.some((e) => e._k === k)) left.push(k);
  }
  return { names, left };
}

// SHELL_SAFE matches a word that a POSIX shell takes as it is, as shellSafe
// does in package launch.
const SHELL_SAFE = /^[A-Za-z0-9_@%+=:,./-]+$/;

// shellQuote returns word as one word of a POSIX shell: as it is when it is
// safe, else in single quotes, where a single quote itself is written '\''.
function shellQuote(word) {
  return SHELL_SAFE.test(word) ? word : `'${word.replaceAll("'", "'\\''")}'`;
}

// shellWord quotes path for a POSIX shell, as --dry-run quotes its command
// line, but leaves a leading ~ outside the quotes, so that the shell still
// expands it.
export function shellWord(path) {
  if (path === '~' || path === '~/') return path;
  return path.startsWith('~/') ? `~/${shellQuote(path.slice(2))}` : shellQuote(path);
}

// keyCommand returns a command for a terminal that writes what is pasted
// into it to the key file at path, which only its owner may read.
export function keyCommand(path) {
  // The directory is all before the last slash. mkdir refuses an empty name,
  // so a file in the root has the root for it, and a relative path with no
  // slash, which a config may hold, has the current directory.
  const slash = path.lastIndexOf('/');
  const dir = slash > 0 ? path.slice(0, slash) : slash === 0 ? '/' : '.';
  // An old file goes first: cat would keep its mode. rm, unlike chmod,
  // refuses a directory, which stops the command. Under the umask, mkdir
  // makes the directory 0700, as Save key does.
  const file = shellWord(path);
  return `(umask 077; mkdir -p ${shellWord(dir)} && rm -f ${file} && cat > ${file})`;
}

// isClaude reports whether a profile's model is a model of the
// subscription: an alias or a claude-… ID.
export function isClaude(model) {
  return ALIASES.includes(model) || model.toLowerCase().startsWith('claude-');
}

// subscribed reports whether profile p uses the subscription, as graft
// decides: subscription when set, else whether the model is Claude's. A
// subscription that is no boolean, which graft rejects, counts as none.
export function subscribed(p) {
  return p.subscription === '' ? isClaude(p.model) : yamlBoolean(p.subscription) === true;
}

// BOOLEAN matches a boolean as YAML writes it, in the forms that graft takes:
// true or false in one of the three cases that YAML reads as a boolean,
// plain, or after the tag !!bool, which may quote it, since the tag decides.
const BOOLEAN = /^(?:!!bool (["']?))?(true|True|TRUE|false|False|FALSE)\1$/;

// yamlBoolean returns the boolean that a value as YAML writes it holds for
// graft: true, false, or null when it is no boolean, such as yes or "true"
// in quotes, a string. The draft holds subscription so.
export function yamlBoolean(text) {
  const m = BOOLEAN.exec(text);
  return m ? m[2].toLowerCase() === 'true' : null;
}

// envValue returns the value a variable takes from the text typed into its
// value field: the text itself, but the empty string for "" typed alone, as
// config.Parse tells to write an empty value for a variable that has none.
// A value of two quote characters is for an editor to write.
export function envValue(text) {
  return text === '""' ? '' : text;
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

// takePlaces gives the name field of entry `name` of section the places of the
// references it carries along (see renameRefs), as {from, places}. Places are
// positions in a draft, so they fit only the draft they were taken in; from
// says which: the page's file object, which a load or a save replaces together
// with the draft. taken is what the field holds, or null. It is returned while
// from is the same, so that typing in one focus goes on with the places taken
// at its start, and replaced by places taken anew from draft when it is not, as
// after a reload, which may have changed the order or the number of profiles.
export function takePlaces(taken, from, draft, section, name) {
  return taken && taken.from === from ? taken : { from, places: renameRefs(draft, section, name) };
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

// shownErrors returns the errors that the page counts and shows of errors, the
// errors of its last check. It leaves out the error at profiles, which says
// that a config needs a profile, in two cases. One is while the page shows the
// first-run screen (welcome), where a newcomer would see it before doing
// anything; the draft has no profile there, so it is the only error at
// profiles. The other is while the check was of a draft with no entry
// (checkedEmpty) but draft, the one the page holds, has a profile by now, as
// right after the dialog Add provider has filled the draft of the first-run
// screen: the error is no longer true, the check of the new draft is on its
// way, and shown in between it would flash up. Anywhere else it shows, as on
// the form of defaults of a draft with no entry, and so does every other
// error. Save follows every error of the check (see canSave).
export function shownErrors(errors, { welcome, checkedEmpty, draft }) {
  const hide = welcome || (checkedEmpty && draft.profiles.length > 0);
  return hide ? errors.filter((e) => e.path !== 'profiles') : errors;
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

// canSave is the rule of the Save button, and Ctrl+S follows it too. A save
// needs changes (dirty) and none of: a file that changed on disk under them
// (stale), a save under way (saving), a server that is gone (stopped), and
// errors in the last check (errors is their number), which the server would
// refuse with a 422 anyway.
export function canSave({ dirty, stale, saving, stopped, errors }) {
  return dirty && !stale && !saving && !stopped && errors === 0;
}

// own returns the value obj holds at key, or null when it holds none: a name
// such as constructor must not find what every object inherits.
export function own(obj, key) {
  return Object.hasOwn(obj, key) ? obj[key] : null;
}
