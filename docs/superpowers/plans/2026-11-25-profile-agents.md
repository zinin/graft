# Агенты на моделях профиля: план реализации

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** В сессии graft Claude знает внешние модели профиля и запускает на любой из них субагента, без настройки.

**Architecture:** `plan.Build` строит из внешних моделей профиля объект `--agents` (новый файл `internal/plan/agents.go`), объединяет его с `--agents` пользователя и передаёт claude после `--settings`. `launch.PlanText` печатает его в `--dry-run` и в Preview. Конфиг, роутер и формы `graft ui` не меняются.

**Tech Stack:** Go 1.26, `go.yaml.in/yaml/v3` (единственная зависимость); страница `graft ui` без сборки; Node для её тестов.

**Spec:** `docs/superpowers/specs/2026-11-25-profile-agents-design.md`

**Как проверен план.** 2026-11-25 по нему собран черновик на копии ветки: тесты задач 1–4 и все прежние зелёные (`go test -race ./...`, `node --test`, `go vet` для Linux и macOS), 12 намеренных ошибок в коде из 12 они ловят; README и `make ui-smoke` на черновике не проверялись. Вживую на черновике: Opus перечислил агентов и сказал, что `model` передавать нельзя; задача, отданная агенту по имени, ушла его провайдеру; на вызов с `model: opus` агент ответил `Wrong model`, и Opus запустил его заново без `model`; в прямом режиме агент работает под `id`. Тел функций в плане нет: исполнитель пишет свои по сигнатурам, правилам и тестам.

**Модели исполнителей (если Subagent-driven):** задачи 1–2 — opus, 3–4 — sonnet, итоговое ревью — opus.

## Global Constraints

- Модуль `github.com/zinin/graft`, Go 1.26, новых зависимостей нет. Linux и macOS.
- Вывод graft, тексты страницы, комментарии в коде, README и сообщения коммитов — на английском. Коммиты в стиле репозитория (`feat(plan): …`), с телом «зачем», без трейлеров атрибуции, с явными путями в `git add`.
- Репозиторий публичный: в файлах и коммитах нет личных имён хостов, путей, ключей и имён моделей и профилей из конфига владельца. Примеры берите из `config.example.yaml` (`sol`, `glm`, `kimi`).
- Тексты `description` и `prompt` агента — дословно из спеки. Менять их можно только с новой живой проверкой.
- graft передаёт не больше одного `--agents`, не передаёт `--append-system-prompt` и не пишет файлов Claude Code.
- Минимальная версия Claude Code — `2.1.327`.
- Проверки после каждой задачи: `test -z "$(gofmt -l .)"`, `go vet ./...`, `GOOS=darwin go vet ./...`, `go test -race ./...`, `node --test internal/ui/jstest/draft.test.mjs`. Сборку и тесты запускает `build-forge:build-runner`.
- Стиль кода — как в graft: комментарии полными предложениями объясняют зачем, имена и идиомы — как рядом.

## Review Focus

1. `label` или `description` в несколько строк либо с `"`, `<`, `&`, кириллицей: в списке агентов одна строка, JSON без лишних эскейпов. — `TestAgentTextsFitTheList`, задача 1.
2. Модель, названная в профиле несколько раз (`model`, `picker`, `aliases`, `subagents`): один агент; моделей других профилей нет. — `TestAgentsOncePerModel`, задача 1.
3. `--effort` с уровнем, которого не бывает в определении (`ultracode`, пустой): поля `effort` нет, иначе claude не стартует. — `TestAgentsEffort`, задача 2.
4. Свой `--agents` — путь к файлу, битый JSON, `null`, пустая строка: уходит как написан, второго `--agents` нет. — `TestOwnAgentsThatAreNoObject`, задача 2.
5. `--agents` и `--effort` после `--`: это текст запроса. — `TestAgentsAfterDoubleDashAreText`, `TestAgentsEffort`, задача 2.

---

### Task 1: Агенты профиля в плане запуска

**Files:**
- Create: `internal/plan/agents.go`, `internal/plan/agents_test.go`
- Modify: `internal/plan/plan.go` (`Plan`, `Build`, `args`, `marshal`), `internal/plan/plan_test.go` (`TestExampleProfiles`)

