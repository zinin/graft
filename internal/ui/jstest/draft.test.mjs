// Tests of draft.js: run with node --test internal/ui/jstest/draft.test.mjs

import assert from 'node:assert/strict';
import { test } from 'node:test';

import {
  addProvider, canSave, carryKeys, carryName, defaultKeyPath, emptyDraft, envValue, freeName, hasEntry, isSaveKey,
  keepTyped, keyCommand, keysDir, locate, messagesFor, newModel, newName, newProfile, newProvider, own, pickKind,
  placeErrors, presetFor, providerNameError, renameProvider, renameRefs, runHint, sameDraft, setAt, shellWord,
  shownErrors, subscribed, takePlaces, testTarget, typedModelError, usedBy, withKeys, writtenProfiles, yamlBoolean,
} from '../static/draft.js';

function sample() {
  const d = emptyDraft();
  d.providers.push({ ...newProvider('ollama', '~/.config/graft/config.yaml'), was: 'ollama' });
  d.models.push({ ...newModel('glm', 'ollama'), was: 'glm' }, { ...newModel('glm-5.3', 'ollama'), was: 'glm-5.3' });
  const mix = { ...newProfile('mix'), was: 'mix', model: 'opus', subagents: 'glm', picker: ['glm', 'glm-5.3'] };
  mix.aliases = { ...mix.aliases, haiku: 'glm' };
  d.profiles.push(mix, { ...newProfile('glm'), was: 'glm', model: 'glm' });
  return withKeys(d);
}

// savedOf returns what the page makes of the answer to a save of d: the draft
// of the file written, where the was of each entry is its name, keyed by
// carryKeys as save does.
function savedOf(d) {
  const answer = structuredClone(d);
  for (const s of ['providers', 'models', 'profiles']) {
    for (const e of answer[s]) {
      delete e._k;
      e.was = e.name;
    }
  }
  return carryKeys(d, answer);
}

test('new entries', () => {
  assert.deepEqual(newProvider('ollama', '~/.config/graft/config.yaml').key, { kind: 'file', value: '~/.config/graft/keys/ollama' });
  assert.equal(keysDir('/etc/graft.yaml'), '/etc/keys');
  assert.equal(newName([{ name: 'model-1' }, { name: 'model-3' }], 'model'), 'model-2');
});

test('a default key file follows the name of its provider', () => {
  const path = '~/.config/graft/config.yaml';
  const p = renameProvider(newProvider('provider-1', path), 'ollama', path);
  assert.deepEqual(p.key, { kind: 'file', value: '~/.config/graft/keys/ollama' });
  const own = { ...p, key: { kind: 'file', value: '~/keys/mine' } };
  assert.deepEqual(renameProvider(own, 'other', path).key, { kind: 'file', value: '~/keys/mine' });
  const env = { ...p, key: { kind: 'env', value: 'OLLAMA_API_KEY' } };
  assert.equal(renameProvider(env, 'other', path).key.value, 'OLLAMA_API_KEY');
});

test('renaming a saved provider keeps its key path', () => {
  const path = '~/.config/graft/config.yaml';
  const saved = { was: 'ollama', name: 'ollama', url: '', key: { kind: 'file', value: '~/.config/graft/keys/ollama' } };
  assert.deepEqual(renameProvider(saved, 'ollama-cloud', path).key, { kind: 'file', value: '~/.config/graft/keys/ollama' });
});

test('sameDraft ignores page keys and field order', () => {
  const a = sample();
  const b = structuredClone(a);
  for (const s of ['providers', 'models', 'profiles']) b[s].forEach((e) => { e._k = 'other'; });
  b.defaults = { args: [], env: [], settings: '', effort: '' };
  assert.ok(sameDraft(a, b));
  b.profiles[0].picker = ['glm-5.3', 'glm'];
  assert.ok(!sameDraft(a, b));
});

test('sameDraft tells a variable with no value from one with the empty string', () => {
  // The server sends a variable the file gives no value, FOO:, as null.
  const none = sample();
  none.profiles[1].env = [['FOO', null]];
  const empty = structuredClone(none);
  empty.profiles[1].env = [['FOO', '']];
  assert.ok(!sameDraft(none, empty));
  assert.ok(sameDraft(none, structuredClone(none)));
});

test('envValue makes "" typed alone the empty string', () => {
  // As config.Parse tells for a variable with no value: write "" for an empty value.
  assert.equal(envValue('""'), '');
  // Anything else is the text typed, the quotes too.
  for (const text of ['', 'a', '"', "''", '"a"', ' ""', '"" ', '"""']) assert.equal(envValue(text), text, JSON.stringify(text));
});

test('carryKeys keeps the keys of entries by name', () => {
  const a = sample();
  const saved = structuredClone(a);
  for (const s of ['providers', 'models', 'profiles']) saved[s].forEach((e) => { delete e._k; });
  saved.profiles.push({ ...newProfile('new') });
  const next = carryKeys(a, saved);
  assert.equal(next.profiles[0]._k, a.profiles[0]._k);
  assert.ok(next.profiles[2]._k && !a.profiles.some((p) => p._k === next.profiles[2]._k));
});

test('keepTyped takes the saved draft when nothing changed during the save', () => {
  const sent = sample();
  const next = savedOf(sent);
  assert.equal(keepTyped(sent, sent, next), next);
  // A change typed and undone while the save was under way is no change.
  assert.equal(keepTyped(structuredClone(sent), sent, next), next);
});

test('keepTyped takes the saved draft when the page holds none', () => {
  // The page holds no draft while it shows a file the forms cannot show.
  const sent = sample();
  const next = savedOf(sent);
  assert.equal(keepTyped(null, sent, next), next);
});

test('keepTyped keeps a change typed during the save', () => {
  const sent = sample();
  const next = savedOf(sent);
  // The page changes a draft as update does: the other entries stay shared.
  const current = { ...sent, models: sent.models.map((m, i) => (i === 0 ? { ...m, context: '200000' } : m)) };
  const kept = keepTyped(current, sent, next);
  assert.equal(kept.models[0].context, '200000');
  assert.ok(!sameDraft(kept, next), 'the page must read as unsaved');
});

