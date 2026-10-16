# graft ui, этап 2 (новичок): план реализации

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Новичок без конфига проходит в `graft ui` путь «пресет → ключ → тестовый запрос → модели → профили → Save» и запускает `graft <профиль>`, не открывая README; у каждого профиля есть Preview — текст `graft --dry-run`, в котором ключи показаны источником и не прочитаны.

**Architecture:** Сервер `internal/ui` получает пять маршрутов: пресеты из встроенного `presets.json`, состояние и запись ключа, тестовый запрос через транспорт маршрутизатора и Preview. Preview строит план так же, как `graft PROFILE`, и печатает его общей с `--dry-run` функцией из `internal/launch`. Ключ живёт только на сервере: в браузер уходят состояние и отпечаток. Страница получает экран первого запуска, диалог «Add provider», виджет ключа, кнопку Test и вкладку Preview; чистая логика (свободные имена, записи из диалога, команда для терминала) живёт в `draft.js` под `node --test`.

**Tech Stack:** Go 1.26, `go.yaml.in/yaml/v3` (единственная зависимость), `net/http`, `embed`; Preact + htm и Pico CSS из этапа 1, без сборки; Node для тестов `draft.js`; headless Chrome для `make ui-smoke`.

**Spec:** `docs/superpowers/specs/2026-10-15-graft-ui-design.md`, этап 2: «Этапы», «Этап 2 (новичок) готов, когда», строки API с этапом 2, «Изменения в существующих пакетах», «Этап 2: путь новичка», «Фронтенд», «Вывод в терминал», этап 2 в «Тестировании», «Документации» и «Ограничениях и рисках». Решения владельца от 2026-10-16 уже в дизайне: «Save key» только по пути `keys/<провайдер>`, экран первого запуска и для пустого файла.

**Как проверен план.** Тесты задач 1–6 прогнаны 2026-10-16 на прототипе, на копии этой ветки перед задачей 1: `gofmt`, `go vet` для Linux и macOS, `go test -race ./...` (8 пакетов) и `node --test` (46 тестов) зелёные, а без реализации тесты не собираются. Тел функций в плане нет. Исполнитель пишет свои по сигнатурам, правилам и тестам плана, а тесты переносит как есть. Если тест плана не проходит с реализацией, которая следует описанию, остановитесь и сообщите, а не подгоняйте тест. Страница (задачи 7–8) не прототипировалась: её проверяют `make ui-smoke` и ревью в Chrome.

**Исполнители:** задачи 1–5 — opus (сервер, ключи и безопасность, как выбрал владелец 2026-10-16), 6–8 — sonnet (страница), 9 — opus (сбор моделей по документации), 10 — sonnet (README).

**Перед задачей 1:** ветка `feat/web-ui-newcomer` (база — `master` на `1af87c7`), рабочее дерево чистое, полная проверка зелёная.

## Global Constraints

- Модуль `github.com/zinin/graft`, Go 1.26. Новых зависимостей Go нет: только `go.yaml.in/yaml/v3`. Вендоренные файлы страницы не меняются.
- graft работает на Linux и macOS; Windows вне рамок.
- Тексты интерфейса, вывод graft, комментарии в коде и сообщения коммитов — на английском. Коммиты — в стиле репозитория (`feat(ui): …`), с телом, которое говорит зачем, без трейлеров атрибуции, с явными путями в `git add`.
- Репозиторий публичный: в файлах и коммитах нет личных имён хостов, путей, ключей, отпечатков ключей и имён моделей из конфига владельца.
- Ключ никогда не уходит в браузер, в конфиг и в вывод graft. Ответы API несут состояние и отпечаток — первые 12 hex-знаков sha256 (`keys.Fingerprint`); в тексте ответа провайдера и в ошибках ключ заменён на `<key>`.
- `POST /api/key/write` пишет только `<каталог конфига>/keys/<провайдер>`: имя по правилам `config` (`config.CheckProviderName`), ключ по правилам `keys` (`keys.Clean`). Каталоги создаются с правами 0700, файл — с 0600: временный файл, `fsync`, `rename`, `fsync` каталога. Поверх существующего файла — только с `replace: true`.
- Команда ключа запускается только по Run (`run: true` в `POST /api/key/check`), по одной за раз, а прочитанный ею ключ хранится в памяти по тексту команды до конца сеанса. Тестовый запрос и Preview команд не запускают.
- Тестовый запрос: `POST <url>/v1/messages`, где путь URL и `/v1/messages` соединены одной косой чертой; заголовки `Authorization: Bearer <ключ>`, `anthropic-version: 2023-06-01`, `content-type: application/json`; тело `{"model":"<id>","max_tokens":16,"messages":[{"role":"user","content":"Reply with one word: pong"}]}`. Транспорт — `router.Transport()`, общий тайм-аут 60 с, без потока, без повторов и без переходов по редиректу. Странице — не больше 200 знаков ответа 2xx и 2 КиБ тела другого статуса.
- Preview — тот же текст, что `graft --dry-run`, через `launch.PlanText`, с ключами в виде `<источник>`, без чтения ключей. Проверки `--dry-run` в `cmd/graft` не меняются.
- CSP, токен и проверки `Host`, `Origin` и uid — как на этапе 1; новые маршруты живут под `/api/` и проходят ту же защиту. Ссылки наружу — `<a target="_blank" rel="noopener noreferrer">`; сама страница наружу не обращается.
- `\u`-эскейпы в Go и JS пишутся эскейпами, не сырыми символами: Go не принимает BOM (U+FEFF) нигде, кроме начала файла, даже внутри строки. Инструменты записи файлов умеют превращать эскейп в символ, поэтому после записи проверьте файл.
- Проверки после каждой задачи: `test -z "$(gofmt -l .)"`, `go vet ./...`, `GOOS=darwin go vet ./...`, `go test -race ./...`, `node --test internal/ui/jstest/draft.test.mjs`; в задачах 6–9 ещё `make ui-smoke`.
- Стиль кода — как в graft: комментарии полными предложениями объясняют зачем; имена и идиомы — как в соседнем коде.

## Review Focus

1. Ключ, вставленный с переводом строки или пробелами по краям, сохраняется обрезанным; ключ с пробелом внутри или с BOM — отказ с сообщением `keys`; ни тот ни другой не попадает в терминал и в ответы. — `TestKeyWrite`, `TestKeyWriteRefusals`, задача 3.
2. Провайдер повторяет ключ в теле ошибки, в том числе на границе обрезки 2 КиБ: странице приходит `<key>`, а не начало ключа. — `TestTestRequestAnswers`, задача 4.
3. URL провайдера с путём и косой чертой в конце (`…/anthropic/`): тестовый запрос идёт на `…/anthropic/v1/messages`, куда пойдёт и маршрутизатор. — `TestTestRequest`, `TestTestRequestJoinsTheURL`, задача 4.
4. Команда ключа, которая спрашивает пароль: запускается только по Run, никогда две сразу, а чтение `env` и `file` её не ждёт; Test и Preview её не запускают. — `TestKeyCommandRunsOnlyWhenAsked`, `TestKeyCommandsRunOneAtATime`, `TestTestRequestUsesTheKeptKey`, `TestPreview`, задачи 3–5.
5. Занятые имена: второй пресет Ollama даёт провайдера `ollama-2`, его модели и профили получают суффиксы, а профили ссылаются на модели под новыми именами. — тесты `freeName` и `addProvider` в `draft.test.mjs`, задача 6.

---

### Task 1: Текст `--dry-run` в `internal/launch`, файлы настроек профиля в `plan`

Preview печатает тот же текст, что `--dry-run`, поэтому текст переезжает из `cmd/graft` в `internal/launch`, к `SessionValue`, а выбор файлов настроек Claude Code для профиля — в `plan`.

**Files:**
- Create: `internal/launch/text.go`, `internal/launch/text_test.go`
- Modify: `internal/plan/claude.go`, `internal/plan/claude_test.go`, `cmd/graft/start.go`, `cmd/graft/main_test.go`

**Interfaces:**
- Consumes: `plan.Plan`, `plan.EnvVar`, `plan.Route`, `launch.SessionValue`, `plan.UserSettingsDir`, `plan.LoadClaudeSettings` — без изменений.
- Produces:
  - `func launch.PlanText(p *plan.Plan, claude string, show func(provider string) (string, error)) (string, error)` — текст `--dry-run`; `show` даёт то, что стоит на месте ключа провайдера.
  - `func plan.LoadSettingsFor(cfg *config.Config, profile string, environ []string, home, dir string) (files []plan.SettingsFile, warnings []string, err error)` — файлы настроек, которые проверяет запуск профиля.

- [ ] **Шаг 1: Тесты текста плана и файлов настроек**

Создайте `internal/launch/text_test.go`:

```go
package launch

import (
	"errors"
	"testing"

	"github.com/zinin/graft/internal/config"
	"github.com/zinin/graft/internal/plan"
)

// shownKey stands in for the key of a provider in the tests of PlanText.
func shownKey(provider string) (string, error) { return "<key of " + provider + ">", nil }

func TestPlanTextDirect(t *testing.T) {
	p := &plan.Plan{
		Profile:   "glm",
		Mode:      config.Direct,
		Providers: []string{"ollama"},
		Args:      []string{"--model", "glm-5.3-flash", "--settings", `{"env":{}}`, "--resume"},
		Unset:     []string{"ANTHROPIC_API_KEY"},
		Set: []plan.EnvVar{
			{Name: "ANTHROPIC_BASE_URL", Value: "https://ollama.com"},
			{Name: "ANTHROPIC_AUTH_TOKEN", KeyOf: "ollama"},
		},
		Settings: map[string]any{"env": map[string]any{}},
	}
	got, err := PlanText(p, "/opt/claude/bin/claude", shownKey)
	if err != nil {
		t.Fatal(err)
	}
	want := `profile glm: direct, provider ollama
unset  ANTHROPIC_API_KEY
set    ANTHROPIC_BASE_URL=https://ollama.com
       ANTHROPIC_AUTH_TOKEN=<key of ollama>
exec   /opt/claude/bin/claude --model glm-5.3-flash --settings '{"env":{}}' --resume
settings
{
  "env": {}
}
`
	if got != want {
		t.Errorf("PlanText:\n%s\nwant:\n%s", got, want)
	}
}

func TestPlanTextRouter(t *testing.T) {
	p := &plan.Plan{
		Profile:      "mix",
		Mode:         config.Router,
		Subscription: true,
		Providers:    []string{"ollama", "cliproxy"},
		Routes: []plan.Route{
			{Model: "glm", Provider: "ollama", URL: "https://ollama.com", ID: "glm-5.3-flash"},
			{Model: "sol", Provider: "cliproxy", URL: "http://127.0.0.1:8317", ID: "gpt-6.1-sol"},
		},
		Args: []string{"--model", "opus", "--settings", "{}"},
		Set: []plan.EnvVar{
			{Name: "ANTHROPIC_BASE_URL", Session: plan.SessionURL},
			{Name: "ANTHROPIC_CUSTOM_HEADERS", Session: plan.SessionHeader},
		},
		Settings: map[string]any{"a": "<b>"},
	}
	got, err := PlanText(p, "/opt/claude/bin/claude", shownKey)
	if err != nil {
		t.Fatal(err)
	}
	want := `profile mix: router, providers ollama, cliproxy, subscription
set    ANTHROPIC_BASE_URL=http://127.0.0.1:<port>
       ANTHROPIC_CUSTOM_HEADERS=X-Graft-Session: <session secret>
route  glm -> ollama glm-5.3-flash <key of ollama>
       sol -> cliproxy gpt-6.1-sol <key of cliproxy>
