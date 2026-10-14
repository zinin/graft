# graft ui, этап 1 (редактор): план реализации

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** `graft ui` открывает в браузере редактор конфига graft: формы провайдеров, моделей, профилей и defaults, проверку на лету тем же `config.Parse`, что и CLI, сохранение с комментариями и защиту сервера от чужих пользователей и страниц.

**Architecture:** Новый пакет `internal/edit` переводит YAML в черновик — JSON для страницы — и обратно: правки черновика вливаются в дерево `yaml.Node` поле за полем, остальное остаётся как было, а файл рендерит `yaml.v3`. Пакет `internal/ui` — HTTP-сервер на `127.0.0.1` с токеном, API (`config`, `version`, `validate`, `save`), запись файла с `.bak` и встроенная статика. Страница — модули ES на Preact + htm и Pico.css без сборки; логика страницы, которую стоит тестировать, живёт в `draft.js` и проверяется `node --test`.

**Tech Stack:** Go 1.26, `go.yaml.in/yaml/v3` (единственная зависимость), `net/http`, `embed`; htm 3.1.1 (бандл с Preact), Pico CSS 2.1.1; Node для тестов `draft.js`; headless Chrome для проверки в браузере (`make ui-smoke`).

**Spec:** `docs/superpowers/specs/2026-10-15-graft-ui-design.md`. План реализует этап 1: разделы «Архитектура», «Черновик», «Проверка, сохранение и ручные правки», «Сервер и защита», «Интерфейс этапа 1», «Фронтенд», «Вывод в терминал», «Тестирование» и «Документация» в части этапа 1. Этап 2 (пресеты, ключи, тестовый запрос, Preview) получит свой план после приёмки этого.

**Код проверен.** Весь код плана прогнан 2026-10-15 задача за задачей на чистой копии репозитория: каждый шаг «тест падает» падал, каждый шаг «тесты проходят» проходил, `make ui-smoke` в headless Chrome дал 22 из 22 пять раз подряд. Переносите код как есть; если что-то расходится с описанием — остановитесь и сообщите, а не подгоняйте.

**Перед задачей 1:** ветка `feat/web-ui`, рабочее дерево чистое, `go test -race ./...` зелёный.

## Global Constraints

- Модуль `github.com/zinin/graft`, Go 1.26. Новых зависимостей Go нет: только `go.yaml.in/yaml/v3`.
- graft работает на Linux и macOS; Windows вне рамок.
- Тексты интерфейса, вывод graft, комментарии в коде и сообщения коммитов — на английском. Коммиты без трейлеров атрибуции.
- Репозиторий публичный: в файлах и коммитах нет личных имён хостов, путей, ключей, отпечатков ключей и имён моделей из конфига владельца.
- Сервер слушает только `127.0.0.1`, порт случайный. Токен — 32 байта из `crypto/rand` в hex; адрес — `http://127.0.0.1:<порт>/#token=<токен>`.
- CSP: `default-src 'none'; script-src 'self'; style-src 'self'; img-src 'self' data:; connect-src 'self'; base-uri 'none'; form-action 'none'; frame-ancestors 'none'`. Никакого inline-кода, никаких CDN.
- Тело запроса — не больше 1 МиБ (`1 << 20`), тайм-аут заголовков — 10 с.
- Новый конфиг создаётся с правами 0600, новый каталог — с 0700; старый файл сохраняет права; прежняя версия — `<файл>.bak` с теми же правами.
- Вендоренные файлы: `htm@3.1.1/preact/standalone.module.js` (sha256 `72284e8e9079c87817145df1110f74e8a2aa040b2fc384922e18dfcb46fc1fd7`) и `@picocss/pico@2.1.1/css/pico.min.css` (sha256 `fbc9a63fc9fc9f72d12fd7fc9806e11fa9f77ae4f9cad146b27003a1119ba3db`).
- Проверки после каждой задачи: `test -z "$(gofmt -l .)"` (как в CI), `go vet ./...`, `go test -race ./...`; с задачи 7 — ещё `node --test internal/ui/jstest/draft.test.mjs`.
- Стиль кода — как в graft: комментарии полными предложениями объясняют зачем; имена и идиомы — как в соседнем коде.

## Review Focus

1. Комментарии и значения на любом языке (кириллица, эмодзи) и файл с концами строк CRLF: сохранение не теряет текст и пишет валидный YAML с LF. — `TestApplyUnicodeAndCRLF`, задача 2.
2. JSON из документации Claude Code или из редактора, с отступами табуляцией, вставленный в поле settings, принимается как есть. — `TestApplyJSONSettings`, задача 2.
3. Имена и значения, которые YAML прочёл бы иначе (`true`, `1.0`, `0123`, `null`, пустая строка в env), остаются строками. — `TestApplySpecialStrings`, задача 2.
4. Две вкладки на одной версии файла: вторая при сохранении получает 409, а не затирает правки первой. — `TestTwoTabsSave`, задача 4.
5. Имя модели с точкой (`glm-5.3`) и промежуточное имя, совпавшее при наборе с чужим: ошибки попадают к своей записи, а ссылки не перетягиваются. — тесты `locate` и `renameRefs` в `draft.test.mjs`, задача 7.

---

### Task 1: Черновик конфига — `edit.Read`

**Files:**
- Modify: `internal/config/config.go` (списки полей), `internal/config/parse.go` (парсер берёт их)
- Create: `internal/edit/draft.go`, `internal/edit/read.go`, `internal/edit/render.go`
- Test: `internal/edit/read_test.go`

**Interfaces:**
- Consumes: `config.AliasNames`; парсер `config` без изменений поведения.
- Produces:
  - `config.Sections`, `config.ProviderFields`, `config.KeyFields`, `config.ModelFields`, `config.DefaultsFields`, `config.ProfileFields []string` — поля в порядке README.
  - `edit.Draft{Providers []Provider; Models []Model; Defaults Defaults; Profiles []Profile}`; `edit.Provider{Was, Name, URL string; Key Key}`; `edit.Key{Kind, Value string}`; `edit.Model{Was, Name, Provider, ID, Context, Label, Description, Settings string}`; `edit.Defaults{Effort, Settings string; Env []Pair; Args []string}`; `edit.Profile{Was, Name, Model, Subscription string; Picker []string; Aliases map[string]string; Subagents, Effort, Settings string; Env []Pair; Args []string}`; `edit.Pair [2]string`. JSON-имена полей — в тегах `draft.go`; на них опирается страница.
  - `func (d *Draft) Normalize()`, `func edit.Same(a, b Draft) bool`.
  - `func edit.Read(data []byte) (Draft, error)`; ошибка формы — `*edit.ShapeError{Problems []Problem}`, `edit.Problem{Line int; Path, Msg string}`.
  - Неэкспортируемые, для задачи 2: `parse`, `decode`, `deref`, `isNull`, `join`, `encode`, `snippet`.

- [ ] **Шаг 1: Вынести списки полей конфига**

Схема полей должна жить в одном месте: и парсер, и `edit` берут её из `config`.

В `internal/config/config.go` замените

```go
// Efforts are the values of effort.
var Efforts = []string{"low", "medium", "high", "xhigh", "max"}
```

на

```go
// Efforts are the values of effort.
var Efforts = []string{"low", "medium", "high", "xhigh", "max"}

// The fields of each part of the config file, in the order README lists
// them. Package edit puts a new field in its place by this order.
var (
	Sections       = []string{"providers", "models", "defaults", "profiles"}
	ProviderFields = []string{"url", "key"}
	KeyFields      = []string{"env", "file", "command"}
	ModelFields    = []string{"provider", "id", "context", "label", "description", "settings"}
	DefaultsFields = []string{"effort", "settings", "env", "args"}
	ProfileFields  = []string{"model", "subscription", "picker", "aliases", "subagents", "effort", "settings", "env", "args"}
)
```

В `internal/config/parse.go` замените

```go
top := p.fields(root, "", "providers", "models", "defaults", "profiles")
```

на

```go
top := p.fields(root, "", Sections...)
```

В `internal/config/parse.go` замените

```go
cfg.Defaults = p.layer(p.fields(top["defaults"], "defaults", "effort", "settings", "env", "args"), "defaults")
```

на

```go
cfg.Defaults = p.layer(p.fields(top["defaults"], "defaults", DefaultsFields...), "defaults")
```

В `internal/config/parse.go` замените

```go
f := p.fields(e.value, path, "url", "key")
```

на

```go
f := p.fields(e.value, path, ProviderFields...)
```

В `internal/config/parse.go` замените

```go
f := p.fields(n, path, "env", "file", "command")
```

на

```go
f := p.fields(n, path, KeyFields...)
```

В `internal/config/parse.go` замените

```go
f := p.fields(e.value, path, "provider", "id", "context", "label", "description", "settings")
```

на

```go
f := p.fields(e.value, path, ModelFields...)
```

В `internal/config/parse.go` замените

```go
	f := p.fields(e.value, path, "model", "subscription", "picker", "aliases", "subagents",
		"effort", "settings", "env", "args")
```

на

```go
	f := p.fields(e.value, path, ProfileFields...)
```

Запустите: `go test ./internal/config/`

Ожидается: `ok` — поведение парсера не изменилось.

- [ ] **Шаг 2: Написать падающие тесты чтения**

`readExample` читает `config.example.yaml` из корня репозитория; задача 2 тоже им пользуется.

`internal/edit/read_test.go`:

```go
package edit

import (
	"errors"
	"os"
	"reflect"
	"strings"
	"testing"
)

func readExample(t *testing.T) []byte {
	t.Helper()
	data, err := os.ReadFile("../../config.example.yaml")
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func TestReadExample(t *testing.T) {
	d, err := Read(readExample(t))
	if err != nil {
		t.Fatal(err)
	}
	if len(d.Providers) != 3 || len(d.Models) != 5 || len(d.Profiles) != 5 {
		t.Fatalf("counts %d %d %d", len(d.Providers), len(d.Models), len(d.Profiles))
	}
	if got, want := d.Providers[0], (Provider{Was: "cliproxy", Name: "cliproxy", URL: "http://127.0.0.1:8317",
		Key: Key{Kind: "file", Value: "~/.config/graft/keys/cliproxy"}}); got != want {
		t.Errorf("providers[0] = %+v, want %+v", got, want)
	}
	if got := d.Providers[2].Key; got != (Key{Kind: "command", Value: "pass show mimo/api-key"}) {
		t.Errorf("mimo key = %+v", got)
	}
	kimi := d.Models[3]
	if kimi.Name != "kimi" || kimi.ID != "kimi-k3" || kimi.Context != "1048576" || kimi.Label != "" ||
		!strings.Contains(kimi.Settings, "maxEffortLevel: high") {
		t.Errorf("kimi = %+v", kimi)
	}
	if want := (Defaults{Effort: "max", Env: []Pair{}, Args: []string{"--allow-dangerously-skip-permissions"}}); !reflect.DeepEqual(d.Defaults, want) {
		t.Errorf("defaults = %+v", d.Defaults)
	}
	mix := d.Profiles[2]
	if mix.Model != "opus" || mix.Subagents != "glm" || !reflect.DeepEqual(mix.Picker, []string{"sol", "glm", "kimi"}) ||
		mix.Subscription != "" || mix.Aliases["haiku"] != "" || len(mix.Aliases) != 4 {
		t.Errorf("mix = %+v", mix)
	}
	if multi := d.Profiles[4]; multi.Aliases["haiku"] != "glm-proxy" {
		t.Errorf("multi aliases = %v", multi.Aliases)
	}
}

func TestReadEmpty(t *testing.T) {
	for _, data := range []string{"", "# only a comment\n", "~\n"} {
		d, err := Read([]byte(data))
		if err != nil {
			t.Fatalf("Read(%q): %v", data, err)
		}
		if d.Providers == nil || d.Models == nil || d.Profiles == nil || d.Defaults.Env == nil || d.Defaults.Args == nil {
			t.Errorf("Read(%q) has nil lists: %+v", data, d)
		}
	}
}

func TestReadKeepsKeysOutOfTheDraft(t *testing.T) {
	d, err := Read([]byte("providers:\n  p:\n    url: https://x\n    key: sk-secret\n"))
	if err != nil {
		t.Fatal(err)
	}
	if d.Providers[0].Key != (Key{}) {
		t.Errorf("key = %+v, want none", d.Providers[0].Key)
	}
}

func TestReadTakesTheFirstKeySource(t *testing.T) {
	d, err := Read([]byte("providers:\n  p:\n    key: { command: c, env: E }\n"))
	if err != nil {
		t.Fatal(err)
	}
	if got := d.Providers[0].Key; got != (Key{Kind: "env", Value: "E"}) {
		t.Errorf("key = %+v", got)
	}
}

func TestReadFollowsAliasesAndNulls(t *testing.T) {
	d, err := Read([]byte(`
models:
  a: &m { provider: p, context: 100000 }
  b: *m
  c:
profiles:
  x:
    model: a
    env: { FOO: "", BAR: "1" }
`))
	if err != nil {
		t.Fatal(err)
	}
	if d.Models[1].Provider != "p" || d.Models[1].Context != "100000" || d.Models[2].Name != "c" {
		t.Errorf("models = %+v", d.Models)
	}
	if want := []Pair{{"FOO", ""}, {"BAR", "1"}}; !reflect.DeepEqual(d.Profiles[0].Env, want) {
		t.Errorf("env = %v, want %v", d.Profiles[0].Env, want)
	}
}

func TestReadIgnoresUnknownFields(t *testing.T) {
	d, err := Read([]byte("models:\n  m: { provider: p, contxt: 5 }\nextra: 1\n"))
	if err != nil {
		t.Fatal(err)
	}
	if d.Models[0].Context != "" {
		t.Errorf("model = %+v", d.Models[0])
	}
}

func TestReadShapeErrors(t *testing.T) {
	tests := []struct{ data, want string }{
		{"providers: [a]\n", "line 1: providers: must be a mapping"},
		{"models:\n  m: text\n", "line 2: models.m: must be a mapping"},
		{"profiles:\n  p: { picker: glm }\n", "line 2: profiles.p.picker: must be a list"},
		{"profiles:\n  p: { picker: [a, {b: c}] }\n", "line 2: profiles.p.picker[1]: must be a single value"},
		{"profiles:\n  p: { model: [a] }\n", "line 2: profiles.p.model: must be a single value"},
		{"profiles:\n  p: { env: { A: [1] } }\n", "line 2: profiles.p.env.A: must be a single value"},
		{"profiles:\n  p: {}\n  p: {}\n", "line 3: profiles.p: duplicate key"},
		{"a: 1\n---\nb: 2\n", "line 2: must be one YAML document"},
		{"providers: [\n", "yaml: line"},
		{"- a\n", "line 1: must be a mapping"},
	}
	for _, tt := range tests {
		_, err := Read([]byte(tt.data))
		var se *ShapeError
		if !errors.As(err, &se) {
			t.Errorf("Read(%q) = %v, want a ShapeError", tt.data, err)
			continue
		}
		if !strings.Contains(err.Error(), tt.want) {
			t.Errorf("Read(%q) = %q, want %q", tt.data, err, tt.want)
		}
	}
}

func TestSame(t *testing.T) {
	a := Draft{Profiles: []Profile{{Was: "x", Name: "x", Settings: "a: 1\nb: [2]\n"}}}
	b := Draft{Profiles: []Profile{{Name: "x", Settings: "{b: [2], a: 1}"}}}
	if !Same(a, b) {
		t.Error("Was or the form of settings counted")
	}
	b.Profiles[0].Settings = "a: 2\n"
	if Same(a, b) {
		t.Error("different settings counted as the same")
	}
	b.Profiles[0].Settings = a.Profiles[0].Settings
	b.Profiles[0].Picker = []string{"glm"}
	if Same(a, b) {
		t.Error("different pickers counted as the same")
	}
}
```

- [ ] **Шаг 3: Убедиться, что тесты падают**

Запустите: `go test ./internal/edit/`

Ожидается: FAIL: пакет не собирается, `undefined: Read`.

- [ ] **Шаг 4: Типы черновика**

Черновик хранит значения так, как они записаны в файле: пустая строка — поля нет. `Same` сравнивает черновики без `Was` и с `settings` по значению: это нужно проверке сохранения в задаче 4.

`internal/edit/draft.go`:

```go
// Package edit turns a config file into the draft that graft ui edits, and
// applies a changed draft back to the file. What the draft does not change
// keeps its comments, style and anchors, and fields the draft does not know
// stay as they are. It does no I/O.
package edit

import (
	"encoding/json"
	"reflect"

	"go.yaml.in/yaml/v3"

	"github.com/zinin/graft/internal/config"
)

// Draft is a config as its file writes it: values are strings as written,
// and "" is a field the file does not set.
type Draft struct {
	Providers []Provider `json:"providers"`
	Models    []Model    `json:"models"`
	Defaults  Defaults   `json:"defaults"`
	Profiles  []Profile  `json:"profiles"`
}

// Provider is an entry of providers. Was is its name in the file, "" for an
// entry the file does not have yet; so are the Was of models and profiles.
type Provider struct {
	Was  string `json:"was"`
	Name string `json:"name"`
	URL  string `json:"url"`
	Key  Key    `json:"key"`
}

// Key is a key source. Kind is env, file, command, or "" when the file
// names none; Value is the variable, path or command as written.
type Key struct {
	Kind  string `json:"kind"`
	Value string `json:"value"`
}

// Model is an entry of models. Settings is YAML text.
type Model struct {
	Was         string `json:"was"`
	Name        string `json:"name"`
	Provider    string `json:"provider"`
	ID          string `json:"id"`
	Context     string `json:"context"`
	Label       string `json:"label"`
	Description string `json:"description"`
	Settings    string `json:"settings"`
}

// Defaults is the defaults section.
type Defaults struct {
	Effort   string   `json:"effort"`
	Settings string   `json:"settings"`
	Env      []Pair   `json:"env"`
	Args     []string `json:"args"`
}

// Profile is an entry of profiles. Aliases holds every name of
// config.AliasNames, "" for an alias the file does not set.
type Profile struct {
	Was          string            `json:"was"`
	Name         string            `json:"name"`
	Model        string            `json:"model"`
	Subscription string            `json:"subscription"`
	Picker       []string          `json:"picker"`
	Aliases      map[string]string `json:"aliases"`
	Subagents    string            `json:"subagents"`
	Effort       string            `json:"effort"`
	Settings     string            `json:"settings"`
	Env          []Pair            `json:"env"`
	Args         []string          `json:"args"`
}

// Pair is a variable of env, [name, value] in JSON. A value of "" is the
// empty string.
type Pair [2]string

// Normalize fills in what JSON may leave out: empty lists for null, and
// every alias name.
func (d *Draft) Normalize() {
	d.Providers = orEmpty(d.Providers)
	d.Models = orEmpty(d.Models)
	d.Profiles = orEmpty(d.Profiles)
	d.Defaults.Env = orEmpty(d.Defaults.Env)
	d.Defaults.Args = orEmpty(d.Defaults.Args)
	for i := range d.Profiles {
		p := &d.Profiles[i]
		p.Picker = orEmpty(p.Picker)
		p.Env = orEmpty(p.Env)
		p.Args = orEmpty(p.Args)
		aliases := map[string]string{}
		for _, name := range config.AliasNames {
			aliases[name] = p.Aliases[name]
		}
		p.Aliases = aliases
	}
}

func orEmpty[T any](s []T) []T {
	if s == nil {
		return []T{}
	}
	return s
}

// Same reports whether a and b hold the same config: Was does not count,
// and settings count by value, not by text.
func Same(a, b Draft) bool {
	return reflect.DeepEqual(a.canonical(), b.canonical())
}

// canonical returns a copy of d without Was and with settings in one form.
func (d Draft) canonical() Draft {
	d.Normalize()
	out := Draft{Defaults: d.Defaults}
	out.Defaults.Settings = canonicalYAML(d.Defaults.Settings)
	for _, p := range d.Providers {
		p.Was = ""
		out.Providers = append(out.Providers, p)
	}
	for _, m := range d.Models {
		m.Was, m.Settings = "", canonicalYAML(m.Settings)
		out.Models = append(out.Models, m)
	}
	for _, p := range d.Profiles {
		p.Was, p.Settings = "", canonicalYAML(p.Settings)
		out.Profiles = append(out.Profiles, p)
	}
	out.Normalize()
	return out
}

// canonicalYAML returns the value of YAML text as JSON, or the text itself
// when it is not YAML that JSON can hold.
func canonicalYAML(text string) string {
	var v any
	if yaml.Unmarshal([]byte(text), &v) != nil {
		return text
	}
	data, err := json.Marshal(v)
	if err != nil {
		return text
	}
	return string(data)
}
```

- [ ] **Шаг 5: Чтение файла**

`Read` отказывает (`*ShapeError`) только тогда, когда черновик не может представить файл; ошибки конфига как такового — дело `config.Parse`. Ключ, вписанный прямо в конфиг (скаляр в `key`), в черновик не попадает.

`internal/edit/read.go`:

```go
package edit

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"slices"
	"strings"

	"go.yaml.in/yaml/v3"

	"github.com/zinin/graft/internal/config"
)

// ShapeError says why the forms cannot show a config file: the file is not
// one YAML document, or a part of it has a shape the draft cannot hold.
type ShapeError struct {
	Problems []Problem
}

// Problem is one reason, at its line of the file when known.
type Problem struct {
	Line int    `json:"line"`
	Path string `json:"path"`
	Msg  string `json:"msg"`
}

func (e *ShapeError) Error() string {
	lines := make([]string, len(e.Problems))
	for i, p := range e.Problems {
		s := ""
		if p.Line > 0 {
			s = fmt.Sprintf("line %d: ", p.Line)
		}
		if p.Path != "" {
			s += p.Path + ": "
		}
		lines[i] = s + p.Msg
	}
	return strings.Join(lines, "\n")
}

// Read returns the draft of the config in data; empty data is an empty
// config. It returns a *ShapeError when the forms cannot show the file.
func Read(data []byte) (Draft, error) {
	doc, err := parse(data)
	if err != nil {
		return Draft{}, err
	}
	r := &reader{}
	d := r.draft(doc.Content[0])
	if len(r.problems) > 0 {
		return Draft{}, &ShapeError{r.problems}
	}
	d.Normalize()
	return d, nil
}

// parse returns the document in data. Its root is a mapping when the file
// is empty or holds only comments.
func parse(data []byte) (*yaml.Node, error) {
	doc, err := decode(data)
	if err != nil {
		return nil, err
	}
	if len(doc.Content) == 0 {
		doc.Content = []*yaml.Node{{Kind: yaml.MappingNode, Tag: "!!map"}}
	} else if root := doc.Content[0]; isNull(root) {
		doc.Content[0] = &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map",
			HeadComment: root.HeadComment, LineComment: root.LineComment, FootComment: root.FootComment}
	}
	return doc, nil
}

// decode returns the one YAML document in data; its Content is empty when
// data holds no value.
func decode(data []byte) (*yaml.Node, error) {
	dec := yaml.NewDecoder(bytes.NewReader(data))
	var doc, next yaml.Node
	if err := dec.Decode(&doc); err != nil && !errors.Is(err, io.EOF) {
		return nil, &ShapeError{[]Problem{{Msg: err.Error()}}}
	}
	switch err := dec.Decode(&next); {
	case err == nil:
		return nil, &ShapeError{[]Problem{{Line: next.Line, Msg: "must be one YAML document"}}}
	case !errors.Is(err, io.EOF):
		return nil, &ShapeError{[]Problem{{Msg: err.Error()}}}
	}
	doc.Kind = yaml.DocumentNode
	return &doc, nil
}

type reader struct {
	problems []Problem
}

func (r *reader) problem(n *yaml.Node, path, format string, args ...any) {
	line := 0
	if n != nil {
		line = n.Line
	}
	r.problems = append(r.problems, Problem{Line: line, Path: path, Msg: fmt.Sprintf(format, args...)})
}

// pair is one key and value of a mapping, the value with aliases followed.
type pair struct {
	key   string
	value *yaml.Node
}

// pairs returns the pairs of mapping n in file order; null is no pairs.
// Keys must be strings and appear once: the draft could not tell two
// apart.
func (r *reader) pairs(n *yaml.Node, path string) []pair {
	n = deref(n)
	if n == nil || isNull(n) {
		return nil
	}
	if n.Kind != yaml.MappingNode {
		r.problem(n, path, "must be a mapping")
		return nil
	}
	var out []pair
	seen := map[string]bool{}
	for i := 0; i+1 < len(n.Content); i += 2 {
		k := deref(n.Content[i])
		switch {
		case k.Kind != yaml.ScalarNode:
			r.problem(k, path, "keys must be strings")
		case seen[k.Value]:
			r.problem(k, join(path, k.Value), "duplicate key")
		default:
			seen[k.Value] = true
			out = append(out, pair{k.Value, deref(n.Content[i+1])})
		}
	}
	return out
}

// fields returns the values of mapping n by key.
func (r *reader) fields(n *yaml.Node, path string) map[string]*yaml.Node {
	out := map[string]*yaml.Node{}
	for _, p := range r.pairs(n, path) {
		out[p.key] = p.value
	}
	return out
}

// scalar returns a value as written; a missing or null value is "".
func (r *reader) scalar(n *yaml.Node, path string) string {
	switch {
	case n == nil || isNull(n):
		return ""
	case n.Kind != yaml.ScalarNode:
		r.problem(n, path, "must be a single value")
		return ""
	}
	return n.Value
}

// list returns the items of sequence n; null is an empty list.
func (r *reader) list(n *yaml.Node, path string) []string {
	if n == nil || isNull(n) {
		return []string{}
	}
	if n.Kind != yaml.SequenceNode {
		r.problem(n, path, "must be a list")
		return []string{}
	}
	out := []string{}
	for i, item := range n.Content {
		out = append(out, r.scalar(deref(item), fmt.Sprintf("%s[%d]", path, i)))
	}
	return out
}

// env returns the variables of mapping n in file order.
func (r *reader) env(n *yaml.Node, path string) []Pair {
	out := []Pair{}
	for _, p := range r.pairs(n, path) {
		out = append(out, Pair{p.key, r.scalar(p.value, join(path, p.key))})
	}
	return out
}

// settings returns the value of n as YAML text; a missing or null value is
// "". Any value can be text, so settings never limit the forms.
func (r *reader) settings(n *yaml.Node, path string) string {
	if n == nil || isNull(n) {
		return ""
	}
	text, err := snippet(n)
	if err != nil {
		r.problem(n, path, "%v", err)
	}
	return text
}

func (r *reader) draft(root *yaml.Node) Draft {
	var d Draft
	f := r.fields(root, "")
	for _, p := range r.pairs(f["providers"], "providers") {
		d.Providers = append(d.Providers, r.provider(p, "providers."+p.key))
	}
	for _, p := range r.pairs(f["models"], "models") {
		d.Models = append(d.Models, r.model(p, "models."+p.key))
	}
	df := r.entry(f["defaults"], "defaults")
	d.Defaults = Defaults{
		Effort:   r.scalar(df["effort"], "defaults.effort"),
		Settings: r.settings(df["settings"], "defaults.settings"),
		Env:      r.env(df["env"], "defaults.env"),
		Args:     r.list(df["args"], "defaults.args"),
	}
	for _, p := range r.pairs(f["profiles"], "profiles") {
		d.Profiles = append(d.Profiles, r.profile(p, "profiles."+p.key))
	}
	return d
}

// entry returns the fields of an entry, which is a mapping or null.
func (r *reader) entry(n *yaml.Node, path string) map[string]*yaml.Node {
	return r.fields(n, path)
}

func (r *reader) provider(p pair, path string) Provider {
	f := r.entry(p.value, path)
	return Provider{
		Was:  p.key,
		Name: p.key,
		URL:  r.scalar(f["url"], path+".url"),
		Key:  r.key(f["key"], path+".key"),
	}
}

// key reads a key source. A key written into the config itself, a scalar,
// is no source: its value must not reach the browser.
func (r *reader) key(n *yaml.Node, path string) Key {
	if n == nil || n.Kind == yaml.ScalarNode {
		return Key{}
	}
	f := r.fields(n, path)
	var k Key
	for _, kind := range config.KeyFields {
		if v, ok := f[kind]; ok {
			value := r.scalar(v, path+"."+kind)
			if k.Kind == "" {
				k = Key{Kind: kind, Value: value}
			}
		}
	}
	return k
}

func (r *reader) model(p pair, path string) Model {
	f := r.entry(p.value, path)
	return Model{
		Was:         p.key,
		Name:        p.key,
		Provider:    r.scalar(f["provider"], path+".provider"),
		ID:          r.scalar(f["id"], path+".id"),
		Context:     r.scalar(f["context"], path+".context"),
		Label:       r.scalar(f["label"], path+".label"),
		Description: r.scalar(f["description"], path+".description"),
		Settings:    r.settings(f["settings"], path+".settings"),
	}
}

func (r *reader) profile(p pair, path string) Profile {
	f := r.entry(p.value, path)
	aliases := map[string]string{}
	for _, a := range r.pairs(f["aliases"], path+".aliases") {
		if slices.Contains(config.AliasNames, a.key) {
			aliases[a.key] = r.scalar(a.value, path+".aliases."+a.key)
		}
	}
	return Profile{
		Was:          p.key,
		Name:         p.key,
		Model:        r.scalar(f["model"], path+".model"),
		Subscription: r.scalar(f["subscription"], path+".subscription"),
		Picker:       r.list(f["picker"], path+".picker"),
		Aliases:      aliases,
		Subagents:    r.scalar(f["subagents"], path+".subagents"),
		Effort:       r.scalar(f["effort"], path+".effort"),
		Settings:     r.settings(f["settings"], path+".settings"),
		Env:          r.env(f["env"], path+".env"),
		Args:         r.list(f["args"], path+".args"),
	}
}

// deref follows YAML aliases.
func deref(n *yaml.Node) *yaml.Node {
	for n != nil && n.Kind == yaml.AliasNode {
		n = n.Alias
	}
	return n
}

func isNull(n *yaml.Node) bool {
	return n.Kind == yaml.ScalarNode && n.ShortTag() == "!!null"
}

func join(path, key string) string {
	if path == "" {
		return key
	}
	return path + "." + key
}
```

Пока в `render.go` только запись узла; задача 2 дополнит его рендером файла. `internal/edit/render.go`:

```go
package edit

import (
	"bytes"

	"go.yaml.in/yaml/v3"
)

// encode writes n as YAML with an indent of 2.
func encode(n *yaml.Node) ([]byte, error) {
	var b bytes.Buffer
	enc := yaml.NewEncoder(&b)
	enc.SetIndent(2)
	if err := enc.Encode(n); err != nil {
		return nil, err
	}
	if err := enc.Close(); err != nil {
		return nil, err
	}
	return b.Bytes(), nil
}

// snippet returns n as YAML text, as a settings field shows it: without
// the anchor that aliases of it use.
func snippet(n *yaml.Node) (string, error) {
	c := *deref(n)
	c.Anchor = ""
	data, err := encode(&c)
	return string(data), err
}
```

- [ ] **Шаг 6: Тесты проходят**

Запустите: `test -z "$(gofmt -l .)" && go vet ./... && go test -race ./...`

Ожидается: команда завершается с кодом 0: `gofmt` не нашёл неотформатированных файлов, `go vet` молчит, все пакеты `ok`.

- [ ] **Шаг 7: Commit**

```bash
git add internal/config/config.go internal/config/parse.go internal/edit
git commit -m "feat(edit): read a config file into a draft the web UI edits"
```

### Task 2: Правки черновика в файле — `edit.Apply`

**Files:**
- Create: `internal/edit/apply.go`
- Modify: `internal/edit/render.go` (рендер документа с пустыми строками между разделами)
- Test: `internal/edit/apply_test.go`

**Interfaces:**
- Consumes: всё из задачи 1, `config.Parse` (в тестах).
- Produces:
  - `func edit.Apply(data []byte, d Draft) ([]byte, error)` — файл с правками от `Read(data)` к `d`. Ошибки черновика — `edit.Errors []Error`, `edit.Error{Path, Msg string}` (JSON `path`, `msg`); файл, который формы не показывают, — `*edit.ShapeError`.
  - Гарантия, на которую опирается задача 4: для черновика без ошибок `Read(Apply(data, d))` даёт то же, что `d`, по `Same`.

- [ ] **Шаг 1: Написать падающие тесты**

`change` — общий помощник: читает файл, меняет черновик, применяет его и проверяет инвариант «черновик → файл → черновик», а при `valid` ещё и `config.Parse`. Последние три теста закрывают Review Focus 1–3.

`internal/edit/apply_test.go`:

```go
package edit

import (
	"errors"
	"slices"
	"strings"
	"testing"

	"github.com/zinin/graft/internal/config"
)

// change reads data, lets edit change the draft, applies it and checks that
// the result reads back as the changed draft and passes config.Parse when
// valid is set. It returns the new file.
func change(t *testing.T, data string, valid bool, edit func(d *Draft)) string {
	t.Helper()
	d, err := Read([]byte(data))
	if err != nil {
		t.Fatal(err)
	}
	edit(&d)
	out, err := Apply([]byte(data), d)
	if err != nil {
		t.Fatalf("Apply: %v", err)
	}
	back, err := Read(out)
	if err != nil {
		t.Fatalf("Read of the result: %v\n%s", err, out)
	}
	if !Same(back, d) {
		t.Errorf("the result reads back as\n%+v\nwant\n%+v\nfile:\n%s", back, d, out)
	}
	if _, err := config.Parse(out, "/home/u/.config/graft/config.yaml"); valid && err != nil {
		t.Errorf("config.Parse: %v\nfile:\n%s", err, out)
	}
	return string(out)
}

func TestApplyWithoutChangesKeepsComments(t *testing.T) {
	data := string(readExample(t))
	out := change(t, data, true, func(*Draft) {})
	for _, comment := range []string{
		"# graft config: copy to ~/.config/graft/config.yaml and edit.",
		"cliproxy: # CLIProxyAPI on this machine",
		"label: GPT-6.1 Sol # the /model row; defaults to the name",
		"  # The whole session on one external model: graft sol",
		"  # GPT as the main model; agents with model: opus and the auto mode",
	} {
		if !strings.Contains(out, comment) {
			t.Errorf("lost %q:\n%s", comment, out)
		}
	}
	for _, section := range []string{"\n\n# Models: the name", "\n\n# Defaults for every profile", "\n\nprofiles:"} {
		if !strings.Contains(out, section) {
			t.Errorf("no blank line before %q:\n%s", section, out)
		}
	}
}

func TestApplyScalars(t *testing.T) {
	data := `models:
  sol:
    provider: cliproxy
    context: 400000
    label: GPT Sol # the /model row
    description: old
profiles:
  sol: { model: sol, effort: max }
`
	out := change(t, data, false, func(d *Draft) {
		d.Models[0].Label = "true"
		d.Models[0].Context = "abc"
		d.Models[0].Description = ""
		d.Models[0].ID = "gpt-6.1-sol"
		d.Profiles[0].Subscription = "true"
	})
	for _, want := range []string{
		`    label: "true" # the /model row`,
		"    context: abc\n",
		"    provider: cliproxy\n    id: gpt-6.1-sol\n    context:",
		"sol: {model: sol, subscription: true, effort: max}",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("want %q in:\n%s", want, out)
		}
	}
	if strings.Contains(out, "description") {
		t.Errorf("description was not removed:\n%s", out)
	}
}

func TestApplyEntries(t *testing.T) {
	data := `providers:
  # local proxy
  cliproxy: # on this machine
    url: http://127.0.0.1:8317
    key: { file: keys/cliproxy }
models:
  glm: { provider: cliproxy, context: 524288 }
profiles:
  # the first one
  a: { model: glm }
  # the second one
  b: { model: glm }
`
	out := change(t, data, true, func(d *Draft) {
		d.Providers[0].Name = "proxy"
		d.Models[0].Provider = "proxy"
		d.Profiles = []Profile{d.Profiles[1], {Name: "c", Model: "glm", Effort: "high"}}
	})
	for _, want := range []string{
		"  # local proxy\n  proxy: # on this machine\n",
		"  # the second one\n  b: {model: glm}\n  c:\n    model: glm\n    effort: high\n",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("want %q in:\n%s", want, out)
		}
	}
	if strings.Contains(out, "the first one") {
		t.Errorf("profile a and its comment were not removed:\n%s", out)
	}
}

func TestApplySwapsNames(t *testing.T) {
	data := "profiles:\n  a: { model: opus }\n  b: { model: sonnet }\n"
	out := change(t, data, true, func(d *Draft) {
		d.Profiles[0].Name, d.Profiles[1].Name = "b", "a"
	})
	if want := "profiles:\n  b: {model: opus}\n  a: {model: sonnet}\n"; out != want {
		t.Errorf("got:\n%s\nwant:\n%s", out, want)
	}
}

func TestApplyFromNothing(t *testing.T) {
	out := change(t, "", true, func(d *Draft) {
		d.Profiles = append(d.Profiles, Profile{Name: "glm", Model: "glm"})
		d.Models = append(d.Models, Model{Name: "glm", Provider: "ollama", Context: "1048576"})
		d.Providers = append(d.Providers, Provider{Name: "ollama", URL: "https://ollama.com",
			Key: Key{Kind: "file", Value: "~/.config/graft/keys/ollama"}})
	})
	want := `providers:
  ollama:
    url: https://ollama.com
    key: {file: ~/.config/graft/keys/ollama}

models:
  glm:
    provider: ollama
    context: 1048576

profiles:
  glm:
    model: glm
`
	if out != want {
		t.Errorf("got:\n%s\nwant:\n%s", out, want)
	}
}

func TestApplyRemovesEmptiedSections(t *testing.T) {
	data := "defaults:\n  effort: max\nmodels:\n  m: { provider: p, context: 100000 }\nprofiles:\n  x: { model: opus }\n"
	out := change(t, data, true, func(d *Draft) {
		d.Defaults.Effort = ""
		d.Models = nil
	})
	if want := "profiles:\n  x: {model: opus}\n"; out != want {
		t.Errorf("got:\n%s\nwant:\n%s", out, want)
	}
}

func TestApplyLists(t *testing.T) {
	data := `defaults:
  args:
    - --one
profiles:
  x: { model: opus, picker: [a, b] }
models:
  a: { provider: p, context: 100000 }
  b: { provider: p, context: 100000 }
  c: { provider: p, context: 100000 }
`
	out := change(t, data, false, func(d *Draft) {
		d.Defaults.Args = append(d.Defaults.Args, "--two")
		d.Profiles[0].Picker = []string{"c", "a"}
	})
	for _, want := range []string{"  args:\n    - --one\n    - --two\n", "x: {model: opus, picker: [c, a]}"} {
		if !strings.Contains(out, want) {
			t.Errorf("want %q in:\n%s", want, out)
		}
	}
	out = change(t, data, false, func(d *Draft) { d.Profiles[0].Picker = nil })
	if strings.Contains(out, "picker") {
		t.Errorf("picker was not removed:\n%s", out)
	}
}

func TestApplyEnv(t *testing.T) {
	data := `profiles:
  x:
    model: opus
    env:
      KEEP: "1" # stays
      CHANGE: a
      DROP: b
`
	out := change(t, data, true, func(d *Draft) {
		d.Profiles[0].Env = []Pair{{"KEEP", "1"}, {"CHANGE", ""}, {"NEW", "0"}}
	})
	if want := "    env:\n      KEEP: \"1\" # stays\n      CHANGE: \"\"\n      NEW: \"0\"\n"; !strings.Contains(out, want) {
		t.Errorf("want %q in:\n%s", want, out)
	}
	out = change(t, data, true, func(d *Draft) { d.Profiles[0].Env = nil })
	if strings.Contains(out, "env") {
		t.Errorf("env was not removed:\n%s", out)
	}
}

func TestApplyAliases(t *testing.T) {
	data := `models:
  glm: { provider: p, context: 100000 }
profiles:
  x:
    model: glm
    picker: [glm]
    effort: max
  y:
    model: glm
    aliases: { haiku: glm, opux: glm }
`
	out := change(t, data, false, func(d *Draft) {
		d.Profiles[0].Aliases["haiku"] = "glm"
		d.Profiles[1].Aliases["haiku"] = ""
	})
	for _, want := range []string{"    picker: [glm]\n    aliases: {haiku: glm}\n    effort: max\n", "aliases: {opux: glm}"} {
		if !strings.Contains(out, want) {
			t.Errorf("want %q in:\n%s", want, out)
		}
	}
	data = "profiles:\n  y: { model: opus, aliases: { haiku: claude-haiku-5-5 } }\n"
	out = change(t, data, true, func(d *Draft) { d.Profiles[0].Aliases["haiku"] = "" })
	if want := "profiles:\n  y: {model: opus}\n"; out != want {
		t.Errorf("got:\n%s\nwant:\n%s", out, want)
	}
}

func TestApplyKey(t *testing.T) {
	data := `providers:
  a:
    url: https://a
    key: { file: keys/a }
  b:
    url: https://b
    key:
      env: B_KEY
`
	out := change(t, data, false, func(d *Draft) {
		d.Providers[0].Key = Key{Kind: "env", Value: "A_KEY"}
		d.Providers[1].Key = Key{Kind: "command", Value: "pass show b"}
	})
	for _, want := range []string{"    key: {env: A_KEY}\n", "    key:\n      command: pass show b\n"} {
		if !strings.Contains(out, want) {
			t.Errorf("want %q in:\n%s", want, out)
		}
	}
}

func TestApplySettings(t *testing.T) {
	data := `defaults:
  settings: &common
    autoCompactEnabled: true
profiles:
  x:
    model: opus
    settings: *common
`
	out := change(t, data, true, func(d *Draft) {
		d.Profiles[0].Settings = "# mine\nautoCompactEnabled: false\n"
	})
	if want := "    settings:\n      # mine\n      autoCompactEnabled: false\n"; !strings.Contains(out, want) {
		t.Errorf("want %q in:\n%s", want, out)
	}
	if !strings.Contains(out, "  settings: &common\n    autoCompactEnabled: true\n") {
		t.Errorf("defaults lost its settings or anchor:\n%s", out)
	}
	out = change(t, data, true, func(d *Draft) { d.Defaults.Settings = `{"autoCompactEnabled": false}` })
	if want := "defaults:\n  settings: {\"autoCompactEnabled\": false}\n"; !strings.Contains(out, want) {
		t.Errorf("want %q in:\n%s", want, out)
	}
	if want := "    settings:\n      autoCompactEnabled: true\n"; !strings.Contains(out, want) || strings.Contains(out, "common") {
		t.Errorf("the alias did not get a copy of the old value:\n%s", out)
	}
	out = change(t, data, true, func(d *Draft) { d.Profiles[0].Settings = "  # nothing\n" })
	if strings.Contains(out, "*common") {
		t.Errorf("settings were not removed:\n%s", out)
	}
}

func TestApplyKeepsAChangeOutOfAliases(t *testing.T) {
	data := `models:
  a: { provider: p, id: &i glm-5, context: 100000 }
  b: { provider: p, id: *i, context: 100000 }
`
	out := change(t, data, false, func(d *Draft) { d.Models[0].ID = "glm-6" })
	if !strings.Contains(out, "a: {provider: p, id: glm-6, context: 100000}") || !strings.Contains(out, "b: {provider: p, id: glm-5, context: 100000}") {
		t.Errorf("got:\n%s", out)
	}
	out = change(t, data, false, func(d *Draft) { d.Models = d.Models[1:] })
	if want := "models:\n  b: {provider: p, id: glm-5, context: 100000}\n"; out != want {
		t.Errorf("got:\n%s\nwant:\n%s", out, want)
	}
}

func TestApplyCopiesAliasedEntries(t *testing.T) {
	data := `models:
  a: &m { provider: p, context: 100000 }
  b: *m
`
	out := change(t, data, false, func(d *Draft) { d.Models[1].Context = "200000" })
	if !strings.Contains(out, "a: &m {provider: p, context: 100000}") || !strings.Contains(out, "b: {provider: p, context: 200000}") {
		t.Errorf("got:\n%s", out)
	}
}

// TestApplyKeepsUnknownFields: the forms do not show a field graft does
// not know, and a save leaves it for the user to fix in the editor.
func TestApplyKeepsUnknownFields(t *testing.T) {
	data := "models:\n  m: { provider: p, context: 100000, contxt: 5 }\nextra: 1\n"
	out := change(t, data, false, func(d *Draft) { d.Models[0].Context = "200000" })
	if !strings.Contains(out, "m: {provider: p, context: 200000, contxt: 5}") || !strings.Contains(out, "extra: 1") {
		t.Errorf("got:\n%s", out)
	}
}

func TestApplyErrors(t *testing.T) {
	data := "profiles:\n  a: { model: opus }\n"
	d, _ := Read([]byte(data))
	tests := []struct {
		edit func(d *Draft)
		want string
	}{
		{func(d *Draft) { d.Profiles[0].Was = "zzz" }, `profiles: the file has no entry "zzz"`},
		{func(d *Draft) { d.Profiles = append(d.Profiles, d.Profiles[0]) }, `profiles: entry "a" appears twice`},
		{func(d *Draft) { d.Profiles[0].Settings = "a: [" }, "profiles.a.settings: yaml:"},
		{func(d *Draft) { d.Profiles[0].Settings = "a: 1\n---\nb: 2\n" }, "profiles.a.settings: line 2: must be one YAML document"},
	}
	for _, tt := range tests {
		c := d
		c.Profiles = slices.Clone(d.Profiles)
		tt.edit(&c)
		_, err := Apply([]byte(data), c)
		var es Errors
		if !errors.As(err, &es) || !strings.Contains(err.Error(), tt.want) {
			t.Errorf("Apply = %v, want Errors with %q", err, tt.want)
		}
	}
}

// TestApplyRoundTrip makes many changes to the example config and checks
// that each result reads back as the draft that made it.
func TestApplyRoundTrip(t *testing.T) {
	data := string(readExample(t))
	edits := []func(d *Draft){
		func(d *Draft) { d.Providers[1].URL = "https://ollama.example" },
		func(d *Draft) { d.Models[0].Settings = "maxEffortLevel: medium\n" },
		func(d *Draft) { d.Models = slices.Delete(d.Models, 4, 5) },
		func(d *Draft) { d.Profiles[3].Subscription = "" },
		func(d *Draft) { d.Profiles[0].Env = []Pair{{"CLAUDE_CODE_EFFORT_LEVEL", "max"}} },
		func(d *Draft) { d.Defaults = Defaults{} },
		func(d *Draft) {
			d.Models[1].Name = "glm-cliproxy"
			d.Profiles[1].Model = "glm-cliproxy"
			d.Profiles[4].Aliases["haiku"] = "glm-cliproxy"
		},
		func(d *Draft) {
			d.Profiles = append(d.Profiles, Profile{Name: "kimi", Model: "kimi", Args: []string{"--verbose"}})
		},
	}
	for _, edit := range edits {
		change(t, data, false, edit)
	}
}