test('keepTyped keeps a change of defaults typed during the save', () => {
  const sent = sample();
  const next = savedOf(sent);
  const current = { ...sent, defaults: { ...sent.defaults, effort: 'high' } };
  assert.equal(keepTyped(current, sent, next).defaults.effort, 'high');
});

test('keepTyped gives an entry the save renamed its new name as was, and keeps the change typed meanwhile', () => {
  const sent = withKeys({ ...emptyDraft(), models: [{ ...newModel('b', 'ollama'), was: 'a' }] });
  const next = savedOf(sent); // the file has b now: was: 'b', name: 'b'
  const current = { ...sent, models: [{ ...sent.models[0], context: '200000' }] };
  const [m] = keepTyped(current, sent, next).models;
  assert.deepEqual([m.was, m.name, m.context], ['b', 'b', '200000']);
});

test('keepTyped gives a new entry renamed during its save the name it was saved under as was', () => {
  const sent = withKeys({ ...emptyDraft(), models: [newModel('n', 'ollama')] });
  const next = savedOf(sent); // the file has n now
  const current = { ...sent, models: [{ ...sent.models[0], name: 'm' }] };
  const [m] = keepTyped(current, sent, next).models;
  // The next save must rename n to m, not drop n and add m as a new entry.
  assert.deepEqual([m.was, m.name], ['n', 'm']);
});

test('keepTyped gives a provider renamed during the save the name it was saved under as was', () => {
  // The save renames ollama to ollama-cloud, and the user renames it again.
  const path = '~/.config/graft/config.yaml';
  const sent = sample();
  sent.providers[0] = renameProvider(sent.providers[0], 'ollama-cloud', path);
  const next = savedOf(sent);
  const current = { ...sent, providers: [renameProvider(sent.providers[0], 'cloud', path)] };
  const [p] = keepTyped(current, sent, next).providers;
  assert.deepEqual([p.was, p.name], ['ollama-cloud', 'cloud']);
});

test('keepTyped keeps an entry added during the save, with no was', () => {
  const sent = sample();
  const next = savedOf(sent);
  const current = structuredClone(sent);
  current.profiles.push(newProfile('late'));
  withKeys(current);
  const late = keepTyped(current, sent, next).profiles.find((p) => p.name === 'late');
  assert.ok(late, 'the entry added during the save must stay');
  assert.equal(late.was, '');
});

test('keepTyped does not bring back an entry deleted during the save', () => {
  const sent = sample();
  const next = savedOf(sent);
  const current = { ...sent, models: sent.models.filter((m) => m.name !== 'glm-5.3') };
  assert.deepEqual(keepTyped(current, sent, next).models.map((m) => m.name), ['glm']);
});

test('keepTyped differs from the saved draft only by what was typed during the save', () => {
  // A save that renames an entry and writes a new one, and a change typed
  // meanwhile: undone, it leaves the draft of the file, which reads as saved.
  const sent = sample();
  sent.profiles[1].name = 'glm-only';
  sent.models.push(newModel('n', 'ollama'));
  withKeys(sent);
  const next = savedOf(sent);
  const withEffort = (d, effort) => ({ ...d, profiles: d.profiles.map((p, i) => (i === 0 ? { ...p, effort } : p)) });
  const kept = keepTyped(withEffort(sent, 'high'), sent, next);
  assert.ok(!sameDraft(kept, next));
  const undone = withEffort(kept, sent.profiles[0].effort);
  assert.ok(sameDraft(undone, next), 'undoing the change typed must leave the draft of the file');
});

test('keepTyped does not change its arguments', () => {
  // A save that renames an entry and writes a new one, and a change typed
  // meanwhile, made as the page makes it: current shares entries with sent.
  const sent = sample();
  sent.profiles[1].name = 'glm-only';
  sent.models.push(newModel('n', 'ollama'));
  withKeys(sent);
  const next = savedOf(sent);
  const current = { ...sent, profiles: sent.profiles.map((p, i) => (i === 0 ? { ...p, effort: 'high' } : p)) };
  const before = structuredClone({ current, sent, next });
  keepTyped(current, sent, next);
  assert.deepEqual(current, before.current, 'keepTyped must not change the draft of the page');
  assert.deepEqual(sent, before.sent, 'keepTyped must not change the draft sent');
  assert.deepEqual(next, before.next, 'keepTyped must not change the saved draft');
});

test('renameRefs finds every reference to a model', () => {
  const d = sample();
  const refs = renameRefs(d, 'models', 'glm');
  assert.deepEqual(refs, [
    ['profiles', 0, 'subagents'], ['profiles', 0, 'picker', 0], ['profiles', 0, 'aliases', 'haiku'], ['profiles', 1, 'model'],
  ]);
  const renamed = setAt(d, refs, 'glm-ollama');
  assert.equal(renamed.profiles[0].subagents, 'glm-ollama');
  assert.deepEqual(renamed.profiles[0].picker, ['glm-ollama', 'glm-5.3']);
  assert.equal(renamed.profiles[0].aliases.haiku, 'glm-ollama');
  assert.equal(renamed.profiles[1].model, 'glm-ollama');
  assert.equal(d.profiles[1].model, 'glm', 'setAt must not change its input');
});

test('renameRefs leaves references alone when two entries share the name', () => {
  const d = sample();
  d.models[1].name = 'glm';
  assert.deepEqual(renameRefs(d, 'models', 'glm'), []);
});

test('renameRefs of a provider', () => {
  assert.deepEqual(renameRefs(sample(), 'providers', 'ollama'), [['models', 0, 'provider'], ['models', 1, 'provider']]);
});

test('carryName keeps the references at their old value when no reference can mean the name', () => {
  const d = sample();
  const before = structuredClone(d);
  const at = (draft, path) => path.reduce((o, k) => o[k], draft);
  const refs = renameRefs(d, 'models', 'glm');
  assert.equal(refs.length, 4);

  // A name that a reference can mean goes to every reference, as setAt does.
  const renamed = carryName(d, 'models', refs, 'glm-b');
  assert.deepEqual(renamed, setAt(d, refs, 'glm-b'));
  for (const path of refs) assert.equal(at(renamed, path), 'glm-b', path.join('.'));

  // No name, or a Claude name for a model, leaves every reference at glm.
  for (const name of ['', 'opus']) {
    const kept = carryName(d, 'models', refs, name);
    for (const path of refs) assert.equal(at(kept, path), 'glm', `${path.join('.')} after ${JSON.stringify(name)}`);
    kept.profiles[1].model = 'changed'; // a copy, which the caller may change
  }

  // For a provider only the empty name is held back: a provider may be named opus.
  const providerRefs = renameRefs(d, 'providers', 'ollama');
  const emptied = carryName(d, 'providers', providerRefs, '');
  assert.deepEqual(emptied.models.map((m) => m.provider), ['ollama', 'ollama']);
  assert.deepEqual(carryName(d, 'providers', providerRefs, 'opus').models.map((m) => m.provider), ['opus', 'opus']);

  assert.deepEqual(d, before, 'carryName must not change its input');
});