exec   /opt/claude/bin/claude --model opus --settings '{}'
settings
{
  "a": "<b>"
}
`
	if got != want {
		t.Errorf("PlanText:\n%s\nwant:\n%s", got, want)
	}
}

func TestPlanTextStopsAtAKeyItCannotShow(t *testing.T) {
	p := &plan.Plan{Profile: "glm", Set: []plan.EnvVar{{Name: "ANTHROPIC_AUTH_TOKEN", KeyOf: "ollama"}}}
	refused := errors.New("provider ollama: env OLLAMA_API_KEY is not set")
	_, err := PlanText(p, "claude", func(string) (string, error) { return "", refused })
	if !errors.Is(err, refused) {
		t.Errorf("error %v, want %v", err, refused)
	}
}

func TestShellJoin(t *testing.T) {
	got := shellJoin([]string{"/bin/claude", "--model", "a b", "it's", "", `{"x":1}`})
	want := `/bin/claude --model 'a b' 'it'\''s' '' '{"x":1}'`
	if got != want {
		t.Errorf("shellJoin = %s, want %s", got, want)
	}
}
```

В `cmd/graft/main_test.go` удалите `TestShellJoin` целиком: он переехал в `text_test.go` вместе с `shellJoin`.

Допишите в конец `internal/plan/claude_test.go`:

```go
func TestLoadSettingsFor(t *testing.T) {
	cfg, err := config.Parse([]byte(`
providers:
  ollama: { url: https://ollama.com, key: { env: OLLAMA_API_KEY } }
models:
  glm: { provider: ollama, context: 1048576 }
profiles:
  glm: { model: glm }
  opus: { model: opus }
`), filepath.Join(t.TempDir(), "config.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	home, dir := t.TempDir(), t.TempDir()
	writeFile(t, filepath.Join(home, ".claude", "settings.json"), `{"apiKeyHelper": "get-key"}`)
	writeFile(t, filepath.Join(dir, ".claude", "settings.json"), `{not json`)

	files, warnings, err := LoadSettingsFor(cfg, "glm", nil, home, dir)
	if err != nil || len(files) != 1 || len(warnings) != 1 || !strings.Contains(warnings[0], "cannot parse") {
		t.Errorf("glm: %d files, warnings %q, %v; want the user settings and a warning about the broken file", len(files), warnings, err)
	}
	// Only a profile with external models needs the files, and a home.
	for _, profile := range []string{"opus", "none"} {
		if files, warnings, err := LoadSettingsFor(cfg, profile, nil, "", dir); files != nil || warnings != nil || err != nil {
			t.Errorf("%s: %v, %q, %v; want nothing", profile, files, warnings, err)
		}
	}
	if _, _, err := LoadSettingsFor(cfg, "glm", nil, "", dir); err == nil || !strings.Contains(err.Error(), "cannot find the home directory") {
		t.Errorf("glm without a home: %v", err)
	}
}
```

- [ ] **Шаг 2: Убедиться, что тесты не собираются**

Запустите: `go test ./internal/launch/ ./internal/plan/`

Ожидается: FAIL, ошибки сборки `undefined: PlanText`, `undefined: shellJoin` и `undefined: LoadSettingsFor`.

- [ ] **Шаг 3: `launch.PlanText` в `internal/launch/text.go`**

```go
// PlanText returns what graft --dry-run prints for plan p: the mode and the
// providers, the variables graft unsets and sets, the routes, the command
// line with claude at path claude, and the settings. show returns what
// stands in for the key of a provider; no key is ever shown.
func PlanText(p *plan.Plan, claude string, show func(provider string) (string, error)) (string, error)
```

Перенесите построение текста из `printPlan` в `cmd/graft/start.go` как есть: те же строки, метки и их ширина (`block`), JSON настроек с отступом и без HTML-экранирования (`indentJSON`), строка `exec` через `shellJoin` с путём `claude` первым. `block`, `indentJSON`, `shellJoin` и `shellSafe` переезжают в `text.go` без изменений. Значения сеанса — `SessionValue(e.Session, "<port>", "<session secret>")`. `show` вызывается для каждой переменной `Set` с `KeyOf`, затем для каждого маршрута, в этом порядке; его ошибка возвращается как есть.

- [ ] **Шаг 4: `plan.LoadSettingsFor` в `internal/plan/claude.go`**

```go
// LoadSettingsFor returns the Claude Code settings files that matter when
// profile starts in directory dir: those of LoadClaudeSettings, for the user
// settings of UserSettingsDir. They matter only to a profile with external
// models, so for any other there are none, and it needs no home directory.
// warnings name the files that could not be read.
func LoadSettingsFor(cfg *config.Config, profile string, environ []string, home, dir string) (files []SettingsFile, warnings []string, err error)
```

Правило — то, что сейчас стоит в начале `start` в `cmd/graft/start.go`. Профиль, которого нет в `cfg`, и профиль без внешних моделей (`cfg.ExternalModels`) дают `nil, nil, nil`. Иначе — `UserSettingsDir(cfg, profile, environ, home)`, чья ошибка возвращается, и `LoadClaudeSettings(userDir, dir)`.

- [ ] **Шаг 5: `cmd/graft/start.go` через `launch` и `plan`**

- `start` берёт файлы из `plan.LoadSettingsFor(cfg, profile, sys.environ, sys.home, sys.cwd)`: ошибка — `graft: <ошибка>` и код 1, каждое предупреждение — `graft: warning: <предупреждение>`, как сейчас.
- `printPlan` ищет `claude` как сейчас (`graft: claude not found in PATH` и код 1), берёт текст из `launch.PlanText(p, path, show)`, где `show` — `showKey(cfg, provider, key)`, печатает ошибку как `graft: <ошибка>` с кодом 1, а текст пишет в `sys.stdout`.
- `block`, `indentJSON`, `shellJoin`, `shellSafe` и ставшие лишними импорты (`encoding/json`, `regexp`, `strings`) из `start.go` удаляются.

- [ ] **Шаг 6: Полная проверка**

Запустите: `test -z "$(gofmt -l .)" && go vet ./... && GOOS=darwin go vet ./... && go test -race ./... && node --test internal/ui/jstest/draft.test.mjs`

Ожидается: код 0. Проверки `--dry-run` в `cmd/graft` (`TestDryRun`, `TestDryRunRouter`, `TestKeysAreReadOnce`, `TestDryRunNeedsClaude`) не менялись и проходят: вывод `--dry-run` прежний.

- [ ] **Шаг 7: Commit**

```bash
git add internal/launch/text.go internal/launch/text_test.go internal/plan/claude.go internal/plan/claude_test.go cmd/graft/start.go cmd/graft/main_test.go
git commit -m "refactor: print the plan of --dry-run from package launch" -m "Preview in graft ui prints the same text, with each key shown by its source and never read, so the text moves out of cmd/graft into launch, next to SessionValue. The choice of Claude Code settings files for a profile moves into plan for the same reason. --dry-run prints what it printed."
```

### Task 2: Пресеты провайдеров

Встроенный `presets.json` с восемью провайдерами дизайна и маршрут, который отдаёт его странице. Модели пресетов добавит задача 9: их собирают ближе к релизу.

**Files:**
- Create: `internal/ui/presets.json`, `internal/ui/presets.go`, `internal/ui/presets_test.go`
- Modify: `internal/ui/api.go` (маршрут)

**Interfaces:**
- Consumes: `config.Parse`; `startAPI` и `(*testAPI).call` из `api_test.go`.
- Produces:
  - `GET /api/presets` — байты `presets.json`: `{"providers": [{"title", "name", "url", "keys", "docs", "catalog", "models": [{"name", "id", "context", "label"}]}]}`;
  - в `presets.go`: `presetsJSON []byte`, типы `preset` и `presetModel`, `func (a *api) presets(w http.ResponseWriter, r *http.Request)`.

- [ ] **Шаг 1: Тест пресетов**

Создайте `internal/ui/presets_test.go`:

```go
package ui

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"go.yaml.in/yaml/v3"

	"github.com/zinin/graft/internal/config"
)

// TestPresets checks each preset as the dialog Add provider uses it: the
// config it makes of the preset, with a key file, a profile per model and
// mix, passes config.Parse.
func TestPresets(t *testing.T) {
	var file struct {
		Providers []preset `json:"providers"`
	}
	dec := json.NewDecoder(bytes.NewReader(presetsJSON))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&file); err != nil {
		t.Fatal(err)
	}
	if len(file.Providers) == 0 {
		t.Fatal("no presets")
	}
	seen := map[string]bool{}
	for _, p := range file.Providers {
		t.Run(p.Name, func(t *testing.T) {
			if seen[p.Name] {
				t.Errorf("two presets are named %s", p.Name)
			}
			seen[p.Name] = true
			if p.Title == "" || p.Docs == "" {
				t.Errorf("preset %s needs a title and docs", p.Name)
			}
			for _, link := range []string{p.Keys, p.Docs, p.Catalog} {
				if link != "" && !strings.HasPrefix(link, "https://") {
					t.Errorf("link %q is not https", link)
				}
			}
			models, profiles := map[string]any{}, map[string]any{}
			picker := []string{}
			for _, m := range p.Models {
				if strings.Contains(m.ID, "[1m]") {
					t.Errorf("model %s: the id %q has [1m]: graft sets the window itself", m.Name, m.ID)
				}
				if m.Label == "" {
					t.Errorf("model %s has no label", m.Name)
				}
				models[m.Name] = map[string]any{"provider": p.Name, "id": m.ID, "context": m.Context, "label": m.Label}
				profiles[m.Name] = map[string]any{"model": m.Name}
				picker = append(picker, m.Name)
			}
			profiles["mix"] = map[string]any{"model": "opus", "picker": picker}
			cfg := map[string]any{
				"providers": map[string]any{p.Name: map[string]any{"url": p.URL, "key": map[string]any{"file": "keys/" + p.Name}}},
				"profiles":  profiles,
			}
			if len(models) > 0 {
				cfg["models"] = models
			}
			data, err := yaml.Marshal(cfg)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := config.Parse(data, "/graft/config.yaml"); err != nil {
				t.Errorf("the config of the preset:\n%s\n%v", data, err)
			}
		})
	}
}

func TestPresetsAPI(t *testing.T) {
	s := startAPI(t, "")
	var file struct {
		Providers []preset `json:"providers"`
	}
	if code := s.call(t, "GET", "/api/presets", nil, &file); code != 200 || len(file.Providers) == 0 {
		t.Fatalf("GET /api/presets: %d, %d presets", code, len(file.Providers))
	}
}
```

- [ ] **Шаг 2: Убедиться, что тест не собирается**

Запустите: `go test ./internal/ui/`

Ожидается: FAIL, ошибки сборки `undefined: preset` и `undefined: presetsJSON`.

- [ ] **Шаг 3: `presets.json` и `presets.go`**

Создайте `internal/ui/presets.json`. Адреса, страницы ключей и руководства — из таблицы «Пресеты» дизайна, проверены 2026-10-15; `catalog` — у OpenRouter и CLIProxyAPI, моделей пока нет:

```json
{
  "providers": [
    {
      "title": "Ollama Cloud",
      "name": "ollama",
      "url": "https://ollama.com",
      "keys": "https://ollama.com/settings/keys",
      "docs": "https://docs.ollama.com/api/anthropic-compatibility",
      "catalog": "",
      "models": []
    },
    {
      "title": "DeepSeek",
      "name": "deepseek",
      "url": "https://api.deepseek.com/anthropic",
      "keys": "https://platform.deepseek.com/api_keys",
      "docs": "https://api-docs.deepseek.com/guides/anthropic_api",
      "catalog": "",
      "models": []
    },
    {
      "title": "Kimi",
      "name": "kimi",
      "url": "https://api.moonshot.ai/anthropic",
      "keys": "https://platform.kimi.ai/console/api-keys",
      "docs": "https://platform.kimi.ai/docs/guide/claude-code-kimi",
      "catalog": "",
      "models": []
    },
    {
      "title": "Z.ai",
      "name": "zai",
      "url": "https://api.z.ai/api/anthropic",
      "keys": "https://z.ai/manage-apikey/apikey-list",
      "docs": "https://docs.z.ai/scenario-example/develop-tools/claude",
      "catalog": "",
      "models": []
    },
    {
      "title": "Xiaomi MiMo",
      "name": "mimo",
      "url": "https://api.xiaomimimo.com/anthropic",
      "keys": "https://platform.xiaomimimo.com/console",
      "docs": "https://mimo.mi.com/docs/en-US/api/chat/anthropic-api",
      "catalog": "",
      "models": []
    },
    {
      "title": "MiniMax",
      "name": "minimax",
      "url": "https://api.minimax.io/anthropic",
      "keys": "https://platform.minimax.io/console/plan",
      "docs": "https://platform.minimax.io/docs/token-plan/claude-code",
      "catalog": "",
      "models": []
    },
    {
      "title": "OpenRouter",
      "name": "openrouter",
      "url": "https://openrouter.ai/api",
      "keys": "https://openrouter.ai/settings/keys",
      "docs": "https://openrouter.ai/docs/guides/coding-agents/claude-code-integration",
      "catalog": "https://openrouter.ai/models",
      "models": []
    },
    {
      "title": "CLIProxyAPI",
      "name": "cliproxy",
      "url": "http://127.0.0.1:8317",
      "keys": "",
      "docs": "https://github.com/router-for-me/CLIProxyAPI",
      "catalog": "https://github.com/router-for-me/CLIProxyAPI",
      "models": []
    }
  ]
}
```

Создайте `internal/ui/presets.go`. Типы — это контракт со страницей, а TODO требует дизайн:

```go
package ui

import (
	_ "embed"
	"net/http"
)

// presetsJSON lists the providers that the first-run screen and the dialog
// Add provider offer, with the models whose windows their documentation
// names.
//
// TODO: load the current list of models from the provider, so that a new
// model needs no release of graft. The windows will likely stay here:
// /v1/models seldom gives them.
//
//go:embed presets.json
var presetsJSON []byte

// preset is a provider of presets.json.
type preset struct {
	Title   string        `json:"title"`
	Name    string        `json:"name"`
	URL     string        `json:"url"`
	Keys    string        `json:"keys"`    // the page for keys, "" when there is none
	Docs    string        `json:"docs"`    // the provider's guide for Claude Code
	Catalog string        `json:"catalog"` // where to find the models to add by hand, "" when there is no such page
	Models  []presetModel `json:"models"`
}

type presetModel struct {
	Name    string `json:"name"` // a model name of the config
	ID      string `json:"id"`
	Context int    `json:"context"`
	Label   string `json:"label"`
}

func (a *api) presets(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.Write(presetsJSON)
}
```

В `routes` в `internal/ui/api.go` добавьте после `POST /api/save`:

```go
	mux.HandleFunc("GET /api/presets", a.presets)
```

- [ ] **Шаг 4: Тесты проходят**

Запустите: `go test ./internal/ui/ -run 'TestPresets' -v`

Ожидается: PASS, `TestPresets` с восемью подтестами (`ollama`, `deepseek`, `kimi`, `zai`, `mimo`, `minimax`, `openrouter`, `cliproxy`) и `TestPresetsAPI`.

- [ ] **Шаг 5: Полная проверка**

Запустите: `test -z "$(gofmt -l .)" && go vet ./... && GOOS=darwin go vet ./... && go test -race ./... && node --test internal/ui/jstest/draft.test.mjs`

Ожидается: код 0.

- [ ] **Шаг 6: Commit**

```bash
git add internal/ui/presets.json internal/ui/presets.go internal/ui/presets_test.go internal/ui/api.go
git commit -m "feat(ui): serve the presets of providers" -m "The first-run screen and the dialog Add provider offer them. The file holds the eight providers of the design for now, with their addresses and pages for keys; the models whose windows the providers' documentation names come later, closer to the release, as they change the fastest."
```

### Task 3: Ключи: состояние, запись, строка `wrote`

Виджет ключа спрашивает у сервера, что даёт источник ключа, а «Save key» пишет вставленный ключ в `keys/<провайдер>`. Для этого `config` и `keys` экспортируют свои проверки, а сервер получает окружение и терминал `graft ui`.

**Files:**
- Modify: `internal/config/parse.go`, `internal/config/config_test.go`, `internal/keys/keys.go`, `internal/keys/keys_test.go`, `internal/ui/server.go`, `internal/ui/api.go`, `internal/ui/api_test.go`, `cmd/graft/ui.go`
- Create: `internal/ui/keys.go`, `internal/ui/keys_test.go`

**Interfaces:**
- Consumes: `writeTemp` и `syncDir` из `store.go`; `keys.Reader`, `keys.Describe`, `keys.Fingerprint`, `config.Tildify`; `edit.Key`.
- Produces:
  - `func config.CheckURL(s string) string` (сейчас `checkURL`), `func config.CheckProviderName(name string) string`, `func config.ResolvePath(s, dir string) (string, error)` (сейчас метод парсера `resolvePath`); сообщения `config.Parse` не меняются;
  - `func keys.Clean(raw string) (string, error)`; сообщения `Reader.Read` не меняются;
  - поля `ui.Options`: `Environ []string`, `Stdin io.Reader`, `Stderr io.Writer`;
  - `func newAPI(opts Options) *api`; поля `api`: `opts Options`, `keys keys.Reader`, `keyMu`, `cmdMu`, `keptMu sync.Mutex`, `kept map[string]string`; `func lookupIn(environ []string) func(string) (string, bool)`;
  - `func (a *api) source(k edit.Key) (config.KeySource, error)`, `func (a *api) readKey(src config.KeySource, run bool) (key, warning string, err error)`, `var errNotRun error`;
  - `POST /api/key/check` и `POST /api/key/write` с контрактами шага 7;
  - тестовые помощники: `startWith(t, content, opts)` в `api_test.go`; `(*testAPI).checkKey`, `writeKeyFor`, `readFile`, `waitFile` в `keys_test.go`.

- [ ] **Шаг 1: Тесты экспортов `config` и `keys`**

Допишите в конец `internal/config/config_test.go`:

```go
func TestCheckURL(t *testing.T) {
	for url, want := range map[string]string{
		"https://ollama.com":              "",
		"http://127.0.0.1:8317":           "",
		"https://api.z.ai/api/anthropic/": "",
		"ftp://example.com":               "must be an absolute http or https URL",
		"ollama.com":                      "must be an absolute http or https URL",
		"https://me:pw@example.com":       "must not contain a user name or password: the key comes from key",
		"https://example.com/?a=1":        "must not have a query or fragment: Claude Code appends /v1/messages to it",
	} {
		if got := CheckURL(url); got != want {
			t.Errorf("CheckURL(%q) = %q, want %q", url, got, want)
		}
	}
}

func TestCheckProviderName(t *testing.T) {
	const rule = "a provider name may contain lowercase letters, digits, '-' and '_'"
	for name, want := range map[string]string{
		"ollama":     "",
		"my_proxy-2": "",
		"Ollama":     rule,
		"../x":       rule,
		"":           rule,
		"graft":      `"graft" is reserved`,
		"anthropic":  `"anthropic" is reserved`,
	} {
		if got := CheckProviderName(name); got != want {
			t.Errorf("CheckProviderName(%q) = %q, want %q", name, got, want)
		}
	}
}