**Interfaces:**
- Consumes: `view.external []*config.Model`, `view.effort string`, `view.extra []string`, `view.prof.Subscription bool`, `(*view).seen(config.Ref) string`.
- Produces:
  - `Plan.Agents map[string]any` — объект `--agents`: имя агента → определение, `map[string]any` с ключами `description`, `prompt`, `model` и, если есть уровень, `effort`. nil, когда graft `--agents` не передаёт.
  - `func (v *view) agents(args []string) (agents map[string]any, rest []string)` — `args`: аргументы defaults, профиля и командной строки подряд; `rest`: то, что из них уходит claude. В этой задаче `rest` — это `args`.
  - `func (v *view) args(cli []string, settings, agents string, rest []string) []string`
  - `func agentDescription(m *config.Model) string`, `func agentPrompt(m *config.Model, subscription bool) string`, `func oneLine(s string) string`

- [ ] **Шаг 1: Тесты в `internal/plan/agents_test.go`**

Константы — тексты спеки для модели `sol` из `testConfig`:

```go
// The texts of the agents were checked with live models: a change of them
// needs that check again.
const (
	solDescription = "General-purpose agent on GPT-6.1 Sol: Codex via CLIProxyAPI. Use it when the user or their " +
		"instructions ask for this model. Call it without `model`: a `model` in the call would replace GPT-6.1 Sol."
	solGuard = "This agent runs on GPT-6.1 Sol and on no other model. Before anything else, look at the model that your " +
		"environment section says you are powered by. If its ID starts with `claude-`, the call that started you replaced " +
		"the model by mistake: use no tool, leave the task undone, and reply with this one line only: " +
		"\"Wrong model: this agent runs on GPT-6.1 Sol. Call it again without `model`.\"\n\nOtherwise, do "
	agentTask = "the task in the message you got with the tools you have. Complete it fully: don't gold-plate, but don't " +
		"leave it half-done. You are the agent for this task: do the work yourself, and do not hand the whole task to " +
		"another agent. When you are done, reply with a concise report of what you did and what you found: the agent that " +
		"started you reads only your reply."
)

// agentsArg returns the value of --agents in args, "" for none. Claude Code
// would take the last of several, so more than one fails the test.
func agentsArg(t *testing.T, args []string) string

// agentNames returns the names of the agents of p, sorted.
func agentNames(p *Plan) []string { return slices.Sorted(maps.Keys(p.Agents)) }
```

`agentsArg` понимает обе формы (`--agents X`, `--agents=X`) и останавливается на `--`.

| Тест | Проверка |
| --- | --- |
| `TestAgentsWithTheSubscription` | `build(t, "mix")`: имена `glm, kimi, sol`; `p.Agents["sol"]` равен `{"description": solDescription, "prompt": solGuard + agentTask, "model": "sol", "effort": "max"}`; описание `glm` начинается с `General-purpose agent on glm: via ollama. Use it `; в `p.Args` перед `--agents` стоят `--settings` и его значение, после значения `--agents` — `--allow-dangerously-skip-permissions`; JSON из `--agents` равен `p.Agents` |
| `TestAgentsOfADirectProfile` | `build(t, "pair")`: имена `glm-proxy, sol`; у `sol` `"model": "gpt-6.1-sol"`, `"prompt": "This agent runs on GPT-6.1 Sol.\n\nDo " + agentTask`, `"description": solDescription`, `"effort": "max"`; `model` у `glm-proxy` — `glm-5.3-flash` |
| `TestAgentsNeedAChoice` | профили `sol`, `glm-proxy`, `kimi`, `tuned`, `plain`: `p.Agents == nil` и нет `--agents`; `sol-plus`: ровно `sol` |
| `TestAgentsOncePerModel` | модели `a`, `b`, `c`, `other` одного провайдера, профили `all: { model: a, picker: [a, b], aliases: { haiku: c, opus: a }, subagents: c }` и `else: { model: other }`: у `all` имена `a, b, c` |

`TestAgentTextsFitTheList`: профиль `two` с моделями `m` и `odd`; у `odd` текст в несколько строк и со спецсимволами.

```go
cfg, err := config.Parse([]byte(`
providers:
  p: { url: "https://example.com", key: { env: P_KEY } }
models:
  m: { provider: p, context: 200000 }
  odd:
    provider: p
    context: 200000
    label: "GLM \"Flash\"\n<beta> & ещё"
    description: |
      fast,
        cheap.
profiles:
  two: { model: m, picker: [odd] }