test('takePlaces keeps the places of one draft, and takes them anew in another', () => {
  const file = {}; // what the page has loaded; a reload or a save gives another
  const old = sample();
  const taken = takePlaces(null, file, old, 'models', 'glm');
  assert.deepEqual(taken.places, renameRefs(old, 'models', 'glm'));
  assert.equal(taken.places.length, 4);

  // Typing in one focus: the draft is the same, so are the places, though the
  // name is not the one they were taken for.
  const typed = carryName(old, 'models', taken.places, 'glm2');
  assert.equal(takePlaces(taken, file, typed, 'models', 'glm2'), taken);

  // A reload has replaced the draft by one with the profiles in another order:
  // the places are taken anew, and a rename reaches the right profiles.
  const reloaded = structuredClone(old);
  reloaded.profiles.reverse();
  const again = takePlaces(taken, {}, reloaded, 'models', 'glm');
  assert.notEqual(again, taken);
  assert.deepEqual(again.places, renameRefs(reloaded, 'models', 'glm'));
  const [glm, mix] = carryName(reloaded, 'models', again.places, 'glm2').profiles;
  assert.equal(glm.model, 'glm2');
  assert.equal(mix.model, 'opus', 'a profile that did not use glm must stay as it was');
  assert.equal(mix.subagents, 'glm2');
  assert.deepEqual(mix.picker, ['glm2', 'glm-5.3']);
  assert.equal(mix.aliases.haiku, 'glm2');

  // A reload that has dropped a profile: the old places would point past the
  // end of the list.
  const shrunk = structuredClone(old);
  shrunk.profiles.pop();
  const fewer = takePlaces(taken, {}, shrunk, 'models', 'glm');
  assert.deepEqual(fewer.places, renameRefs(shrunk, 'models', 'glm'));
  assert.equal(carryName(shrunk, 'models', fewer.places, 'glm2').profiles[0].subagents, 'glm2');
});

test('usedBy', () => {
  const d = sample();
  assert.deepEqual(usedBy(d, 'models', 'glm'), ['profile mix', 'profile glm']);
  assert.deepEqual(usedBy(d, 'providers', 'ollama'), ['model glm', 'model glm-5.3']);
  assert.deepEqual(usedBy(d, 'profiles', 'mix'), []);
});

test('an empty name refers to nothing', () => {
  const d = emptyDraft();
  d.models.push(newModel('', 'ollama'));
  d.profiles.push(newProfile('mix'), newProfile('glm'));
  assert.deepEqual(renameRefs(d, 'models', ''), []);
  assert.deepEqual(usedBy(d, 'models', ''), []);
});

test('a model named as a Claude alias refers to Claude, not to the entry', () => {
  const d = emptyDraft();
  d.models.push(newModel('opus', 'ollama'));
  d.profiles.push({ ...newProfile('mix'), model: 'opus' });
  assert.deepEqual(renameRefs(d, 'models', 'opus'), []);
  assert.deepEqual(usedBy(d, 'models', 'opus'), []);
});

test('subscribed follows graft', () => {
  assert.equal(subscribed({ model: 'opus', subscription: '' }), true);
  assert.equal(subscribed({ model: 'claude-opus-5-5', subscription: '' }), true);
  assert.equal(subscribed({ model: 'glm', subscription: '' }), false);
  assert.equal(subscribed({ model: 'glm', subscription: 'true' }), true);
  assert.equal(subscribed({ model: 'opus', subscription: 'false' }), false);
  // Every boolean that graft takes, as the draft shows it.
  assert.equal(subscribed({ model: 'glm', subscription: 'True' }), true);
  assert.equal(subscribed({ model: 'glm', subscription: '!!bool true' }), true);
  assert.equal(subscribed({ model: 'opus', subscription: 'FALSE' }), false);
  assert.equal(subscribed({ model: 'opus', subscription: '!!bool false' }), false);
  // A subscription that is no boolean, which graft rejects, counts as none.
  assert.equal(subscribed({ model: 'opus', subscription: '"true"' }), false);
  assert.equal(subscribed({ model: 'opus', subscription: 'yes' }), false);
});

test('yamlBoolean reads the booleans that graft takes, as YAML writes them', () => {
  const trues = ['true', 'True', 'TRUE', '!!bool true', '!!bool True', '!!bool TRUE', '!!bool "true"', "!!bool 'TRUE'"];
  const falses = ['false', 'False', 'FALSE', '!!bool false', '!!bool FALSE', '!!bool "False"', "!!bool 'false'"];
  // A string in quotes, another case or word, another tag, quotes that do not
  // match, and spaces that YAML does not write.
  const others = ['', 'tRUE', 'yes', 'no', 'on', '"true"', "'false'", '!!str true', '!!bool yes', '!!bool "true\'',
    '!!booltrue', ' true', 'true ', '!!bool  true', 'null', '~', '1'];
  const wrong = [];
  for (const s of trues) if (yamlBoolean(s) !== true) wrong.push(`${JSON.stringify(s)} is not true`);
  for (const s of falses) if (yamlBoolean(s) !== false) wrong.push(`${JSON.stringify(s)} is not false`);
  for (const s of others) if (yamlBoolean(s) !== null) wrong.push(`${JSON.stringify(s)} is a boolean`);
  assert.deepEqual(wrong, []);
});

