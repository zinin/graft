// Tests of draft.js: run with node --test internal/ui/jstest/draft.test.mjs

import assert from 'node:assert/strict';
import { test } from 'node:test';

import {
  canSave, carryKeys, carryName, emptyDraft, isSaveKey, keysDir, locate, messagesFor, newModel, newName, newProfile,
  newProvider, own, placeErrors, renameProvider, renameRefs, sameDraft, setAt, subscribed, takePlaces, usedBy, withKeys,
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

test('carryKeys keeps the keys of entries by name', () => {
  const a = sample();
  const saved = structuredClone(a);
  for (const s of ['providers', 'models', 'profiles']) saved[s].forEach((e) => { delete e._k; });
  saved.profiles.push({ ...newProfile('new') });
  const next = carryKeys(a, saved);
  assert.equal(next.profiles[0]._k, a.profiles[0]._k);
  assert.ok(next.profiles[2]._k && !a.profiles.some((p) => p._k === next.profiles[2]._k));
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