// TestApplyUnicodeAndCRLF: comments and values in any language survive a
// save, and a file with CRLF line endings comes out valid, with LF.
func TestApplyUnicodeAndCRLF(t *testing.T) {
	data := "# конфиг\r\nprofiles:\r\n  x: { model: opus } # основной 🚀\r\n  y: { model: sonnet }\r\n"
	out := change(t, data, true, func(d *Draft) {
		d.Profiles[1].Effort = "high"
		d.Profiles[1].Env = []Pair{{"GREETING", "привет"}}
	})
	for _, want := range []string{"# конфиг\n", "x: {model: opus} # основной 🚀\n", "GREETING: привет"} {
		if !strings.Contains(out, want) {
			t.Errorf("want %q in:\n%s", want, out)
		}
	}
	if strings.Contains(out, "\r") {
		t.Errorf("CR left in:\n%q", out)
	}
}

// TestApplySpecialStrings: names and values that YAML would read as
// something else stay strings.
func TestApplySpecialStrings(t *testing.T) {
	out := change(t, "profiles:\n  x: { model: opus }\n", false, func(d *Draft) {
		d.Profiles = append(d.Profiles, Profile{Name: "true", Model: "opus", Env: []Pair{{"N", "0123"}, {"B", "yes"}, {"E", ""}}})
		d.Models = append(d.Models, Model{Name: "1.0", Provider: "p", ID: "1.0", Context: "100000", Label: "null"})
	})
	for _, want := range []string{`"1.0":`, `id: "1.0"`, `label: "null"`, `"true":`, `N: "0123"`, `E: ""`} {
		if !strings.Contains(out, want) {
			t.Errorf("want %q in:\n%s", want, out)
		}
	}
}

// TestApplyJSONSettings: JSON pasted from the Claude Code docs or an
// editor, indented with tabs, is a valid settings text.
func TestApplyJSONSettings(t *testing.T) {
	out := change(t, "profiles:\n  x: { model: opus }\n", true, func(d *Draft) {
		d.Profiles[0].Settings = "{\n\t\"autoCompactEnabled\": true,\n\t\"permissions\": {\"allow\": [\"Bash\"]}\n}\n"
	})
	if !strings.Contains(out, `settings: {"autoCompactEnabled": true, "permissions": {"allow": ["Bash"]}}`) {
		t.Errorf("got:\n%s", out)
	}
}
```

- [ ] **Шаг 2: Убедиться, что тесты падают**

Запустите: `go test ./internal/edit/`

Ожидается: FAIL: `undefined: Apply`.

- [ ] **Шаг 3: Рендер файла**

Замените `render.go` целиком: добавляются `render`, `spaceSections` и `isTopKey`.

`internal/edit/render.go`:

```go
package edit

import (
	"bytes"
	"strings"

	"go.yaml.in/yaml/v3"
)

// encode writes n as YAML with an indent of 2.
func encode(n *yaml.Node) ([]byte, error) {
	var b bytes.Buffer
	enc := yaml.NewEncoder(&b)
	enc.SetIndent(2)
	if err := enc.Encode(n); err != nil {
		return nil, err
	}
	if err := enc.Close(); err != nil {
		return nil, err
	}
	return b.Bytes(), nil
}

// render writes the document with a blank line before every top-level key
// but the first, so that the sections stand apart.
func render(doc *yaml.Node) ([]byte, error) {
	data, err := encode(doc)
	if err != nil {
		return nil, err
	}
	return spaceSections(data), nil
}

// spaceSections puts a blank line before each top-level key but the first,
// and before the comment lines right above such a key.
func spaceSections(data []byte) []byte {
	lines := strings.SplitAfter(string(data), "\n")
	seenKey := false
	for i, line := range lines {
		if !isTopKey(line) {
			continue
		}
		if seenKey {
			// The key and the comment lines right above it.
			start := i
			for start > 0 && strings.HasPrefix(lines[start-1], "#") {
				start--
			}
			if start > 0 && strings.TrimSpace(lines[start-1]) != "" {
				lines[start] = "\n" + lines[start]
			}
		}
		seenKey = true
	}
	return []byte(strings.Join(lines, ""))
}

// isTopKey reports whether line starts a key of the top-level mapping.
func isTopKey(line string) bool {
	if line == "" || line == "\n" {
		return false
	}
	switch line[0] {
	case ' ', '\t', '#', '-', '\n', '{', '[', '}', ']', '.':
		return false
	}
	return true
}

// snippet returns n as YAML text, as a settings field shows it: without
// the anchor that aliases of it use.
func snippet(n *yaml.Node) (string, error) {
	c := *deref(n)
	c.Anchor = ""
	data, err := encode(&c)
	return string(data), err
}
```

- [ ] **Шаг 4: Слияние**

Главные правила, которые держит код:
- запись сопоставляется по `Was`; ключи запоминаются до переименований, поэтому две записи могут обменяться именами;
- запись, у которой изменилось только имя, не трогается дальше узла ключа;
- поле меняется, только если отличается от черновика файла; скаляр — на месте, с комментариями;
- `unshare`: перед правкой или удалением значения с якорем каждая ссылка `*x` получает копию прежнего значения, а якорь исчезает — правка одного поля никогда не меняет другое (раздел «Слияние» спеки, п. 4).

`internal/edit/apply.go`:

```go
package edit

import (
	"fmt"
	"reflect"
	"slices"
	"strings"

	"go.yaml.in/yaml/v3"

	"github.com/zinin/graft/internal/config"
)

// Error is a problem of a draft at a path of the config, such as
// profiles.mix.settings.
type Error struct {
	Path string `json:"path"`
	Msg  string `json:"msg"`
}

// Errors is every problem of a draft that Apply found.
type Errors []Error

func (es Errors) Error() string {
	lines := make([]string, len(es))
	for i, e := range es {
		lines[i] = e.Path + ": " + e.Msg
	}
	return strings.Join(lines, "\n")
}

// Apply returns data with the changes from Read(data) to d made: entries
// added, removed and renamed by Was, fields set and cleared. Nodes the
// changes do not touch keep their comments, style and anchors. A change
// never reaches another field through a YAML alias: before a value with an
// anchor changes, each alias of it becomes a copy of the old value. Apply
// returns Errors when d does not fit data, such as an entry whose Was the
// file lacks, or a settings text that is not YAML.
func Apply(data []byte, d Draft) ([]byte, error) {
	base, err := Read(data)
	if err != nil {
		return nil, err
	}
	doc, err := parse(data)
	if err != nil {
		return nil, err
	}
	d.Normalize()
	a := &applier{root: doc.Content[0]}
	entries(a, "providers", base.Providers, d.Providers, a.provider)
	entries(a, "models", base.Models, d.Models, a.model)
	a.defaults(base.Defaults, d.Defaults)
	entries(a, "profiles", base.Profiles, d.Profiles, a.profile)
	if len(a.errs) > 0 {
		return nil, a.errs
	}
	return render(doc)
}

type applier struct {
	root *yaml.Node
	errs Errors
}

func (a *applier) errorf(path, format string, args ...any) {
	a.errs = append(a.errs, Error{Path: path, Msg: fmt.Sprintf(format, args...)})
}

// entry is what entries needs of Provider, Model and Profile.
type entry[T any] interface {
	names() (was, name string)
	body() T // the entry without Was and Name
}

func (p Provider) names() (string, string) { return p.Was, p.Name }
func (m Model) names() (string, string)    { return m.Was, m.Name }
func (p Profile) names() (string, string)  { return p.Was, p.Name }

func (p Provider) body() Provider { p.Was, p.Name = "", ""; return p }
func (m Model) body() Model       { m.Was, m.Name = "", ""; return m }
func (p Profile) body() Profile   { p.Was, p.Name = "", ""; return p }

// entries applies the entries of a section: it removes the entries of base
// that want lacks, renames the rest, fills those whose fields changed and
// adds the new ones. fill sets the fields of an entry's mapping from old,
// nil for a new entry, to cur.
func entries[T entry[T]](a *applier, section string, base, want []T, fill func(m *yaml.Node, old *T, cur T, path string)) {
	olds := map[string]*T{}
	for i := range base {
		was, _ := base[i].names()
		olds[was] = &base[i]
	}
	used := map[string]bool{}
	for _, e := range want {
		was, _ := e.names()
		switch {
		case was == "":
		case olds[was] == nil:
			a.errorf(section, "the file has no entry %q: reload the page", was)
			return
		case used[was]:
			a.errorf(section, "entry %q appears twice", was)
			return
		default:
			used[was] = true
		}
	}
	if len(base) == 0 && len(want) == 0 {
		return
	}
	sec := a.mapping(a.root, section, config.Sections, 0)
	// Keys by Was before any rename, so that two entries can swap names.
	keys := map[string]*yaml.Node{}
	for was := range olds {
		i := find(sec, was)
		if !used[was] {
			a.drop(sec, i)
			continue
		}
		keys[was] = sec.Content[i]
	}
	for _, e := range want {
		was, name := e.names()
		path := section + "." + name
		if was == "" {
			m := &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map"}
			fill(m, nil, e, path)
			sec.Content = append(sec.Content, scalarNode(name, "!!str"), m)
			continue
		}
		k := keys[was]
		k.Value = name
		if old := olds[was]; !reflect.DeepEqual((*old).body(), e.body()) {
			fill(a.own(sec, slices.Index(sec.Content, k)+1), old, e, path)
		}
	}
	if len(sec.Content) == 0 && len(base) > 0 {
		a.drop(a.root, find(a.root, section))
	}
}

func (a *applier) provider(m *yaml.Node, old *Provider, cur Provider, path string) {
	var o Provider
	if old != nil {
		o = *old
	}
	a.scalar(m, "url", config.ProviderFields, o.URL, cur.URL, strTag)
	a.key(m, o.Key, cur.Key)
}

func (a *applier) model(m *yaml.Node, old *Model, cur Model, path string) {
	var o Model
	if old != nil {
		o = *old
	}
	f := config.ModelFields
	a.scalar(m, "provider", f, o.Provider, cur.Provider, strTag)
	a.scalar(m, "id", f, o.ID, cur.ID, strTag)
	a.scalar(m, "context", f, o.Context, cur.Context, intTag)
	a.scalar(m, "label", f, o.Label, cur.Label, strTag)
	a.scalar(m, "description", f, o.Description, cur.Description, strTag)
	a.settings(m, f, o.Settings, cur.Settings, path)
}

func (a *applier) defaults(old, cur Defaults) {
	if reflect.DeepEqual(old, cur) {
		return
	}
	m := a.mapping(a.root, "defaults", config.Sections, 0)
	f := config.DefaultsFields
	a.scalar(m, "effort", f, old.Effort, cur.Effort, strTag)
	a.settings(m, f, old.Settings, cur.Settings, "defaults")
	a.env(m, f, old.Env, cur.Env)
	a.list(m, "args", f, old.Args, cur.Args)
	if len(m.Content) == 0 {
		a.drop(a.root, find(a.root, "defaults"))
	}
}

func (a *applier) profile(m *yaml.Node, old *Profile, cur Profile, path string) {
	var o Profile
	if old != nil {
		o = *old
	}
	f := config.ProfileFields
	a.scalar(m, "model", f, o.Model, cur.Model, strTag)
	a.scalar(m, "subscription", f, o.Subscription, cur.Subscription, boolTag)
	a.list(m, "picker", f, o.Picker, cur.Picker)
	a.aliases(m, o.Aliases, cur.Aliases)
	a.scalar(m, "subagents", f, o.Subagents, cur.Subagents, strTag)
	a.scalar(m, "effort", f, o.Effort, cur.Effort, strTag)
	a.settings(m, f, o.Settings, cur.Settings, path)
	a.env(m, f, o.Env, cur.Env)
	a.list(m, "args", f, o.Args, cur.Args)
}

// scalar sets field of mapping m from old to cur. order gives the place of
// a new field; tag gives the tag of a value.
func (a *applier) scalar(m *yaml.Node, field string, order []string, old, cur string, tag func(string) string) {
	if old == cur {
		return
	}
	i := find(m, field)
	switch {
	case cur == "":
		a.drop(m, i)
	case i >= 0 && m.Content[i+1].Kind == yaml.ScalarNode:
		// In place: the value keeps its comments.
		v := m.Content[i+1]
		a.unshare(v)
		v.Value, v.Tag = cur, tag(cur)
		if v.Style&(yaml.LiteralStyle|yaml.FoldedStyle|yaml.TaggedStyle) != 0 {
			v.Style = 0
		}
	default:
		// No field yet, or an alias, which gets a value of its own.
		a.set(m, order, field, i, scalarNode(cur, tag(cur)))
	}
}

func strTag(string) string { return "!!str" }

// intTag keeps a value that is not a number a string, which config.Parse
// then reports, instead of writing text YAML would read as something else.
func intTag(v string) string {
	if v != "" && strings.Trim(v, "0123456789") == "" {
		return "!!int"
	}
	return "!!str"
}

func boolTag(v string) string {
	if v == "true" || v == "false" {
		return "!!bool"
	}
	return "!!str"
}

// list sets the list field of m from old to cur. A rebuilt list keeps the
// style and comments of the old one; a new one is written as [a, b].
func (a *applier) list(m *yaml.Node, field string, order []string, old, cur []string) {
	if slices.Equal(old, cur) {
		return
	}
	i := find(m, field)
	if len(cur) == 0 {
		a.drop(m, i)
		return
	}
	seq := &yaml.Node{Kind: yaml.SequenceNode, Tag: "!!seq", Style: yaml.FlowStyle}
	if i >= 0 && m.Content[i+1].Kind == yaml.SequenceNode {
		keepLook(seq, m.Content[i+1])
	}
	for _, s := range cur {
		seq.Content = append(seq.Content, scalarNode(s, "!!str"))
	}
	a.set(m, order, field, i, seq)
}

// env sets the env field of m from old to cur. A variable that keeps its
// name keeps its key node, and its value node when the value is the same.
func (a *applier) env(m *yaml.Node, order []string, old, cur []Pair) {
	if slices.Equal(old, cur) {
		return
	}
	i := find(m, "env")
	if len(cur) == 0 {
		a.drop(m, i)
		return
	}
	out := &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map"}
	var prev *yaml.Node
	if i >= 0 && m.Content[i+1].Kind == yaml.MappingNode {
		prev = m.Content[i+1]
		keepLook(out, prev)
	}
	for _, p := range cur {
		var k, v *yaml.Node
		if j := find(prev, p[0]); j >= 0 {
			k, v = prev.Content[j], prev.Content[j+1]
		}
		if k == nil {
			k = scalarNode(p[0], "!!str")
		}
		if v == nil || v.Kind != yaml.ScalarNode || v.Value != p[1] || isNull(v) {
			v = scalarNode(p[1], "!!str")
		}
		out.Content = append(out.Content, k, v)
	}
	a.set(m, order, "env", i, out)
}

// aliases sets the aliases of m from old to cur, one alias at a time, so
// that the names the draft does not know stay.
func (a *applier) aliases(m *yaml.Node, old, cur map[string]string) {
	var am *yaml.Node
	for _, name := range config.AliasNames {
		if old[name] == cur[name] {
			continue
		}
		if am == nil {
			am = a.mapping(m, "aliases", config.ProfileFields, yaml.FlowStyle)
		}
		a.scalar(am, name, config.AliasNames, old[name], cur[name], strTag)
	}
	if am != nil && len(am.Content) == 0 {
		a.drop(m, find(m, "aliases"))
	}
}

// key sets the key source of provider m: a mapping with one source, in
// the style of the old one, or {kind: value} when there was none.
func (a *applier) key(m *yaml.Node, old, cur Key) {
	if old == cur {
		return
	}
	i := find(m, "key")
	if cur.Kind == "" {
		a.drop(m, i)
		return
	}
	k := &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map", Style: yaml.FlowStyle}
	if i >= 0 && m.Content[i+1].Kind == yaml.MappingNode {
		keepLook(k, m.Content[i+1])
	}
	k.Content = []*yaml.Node{scalarNode(cur.Kind, "!!str"), scalarNode(cur.Value, "!!str")}
	a.set(m, config.ProviderFields, "key", i, k)
}

// settings sets the settings of m from the text old to the text cur.
func (a *applier) settings(m *yaml.Node, order []string, old, cur, path string) {
	if old == cur {
		return
	}
	i := find(m, "settings")
	n, err := parseSnippet(cur)
	switch {
	case err != nil:
		a.errorf(path+".settings", "%v", err)
	case n == nil:
		a.drop(m, i)
	default:
		a.set(m, order, "settings", i, n)
	}
}

// parseSnippet returns the value of a settings text, nil for a text with
// no value: empty, only comments, or null.
func parseSnippet(text string) (*yaml.Node, error) {
	doc, err := decode([]byte(text))
	if err != nil {
		return nil, err
	}
	if len(doc.Content) == 0 || isNull(doc.Content[0]) {
		return nil, nil
	}
	return doc.Content[0], nil
}

// mapping returns the mapping of field in m, made a node of m's own. A
// missing field gets an empty mapping of style in its place by order.
func (a *applier) mapping(m *yaml.Node, field string, order []string, style yaml.Style) *yaml.Node {
	i := find(m, field)
	if i < 0 {
		n := &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map", Style: style}
		insert(m, order, field, n)
		return n
	}
	return a.own(m, i+1)
}

// own makes m.Content[i] a mapping that m alone changes: a copy of what an
// alias points at, an empty mapping in place of null, and no longer the
// target of aliases.
func (a *applier) own(m *yaml.Node, i int) *yaml.Node {
	n := m.Content[i]
	switch {
	case n.Kind == yaml.AliasNode:
		n = clone(deref(n))
		m.Content[i] = n
	case isNull(n):
		n = &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map",
			HeadComment: n.HeadComment, LineComment: n.LineComment, FootComment: n.FootComment}
		m.Content[i] = n
	default:
		a.unshare(n)
	}
	return n
}

// set puts v as the value of field: in place of the value at index i of m,
// or in its place by order when i is -1.
func (a *applier) set(m *yaml.Node, order []string, field string, i int, v *yaml.Node) {
	if i < 0 {
		insert(m, order, field, v)
		return
	}
	a.unshareAll(m.Content[i+1])
	m.Content[i+1] = v
}

// drop removes the pair at index i of m; -1 does nothing.
func (a *applier) drop(m *yaml.Node, i int) {
	if i >= 0 {
		a.unshareAll(m.Content[i+1])
		remove(m, i)
	}
}

// unshare gives every alias of n a copy of n's value and takes n's anchor
// away, so that a change of n stays in n.
func (a *applier) unshare(n *yaml.Node) {
	if n.Anchor == "" {
		return
	}
	var walk func(p *yaml.Node)
	walk = func(p *yaml.Node) {
		for i, c := range p.Content {
			if c.Kind == yaml.AliasNode && c.Alias == n {
				p.Content[i] = clone(n)
				continue
			}
			walk(c)
		}
	}
	walk(a.root)
	n.Anchor = ""
}

// unshareAll unshares n and every node under it: n is about to go.
func (a *applier) unshareAll(n *yaml.Node) {
	a.unshare(n)
	if n.Kind != yaml.AliasNode {
		for _, c := range n.Content {
			a.unshareAll(c)
		}
	}
}

// clone copies n without its anchors: the anchors stay with the original,
// which other aliases point at.
func clone(n *yaml.Node) *yaml.Node {
	c := *n
	c.Anchor = ""
	c.Content = nil
	if n.Kind != yaml.AliasNode {
		for _, child := range n.Content {
			c.Content = append(c.Content, clone(child))
		}
	}
	return &c
}

// keepLook gives n the style and comments of prev, which n replaces.
func keepLook(n, prev *yaml.Node) {
	n.Style = prev.Style
	n.HeadComment, n.LineComment, n.FootComment = prev.HeadComment, prev.LineComment, prev.FootComment
}

// insert adds field before the first key of m that comes after it in
// order, or at the end.
func insert(m *yaml.Node, order []string, field string, v *yaml.Node) {
	at := len(m.Content)
	if rank := slices.Index(order, field); rank >= 0 {
		for i := 0; i+1 < len(m.Content); i += 2 {
			if r := slices.Index(order, m.Content[i].Value); r > rank {
				at = i
				break
			}
		}
	}
	m.Content = slices.Insert(m.Content, at, scalarNode(field, "!!str"), v)
}

// find returns the index of key in mapping m, or -1.
func find(m *yaml.Node, key string) int {
	if m == nil {
		return -1
	}
	for i := 0; i+1 < len(m.Content); i += 2 {
		if k := m.Content[i]; k.Kind == yaml.ScalarNode && k.Value == key {
			return i
		}
	}
	return -1
}

// remove deletes the pair at index i of mapping m; -1 does nothing.
func remove(m *yaml.Node, i int) {
	if i >= 0 {
		m.Content = slices.Delete(m.Content, i, i+2)
	}
}

func scalarNode(value, tag string) *yaml.Node {
	return &yaml.Node{Kind: yaml.ScalarNode, Tag: tag, Value: value}
}
```

- [ ] **Шаг 5: Тесты проходят**

Запустите: `test -z "$(gofmt -l .)" && go vet ./... && go test -race ./...`

Ожидается: команда завершается с кодом 0: `gofmt` не нашёл неотформатированных файлов, `go vet` молчит, все пакеты `ok`.

- [ ] **Шаг 6: Commit**

```bash
git add internal/edit
git commit -m "feat(edit): apply a changed draft to the config file"
```

### Task 3: Файл конфига — `ui.Store`

**Files:**
- Create: `internal/ui/store.go`
- Test: `internal/ui/store_test.go`

**Interfaces:**
- Consumes: ничего из предыдущих задач.
- Produces:
  - `var ui.ErrConflict` — файл изменился после версии, на которой основана запись.
  - `func ui.NewStore(path string) *Store`; `(*Store) Version() (string, error)`; `(*Store) Read() ([]byte, string, error)`; `(*Store) Write(base string, data []byte) (ver, backup string, err error)`.
  - Версия — sha256 файла в hex, `""` — файла нет. Неэкспортируемая `version(data []byte) string` нужна тестам задач 4–7.

- [ ] **Шаг 1: Написать падающие тесты**

`internal/ui/store_test.go`:

```go
package ui

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestStoreCreatesTheFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "graft", "config.yaml")
	s := NewStore(path)
	if data, v, err := s.Read(); data != nil || v != "" || err != nil {
		t.Fatalf("Read of no file = %q, %q, %v", data, v, err)
	}
	v, backup, err := s.Write("", []byte("a: 1\n"))
	if err != nil || backup != "" {
		t.Fatalf("Write = %q, %q, %v", v, backup, err)
	}
	if v != version([]byte("a: 1\n")) {
		t.Errorf("version %q", v)
	}
	if info, _ := os.Stat(path); info.Mode().Perm() != 0o600 {
		t.Errorf("file mode %v, want 0600", info.Mode().Perm())
	}
	if info, _ := os.Stat(filepath.Dir(path)); info.Mode().Perm() != 0o700 {
		t.Errorf("directory mode %v, want 0700", info.Mode().Perm())
	}
	if _, err := os.Stat(path + ".bak"); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("a new file has a backup: %v", err)
	}
}

