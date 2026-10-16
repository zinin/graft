// forms.js holds the forms of the four kinds of entry. Each form gets the
// entry, the whole draft for the names it offers, and errs(field), which
// returns the messages of the entry's errors about a field ('' for the
// entry itself).

import { html } from './vendor/htm-preact-standalone-3.1.1.module.js';
import { ALIASES, EFFORTS, isClaude, subscribed, usedBy, yamlBoolean } from './draft.js';
import { EnvEditor, Field, KeySource, ModelRef, NameField, OrderedList, Select, YamlField } from './fields.js';

function Head({ kind, name, tags = [], onDelete, errors }) {
  return html`
    <header class="form-head">
      <h2>${kind} <code>${name || '(no name)'}</code></h2>
      ${tags.map((t) => html`<span class=${'tag ' + t}>${t}</span>`)}
      ${onDelete && html`<button type="button" class="secondary outline" onClick=${onDelete}>Delete</button>`}
    </header>
    ${errors.length > 0 && html`<div class="entry-errors">${errors.map((m) => html`<p class="error">${m}</p>`)}</div>`}`;
}

const NAME_HINT = "letters, digits, '.', '_' and '-'";

export function ProviderForm({ entry: p, draft, errs, update, rename, refs, remove, autoFocus }) {
  const users = usedBy(draft, 'providers', p.name);
  return html`
    <${Head} kind="Provider" name=${p.name} onDelete=${remove} errors=${errs('')} />
    <${NameField} value=${p.name} refs=${refs} onRename=${rename} autoFocus=${autoFocus}
      hint="lowercase letters, digits, '-' and '_'; renaming also renames it in its models" />
    <${Field} label="URL" value=${p.url} placeholder="https://ollama.com" errors=${errs('url')}
      hint="the base URL of the provider's Anthropic Messages API; Claude Code appends /v1/messages"
      onInput=${(v) => update({ ...p, url: v })} />
    <${KeySource} value=${p.key} errors=${errs('key')} onChange=${(v) => update({ ...p, key: v })} />
    <p class="muted">${users.length ? 'Used by ' + users.join(', ') : 'No model uses this provider yet.'}</p>`;
}

export function ModelForm({ entry: m, draft, errs, update, rename, refs, remove, autoFocus }) {
  const set = (field) => (v) => update({ ...m, [field]: v });
  const providers = [{ value: '', label: '— choose a provider —' }, ...draft.providers.map((p) => ({ value: p.name, label: p.name }))];
  const users = usedBy(draft, 'models', m.name);
  return html`
    <${Head} kind="Model" name=${m.name} onDelete=${remove} errors=${errs('')} />
    <${NameField} value=${m.name} refs=${refs} onRename=${rename} autoFocus=${autoFocus}
      hint=${NAME_HINT + '; not a Claude Code alias and not claude…; renaming also renames it in profiles'} />
    <${Select} label="Provider" value=${m.provider} options=${providers} errors=${errs('provider')} onChange=${set('provider')} />
    <${Field} label="ID" value=${m.id} placeholder=${m.name} errors=${errs('id')} onInput=${set('id')}
      hint="the model ID the provider expects; defaults to the name" />
    <${Field} label="Context" value=${m.context} placeholder="1048576" errors=${errs('context')} onInput=${set('context')}
      hint="the context window in tokens, 100000 or more" />
    <${Field} label="Label" value=${m.label} placeholder=${m.name} mono=${false} errors=${errs('label')} onInput=${set('label')}
      hint="the /model row; defaults to the name" />
    <${Field} label="Description" value=${m.description} placeholder=${'via ' + (m.provider || 'the provider')} mono=${false}
      errors=${errs('description')} onInput=${set('description')} hint="the second line of the /model row" />
    <${YamlField} label="Settings (YAML)" value=${m.settings} errors=${errs('settings')} onInput=${set('settings')}
      hint="Claude Code modelSettings for this model, such as maxEffortLevel: high" />
    <p class="muted">${users.length ? 'Used by ' + users.join(', ') : 'No profile uses this model yet.'}</p>`;
}

function effortOptions(inherited) {
  return [{ value: '', label: inherited }, ...EFFORTS.map((e) => ({ value: e, label: e }))];
}