`), filepath.Join(t.TempDir(), "config.yaml"))

const label = `GLM "Flash" <beta> & ещё`
// odd["description"] ==
"General-purpose agent on " + label + ": fast, cheap. Use it when the user or their instructions ask for this model. " +
	"Call it without `model`: a `model` in the call would replace " + label + "."
// odd["prompt"] starts with
"This agent runs on " + label + ".\n\nDo the task "
// the value of --agents contains, without escapes
`<beta> & ещё: fast, cheap.`
```

- [ ] **Шаг 2: `TestExampleProfiles` в `internal/plan/plan_test.go`**

Поле таблицы `agents bool // whether the profile has agents: see agents_test.go`, `true` у `mix`, `sol-plus` и `multi`. В ожидаемых аргументах этих профилей после `"<settings>"` идут `"--agents", "<agents>"`; в копии `p.Args` значение под индексом 7 заменяется на `"<agents>"`.

- [ ] **Шаг 3: Убедиться, что тесты не собираются**

Run: `go test ./internal/plan/`
Expected: FAIL, `p.Agents undefined`

- [ ] **Шаг 4: `internal/plan/agents.go`**

- `agents`: nil и `args` без изменений, когда внешних моделей нет или она одна и подписки нет. Иначе определение на каждую модель `v.external`, имя — `m.Name`, `model` — `v.seen(config.Ref{Model: m.Name})`, `effort` — `v.effort`, при пустом ключа нет.
- `agentDescription` и `agentPrompt`: тексты спеки с `oneLine(m.Label)`; у `oneLine(m.Description)` убирается одна точка в конце. Без подписки промпт — `This agent runs on <label>.\n\nDo ` и общая часть.
- `oneLine`: слова через один пробел (`strings.Fields`).

- [ ] **Шаг 5: `internal/plan/plan.go`**

- `Build` вызывает `v.agents` на `v.extra` плюс `in.Args`, кодирует агентов тем же `marshal` (без HTML-эскейпов; в его комментарии и ошибке — не только settings) и кладёт объект в `Plan.Agents`.
- `args` ставит `--agents` после `--settings`, если строка `agents` не пуста, затем `rest`. Правила `--model` и `--effort` прежние: их отменяют только опции из `cli`.

- [ ] **Шаг 6: Тесты зелёные**

Run: `go test ./internal/plan/`
Expected: `ok`. Затем проверки из Global Constraints.

- [ ] **Шаг 7: Commit**

```bash
git add internal/plan/agents.go internal/plan/agents_test.go internal/plan/plan.go internal/plan/plan_test.go
git commit -m "feat(plan): pass an agent on each external model of a profile" -m "Claude Code's Agent tool takes only Claude's aliases for its model, so a session could not hand a task to the profile's external models, nor did Claude know of them. Each is an agent in --agents now, when the session has models to choose from."
```

---

### Task 2: `--effort` и `--agents` пользователя

**Files:**
- Modify: `internal/plan/agents.go`
- Test: `internal/plan/agents_test.go`

**Interfaces:**
- Consumes: `(*view).agents`, `Plan.Agents`, `agentsArg`, `agentNames` из задачи 1.
- Produces:
  - `func takeOption(args []string, name string) (rest, values []string)` — `args` без опции `name` и её значения по порядку, в формах `name value` и `name=value`, до `--`. `name` последним аргументом остаётся в `rest`. `args` не меняется.
  - `func (v *view) startEffort(args []string) string`
  - В `Plan.Agents` определения пользователя лежат как `json.RawMessage`.

- [ ] **Шаг 1: Тесты**

`TestTakeOption`, опция `--x`, порядок `args → rest, values`:

```go
{nil, nil, nil},
{[]string{"a", "--x", "1", "b"}, []string{"a", "b"}, []string{"1"}},
{[]string{"--x=1", "--x", "", "--x=2=3"}, nil, []string{"1", "", "2=3"}},
{[]string{"--xy", "1", "--x"}, []string{"--xy", "1", "--x"}, nil},
{[]string{"--x", "1", "--", "--x", "2"}, []string{"--", "--x", "2"}, []string{"1"}},
```

и `args` после вызова прежний.

`TestAgentsEffort`: модели `a`, `b` одного провайдера; профили `none: { model: a, picker: [b] }`, `high: { model: a, picker: [b], effort: high }`, `argued: { model: a, picker: [b], effort: high, args: [--effort, low] }`. `effort` у обоих агентов (nil — ключа нет):