func TestStoreKeepsModeAndBackup(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yaml")
	os.WriteFile(path, []byte("old\n"), 0o640)
	s := NewStore(path)
	_, v, _ := s.Read()
	if _, backup, err := s.Write(v, []byte("new\n")); err != nil || backup != path+".bak" {
		t.Fatalf("Write = %q, %v", backup, err)
	}
	if got, _ := os.ReadFile(path); string(got) != "new\n" {
		t.Errorf("file %q", got)
	}
	if got, _ := os.ReadFile(path + ".bak"); string(got) != "old\n" {
		t.Errorf("backup %q", got)
	}
	for _, p := range []string{path, path + ".bak"} {
		if info, _ := os.Stat(p); info.Mode().Perm() != 0o640 {
			t.Errorf("%s mode %v, want 0640", p, info.Mode().Perm())
		}
	}
	entries, _ := os.ReadDir(filepath.Dir(path))
	if len(entries) != 2 {
		t.Errorf("left files behind: %v", entries)
	}
}

func TestStoreRefusesAStaleVersion(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yaml")
	os.WriteFile(path, []byte("old\n"), 0o600)
	s := NewStore(path)
	_, v, _ := s.Read()
	os.WriteFile(path, []byte("edited by hand\n"), 0o600)
	if _, _, err := s.Write(v, []byte("new\n")); !errors.Is(err, ErrConflict) {
		t.Errorf("Write = %v, want ErrConflict", err)
	}
	if _, _, err := s.Write("", []byte("new\n")); !errors.Is(err, ErrConflict) {
		t.Errorf("Write as a new file = %v, want ErrConflict", err)
	}
	if got, _ := os.ReadFile(path); string(got) != "edited by hand\n" {
		t.Errorf("file %q", got)
	}
}

func TestStoreWritesThroughASymlink(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "dotfiles", "config.yaml")
	os.MkdirAll(filepath.Dir(target), 0o700)
	os.WriteFile(target, []byte("old\n"), 0o600)
	link := filepath.Join(dir, "config.yaml")
	os.Symlink(target, link)
	s := NewStore(link)
	_, v, _ := s.Read()
	if _, _, err := s.Write(v, []byte("new\n")); err != nil {
		t.Fatal(err)
	}
	if info, err := os.Lstat(link); err != nil || info.Mode()&os.ModeSymlink == 0 {
		t.Errorf("the link is gone: %v, %v", info, err)
	}
	if got, _ := os.ReadFile(target); string(got) != "new\n" {
		t.Errorf("target %q", got)
	}
	if got, _ := os.ReadFile(target + ".bak"); string(got) != "old\n" {
		t.Errorf("backup %q", got)
	}
}

func TestStoreRefusesABrokenSymlink(t *testing.T) {
	dir := t.TempDir()
	link := filepath.Join(dir, "config.yaml")
	os.Symlink(filepath.Join(dir, "missing.yaml"), link)
	if _, _, err := NewStore(link).Write("", []byte("a: 1\n")); err == nil {
		t.Error("Write through a broken symlink succeeded")
	}
}

func TestStoreVersionFollowsTheFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yaml")
	s := NewStore(path)
	if v, err := s.Version(); v != "" || err != nil {
		t.Fatalf("Version of no file = %q, %v", v, err)
	}
	os.WriteFile(path, []byte("a\n"), 0o600)
	v1, _ := s.Version()
	if v1 != version([]byte("a\n")) {
		t.Fatalf("version %q", v1)
	}
	os.WriteFile(path, []byte("bb\n"), 0o600)
	os.Chtimes(path, time.Now(), time.Now().Add(time.Second))
	if v2, _ := s.Version(); v2 != version([]byte("bb\n")) {
		t.Errorf("version after a change %q", v2)
	}
}
```

- [ ] **Шаг 2: Убедиться, что тесты падают**

Запустите: `go test ./internal/ui/`

Ожидается: FAIL: `undefined: NewStore`.

- [ ] **Шаг 3: Реализация**

Запись атомарна: временный файл в том же каталоге, `fsync`, `rename`, `fsync` каталога. Симлинк раскрывается, чтобы конфиг из dotfiles остался ссылкой; ссылка в никуда — ошибка, а не новый файл на её месте.

`internal/ui/store.go`:

```go
package ui

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// ErrConflict means that the file changed since the version a write was
// based on.
var ErrConflict = errors.New("the file changed on disk")

// Store reads and writes the config file. A version is the SHA-256 of the
// file in hex, "" when there is no file.
type Store struct {
	path string // as given; it may be a symbolic link

	mu    sync.Mutex
	stamp stamp  // of the file the cached version is of
	cache string // its version
}

type stamp struct {
	size int64
	mod  time.Time
}

// NewStore returns the store of the config file at path.
func NewStore(path string) *Store {
	return &Store{path: path}
}

// Version returns the version of the file. It reads the file only when its
// size or modification time changed since the last call.
func (s *Store) Version() (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	info, err := os.Stat(s.path)
	if errors.Is(err, fs.ErrNotExist) {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	if st := (stamp{info.Size(), info.ModTime()}); st != s.stamp || s.cache == "" {
		data, err := os.ReadFile(s.path)
		if err != nil {
			return "", err
		}
		s.stamp, s.cache = st, version(data)
	}
	return s.cache, nil
}

// Read returns the file and its version; no file is nil and "".
func (s *Store) Read() ([]byte, string, error) {
	data, err := os.ReadFile(s.path)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, "", nil
	}
	if err != nil {
		return nil, "", err
	}
	return data, version(data), nil
}

// Write replaces the file with data if the file is still at version base.
// It returns the new version, and the backup of the old file: <file>.bak,
// or "" when there was no file. When the config is a symbolic link, the
// file it points at is written, and the link stays. A new file is made with
// mode 0600 in a directory made with mode 0700; an old file keeps its mode.
func (s *Store) Write(base string, data []byte) (ver, backup string, err error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	target, err := s.target()
	if err != nil {
		return "", "", err
	}
	old, err := os.ReadFile(target)
	exists := err == nil
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return "", "", err
	}
	if (exists && version(old) != base) || (!exists && base != "") {
		return "", "", ErrConflict
	}
	mode := fs.FileMode(0o600)
	if exists {
		info, err := os.Stat(target)
		if err != nil {
			return "", "", err
		}
		mode = info.Mode().Perm()
		backup = target + ".bak"
		if err := writeFile(backup, old, mode); err != nil {
			return "", "", err
		}
	} else if err := os.MkdirAll(filepath.Dir(target), 0o700); err != nil {
		return "", "", err
	}
	if err := writeFile(target, data, mode); err != nil {
		return "", "", err
	}
	s.stamp, s.cache = stamp{}, ""
	return version(data), backup, nil
}

// target returns the file the config path names, following symbolic links.
// A missing file is the path itself.
func (s *Store) target() (string, error) {
	t, err := filepath.EvalSymlinks(s.path)
	if errors.Is(err, fs.ErrNotExist) {
		if _, lerr := os.Lstat(s.path); lerr == nil {
			return "", errors.New(s.path + " is a symbolic link to a missing file")
		}
		return s.path, nil
	}
	return t, err
}

// writeFile replaces path with data atomically: a temporary file in the
// same directory, synced, then renamed over path.
func writeFile(path string, data []byte, mode fs.FileMode) error {
	dir := filepath.Dir(path)
	f, err := os.CreateTemp(dir, "."+filepath.Base(path)+".tmp-*")
	if err != nil {
		return err
	}
	tmp := f.Name()
	defer os.Remove(tmp) // after the rename, there is nothing to remove
	if _, err := f.Write(data); err != nil {
		f.Close()
		return err
	}
	if err := f.Chmod(mode); err != nil {
		f.Close()
		return err
	}
	if err := f.Sync(); err != nil {
		f.Close()
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	if err := os.Rename(tmp, path); err != nil {
		return err
	}
	if d, err := os.Open(dir); err == nil {
		d.Sync()
		d.Close()
	}
	return nil
}

func version(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}
```

- [ ] **Шаг 4: Тесты проходят**

Запустите: `test -z "$(gofmt -l .)" && go vet ./... && go test -race ./...`

Ожидается: команда завершается с кодом 0: `gofmt` не нашёл неотформатированных файлов, `go vet` молчит, все пакеты `ok`.

- [ ] **Шаг 5: Commit**

```bash
git add internal/ui
git commit -m "feat(ui): read and write the config file with a backup"
```

### Task 4: API страницы

**Files:**
- Create: `internal/ui/api.go`
- Test: `internal/ui/api_test.go`

**Interfaces:**
- Consumes: `edit.Read`, `edit.Apply`, `edit.Same`, `edit.Errors`, `*edit.ShapeError`; `config.Parse`, `config.Errors`, `config.Tildify`, `(*Config).ModeOf/ProvidersOf`; `ui.Store`, `ui.ErrConflict`.
- Produces:
  - `api{path string; store *Store; log io.Writer}` и `(*api) routes(mux *http.ServeMux)` — задача 5 вешает их за защиту.
  - `GET /api/config` → `fileState{path, exists, version, editable, draft, problems}`; `GET /api/version` → `{"version"}`; `POST /api/validate` с `{version, draft}` → `checkResult{stale, errors[{path,msg}], profiles{name:{mode,providers,subscription}}}`; `POST /api/save` с `{version, draft}` → 200 `{version, draft}`, 409 `{"error"}`, 422 `checkResult`, 500 `{"error"}`.
  - `writeJSON(w, status, v)`, `readJSON(w, r, v) bool`, `errorBody{Error}` — для задачи 5.
  - В тестах: `lockedBuffer` и `testConfig` — их используют тесты задачи 5.

- [ ] **Шаг 1: Написать падающие тесты**

API проверяется на обычном `httptest.Server` без защиты сервера: токен, `Host` и лимит тела — задача 5. `TestTwoTabsSave` закрывает Review Focus 4.

`internal/ui/api_test.go`:

```go
package ui

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"

	"github.com/zinin/graft/internal/edit"
)

const testConfig = `# my config
providers:
  ollama:
    url: https://ollama.com
    key: { env: OLLAMA_API_KEY }
models:
  glm: { provider: ollama, id: glm-5.3-flash, context: 1048576 } # the big one
profiles:
  glm: { model: glm }
  mix: { model: opus, picker: [glm] }
`

// lockedBuffer is a terminal that tests can read while the server writes.
type lockedBuffer struct {
	mu sync.Mutex
	b  bytes.Buffer
}

func (l *lockedBuffer) Write(p []byte) (int, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.b.Write(p)
}

func (l *lockedBuffer) String() string {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.b.String()
}

// testAPI serves the API of a config file in a temporary directory, without
// the guards of Server; content "" means no file.
type testAPI struct {
	srv  *httptest.Server
	path string
	log  *lockedBuffer
}

func startAPI(t *testing.T, content string) *testAPI {
	t.Helper()
	path := filepath.Join(t.TempDir(), "config.yaml")
	if content != "" {
		if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	log := &lockedBuffer{}
	mux := http.NewServeMux()
	(&api{path: path, store: NewStore(path), log: log}).routes(mux)
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return &testAPI{srv, path, log}
}

// call sends a request and decodes the JSON answer into out.
func (s *testAPI) call(t *testing.T, method, path string, body, out any) int {
	t.Helper()
	var r io.Reader
	if body != nil {
		data, _ := json.Marshal(body)
		r = bytes.NewReader(data)
	}
	req, _ := http.NewRequest(method, s.srv.URL+path, r)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if out != nil {
		if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
			t.Fatalf("%s %s: %v", method, path, err)
		}
	}
	return resp.StatusCode
}

func (s *testAPI) load(t *testing.T) fileState {
	t.Helper()
	var st fileState
	if code := s.call(t, "GET", "/api/config", nil, &st); code != 200 {
		t.Fatalf("GET /api/config: %d", code)
	}
	return st
}

func TestConfigOfAFile(t *testing.T) {
	s := startAPI(t, testConfig)
	st := s.load(t)
	if !st.Exists || !st.Editable || st.Version != version([]byte(testConfig)) || st.Draft == nil {
		t.Fatalf("state %+v", st)
	}
	if st.Path != s.path || len(st.Draft.Profiles) != 2 || st.Draft.Models[0].ID != "glm-5.3-flash" {
		t.Errorf("state %+v", st)
	}
}

func TestConfigWithoutAFile(t *testing.T) {
	s := startAPI(t, "")
	st := s.load(t)
	if st.Exists || !st.Editable || st.Version != "" || st.Draft == nil || len(st.Draft.Profiles) != 0 {
		t.Errorf("state %+v", st)
	}
}

func TestConfigTheFormsCannotShow(t *testing.T) {
	s := startAPI(t, "profiles:\n  p: { model: opus, picker: glm }\n")
	st := s.load(t)
	if st.Editable || st.Draft != nil || len(st.Problems) != 1 {
		t.Fatalf("state %+v", st)
	}
	if p := st.Problems[0]; p.Line != 2 || p.Path != "profiles.p.picker" || p.Msg != "must be a list" {
		t.Errorf("problem %+v", p)
	}
}

func TestValidate(t *testing.T) {
	s := startAPI(t, testConfig)
	st := s.load(t)
	var res checkResult
	s.call(t, "POST", "/api/validate", draftRequest{st.Version, *st.Draft}, &res)
	if res.Stale || len(res.Errors) != 0 {
		t.Fatalf("result %+v", res)
	}
	if got := res.Profiles["mix"]; got.Mode != "router" || !got.Subscription || len(got.Providers) != 1 {
		t.Errorf("mix %+v", got)
	}
	d := *st.Draft
	d.Profiles[1].Picker = []string{"glm", "nope"}
	res = checkResult{}
	s.call(t, "POST", "/api/validate", draftRequest{st.Version, d}, &res)
	if len(res.Errors) != 1 || res.Errors[0].Path != "profiles.mix.picker[1]" || len(res.Profiles) != 0 {
		t.Errorf("result %+v", res)
	}
	d.Profiles[1].Settings = "a: ["
	res = checkResult{}
	s.call(t, "POST", "/api/validate", draftRequest{st.Version, d}, &res)
	if len(res.Errors) != 1 || res.Errors[0].Path != "profiles.mix.settings" {
		t.Errorf("result %+v", res)
	}
	res = checkResult{}
	s.call(t, "POST", "/api/validate", draftRequest{"0123", *st.Draft}, &res)
	if !res.Stale {
		t.Errorf("result %+v, want stale", res)
	}
}

func TestSave(t *testing.T) {
	s := startAPI(t, testConfig)
	st := s.load(t)
	d := *st.Draft
	d.Profiles[0].Effort = "high"
	var saved savedFile
	if code := s.call(t, "POST", "/api/save", draftRequest{st.Version, d}, &saved); code != 200 {
		t.Fatalf("save: %d", code)
	}
	data, _ := os.ReadFile(s.path)
	if saved.Version != version(data) || !edit.Same(saved.Draft, d) {
		t.Errorf("saved %+v", saved)
	}
	for _, want := range []string{"# my config", "# the big one", "glm: {model: glm, effort: high}"} {
		if !strings.Contains(string(data), want) {
			t.Errorf("want %q in:\n%s", want, data)
		}
	}
	if backup, _ := os.ReadFile(s.path + ".bak"); string(backup) != testConfig {
		t.Errorf("backup:\n%s", backup)
	}
	if want := "graft ui: saved " + s.path + " (previous version in " + s.path + ".bak)\n"; s.log.String() != want {
		t.Errorf("log %q, want %q", s.log.String(), want)
	}
	// The page saves again from the version it got.
	d = saved.Draft
	d.Profiles[0].Effort = ""
	if code := s.call(t, "POST", "/api/save", draftRequest{saved.Version, d}, &saved); code != 200 {
		t.Errorf("second save: %d", code)
	}
}

func TestSaveRefusals(t *testing.T) {
	s := startAPI(t, testConfig)
	st := s.load(t)
	var body map[string]any
	if code := s.call(t, "POST", "/api/save", draftRequest{"0123", *st.Draft}, &body); code != 409 {
		t.Errorf("stale save: %d %v", code, body)
	}
	d := *st.Draft
	d.Profiles[0].Model = "nope"
	var res checkResult
	if code := s.call(t, "POST", "/api/save", draftRequest{st.Version, d}, &res); code != 422 || len(res.Errors) != 1 {
		t.Errorf("invalid save: %d %+v", code, res)
	}
	if data, _ := os.ReadFile(s.path); string(data) != testConfig {
		t.Errorf("the file changed:\n%s", data)
	}
}

// TestSaveStopsWhenTheFileDiffers: a fault of the merge, which writes
// something else than the page shows, must not reach the disk.
func TestSaveStopsWhenTheFileDiffers(t *testing.T) {
	s := startAPI(t, testConfig)
	st := s.load(t)
	applyDraft = func(data []byte, d edit.Draft) ([]byte, error) {
		d.Profiles = slices.Clone(d.Profiles)
		d.Profiles[0].Effort = "low"
		return edit.Apply(data, d)
	}
	t.Cleanup(func() { applyDraft = edit.Apply })
	var body errorBody
	if code := s.call(t, "POST", "/api/save", draftRequest{st.Version, *st.Draft}, &body); code != 500 {
		t.Fatalf("save: %d %+v", code, body)
	}
	if data, _ := os.ReadFile(s.path); string(data) != testConfig {
		t.Errorf("the file changed:\n%s", data)
	}
	if !strings.Contains(s.log.String(), "did not come out as the page showed it") {
		t.Errorf("log %q", s.log.String())
	}
}

func TestSaveCreatesTheFile(t *testing.T) {
	s := startAPI(t, "")
	st := s.load(t)
	d := *st.Draft
	d.Profiles = append(d.Profiles, edit.Profile{Name: "plain", Model: "opus"})
	var saved savedFile
	if code := s.call(t, "POST", "/api/save", draftRequest{"", d}, &saved); code != 200 {
		t.Fatalf("save: %d", code)
	}
	if data, _ := os.ReadFile(s.path); string(data) != "profiles:\n  plain:\n    model: opus\n" {
		t.Errorf("file:\n%s", data)
	}
	if want := "graft ui: created " + s.path + "\n"; s.log.String() != want {
		t.Errorf("log %q", s.log.String())
	}
}

func TestBadJSON(t *testing.T) {
	s := startAPI(t, testConfig)
	resp, err := http.Post(s.srv.URL+"/api/validate", "application/json", strings.NewReader("{"))
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != 400 {
		t.Errorf("bad JSON: %d", resp.StatusCode)
	}
}

func TestTwoTabsSave(t *testing.T) {
	s := startAPI(t, testConfig)
	st := s.load(t)
	first, second := *st.Draft, *st.Draft
	first.Profiles = slices.Clone(first.Profiles)
	first.Profiles[0].Effort = "high"
	second.Profiles = slices.Clone(second.Profiles)
	second.Profiles[0].Effort = "low"
	if code := s.call(t, "POST", "/api/save", draftRequest{st.Version, first}, nil); code != 200 {
		t.Fatalf("first save: %d", code)
	}
	if code := s.call(t, "POST", "/api/save", draftRequest{st.Version, second}, nil); code != 409 {
		t.Errorf("second save from the same version: %d, want 409", code)
	}
	if data, _ := os.ReadFile(s.path); !strings.Contains(string(data), "effort: high") {
		t.Errorf("file:\n%s", data)
	}
}
```

- [ ] **Шаг 2: Убедиться, что тесты падают**

Запустите: `go test ./internal/ui/`

Ожидается: FAIL: `undefined: api`.

- [ ] **Шаг 3: Реализация**

Проверка и сохранение идут одним путём (`check`): слияние, рендер, `config.Parse`. Сохранение вдобавок читает получившийся текст и сравнивает с присланным черновиком (`edit.Same`): ошибка слияния не должна попасть на диск. Переменная `applyDraft` — это `edit.Apply`; тест `TestSaveStopsWhenTheFileDiffers` подставляет на её место слияние с ошибкой. Пути ошибок без строк — номера строк относятся к тексту, которого страница не видит.

`internal/ui/api.go`:

```go
package ui

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"sync"

	"github.com/zinin/graft/internal/config"
	"github.com/zinin/graft/internal/edit"
)

// applyDraft is edit.Apply; a test puts a fault in its place to see that
// a save stops when the file would differ from the page.
var applyDraft = edit.Apply

// api serves /api/: the draft of the config, its checks and its saves.
type api struct {
	path  string // the config file, absolute
	store *Store
	log   io.Writer // the terminal

	mu sync.Mutex // one save at a time
}

func (a *api) routes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/config", a.config)
	mux.HandleFunc("GET /api/version", a.version)
	mux.HandleFunc("POST /api/validate", a.validate)
	mux.HandleFunc("POST /api/save", a.save)
}

// fileState is the config file as the page opens it.
type fileState struct {
	Path     string      `json:"path"`     // as shown, with ~
	Exists   bool        `json:"exists"`   // false: the page starts from an empty config
	Version  string      `json:"version"`  // "" when the file does not exist
	Editable bool        `json:"editable"` // whether the forms can show the file
	Draft    *edit.Draft `json:"draft"`    // nil when not editable
	Problems []problem   `json:"problems"` // why not editable, at lines of the file
}

// problem is an error of the config: at a line of the file on disk, or at
// a path for a draft, whose lines the page never shows.
type problem struct {
	Line int    `json:"line,omitempty"`
	Path string `json:"path"`
	Msg  string `json:"msg"`
}

// checkResult is what the page shows after a change.
type checkResult struct {
	Stale    bool                    `json:"stale"` // the file changed: nothing was checked
	Errors   []problem               `json:"errors"`
	Profiles map[string]profileState `json:"profiles"` // empty while there are errors
}

type profileState struct {
	Mode         string   `json:"mode"`
	Providers    []string `json:"providers"`
	Subscription bool     `json:"subscription"`
}

// draftRequest is a draft and the version of the file it is based on.
type draftRequest struct {
	Version string     `json:"version"`
	Draft   edit.Draft `json:"draft"`
}

type savedFile struct {
	Version string     `json:"version"`
	Draft   edit.Draft `json:"draft"`
}

type errorBody struct {
	Error string `json:"error"`
}

func (a *api) config(w http.ResponseWriter, r *http.Request) {
	data, ver, err := a.store.Read()
	if err != nil {
		a.fail(w, http.StatusInternalServerError, err)
		return
	}
	st := fileState{Path: config.Tildify(a.path), Exists: ver != "", Version: ver, Problems: []problem{}}
	d, err := edit.Read(data)
	var se *edit.ShapeError
	switch {
	case errors.As(err, &se):
		st.Problems = a.fileProblems(data, se)
	case err != nil:
		a.fail(w, http.StatusInternalServerError, err)
		return
	default:
		st.Editable, st.Draft = true, &d
	}
	writeJSON(w, http.StatusOK, st)
}

// fileProblems lists what is wrong with a file the forms cannot show: the
// errors of config.Parse, which have lines, or the shape problems when
// Parse finds none.
func (a *api) fileProblems(data []byte, se *edit.ShapeError) []problem {
	var out []problem
	var errs config.Errors
	if _, err := config.Parse(data, a.path); errors.As(err, &errs) {
		for _, e := range errs {
			out = append(out, problem{Line: e.Line, Path: e.Path, Msg: e.Msg})
		}
		return out
	}
	for _, p := range se.Problems {
		out = append(out, problem{Line: p.Line, Path: p.Path, Msg: p.Msg})
	}
	return out
}