test('locate takes the longest name, which may hold dots', () => {
  const d = sample();
  assert.deepEqual(locate(d, 'models.glm-5.3.context'), { section: 'models', name: 'glm-5.3', field: 'context' });
  assert.deepEqual(locate(d, 'models.glm.context'), { section: 'models', name: 'glm', field: 'context' });
  assert.deepEqual(locate(d, 'profiles.mix.picker[1]'), { section: 'profiles', name: 'mix', field: 'picker[1]' });
  assert.deepEqual(locate(d, 'profiles.mix'), { section: 'profiles', name: 'mix', field: '' });
  assert.deepEqual(locate(d, 'profiles'), { section: 'profiles', name: null, field: '' });
  assert.deepEqual(locate(d, 'profiles.gone.model'), { section: 'profiles', name: null, field: '' });
  assert.deepEqual(locate(d, 'defaults.env.FOO'), { section: 'defaults', name: '', field: 'env.FOO' });
  assert.equal(locate(d, ''), null);
  assert.equal(locate(d, 'extra'), null);
});

test('locate takes the longer of two names that both fit a path', () => {
  const d = emptyDraft();
  d.models.push(newModel('glm-5', 'ollama'), newModel('glm-5.3', 'ollama'));
  assert.deepEqual(locate(d, 'models.glm-5.3.context'), { section: 'models', name: 'glm-5.3', field: 'context' });
  assert.deepEqual(locate(d, 'models.glm-5.context'), { section: 'models', name: 'glm-5', field: 'context' });
});

test('placeErrors and messagesFor', () => {
  const d = sample();
  const placed = placeErrors(d, [
    { path: 'profiles.mix.picker[1]', msg: 'unknown model "x"' },
    { path: 'profiles.mix', msg: 'models share an id' },
    { path: 'profiles', msg: 'at least one profile is required' },
    { path: 'defaults.effort', msg: 'must be one of low, medium, high, xhigh, max' },
    { path: '', msg: 'yaml: broken' },
    { path: 'extra', msg: 'unknown field' },
  ]);
  assert.deepEqual(placed.global, ['yaml: broken', 'extra: unknown field']);
  assert.deepEqual(placed.sections, { profiles: ['at least one profile is required'] });
  assert.deepEqual(placed.counts, { 'profiles.mix': 2, profiles: 1, defaults: 1 });
  const mix = placed.entries['profiles.mix'];
  assert.deepEqual(messagesFor(mix, 'picker'), ['unknown model "x"']);
  assert.deepEqual(messagesFor(mix, 'picker[1]'), ['unknown model "x"']);
  assert.deepEqual(messagesFor(mix, 'picker[0]'), []);
  assert.deepEqual(messagesFor(mix, ''), ['models share an id']);
  assert.deepEqual(messagesFor(placed.entries.defaults, 'effort'), ['must be one of low, medium, high, xhigh, max']);
});

test('placeErrors puts an error at a field with no input into the head of its form', () => {
  const d = sample();
  const cases = [
    ['providers.ollama.urll', 'providers.ollama', 'urll: unknown field'],
    ['models.glm.contxt', 'models.glm', 'contxt: unknown field'],
    ['models.glm.key', 'models.glm', 'key: unknown field'],
    ['profiles.mix.modl', 'profiles.mix', 'modl: unknown field'],
    ['profiles.mix.aliases.opsu', 'profiles.mix', 'aliases.opsu: unknown alias: use opus, sonnet, haiku, fable'],
    ['profiles.mix.aliases', 'profiles.mix', 'aliases: must be a mapping'],
    ['profiles.mix.env', 'profiles.mix', 'env: must be a mapping'],
    ['profiles.mix.picker', 'profiles.mix', 'picker: must be a list'],
    ['profiles.mix.args', 'profiles.mix', 'args: must be a list'],
    ['defaults.foo', 'defaults', 'foo: unknown field'],
    ['defaults.picker', 'defaults', 'picker: unknown field'],
    ['defaults.env', 'defaults', 'env: must be a mapping'],
    ['defaults.args', 'defaults', 'args: must be a list'],
  ];
  for (const [path, key, msg] of cases) {
    const placed = placeErrors(d, [{ path, msg: msg.slice(msg.indexOf(': ') + 2) }]);
    assert.deepEqual(placed.entries[key], [{ field: '', msg }], path);
    assert.deepEqual(messagesFor(placed.entries[key], ''), [msg], path);
    assert.equal(placed.counts[key], 1, path);
  }
  // It sits in the head with the errors of the entry itself, after them.
  const mix = placeErrors(d, [
    { path: 'profiles.mix', msg: 'models share an id' },
    { path: 'profiles.mix.contxt', msg: 'unknown field' },
    { path: 'profiles.mix.effort', msg: 'must be one of low, medium, high, xhigh, max' },
  ]).entries['profiles.mix'];
  assert.deepEqual(messagesFor(mix, ''), ['models share an id', 'contxt: unknown field']);
  assert.deepEqual(messagesFor(mix, 'effort'), ['must be one of low, medium, high, xhigh, max']);
});

test('placeErrors keeps an error at the input of its field', () => {
  const d = sample();
  const fields = {
    providers: ['url', 'key', 'key.env', 'key.file', 'key.command'],
    models: ['provider', 'id', 'context', 'label', 'description', 'settings', 'settings.env', 'settings.a.b'],
    profiles: [
      'model', 'subscription', 'picker[0]', 'picker[12]', 'aliases.opus', 'aliases.sonnet', 'aliases.haiku', 'aliases.fable',
      'subagents', 'effort', 'settings', 'settings.modelPicker', 'env.FOO', 'env.', 'args[0]', 'args[7]',
    ],
    defaults: ['effort', 'settings', 'settings.env', 'env.FOO', 'env.', 'args[0]', 'args[7]'],
  };
  const names = { providers: 'ollama', models: 'glm', profiles: 'mix' };
  for (const [section, list] of Object.entries(fields)) {
    const key = section === 'defaults' ? 'defaults' : `${section}.${names[section]}`;
    for (const field of list) {
      const path = section === 'defaults' ? `defaults.${field}` : `${section}.${names[section]}.${field}`;
      const placed = placeErrors(d, [{ path, msg: 'bad' }]);
      assert.deepEqual(placed.entries[key], [{ field, msg: 'bad' }], path);
      assert.deepEqual(messagesFor(placed.entries[key], field), ['bad'], path);
    }
  }
});