export function ProfileForm({ entry: p, draft, errs, update, rename, refs, remove, info, autoFocus }) {
  const set = (field) => (v) => update({ ...p, [field]: v });
  const models = draft.models.map((m) => m.name);
  const sub = subscribed(p);
  const noClaude = yamlBoolean(p.subscription) === false;
  const tags = info ? [info.mode, ...(info.subscription ? ['subscription'] : []), ...info.providers] : [];
  // The default option tells what an unset subscription gives, whatever is
  // chosen now; sub, the effective value, is for the labels of the models.
  const claude = isClaude(p.model);
  const subDefault = p.model === '' ? 'default' : `default: ${claude ? 'yes' : 'no'} (${p.model} ${claude ? 'is' : 'is not'} a Claude model)`;
  const inherited = draft.defaults.effort ? `from defaults: ${draft.defaults.effort}` : 'not set';
  return html`
    <${Head} kind="Profile" name=${p.name} tags=${tags} onDelete=${remove} errors=${errs('')} />
    <${NameField} value=${p.name} refs=${refs} onRename=${rename} autoFocus=${autoFocus}
      hint=${NAME_HINT + '; start it with graft ' + (p.name || 'NAME')} />
    <${ModelRef} label="Model" value=${p.model} models=${models} claude="aliases" noClaude=${noClaude}
      empty="— choose a model —" errors=${errs('model')} onChange=${set('model')}
      hint="the model the session starts on" />
    <${Select} label="Subscription" value=${p.subscription} errors=${errs('subscription')} onChange=${set('subscription')}
      options=${[{ value: '', label: subDefault }, { value: 'true', label: 'yes' }, { value: 'false', label: 'no' }]}
      hint="whether the session uses your claude.ai login" />
    <${OrderedList} label="Picker" items=${p.picker} choices=${models} onChange=${set('picker')}
      errorsAt=${(i) => errs(`picker[${i}]`)} hint="more models for /model, in this order" />
    <fieldset>
      <legend>Aliases</legend>
      <div class="grid">
        ${ALIASES.map((a) => html`
          <${ModelRef} label=${a} value=${p.aliases[a]} models=${models} claude="ids" noClaude=${noClaude}
            empty=${sub ? "Claude's own" : 'the model'} errors=${errs(`aliases.${a}`)}
            onChange=${(v) => update({ ...p, aliases: { ...p.aliases, [a]: v } })} />`)}
      </div>
      <small>Where opus, sonnet, haiku and fable point. Without the subscription, sonnet also runs the auto mode classifier, and haiku the background tasks.</small>
    </fieldset>
    <${ModelRef} label="Subagents" value=${p.subagents} models=${models} claude="aliases" noClaude=${noClaude}
      empty=${sub ? "Claude Code's choice" : 'the model'} errors=${errs('subagents')} onChange=${set('subagents')}
      hint="the model of subagents that have no model of their own" />
    <${Select} label="Effort" value=${p.effort} options=${effortOptions(inherited)} errors=${errs('effort')} onChange=${set('effort')} />
    <${YamlField} label="Settings (YAML)" value=${p.settings} errors=${errs('settings')} onInput=${set('settings')}
      hint="Claude Code settings for this session, such as autoCompactEnabled: true; not env, modelPicker or modelSettings" />
    <${EnvEditor} pairs=${p.env} onChange=${set('env')} errorsOf=${(name) => errs(`env.${name}`)} />
    <${OrderedList} label="Args" items=${p.args} onChange=${set('args')} addLabel="+ Argument"
      errorsAt=${(i) => errs(`args[${i}]`)} hint="more arguments for claude; not --settings" />`;
}

export function DefaultsForm({ entry: d, errs, update }) {
  const set = (field) => (v) => update({ ...d, [field]: v });
  return html`
    <${Head} kind="Defaults" name="every profile" errors=${errs('')} />
    <p class="muted">A profile's own values win; the args of defaults come first.</p>
    <${Select} label="Effort" value=${d.effort} options=${effortOptions('not set')} errors=${errs('effort')} onChange=${set('effort')} />
    <${YamlField} label="Settings (YAML)" value=${d.settings} errors=${errs('settings')} onInput=${set('settings')}
      hint="Claude Code settings for every profile" />
    <${EnvEditor} pairs=${d.env} onChange=${set('env')} errorsOf=${(name) => errs(`env.${name}`)} />
    <${OrderedList} label="Args" items=${d.args} onChange=${set('args')} addLabel="+ Argument"
      errorsAt=${(i) => errs(`args[${i}]`)} hint="arguments for claude in every profile" />`;
}