func (a *api) version(w http.ResponseWriter, r *http.Request) {
	v, err := a.store.Version()
	if err != nil {
		a.fail(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"version": v})
}

func (a *api) validate(w http.ResponseWriter, r *http.Request) {
	var req draftRequest
	if !readJSON(w, r, &req) {
		return
	}
	data, ver, err := a.store.Read()
	if err != nil {
		a.fail(w, http.StatusInternalServerError, err)
		return
	}
	if ver != req.Version {
		writeJSON(w, http.StatusOK, checkResult{Stale: true, Errors: []problem{}, Profiles: map[string]profileState{}})
		return
	}
	_, res := a.check(data, req.Draft)
	writeJSON(w, http.StatusOK, res)
}

func (a *api) save(w http.ResponseWriter, r *http.Request) {
	var req draftRequest
	if !readJSON(w, r, &req) {
		return
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	data, ver, err := a.store.Read()
	if err != nil {
		a.fail(w, http.StatusInternalServerError, err)
		return
	}
	if ver != req.Version {
		a.fail(w, http.StatusConflict, ErrConflict)
		return
	}
	out, res := a.check(data, req.Draft)
	if out == nil {
		writeJSON(w, http.StatusUnprocessableEntity, res)
		return
	}
	// The file must hold what the page shows: a fault in edit must not
	// reach the disk.
	back, err := edit.Read(out)
	if err != nil || !edit.Same(back, req.Draft) {
		fmt.Fprintf(a.log, "graft ui: a save did not come out as the page showed it, so %s stays as it was\n", config.Tildify(a.path))
		a.fail(w, http.StatusInternalServerError, errors.New("graft ui could not apply the changes as shown, so the file is unchanged: reload the page"))
		return
	}
	newVer, backup, err := a.store.Write(ver, out)
	switch {
	case errors.Is(err, ErrConflict):
		a.fail(w, http.StatusConflict, err)
		return
	case err != nil:
		fmt.Fprintf(a.log, "graft ui: cannot save %s: %v\n", config.Tildify(a.path), err)
		a.fail(w, http.StatusInternalServerError, err)
		return
	case backup == "":
		fmt.Fprintf(a.log, "graft ui: created %s\n", config.Tildify(a.path))
	default:
		fmt.Fprintf(a.log, "graft ui: saved %s (previous version in %s)\n", config.Tildify(a.path), config.Tildify(backup))
	}
	writeJSON(w, http.StatusOK, savedFile{Version: newVer, Draft: back})
}

// check applies d to data and checks the result as graft would. It returns
// the new file, nil when d has errors, and what the page shows.
func (a *api) check(data []byte, d edit.Draft) ([]byte, checkResult) {
	res := checkResult{Errors: []problem{}, Profiles: map[string]profileState{}}
	out, err := applyDraft(data, d)
	if err == nil {
		var cfg *config.Config
		if cfg, err = config.Parse(out, a.path); err == nil {
			for _, name := range cfg.ProfileOrder {
				p := cfg.Profiles[name]
				providers := cfg.ProvidersOf(p)
				if providers == nil {
					providers = []string{}
				}
				res.Profiles[name] = profileState{Mode: cfg.ModeOf(p).String(), Providers: providers, Subscription: p.Subscription}
			}
			return out, res
		}
	}
	res.Errors = draftProblems(err)
	return nil, res
}

// draftProblems turns the errors of edit.Apply or config.Parse into
// problems at paths.
func draftProblems(err error) []problem {
	var out []problem
	var ee edit.Errors
	var ce config.Errors
	var se *edit.ShapeError
	switch {
	case errors.As(err, &ee):
		for _, e := range ee {
			out = append(out, problem{Path: e.Path, Msg: e.Msg})
		}
	case errors.As(err, &ce):
		for _, e := range ce {
			out = append(out, problem{Path: e.Path, Msg: e.Msg})
		}
	case errors.As(err, &se):
		for _, p := range se.Problems {
			out = append(out, problem{Path: p.Path, Msg: p.Msg})
		}
	default:
		out = append(out, problem{Msg: err.Error()})
	}
	return out
}

func (a *api) fail(w http.ResponseWriter, status int, err error) {
	writeJSON(w, status, errorBody{err.Error()})
}

// readJSON decodes the body of r into v; on failure it answers 400, or 413
// for a body over the limit, and returns false.
func readJSON(w http.ResponseWriter, r *http.Request, v any) bool {
	err := json.NewDecoder(r.Body).Decode(v)
	if err == nil {
		return true
	}
	status := http.StatusBadRequest
	var tooBig *http.MaxBytesError
	if errors.As(err, &tooBig) {
		status = http.StatusRequestEntityTooLarge
	}
	writeJSON(w, status, errorBody{"graft ui: bad request: " + err.Error()})
	return false
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}
```

- [ ] **Шаг 4: Тесты проходят**

Запустите: `test -z "$(gofmt -l .)" && go vet ./... && go test -race ./...`

Ожидается: команда завершается с кодом 0: `gofmt` не нашёл неотформатированных файлов, `go vet` молчит, все пакеты `ok`.

- [ ] **Шаг 5: Commit**

```bash
git add internal/ui
git commit -m "feat(ui): serve the draft, its checks and its saves"
```

### Task 5: Сервер и его защита

**Files:**
- Create: `internal/ui/server.go`, `internal/ui/peer.go`, `internal/ui/peer_linux.go`, `internal/ui/peer_other.go`, `internal/ui/static/index.html`
- Test: `internal/ui/server_test.go`, `internal/ui/peer_test.go`, `internal/ui/peer_linux_test.go`

**Interfaces:**
- Consumes: `api` и помощники задачи 4.
- Produces:
  - `ui.Options{Path string; Log io.Writer}` (`Log` должен выдерживать одновременную запись), `func ui.New(Options) (*Server, error)`, `(*Server) URL() string` (`http://127.0.0.1:<порт>/#token=<токен>`), `(*Server) Port() int`, `(*Server) Serve() error` (`nil` после `Shutdown`), `(*Server) Shutdown(ctx) error`.
  - `var static embed.FS` — каталог `static/`; страница задач 7–8 кладёт туда файлы.
  - Константы `csp` и `maxBody`.

- [ ] **Шаг 1: Написать падающие тесты**

`internal/ui/server_test.go`:

```go
package ui

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// testServer is a running Server for a config file in a temporary
// directory.
type testServer struct {
	*Server
	log *lockedBuffer
}

func startServer(t *testing.T, content string) *testServer {
	t.Helper()
	path := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	log := &lockedBuffer{}
	s, err := New(Options{Path: path, Log: log})
	if err != nil {
		t.Fatal(err)
	}
	go s.Serve()
	t.Cleanup(func() { s.Shutdown(t.Context()) })
	return &testServer{s, log}
}

func (s *testServer) base() string {
	return strings.Split(s.URL(), "/#")[0]
}

func TestURL(t *testing.T) {
	s := startServer(t, testConfig)
	if !regexp.MustCompile(`^http://127\.0\.0\.1:\d+/#token=[0-9a-f]{64}$`).MatchString(s.URL()) {
		t.Errorf("URL %q", s.URL())
	}
}

func TestGuard(t *testing.T) {
	s := startServer(t, testConfig)
	tests := []struct {
		name   string
		path   string
		host   string // "" is the server's own
		origin string
		auth   string // "" is the right token
		code   int
	}{
		{"page", "/", "", "", "none", 200},
		{"api", "/api/version", "", "", "", 200},
		{"no token", "/api/version", "", "", "none", 401},
		{"wrong token", "/api/version", "", "", "Bearer 0000", 401},
		{"localhost", "/api/version", "localhost:%d", "", "", 200},
		{"rebinding", "/api/version", "evil.example:%d", "", "", 403},
		{"rebinding to the page", "/", "evil.example:%d", "", "none", 403},
		{"other port", "/api/version", "127.0.0.1:1", "", "", 403},
		{"own origin", "/api/version", "", "http://127.0.0.1:%d", "", 200},
		{"other origin", "/api/version", "", "https://evil.example", "", 403},
	}
	for _, tt := range tests {
		req, _ := http.NewRequest("GET", s.base()+tt.path, nil)
		if tt.host != "" {
			req.Host = fmt.Sprintf(tt.host, s.Port())
		}
		if tt.origin != "" {
			req.Header.Set("Origin", fmt.Sprintf(tt.origin, s.Port()))
		}
		switch tt.auth {
		case "":
			req.Header.Set("Authorization", "Bearer "+s.token)
		case "none":
		default:
			req.Header.Set("Authorization", tt.auth)
		}
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()
		if resp.StatusCode != tt.code {
			t.Errorf("%s: %d, want %d", tt.name, resp.StatusCode, tt.code)
		}
		if got := resp.Header.Get("Content-Security-Policy"); !strings.Contains(got, "script-src 'self'") {
			t.Errorf("%s: CSP %q", tt.name, got)
		}
	}
}

func TestHeaders(t *testing.T) {
	s := startServer(t, testConfig)
	get := func(path string, auth bool) *http.Response {
		req, _ := http.NewRequest("GET", s.base()+path, nil)
		if auth {
			req.Header.Set("Authorization", "Bearer "+s.token)
		}
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		io.Copy(io.Discard, resp.Body)
		resp.Body.Close()
		return resp
	}
	page := get("/", false)
	if page.Header.Get("Cache-Control") != "no-cache" || page.Header.Get("X-Content-Type-Options") != "nosniff" ||
		page.Header.Get("Referrer-Policy") != "no-referrer" || !strings.HasPrefix(page.Header.Get("Content-Type"), "text/html") {
		t.Errorf("page headers %v", page.Header)
	}
	if api := get("/api/version", true); api.Header.Get("Cache-Control") != "no-store" {
		t.Errorf("api headers %v", api.Header)
	}
}

func TestBodyLimit(t *testing.T) {
	s := startServer(t, testConfig)
	big := `{"version": "` + strings.Repeat("a", maxBody) + `"}`
	req, _ := http.NewRequest("POST", s.base()+"/api/validate", strings.NewReader(big))
	req.Header.Set("Authorization", "Bearer "+s.token)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != 413 {
		t.Errorf("big body: %d, want 413", resp.StatusCode)
	}
}
```

`internal/ui/peer_test.go`:

```go
package ui

import (
	"errors"
	"fmt"
	"net"
	"strings"
	"testing"
)

const tableHeader = "  sl  local_address rem_address   st tx_queue rx_queue tr tm->when retrnsmt   uid  timeout inode\n"

// tableRow is a line of /proc/net/tcp for a socket owned by uid.
func tableRow(local, remote *net.TCPAddr, uid int) string {
	return fmt.Sprintf("   0: %s %s 01 00000000:00000000 00:00000000 00000000 %5d        0 1 1 0000000000000000 20 4 30 10 -1\n",
		procAddr(local), procAddr(remote), uid)
}

var (
	serverAddr = &net.TCPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 8000}
	clientAddr = &net.TCPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 5000}
)

func TestSocketUID(t *testing.T) {
	table := []byte(tableHeader + tableRow(serverAddr, clientAddr, 1000) + tableRow(clientAddr, serverAddr, 1001))
	if uid, found := socketUID(table, clientAddr, serverAddr); !found || uid != 1001 {
		t.Errorf("client socket: %d, %v", uid, found)
	}
	other := &net.TCPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 5001}
	if _, found := socketUID(table, other, serverAddr); found {
		t.Error("found a socket that is not in the table")
	}
}

func TestProcAddrIsLittleEndianOnThisMachine(t *testing.T) {
	// graft is built for amd64 and arm64, both little-endian.
	if got := procAddr(clientAddr); got != "0100007F:1388" {
		t.Errorf("procAddr = %q", got)
	}
}

type fakeConn struct {
	net.Conn
	closed bool
}

func (c *fakeConn) LocalAddr() net.Addr  { return serverAddr }
func (c *fakeConn) RemoteAddr() net.Addr { return clientAddr }
func (c *fakeConn) Close() error         { c.closed = true; return nil }

type fakeListener struct {
	net.Listener
	conns []*fakeConn
}

func (l *fakeListener) Accept() (net.Conn, error) {
	if len(l.conns) == 0 {
		return nil, errors.New("no more connections")
	}
	c := l.conns[0]
	l.conns = l.conns[1:]
	return c, nil
}

func TestUserListener(t *testing.T) {
	owners := []int{4242, 4242, 0, 1000}
	conns := []*fakeConn{{}, {}, {}, {}}
	log := &lockedBuffer{}
	l := &userListener{
		Listener: &fakeListener{conns: append([]*fakeConn(nil), conns...)},
		uid:      1000,
		table: func() ([]byte, error) {
			uid := owners[0]
			owners = owners[1:]
			return []byte(tableHeader + tableRow(clientAddr, serverAddr, uid)), nil
		},
		log:     log,
		refused: map[int]bool{},
	}
	for _, want := range []*fakeConn{conns[2], conns[3]} {
		c, err := l.Accept()
		if err != nil || c != want {
			t.Fatalf("Accept = %v, %v", c, err)
		}
	}
	if !conns[0].closed || !conns[1].closed || conns[2].closed || conns[3].closed {
		t.Errorf("closed: %v %v %v %v", conns[0].closed, conns[1].closed, conns[2].closed, conns[3].closed)
	}
	if got := log.String(); got != "graft ui: refused a connection from uid 4242\n" {
		t.Errorf("log %q", got)
	}
}

func TestUserListenerPassesAnUnknownSocket(t *testing.T) {
	c := &fakeConn{}
	l := &userListener{
		Listener: &fakeListener{conns: []*fakeConn{c}},
		uid:      1000,
		table:    func() ([]byte, error) { return []byte(tableHeader), nil },
		log:      &strings.Builder{},
		refused:  map[int]bool{},
	}
	if got, err := l.Accept(); got != c || err != nil {
		t.Errorf("Accept = %v, %v", got, err)
	}
}
```

Тест на настоящем `/proc/net/tcp`; суффикс `_linux` собирает его только на Linux. `internal/ui/peer_linux_test.go`:

```go
package ui

import (
	"net"
	"os"
	"testing"
)

// TestUserGuardFindsOwnSocket checks the real /proc/net/tcp: a connection
// from this process is found, belongs to this user, and passes.
func TestUserGuardFindsOwnSocket(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	guarded := userGuard(ln, &lockedBuffer{})
	defer guarded.Close()
	accepted := make(chan net.Conn, 1)
	go func() {
		c, _ := guarded.Accept()
		accepted <- c
	}()
	conn, err := net.Dial("tcp", ln.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	if c := <-accepted; c == nil {
		t.Fatal("own connection refused")
	}
	table, _ := os.ReadFile("/proc/net/tcp")
	uid, found := socketUID(table, conn.LocalAddr().(*net.TCPAddr), conn.RemoteAddr().(*net.TCPAddr))
	if !found || uid != os.Getuid() {
		t.Errorf("socketUID = %d, %v; want %d", uid, found, os.Getuid())
	}
}
```

- [ ] **Шаг 2: Убедиться, что тесты падают**

Запустите: `go test ./internal/ui/`

Ожидается: FAIL: `undefined: New`, `undefined: socketUID`.

- [ ] **Шаг 3: Страница-каркас**

Файлы, на которые она ссылается, появятся в задачах 7–8; встроенной статике нужен хотя бы один файл уже сейчас. Пустая иконка `data:,` избавляет от запроса `/favicon.ico`.

`internal/ui/static/index.html`:

```html
<!doctype html>
<html lang="en">
<head>
  <meta charset="utf-8">
  <meta name="viewport" content="width=device-width, initial-scale=1">
  <title>graft ui</title>
  <link rel="icon" href="data:,">
  <link rel="stylesheet" href="vendor/pico-2.1.1.min.css">
  <link rel="stylesheet" href="style.css">
  <script type="module" src="app.js"></script>
</head>
<body>
  <div id="app"></div>
</body>
</html>
```

- [ ] **Шаг 4: Проверка пользователя соединения**

На Linux URL с токеном попадает в аргументы `xdg-open`, а их видят все пользователи машины. Поэтому слушатель смотрит в `/proc/net/tcp`, чей процесс на том конце, и закрывает соединения чужих пользователей. Свой uid и root проходят; сокет, которого нет в таблице (ретранслятор из другого сетевого пространства), тоже. На macOS ядро не показывает аргументы чужих процессов, и проверка не нужна.

`internal/ui/peer.go`:

```go
package ui

import (
	"encoding/binary"
	"fmt"
	"io"
	"net"
	"strconv"
	"strings"
	"sync"
)

// userListener drops the connections of processes that belong to other
// users of the machine. Root and the user of graft ui pass, and so does a
// connection whose socket the table lacks, such as one relayed from
// another network namespace.
type userListener struct {
	net.Listener
	uid   int                    // the user of graft ui
	table func() ([]byte, error) // the text of /proc/net/tcp
	log   io.Writer

	mu      sync.Mutex
	refused map[int]bool // the users already reported
}

func (l *userListener) Accept() (net.Conn, error) {
	for {
		c, err := l.Listener.Accept()
		if err != nil {
			return nil, err
		}
		uid, found := l.peer(c)
		if !found || uid == l.uid || uid == 0 {
			return c, nil
		}
		c.Close()
		l.mu.Lock()
		if !l.refused[uid] {
			l.refused[uid] = true
			fmt.Fprintf(l.log, "graft ui: refused a connection from uid %d\n", uid)
		}
		l.mu.Unlock()
	}
}

// peer returns the user of the process at the other end of c.
func (l *userListener) peer(c net.Conn) (int, bool) {
	server, ok1 := c.LocalAddr().(*net.TCPAddr)
	client, ok2 := c.RemoteAddr().(*net.TCPAddr)
	if !ok1 || !ok2 {
		return 0, false
	}
	table, err := l.table()
	if err != nil {
		return 0, false
	}
	// The client's socket: its local address is the client, its remote
	// address the server.
	return socketUID(table, client, server)
}

// socketUID finds in table, the text of /proc/net/tcp, the socket with the
// addresses local and remote, and returns the user that owns it.
func socketUID(table []byte, local, remote *net.TCPAddr) (int, bool) {
	l, r := procAddr(local), procAddr(remote)
	if l == "" || r == "" {
		return 0, false
	}
	for _, line := range strings.Split(string(table), "\n") {
		f := strings.Fields(line)
		if len(f) < 8 || f[1] != l || f[2] != r {
			continue
		}
		uid, err := strconv.Atoi(f[7])
		return uid, err == nil
	}
	return 0, false
}

// procAddr writes an IPv4 address as /proc/net/tcp does: the 32 bits of the
// address in the machine's byte order, then the port, both in hex.
func procAddr(a *net.TCPAddr) string {
	ip := a.IP.To4()
	if ip == nil {
		return ""
	}
	return fmt.Sprintf("%08X:%04X", binary.NativeEndian.Uint32(ip), a.Port)
}
```

`internal/ui/peer_linux.go`:

```go
package ui

import (
	"fmt"
	"io"
	"net"
	"os"
)

// userGuard drops the connections of other users' processes. On Linux
// another user can read the URL with the token from the arguments of
// xdg-open in /proc, so the token alone is not enough.
func userGuard(ln net.Listener, log io.Writer) net.Listener {
	table := func() ([]byte, error) { return os.ReadFile("/proc/net/tcp") }
	if _, err := table(); err != nil {
		fmt.Fprintf(log, "graft ui: warning: cannot read /proc/net/tcp, so graft ui cannot refuse other users of this machine who learn its address: %v\n", err)
		return ln
	}
	return &userListener{Listener: ln, uid: os.Getuid(), table: table, log: log, refused: map[int]bool{}}
}
```

`internal/ui/peer_other.go`:

```go
//go:build !linux

package ui

import (
	"io"
	"net"
)

// userGuard returns ln: macOS does not show the arguments of a process to
// other users, so they cannot read the token from the command line of
// open.
func userGuard(ln net.Listener, log io.Writer) net.Listener {
	return ln
}
```

- [ ] **Шаг 5: Сервер**

`guard` пропускает только запросы к самому серверу (`Host` — `127.0.0.1:<порт>` или `localhost:<порт>`: защита от DNS rebinding), отказывает чужому `Origin` и требует токен у `/api/`. Заголовки безопасности стоят на каждом ответе, включая отказы.

`internal/ui/server.go`:

```go
// Package ui serves graft ui: a page in the browser that edits the config
// file, on 127.0.0.1, for the user who started it.
package ui

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"embed"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net"
	"net/http"
	"slices"
	"strings"
	"time"
)

// csp lets the page load only its own files and talk only to its server;
// data: is for the icons of Pico's stylesheet.
const csp = "default-src 'none'; script-src 'self'; style-src 'self'; img-src 'self' data:; " +
	"connect-src 'self'; base-uri 'none'; form-action 'none'; frame-ancestors 'none'"

// maxBody is the largest request body: a config is a few kilobytes.
const maxBody = 1 << 20

//go:embed static
var static embed.FS

// Options configure a Server.
type Options struct {
	Path string    // the config file, absolute
	Log  io.Writer // the terminal: a line for each write and each refused user; safe for concurrent use
}

// Server is the HTTP server of graft ui.
type Server struct {
	http  *http.Server
	ln    net.Listener
	port  int
	token string
}

// New opens the server's socket on 127.0.0.1 at a random port. Serve
// serves it.
func New(opts Options) (*Server, error) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return nil, err
	}
	token, err := newToken()
	if err != nil {
		ln.Close()
		return nil, err
	}
	port := ln.Addr().(*net.TCPAddr).Port
	files, err := fs.Sub(static, "static")
	if err != nil {
		ln.Close()
		return nil, err
	}
	a := &api{path: opts.Path, store: NewStore(opts.Path), log: opts.Log}
	mux := http.NewServeMux()
	a.routes(mux)
	mux.Handle("GET /", http.FileServerFS(files))
	return &Server{
		http:  &http.Server{Handler: guard(mux, port, token), ReadHeaderTimeout: 10 * time.Second},
		ln:    userGuard(ln, opts.Log),
		port:  port,
		token: token,
	}, nil
}

// URL is the address of the page with the token, which the page moves out
// of the address bar.
func (s *Server) URL() string {
	return fmt.Sprintf("http://127.0.0.1:%d/#token=%s", s.port, s.token)
}

// Port is the server's port on 127.0.0.1.
func (s *Server) Port() int { return s.port }

// Serve serves until Shutdown.
func (s *Server) Serve() error {
	if err := s.http.Serve(s.ln); !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	return nil
}

// Shutdown stops the server, waiting for the requests in progress until
// ctx ends.
func (s *Server) Shutdown(ctx context.Context) error {
	return s.http.Shutdown(ctx)
}

func newToken() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