func TestResolvePath(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	for path, want := range map[string]string{
		"~":           home,
		"~/keys/x":    filepath.Join(home, "keys", "x"),
		"keys/x":      "/etc/graft/keys/x",
		"keys//x":     "/etc/graft/keys/x",
		"/opt/keys/x": "/opt/keys/x",
	} {
		if got, err := ResolvePath(path, "/etc/graft"); err != nil || got != want {
			t.Errorf("ResolvePath(%q) = %q, %v, want %q", path, got, err, want)
		}
	}
}
```

Допишите в конец `internal/keys/keys_test.go`:

```go
func TestClean(t *testing.T) {
	tests := []struct {
		raw, key, err string
	}{
		{raw: " sk-pasted \n", key: "sk-pasted"},
		{raw: "sk-A1_b.c~d+e/f=", key: "sk-A1_b.c~d+e/f="},
		{raw: " \n", err: "the key is empty"},
		{raw: "sk-a sk-b", err: "the key must be one line without spaces"},
		{raw: "sk-a\nsk-b", err: "the key must be one line without spaces"},
		{raw: "\ufeffsk-a", err: "the key contains U+FEFF, which is not a printable ASCII character"},
	}
	for _, tt := range tests {
		key, err := Clean(tt.raw)
		if tt.err != "" {
			if err == nil || err.Error() != tt.err {
				t.Errorf("Clean(%q) error %v, want %q", tt.raw, err, tt.err)
			}
			continue
		}
		if err != nil || key != tt.key {
			t.Errorf("Clean(%q) = %q, %v, want %q", tt.raw, key, err, tt.key)
		}
	}
}
```

- [ ] **Шаг 2: Убедиться, что тесты не собираются**

Запустите: `go test ./internal/config/ ./internal/keys/`

Ожидается: FAIL, ошибки сборки `undefined: CheckURL`, `undefined: CheckProviderName`, `undefined: ResolvePath` и `undefined: Clean`.

- [ ] **Шаг 3: Экспорты `config` в `internal/config/parse.go`**

```go
// CheckProviderName says what is wrong with a provider name, or returns "".
func CheckProviderName(name string) string

// CheckURL says what is wrong with a provider URL, or returns "". Claude
// Code appends the API path to the URL, so it cannot end in a query or a
// fragment, and the key comes from key, not from the URL.
func CheckURL(s string) string

// ResolvePath returns the file that path s names in a config file in
// directory dir: a leading ~ is the home directory, and a relative path is
// relative to dir.
func ResolvePath(s, dir string) (string, error)
```

- `CheckProviderName` берёт обе проверки из `providers`: шаблон `providerName` — `a provider name may contain lowercase letters, digits, '-' and '_'`, затем `reservedProviders` — `"<имя>" is reserved` (`%q`). `providers` вызывает её вместо своего `switch`: сообщение у ключа записи, имя в `badProviders`, `continue`.
- `CheckURL` — это `checkURL` с новым именем; `ResolvePath` — это `resolvePath` без получателя, с каталогом вторым аргументом, и `keySource` вызывает `ResolvePath(s, p.dir)`.

- [ ] **Шаг 4: `keys.Clean` в `internal/keys/keys.go`**

```go
// Clean returns the key in raw, without white space at the ends, or an
// error when raw holds no key that can go into an HTTP header: a key is one
// line of printable ASCII characters. An invisible character, such as the
// byte order mark some editors write, would break the header.
func Clean(raw string) (string, error)
```

Ошибки: пусто после обрезки — `the key is empty`; пробельный или управляющий символ — `the key must be one line without spaces`; другой символ вне печатного ASCII — `the key contains %U, which is not a printable ASCII character`. Правило символов — нынешнего `badChar`, который уходит. `Reader.Read` получает ключ из `Clean` и оборачивает его ошибку как `fmt.Errorf("%s: %w", Describe(src), err)`, так что прежние сообщения (`env EMPTY: the key is empty`) не меняются.

- [ ] **Шаг 5: Тесты проходят, commit**

Запустите: `test -z "$(gofmt -l .)" && go vet ./... && go test -race ./internal/config/ ./internal/keys/ ./cmd/graft/`

Ожидается: код 0.

```bash
git add internal/config/parse.go internal/config/config_test.go internal/keys/keys.go internal/keys/keys_test.go
git commit -m "refactor: export the checks of a provider name, a URL, a key path and a key" -m "graft ui checks a key source, a pasted key and the URL of a test request before the config is saved, by the same rules as config.Parse and keys.Reader, so those rules become functions of their own. The messages of the config and of the keys stay as they were."
```

- [ ] **Шаг 6: Тесты API ключей**

В `internal/ui/api_test.go` замените `startAPI` на две функции:

```go
func startAPI(t *testing.T, content string) *testAPI {
	t.Helper()
	return startWith(t, content, Options{})
}

// startWith is startAPI with the options of a Server; Path and Log are the
// test's own.
func startWith(t *testing.T, content string, opts Options) *testAPI {
	t.Helper()
	path := filepath.Join(tempDir(t), "config.yaml")
	if content != "" {
		if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	log := &lockedBuffer{}
	opts.Path, opts.Log = path, log
	mux := http.NewServeMux()
	newAPI(opts).routes(mux)
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return &testAPI{srv, path, log}
}
```

Создайте `internal/ui/keys_test.go`:

```go
package ui

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/zinin/graft/internal/config"
	"github.com/zinin/graft/internal/keys"
)

// checkKey asks the API for the state of a key source.
func (s *testAPI) checkKey(t *testing.T, kind, value string, run bool) keyState {
	t.Helper()
	var st keyState
	if code := s.call(t, "POST", "/api/key/check", map[string]any{"kind": kind, "value": value, "run": run}, &st); code != 200 {
		t.Fatalf("POST /api/key/check: %d", code)
	}
	return st
}

// writeKeyFor puts content into keys/name next to the config of s.
func writeKeyFor(t *testing.T, s *testAPI, name, content string, mode os.FileMode) string {
	t.Helper()
	path := filepath.Join(filepath.Dir(s.path), "keys", name)
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), mode); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(path, mode); err != nil { // WriteFile's mode is filtered by umask
		t.Fatal(err)
	}
	return path
}

func TestKeyCheckFile(t *testing.T) {
	s := startAPI(t, "")
	good := writeKeyFor(t, s, "good", " sk-good \n", 0o600)
	open := writeKeyFor(t, s, "open", "sk-open\n", 0o644)
	two := writeKeyFor(t, s, "two", "sk-a\nsk-b\n", 0o600)
	none := filepath.Join(filepath.Dir(s.path), "keys", "none")
	tests := []struct {
		value string
		want  keyState
	}{
		{"keys/good", keyState{State: "ok", Path: config.Tildify(good), Fingerprint: keys.Fingerprint("sk-good")}},
		{good, keyState{State: "ok", Path: config.Tildify(good), Fingerprint: keys.Fingerprint("sk-good")}},
		{"keys//good", keyState{State: "ok", Path: config.Tildify(good), Fingerprint: keys.Fingerprint("sk-good")}},
		{"keys/none", keyState{State: "missing", Path: config.Tildify(none)}},
		{"keys/open", keyState{State: "ok", Path: config.Tildify(open), Fingerprint: keys.Fingerprint("sk-open"),
			Warning: "key file " + config.Tildify(open) + " is readable by other users: run chmod 600 on it"}},
		{"keys/two", keyState{State: "error", Path: config.Tildify(two),
			Error: "file " + config.Tildify(two) + ": the key must be one line without spaces"}},
		{" ", keyState{State: "error", Error: "must not be empty"}},
	}
	for _, tt := range tests {
		if got := s.checkKey(t, "file", tt.value, false); got != tt.want {
			t.Errorf("file %q: %+v, want %+v", tt.value, got, tt.want)
		}
	}
}

func TestKeyCheckEnv(t *testing.T) {
	s := startWith(t, "", Options{Environ: []string{"OLLAMA_API_KEY=sk-env", "EMPTY="}})
	tests := []struct {
		name string
		want keyState
	}{
		{"OLLAMA_API_KEY", keyState{State: "ok", Fingerprint: keys.Fingerprint("sk-env")}},
		{"MISSING", keyState{State: "unset"}},
		{"EMPTY", keyState{State: "error", Error: "env EMPTY: the key is empty"}},
	}
	for _, tt := range tests {
		if got := s.checkKey(t, "env", tt.name, false); got != tt.want {
			t.Errorf("env %s: %+v, want %+v", tt.name, got, tt.want)
		}
	}
	if got := s.checkKey(t, "", "x", false); got.State != "error" {
		t.Errorf("no kind: %+v, want an error", got)
	}
}