```go
{"none", nil, nil},
{"none", []string{"--effort", "max"}, "max"},
{"high", nil, "high"},
{"high", []string{"--effort", "low"}, "low"},
{"high", []string{"--effort=xhigh", "-p"}, "xhigh"},
{"high", []string{"--effort", "low", "--effort", "max"}, "max"},
{"high", []string{"--effort", "ultracode"}, nil},
{"high", []string{"--effort="}, nil},
{"high", []string{"-p", "--", "--effort", "low"}, "high"},
{"argued", nil, "low"},
{"argued", []string{"--effort", "medium"}, "medium"},
```

`TestOwnAgentsJoin`, профиль `mix`, `mine` = `{"mine":{"description":"Mine.","prompt":"a<b","maxTurns":20},"sol":{"description":"My Sol.","prompt":""}}`; столбцы — аргументы, имена агентов, что стоит в `p.Args` после значения `--agents` и `--allow-dangerously-skip-permissions`:

```go
{[]string{"--agents", mine, "--resume"}, []string{"glm", "kimi", "mine", "sol"}, []string{"--resume"}},
{[]string{"-p", "--agents=" + mine, "--", "hi"}, []string{"glm", "kimi", "mine", "sol"}, []string{"-p", "--", "hi"}},
{[]string{"--agents", `{"first":{"description":"x","prompt":""}}`, "--agents", mine}, []string{"glm", "kimi", "mine", "sol"}, nil},
{[]string{"--agents", "{}"}, []string{"glm", "kimi", "sol"}, nil},
```

В строках с `mine` значение `--agents` содержит оба определения пользователя дословно: `"mine":{"description":"Mine.","prompt":"a<b","maxTurns":20}` и `"sol":{"description":"My Sol.","prompt":""}`.

| Тест | Проверка |
| --- | --- |
| `TestOwnAgentsInTheConfig` | `defaults.args: [--verbose, --agents, '{"kept":{"description":"x","prompt":""}}']`, профили `two: { model: a, picker: [b] }` и `one: { model: a }`: у `two` имена `a, b, kept`; с `--agents '{"mine":…}'` в командной строке — `a, b, mine`; у `one` `p.Agents == nil`, а `--agents` ушёл как написан |
| `TestOwnAgentsThatAreNoObject` | профиль `mix`, значения `agents.json`, `{broken`, `null`, `[]`, `"text"`, пустая строка, в обеих формах между `-p` и `hi`: `p.Agents == nil`, единственный `--agents` — пользователя, `p.Args` кончается его аргументами без изменений |
| `TestAgentsAfterDoubleDashAreText` | `build(t, "mix", "-p", "--", "--agents", "{}")`: `p.Args` кончается этими четырьмя аргументами, имена `glm, kimi, sol` |

- [ ] **Шаг 2: Убедиться, что тесты не собираются**

Run: `go test ./internal/plan/`
Expected: FAIL, `undefined: takeOption`

- [ ] **Шаг 3: Реализация в `agents.go`**

- `agents`: `takeOption(args, "--agents")`; действует последнее значение, как у Claude Code. Если оно не объект JSON (и `null` не объект) — nil и `args` как есть. Определения пользователя поверх агентов graft, как `json.RawMessage`.
- `startEffort(args)`: последний `--effort`, если он из `config.Efforts`; при другом значении `""`; без `--effort` — `v.effort`. `agents` берёт уровень отсюда.

- [ ] **Шаг 4: Тесты зелёные**

Run: `go test ./internal/plan/`
Expected: `ok`. Затем проверки из Global Constraints.

- [ ] **Шаг 5: Commit**

```bash
git add internal/plan/agents.go internal/plan/agents_test.go
git commit -m "feat(plan): join the user's --agents and --effort with graft's agents" -m "Of two --agents Claude Code takes the last, so the user's would have dropped graft's without a word: graft passes one with both. An agent runs at the effort the session starts at, whoever set it."
```

---

### Task 3: `--dry-run`, Preview и минимальная версия

**Files:**
- Modify: `internal/launch/text.go`, `cmd/graft/check.go`, `README.md:15`
- Test: `internal/launch/text_test.go`, `cmd/graft/main_test.go`, `internal/ui/preview_test.go`

**Interfaces:**
- Consumes: `Plan.Agents` (значения — `map[string]any` или `json.RawMessage`).
- Produces: раздел `agents` в конце `launch.PlanText`; `minClaude = "2.1.327"`.