// guard lets through only requests for this server from its own page:
// Host must be 127.0.0.1 or localhost at port, which stops DNS rebinding;
// Origin, when present, must be the page's own; and /api/ needs the token.
// It also sets the headers every response carries.
func guard(next http.Handler, port int, token string) http.Handler {
	hosts := []string{fmt.Sprintf("127.0.0.1:%d", port), fmt.Sprintf("localhost:%d", port)}
	origins := []string{"http://" + hosts[0], "http://" + hosts[1]}
	auth := []byte("Bearer " + token)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := w.Header()
		h.Set("Content-Security-Policy", csp)
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("Referrer-Policy", "no-referrer")
		if !slices.Contains(hosts, r.Host) {
			http.Error(w, "graft ui: this server answers only at "+hosts[0], http.StatusForbidden)
			return
		}
		if o := r.Header.Get("Origin"); o != "" && !slices.Contains(origins, o) {
			http.Error(w, "graft ui: requests from other pages are refused", http.StatusForbidden)
			return
		}
		if !strings.HasPrefix(r.URL.Path, "/api/") {
			h.Set("Cache-Control", "no-cache")
			next.ServeHTTP(w, r)
			return
		}
		h.Set("Cache-Control", "no-store")
		if subtle.ConstantTimeCompare([]byte(r.Header.Get("Authorization")), auth) != 1 {
			writeJSON(w, http.StatusUnauthorized, errorBody{"graft ui: missing or wrong token: open the address that graft ui printed"})
			return
		}
		r.Body = http.MaxBytesReader(w, r.Body, maxBody)
		next.ServeHTTP(w, r)
	})
}
```

- [ ] **Шаг 6: Тесты проходят**

Запустите: `test -z "$(gofmt -l .)" && go vet ./... && go test -race ./...`

Ожидается: команда завершается с кодом 0: `gofmt` не нашёл неотформатированных файлов, `go vet` молчит, все пакеты `ok`.

- [ ] **Шаг 7: Commit**

```bash
git add internal/ui
git commit -m "feat(ui): serve the page on 127.0.0.1 for its own user only"
```

### Task 6: Команда `graft ui`

**Files:**
- Create: `cmd/graft/ui.go`
- Modify: `cmd/graft/main.go` (usage, поля `system`, разбор команды)
- Test: `cmd/graft/ui_test.go`

**Interfaces:**
- Consumes: `ui.New`, `ui.Options`, `(*Server).URL/Port/Serve/Shutdown`; `config.Tildify`; `configPath` из `main.go`.
- Produces: `graft [--config FILE] ui`; поля `system`: `goos string`, `openBrowser func(url string)`, `notify func() (<-chan os.Signal, func())`.

- [ ] **Шаг 1: Написать падающие тесты**

Тесты подменяют браузер, сигналы и терминал; терминал — `syncBuffer`, потому что сервер пишет в него из своих горутин.

`cmd/graft/ui_test.go`:

```go
package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

// syncBuffer is a terminal that a test reads while graft ui writes to it.
type syncBuffer struct {
	mu sync.Mutex
	b  bytes.Buffer
}

func (s *syncBuffer) Write(p []byte) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.b.Write(p)
}

func (s *syncBuffer) String() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.b.String()
}

// uiRun is graft ui running in the background with a fake terminal,
// browser and signals.
type uiRun struct {
	out    *syncBuffer
	stop   chan os.Signal
	done   chan int
	opened []string // read only after done
}

func startUI(t *testing.T, f *fake, args ...string) *uiRun {
	t.Helper()
	r := &uiRun{out: &syncBuffer{}, stop: make(chan os.Signal, 1), done: make(chan int, 1)}
	f.sys.stdout = r.out
	f.sys.notify = func() (<-chan os.Signal, func()) { return r.stop, func() {} }
	f.sys.openBrowser = func(url string) { r.opened = append(r.opened, url) }
	go func() { r.done <- run(append(args, "ui"), f.sys) }()
	return r
}

// url waits for the address that graft ui prints.
func (r *uiRun) url(t *testing.T) string {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		for _, line := range strings.Split(r.out.String(), "\n") {
			if url, ok := strings.CutPrefix(line, "graft ui: open "); ok {
				return url
			}
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("no address in:\n%s", r.out.String())
	return ""
}

// finish stops graft ui and returns its exit code.
func (r *uiRun) finish(t *testing.T) int {
	t.Helper()
	r.stop <- os.Interrupt
	select {
	case code := <-r.done:
		return code
	case <-time.After(5 * time.Second):
		t.Fatal("graft ui did not stop")
		return -1
	}
}

// getConfig calls GET /api/config of the page at url.
func getConfig(t *testing.T, url string) map[string]any {
	t.Helper()
	base, token, _ := strings.Cut(url, "/#token=")
	req, _ := http.NewRequest("GET", base+"/api/config", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var st map[string]any
	json.NewDecoder(resp.Body).Decode(&st)
	return st
}

func TestUI(t *testing.T) {
	f := newFake(t, "DISPLAY=:0")
	f.sys.goos = "linux"
	f.writeConfig(t)
	r := startUI(t, f)
	url := r.url(t)
	if st := getConfig(t, url); st["editable"] != true || st["exists"] != true {
		t.Errorf("config %v", st)
	}
	if code := r.finish(t); code != 0 {
		t.Errorf("code %d", code)
	}
	if len(r.opened) != 1 || r.opened[0] != url {
		t.Errorf("opened %v, want %s", r.opened, url)
	}
	if want := "graft ui: editing ~/.config/graft/config.yaml\n"; !strings.HasPrefix(r.out.String(), want) {
		t.Errorf("stdout %q", r.out.String())
	}
}

func TestUIWithoutConfig(t *testing.T) {
	f := newFake(t)
	r := startUI(t, f, "--config", filepath.Join(t.TempDir(), "config.yaml"))
	if st := getConfig(t, r.url(t)); st["exists"] != false || st["editable"] != true {
		t.Errorf("config %v", st)
	}
	r.finish(t)
	if len(r.opened) != 0 {
		t.Errorf("opened a browser without a display: %v", r.opened)
	}
}

func TestUIOverSSH(t *testing.T) {
	f := newFake(t, "DISPLAY=localhost:10.0", "SSH_CONNECTION=10.0.0.1 50000 10.0.0.2 22")
	f.sys.goos = "linux"
	r := startUI(t, f)
	r.url(t)
	r.finish(t)
	if len(r.opened) != 0 {
		t.Errorf("opened a browser over SSH: %v", r.opened)
	}
	if !strings.Contains(r.out.String(), "graft ui: from another machine: ssh -L ") {
		t.Errorf("stdout %q", r.out.String())
	}
}

func TestUIOnMacOS(t *testing.T) {
	f := newFake(t)
	f.sys.goos = "darwin"
	r := startUI(t, f)
	url := r.url(t)
	r.finish(t)
	if len(r.opened) != 1 || r.opened[0] != url {
		t.Errorf("opened %v", r.opened)
	}
}

func TestUITakesNoArguments(t *testing.T) {
	f := newFake(t)
	if code := run([]string{"ui", "extra"}, f.sys); code != 1 || !strings.Contains(f.stderr.String(), "ui takes no arguments") {
		t.Errorf("code %d, stderr %q", code, f.stderr.String())
	}
}
```

- [ ] **Шаг 2: Убедиться, что тесты падают**

Запустите: `go test ./cmd/graft/`

Ожидается: FAIL: у `system` нет полей `notify`, `openBrowser`, `goos`.

- [ ] **Шаг 3: Подключить команду в `main.go`**

`ui` разбирается до `config.Load`: отсутствующий или сломанный конфиг для редактора не ошибка.

В `cmd/graft/main.go` замените

```go
  graft [--config FILE] check
  graft help | version

Starts Claude Code with the models and settings of PROFILE. The arguments
after PROFILE go to claude unchanged.
```

на

```go
  graft [--config FILE] check
  graft [--config FILE] ui
  graft help | version

Starts Claude Code with the models and settings of PROFILE. The arguments
after PROFILE go to claude unchanged. ui edits the config in a browser.
```

В `cmd/graft/main.go` замените

```go
	"path/filepath"
	"runtime/debug"
```

на

```go
	"path/filepath"
	"runtime"
	"runtime/debug"
```

В `cmd/graft/main.go` замените

```go
	cwd           string
	lookPath      func(string) (string, error)
	exec          func(argv0 string, argv, envv []string) error
	claudeVersion func(path string) (string, error)
}
```

на

```go
	cwd           string
	goos          string
	lookPath      func(string) (string, error)
	exec          func(argv0 string, argv, envv []string) error
	claudeVersion func(path string) (string, error)
	openBrowser   func(url string)
	notify        func() (signals <-chan os.Signal, stop func()) // SIGINT and SIGTERM, for ui
}
```

В `cmd/graft/main.go` замените

```go
		cwd:           cwd,
		lookPath:      exec.LookPath,
		exec:          syscall.Exec,
		claudeVersion: claudeVersion,
	}
```

на

```go
		cwd:           cwd,
		goos:          runtime.GOOS,
		lookPath:      exec.LookPath,
		exec:          syscall.Exec,
		claudeVersion: claudeVersion,
		openBrowser:   openBrowser,
		notify:        notifyStop,
	}
```

В `cmd/graft/main.go` замените

```go
	case len(rest) > 1 && slices.Contains([]string{"help", "version", "list", "check"}, rest[0]):
```

на

```go
	case len(rest) > 1 && slices.Contains([]string{"help", "version", "list", "check", "ui"}, rest[0]):
```

В `cmd/graft/main.go` замените

```go
		fmt.Fprintf(sys.stderr, "graft: %v\n", err)
		return 1
	}
	cfg, err := config.Load(path)
```

на

```go
		fmt.Fprintf(sys.stderr, "graft: %v\n", err)
		return 1
	}
	if rest[0] == "ui" {
		// The editor opens a missing or broken config too.
		return runUI(path, sys)
	}
	cfg, err := config.Load(path)
```

- [ ] **Шаг 4: Команда**

`cmd/graft/ui.go`:

```go
package main

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"runtime"
	"sync"
	"syscall"
	"time"

	"github.com/zinin/graft/internal/config"
	"github.com/zinin/graft/internal/ui"
)

// runUI serves the editor of the config at path until SIGINT or SIGTERM.
func runUI(path string, sys system) int {
	abs, err := filepath.Abs(path)
	if err != nil {
		fmt.Fprintf(sys.stderr, "graft: %v\n", err)
		return 1
	}
	out := &lockedWriter{w: sys.stdout} // the server writes to it too
	srv, err := ui.New(ui.Options{Path: abs, Log: out})
	if err != nil {
		fmt.Fprintf(sys.stderr, "graft: %v\n", err)
		return 1
	}
	signals, stop := sys.notify()
	defer stop()
	served := make(chan error, 1)
	go func() { served <- srv.Serve() }()

	fmt.Fprintf(out, "graft ui: editing %s\n", config.Tildify(abs))
	fmt.Fprintf(out, "graft ui: open %s\n", srv.URL())
	if overSSH(sys) {
		fmt.Fprintf(out, "graft ui: from another machine: ssh -L %d:127.0.0.1:%d <host>, then open the address above\n",
			srv.Port(), srv.Port())
	}
	fmt.Fprintln(out, "graft ui: press Ctrl+C to stop")
	if canBrowse(sys) {
		sys.openBrowser(srv.URL())
	}

	select {
	case <-signals:
	case err := <-served:
		fmt.Fprintf(sys.stderr, "graft: %v\n", err)
		return 1
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	srv.Shutdown(ctx)
	return 0
}

func overSSH(sys system) bool {
	v, _ := sys.lookupEnv("SSH_CONNECTION")
	return v != ""
}

// canBrowse reports whether graft ui opens a browser itself: on macOS, and
// on Linux in a graphical session, but not over SSH.
func canBrowse(sys system) bool {
	if overSSH(sys) {
		return false
	}
	switch sys.goos {
	case "darwin":
		return true
	case "linux":
		x, _ := sys.lookupEnv("DISPLAY")
		wayland, _ := sys.lookupEnv("WAYLAND_DISPLAY")
		return x != "" || wayland != ""
	}
	return false
}

// openBrowser opens url in the default browser. It does not wait, and the
// browser does not write to the terminal; on failure the printed address
// is enough.
func openBrowser(url string) {
	name := "xdg-open"
	if runtime.GOOS == "darwin" {
		name = "open"
	}
	cmd := exec.Command(name, url)
	if cmd.Start() == nil {
		go cmd.Wait()
	}
}

func notifyStop() (<-chan os.Signal, func()) {
	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt, syscall.SIGTERM)
	return c, func() { signal.Stop(c) }
}

// lockedWriter lets the server and runUI write lines to one terminal.
type lockedWriter struct {
	mu sync.Mutex
	w  io.Writer
}

func (l *lockedWriter) Write(p []byte) (int, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.w.Write(p)
}
```

- [ ] **Шаг 5: Тесты проходят**

Запустите: `test -z "$(gofmt -l .)" && go vet ./... && go test -race ./...`

Ожидается: команда завершается с кодом 0: `gofmt` не нашёл неотформатированных файлов, `go vet` молчит, все пакеты `ok`.

- [ ] **Шаг 6: Проверить руками**

Запустите: `go build -o bin/graft ./cmd/graft && bin/graft --config "$(mktemp -d)/config.yaml" ui`

Ожидается: строки `graft ui: editing …/config.yaml`, `graft ui: open http://127.0.0.1:…/#token=…`, `graft ui: press Ctrl+C to stop`; страница пока пустая (задачи 7–8). Ctrl+C завершает команду с кодом 0. Если сессия без дисплея, браузер не открывается — это ожидаемо.

- [ ] **Шаг 7: Commit**

```bash
git add cmd/graft
git commit -m "feat: add graft ui, an editor of the config in the browser"
```

### Task 7: Основа страницы — вендоренные библиотеки и `draft.js`

**Files:**
- Create: `internal/ui/static/vendor/` (бандл htm + Preact, Pico, лицензии), `internal/ui/static/package.json`, `internal/ui/static/draft.js`
- Create: `internal/ui/static_test.go`
- Modify: `Makefile` (цель `test-js`), `.github/workflows/ci.yml` (шаг `node --test`)
- Test: `internal/ui/jstest/draft.test.mjs`

**Interfaces:**
- Consumes: `static` из задачи 5; `version` из задачи 3.
- Produces: модуль `draft.js` без DOM — `ALIASES`, `EFFORTS`, `SECTIONS`, `withKeys(draft)`, `carryKeys(prev, next)`, `sameDraft(a, b)`, `emptyDraft()`, `keysDir(path)`, `newProvider(name, configPath)`, `renameProvider(provider, name, configPath)`, `newModel(name, provider)`, `newProfile(name)`, `newName(entries, base)`, `isClaude(model)`, `subscribed(profile)`, `refsTo(draft, section, name)`, `renameRefs(draft, section, name)`, `setAt(draft, paths, value)`, `usedBy(draft, section, name)`, `locate(draft, path)`, `placeErrors(draft, errors)`, `messagesFor(errors, field)`. Страница задачи 8 импортирует их.

- [ ] **Шаг 1: Скачать и проверить вендоренные файлы**

```bash
mkdir -p internal/ui/static/vendor
cd internal/ui/static/vendor
curl -fsSL -o htm-preact-standalone-3.1.1.module.js https://unpkg.com/htm@3.1.1/preact/standalone.module.js
curl -fsSL -o pico-2.1.1.min.css https://unpkg.com/@picocss/pico@2.1.1/css/pico.min.css
curl -fsSL -o htm.LICENSE https://unpkg.com/htm@3.1.1/LICENSE
curl -fsSL -o preact.LICENSE https://unpkg.com/preact@10.11.3/LICENSE
curl -fsSL -o pico.LICENSE.md https://unpkg.com/@picocss/pico@2.1.1/LICENSE.md
sha256sum htm-preact-standalone-3.1.1.module.js pico-2.1.1.min.css
cd -
```

Ожидается ровно:

```
72284e8e9079c87817145df1110f74e8a2aa040b2fc384922e18dfcb46fc1fd7  htm-preact-standalone-3.1.1.module.js
fbc9a63fc9fc9f72d12fd7fc9806e11fa9f77ae4f9cad146b27003a1119ba3db  pico-2.1.1.min.css
```

Другая сумма — остановитесь и сообщите: файл на unpkg не тот, что проверен. htm распространяется под Apache-2.0, Preact (внутри бандла htm) и Pico — под MIT; тексты лицензий лежат рядом с файлами.

- [ ] **Шаг 2: Тест вендоренных файлов**

`internal/ui/static_test.go`:

```go
package ui

import "testing"

// TestVendoredFiles checks that the vendored files are the releases they
// are named after: htm@3.1.1/preact/standalone.module.js and
// @picocss/pico@2.1.1/css/pico.min.css.
func TestVendoredFiles(t *testing.T) {
	want := map[string]string{
		"static/vendor/htm-preact-standalone-3.1.1.module.js": "72284e8e9079c87817145df1110f74e8a2aa040b2fc384922e18dfcb46fc1fd7",
		"static/vendor/pico-2.1.1.min.css":                    "fbc9a63fc9fc9f72d12fd7fc9806e11fa9f77ae4f9cad146b27003a1119ba3db",
	}
	for name, sum := range want {
		data, err := static.ReadFile(name)
		if err != nil {
			t.Errorf("%s: %v", name, err)
			continue
		}
		if got := version(data); got != sum {
			t.Errorf("%s has sha256 %s, want %s", name, got, sum)
		}
	}
}
```

Запустите: `go test -run TestVendoredFiles ./internal/ui/`

Ожидается: `ok`: суммы совпадают. (Тест пишется после файлов: он проверяет скачанное, а не код.)

- [ ] **Шаг 3: Написать падающие тесты `draft.js`**

Тесты `locate` и `renameRefs` закрывают Review Focus 5: имя с точкой и промежуточное имя, совпавшее с чужим.

`internal/ui/jstest/draft.test.mjs`:

```js
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
```

- [ ] **Шаг 4: Убедиться, что тесты падают**

Запустите: `node --test internal/ui/jstest/draft.test.mjs`

Ожидается: FAIL: `Cannot find module …/static/draft.js`.

- [ ] **Шаг 5: `draft.js` и тип модулей**

`package.json` с `"type": "module"` нужен Node, чтобы читать `draft.js` как модуль ES; браузеру он не мешает.

`internal/ui/static/package.json`:

```json
{
  "private": true,
  "type": "module"
}
```

`internal/ui/static/draft.js`:

```js
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
// aliases of profiles; providers by the provider of models.
export function refsTo(draft, section, name) {
  const out = [];
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

// usedBy names the entries that refer to entry `name` of section.
export function usedBy(draft, section, name) {
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
```

- [ ] **Шаг 6: Тесты проходят**

Запустите: `node --test internal/ui/jstest/draft.test.mjs`

Ожидается: `pass 11`, `fail 0`.

Запустите: `test -z "$(gofmt -l .)" && go vet ./... && go test -race ./...`

Ожидается: команда завершается с кодом 0: `gofmt` не нашёл неотформатированных файлов, `go vet` молчит, все пакеты `ok`.

- [ ] **Шаг 7: Цель `test-js` и шаг CI**

Node на раннерах GitHub уже установлен: `setup-node` не нужен.

В `Makefile` замените

```make
.PHONY: build test vet install dist

build:
	go build -ldflags "-X main.version=$(VERSION)" -o $(BIN) ./cmd/graft

test:
	go test -race ./...
```

на

```make
.PHONY: build test test-js vet install dist

build:
	go build -ldflags "-X main.version=$(VERSION)" -o $(BIN) ./cmd/graft

test:
	go test -race ./...

# test-js tests the logic of the graft ui page; it needs Node.
test-js:
	node --test internal/ui/jstest/draft.test.mjs
```

В `.github/workflows/ci.yml` замените

```yaml
      - run: go vet ./...
      - run: go test -race ./...
```

на

```yaml
      - run: go vet ./...
      - run: go test -race ./...
      - run: node --test internal/ui/jstest/draft.test.mjs
```

Запустите: `make test-js`

Ожидается: те же 11 тестов проходят.

- [ ] **Шаг 8: Commit**

```bash
git add internal/ui Makefile .github/workflows/ci.yml
git commit -m "feat(ui): vendor Preact, htm and Pico, and test the page's logic"
```

### Task 8: Страница редактора

**Files:**
- Create: `internal/ui/static/api.js`, `fields.js`, `forms.js`, `layout.js`, `app.js`, `style.css`
- Create: `internal/ui/jstest/smoke.mjs`
- Modify: `internal/ui/static_test.go` (тест ссылок страницы), `Makefile` (цель `ui-smoke`)

**Interfaces:**
- Consumes: API задачи 4 (`/api/config`, `/api/version`, `/api/validate`, `/api/save`), `draft.js` задачи 7, вендоренные модули.
- Produces: страница по разделу «Интерфейс этапа 1» спеки. `api.js`: `takeToken()`, `ApiError`, `call(method, path, body)`. `fields.js`: `Help`, `Field`, `NameField`, `Select`, `ModelRef`, `YamlField`, `OrderedList`, `EnvEditor`, `KeySource`. `forms.js`: `ProviderForm`, `ModelForm`, `ProfileForm`, `DefaultsForm`. `layout.js`: `TopBar`, `Banner`, `Messages`, `ErrorList`, `Menu`, `Empty`, `NotEditable`. Задача 2 этапа 2 добавит к ним свои компоненты.

- [ ] **Шаг 1: Написать падающий тест ссылок страницы**

Замените `static_test.go` целиком: к проверке вендоренных файлов добавляется `TestPageLoadsItsFiles` — страница и модули ссылаются только на существующие файлы, inline-кода, который запретил бы CSP, нет.

`internal/ui/static_test.go`:

```go
package ui

import (
	"io/fs"
	"regexp"
	"strings"
	"testing"
)

// TestVendoredFiles checks that the vendored files are the releases they
// are named after: htm@3.1.1/preact/standalone.module.js and
// @picocss/pico@2.1.1/css/pico.min.css.
func TestVendoredFiles(t *testing.T) {
	want := map[string]string{
		"static/vendor/htm-preact-standalone-3.1.1.module.js": "72284e8e9079c87817145df1110f74e8a2aa040b2fc384922e18dfcb46fc1fd7",
		"static/vendor/pico-2.1.1.min.css":                    "fbc9a63fc9fc9f72d12fd7fc9806e11fa9f77ae4f9cad146b27003a1119ba3db",
	}
	for name, sum := range want {
		data, err := static.ReadFile(name)
		if err != nil {
			t.Errorf("%s: %v", name, err)
			continue
		}
		if got := version(data); got != sum {
			t.Errorf("%s has sha256 %s, want %s", name, got, sum)
		}
	}
}

// TestPageLoadsItsFiles checks that index.html and the modules load only
// files that exist, and that the page has no inline script or style, which
// the CSP would block.
func TestPageLoadsItsFiles(t *testing.T) {
	page, err := static.ReadFile("static/index.html")
	if err != nil {
		t.Fatal(err)
	}
	for _, m := range regexp.MustCompile(`(?:src|href)="([^"]+)"`).FindAllStringSubmatch(string(page), -1) {
		if strings.HasPrefix(m[1], "data:") {
			continue
		}
		if _, err := static.ReadFile("static/" + m[1]); err != nil {
			t.Errorf("index.html loads %s: %v", m[1], err)
		}
	}
	for _, bad := range []string{"<style", " style=", " onclick=", " onload="} {
		if strings.Contains(string(page), bad) {
			t.Errorf("index.html has %q, which the CSP blocks", bad)
		}
	}
	for _, m := range regexp.MustCompile(`<script[^>]*>`).FindAllString(string(page), -1) {
		if !strings.Contains(m, " src=") {
			t.Errorf("inline script %s", m)
		}
	}
	imports := regexp.MustCompile(`from '\./([^']+)'`)
	modules, _ := fs.Glob(static, "static/*.js")
	if len(modules) == 0 {
		t.Fatal("no modules")
	}
	for _, name := range modules {
		data, _ := static.ReadFile(name)
		for _, m := range imports.FindAllStringSubmatch(string(data), -1) {
			if _, err := static.ReadFile("static/" + m[1]); err != nil {
				t.Errorf("%s imports %s: %v", name, m[1], err)
			}
		}
	}
}
```

- [ ] **Шаг 2: Убедиться, что тест падает**

Запустите: `go test -run TestPageLoadsItsFiles ./internal/ui/`

Ожидается: FAIL: `index.html loads style.css`, `index.html loads app.js`.

- [ ] **Шаг 3: Связь с сервером**

Токен переезжает из адреса в `sessionStorage` вкладки: перезагрузка его не теряет, адресная строка его не показывает.

`internal/ui/static/api.js`:

```js
// api.js talks to the server of graft ui with the token of this tab.