// TestKeyCommandRunsOnlyWhenAsked: a key command may ask for a password, so
// it runs only for Run, and its key is kept for what follows.
func TestKeyCommandRunsOnlyWhenAsked(t *testing.T) {
	s := startAPI(t, "")
	dir := t.TempDir()
	count, key := filepath.Join(dir, "count"), filepath.Join(dir, "key")
	if err := os.WriteFile(key, []byte("sk-cmd\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	cmd := "echo run >> '" + count + "'; cat '" + key + "'"
	if st := s.checkKey(t, "command", cmd, false); st != (keyState{State: "not run"}) {
		t.Errorf("before Run: %+v", st)
	}
	if _, err := os.Stat(count); err == nil {
		t.Fatal("the command ran without Run")
	}
	want := keyState{State: "ok", Fingerprint: keys.Fingerprint("sk-cmd")}
	if st := s.checkKey(t, "command", cmd, true); st != want {
		t.Errorf("Run: %+v, want %+v", st, want)
	}
	if st := s.checkKey(t, "command", cmd, false); st != want {
		t.Errorf("after Run: %+v, want the kept key %+v", st, want)
	}
	if got := readFile(t, count); got != "run\n" {
		t.Errorf("the command ran %d times, want once", strings.Count(got, "run"))
	}
	// A run that fails drops the kept key.
	os.Remove(key)
	if st := s.checkKey(t, "command", cmd, true); st.State != "error" {
		t.Errorf("a failed run: %+v", st)
	}
	if st := s.checkKey(t, "command", cmd, false); st != (keyState{State: "not run"}) {
		t.Errorf("after a failed run: %+v, want no kept key", st)
	}
}

// TestKeyCommandsRunOneAtATime: two commands never ask for a password at
// once, and env and file do not wait for a command.
func TestKeyCommandsRunOneAtATime(t *testing.T) {
	s := startWith(t, "", Options{Environ: []string{"OLLAMA_API_KEY=sk-env"}})
	dir := t.TempDir()
	log, gate := filepath.Join(dir, "log"), filepath.Join(dir, "gate")
	// Each command waits for the gate, so the first holds its turn until the
	// test opens it.
	cmd := func(n string) string {
		return "echo start >> '" + log + "'; while [ ! -e '" + gate + "' ]; do sleep 0.02; done; echo end >> '" + log + "'; echo sk-" + n
	}
	var wg sync.WaitGroup
	for _, n := range []string{"1", "2"} {
		wg.Add(1)
		go func() {
			defer wg.Done()
			s.checkKey(t, "command", cmd(n), true)
		}()
	}
	waitFile(t, log, "start\n")
	if st := s.checkKey(t, "env", "OLLAMA_API_KEY", false); st.State != "ok" {
		t.Errorf("env while a command waits: %+v", st)
	}
	time.Sleep(100 * time.Millisecond)
	if got := readFile(t, log); got != "start\n" {
		t.Errorf("a second command started while the first waited:\n%s", got)
	}
	if err := os.WriteFile(gate, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	wg.Wait()
	if got := readFile(t, log); got != "start\nend\nstart\nend\n" {
		t.Errorf("the commands overlapped:\n%s", got)
	}
}

// waitFile waits until the file at path holds want.
func waitFile(t *testing.T, path, want string) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if data, _ := os.ReadFile(path); string(data) == want {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("%s never held %q", path, want)
}

func readFile(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

func TestKeyWrite(t *testing.T) {
	s := startAPI(t, "")
	path := filepath.Join(filepath.Dir(s.path), "keys", "ollama")
	var got keyWritten
	if code := s.call(t, "POST", "/api/key/write", map[string]any{"provider": "ollama", "key": "  sk-new \n"}, &got); code != 200 {
		t.Fatalf("write: %d", code)
	}
	if want := (keyWritten{Path: config.Tildify(path), Fingerprint: keys.Fingerprint("sk-new")}); got != want {
		t.Errorf("answer %+v, want %+v", got, want)
	}
	if content := readFile(t, path); content != "sk-new\n" {
		t.Errorf("file %q", content)
	}
	for p, mode := range map[string]os.FileMode{path: 0o600, filepath.Dir(path): 0o700} {
		if info, err := os.Stat(p); err != nil || info.Mode().Perm() != mode {
			t.Errorf("%s: mode %v, want %v (%v)", p, info.Mode().Perm(), mode, err)
		}
	}
	line := "graft ui: wrote " + config.Tildify(path) + " (sha256:" + keys.Fingerprint("sk-new") + ")\n"
	if s.log.String() != line {
		t.Errorf("terminal %q, want %q", s.log.String(), line)
	}

	// Another write needs replace, and the answer tells which key is there.
	var conflict keyConflict
	if code := s.call(t, "POST", "/api/key/write", map[string]any{"provider": "ollama", "key": "sk-other"}, &conflict); code != 409 {
		t.Fatalf("write over a key: %d, want 409", code)
	}
	if want := (keyConflict{Error: config.Tildify(path) + " already holds a key", Fingerprint: keys.Fingerprint("sk-new")}); conflict != want {
		t.Errorf("conflict %+v, want %+v", conflict, want)
	}
	if code := s.call(t, "POST", "/api/key/write", map[string]any{"provider": "ollama", "key": "sk-other", "replace": true}, nil); code != 200 {
		t.Fatalf("replace: %d", code)
	}
	if content := readFile(t, path); content != "sk-other\n" {
		t.Errorf("file after replace %q", content)
	}
	if strings.Contains(s.log.String(), "sk-new") || strings.Contains(s.log.String(), "sk-other") {
		t.Errorf("the terminal shows a key:\n%s", s.log.String())
	}
}

func TestKeyWriteRefusals(t *testing.T) {
	s := startAPI(t, "")
	tests := []struct {
		provider, key, msg string
	}{
		{"../x", "sk-1", "a provider name may contain lowercase letters, digits, '-' and '_'"},
		{"Ollama", "sk-1", "a provider name may contain lowercase letters, digits, '-' and '_'"},
		{"graft", "sk-1", `"graft" is reserved`},
		{"ollama", " \n", "the key is empty"},
		{"ollama", "sk-1 sk-2", "the key must be one line without spaces"},
		{"ollama", "\ufeffsk-1", "the key contains U+FEFF, which is not a printable ASCII character"},
	}
	for _, tt := range tests {
		var body errorBody
		if code := s.call(t, "POST", "/api/key/write", map[string]any{"provider": tt.provider, "key": tt.key}, &body); code != 422 || body.Error != tt.msg {
			t.Errorf("%q %q: %d %q, want 422 %q", tt.provider, tt.key, code, body.Error, tt.msg)
		}
	}
	entries, _ := os.ReadDir(filepath.Dir(s.path))
	if len(entries) != 0 {
		t.Errorf("a refused write left %v", entries)
	}
}

func TestKeyWriteMakesTheDirectories(t *testing.T) {
	path := filepath.Join(tempDir(t), "new", "config.yaml")
	mux := http.NewServeMux()
	newAPI(Options{Path: path, Log: &lockedBuffer{}}).routes(mux)
	srv := httptest.NewServer(mux)
	defer srv.Close()
	s := &testAPI{srv: srv, path: path}
	if code := s.call(t, "POST", "/api/key/write", map[string]any{"provider": "ollama", "key": "sk-1"}, nil); code != 200 {
		t.Fatalf("write: %d", code)
	}
	for _, dir := range []string{filepath.Dir(path), filepath.Join(filepath.Dir(path), "keys")} {
		if info, err := os.Stat(dir); err != nil || info.Mode().Perm() != 0o700 {
			t.Errorf("%s: %v %v, want mode 0700", dir, info, err)
		}
	}
}

func TestKeyWriteFailure(t *testing.T) {
	s := startAPI(t, "")
	path := filepath.Join(filepath.Dir(s.path), "keys", "ollama")
	if err := os.MkdirAll(filepath.Join(path, "inside"), 0o700); err != nil {
		t.Fatal(err)
	}
	var body errorBody
	if code := s.call(t, "POST", "/api/key/write", map[string]any{"provider": "ollama", "key": "sk-1", "replace": true}, &body); code != 500 {
		t.Fatalf("write over a directory: %d, want 500", code)
	}
	if !strings.HasPrefix(s.log.String(), "graft ui: cannot write "+config.Tildify(path)+": ") {
		t.Errorf("terminal %q", s.log.String())
	}
	entries, _ := os.ReadDir(filepath.Dir(path))
	if len(entries) != 1 {
		t.Errorf("the failed write left %v", entries)
	}
}
```

Запустите: `go test ./internal/ui/`

Ожидается: FAIL, ошибки сборки `undefined: newAPI`, `unknown field Environ`, `undefined: keyState`, `undefined: keyWritten` и `undefined: keyConflict`.

- [ ] **Шаг 7: Сервер ключей**

В `internal/ui/server.go` поля `Options` после `Log`:

```go
	// What a profile would start with, for the key widget and the test
	// request: the environment of graft ui.
	Environ []string
	// Stdin and Stderr are the terminal of graft ui, where a key command may
	// ask for a password.
	Stdin  io.Reader
	Stderr io.Writer
```

`New` строит API через `newAPI(opts)`.

В `internal/ui/api.go`:

```go
// api serves /api/: the draft of the config, its checks and its saves, the
// presets, the keys of providers, the test request and Preview.
type api struct {
	path  string // the config file, absolute
	store *Store
	log   io.Writer // the terminal
	opts  Options
	keys  keys.Reader // reads keys from the environment and the terminal of graft ui

	mu sync.Mutex // one save at a time

	keyMu  sync.Mutex        // one write of a key file at a time
	cmdMu  sync.Mutex        // one key command at a time: each may ask for a password
	keptMu sync.Mutex        // guards kept
	kept   map[string]string // the keys that key commands printed, by the text of the command
}

// newAPI returns the API of the config file opts.Path.
func newAPI(opts Options) *api

// lookupIn finds a variable in environ, as os.LookupEnv does in the
// environment of the process.
func lookupIn(environ []string) func(string) (string, bool)
```

`newAPI` заполняет `path`, `store`, `log` из `opts`, сохраняет `opts`, `keys` — `keys.Reader{LookupEnv: lookupIn(opts.Environ), Stdin: opts.Stdin, Stderr: opts.Stderr}`, `kept` — пустая карта. В `routes` после `GET /api/presets`:

```go
	mux.HandleFunc("POST /api/key/check", a.keyCheck)
	mux.HandleFunc("POST /api/key/write", a.keyWrite)
```

Создайте `internal/ui/keys.go`. Типы и сообщение `errNotRun` — контракт со страницей:

```go
// errNotRun is the answer for a key command that has not run in this
// session: it runs only when the user asks, since it may ask for a password.
var errNotRun = errors.New("the key command has not run yet: press Run at the key of the provider; the command may ask for a password in the terminal of graft ui")

type keyCheckRequest struct {
	Kind  string `json:"kind"`
	Value string `json:"value"`
	Run   bool   `json:"run"` // whether a key command may run now
}

// keyState is what the key widget shows of a key source. The key itself
// never leaves the server.
type keyState struct {
	State       string `json:"state"`                 // ok, missing, unset, not run or error
	Path        string `json:"path,omitempty"`        // the key file graft reads, shown with ~
	Fingerprint string `json:"fingerprint,omitempty"` // the first 12 hex digits of the key's SHA-256
	Warning     string `json:"warning,omitempty"`
	Error       string `json:"error,omitempty"`
}

type keyWriteRequest struct {
	Provider string `json:"provider"`
	Key      string `json:"key"`
	Replace  bool   `json:"replace"`
}

type keyWritten struct {
	Path        string `json:"path"`
	Fingerprint string `json:"fingerprint"`
}

// keyConflict refuses to write over a key file without replace.
type keyConflict struct {
	Error       string `json:"error"`
	Fingerprint string `json:"fingerprint"` // of the key in the file, "" when graft cannot read one there
}

func (a *api) keyCheck(w http.ResponseWriter, r *http.Request)
func (a *api) keyWrite(w http.ResponseWriter, r *http.Request)

// source returns the key source k names, as config.Parse reads it: a file
// with a leading ~ expanded, and relative to the directory of the config.
func (a *api) source(k edit.Key) (config.KeySource, error)

// keyState reads the key of k. A key command runs only when run is set.
func (a *api) keyState(k edit.Key, run bool) keyState

// readKey reads the key of src. A key command runs only when run is set,
// one at a time, since each may ask for a password in the terminal, and its
// key is kept for the session by the text of the command: so a test request
// does not ask for the password again. Without run, the key is the one kept,
// or errNotRun. A failed run drops the kept key. Env and file do not wait
// for a command.
func (a *api) readKey(src config.KeySource, run bool) (key, warning string, err error)

// writeKeyFile puts key into the file at path, readable by its owner only,
// in a directory made with mode 0700 when there is none: a temporary file,
// synced, renamed over the file.
func writeKeyFile(path, key string) error
```

Правила:

- `source`: значение, пустое после `strings.TrimSpace`, — `must not be empty`; `env` — `KeySource{Env: value}`; `file` — `KeySource{File: config.ResolvePath(value, filepath.Dir(a.path))}`, ошибка `ResolvePath` возвращается; `command` — `KeySource{Command: value}`; другой вид — `say where the key comes from: env, file or command`.
- `keyCheck` отвечает 200 и `keyState`: ошибка `source` — `{state: "error", error}`. У `file` всегда есть `path` — `config.Tildify` раскрытого пути; нет файла (`os.Stat` даёт `fs.ErrNotExist`) — `missing`. Переменной `env` нет в `opts.Environ` — `unset`. Дальше `readKey`: `errNotRun` — `not run`; другая ошибка — `error` с её текстом и предупреждением; ключ — `ok` с `keys.Fingerprint` и предупреждением `Reader.Read` (права файла).
- `readKey`: не команда — `a.keys.Read(src)`. Команда без `run` — ключ из `kept` под `keptMu` или `errNotRun`. Команда с `run` — `a.keys.Read(src)` под `cmdMu`, затем под `keptMu` удачный ключ кладётся в `kept`, а неудача его удаляет. `keptMu` отдельный от `cmdMu`: проверка без `run` не ждёт команду, которая ждёт пароль.
- `keyWrite`: `config.CheckProviderName` — 422 с её сообщением; `keys.Clean` — 422 с его ошибкой. Путь — `filepath.Join(filepath.Dir(a.path), "keys", provider)`, показывается через `config.Tildify`. Дальше под `keyMu`: если `os.Lstat` пути удался и `replace` не задан — 409 `keyConflict{Error: "<путь> already holds a key", Fingerprint}`, где отпечаток — ключа, который `a.keys.Read` читает из файла, или `""`. Иначе `writeKeyFile`: `os.MkdirAll` каталога с 0700, `writeTemp(path, []byte(key+"\n"), 0o600)`, `os.Rename` (при ошибке временный файл удаляется), `syncDir`. Неудача — строка `graft ui: cannot write <путь>: <ошибка>` в терминал и 500; удача — строка `graft ui: wrote <путь> (sha256:<отпечаток>)` и 200 `keyWritten`.

В `cmd/graft/ui.go` `ui.New` получает ещё `Environ: sys.environ, Stdin: sys.stdin, Stderr: sys.stderr`.

- [ ] **Шаг 8: Полная проверка**

Запустите: `test -z "$(gofmt -l .)" && go vet ./... && GOOS=darwin go vet ./... && go test -race ./... && node --test internal/ui/jstest/draft.test.mjs`

Ожидается: код 0; `TestKeyCommandsRunOneAtATime` проходит и под `-race`.

- [ ] **Шаг 9: Commit**

```bash
git add internal/ui/server.go internal/ui/api.go internal/ui/api_test.go internal/ui/keys.go internal/ui/keys_test.go cmd/graft/ui.go
git commit -m "feat(ui): show the state of a key and save a pasted key" -m "The key widget asks the server what a key source gives: a state and a fingerprint, never the key. A key command runs only when the user presses Run, one at a time, since it may ask for a password in the terminal, and its key stays in memory for the test request. Save key writes keys/<provider> next to the config, readable only by its owner, and the terminal says so with the fingerprint."
```

### Task 4: Тестовый запрос

Кнопка Test проверяет адрес провайдера, ключ и ID модели до сохранения конфига: один запрос Messages на 16 токенов через транспорт маршрутизатора.

**Files:**
- Modify: `internal/router/router.go`, `internal/ui/api.go` (маршрут)
- Create: `internal/ui/testrequest.go`, `internal/ui/testrequest_test.go`

**Interfaces:**
- Consumes: `config.CheckURL`, `(*api).source`, `(*api).readKey`, `errNotRun`, `startWith`, `(*testAPI).checkKey` (задача 3).
- Produces:
  - `func router.Transport() *http.Transport` (сейчас `transport`);
  - `POST /api/test`: запрос `{url, key: {kind, value}, id}`; 200 `{status, ms, text, body}`; 422 и 502 `{error}`;
  - `var testTimeout = 60 * time.Second`; `func redact(s, key string) string`, `func cutRunes(s string, n int) string`, `func cutBytes(s string, n int) string`.

- [ ] **Шаг 1: Тесты тестового запроса**

Создайте `internal/ui/testrequest_test.go`:

```go
package ui

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
	"unicode/utf8"
)

// provider is a fake provider: it answers each request with status and
// body, and keeps the last request.
type provider struct {
	*httptest.Server
	status int
	body   string
	calls  int
	last   *http.Request
	sent   map[string]any // the JSON body of the last request
}

func startProvider(t *testing.T, status int, body string) *provider {
	t.Helper()
	p := &provider{status: status, body: body}
	p.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		p.calls++
		p.last = r
		p.sent = nil
		json.NewDecoder(r.Body).Decode(&p.sent)
		if p.status/100 == 3 {
			w.Header().Set("Location", "http://elsewhere.invalid/")
		}
		w.WriteHeader(p.status)
		io.WriteString(w, p.body)
	}))
	t.Cleanup(p.Close)
	return p
}

// sendTestRequest asks the API to test model id at url with the key of the
// variable TEST_KEY, sk-test.
func sendTestRequest(t *testing.T, s *testAPI, url, id string, out any) int {
	t.Helper()
	return s.call(t, "POST", "/api/test", map[string]any{"url": url, "key": map[string]string{"kind": "env", "value": "TEST_KEY"}, "id": id}, out)
}

func testEnv() Options { return Options{Environ: []string{"TEST_KEY=sk-test"}} }

func TestTestRequest(t *testing.T) {
	p := startProvider(t, 200, `{"type":"message","content":[{"type":"thinking","thinking":"hm"},{"type":"text","text":"pong"}]}`)
	s := startWith(t, "", testEnv())
	var res testResult
	if code := sendTestRequest(t, s, p.URL+"/anthropic/", "glm-5.3-flash", &res); code != 200 {
		t.Fatalf("POST /api/test: %d", code)
	}
	if res.Status != 200 || res.Text != "pong" || res.Body != "" || res.MS < 0 {
		t.Errorf("result %+v", res)
	}
	r := p.last
	if r.Method != "POST" || r.URL.Path != "/anthropic/v1/messages" {
		t.Errorf("request %s %s, want POST /anthropic/v1/messages", r.Method, r.URL.Path)
	}
	for name, want := range map[string]string{"Authorization": "Bearer sk-test", "Anthropic-Version": "2023-06-01", "Content-Type": "application/json"} {
		if got := r.Header.Get(name); got != want {
			t.Errorf("header %s: %q, want %q", name, got, want)
		}
	}
	want := map[string]any{
		"model":      "glm-5.3-flash",
		"max_tokens": float64(16),
		"messages":   []any{map[string]any{"role": "user", "content": "Reply with one word: pong"}},
	}
	if !reflect.DeepEqual(p.sent, want) {
		t.Errorf("body %v, want %v", p.sent, want)
	}
}

func TestTestRequestJoinsTheURL(t *testing.T) {
	p := startProvider(t, 200, `{"content":[]}`)
	s := startWith(t, "", testEnv())
	for base, path := range map[string]string{"": "/v1/messages", "/": "/v1/messages", "/api": "/api/v1/messages"} {
		if code := sendTestRequest(t, s, p.URL+base, "m", nil); code != 200 || p.last.URL.Path != path {
			t.Errorf("url %q: %d, path %q, want %q", base, code, p.last.URL.Path, path)
		}
	}
}

// TestTestRequestAnswers: the page gets the status and some of the answer,
// never the key.
func TestTestRequestAnswers(t *testing.T) {
	long := strings.Repeat("é", 300)
	tests := []struct {
		name   string
		status int
		body   string
		want   testResult
	}{
		{"a model that only thinks", 200, `{"content":[{"type":"thinking","thinking":"..."}]}`, testResult{Status: 200}},
		{"a long answer", 200, `{"content":[{"type":"text","text":"sk-test ` + long + `"}]}`,
			testResult{Status: 200, Text: "<key> " + strings.Repeat("é", 194)}},
		{"a refused key", 401, `{"error":{"message":"invalid key sk-test"}}`,
			testResult{Status: 401, Body: `{"error":{"message":"invalid key <key>"}}`}},
		{"a long error", 500, "x" + strings.Repeat("é", 1500), testResult{Status: 500, Body: "x" + strings.Repeat("é", 1023)}},
		// Cut first, the body would end in sk-t, the start of the key.
		{"a key across the cut", 401, strings.Repeat("x", 2044) + "sk-test", testResult{Status: 401, Body: strings.Repeat("x", 2044) + "<key"}},
		{"a redirect", 307, "", testResult{Status: 307}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := startProvider(t, tt.status, tt.body)
			s := startWith(t, "", testEnv())
			var res testResult
			if code := sendTestRequest(t, s, p.URL, "m", &res); code != 200 {
				t.Fatalf("POST /api/test: %d", code)
			}
			res.MS = 0
			if res != tt.want {
				t.Errorf("result %+v, want %+v", res, tt.want)
			}
			if !utf8.ValidString(res.Body) || len(res.Body) > 2048 || utf8.RuneCountInString(res.Text) > 200 {
				t.Errorf("text %d characters, body %d bytes", utf8.RuneCountInString(res.Text), len(res.Body))
			}
			if p.calls != 1 {
				t.Errorf("%d requests, want 1: no retry and no redirect", p.calls)
			}
		})
	}
}