test('isSaveKey takes Ctrl+S in any layout, and no other key', () => {
  // The parts of a keydown event that the rule reads.
  const ev = (key, code, mods = {}) => ({ key, code, ctrlKey: false, metaKey: false, altKey: false, ...mods });
  const ctrl = { ctrlKey: true };
  const saves = {
    'Ctrl+S': ev('s', 'KeyS', ctrl),
    'Cmd+S': ev('s', 'KeyS', { metaKey: true }),
    'Ctrl+Shift+S': ev('S', 'KeyS', { ...ctrl, shiftKey: true }),
    'Ctrl+S with Caps Lock': ev('S', 'KeyS', ctrl),
    'Ctrl+S on Dvorak, where S is in the place of the semicolon': ev('s', 'Semicolon', ctrl),
    'Ctrl+S on Colemak, where S is in the place of D': ev('s', 'KeyD', ctrl),
    'Ctrl+ы on a Russian layout': ev('ы', 'KeyS', ctrl),
    'Ctrl+Ы on a Russian layout with Caps Lock': ev('Ы', 'KeyS', ctrl),
    'Cmd+ы on a Russian layout': ev('ы', 'KeyS', { metaKey: true }),
    'Ctrl+σ on a Greek layout': ev('σ', 'KeyS', ctrl),
    'Ctrl+ד on a Hebrew layout': ev('ד', 'KeyS', ctrl),
  };
  const wrong = [];
  for (const [name, e] of Object.entries(saves)) if (!isSaveKey(e)) wrong.push(`${name} does not save`);
  const others = {
    'S with no modifier, typed in a field': ev('s', 'KeyS'),
    'Shift+S, typed in a field': ev('S', 'KeyS', { shiftKey: true }),
    'Ctrl+A': ev('a', 'KeyA', ctrl),
    'Ctrl+ф on a Russian layout, which is Ctrl+A': ev('ф', 'KeyA', ctrl),
    'Ctrl+O on Dvorak, at the place of S': ev('o', 'KeyS', ctrl),
    'Cmd+O on Dvorak, at the place of S': ev('o', 'KeyS', { metaKey: true }),
    'Ctrl+R on Colemak, at the place of S': ev('r', 'KeyS', ctrl),
    'AltGr+S that types ś, which browsers report as Ctrl+Alt': ev('ś', 'KeyS', { ctrlKey: true, altKey: true }),
    'Ctrl+Alt+S': ev('s', 'KeyS', { ctrlKey: true, altKey: true }),
    'Cmd+Option+S, which types ß': ev('ß', 'KeyS', { metaKey: true, altKey: true }),
    'Alt+S': ev('s', 'KeyS', { altKey: true }),
  };
  for (const [name, e] of Object.entries(others)) if (isSaveKey(e)) wrong.push(`${name} saves`);
  assert.deepEqual(wrong, []);
});

test('canSave follows the Save button', () => {
  // One draft that may be saved, and one thing at a time that forbids it.
  const ready = { dirty: true, stale: false, saving: false, stopped: false, errors: 0 };
  assert.equal(canSave(ready), true);
  assert.equal(canSave({ ...ready, dirty: false }), false, 'no changes to save');
  assert.equal(canSave({ ...ready, stale: true }), false, 'the file changed on disk');
  assert.equal(canSave({ ...ready, saving: true }), false, 'a save is under way');
  assert.equal(canSave({ ...ready, stopped: true }), false, 'the server has stopped');
  assert.equal(canSave({ ...ready, errors: 1 }), false, 'the last check found an error');
});

test('own does not find what every object inherits', () => {
  assert.equal(own({}, 'constructor'), null);
  assert.equal(own({}, 'toString'), null);
  assert.equal(own({ a: 1 }, 'a'), 1);
  // The state of a profile named constructor, as the server sends it.
  assert.deepEqual(own(JSON.parse('{"constructor":{"mode":"direct"}}'), 'constructor'), { mode: 'direct' });
});

test('freeName takes the name, or the first free one with a number', () => {
  assert.equal(freeName([{ name: 'kimi' }], 'ollama'), 'ollama');
  assert.equal(freeName([{ name: 'ollama' }], 'ollama'), 'ollama-2');
  assert.equal(freeName([{ name: 'ollama' }, { name: 'ollama-2' }, { name: 'ollama-4' }], 'ollama'), 'ollama-3');
});

test('defaultKeyPath is where Save key writes', () => {
  assert.equal(defaultKeyPath('~/.config/graft/config.yaml', 'ollama'), '~/.config/graft/keys/ollama');
  assert.equal(defaultKeyPath('/etc/graft/graft.yaml', 'x'), '/etc/graft/keys/x');
});

test('pickKind empties a source for another kind, and starts a file at the path Save key writes', () => {
  const path = '~/.config/graft/keys/zai';
  // The value of one kind means nothing to another.
  assert.deepEqual(pickKind({ kind: 'file', value: path }, 'env', path), { kind: 'env', value: '' });
  assert.deepEqual(pickKind({ kind: 'env', value: 'ZAI_API_KEY' }, 'command', path), { kind: 'command', value: '' });
  // A file starts at the key file of the provider: env and back to file keeps Save key.
  assert.deepEqual(pickKind({ kind: 'env', value: 'ZAI_API_KEY' }, 'file', path), { kind: 'file', value: path });
  assert.deepEqual(pickKind({ kind: '', value: '' }, 'file', path), { kind: 'file', value: path });
  // The kind a source has keeps its value, a path of the user's own too, and an empty file gets the path.
  assert.deepEqual(pickKind({ kind: 'file', value: '~/keys/mine' }, 'file', path), { kind: 'file', value: '~/keys/mine' });
  assert.deepEqual(pickKind({ kind: 'env', value: 'ZAI_API_KEY' }, 'env', path), { kind: 'env', value: 'ZAI_API_KEY' });
  assert.deepEqual(pickKind({ kind: 'file', value: '' }, 'file', path), { kind: 'file', value: path });
  // A provider with no name has no key file of its own: its file starts empty.
  assert.deepEqual(pickKind({ kind: 'env', value: 'ZAI_API_KEY' }, 'file', ''), { kind: 'file', value: '' });
  // Back at that path, the key file of a new provider follows its name again.
  const config = '~/.config/graft/config.yaml';
  const p = newProvider('zai', config);
  const env = { ...p, key: pickKind(p.key, 'env', defaultKeyPath(config, 'zai')) };
  const file = { ...env, key: pickKind(env.key, 'file', defaultKeyPath(config, 'zai')) };
  assert.equal(renameProvider(file, 'z', config).key.value, '~/.config/graft/keys/z');
});