const KEY = 'graft-ui-token';
let token = '';

// takeToken moves the token from the address into the tab's
// sessionStorage, so that the address bar no longer shows it and a reload
// still has it.
export function takeToken() {
  const m = location.hash.match(/token=([0-9a-f]+)/);
  if (m) {
    token = m[1];
    try {
      sessionStorage.setItem(KEY, token);
    } catch {
      // No storage: the token lives until the tab reloads.
    }
    history.replaceState(null, '', location.pathname);
  } else {
    try {
      token = sessionStorage.getItem(KEY) || '';
    } catch {
      token = '';
    }
  }
  return token;
}

// ApiError is an answer other than 2xx, with its status and JSON body.
export class ApiError extends Error {
  constructor(status, body) {
    super(body && body.error ? body.error : `graft ui answered ${status}`);
    this.status = status;
    this.body = body;
  }
}

// call sends a request and returns the JSON answer. It throws ApiError for
// an answer other than 2xx, and TypeError when the server is gone.
export async function call(method, path, body) {
  const headers = { Authorization: `Bearer ${token}` };
  if (body !== undefined) headers['Content-Type'] = 'application/json';
  const resp = await fetch(path, { method, headers, body: body === undefined ? undefined : JSON.stringify(body) });
  let data = null;
  try {
    data = await resp.json();
  } catch {
    // Not JSON: the status says enough.
  }
  if (!resp.ok) throw new ApiError(resp.status, data);
  return data;
}
```

- [ ] **Шаг 4: Поля форм**

`NameField` запоминает места ссылок на запись, когда получает фокус, пока имя принадлежит одной записи; дальше каждая буква меняет именно эти места. `YamlField` раскрывается сам, когда в поле есть текст или ошибка, а сворачивает его только пользователь.

`internal/ui/static/fields.js`:

```js
// fields.js holds the inputs the forms are made of.

import { html, useEffect, useRef, useState } from './vendor/htm-preact-standalone-3.1.1.module.js';
import { ALIASES } from './draft.js';

// Help shows the errors of a field, or its hint when there are none.
export function Help({ errors = [], hint }) {
  if (errors.length > 0) return html`<small class="error">${errors.join('; ')}</small>`;
  return hint ? html`<small>${hint}</small>` : null;
}

const invalid = (errors) => (errors.length > 0 ? 'true' : null);

// Field is a labelled text input.
export function Field({ label, value, onInput, hint, errors = [], placeholder = '', mono = true }) {
  return html`
    <label>
      ${label}
      <input class=${mono ? 'mono' : ''} value=${value} placeholder=${placeholder} aria-invalid=${invalid(errors)}
        onInput=${(e) => onInput(e.target.value)} />
      <${Help} errors=${errors} hint=${hint} />
    </label>`;
}

// NameField edits the name of an entry. The references to the entry that
// renames carries along are taken when the field gets the focus, while the
// name is still the entry's own.
export function NameField({ value, refs, onRename, errors = [], hint, autoFocus }) {
  const taken = useRef([]);
  const input = useRef(null);
  useEffect(() => {
    if (autoFocus && input.current) {
      input.current.focus();
      input.current.select();
    }
  }, [autoFocus]);
  return html`
    <label>
      Name
      <input ref=${input} class="mono" value=${value} aria-invalid=${invalid(errors)}
        onFocus=${() => { taken.current = refs(); }}
        onInput=${(e) => onRename(e.target.value, taken.current)} />
      <${Help} errors=${errors} hint=${hint} />
    </label>`;
}

// Select is a labelled select. A value that is not among options, such as
// an unknown model in the file, is shown as an option of its own.
export function Select({ label, value, options, onChange, hint, errors = [] }) {
  const all = options.some((o) => o.value === value) ? options : [...options, { value, label: value }];
  return html`
    <label>
      ${label}
      <select aria-invalid=${invalid(errors)} onChange=${(e) => onChange(e.target.value)}>
        ${all.map((o) => html`<option value=${o.value} selected=${o.value === value} disabled=${o.disabled}>${o.label}</option>`)}
      </select>
      <${Help} errors=${errors} hint=${hint} />
    </label>`;
}

const CLAUDE_ID = '\u0000claude-id';

// ModelRef chooses a model: one of the config, or a Claude model of the
// subscription. claude is 'aliases' for opus, sonnet, haiku, fable and any
// claude-… ID, or 'ids' for claude-… IDs only, as aliases take. noClaude
// turns the Claude models off.
export function ModelRef({ label, value, models, claude, noClaude, empty, onChange, hint, errors = [] }) {
  const isID = value.toLowerCase().startsWith('claude-');
  const options = [{ value: '', label: empty }, ...models.map((m) => ({ value: m, label: m }))];
  if (claude === 'aliases') {
    for (const a of ALIASES) options.push({ value: a, label: `${a} (Claude)`, disabled: noClaude });
  }
  options.push({ value: CLAUDE_ID, label: 'Claude model ID…', disabled: noClaude });
  return html`
    <div class="ref">
      <${Select} label=${label} value=${isID ? CLAUDE_ID : value} options=${options} errors=${errors} hint=${hint}
        onChange=${(v) => onChange(v === CLAUDE_ID ? 'claude-' : v)} />
      ${isID && html`<input class="mono" value=${value} aria-label=${label + ' ID'} onInput=${(e) => onChange(e.target.value)} />`}
    </div>`;
}

// YamlField edits YAML text in a section that opens when it has text or
// errors, and that only the user closes.
export function YamlField({ label, value, onInput, hint, errors = [] }) {
  const box = useRef(null);
  useEffect(() => {
    if (box.current && (value !== '' || errors.length > 0)) box.current.open = true;
  }, [errors.length]);
  return html`
    <details ref=${box}>
      <summary>${label}</summary>
      <textarea class="mono" rows="6" spellcheck="false" value=${value} aria-invalid=${invalid(errors)}
        onInput=${(e) => onInput(e.target.value)}></textarea>
      <${Help} errors=${errors} hint=${hint} />
    </details>`;
}

// OrderedList edits a list of strings in order: each item has ↑, ↓ and ×.
// With choices, new items come from a select of them; without, a new
// item is an empty text input. errorsAt(i) returns the errors of item i.
export function OrderedList({ label, items, onChange, choices, addLabel, hint, errorsAt = () => [] }) {
  const move = (i, j) => {
    const next = [...items];
    [next[i], next[j]] = [next[j], next[i]];
    onChange(next);
  };
  const [pick, setPick] = useState('');
  const left = choices ? choices.filter((c) => !items.includes(c)) : [];
  return html`
    <fieldset>
      <legend>${label}</legend>
      ${items.map((item, i) => html`
        <div class="row">
          ${choices
            ? html`<span class="mono item" aria-invalid=${invalid(errorsAt(i))}>${item}</span>`
            : html`<input class="mono" value=${item} aria-invalid=${invalid(errorsAt(i))}
                onInput=${(e) => onChange(items.map((x, k) => (k === i ? e.target.value : x)))} />`}
          <button type="button" class="secondary outline" disabled=${i === 0} aria-label="Up" onClick=${() => move(i, i - 1)}>↑</button>
          <button type="button" class="secondary outline" disabled=${i === items.length - 1} aria-label="Down" onClick=${() => move(i, i + 1)}>↓</button>
          <button type="button" class="secondary outline" aria-label="Remove" onClick=${() => onChange(items.filter((_, k) => k !== i))}>×</button>
        </div>
        <${Help} errors=${errorsAt(i)} />`)}
      ${choices
        ? html`<div class="row">
            <select value=${pick} onChange=${(e) => setPick(e.target.value)}>
              <option value="">${left.length ? '— add a model —' : '— no more models —'}</option>
              ${left.map((c) => html`<option value=${c}>${c}</option>`)}
            </select>
            <button type="button" class="secondary" disabled=${pick === ''} onClick=${() => { onChange([...items, pick]); setPick(''); }}>Add</button>
          </div>`
        : html`<button type="button" class="secondary outline" onClick=${() => onChange([...items, ''])}>${addLabel}</button>`}
      <${Help} hint=${hint} />
    </fieldset>`;
}

// EnvEditor edits variables: a name and a value per row. errorsOf(name)
// returns the errors of a variable.
export function EnvEditor({ pairs, onChange, errorsOf }) {
  const set = (i, j, v) => onChange(pairs.map((p, k) => (k !== i ? p : j === 0 ? [v, p[1]] : [p[0], v])));
  return html`
    <fieldset>
      <legend>Env</legend>
      ${pairs.map((p, i) => html`
        <div class="row">
          <input class="mono" placeholder="NAME" value=${p[0]} aria-invalid=${invalid(errorsOf(p[0]))} onInput=${(e) => set(i, 0, e.target.value)} />
          <input class="mono" placeholder="value" value=${p[1]} onInput=${(e) => set(i, 1, e.target.value)} />
          <button type="button" class="secondary outline" aria-label="Remove" onClick=${() => onChange(pairs.filter((_, k) => k !== i))}>×</button>
        </div>
        <${Help} errors=${errorsOf(p[0])} />`)}
      <button type="button" class="secondary outline" onClick=${() => onChange([...pairs, ['', '']])}>+ Variable</button>
      <${Help} hint="Values are strings; an empty value is an empty string." />
    </fieldset>`;
}

const KIND_HINTS = {
  env: 'a variable of the shell that runs graft',
  file: 'a file that holds only the key; only you may read it (chmod 600)',
  command: 'a command that prints the key; it may ask for a password in the terminal',
};
const KIND_PLACEHOLDERS = { env: 'OLLAMA_API_KEY', file: '~/.config/graft/keys/ollama', command: 'pass show ollama' };

// KeySource chooses where graft reads a provider's key. The key itself
// never goes into the config.
export function KeySource({ value, onChange, errors = [] }) {
  return html`
    <fieldset>
      <legend>Key</legend>
      <div role="group">
        ${['env', 'file', 'command'].map((k) => html`
          <button type="button" class=${value.kind === k ? '' : 'secondary outline'} aria-pressed=${value.kind === k}
            onClick=${() => onChange({ kind: k, value: value.kind === k ? value.value : '' })}>${k}</button>`)}
      </div>
      <input class="mono" value=${value.value} placeholder=${KIND_PLACEHOLDERS[value.kind] || ''} disabled=${!value.kind}
        aria-label="Key source" aria-invalid=${invalid(errors)} onInput=${(e) => onChange({ kind: value.kind, value: e.target.value })} />
      <${Help} errors=${errors} hint=${value.kind ? KIND_HINTS[value.kind] : 'Choose where graft reads the key.'} />
    </fieldset>`;
}
```

- [ ] **Шаг 5: Формы**

Подсказки — по таблицам README. Значение по умолчанию подписки страница вычисляет так же, как graft (`subscribed`), а режим и провайдеров профиля берёт из ответа проверки. У нового провайдера ключ читается из файла `keys/<имя>` рядом с конфигом; пока путь не трогали, `renameProvider` ведёт его за именем.

`internal/ui/static/forms.js`:

```js
// forms.js holds the forms of the four kinds of entry. Each form gets the
// entry, the whole draft for the names it offers, and errs(field), which
// returns the messages of the entry's errors about a field ('' for the
// entry itself).

import { html } from './vendor/htm-preact-standalone-3.1.1.module.js';
import { ALIASES, EFFORTS, subscribed, usedBy } from './draft.js';
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
  const noClaude = p.subscription === 'false';
  const tags = info ? [info.mode, ...(info.subscription ? ['subscription'] : []), ...info.providers] : [];
  const subDefault = p.model === '' ? 'default' : `default: ${sub ? 'yes' : 'no'} (${p.model} ${sub ? 'is' : 'is not'} a Claude model)`;
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
```

- [ ] **Шаг 6: Рамка страницы**

`internal/ui/static/layout.js`:

```js
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
```

- [ ] **Шаг 7: Приложение**

Состояние и его правила:
- черновик проверяется через 300 мс после последней правки; ответ `stale` про версию, которую страница сама уже перезаписала, не считается конфликтом;
- версия файла опрашивается каждые 2 с, кроме времени сохранения; без несохранённых правок страница перечитывает файл, с ними — показывает баннер;
- после сохранения записи сохраняют ключи страницы по именам (`carryKeys`), и выбор в меню не сбрасывается;
- Ctrl+S сохраняет, закрытие вкладки с правками спрашивает подтверждение.

`internal/ui/static/app.js`:

```js
// app.js is the page of graft ui: it loads the draft of the config, checks
// each change on the server, saves, and notices changes of the file.

import { html, render, useEffect, useRef, useState } from './vendor/htm-preact-standalone-3.1.1.module.js';
import { ApiError, call, takeToken } from './api.js';
import {
  carryKeys, locate, messagesFor, newModel, newName, newProfile, newProvider, placeErrors, renameProvider,
  renameRefs, sameDraft, setAt, usedBy, withKeys,
} from './draft.js';
import { DefaultsForm, ModelForm, ProfileForm, ProviderForm } from './forms.js';
import { Banner, Empty, ErrorList, Menu, Messages, NotEditable, TopBar } from './layout.js';

const BASES = { providers: 'provider', models: 'model', profiles: 'profile' };
const FORMS = { providers: ProviderForm, models: ModelForm, profiles: ProfileForm };

function App() {
  const [file, setFile] = useState(null); // the config file as /api/config gave it
  const [saved, setSaved] = useState(null); // the draft of the file on disk
  const [draft, setDraft] = useState(null); // the draft the forms edit
  const [check, setCheck] = useState({ errors: [], profiles: {} });
  const [selected, setSelected] = useState(null); // an entry's _k, or 'defaults'
  const [added, setAdded] = useState(null); // the _k of the entry just added
  const [stale, setStale] = useState(false); // the file changed under unsaved edits
  const [banner, setBanner] = useState(null); // 'changed', 'stopped' or null
  const [showErrors, setShowErrors] = useState(false);
  const [toast, setToast] = useState('');
  const [failure, setFailure] = useState('');
  const [saving, setSaving] = useState(false);

  const dirty = draft !== null && saved !== null && !sameDraft(draft, saved);
  // Timers and listeners read the state through now.
  const now = useRef({});
  now.current = { file, draft, dirty, stale, saving, check };

  async function load(note) {
    try {
      const st = await call('GET', '/api/config');
      setFile(st);
      setStale(false);
      setBanner(null);
      if (st.editable) {
        const d = withKeys(st.draft);
        setSaved(structuredClone(d));
        setDraft(d);
      } else {
        setSaved(null);
        setDraft(null);
      }
      if (note) setToast(note);
    } catch (e) {
      setFailure(e instanceof ApiError ? e.message : 'graft ui is not running: start it again and open the address it prints.');
    }
  }

  // changed handles a new version of the file on disk.
  function changed() {
    if (!now.current.dirty) {
      load('Reloaded from disk');
    } else if (!now.current.stale) {
      setStale(true);
      setBanner('changed');
    }
  }

  function trouble(e) {
    if (e instanceof ApiError) setToast(e.message);
    else setBanner('stopped');
  }

  useEffect(() => {
    load();
  }, []);

  // Check the draft 300 ms after its last change.
  useEffect(() => {
    if (!draft || !file) return undefined;
    const t = setTimeout(async () => {
      try {
        const res = await call('POST', '/api/validate', { version: file.version, draft });
        // A check of a version this page has since saved over is stale by
        // its own doing.
        if (res.stale && file.version === now.current.file.version) changed();
        else if (!res.stale) setCheck(res);
      } catch (e) {
        trouble(e);
      }
    }, 300);
    return () => clearTimeout(t);
  }, [draft, file]);

  // Watch the file: every 2 s, ask for its version.
  useEffect(() => {
    const t = setInterval(async () => {
      const f = now.current.file;
      if (!f || now.current.saving) return;
      try {
        const { version } = await call('GET', '/api/version');
        // The page's own save may have changed the version meanwhile.
        if (version !== now.current.file.version && !now.current.saving) changed();
      } catch (e) {
        trouble(e);
      }
    }, 2000);
    return () => clearInterval(t);
  }, []);

  useEffect(() => {
    if (!toast) return undefined;
    const t = setTimeout(() => setToast(''), 3000);
    return () => clearTimeout(t);
  }, [toast]);

  async function save() {
    const { file: f, draft: d, dirty: changedDraft, stale: old, saving: busy } = now.current;
    if (!d || !changedDraft || old || busy) return;
    setSaving(true);
    try {
      const res = await call('POST', '/api/save', { version: f.version, draft: d });
      const next = carryKeys(d, res.draft);
      const saved = { ...f, exists: true, version: res.version };
      now.current.file = saved; // before the next render, for the watch
      setFile(saved);
      setStale(false);
      setBanner(null);
      setSaved(structuredClone(next));
      setDraft(next);
      setToast('Saved');
    } catch (e) {
      if (e instanceof ApiError && e.status === 409) changed();
      else if (e instanceof ApiError && e.status === 422) setCheck({ errors: e.body.errors, profiles: {} });
      else trouble(e);
    } finally {
      setSaving(false);
    }
  }
  const saveRef = useRef(save);
  saveRef.current = save;

  useEffect(() => {
    const onKey = (e) => {
      if ((e.ctrlKey || e.metaKey) && e.key === 's') {
        e.preventDefault();
        saveRef.current();
      }
    };
    const onLeave = (e) => {
      if (now.current.dirty) {
        e.preventDefault();
        e.returnValue = '';
      }
    };
    window.addEventListener('keydown', onKey);
    window.addEventListener('beforeunload', onLeave);
    return () => {
      window.removeEventListener('keydown', onKey);
      window.removeEventListener('beforeunload', onLeave);
    };
  }, []);

  function add(section) {
    const d = structuredClone(draft);
    const name = newName(d[section], BASES[section]);
    const entry = section === 'providers' ? newProvider(name, file.path)
      : section === 'models' ? newModel(name, d.providers.length ? d.providers[0].name : '')
        : newProfile(name);
    d[section].push(entry);
    withKeys(d);
    setDraft(d);
    setSelected(entry._k);
    setAdded(entry._k);
  }

  function update(section, k, entry) {
    setDraft((d) => ({ ...d, [section]: d[section].map((e) => (e._k === k ? entry : e)) }));
  }

  function rename(section, k, name, refs) {
    setDraft((d) => {
      const next = setAt(d, refs, name);
      next[section] = next[section].map((e) => {
        if (e._k !== k) return e;
        return section === 'providers' ? renameProvider(e, name, file.path) : { ...e, name };
      });
      return next;
    });
  }

  function remove(section, k) {
    const entry = draft[section].find((e) => e._k === k);
    const users = usedBy(draft, section, entry.name);
    const what = `${BASES[section]} ${entry.name}`;
    const note = users.length ? `\n\nUsed by ${users.join(', ')}: they will show errors until you change them.` : '';
    if (!window.confirm(`Delete ${what}?${note}`)) return;
    setDraft({ ...draft, [section]: draft[section].filter((e) => e._k !== k) });
    setSelected(null);
  }

  // goTo selects the entry an error path is about.
  function goTo(path) {
    const at = locate(draft, path);
    if (at && at.section === 'defaults') setSelected('defaults');
    else if (at && at.name !== null) setSelected(draft[at.section].find((e) => e.name === at.name)._k);
    setShowErrors(false);
  }

  const placed = draft ? placeErrors(draft, check.errors) : { global: [], sections: {}, entries: {}, counts: {} };

  let main;
  if (failure) {
    main = html`<article class="error">${failure}</article>`;
  } else if (!file) {
    main = html`<p aria-busy="true">Loading…</p>`;
  } else if (!file.editable) {
    main = html`<${NotEditable} path=${file.path} problems=${file.problems} />`;
  } else if (selected === 'defaults') {
    const errors = placed.entries.defaults || [];
    main = html`<${DefaultsForm} entry=${draft.defaults} errs=${(f) => messagesFor(errors, f)}
      update=${(d) => setDraft((cur) => ({ ...cur, defaults: d }))} />`;
  } else {
    let form = null;
    for (const section of ['providers', 'models', 'profiles']) {
      const entry = draft[section].find((e) => e._k === selected);
      if (!entry) continue;
      const errors = placed.entries[`${section}.${entry.name}`] || [];
      const Form = FORMS[section];
      form = html`<${Form} key=${entry._k} entry=${entry} draft=${draft} info=${check.profiles[entry.name]}
        autoFocus=${added === entry._k}
        errs=${(f) => messagesFor(errors, f)}
        update=${(e) => update(section, entry._k, e)}
        refs=${() => renameRefs(now.current.draft, section, entry.name)}
        rename=${(name, refs) => rename(section, entry._k, name, refs)}
        remove=${() => remove(section, entry._k)} />`;
    }
    const empty = draft.providers.length + draft.models.length + draft.profiles.length === 0;
    main = form || (empty
      ? html`<${Empty} path=${file.path} exists=${file.exists} onAdd=${add} />`
      : html`<p class="muted">Choose an entry on the left, or add one with +.</p>`);
  }

  const state = !file ? '' : !file.editable ? 'read only' : dirty ? 'unsaved' : file.exists ? 'saved' : 'not created yet';
  const canSave = dirty && !stale && !saving && check.errors.length === 0;
  return html`
    <${TopBar} path=${file ? file.path : ''} state=${state} errorCount=${check.errors.length} canSave=${canSave}
      onSave=${save} onErrors=${() => setShowErrors(!showErrors)} />
    <${Banner} kind=${banner} onReload=${() => load('Reloaded from disk')} onKeep=${() => setBanner(null)} />
    <${Messages} items=${placed.global} />
    ${showErrors && html`<${ErrorList} errors=${check.errors} onGo=${goTo} onClose=${() => setShowErrors(false)} />`}
    <div class="layout">
      ${draft && html`<${Menu} draft=${draft} selected=${selected} counts=${placed.counts} modes=${check.profiles}
        sectionErrors=${placed.sections} onSelect=${(s, k) => setSelected(k)} onAdd=${add} />`}
      <main class="form">${main}</main>
    </div>
    ${toast && html`<div class="toast" role="status">${toast}</div>`}`;
}

takeToken();
render(html`<${App} />`, document.getElementById('app'));
```

Pico делает `nav ul` горизонтальным; меню переопределяет это. `internal/ui/static/style.css`:

```css
/* graft ui: the frame of the page; Pico styles the forms. */

:root {
  --side: 15rem;
}

body {
  margin: 0;
}

.mono,
code,
textarea {
  font-family: ui-monospace, SFMono-Regular, Menlo, Consolas, monospace;
}

header.top {
  position: sticky;
  top: 0;
  z-index: 2;
  display: flex;
  align-items: center;
  gap: 1rem;
  padding: 0.5rem 1rem;
  background: var(--pico-background-color);
  border-bottom: 1px solid var(--pico-muted-border-color);
}