func TestTestRequestRefusals(t *testing.T) {
	s := startWith(t, "", testEnv())
	dir := t.TempDir()
	cmd := "cat '" + filepath.Join(dir, "key") + "'"
	tests := []struct {
		name string
		req  map[string]any
		code int
		msg  string
	}{
		{"not http", map[string]any{"url": "ftp://x", "key": map[string]string{"kind": "env", "value": "TEST_KEY"}, "id": "m"},
			422, "url must be an absolute http or https URL"},
		{"a query", map[string]any{"url": "https://x/?a=1", "key": map[string]string{"kind": "env", "value": "TEST_KEY"}, "id": "m"},
			422, "url must not have a query or fragment: Claude Code appends /v1/messages to it"},
		{"no id", map[string]any{"url": "https://x", "key": map[string]string{"kind": "env", "value": "TEST_KEY"}, "id": ""},
			422, "the model needs an id or a name, without white space"},
		{"no key", map[string]any{"url": "https://x", "key": map[string]string{"kind": "env", "value": "MISSING"}, "id": "m"},
			422, "env MISSING is not set"},
		{"an empty source", map[string]any{"url": "https://x", "key": map[string]string{"kind": "file", "value": ""}, "id": "m"},
			422, "key: must not be empty"},
		{"a command not run", map[string]any{"url": "https://x", "key": map[string]string{"kind": "command", "value": cmd}, "id": "m"},
			422, errNotRun.Error()},
		{"no provider", map[string]any{"url": "http://127.0.0.1:1", "key": map[string]string{"kind": "env", "value": "TEST_KEY"}, "id": "m"},
			502, "connection refused"},
	}
	for _, tt := range tests {
		var body errorBody
		code := s.call(t, "POST", "/api/test", tt.req, &body)
		if code != tt.code || !strings.Contains(body.Error, tt.msg) {
			t.Errorf("%s: %d %q, want %d %q", tt.name, code, body.Error, tt.code, tt.msg)
		}
	}
}

// TestTestRequestUsesTheKeptKey: the test asks no password again.
func TestTestRequestUsesTheKeptKey(t *testing.T) {
	p := startProvider(t, 200, `{"content":[{"type":"text","text":"pong"}]}`)
	s := startAPI(t, "")
	dir := t.TempDir()
	count := filepath.Join(dir, "count")
	cmd := "echo run >> '" + count + "'; echo sk-cmd"
	s.checkKey(t, "command", cmd, true)
	req := map[string]any{"url": p.URL, "key": map[string]string{"kind": "command", "value": cmd}, "id": "m"}
	if code := s.call(t, "POST", "/api/test", req, nil); code != 200 {
		t.Fatalf("POST /api/test: %d", code)
	}
	if got := p.last.Header.Get("Authorization"); got != "Bearer sk-cmd" {
		t.Errorf("Authorization %q", got)
	}
	if data, _ := os.ReadFile(count); string(data) != "run\n" {
		t.Errorf("the command ran %d times, want once", strings.Count(string(data), "run"))
	}
}

func TestTestRequestTimesOut(t *testing.T) {
	defer func(d time.Duration) { testTimeout = d }(testTimeout)
	testTimeout = 100 * time.Millisecond
	release := make(chan struct{})
	p := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { <-release }))
	defer p.Close()
	defer close(release)
	s := startWith(t, "", testEnv())
	var body errorBody
	if code := sendTestRequest(t, s, p.URL, "m", &body); code != 502 || !strings.Contains(body.Error, "Client.Timeout exceeded") {
		t.Errorf("%d %q, want 502 and a timeout", code, body.Error)
	}
}
```

- [ ] **Шаг 2: Убедиться, что тесты не собираются**

Запустите: `go test ./internal/ui/`

Ожидается: FAIL, ошибки сборки `undefined: testResult` и `undefined: testTimeout`.

- [ ] **Шаг 3: `router.Transport`**

В `internal/router/router.go` переименуйте `transport` в `Transport`, с комментарием `// Transport returns the transport to providers and Anthropic: …`; `Handler` вызывает `Transport()`.

- [ ] **Шаг 4: `internal/ui/testrequest.go`**

Типы и пределы — контракт и значения дизайна:

```go
// testTimeout is how long a test request may take in all; a test makes it
// shorter.
var testTimeout = 60 * time.Second

const (
	maxAnswer    = 200     // characters of a 2xx answer that the page shows
	maxErrorBody = 2 << 10 // bytes of the body of another status
	maxRead      = 1 << 20 // bytes of a response the server reads
)

type testRequest struct {
	URL string   `json:"url"`
	Key edit.Key `json:"key"`
	ID  string   `json:"id"`
}

// testResult is the provider's answer to a test request, without its key.
type testResult struct {
	Status int    `json:"status"`
	MS     int64  `json:"ms"`
	Text   string `json:"text"` // of a 2xx: the first text block; "" when there is none
	Body   string `json:"body"` // of another status
}

// testMessage is the body of a test request.
type testMessage struct {
	Model     string        `json:"model"`
	MaxTokens int           `json:"max_tokens"`
	Messages  []testContent `json:"messages"`
}

type testContent struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

func (a *api) test(w http.ResponseWriter, r *http.Request)

// sendTest asks model id at the provider at base for one word, as Claude
// Code would ask it: through the router's transport, with key as a bearer
// token, without a stream or retries, and without following a redirect,
// which the router does not follow either.
func sendTest(ctx context.Context, base, key, id string) (testResult, error)

// firstText returns the text of the first text block of a Messages API
// answer, or "": a model that thinks may spend its 16 tokens before it
// writes.
func firstText(data []byte) string

// redact puts <key> in place of each occurrence of key in s.
func redact(s, key string) string

// cutRunes returns the first n characters of s.
func cutRunes(s string, n int) string

// cutBytes returns at most the first n bytes of s, without a character cut
// in two.
func cutBytes(s string, n int) string
```

Правила `test`, по порядку:

1. `config.CheckURL(url)` не пуст — 422 `url <сообщение>`.
2. `id` пуст или содержит пробельный символ — 422 `the model needs an id or a name, without white space`.
3. Ошибка `a.source(key)` — 422 `key: <ошибка>`.
4. Ошибка `a.readKey(src, false)` — 422 с её текстом как есть: для команды без Run это `errNotRun`.
5. Ошибка `sendTest` — 502 с её текстом, где ключ заменён через `redact`.
6. Иначе 200 `testResult`.

Правила `sendTest`:

- путь URL — `strings.TrimSuffix(u.Path, "/") + "/v1/messages"`, `RawPath` пуст;
- тело — JSON `testMessage{Model: id, MaxTokens: 16, Messages: []testContent{{Role: "user", Content: "Reply with one word: pong"}}}`, заголовки — из Global Constraints;
- клиент — `&http.Client{Transport: router.Transport(), Timeout: testTimeout, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}`;
- `MS` — от отправки до конца чтения ответа; читается не больше `maxRead` байт;
- для 2xx `Text` — `cutRunes(redact(firstText(data), key), maxAnswer)`, для другого статуса `Body` — `cutBytes(redact(string(data), key), maxErrorBody)`. Замена ключа идёт до обрезки, иначе обрезка оставила бы начало ключа.

`cutBytes` отступает от `n` к началу символа (`utf8.RuneStart`), чтобы не разрезать его. В `routes` после `POST /api/key/write`:

```go
	mux.HandleFunc("POST /api/test", a.test)
```

- [ ] **Шаг 5: Полная проверка**

Запустите: `test -z "$(gofmt -l .)" && go vet ./... && GOOS=darwin go vet ./... && go test -race ./... && node --test internal/ui/jstest/draft.test.mjs`

Ожидается: код 0.

- [ ] **Шаг 6: Commit**

```bash
git add internal/router/router.go internal/ui/api.go internal/ui/testrequest.go internal/ui/testrequest_test.go
git commit -m "feat(ui): send a test request to a model" -m "Test checks the URL of a provider, its key and a model ID before the config is saved: one Messages request of 16 tokens through the router's transport, so that it takes the proxy the session would take. The page gets the status and the start of the answer, with the key replaced wherever the provider repeats it."
```

### Task 5: Preview

Вкладка Preview показывает, что запустит профиль: сервер сливает черновик, проверяет конфиг, читает файлы настроек Claude Code, как `graft PROFILE`, и печатает план через `launch.PlanText`, с ключами в виде источника.

**Files:**
- Modify: `internal/ui/server.go`, `internal/ui/api.go`, `cmd/graft/ui.go`, `cmd/graft/ui_test.go`
- Create: `internal/ui/preview.go`, `internal/ui/preview_test.go`

**Interfaces:**
- Consumes: `launch.PlanText`, `plan.LoadSettingsFor` (задача 1); `plan.Build`, `keys.Describe`; `newAPI`, `startWith` (задача 3).
- Produces:
  - поля `ui.Options`: `Home`, `Dir string`, `LookPath func(string) (string, error)`;
  - `func (a *api) check(data []byte, d edit.Draft) ([]byte, *config.Config, checkResult)` — теперь отдаёт и конфиг;
  - `POST /api/preview`: запрос `{version, draft, profile}`; 200 `{stale, errors, warnings, text, failure}`;
  - `func (a *api) planText(cfg *config.Config, profile string) (string, []string, error)`;
  - в `cmd/graft/ui_test.go`: `callAPI(t, url, method, path, body, out)`.

- [ ] **Шаг 1: Тесты Preview**

Создайте `internal/ui/preview_test.go`:

```go
package ui

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/zinin/graft/internal/config"
)

// previewConfig has a key in a file and a key from a command that leaves a
// mark when it runs.
func previewConfig(mark string) string {
	return `providers:
  ollama: {url: https://ollama.com, key: {file: keys/ollama}}
  local: {url: "http://127.0.0.1:8317", key: {command: "touch '` + mark + `'; echo sk"}}
models:
  glm: {provider: ollama, id: glm-5.3-flash, context: 1048576}
  sol: {provider: local, id: gpt-6.1-sol, context: 400000}
profiles:
  glm: {model: glm}
  mix: {model: opus, picker: [glm, sol]}
`
}

func previewOptions(t *testing.T) Options {
	return Options{
		Environ:  []string{"ANTHROPIC_API_KEY=sk-ant-leak", "PATH=/usr/bin:/bin"},
		Home:     t.TempDir(),
		Dir:      t.TempDir(),
		LookPath: func(name string) (string, error) { return "/opt/claude/bin/" + name, nil },
	}
}

// previewOf asks the API for the preview of profile in the draft of the file.
func (s *testAPI) previewOf(t *testing.T, profile string) previewResult {
	t.Helper()
	st := s.load(t)
	var res previewResult
	if code := s.call(t, "POST", "/api/preview", map[string]any{"version": st.Version, "draft": st.Draft, "profile": profile}, &res); code != 200 {
		t.Fatalf("POST /api/preview: %d", code)
	}
	return res
}

// TestPreview: the text of --dry-run, with each key as its source; no key is
// read.
func TestPreview(t *testing.T) {
	mark := filepath.Join(t.TempDir(), "ran")
	s := startWith(t, previewConfig(mark), previewOptions(t))
	file := "<file " + config.Tildify(filepath.Join(filepath.Dir(s.path), "keys", "ollama")) + ">"
	command := `<command "touch '` + mark + `'; echo sk">`
	tests := map[string][]string{
		"glm": {
			"profile glm: direct, provider ollama\n",
			"unset  ANTHROPIC_API_KEY\n",
			"set    ANTHROPIC_BASE_URL=https://ollama.com\n",
			"       ANTHROPIC_AUTH_TOKEN=" + file + "\n",
			"exec   /opt/claude/bin/claude --model glm-5.3-flash --settings '{",
			"settings\n{\n",
		},
		"mix": {
			"profile mix: router, providers ollama, local, subscription\n",
			"route  glm -> ollama glm-5.3-flash " + file + "\n",
			"       sol -> local gpt-6.1-sol " + command + "\n",
		},
	}
	for profile, lines := range tests {
		res := s.previewOf(t, profile)
		if res.Stale || len(res.Errors) > 0 || res.Failure != "" || len(res.Warnings) > 0 {
			t.Errorf("%s: %+v", profile, res)
		}
		for _, line := range lines {
			if !strings.Contains(res.Text, line) {
				t.Errorf("%s: the preview lacks %q:\n%s", profile, line, res.Text)
			}
		}
	}
	if _, err := os.Stat(mark); err == nil {
		t.Error("Preview ran a key command")
	}
}

func TestPreviewOfADraftWithErrors(t *testing.T) {
	s := startWith(t, previewConfig("/nonexistent/mark"), previewOptions(t))
	st := s.load(t)
	d := *st.Draft
	d.Models[0].Context = "5"
	var res previewResult
	s.call(t, "POST", "/api/preview", map[string]any{"version": st.Version, "draft": d, "profile": "glm"}, &res)
	if len(res.Errors) != 1 || res.Errors[0].Path != "models.glm.context" || res.Text != "" {
		t.Errorf("result %+v", res)
	}
	res = previewResult{}
	s.call(t, "POST", "/api/preview", map[string]any{"version": "old", "draft": st.Draft, "profile": "glm"}, &res)
	if !res.Stale || res.Text != "" {
		t.Errorf("a stale version: %+v", res)
	}
}

func TestPreviewFailures(t *testing.T) {
	broken := previewOptions(t)
	if err := os.MkdirAll(filepath.Join(broken.Dir, ".claude"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(broken.Dir, ".claude", "settings.json"), []byte(`{"apiKeyHelper": "x"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	unreadable := previewOptions(t)
	if err := os.MkdirAll(filepath.Join(unreadable.Dir, ".claude", "settings.local.json"), 0o700); err != nil {
		t.Fatal(err)
	}
	noClaude := previewOptions(t)
	noClaude.LookPath = func(string) (string, error) { return "", errors.New("not found") }
	tests := []struct {
		name    string
		opts    Options
		profile string
		failure string // substring; "" means none
		warning string // substring; "" means none
	}{
		{"settings that would override the key", broken, "glm", "Claude Code settings would override its address or key", ""},
		{"a settings file that cannot be read", unreadable, "glm", "", "settings.local.json"},
		{"no claude", noClaude, "glm", "claude not found in PATH", ""},
		{"no such profile", previewOptions(t), "nope", `no profile "nope"`, ""},
	}
	for _, tt := range tests {
		s := startWith(t, previewConfig("/nonexistent/mark"), tt.opts)
		res := s.previewOf(t, tt.profile)
		if (tt.failure == "") != (res.Failure == "") || !strings.Contains(res.Failure, tt.failure) {
			t.Errorf("%s: failure %q, want %q", tt.name, res.Failure, tt.failure)
		}
		if got := strings.Join(res.Warnings, "\n"); (tt.warning == "") != (got == "") || !strings.Contains(got, tt.warning) {
			t.Errorf("%s: warnings %q, want %q", tt.name, got, tt.warning)
		}
	}
}
```

В `cmd/graft/ui_test.go` добавьте импорты `regexp` и `github.com/zinin/graft/internal/keys` и допишите в конец:

```go
// callAPI sends a request to the API of the page at url and decodes the
// JSON answer into out.
func callAPI(t *testing.T, url, method, path string, body, out any) int {
	t.Helper()
	base, token, _ := strings.Cut(url, "/#token=")
	data, _ := json.Marshal(body)
	req, _ := http.NewRequest(method, base+path, bytes.NewReader(data))
	req.Header.Set("Authorization", "Bearer "+token)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
		t.Fatalf("%s %s: %v", method, path, err)
	}
	return resp.StatusCode
}