- [ ] **Шаг 1: Тесты**

`TestPlanTextAgents` в `internal/launch/text_test.go`:

```go
p := &plan.Plan{
	Profile:  "mix",
	Mode:     config.Router,
	Args:     []string{"--model", "opus", "--settings", "{}", "--agents", `{"sol":{"model":"sol"}}`},
	Settings: map[string]any{},
	Agents: map[string]any{
		"sol":  map[string]any{"model": "sol", "description": "a<b"},
		"mine": json.RawMessage(`{"prompt":"a<b","maxTurns":20}`),
	},
}
// PlanText(p, "/opt/claude/bin/claude", shownKey) ==
want := `profile mix: router
exec   /opt/claude/bin/claude --model opus --settings '{}' --agents '{"sol":{"model":"sol"}}'
settings
{}
agents
{
  "mine": {
    "prompt": "a<b",
    "maxTurns": 20
  },
  "sol": {
    "description": "a<b",
    "model": "sol"
  }
}
`
```

`TestPlanTextDirect` и `TestPlanTextRouter` не меняются: без агентов раздела нет.

| Тест | Что добавить или заменить |
| --- | --- |
| `TestDryRunRouter` | в ожидаемых строках `}' --agents '{"glm":{"description":"General-purpose agent on glm: via ollama. ` и `\nagents\n{\n  "glm": {\n    "description": "General-purpose agent on glm: via ollama. ` |
| `TestDryRun` | в выводе нет слова `agents` |
| `TestPreview`, профиль `mix` | строка `agents\n{\n  "glm": {\n` |
| `newFake`, `TestCheck` | версия claude `2.1.331` вместо `2.1.296` |
| `TestCheckWarnings` | версия `2.1.326`, сообщение `graft needs 2.1.327 or later` |
| `TestOlderThan` | `2.1.327`, `2.1.331`, `2.2.0`, `3.0` — не старые; `2.1.326`, `2.0.999`, `1.9`, `garbage` — старые |

- [ ] **Шаг 2: Убедиться, что тесты падают**

Run: `go test ./internal/launch/ ./internal/ui/ ./cmd/graft/`
Expected: FAIL в `TestPlanTextAgents`, `TestPreview`, `TestDryRunRouter`, `TestCheckWarnings`, `TestOlderThan`

- [ ] **Шаг 3: Реализация**

- `PlanText`: после settings, если `p.Agents != nil`, строка `agents` и `indentJSON(p.Agents)`; комментарий функции называет агентов.
- `minClaude = "2.1.327"` с комментарием: до 2.1.327 Claude Code терял агентов `--agents` после загрузки плагинов.
- README, строка 15: `Claude Code 2.1.327 or later`.

- [ ] **Шаг 4: Тесты зелёные**

Run: `go test ./internal/launch/ ./internal/ui/ ./cmd/graft/`
Expected: `ok` у трёх пакетов. Затем проверки из Global Constraints.

- [ ] **Шаг 5: Commit**

```bash
git add internal/launch/text.go internal/launch/text_test.go internal/ui/preview_test.go cmd/graft/check.go cmd/graft/main_test.go README.md
git commit -m "feat: show the agents in --dry-run and ask for Claude Code 2.1.327" -m "The agents end the text of --dry-run and Preview, readable next to the command line that holds them. Before 2.1.327 Claude Code lost them once plugins had loaded."
```

---

### Task 4: Документация, пример конфига, подсказка

**Files:**
- Modify: `README.md`, `config.example.yaml`, `internal/ui/static/forms.js:71`
- Test: `internal/edit/read_test.go` (`TestReadExample`)

**Interfaces:**
- Consumes: поведение задач 1–3.
- Produces: раздел README `## Subagents` (якорь `#subagents`), профиль `lead` в примере.

- [ ] **Шаг 1: Тест примера**

`TestReadExample`: профилей 6; `d.Profiles[5]` — `lead`, `Model` `opus`, `Picker` `[sol]`, `Args` — `--append-system-prompt` и `Give implementation tasks to the sol agent and review its work yourself.`

Run: `go test ./internal/edit/`
Expected: FAIL, `counts 3 5 5`

- [ ] **Шаг 2: `config.example.yaml`, в конец**