const CHOICE = {
  name: 'ollama', url: 'https://ollama.com', key: { kind: 'file', value: '~/.config/graft/keys/ollama' },
  models: [{ name: 'glm', id: 'glm-5.3-flash', context: 1048576, label: 'GLM-5.3 Flash' }, { name: 'kimi', id: 'kimi-k3', context: 1048576, label: 'Kimi K3' }],
  perModel: true, mix: false,
};

test('addProvider adds the provider, its models and a profile on each', () => {
  const before = sample();
  const copy = structuredClone(before);
  const { draft, models, profiles } = addProvider(before, { ...CHOICE, name: 'cloud' });
  assert.deepEqual(before, copy, 'the draft it was given stays as it was');
  assert.deepEqual(models, ['glm-2', 'kimi'], 'a taken model name gets a number');
  assert.deepEqual(profiles, ['glm-2', 'kimi']);
  const p = draft.providers.at(-1);
  assert.deepEqual({ ...p, _k: undefined }, { was: '', name: 'cloud', url: 'https://ollama.com', key: { kind: 'file', value: '~/.config/graft/keys/ollama' }, _k: undefined });
  assert.deepEqual({ ...draft.models.at(-2), _k: undefined }, {
    ...newModel('glm-2', 'cloud'), id: 'glm-5.3-flash', context: '1048576', label: 'GLM-5.3 Flash', _k: undefined,
  });
  assert.equal(draft.profiles.at(-2).model, 'glm-2');
  assert.equal(draft.profiles.at(-1).model, 'kimi');
  assert.ok(draft.providers.concat(draft.models, draft.profiles).every((e) => e._k), 'every entry has a key of the page');
});

test('addProvider adds mix only with a model, and no profile per model when told so', () => {
  const d = emptyDraft();
  const one = addProvider(d, { ...CHOICE, perModel: false, mix: true });
  assert.deepEqual(one.profiles, ['mix']);
  assert.equal(one.draft.profiles[0].model, 'opus');
  assert.deepEqual(one.draft.profiles[0].picker, ['glm', 'kimi']);
  const none = addProvider(d, { ...CHOICE, models: [], mix: true });
  assert.deepEqual(none.profiles, []);
  assert.deepEqual(none.draft.models, []);
  const taken = addProvider({ ...d, profiles: [{ ...newProfile('mix'), was: 'mix' }] }, { ...CHOICE, perModel: false, mix: true });
  assert.deepEqual(taken.profiles, ['mix-2']);
});

test('addProvider puts the numbered names of the models into the picker of mix', () => {
  // sample() has a model glm, and profiles glm and mix.
  const { draft, models, profiles } = addProvider(sample(), { ...CHOICE, name: 'cloud', mix: true });
  assert.deepEqual(models, ['glm-2', 'kimi'], 'a taken model name gets a number');
  assert.deepEqual(profiles, ['glm-2', 'kimi', 'mix-2'], 'so do taken profile names');
  const mix = draft.profiles.find((p) => p.name === 'mix-2');
  assert.equal(mix.model, 'opus');
  assert.deepEqual(mix.picker, ['glm-2', 'kimi'], 'the picker names the models as the draft has them');
  assert.deepEqual(draft.profiles.find((p) => p.name === 'mix').picker, ['glm', 'glm-5.3'], 'the mix there was stays as it was');
});

test('addProvider keeps an id or a window left empty empty', () => {
  const { draft } = addProvider(emptyDraft(), { ...CHOICE, models: [{ name: 'mine', id: '', context: '' }] });
  assert.equal(draft.models[0].id, '');
  assert.equal(draft.models[0].context, '');
  assert.equal(draft.models[0].label, '');
});

test('addProvider gives a numbered model with no id the name it was given as its id', () => {
  // sample() has a model glm. A model typed in as glm with no id goes in as
  // glm-2, and must still ask the provider for glm, which its Test in the dialog sent.
  const typed = addProvider(sample(), { ...CHOICE, name: 'cloud', models: [{ name: 'glm', id: '', context: '200000' }] });
  assert.deepEqual(typed.models, ['glm-2']);
  const m = typed.draft.models.at(-1);
  assert.deepEqual([m.name, m.id], ['glm-2', 'glm']);
  assert.equal(testTarget(typed.draft, m).id, 'glm', 'the Test of its form asks for glm too');
  // A model with an id keeps it, numbered or not.
  const withId = addProvider(sample(), { ...CHOICE, name: 'cloud', models: [{ name: 'glm', id: 'glm-5.3-flash', context: '' }] });
  assert.deepEqual([withId.draft.models.at(-1).name, withId.draft.models.at(-1).id], ['glm-2', 'glm-5.3-flash']);
});

test('providerNameError follows the rules of the config for the name of a provider', () => {
  for (const name of ['ollama', 'z-ai', 'my_keys', '42']) assert.equal(providerNameError(name), '', name);
  assert.equal(providerNameError(''), '', 'an empty name gets no message');
  for (const name of ['Ollama', 'z.ai', 'z-ai/glm', 'my keys', ' ollama']) {
    assert.equal(providerNameError(name), "a provider name may contain lowercase letters, digits, '-' and '_'", name);
  }
  assert.equal(providerNameError('anthropic'), '"anthropic" is reserved');
  assert.equal(providerNameError('graft'), '"graft" is reserved');
});