// TestPreviewMatchesDryRun: Preview prints what --dry-run prints, with each
// key shown by its source instead of its source and fingerprint, and graft
// ui reads keys from its own environment.
func TestPreviewMatchesDryRun(t *testing.T) {
	f := newFake(t, "OLLAMA_API_KEY=sk-ollama", "ANTHROPIC_API_KEY=sk-ant-leak")
	f.writeConfig(t)
	fingerprint := regexp.MustCompile(`, sha256:[0-9a-f]{12}>`)
	profiles := []string{"glm-proxy", "mix"}
	want := map[string]string{}
	for _, profile := range profiles {
		f.stdout.Reset()
		if code := run([]string{"--dry-run", profile}, f.sys); code != 0 {
			t.Fatalf("--dry-run %s: code %d, stderr %s", profile, code, f.stderr.String())
		}
		want[profile] = fingerprint.ReplaceAllString(f.stdout.String(), ">")
	}
	r := startUI(t, f)
	url := r.url(t)
	var st struct {
		Version string         `json:"version"`
		Draft   map[string]any `json:"draft"`
	}
	callAPI(t, url, "GET", "/api/config", nil, &st)
	for _, profile := range profiles {
		var res struct {
			Text, Failure string
		}
		callAPI(t, url, "POST", "/api/preview", map[string]any{"version": st.Version, "draft": st.Draft, "profile": profile}, &res)
		if res.Text != want[profile] || res.Failure != "" {
			t.Errorf("Preview of %s:\n%s%s\nwant what --dry-run prints:\n%s", profile, res.Text, res.Failure, want[profile])
		}
	}
	var key struct {
		State, Fingerprint string
	}
	callAPI(t, url, "POST", "/api/key/check", map[string]any{"kind": "env", "value": "OLLAMA_API_KEY"}, &key)
	if key.State != "ok" || key.Fingerprint != keys.Fingerprint("sk-ollama") {
		t.Errorf("the key of env OLLAMA_API_KEY: %+v", key)
	}
	r.finish(t)
}
```

- [ ] **Шаг 2: Убедиться, что тесты не проходят**

Запустите: `go test ./internal/ui/ ./cmd/graft/`

Ожидается: FAIL, в `internal/ui` ошибки сборки `unknown field Home` (и `Dir`, `LookPath`) и `undefined: previewResult`, в `cmd/graft` `TestPreviewMatchesDryRun` падает на `POST /api/preview`: маршрута нет.

- [ ] **Шаг 3: Сервер Preview**

В `internal/ui/server.go` комментарий над `Environ` и новые поля `Options`:

```go
	// What a profile would start with, for the key widget, the test request
	// and Preview: the environment of graft ui, its home directory and the
	// directory it runs in, whose .claude settings Preview checks.
	Environ []string
	Home    string
	Dir     string
	// LookPath finds claude for Preview; exec.LookPath outside tests.
	LookPath func(string) (string, error)
```

В `internal/ui/api.go` `check` возвращает и конфиг, который проверил: `([]byte, *config.Config, checkResult)`, `nil` вместе с файлом при ошибках; `validate` и `save` его не берут. В `routes` после `POST /api/test`:

```go
	mux.HandleFunc("POST /api/preview", a.preview)
```

Создайте `internal/ui/preview.go`:

```go
type previewRequest struct {
	Version string     `json:"version"`
	Draft   edit.Draft `json:"draft"`
	Profile string     `json:"profile"`
}

// previewResult is what the tab Preview shows of a profile.
type previewResult struct {
	Stale    bool      `json:"stale"`    // the file changed: nothing was checked
	Errors   []problem `json:"errors"`   // of the config: there is no preview while it has any
	Warnings []string  `json:"warnings"` // about Claude Code's settings files
	Text     string    `json:"text"`     // what graft --dry-run would print, with each key as its source
	Failure  string    `json:"failure"`  // why graft would not start the profile
}

func (a *api) preview(w http.ResponseWriter, r *http.Request)

// planText returns what graft --dry-run would print for profile when it
// starts in the directory of graft ui, with its environment, and the
// warnings about Claude Code's settings files. It shows each key as its
// source and reads none.
func (a *api) planText(cfg *config.Config, profile string) (string, []string, error)
```

Правила:

- `preview`: `Errors` и `Warnings` в ответе — всегда списки, `[]` когда пусто. Ошибка чтения файла — 500. Версия файла не та — 200 `{stale: true}`. Ошибки `check` — 200 с ними. Иначе `planText`: текст и предупреждения в ответ, ошибка — в `Failure`.
- `planText`: `plan.LoadSettingsFor(cfg, profile, opts.Environ, opts.Home, opts.Dir)` (её ошибка — отказ), `plan.Build(plan.Input{Config: cfg, Profile: profile, Environ: opts.Environ, ClaudeSettings: files})` без аргументов командной строки (ошибка — отказ), `claude` через `opts.LookPath` (`nil` — `exec.LookPath`; не нашёлся — `claude not found in PATH`), затем `launch.PlanText` с `show`, который возвращает `"<" + keys.Describe(cfg.Providers[provider].Key) + ">"` и не читает ключ.

В `cmd/graft/ui.go` `ui.New` получает ещё `Home: sys.home, Dir: sys.cwd, LookPath: sys.lookPath`.

- [ ] **Шаг 4: Полная проверка**

Запустите: `test -z "$(gofmt -l .)" && go vet ./... && GOOS=darwin go vet ./... && go test -race ./... && node --test internal/ui/jstest/draft.test.mjs`

Ожидается: код 0; `TestPreviewMatchesDryRun` показывает, что Preview и `--dry-run` расходятся только отпечатками ключей.

- [ ] **Шаг 5: Commit**

```bash
git add internal/ui/server.go internal/ui/api.go internal/ui/preview.go internal/ui/preview_test.go cmd/graft/ui.go cmd/graft/ui_test.go
git commit -m "feat(ui): preview what a profile would start" -m "Preview merges the draft, checks the config and builds the plan as graft PROFILE builds it in the directory of graft ui, with its environment and Claude Code's settings files there. It prints the text of --dry-run with each key shown by its source: Preview reads no key, so it never runs a key command."
```

### Task 6: Логика страницы этапа 2 в `draft.js`

Чистые функции пути новичка, которые страница берёт в задачах 7 и 8: свободные имена, записи из диалога, пресет провайдера по URL, цель кнопки Test, подсказка после сохранения и команда для терминала.

**Files:**
- Modify: `internal/ui/static/draft.js`, `internal/ui/jstest/draft.test.mjs`

**Interfaces:**
- Consumes: `keysDir`, `newModel`, `newProfile`, `withKeys` из `draft.js`.
- Produces (экспорты `draft.js`):
  - `defaultKeyPath(configPath, name)` — `keys/<имя>` рядом с конфигом; через неё теперь берут путь `newProvider` и `renameProvider`;
  - `freeName(entries, name)`;
  - `addProvider(draft, choice)` → `{ draft, models, profiles }`, где `choice` — `{ name, url, key: { kind, value }, models: [{ name, id, context, label }], perModel, mix }`, а `models` и `profiles` — имена добавленного;
  - `presetFor(presets, url)` → пресет или `null`;
  - `testTarget(draft, model)` → `{ url, key, id }` или `null`;
  - `runHint(names)` → строка подсказки;
  - `shellWord(path)`, `keyCommand(path)` → строки для терминала.

- [ ] **Шаг 1: Тесты**

В `internal/ui/jstest/draft.test.mjs` замените импорт:

```js
import {
  addProvider, canSave, carryKeys, carryName, defaultKeyPath, emptyDraft, envValue, freeName, isSaveKey, keepTyped,
  keyCommand, keysDir, locate, messagesFor, newModel, newName, newProfile, newProvider, own, placeErrors, presetFor,
  renameProvider, renameRefs, runHint, sameDraft, setAt, shellWord, subscribed, takePlaces, testTarget, usedBy,
  withKeys, yamlBoolean,
} from '../static/draft.js';
```

и допишите в конец файла:

```js
test('freeName takes the name, or the first free one with a number', () => {
  assert.equal(freeName([{ name: 'kimi' }], 'ollama'), 'ollama');
  assert.equal(freeName([{ name: 'ollama' }], 'ollama'), 'ollama-2');
  assert.equal(freeName([{ name: 'ollama' }, { name: 'ollama-2' }, { name: 'ollama-4' }], 'ollama'), 'ollama-3');
});