header.top .path {
  flex: 1;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

header.top button {
  width: auto;
  margin: 0;
  padding: 0.3rem 1rem;
}

.state.unsaved {
  color: var(--pico-del-color);
}

.state.saved {
  color: var(--pico-muted-color);
}

.layout {
  display: grid;
  grid-template-columns: var(--side) 1fr;
  min-height: calc(100vh - 3.5rem);
}

nav.side,
nav.side ul {
  display: block;
}

nav.side {
  padding: 0.5rem 0;
  border-right: 1px solid var(--pico-muted-border-color);
  overflow-y: auto;
}

nav.side .sec {
  display: flex;
  align-items: center;
  gap: 0.5rem;
  padding: 0.75rem 1rem 0.25rem;
  font-size: 0.75rem;
  letter-spacing: 0.05em;
  text-transform: uppercase;
  color: var(--pico-muted-color);
}

nav.side .sec .add {
  width: auto;
  margin: 0 0 0 auto;
  padding: 0 0.5rem;
  line-height: 1.4;
}

nav.side .sec-error {
  display: block;
  padding: 0 1rem;
}

nav.side ul {
  margin: 0;
  padding: 0;
}

nav.side li {
  display: block;
  margin: 0;
  padding: 0;
  list-style: none;
}

nav.side li a,
nav.side .sec a {
  display: flex;
  align-items: center;
  gap: 0.5rem;
  margin: 0;
  padding: 0.2rem 1rem 0.2rem 1.5rem;
  color: inherit;
  text-decoration: none;
}

nav.side .sec a {
  padding: 0;
}

nav.side a.sel {
  background: var(--pico-primary-background);
  color: var(--pico-primary-inverse);
}

.tag {
  padding: 0 0.3rem;
  border: 1px solid currentColor;
  border-radius: 0.3rem;
  font-size: 0.7rem;
  opacity: 0.8;
}

.tag.router {
  color: var(--pico-primary);
}

.badge {
  padding: 0 0.45rem;
  border-radius: 1rem;
  background: var(--pico-del-color);
  color: #fff;
  font-size: 0.7rem;
}

main.form {
  max-width: 52rem;
  padding: 1rem 2rem 4rem;
}

.form-head {
  display: flex;
  align-items: center;
  gap: 0.75rem;
  margin-bottom: 1rem;
}

.form-head h2 {
  flex: 1;
  margin: 0;
  font-size: 1.3rem;
}

.form-head button {
  width: auto;
  margin: 0;
  padding: 0.3rem 1rem;
}

.error,
small.error {
  color: var(--pico-del-color);
}

.entry-errors p {
  margin: 0 0 0.5rem;
}

.banner {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: 1rem;
  padding: 0.5rem 1rem;
}

.banner p {
  margin: 0;
}

.banner button {
  width: auto;
  margin: 0;
  padding: 0.25rem 0.75rem;
}

.banner.warn {
  background: rgba(255, 193, 7, 0.2);
}

.banner.bad {
  flex-direction: column;
  align-items: flex-start;
  gap: 0.25rem;
  background: rgba(220, 53, 69, 0.15);
}

.error-list {
  margin: 1rem;
}

.error-list header {
  display: flex;
  justify-content: space-between;
  align-items: center;
}

.error-list header button {
  width: auto;
  margin: 0;
}

.row {
  display: flex;
  align-items: center;
  gap: 0.5rem;
}

.row > input,
.row > select,
.row > .item {
  flex: 1;
  margin-bottom: 0.25rem;
}

.row > button {
  width: auto;
  margin: 0 0 0.25rem;
  padding: 0.35rem 0.75rem;
}

.ref input {
  margin-top: -0.5rem;
}

.muted {
  color: var(--pico-muted-color);
}

.toast {
  position: fixed;
  right: 1rem;
  bottom: 1rem;
  padding: 0.5rem 1rem;
  border-radius: 0.4rem;
  background: var(--pico-card-background-color);
  box-shadow: var(--pico-card-box-shadow);
}
```

- [ ] **Шаг 8: Тест проходит**

Запустите: `node --check internal/ui/static/app.js && test -z "$(gofmt -l .)" && go vet ./... && go test -race ./...`

Ожидается: синтаксис `app.js` в порядке; команда завершается с кодом 0: `gofmt` не нашёл неотформатированных файлов, `go vet` молчит, все пакеты `ok`.

- [ ] **Шаг 9: Проверка в браузере**

`smoke.mjs` запускает headless Chrome и три раза `graft ui`: на копии `config.example.yaml` (правка, Ctrl+S, переименование, ошибка у поля, баннер после ручной правки), без файла (конфиг с нуля) и на сломанном файле, который потом чинится на диске. Через DevTools Protocol он действует как человек и сверяет файл после каждого сохранения.

`internal/ui/jstest/smoke.mjs`:

```js
// smoke.mjs drives graft ui in headless Chrome through the DevTools
// Protocol, the way a person would, and checks the config file after each
// save. Three runs: edits of a copy of config.example.yaml, a config built
// from nothing, and a broken file fixed by hand. It needs Chrome and Node
// 22 or later, which has WebSocket built in.
//
// usage: node internal/ui/jstest/smoke.mjs bin/graft
// CHROME names the browser when it is not google-chrome.

import { spawn } from 'node:child_process';
import { copyFileSync, mkdtempSync, readFileSync, rmSync, writeFileSync } from 'node:fs';
import { tmpdir } from 'node:os';
import { join } from 'node:path';
import { fileURLToPath } from 'node:url';

const graftBin = process.argv[2];
if (!graftBin) {
  console.error('usage: node smoke.mjs GRAFT_BINARY');
  process.exit(2);
}
const sleep = (ms) => new Promise((r) => setTimeout(r, ms));
const dir = mkdtempSync(join(tmpdir(), 'graft-ui-smoke-'));
const procs = [];
const problems = []; // errors the page logged
let failed = 0;

function finish(code) {
  for (const p of procs) p.kill('SIGTERM');
  rmSync(dir, { recursive: true, force: true });
  process.exit(code);
}

// waitFor polls f until it returns a value other than false, null or
// undefined.
async function waitFor(what, f, ms = 5000) {
  const end = Date.now() + ms;
  for (;;) {
    const v = await f();
    if (v !== false && v !== null && v !== undefined) return v;
    if (Date.now() > end) throw new Error(`timed out waiting for ${what}`);
    await sleep(50);
  }
}

// check reports one expectation; detail shows only when it fails.
function check(name, ok, detail = '') {
  if (ok) {
    console.log(`PASS ${name}`);
    return;
  }
  failed++;
  console.log(`FAIL ${name}${detail ? `: ${detail}` : ''}`);
}

// startUI runs graft ui for the config at path and returns its address. An
// empty DISPLAY keeps graft from opening a browser of its own.
async function startUI(path) {
  const graft = spawn(graftBin, ['--config', path, 'ui'], { env: { ...process.env, DISPLAY: '', WAYLAND_DISPLAY: '' } });
  procs.push(graft);
  let out = '';
  graft.stdout.on('data', (d) => { out += d; });
  graft.stderr.on('data', (d) => { out += d; });
  const url = await waitFor('the address of graft ui', () => (out.match(/graft ui: open (\S+)/) || [])[1]);
  return { url, output: () => out };
}

// connect starts Chrome and returns send, a call of the DevTools Protocol.
async function connect() {
  const profile = join(dir, 'chrome');
  procs.push(spawn(process.env.CHROME || 'google-chrome', [
    '--headless=new', '--disable-gpu', '--no-first-run', '--no-default-browser-check',
    `--user-data-dir=${profile}`, '--remote-debugging-port=0', 'about:blank',
  ], { stdio: 'ignore' }));
  const port = await waitFor('Chrome', () => {
    try {
      return readFileSync(join(profile, 'DevToolsActivePort'), 'utf8').split('\n')[0];
    } catch {
      return null;
    }
  }, 15000);
  const target = await waitFor('a page of Chrome', async () => {
    try {
      return (await (await fetch(`http://127.0.0.1:${port}/json/list`)).json()).find((t) => t.type === 'page');
    } catch {
      return null;
    }
  });
  const ws = new WebSocket(target.webSocketDebuggerUrl);
  await new Promise((r) => ws.addEventListener('open', r));
  let lastId = 0;
  const pending = new Map();
  ws.addEventListener('message', (ev) => {
    const msg = JSON.parse(ev.data);
    if (msg.id && pending.has(msg.id)) {
      pending.get(msg.id)(msg);
      pending.delete(msg.id);
    } else if (msg.method === 'Runtime.exceptionThrown') {
      problems.push(msg.params.exceptionDetails.exception?.description || msg.params.exceptionDetails.text);
    } else if (msg.method === 'Log.entryAdded' && msg.params.entry.level === 'error') {
      problems.push(msg.params.entry.text);
    }
  });
  const send = (method, params = {}) => new Promise((resolve) => {
    const id = ++lastId;
    pending.set(id, resolve);
    ws.send(JSON.stringify({ id, method, params }));
  });
  await send('Runtime.enable');
  await send('Log.enable');
  return { send, close: () => ws.close() };
}

// page runs code in the page, with helpers to find and use its parts.
function pageOf(send) {
  return async (code) => {
    const expression = `(async () => {
      const $ = (s) => document.querySelector(s);
      const $$ = (s) => [...document.querySelectorAll(s)];
      const fire = (el, type) => el.dispatchEvent(new Event(type, { bubbles: true }));
      const click = (el) => el.dispatchEvent(new MouseEvent('click', { bubbles: true }));
      const type = (el, v) => { el.dispatchEvent(new FocusEvent('focus')); el.value = v; fire(el, 'input'); };
      const choose = (el, v) => { el.value = v; fire(el, 'change'); };
      const label = (t) => $$('label').find((l) => l.firstChild && l.firstChild.textContent.trim() === t);
      const entry = (section, name) => [...$$('nav.side ul')[['providers', 'models', 'profiles'].indexOf(section)]
        .querySelectorAll('a')].find((a) => a.querySelector('.mono').textContent === name);
      const button = (text) => $$('button').find((b) => b.textContent.trim() === text);
      const state = () => $('.state') && $('.state').textContent;
      const saveButton = () => $$('header.top button').pop();
      ${code}
    })()`;
    const r = await send('Runtime.evaluate', { expression, awaitPromise: true, returnByValue: true });
    if (r.result.exceptionDetails) throw new Error(r.result.exceptionDetails.exception?.description || r.result.exceptionDetails.text);
    return r.result.result.value;
  };
}

// editExample edits a copy of config.example.yaml.
async function editExample(send, page) {
  const config = join(dir, 'example.yaml');
  copyFileSync(fileURLToPath(new URL('../../../config.example.yaml', import.meta.url)), config);
  const file = () => readFileSync(config, 'utf8');
  const { url } = await startUI(config);
  await send('Page.navigate', { url });
  await waitFor('the menu', () => page(`return $$('nav.side li').length > 0 || null`));

  check('the token leaves the address bar', !(await page(`return location.hash.includes('token')`)));
  const profiles = await page(`return $$('nav.side ul')[2].querySelectorAll('a').length`);
  check('the menu lists the five profiles', profiles === 5, String(profiles));
  check('profiles show their mode', await waitFor('modes', () => page(`return $$('nav.side .tag.router').length === 3 || null`)));
  check('the file reads as saved', (await page(`return state()`)) === 'saved');

  // Change the effort of profile sol and save with Ctrl+S.
  await page(`click(entry('profiles', 'sol'))`);
  await waitFor('the form of sol', () => page(`return label('Effort') ? true : null`));
  await page(`choose(label('Effort').querySelector('select'), 'high')`);
  check('a change reads as unsaved', (await page(`return state()`)) === '● unsaved');
  await waitFor('the check of the change', () => page(`return !saveButton().disabled || null`));
  await page(`window.dispatchEvent(new KeyboardEvent('keydown', { key: 's', ctrlKey: true }))`);
  await waitFor('the save', () => (file().includes('sol: {model: sol, effort: high}') ? true : null));
  check('Ctrl+S saves the change', true);
  check('the comments stay', file().includes('# The whole session on one external model: graft sol'));
  check('the page reads as saved again', await waitFor('saved', () => page(`return state() === 'saved' || null`)));

  // Rename model kimi: the profiles that use it follow.
  await page(`click(entry('models', 'kimi'))`);
  await waitFor('the form of kimi', () => page(`return label('Name') ? true : null`));
  await page(`type(label('Name').querySelector('input'), 'kimi-k3')`);
  await waitFor('the check of the rename', () => page(`return !saveButton().disabled || null`));
  check('the rename keeps the config valid', await page(`return $('header.top .errors') === null`));
  await page(`click(saveButton())`);
  await waitFor('the save of the rename', () => (file().includes('kimi-k3:') ? true : null));
  check('the rename reaches the profiles', /picker: \[sol, glm, kimi-k3\]/.test(file()) && /picker: \[sol, kimi-k3\]/.test(file()));

  // A wrong value: an error at its field, and no saving.
  await page(`click(entry('models', 'kimi-k3'))`);
  await waitFor('the form of kimi-k3', () => page(`return label('Context') ? true : null`));
  await page(`type(label('Context').querySelector('input'), '5')`);
  const error = await waitFor('the error', () => page(`const s = label('Context').querySelector('small.error'); return s && s.textContent`));
  check('the error shows at its field', error.includes('at least 100000'), error);
  check('Save is off while there are errors', await page(`return saveButton().disabled`));

  // The file changes on disk under unsaved changes: the banner.
  writeFileSync(config, file() + '# edited by hand\n');
  await waitFor('the banner', () => page(`return $('.banner.warn') ? true : null`), 6000);
  check('a change on disk shows the banner', true);
  await page(`click(button('Reload'))`);
  check('Reload drops the unsaved change', await waitFor('saved', () => page(`return state() === 'saved' || null`)));
}

// buildFromNothing starts without a config and builds one.
async function buildFromNothing(send, page) {
  const config = join(dir, 'new', 'config.yaml');
  const { url, output } = await startUI(config);
  await send('Page.navigate', { url });
  const title = await waitFor('the empty screen', () => page(`return $('main article h2') && $('main article h2').textContent`));
  check('a missing config shows the empty screen', title === 'No config yet', title);

  await page(`click(button('Add a provider'))`);
  const focused = await waitFor('the new form', () => page(`return document.activeElement && document.activeElement.value`));
  check('a new provider gets a name and the focus', focused === 'provider-1', String(focused));
  await page(`type(label('Name').querySelector('input'), 'ollama')`);
  await page(`type(label('URL').querySelector('input'), 'https://ollama.com')`);
  const keyPath = await page(`return $('input[aria-label="Key source"]').value`);
  check('the default key file follows the name', keyPath.endsWith('/keys/ollama'), keyPath);

  await page(`click($$('nav.side .sec .add')[1])`);
  await waitFor('the model form', () => page(`return label('Context') ? true : null`));
  await page(`type(label('Name').querySelector('input'), 'glm')`);
  await page(`type(label('Context').querySelector('input'), '1048576')`);
  await page(`click($$('nav.side .sec .add')[2])`);
  await waitFor('the profile form', () => page(`return label('Model') ? true : null`));
  await page(`choose(label('Model').querySelector('select'), 'glm')`);
  await waitFor('a valid config', () => page(`return !saveButton().disabled || null`));
  await page(`click(saveButton())`);
  const file = await waitFor('the new file', () => {
    try {
      return readFileSync(config, 'utf8');
    } catch {
      return null;
    }
  });
  check('Save creates the config', /ollama:\s*\n\s*url: https:\/\/ollama\.com\s*\n\s*key: \{file: .*\/keys\/ollama\}/.test(file) &&
    /glm:\s*\n\s*provider: ollama\s*\n\s*context: 1048576/.test(file) && /profile-1:\s*\n\s*model: glm/.test(file), file);
  check('graft ui says it created the file', await waitFor('the line', () => output().includes('graft ui: created ') || null));
}

// fixBrokenFile opens a file the forms cannot show, then fixes it on disk.
async function fixBrokenFile(send, page) {
  const config = join(dir, 'broken.yaml');
  writeFileSync(config, 'providers: [a]\nprofiles:\n  x: { model: opus }\n');
  const { url } = await startUI(config);
  await send('Page.navigate', { url });
  const text = await waitFor('the problem', () => page(`return $('main article') && $('main article').textContent`));
  check('a broken file shows its problem with the line', text.includes('broken.yaml:1') && text.includes('providers: must be a mapping'), text);
  writeFileSync(config, 'profiles:\n  x: { model: opus }\n');
  check('fixing the file brings the forms back', await waitFor('the menu', () => page(`return $$('nav.side li').length === 1 || null`), 6000));
}

try {
  const { send, close } = await connect();
  const page = pageOf(send);
  await editExample(send, page);
  await buildFromNothing(send, page);
  await fixBrokenFile(send, page);
  check('the page logged no errors', problems.length === 0, problems.join(' | '));
  close();
} catch (e) {
  failed++;
  console.log(`FAIL ${e.message}`);
}
finish(failed ? 1 : 0);
```

В `Makefile` замените

```make
.PHONY: build test test-js vet install dist
```

на

```make
.PHONY: build test test-js ui-smoke vet install dist
```

В `Makefile` замените

```make
# test-js tests the logic of the graft ui page; it needs Node.
test-js:
	node --test internal/ui/jstest/draft.test.mjs
```

на

```make
# test-js tests the logic of the graft ui page; it needs Node.
test-js:
	node --test internal/ui/jstest/draft.test.mjs

# ui-smoke drives graft ui in headless Chrome; it needs Chrome and Node 22
# or later. CHROME names the browser when it is not google-chrome.
ui-smoke: build
	node internal/ui/jstest/smoke.mjs $(BIN)
```

Запустите: `make ui-smoke`

Ожидается: 22 строки `PASS` и ни одной другой, последняя — `PASS the page logged no errors`, код выхода 0. Без Chrome или с Node ниже 22 цель не запустится: тогда установите их или сообщите, что проверка пропущена, — не объявляйте задачу выполненной без неё.

- [ ] **Шаг 10: Commit**

```bash
git add internal/ui Makefile
git commit -m "feat(ui): add the editor page"
```

### Task 9: README

**Files:**
- Modify: `README.md` (раздел Web UI, таблица команд, фраза о том, кто пишет конфиг, Security, Limitations)

**Interfaces:**
- Consumes: поведение задач 1–8.
- Produces: документация `graft ui` для пользователей.

- [ ] **Шаг 1: Правки README**

В `README.md` замените

```markdown
In both modes, settings passed with `--settings` override your settings files for this session only. graft never writes to Claude Code's files, to your shell files or to its own config.
```

на

```markdown
In both modes, settings passed with `--settings` override your settings files for this session only. graft never writes to Claude Code's files or to your shell files, and only `graft ui` writes its config, when you save.
```

В `README.md` замените

```markdown
| `graft check` | checks the config, the keys, `claude` and Claude Code's settings files |
```

на

```markdown
| `graft check` | checks the config, the keys, `claude` and Claude Code's settings files |
| `graft ui` | opens an editor of the config in your browser: see [Web UI](#web-ui) |
```

В `README.md` замените

````markdown
## Security
````

на

````markdown
## Web UI

`graft ui` edits the config in your browser. It edits the same file, so you can go on editing it by hand too.

```sh
graft ui
```

graft serves the page on `127.0.0.1` at a random port, prints its address and opens it; Ctrl+C stops it. The address opens the page in any browser on this machine until graft ui stops. Without a config, the page starts from an empty one and creates the file when you save. On a machine you reach over SSH, forward the port that graft prints, with the same number on both ends: `ssh -L 43117:127.0.0.1:43117 HOST`.

- Each change is checked as `graft check` checks the config, and each problem shows at its field. A config with problems cannot be saved.
- Saving rewrites the whole file in one style: the comments stay, but blank lines inside sections, the alignment of comments and the spaces inside `{ }` go. The previous version stays next to the file in `config.yaml.bak`. When the config is a symbolic link, graft writes the file it points to.
- The page looks at the file every 2 seconds. Without unsaved changes, it reloads the file; with them, it says that the file changed and does not save over it.
- Renaming a model or a provider renames it where profiles and models use it.
- Changing a value that YAML aliases (`*x`) share gives each alias a copy of the old value, so that a change of one field never changes another.
- A file that is not one YAML document, a part of the wrong shape, such as a list where a name belongs, and a field that graft does not know can be fixed only in your editor. The page shows the problem and reloads the file when you fix it.

## Security
````

В `README.md` замените

```markdown
Set `CLAUDE_CODE_DISABLE_AGENT_VIEW: "0"` in `env` at your own risk.
```

на

```markdown
Set `CLAUDE_CODE_DISABLE_AGENT_VIEW: "0"` in `env` at your own risk.
- `graft ui` listens on `127.0.0.1` only and serves its API only with the token in the address it prints; it refuses requests for other host names and from other pages. On Linux, other users can read that address in the arguments of `xdg-open`, so it also refuses connections from their processes. Keep the address to yourself: a key source may be a command, so whoever writes the config runs commands as you.
```

В `README.md` замените

```markdown
go on with `graft PROFILE --resume`.
```

на

```markdown
go on with `graft PROFILE --resume`.
- The first save in `graft ui` rewrites the formatting of the whole file: see [Web UI](#web-ui).
```

- [ ] **Шаг 2: Проверить ссылки и тесты**

Запустите: `grep -c "(#web-ui)" README.md && test -z "$(gofmt -l .)" && go vet ./... && go test -race ./...`

Ожидается: `2` (две ссылки на раздел), затем команда завершается с кодом 0: `gofmt` не нашёл неотформатированных файлов, `go vet` молчит, все пакеты `ok`.

- [ ] **Шаг 3: Commit**

```bash
git add README.md
git commit -m "docs: describe graft ui"
```

### Task 10: Приёмка этапа 1 (с владельцем)

Приёмка — в браузере, вместе с владельцем; спека, раздел «Приёмка». Ничего не коммитится.

- [ ] **Шаг 1: Сборка и автоматические проверки.** `make build test test-js ui-smoke` — всё зелёное.
- [ ] **Шаг 2: Копия конфига владельца.** Владелец копирует свой конфиг во временный файл и запускает `bin/graft --config <копия> ui`. Проверить: в меню все провайдеры, модели и профили, у профилей режимы как в `graft list`; поправить по полю в каждом разделе, переименовать модель, которой пользуются профили, удалить модель со ссылками (диалог называет профили) и вернуть её; сохранить; `diff <оригинал> <копия>` — только правки и нормализация форматирования; комментарии на месте; `bin/graft --config <копия> check` проходит; рядом лежит `.bak`.
- [ ] **Шаг 3: Ручная правка.** Без несохранённых правок поправить копию в редакторе — страница перечитала файл и сказала об этом. С несохранёнными правками — баннер, Save не перезаписывает файл. Внести синтаксическую ошибку — формы скрыты, ошибка со строкой; исправить — страница вернулась.
- [ ] **Шаг 4: Защита.** Открыть `http://127.0.0.1:<порт>/` без токена в другом профиле браузера — данные не загружаются. Ctrl+C в терминале — страница показывает «graft ui stopped».
- [ ] **Шаг 5: Живой конфиг.** `bin/graft ui` без `--config`, небольшая настоящая правка, Save, затем `graft check` и `graft list`.
- [ ] **Шаг 6: Решение.** Если владелец принимает этап, дальше — `superpowers:finishing-a-development-branch` и план этапа 2. Если нет — замечания становятся задачами этого плана.