test('typedModelError follows the rules of the config for a model typed into the dialog', () => {
  const fine = { name: 'glm', context: '200000' };
  for (const m of [fine, { name: 'GLM-5.3_flash', context: '1048576' }, { name: '4o', context: '100000' }]) {
    assert.equal(typedModelError(m), '', m.name);
  }
  assert.equal(typedModelError({ name: ' glm ', context: ' 200000 ' }), '', 'white space at the ends does not count');
  assert.equal(typedModelError({ ...fine, name: '' }), '', 'an empty name gets no message');
  assert.equal(typedModelError({ ...fine, context: '' }), '', 'an empty context window gets no message');
  assert.equal(typedModelError({ name: ' ', context: ' ' }), '');

  const badName = "a model name may contain letters, digits, '.', '_' and '-', and starts with a letter or digit; " +
    'a model ID such as vendor/model goes into the field ID';
  for (const name of ['z-ai/glm-5.3', 'gemma4:31b', '-glm', '.glm', 'glm 5']) {
    assert.equal(typedModelError({ ...fine, name }), badName, name);
  }
  for (const name of ['claude', 'claude-mine', 'Claudette']) {
    assert.equal(typedModelError({ ...fine, name }), 'a model name must not start with "claude": such names belong to the subscription', name);
  }
  for (const name of ['default', 'best', 'opus', 'Sonnet', 'haiku', 'fable', 'opusplan', 'INHERIT']) {
    assert.equal(typedModelError({ ...fine, name }), `"${name}" is a Claude Code alias`, name);
  }
  const badContext = 'the context window must be a whole number of tokens, 100000 or more';
  for (const context of ['99999', '200k', '1e6', '200000.0', '-200000', '200 000']) {
    assert.equal(typedModelError({ ...fine, context }), badContext, context);
  }
  assert.equal(typedModelError({ name: '', context: '5' }), badContext, 'a context window gets its message with no name yet');
  // The first rule broken gives the message, the name before the context window.
  assert.equal(typedModelError({ name: 'claude/opus', context: '5' }), badName);
  assert.equal(typedModelError({ name: 'opus', context: '5' }), '"opus" is a Claude Code alias');
});

test('presetFor finds the preset of a URL', () => {
  const presets = [{ name: 'ollama', url: 'https://ollama.com' }, { name: 'kimi', url: 'https://api.moonshot.ai/anthropic' }];
  assert.equal(presetFor(presets, 'https://api.moonshot.ai/anthropic/').name, 'kimi');
  assert.equal(presetFor(presets, 'https://ollama.com').name, 'ollama');
  assert.equal(presetFor(presets, 'https://example.com'), null);
  assert.equal(presetFor(presets, ''), null);
});

test('testTarget takes the URL and key of the provider, and the name for no id', () => {
  const d = sample();
  d.providers[0].url = 'https://ollama.com';
  assert.deepEqual(testTarget(d, d.models[0]), { url: 'https://ollama.com', key: d.providers[0].key, id: 'glm' });
  assert.equal(testTarget(d, { ...d.models[0], id: 'glm-5.3-flash' }).id, 'glm-5.3-flash');
  assert.equal(testTarget(d, { ...d.models[0], provider: 'gone' }), null);
});

test('runHint names the commands', () => {
  assert.equal(runHint(['glm']), 'Run graft glm in a terminal.');
  assert.equal(runHint(['glm', 'mix']), 'Run graft glm or graft mix in a terminal.');
  assert.equal(runHint(['a', 'b', 'mix']), 'Run graft a, graft b or graft mix in a terminal.');
});

test('runHint names the file of --config in each command', () => {
  assert.equal(runHint(['glm'], '~/x/my config.yaml'), "Run graft --config ~/'x/my config.yaml' glm in a terminal.");
  assert.equal(runHint(['glm', 'mix'], '/tmp/g.yaml'), 'Run graft --config /tmp/g.yaml glm or graft --config /tmp/g.yaml mix in a terminal.');
});

test('writtenProfiles names the profiles of every dialog that a save wrote, under their names in the file', () => {
  // Two dialogs before one save: the profiles of both wait for it, by their keys of the page.
  const one = addProvider(sample(), { ...CHOICE, name: 'cloud' });
  const two = addProvider(one.draft, { ...CHOICE, name: 'local', models: [{ name: 'mine', id: '', context: '' }] });
  const key = (name) => two.draft.profiles.find((p) => p.name === name)._k;
  const pending = [...one.profiles, ...two.profiles].map(key);
  assert.deepEqual([...one.profiles, ...two.profiles], ['glm-2', 'kimi', 'mine']);
  // Before the save, kimi is renamed, since its name is the command to type, and mine is deleted.
  const sent = structuredClone(two.draft);
  sent.profiles.find((p) => p.name === 'kimi').name = 'k3';
  sent.profiles = sent.profiles.filter((p) => p.name !== 'mine');
  const written = savedOf(sent);
  // The profiles of sample() were never pending, and the deleted one drops out.
  assert.deepEqual(writtenProfiles(pending, written, written), { names: ['glm-2', 'k3'], left: [] });
  assert.deepEqual(writtenProfiles([], written, written), { names: [], left: [] });
});

test('writtenProfiles keeps waiting for a profile that the dialog added during the save', () => {
  const sent = sample();
  const written = savedOf(sent);
  // The page keeps kimi, which the save did not write, as unsaved.
  const during = addProvider(sent, { ...CHOICE, name: 'cloud', models: [CHOICE.models[1]] });
  const k = during.draft.profiles.find((p) => p.name === 'kimi')._k;
  const held = keepTyped(during.draft, sent, written);
  assert.deepEqual(writtenProfiles([k], written, held), { names: [], left: [k] });
  // The next save writes it, and names it then.
  const next = savedOf(held);
  assert.deepEqual(writtenProfiles([k], next, next), { names: ['kimi'], left: [] });
});

test('shellWord quotes for the shell and keeps ~ outside the quotes', () => {
  assert.equal(shellWord('~/.config/graft/keys/ollama'), '~/.config/graft/keys/ollama');
  assert.equal(shellWord('~/My Configs/keys/x'), "~/'My Configs/keys/x'");
  assert.equal(shellWord("/tmp/it's"), "'/tmp/it'\\''s'");
  assert.equal(shellWord('~'), '~');
});

test('keyCommand writes the key file that only its owner reads', () => {
  assert.equal(keyCommand('~/.config/graft/keys/ollama'),
    '(umask 077; mkdir -p ~/.config/graft/keys && rm -f ~/.config/graft/keys/ollama && cat > ~/.config/graft/keys/ollama)');
  assert.equal(keyCommand('~/key'), '(umask 077; mkdir -p ~ && rm -f ~/key && cat > ~/key)');
  assert.equal(keyCommand('/srv/My Keys/x'), "(umask 077; mkdir -p '/srv/My Keys' && rm -f '/srv/My Keys/x' && cat > '/srv/My Keys/x')");
});