test('defaultKeyPath is where Save key writes', () => {
  assert.equal(defaultKeyPath('~/.config/graft/config.yaml', 'ollama'), '~/.config/graft/keys/ollama');
  assert.equal(defaultKeyPath('/etc/graft/graft.yaml', 'x'), '/etc/graft/keys/x');
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

test('addProvider keeps an id or a window left empty empty', () => {
  const { draft } = addProvider(emptyDraft(), { ...CHOICE, models: [{ name: 'mine', id: '', context: '' }] });
  assert.equal(draft.models[0].id, '');
  assert.equal(draft.models[0].context, '');
  assert.equal(draft.models[0].label, '');
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

test('shellWord quotes for the shell and keeps ~ outside the quotes', () => {
  assert.equal(shellWord('~/.config/graft/keys/ollama'), '~/.config/graft/keys/ollama');
  assert.equal(shellWord('~/My Configs/keys/x'), "~/'My Configs/keys/x'");
  assert.equal(shellWord("/tmp/it's"), "'/tmp/it'\\''s'");
  assert.equal(shellWord('~'), '~');
});

test('keyCommand writes the key file that only its owner reads', () => {
  assert.equal(keyCommand('~/.config/graft/keys/ollama'), 'mkdir -p ~/.config/graft/keys && (umask 077; cat > ~/.config/graft/keys/ollama)');
  assert.equal(keyCommand('~/key'), 'mkdir -p ~ && (umask 077; cat > ~/key)');
  assert.equal(keyCommand('/srv/My Keys/x'), "mkdir -p '/srv/My Keys' && (umask 077; cat > '/srv/My Keys/x')");
});
```

- [ ] **Шаг 2: Убедиться, что тесты падают**

Запустите: `node --test internal/ui/jstest/draft.test.mjs`

Ожидается: FAIL, `SyntaxError: The requested module '../static/draft.js' does not provide an export named 'addProvider'`.

- [ ] **Шаг 3: Функции в `internal/ui/static/draft.js`**

`defaultKeyPath` встаёт перед `newProvider`, остальные — перед `isClaude`:

```js
// defaultKeyPath is the key file of provider name next to the config at
// configPath: the file Save key writes.
export function defaultKeyPath(configPath, name)

// freeName returns name when no entry has it, else name-2, name-3 or the
// first such name that no entry has.
export function freeName(entries, name)

// addProvider returns what the dialog Add provider adds to draft: a copy of
// draft with the provider of choice (name, url and key), a model of it for
// each of choice.models ({name, id, context, label}), and the profiles: with
// choice.perModel one on each model, named after it; with choice.mix, and a
// model at least, mix, with opus as its model and the models in its picker.
// A model or a profile whose name is taken gets the first free one (see
// freeName). It returns the draft and the names of the models and of the
// profiles it added.
export function addProvider(draft, choice)

// presetFor returns the preset whose URL is url, a slash at the end aside,
// or null: the provider form takes the page for keys from it.
export function presetFor(presets, url)

// testTarget returns what the Test of model m sends: the URL and the key of
// its provider in draft, and its id, which defaults to its name; null while
// draft has no such provider.
export function testTarget(draft, m)

// runHint tells how to start the profiles names once a save has written
// them.
export function runHint(names)

// shellWord quotes path for a POSIX shell, as --dry-run quotes its command
// line, but leaves a leading ~ outside the quotes, so that the shell still
// expands it.
export function shellWord(path)

// keyCommand returns a command for a terminal that writes what is pasted
// into it to the key file at path, which only its owner may read.
export function keyCommand(path)
```

Правила:

- `addProvider` не меняет `draft`: работает на `structuredClone`. Провайдер — `{ was: '', name, url, key: { ...key } }`. Модель — `newModel(имя, choice.name)` с `id` (или `''`), `context` строкой (`String(1048576)`; пустое остаётся `''`) и `label` (или `''`). Свободные имена считаются с учётом уже добавленного этим же вызовом. Ключи страницы — `withKeys`.
- `shellWord`: слово из букв, цифр и `_@%+=:,./-` (`/^[A-Za-z0-9_@%+=:,./-]+$/`, как `shellSafe` в `launch`) остаётся как есть, другое берётся в одинарные кавычки, а `'` внутри — `'\''`. `~` и `~/` остаются как есть, у пути на `~/` в кавычки берётся только остаток.
- `keyCommand(path)` — `mkdir -p <каталог> && (umask 077; cat > <path>)`, оба через `shellWord`; каталог — всё до последней `/`. Для `~/.config/graft/keys/ollama` это ровно команда дизайна.

- [ ] **Шаг 4: Тесты проходят**

Запустите: `node --test internal/ui/jstest/draft.test.mjs`

Ожидается: 46 тестов, все `ok`.

- [ ] **Шаг 5: Полная проверка и commit**

Запустите: `test -z "$(gofmt -l .)" && go vet ./... && go test -race ./... && node --test internal/ui/jstest/draft.test.mjs && make ui-smoke`

Ожидается: код 0, smoke — 25 строк `PASS`.

```bash
git add internal/ui/static/draft.js internal/ui/jstest/draft.test.mjs
git commit -m "feat(ui): add the logic of the newcomer's path to draft.js" -m "The dialog Add provider and the key widget need free names, the entries a preset adds, the preset of a provider, the target of Test, the hint after a save and the command that writes a key file in a terminal. They need no DOM, so they live in draft.js under node --test, as the design asks."
```

### Task 7: Страница: виджет ключа, Test и Preview

Форма провайдера получает виджет ключа, форма модели — кнопку Test, форма профиля — вкладку Preview. Smoke получает собственный `HOME` и чистое окружение: виджет ключа читает ключи, и чужие ключи ему читать нельзя.

**Files:**
- Create: `internal/ui/static/checks.js`
- Modify: `internal/ui/static/fields.js`, `internal/ui/static/forms.js`, `internal/ui/static/app.js`, `internal/ui/static/style.css`, `internal/ui/jstest/smoke.mjs`

**Interfaces:**
- Consumes: `GET /api/presets` (задача 2); `POST /api/key/check`, `POST /api/key/write` (задача 3); `POST /api/test` (задача 4); `POST /api/preview` (задача 5); `defaultKeyPath`, `keyCommand`, `presetFor`, `testTarget` (задача 6).
- Produces:
  - `checks.js`: `KeyWidget({ value, onChange, errors, provider, keysLink, page, onSaved })`, `TestButton({ target, page })`, `Preview({ draft, profile, page })`;
  - `page` — проп форм от `App`: `{ file, presets, request, stopped, errors }`, где `file` — состояние файла страницы, `presets` — список пресетов (`[]`, пока их нет), `request(method, path, body)` — запрос к API, `stopped` — сервер остановлен, `errors` — число ошибок последней проверки;
  - `KeySource({ value, onChange, errors, children })` в `fields.js`: `children` стоят в конце его `<fieldset>`;
  - зацепки для smoke: `.key-state`, `input[aria-label="Key to save"]`, кнопки `Save key`, `Run`, `Copy`, `code.key-command`, кнопка `Test (spends a few tokens)`, `.test-result`, вкладки — кнопки `Edit` и `Preview`, `pre.plan`.

- [ ] **Шаг 1: Smoke: свой `HOME`, ненастоящий `claude` и три проверки**

В `internal/ui/jstest/smoke.mjs`:

- `startUI` запускает `graft ui` только с таким окружением и в каталоге `home`:

  ```js
  // graft ui gets a home of the smoke's own and nothing else of the user's
  // environment: the key widget reads keys, and must never read the user's,
  // from ~/.config/graft/keys or a variable such as OLLAMA_API_KEY. The claude
  // in bin is for Preview, which only looks it up.
  const env = { PATH: `${bin}:${process.env.PATH}`, HOME: home, DISPLAY: '', WAYLAND_DISPLAY: '', SSH_CONNECTION: 'graft-ui-smoke' };
  const graft = spawn(graftBin, ['--config', path, 'ui'], { env, cwd: home });
  ```

  Каталоги `home = join(dir, 'home')` и `bin = join(dir, 'bin')` создаются при старте smoke, в `bin` лежит исполняемый `claude` с текстом `#!/bin/sh\nexit 0\n` (права 0755). Комментарий над `startUI` про `SSH_CONNECTION` остаётся.
- В `editExample`, после проверки `the page reads as saved again`, три проверки:
  1. Открыть провайдера `cliproxy`: `.key-state` через ожидание показывает `No key file yet.` — `PASS the key widget says that the key file is missing`. Его ключ `~/.config/graft/keys/cliproxy` лежит под `HOME` smoke, файла там нет.
  2. В том же виджете нет `input[aria-label="Key to save"]`, а `code.key-command` — ровно `mkdir -p ~/.config/graft/keys && (umask 077; cat > ~/.config/graft/keys/cliproxy)`: путь не `keys/cliproxy` рядом с конфигом — `PASS a key file away from the config gets the command, not Save key`.
  3. Открыть профиль `sol`, нажать вкладку `Preview`: `pre.plan` через ожидание содержит `profile sol: direct, provider cliproxy`, `ANTHROPIC_AUTH_TOKEN=<file ~/.config/graft/keys/cliproxy>` и `exec   ` с путём `bin/claude` smoke — `PASS Preview shows what --dry-run prints, with the key as its source`. Затем вернуть вкладку `Edit`.

- [ ] **Шаг 2: Убедиться, что smoke падает**

Запустите: `make ui-smoke`

Ожидается: `FAIL timed out waiting for …` на первой новой проверке: виджета ключа ещё нет, и smoke обрывается на ней; код 1.

- [ ] **Шаг 3: `checks.js` и формы**

`fields.js`: `KeySource` принимает `children` и ставит их последними в своём `<fieldset>`, после `Help`.

`checks.js` — части форм, которые сами спрашивают сервер. Каждый запрос идёт через `page.request`; при `page.stopped` кнопки и поле вставки выключены и запросов нет. Ответ учитывается, только пока у части тот же вход, для которого он запрошен (тот же источник ключа, та же цель Test, тот же черновик и файл Preview): старый ответ может прийти последним, как проверка на этапе 1.

`KeyWidget({ value, onChange, errors = [], provider, keysLink, page, onSaved })` — `KeySource` с `value`, `onChange`, `errors`, а внутри него:

- через 300 мс после последней правки `value`, если `value.kind` задан, а `value.value.trim()` не пуст, — `POST /api/key/check` с `{ kind, value, run: false }`;
- `<p class="key-state">` по ответу: `missing` — `No key file yet.`; `unset` — `<имя> is not set in the environment of graft ui. Export it in the shell that runs graft, then restart graft ui.`; `not run` — `Not run yet.`; `ok` — `Key ok: sha256:<fingerprint>`; `error` — текст ошибки в `<small class="error">`; предупреждение — `<small class="warn">`;
- для `command`: кнопка `Run` (`POST /api/key/check` с `run: true`; на время запроса выключена, `aria-busy`) и `<small>Runs the command now. It may ask for a password in the terminal of graft ui.</small>`;
- для `file`, когда `path` ответа равен `defaultKeyPath(page.file.path, provider)` и `provider` не пуст: `<input type="password" aria-label="Key to save" placeholder="Paste the key" autocomplete="off">` и кнопка `Save key`. Кнопка выключена, пока поле пусто или идёт запрос. `POST /api/key/write` с `{ provider, key, replace: false }`. На 409 — `window.confirm(`${body.error}${body.fingerprint ? ` (sha256:${body.fingerprint})` : ''}. Replace it with the pasted key?`)`, и при согласии тот же запрос с `replace: true`. Удача: поле очищается, под ним `Saved to <path>.`, состояние спрашивается сразу, без 300 мс, и вызывается `onSaved`. Ошибка — её текст под полем;
- для `file`, когда `path` ответа известен: `<small>Or in a terminal:</small> <code class="key-command">${keyCommand(path)}</code>` и кнопка `Copy` (`navigator.clipboard.writeText`; на 2 с подпись `Copied`);
- при `keysLink`: `<small>Get a key at <a href=… target="_blank" rel="noopener noreferrer">…</a></small>`.

Вставленный ключ живёт только в поле ввода, до «Save key»: ни в черновике, ни в другом состоянии страницы его нет.

`TestButton({ target, page })`, где `target` — `{ url, key, id }` или `null`:

- кнопка `Test (spends a few tokens)` (`secondary outline`); выключена при `target === null`, пустом `target.url`, пустом `target.key.kind`, `page.stopped` или запросе в пути;
- `POST /api/test` с `{ url, key, id }` цели;
- `<small class="test-result">`: для 2xx — `<status> in <ms> ms: <text>` или `<status> in <ms> ms: no text in 16 tokens`, класс `ok`; для другого статуса — `<status> in <ms> ms`, класс `error`, и тело в `<pre class="plan">`, если оно есть; на `ApiError` (422, 502) — его текст, класс `error`;
- результат сбрасывается, когда цель меняется (URL, источник ключа или `id`).

`Preview({ draft, profile, page })`:

- при `page.errors > 0` — `<p class="muted">Fix the errors to see the preview.</p>` и никаких запросов;
- иначе через 300 мс после появления и после каждой смены `draft` или `page.file` — `POST /api/preview` с `{ version: page.file.version, draft, profile }`; ответ `stale` не меняет показанного;
- показывает предупреждения (`<p class="warn">`), отказ (`<pre class="plan error">`) и текст (`<pre class="plan">`); до первого ответа — `<p aria-busy="true">Loading…</p>`.

`forms.js`:

- `ProviderForm` ставит `KeyWidget` на место `KeySource`: `provider` — имя записи, `keysLink` — `keys` пресета из `presetFor(page.presets, p.url)` или `''`, `onChange` правит `key` записи.
- `ModelForm` ставит `TestButton` с `target={testTarget(draft, m)}` после поля `Context`.
- `ProfileForm` под шапкой ставит вкладки `<div role="group" class="tabs">` с кнопками `Edit` и `Preview` (`aria-pressed`; невыбранная — `secondary outline`). `Edit` показывает поля, как сейчас; `Preview` — `Preview` с `profile` — именем записи. Вкладка — состояние формы, у новой формы это `Edit`.
- Все три формы получают `page`.

`app.js`:

- пресеты загружаются один раз при старте: `GET /api/presets` → `providers`; неудача молчит, список остаётся `[]`;
- `request` — `call`, который при отсутствии ответа (не `ApiError`) ставит `stopped`, как `trouble`, и пробрасывает ошибку, чтобы часть формы показала её у себя;
- формы получают `page={ file, presets, request, stopped, errors: check.errors.length }`.

`style.css`: `.key-state`, `.warn` (жёлтый, `#b7791f`), `.ok` (`var(--pico-ins-color)`), `.tabs` (ширина по содержимому, отступ снизу), `pre.plan` (моноширинный, `white-space: pre`, `overflow-x: auto`, мелкий шрифт).

- [ ] **Шаг 4: Smoke проходит**

Запустите: `make ui-smoke`

Ожидается: 28 строк `PASS`, ни одной другой, код 0; среди них три новые проверки шага 1 и `PASS the page logged no errors`.

- [ ] **Шаг 5: Полная проверка**

Запустите: `test -z "$(gofmt -l .)" && go vet ./... && GOOS=darwin go vet ./... && go test -race ./... && node --test internal/ui/jstest/draft.test.mjs`

Ожидается: код 0; `TestPageLoadsItsFiles` находит `checks.js`.

- [ ] **Шаг 6: Commit**

```bash
git add internal/ui/static/checks.js internal/ui/static/fields.js internal/ui/static/forms.js internal/ui/static/app.js internal/ui/static/style.css internal/ui/jstest/smoke.mjs
git commit -m "feat(ui): add the key widget, Test and Preview to the forms" -m "The provider form shows what its key source gives and saves a pasted key, the model form tests the model, and a profile previews what it would start. The smoke now runs graft ui with a home of its own and none of the user's environment, since the key widget reads keys."
```

### Task 8: Страница: экран первого запуска, диалог «Add provider», подсказка

Без записей в конфиге страница показывает пресеты; карточка и «+» у провайдеров открывают диалог, который кладёт провайдера, модели и профили в черновик. После сохранения, которое их записало, страница говорит, как запустить новые профили.

**Files:**
- Create: `internal/ui/static/newcomer.js`
- Modify: `internal/ui/static/app.js`, `internal/ui/static/layout.js`, `internal/ui/static/style.css`, `internal/ui/jstest/smoke.mjs`

**Interfaces:**
- Consumes: `KeyWidget`, `TestButton`, проп `page` (задача 7); `freeName`, `newName`, `defaultKeyPath`, `renameProvider`, `addProvider`, `runHint` (задача 6); пресеты (задача 2).
- Produces:
  - `newcomer.js`: `PresetCards({ presets, onPick })`, `Welcome({ path, exists, presets, onPick })`, `AddProvider({ preset, draft, presets, page, onAdd, onClose })`, где `preset` — `undefined` (выбор в диалоге), `null` («Custom…») или пресет;
  - `layout.js`: `Hint({ text, onClose })`; `Empty` удаляется;
  - зацепки для smoke: `article.welcome`, `.cards button.card`, `dialog[open]`, поля `Name` и `URL` (label), поля моделей с `aria-label` `Model name`, `Model ID`, `Context window`, кнопки `+ Model`, `Add to config`, `Cancel`, `.banner.info`.

- [ ] **Шаг 1: Smoke пути новичка**

В `internal/ui/jstest/smoke.mjs`:

- В `editExample`, после проверки `the file reads as saved`: нажать `+` у Providers (`$$('nav.side .sec .add')[0]`) — открывается `dialog[open]` с `.cards`, где есть `Ollama Cloud` и `Custom…`; `Cancel` его закрывает — `PASS the + of providers opens the dialog with the presets`.
- Ненастоящий провайдер в самом smoke: `http.createServer` на `127.0.0.1:0` записывает каждый запрос (метод, путь, заголовки, разобранный JSON тела) и отвечает 200 `{"type":"message","content":[{"type":"text","text":"pong"}]}`.
- `buildFromNothing` заменяется на `newcomer`: конфиг `join(dir, 'new', 'config.yaml')`, файла нет, ключ — `sk-smoke-key`, его отпечаток — первые 12 hex-знаков `createHash('sha256')` от ключа, путь ключа — `join(dir, 'new', 'keys', 'local')`. Проверки по порядку:
  1. `article.welcome` с карточками `Ollama Cloud` и `Custom…` — `PASS a missing config shows the presets`.
  2. Нажать `Custom…`: поле `Name` — `provider-1`; набрать `local` — источник ключа (`input[aria-label="Key source"]`) кончается на `/new/keys/local` — `PASS Custom starts with a free name, and the key file follows the name`.
  3. Набрать в `URL` адрес ненастоящего провайдера; `.key-state` — `No key file yet.`, поле `Key to save` видно — `PASS the key widget says that there is no key file yet`.
  4. Вставить ключ в `Key to save`, нажать `Save key`; файл — ровно `sk-smoke-key\n` с правами 0600 — `PASS Save key writes the key, readable only by its owner`.
  5. Вывод `graft ui` содержит `graft ui: wrote <путь ключа> (sha256:<отпечаток>)` и не содержит ключа — `PASS graft ui prints the key file with its fingerprint, never the key`.
  6. `.key-state` — `Key ok: sha256:<отпечаток>` — `PASS the widget shows the fingerprint of the saved key`.
  7. Нажать `+ Model`, набрать `fake`, `fake-1` и `200000` в `Model name`, `Model ID` и `Context window`, нажать `Test` этой строки. Провайдер получил `POST /v1/messages` с `authorization: Bearer sk-smoke-key`, `anthropic-version: 2023-06-01`, `model` `fake-1` и `max_tokens` 16 — `PASS Test sends the key and the model to the provider`.
  8. `.test-result` строки — `200 in <число> ms: pong` — `PASS Test shows the answer`.
  9. Нажать `Add to config`: диалог закрыт, в меню провайдер `local`, модель `fake` и профиль `fake` — `PASS Add to config fills the menu`.
  10. Нажать Save, когда она включится. Файл содержит провайдера `local` с `url: http://127.0.0.1:<порт>` и `key: {file: <…>/new/keys/local}`, модель `fake` с `provider: local`, `id: fake-1`, `context: 200000` и профиль `fake` с `model: fake` — `PASS Save creates the config`.
  11. Вывод содержит `graft ui: created ` — `PASS graft ui says it created the file`.
  12. `.banner.info` содержит `Run graft fake in a terminal.` — `PASS the page says how to start the new profile`.
  13. Открыта форма профиля `fake` (первого нового); вкладка `Preview`: `pre.plan` содержит `profile fake: direct, provider local` и `<file <путь ключа>>` — `PASS Preview of the new profile shows its key file`.
  14. Ни текст страницы, ни значения её полей ввода, ни файл конфига не содержат `sk-smoke-key` — `PASS the key reaches neither the page nor the config`.

- [ ] **Шаг 2: Убедиться, что smoke падает**

Запустите: `make ui-smoke`

Ожидается: FAIL уже на проверке `+` у провайдеров (диалога нет), код 1.

- [ ] **Шаг 3: `newcomer.js`, `layout.js`, `app.js`**

`PresetCards({ presets, onPick })` — `<div class="cards">`: на каждый пресет `<button type="button" class="card secondary outline">` с `<strong>title</strong>` и `<small>` — хостом его URL (`new URL(url).host`), затем такая же карточка `Custom…` с `<small>any Anthropic Messages API</small>`; щелчок — `onPick(пресет)` или `onPick(null)`.

`Welcome({ path, exists, presets, onPick })` — `<article class="welcome">`: `<h2>Welcome to graft</h2>`, `<p>graft starts Claude Code on the models of other providers. Choose a provider to begin:</p>`, `PresetCards`, `<p class="muted">` — `<path> has no providers, models or profiles yet.` при `exists`, иначе `graft will create <path> when you save.`

`AddProvider({ preset, draft, presets, page, onAdd, onClose })` — `<dialog>`, открытый `showModal()` при появлении; событие `close` (Esc, `Cancel`) вызывает `onClose`. Внутри `<article>`:

- `preset === undefined`: `<h3>Add a provider</h3>`, `PresetCards`, выбор которых переводит диалог к форме, и `Cancel`.
- форма: заголовок `Add <title>` или `Add a provider`; при `preset.docs` — `<p class="muted"><a …>How <title> works with Claude Code</a></p>`.
- `Name` — `Field`; начальное — `freeName(draft.providers, preset.name)` у пресета, `newName(draft.providers, 'provider')` у «Custom…»; занятое имя — ошибка у поля `another provider has this name`; подсказка — `lowercase letters, digits, '-' and '_'`.
- `URL` — `Field`, начальное — `url` пресета или `''`.
- `KeyWidget` с `provider` — именем, а пока имя занято — `''`: тогда «Save key» не показывается, файл `keys/<имя>` принадлежит другому провайдеру. `keysLink` — `keys` пресета; начальный источник — `{ kind: 'file', value: defaultKeyPath(page.file.path, имя) }`. Пока ключ не сохраняли (`onSaved` ещё не звали), правка имени ведёт за собой путь, как `renameProvider` у новой записи (`was: ''`); путь, изменённый руками, за именем не идёт.
- `<fieldset><legend>Models</legend>`: модели пресета — строки с галочкой, именем (тем, под которым модель ляжет в черновик: имена из `addProvider` для текущего выбора), `id`, окном (`toLocaleString('en-US')` и ` tokens`) и `TestButton` с целью `{ url, key, id }`; отмечена первая. Модели, добавленные `+ Model`, — строки из полей `Model name`, `Model ID`, `Context window`, `TestButton` с `id` из поля ID или, если оно пусто, из имени, и кнопки `×`. При `preset.catalog` рядом с `+ Model` — `<small>Find the models at <a …>…</a></small>`.
- `<fieldset><legend>Profiles</legend>`: галочка `One profile per model` (включена) и галочка `mix: Opus on your Claude subscription, with these models in /model` (выключена и недоступна, пока не отмечено и не добавлено ни одной модели).
- `Add to config` выключена, пока имя или URL пусты, имя занято или у модели, добавленной вручную, нет имени. Она вызывает `onAdd(addProvider(draft, choice))`, где `choice.models` — отмеченные модели пресета в их порядке, затем добавленные вручную (`label: ''`).

`layout.js`: `Empty` удаляется, вместо него:

```js
// Hint tells how to start the profiles that a save has just written.
export function Hint({ text, onClose })
```

`Hint` без текста — ничего; иначе `<div class="banner info" role="status">` с текстом и кнопкой `×` (`aria-label="Close"`), которая вызывает `onClose`.

`app.js`:

- состояния: `dialog` (`null` или `{ preset }`), `pending` — имена профилей, которые добавил диалог и ещё не записал Save, `hint`;
- при редактируемом файле без записей — `Welcome` вместо `Empty`: `onPick` открывает диалог с выбранным пресетом;
- `+` у Providers открывает диалог с `preset: undefined`; `+` у моделей и профилей — как на этапе 1;
- `onAdd`: черновик — из ответа `addProvider`, выбрана форма первого нового профиля или, без профилей, нового провайдера, `pending` — новые профили, диалог закрыт;
- в `save`, после удачи: если `pending` не пуст и все его профили есть в записанном черновике, `hint` — `runHint(pending)`, а `pending` очищается;
- `Hint` стоит под `Banner`, крестик очищает `hint`.

`style.css`: `.cards` (сетка карточек, `repeat(auto-fill, minmax(12rem, 1fr))`), `.card` (колонка, текст слева, без внешнего отступа), `dialog article` (ширина до `48rem`), `.banner.info` (голубой фон, как у `.banner.warn` жёлтый).

- [ ] **Шаг 4: Smoke проходит**

Запустите: `make ui-smoke`

Ожидается: 38 строк `PASS`, ни одной другой, код 0. Из `buildFromNothing` ушли 5 проверок, пришли 14 проверок `newcomer` и одна в `editExample`.

- [ ] **Шаг 5: Полная проверка**

Запустите: `test -z "$(gofmt -l .)" && go vet ./... && GOOS=darwin go vet ./... && go test -race ./... && node --test internal/ui/jstest/draft.test.mjs`

Ожидается: код 0.

- [ ] **Шаг 6: Commit**

```bash
git add internal/ui/static/newcomer.js internal/ui/static/app.js internal/ui/static/layout.js internal/ui/static/style.css internal/ui/jstest/smoke.mjs
git commit -m "feat(ui): add the first-run screen and the dialog Add provider" -m "A config with no entries opens on the presets. A preset, Custom or the + at Providers opens a dialog that adds the provider with its key, the models that are checked or typed in and their profiles to the draft; the usual Save writes them, and the page then names the commands that start the new profiles."
```

### Task 9: Модели пресетов

Список моделей меняется быстрее всего остального, поэтому дизайн собирает его в конце, ближе к релизу, и только по официальной документации провайдеров. Таблица «С чего начать сбор» в дизайне — отправная точка от 2026-10-15, не источник.

**Files:**
- Modify: `internal/ui/presets.json`

**Interfaces:**
- Consumes: `TestPresets` (задача 2), который проверяет каждый пресет через `config.Parse`.
- Produces: модели пресетов; при необходимости — новые адреса, страницы ключей, руководства и каталоги, если официальная документация их сменила.

- [ ] **Шаг 1: Собрать модели по официальной документации**

Для каждого провайдера `presets.json`, кроме OpenRouter и CLIProxyAPI, пройдите его официальную документацию на дату задачи: руководство для Claude Code (`docs`), список или страницы моделей, страницу цен, если окна там. Правила:

- источник — только официальная документация провайдера: его сайт документации, страницы моделей и цен. Сторонние руководства могут подсказать, где искать, но источником не бывают;
- модель попадает в пресет, только если документация называет её окно контекста. Берётся точное число токенов, если оно названо; для округлённого (`256K`, `1M`) — меньшее прочтение: 256000, 1000000. Окно, взятое больше настоящего, даёт Claude Code сжимать историю слишком поздно, и провайдер отвергает запрос;
- `id` — как в документации, без суффикса `[1m]`: graft сам задаёт окно через `CLAUDE_CODE_MAX_CONTEXT_TOKENS`, а маршрутизатор отправил бы суффикс провайдеру как часть ID;
- `name` — `id`, если его принимает `config`: буквы, цифры, `.`, `_`, `-`, начало с буквы или цифры, не начинается с `claude` и не `opus`, `sonnet`, `haiku`, `fable`, `default`, `best`, `opusplan`, `inherit`. Иначе каждый другой символ `id` заменяется на `-` (`gemma4:31b` → `gemma4-31b`);
- `label` — название модели, как его пишет документация (`Kimi K3`);
- не больше четырёх моделей на провайдера. Первой идёт та, что рекомендует руководство для Claude Code: диалог отмечает первую модель. Дальше — другие модели из руководства, потом флагманы из списка моделей. Модели, которые документация называет устаревшими или с датой отключения, не берутся: например, `mimo-v2.5*`, которые перестают работать 2026-10-21;
- у провайдера, чья документация не называет окно ни одной модели, моделей нет. Тогда в `catalog` ставится официальная страница со списком моделей, если она есть, — диалог покажет её рядом с «+ Model»;
- адрес Anthropic API, страница ключей и руководство берутся из официальной документации. Если какой-то из них сменился с 2026-10-15, новый идёт в `presets.json`, а отчёт называет замену.

В отчёте задачи — таблица на каждого провайдера: `name`, `id`, окно, `label`, ссылка на страницу, которая называет окно, и её слова об окне; провайдеры без моделей и почему; сменившиеся ссылки. Дата проверки — дата задачи.

- [ ] **Шаг 2: Записать модели в `internal/ui/presets.json`**

Модели — в поле `models` своего провайдера, в порядке шага 1, с `context` числом. Порядок провайдеров не меняется.

- [ ] **Шаг 3: Проверка**

Запустите: `go test ./internal/ui/ -run TestPresets -v`, затем `test -z "$(gofmt -l .)" && go vet ./... && go test -race ./... && node --test internal/ui/jstest/draft.test.mjs && make ui-smoke`

Ожидается: `TestPresets` и его восемь подтестов проходят; полная проверка — код 0, smoke — 38 строк `PASS`.

- [ ] **Шаг 4: Commit**

```bash
git add internal/ui/presets.json
git commit -m "feat(ui): add the models of the presets" -m "Each preset lists up to four models whose context windows the provider's own documentation names, the one its guide for Claude Code recommends first. Model lists change faster than anything else in the presets, so they are collected last, closer to the release; Test catches an ID that has gone since."
```

Тело коммита дополняется датой проверки и провайдерами без моделей, если такие есть.

### Task 10: README

**Files:**
- Modify: `README.md` (Quick start, фраза о том, кто пишет файлы, Web UI, Security, Limitations)

**Interfaces:**
- Consumes: поведение задач 1–9; список провайдеров `presets.json` после задачи 9.
- Produces: документация пути новичка.

- [ ] **Шаг 1: Правки README**

В `README.md` замените

```markdown
## Quick start

1. Put your provider's key where graft can read it, for example in a file that only you can read:
```

на

```markdown
## Quick start

The quickest start is `graft ui`: in the page it opens, choose your provider, paste its key, test a model and save, then start the profile that the page names, such as `graft glm`. See [Web UI](#web-ui).

To set it up by hand:

1. Put your provider's key where graft can read it, for example in a file that only you can read:
```

В `README.md` замените

```markdown
graft never writes to Claude Code's files or to your shell files, and only `graft ui` writes its config, when you save.
```

на

```markdown
graft never writes to Claude Code's files or to your shell files. Only `graft ui` writes its config, when you save, and a key file, when you save a key.
```

В `README.md` замените

```markdown
On a machine you reach over SSH, forward the port that graft prints, with the same number on both ends: `ssh -L 43117:127.0.0.1:43117 HOST`.

- Each change is checked by the same rules as the config part of `graft check`, and each problem shows at its field. A config with problems cannot be saved. The page neither creates key files nor reads them: write the key file yourself before you start a profile, as in the Quick start, and `graft check` tells whether graft can read it.
```

на

```markdown
On a machine you reach over SSH, forward the port that graft prints, with the same number on both ends: `ssh -L 43117:127.0.0.1:43117 HOST`.

A config without providers, models or profiles, and no config at all, opens on presets: PRESETS, and Custom for any other provider. A preset fills in the provider's address, where to get a key and the models whose context windows its documentation names; OpenRouter and CLIProxyAPI list no models, since those depend on your account. The dialog adds the provider, the models you check or type in, a profile per model and, if you ask, `mix`: Opus on your Claude subscription, with these models in `/model`. The + at Providers opens the same dialog. Nothing reaches the config before you save; then the page names the commands that start the new profiles.

- Each change is checked by the same rules as the config part of `graft check`, and each problem shows at its field. A config with problems cannot be saved.
- A key shows as its state and the first 12 hex digits of its SHA-256, never as the key. For a key file, the page tells whether it exists and whether other users can read it. A file at `keys/PROVIDER` next to the config takes a pasted key: Save key writes it there at once, readable only by you, so that Test can read it before you save the config. For a file elsewhere, the page shows the command that writes it in a terminal. For `env`, it tells whether graft ui's environment has the variable. A key command runs only when you press Run, since it may ask for a password in the terminal of graft ui, which then keeps the key in memory until it stops.
- Test sends a model one short request, as Claude Code would send it, and shows the status and the answer. It works before you save, and it spends a few tokens.
- Preview shows what `graft --dry-run PROFILE` would print, with each key as its source, unread. It checks the Claude Code settings files that a profile started in the directory of graft ui would meet; started elsewhere, a profile may meet others.
```

где `PRESETS` — названия пресетов `presets.json` через запятую в их порядке, с `and` перед последним, кроме «Custom…».

В `README.md` замените

```markdown
- Keys never appear on the command line, in `--settings`, in the log or in graft's output: `--dry-run` and `check` show the first 12 hex digits of a key's SHA-256.
```

на

```markdown
- Keys never appear on the command line, in `--settings`, in the log, in graft's output or in the page of `graft ui`: `--dry-run`, `check` and the page show the first 12 hex digits of a key's SHA-256. Where a provider repeats the key in its answer to Test, the page shows `<key>`.
```

В `README.md` замените

```markdown
Keep the address to yourself: a key source may be a command, so whoever writes the config runs commands as you.
```

на

```markdown
Keep the address to yourself: a key source may be a command, so whoever writes the config runs commands as you. Save key writes only `keys/PROVIDER` next to the config. A key command runs only when you press Run, and graft ui keeps the key it printed in memory until it stops.
```

В `README.md` замените

```markdown
- `graft ui` refuses connections from other users' processes on Linux only: see [Security](#security).
```

на

```markdown
- `graft ui` refuses connections from other users' processes on Linux only: see [Security](#security).
- The presets of `graft ui` go out of date as providers change their models; Test tells when a model ID no longer works.
```

- [ ] **Шаг 2: Проверка**

Запустите: `grep -c "(#web-ui)" README.md && test -z "$(gofmt -l .)" && go vet ./... && go test -race ./...`

Ожидается: `3` (ссылки из Quick start, таблицы команд и Limitations), затем код 0: `TestReadmeExample` по-прежнему разбирает пример конфига из Quick start.

- [ ] **Шаг 3: Commit**

```bash
git add README.md
git commit -m "docs: describe the newcomer's path of graft ui" -m "Quick start begins with graft ui, and Web UI says what the presets, the key widget, Test and Preview do, what Save key writes and where keys may show."
```

### Task 11: Приёмка этапа 2 (с владельцем)

Приёмка — в браузере, вместе с владельцем; дизайн, раздел «Приёмка», этап 2. Ничего не коммитится. Запросы тратят квоту провайдера, поэтому настоящий ключ и `graft <профиль>` — только с разрешения владельца. Ключ вводит сам владелец: контроллер не видит ключей, не читает файлов ключей и не пересказывает отпечатков.

- [ ] **Шаг 1: Сборка и автоматические проверки.** `make build test test-js ui-smoke` и `test -z "$(gofmt -l .)" && go vet ./... && GOOS=darwin go vet ./...` — всё зелёное, smoke — 38 строк `PASS`.
- [ ] **Шаг 2: Разрешение.** Спросить владельца, какой пресет и какой ключ берутся и можно ли тратить квоту на Test и на сеанс `graft <профиль>`.
- [ ] **Шаг 3: Чистый `HOME`.** `HOME=$(mktemp -d)` для `bin/graft ui` в фоне, с `DISPLAY= WAYLAND_DISPLAY=`; владелец открывает напечатанный адрес. Путь из сценария 1, не открывая README: экран первого запуска → пресет → ключ (владелец вставляет его и нажимает «Save key» или пишет файл командой из виджета) → Test → модели → профили → «Add to config» → Save → подсказка «Run `graft <профиль>` in a terminal».
- [ ] **Шаг 4: Сеанс.** Владелец с тем же `HOME` запускает `bin/graft <профиль>`; модель отвечает в Claude Code.
- [ ] **Шаг 5: Ключ нигде не виден.** В конфиге источник ключа, а не ключ; в терминале `graft ui` о ключе — только строка `wrote` с отпечатком; в ответах API на странице (DevTools, вкладка Network) — состояние и отпечаток. Preview профиля совпадает с `HOME=… bin/graft --dry-run <профиль>`, кроме вида ключа: источник вместо источника с отпечатком.
- [ ] **Шаг 6: Решение.** Если владелец принимает этап, дальше `superpowers:finishing-a-development-branch`; перед PR `git rm -r docs/superpowers/` и отдельный коммит. Если нет — замечания становятся задачами этого плана.
