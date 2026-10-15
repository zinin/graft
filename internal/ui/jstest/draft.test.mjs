// Tests of draft.js: run with node --test internal/ui/jstest/draft.test.mjs

import assert from 'node:assert/strict';
import { test } from 'node:test';

import {
  carryKeys, emptyDraft, keysDir, locate, messagesFor, newModel, newName, newProfile, newProvider,
  placeErrors, renameProvider, renameRefs, sameDraft, setAt, subscribed, usedBy, withKeys,
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