test('renameProvider keeps the key path of a new provider whose key Save key has written', () => {
  const path = '~/.config/graft/config.yaml';
  const fresh = newProvider('ollama', path);
  // Until Save key has written the file, its path follows the name.
  assert.equal(renameProvider(fresh, 'cloud', path).key.value, '~/.config/graft/keys/cloud');
  // Once it has, the file stays where it was written, whatever the name.
  const renamed = renameProvider({ ...fresh, _keySaved: true }, 'cloud', path);
  assert.equal(renamed.name, 'cloud');
  assert.deepEqual(renamed.key, { kind: 'file', value: '~/.config/graft/keys/ollama' });
  assert.equal(renamed._keySaved, true, 'the flag stays, so that a second rename keeps the path too');
  assert.equal(renameProvider(renamed, 'other', path).key.value, '~/.config/graft/keys/ollama');
});

test('addProvider flags the provider whose key Save key has written', () => {
  const { draft } = addProvider(emptyDraft(), { ...CHOICE, keySaved: true });
  assert.deepEqual({ ...draft.providers[0], _k: undefined }, {
    was: '', name: 'ollama', url: 'https://ollama.com', key: { kind: 'file', value: '~/.config/graft/keys/ollama' }, _keySaved: true, _k: undefined,
  });
  assert.ok(draft.models.concat(draft.profiles).every((e) => !('_keySaved' in e)), 'only the provider has the flag');
  assert.ok(!('_keySaved' in addProvider(emptyDraft(), CHOICE).draft.providers[0]), 'without keySaved there is no such field, not even false');
});

test('sameDraft ignores the flag _keySaved', () => {
  const a = sample();
  const b = structuredClone(a);
  b.providers[0]._keySaved = true;
  assert.ok(sameDraft(a, b), 'the flag is no unsaved change');
  b.providers[0].url = 'https://ollama.com';
  assert.ok(!sameDraft(a, b), 'what sits beside the flag still counts');
});

test('the helpers for a terminal take a path in the root, one with no directory, and no profile', () => {
  // ~/ alone is the home directory, which needs no quotes.
  assert.equal(shellWord('~/'), '~/');
  // The directory of a file in the root is the root, and a path with no slash has the current directory.
  assert.equal(keyCommand('/key'), '(umask 077; mkdir -p / && rm -f /key && cat > /key)');
  assert.equal(keyCommand('key'), '(umask 077; mkdir -p . && rm -f key && cat > key)');
  // With no profile there is nothing to run.
  assert.equal(runHint([]), '');
});

// draftWithOne returns a draft whose only entry is one of section.
function draftWithOne(section) {
  const d = emptyDraft();
  if (section === 'providers') d.providers.push(newProvider('ollama', '~/.config/graft/config.yaml'));
  if (section === 'models') d.models.push(newModel('glm', 'ollama'));
  if (section === 'profiles') d.profiles.push(newProfile('glm'));
  return withKeys(d);
}

test('hasEntry tells the draft of the first-run screen, which has no provider, model or profile', () => {
  assert.equal(hasEntry(emptyDraft()), false, 'a missing or empty file');
  const defaults = emptyDraft();
  defaults.defaults = { effort: 'high', settings: '', env: [['FOO', 'bar']], args: ['--verbose'] };
  assert.equal(hasEntry(defaults), false, 'defaults are no entry');
  for (const section of ['providers', 'models', 'profiles']) assert.equal(hasEntry(draftWithOne(section)), true, section);
});

test('shownErrors hides the missing profile on the first-run screen, and nothing else', () => {
  const missing = { path: 'profiles', msg: 'at least one profile is required' };
  const effort = { path: 'defaults.effort', msg: 'must be one of low, medium, high, xhigh, max' };
  const broken = { path: '', msg: 'yaml: broken' };
  const unknown = { path: 'extra', msg: 'unknown field' };
  // The first-run screen of a missing or empty file shows no error.
  const first = { welcome: true, checkedEmpty: true, draft: emptyDraft() };
  assert.deepEqual(shownErrors([missing], first), []);
  // A file with only a wrong defaults still shows its error there, and so do
  // the errors that belong to no section.
  const wrong = emptyDraft();
  wrong.defaults.effort = 'hihg';
  const errors = [effort, missing, broken, unknown];
  const before = structuredClone(errors);
  assert.deepEqual(shownErrors(errors, { ...first, draft: wrong }), [effort, broken, unknown]);
  assert.deepEqual(errors, before, 'shownErrors must not change the errors it was given');
  // Only the error of the section itself goes, not one under it.
  const under = { path: 'profiles.gone.model', msg: 'unknown model "x"' };
  assert.deepEqual(shownErrors([under, missing], first), [under]);
  // The form of defaults of the same draft is no first-run screen: the
  // missing profile shows there, from the same check.
  assert.deepEqual(shownErrors([missing, effort], { ...first, welcome: false }), [missing, effort]);
  // As soon as the draft has an entry, every error of its check shows.
  for (const section of ['providers', 'models', 'profiles']) {
    assert.deepEqual(shownErrors([missing, effort], { welcome: false, checkedEmpty: false, draft: draftWithOne(section) }), [missing, effort], section);
  }
});

test('shownErrors keeps the missing profile from flashing up once the dialog has filled the first-run screen', () => {
  const missing = { path: 'profiles', msg: 'at least one profile is required' };
  // The check is still that of the empty draft, and the draft the page holds
  // has a profile by now: the error is no longer true, and stays hidden until
  // the check of the new draft answers.
  const filled = addProvider(emptyDraft(), CHOICE).draft;
  assert.deepEqual(shownErrors([missing], { welcome: false, checkedEmpty: true, draft: filled }), []);
  // A draft with entries but no profile still misses one: that shows at once,
  // on the form of defaults as anywhere else.
  const noProfile = addProvider(emptyDraft(), { ...CHOICE, perModel: false }).draft;
  assert.deepEqual(shownErrors([missing], { welcome: false, checkedEmpty: true, draft: noProfile }), [missing]);
  assert.deepEqual(shownErrors([missing], { welcome: false, checkedEmpty: true, draft: draftWithOne('models') }), [missing]);
  // Once a check of a draft with entries has answered, its errors show.
  assert.deepEqual(shownErrors([missing], { welcome: false, checkedEmpty: false, draft: filled }), [missing]);
});