```yaml

  # Opus plans and reviews, GPT implements. In a session with models to choose
  # from, each external one is also a subagent type, under its name.
  lead:
    model: opus
    picker: [sol]
    args: [--append-system-prompt, "Give implementation tasks to the sol agent and review its work yourself."]
```

Run: `go test ./internal/edit/ ./internal/config/`
Expected: `ok`

- [ ] **Шаг 3: Подсказка Description в `forms.js`**

`hint="the second line of the /model row; Claude also reads it in its list of subagents"`

- [ ] **Шаг 4: README**

- «How it works»: в списке прямого режима — `--agents` с субагентом на каждую внешнюю модель, когда их две или больше, ссылка на `#subagents`; в списке роутера — то же для внешних моделей профиля.
- Таблица `models`, строка `description`: это ещё и то, что Claude читает о модели в списке субагентов.
- Таблица «Commands», `--dry-run`: печатает и агентов.
- «Security»: субагент на внешней модели отправляет её провайдеру свою задачу и то, что читает.
- «Troubleshooting»: `Wrong model: …` от агента — в вызове был `model`, вызвать без него; агентов graft нет в списке — обновить Claude Code до 2.1.327.
- Новый раздел `## Subagents` между «Configuration» и «Commands», по одному предложению или пункту на факт:
  1. когда есть агенты (с подпиской или от двух внешних моделей) и что имя агента — имя модели;
  2. Claude видит `label` и `description` и запускает агента, когда об этом просите вы или ваши инструкции; пример запроса `Give the implementation tasks to sol and review its work yourself.`;
  3. агент универсален, роль приходит в задаче; постоянную роль главной модели пишут в CLAUDE.md, в план или в `args: [--append-system-prompt, "…"]` (`lead` в `config.example.yaml`);
  4. effort — тот, с которым сессия стартует, не выше `maxEffortLevel` модели; `/effort` его не меняет; `CLAUDE_CODE_EFFORT_LEVEL` перекрывает;
  5. `model` в вызове заменяет модель агента; описание просит его не передавать, а с подпиской промпт велит модели Claude остановиться и сказать об этом;
  6. модель субагента видна в `/tasks`, его запросы за роутером — в логе;
  7. чтобы Claude выбирал сам из нескольких моделей, в `description` пишут, для чего модель;
  8. выключить агента: `settings: { permissions: { deny: ["Agent(sol)"] } }`;
  9. свой `--agents` в `args` или в командной строке объединяется с агентами graft, при одном имени побеждает ваш; путь к файлу (Claude Code берёт его с `-p`) их заменяет;
  10. агент graft перекрывает одноимённый из `~/.claude/agents`, `.claude/agents` и встроенный, такой как `Explore`;
  11. `CLAUDE_CODE_SUBAGENT_MODEL_FORCE` отменяет модели агентов;
  12. graft ничего не пишет в `~/.claude/agents`.

- [ ] **Шаг 5: Проверки**

Проверки из Global Constraints и `make ui-smoke`. Expected: всё зелёное; `grep -n '2.1.288' README.md cmd internal -r` ничего не находит.

- [ ] **Шаг 6: Commit**

```bash
git add README.md config.example.yaml internal/ui/static/forms.js internal/edit/read_test.go
git commit -m "docs: describe the subagents on a profile's external models" -m "README tells when they exist, how to ask for one, what sets its effort and how the user's own --agents join; the example config has a profile where Opus leads and GPT implements."
```

---

### После задач: живая проверка (контроллер)

Собрать `bin/graft`. Из `graft list` взять профиль с подпиской и внешними моделями и профиль с одной внешней моделью без подписки. Имена в файлы репозитория не писать.

1. `bin/graft --dry-run <профиль>` — агент на каждую внешнюю модель, `effort` профиля; у второго профиля слова `agents` нет.
2. `bin/graft <профиль> -p --model opus --effort low --strict-mcp-config --setting-sources project --no-session-persistence --permission-mode bypassPermissions -- "<запрос>"` в пустом каталоге, три запроса: назвать модели, доступные субагентам; отдать агенту подсчёт файлов; запустить агента с `model: opus` и сделать, что он скажет. В `~/.local/state/graft/graft.log`: во втором и третьем — `model=<имя> route=<провайдер> status=200`; в третьем агент ответил `Wrong model: …`.
3. Передать владельцу приёмку из спеки. Перед PR убрать `docs/superpowers/` коммитом `git rm`.
