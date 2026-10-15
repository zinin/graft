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

✅ Done — see commit(s): `8a903f3`, `6e5ab9f`

### Task 2: Правки черновика в файле — `edit.Apply`

✅ Done — see commit(s): `6b4c846`, `294e1aa`, `e75acd9`, `3e832b8`, `6639907`

### Task 3: Файл конфига — `ui.Store`

✅ Done — see commit(s): `5b9a7ba`, `10e874b`

### Task 4: API страницы

✅ Done — see commit(s): `ef33bbc`

### Task 5: Сервер и его защита

✅ Done — see commit(s): `3bad2fe`, `749aa08`, `89c0b98`

### Task 6: Команда `graft ui`

✅ Done — see commit(s): `958bf0a`, `5b64333`

### Task 7: Основа страницы — вендоренные библиотеки и `draft.js`

✅ Done — see commit(s): `b5a13ee`, `d26cb23`, `5ed1f7c`, `3ad3c5e`

### Task 8: Страница редактора

✅ Done — see commit(s): `c0e3077`, `1bface1`, `31514b4`, `2e80123`, `4717d44`

### Task 9: README

✅ Done — see commit(s): `23d0823`

### Task 10: Приёмка этапа 1 (с владельцем)

Приёмка — в браузере, вместе с владельцем; спека, раздел «Приёмка». Ничего не коммитится.

- [ ] **Шаг 1: Сборка и автоматические проверки.** `make build test test-js ui-smoke` — всё зелёное.
- [ ] **Шаг 2: Копия конфига владельца.** Владелец копирует свой конфиг во временный файл и запускает `bin/graft --config <копия> ui`. Проверить: в меню все провайдеры, модели и профили, у профилей режимы как в `graft list`; поправить по полю в каждом разделе, переименовать модель, которой пользуются профили, удалить модель со ссылками (диалог называет профили) и вернуть её; сохранить; `diff <оригинал> <копия>` — только правки и нормализация форматирования; комментарии на месте; `bin/graft --config <копия> check` проходит; рядом лежит `.bak`.
- [ ] **Шаг 3: Ручная правка.** Без несохранённых правок поправить копию в редакторе — страница перечитала файл и сказала об этом. С несохранёнными правками — баннер, Save не перезаписывает файл. Внести синтаксическую ошибку — формы скрыты, ошибка со строкой; исправить — страница вернулась.
- [ ] **Шаг 4: Защита.** Открыть `http://127.0.0.1:<порт>/` без токена в другом профиле браузера — данные не загружаются. Ctrl+C в терминале — страница показывает «graft ui stopped».
- [ ] **Шаг 5: Живой конфиг.** `bin/graft ui` без `--config`, небольшая настоящая правка, Save, затем `graft check` и `graft list`.
- [ ] **Шаг 6: Решение.** Если владелец принимает этап, дальше — `superpowers:finishing-a-development-branch` и план этапа 2. Если нет — замечания становятся задачами этого плана.

### Task 11: Строка синтаксической ошибки YAML

Замечание приёмки. Сломанный YAML — например, `url: [https://ollama.com` без `]` в строке 9 — и страница («The forms cannot show this file»), и CLI (`graft check` и любая команда, которая читает конфиг) показывают как `config.yaml: yaml: line 8: did not find expected ',' or ']'`. Номер на единицу меньше настоящего и стоит внутри текста yaml.v3, а не в начале, как у остальных ошибок конфига (`файл:строка: …`) и как в примере спеки («Особые состояния»: `config.yaml:12: …`).

Причина — `go.yaml.in/yaml/v3` v3.0.5, `decode.go`, `(*parser).fail`: метки строк в yaml.v3 считаются с 0, и `fail` прибавляет 1 только к ошибкам сканера (`yaml_SCANNER_ERROR`). Ошибки парсера (`parserc.go`) выходят с номером от 0.

**Files:**
- Create: `internal/config/yamlerr.go`, `internal/config/yamlerr_test.go`
- Modify: `internal/config/parse.go` (`Parse`), `internal/edit/read.go` (`decode`), `internal/edit/read_test.go` (`TestReadShapeErrors`); `internal/config/config_test.go` — только если его случай `syntax` перестанет проходить

**Интерфейс:**

```go
// YAMLError splits an error of the yaml.v3 decoder into the line of the
// file it is about, counted from 1, and the message without the
// "yaml: line N: " in front of it. line is 0 when the error names no line;
// msg is then the text of err as it is.
func YAMLError(err error) (line int, msg string)
```

Правила:

1. Текст ошибки целиком вида `yaml: line N: MSG` (N — десятичное число) → `line = N`, `msg = MSG`.
2. Если MSG — одно из сообщений парсера yaml.v3, то `line = N + 1`. Сообщения парсера (`parserc.go` v3.0.5, дословно; с сообщениями сканера они не совпадают):
   - `did not find expected <stream-start>`
   - `did not find expected <document start>`
   - `did not find expected node content`
   - `did not find expected '-' indicator`
   - `did not find expected key`
   - `did not find expected ',' or ']'`
   - `did not find expected ',' or '}'`
   - `found duplicate %YAML directive`
   - `found duplicate %TAG directive`
   - `found incompatible YAML document`
   - `found undefined tag handle`

   Комментарий у списка говорит, откуда он и зачем (`(*parser).fail`, счёт строк с 0 у парсера).
3. Любой другой текст → `0`, `err.Error()`.

Использование:

- `config.Parse`: оба места, где ошибка `dec.Decode` становится `Errors{{File: p.file, Msg: err.Error()}}`, берут `line, msg := YAMLError(err)` и дают `Errors{{File: p.file, Line: line, Msg: msg}}`. CLI тогда печатает `graft: ~/.config/graft/config.yaml:9: did not find expected ',' or ']'`.
- `edit.decode`: оба места `&ShapeError{[]Problem{{Msg: err.Error()}}}` дают `Problem{Line: line, Msg: msg}`.
- Страница не меняется: экран «The forms cannot show this file» уже показывает проблему со строкой как `config.yaml:9: …`.

- [ ] **Шаг 1: Как сейчас.** Для каждого случая шага 3 запустить текущий код (например, `bin/graft --config <файл во временном каталоге вне репозитория> check`) и записать в отчёт, что печатает yaml.v3: номер и сообщение.
- [ ] **Шаг 2: Тест `YAMLError` на готовых текстах** (`errors.New`), таблицей: сообщение парсера со строкой → N+1 и сообщение без префикса; сообщение сканера (`found character that cannot start any token`) со строкой → N; `yaml: unknown anchor 'x' referenced` (строки нет) → 0 и текст как есть; сообщение парсера без `line N: ` → 0 и текст как есть; многострочный текст `yaml: unmarshal errors:\n  line 3: …` → 0 и текст как есть. Запустить: тест не компилируется (`undefined: YAMLError`).
- [ ] **Шаг 3: Тест через `Parse`** (`TestParseSyntaxLines`, настоящий yaml.v3): у каждой ошибки `Line` — строка, на которую человеку надо смотреть, а `Msg` — без `yaml: line N: `:
  - незакрытая `[` в строке 3 из 4 (`providers:\n  x:\n    url: [https://a\n    key: {file: k}\n`) → 3, `did not find expected ',' or ']'`;
  - то же с незакрытой `{` → строка `{`, `did not find expected ',' or '}'`;
  - табуляция в начале строки 2 (`profiles:\n\tx: {}\n`) → 2, сообщение сканера;
  - `a: b: c` в строке 2 (`x: 1\na: b: c\n`) → 2, `mapping values are not allowed in this context`.

  Если после исправления номер какого-то случая указывает не на названную строку (`[`, `{`, табуляция, `a: b: c`), остановиться и сообщить, а не подгонять ожидание.
- [ ] **Шаг 4: Реализация** `YAMLError` в `internal/config/yamlerr.go` и её использование в `config.Parse` и `edit.decode`. Тесты шагов 2–3 проходят.
- [ ] **Шаг 5: Старые тесты.** `TestReadShapeErrors` (`{"providers: [\n", "yaml: line"}`) ждёт новый текст: номер по правилу и сообщение без префикса; старое и новое — в отчёт. Случай `syntax` в `internal/config/config_test.go` (`profiles: [`: ошибка в строке 1, yaml.v3 даёт её без номера) остаётся как есть, если проходит.
- [ ] **Шаг 6: Проверки.** `test -z "$(gofmt -l .)"`, `go vet ./...`, `GOOS=darwin go vet ./...`, `go test -race ./...`; `make build`, затем `bin/graft --config <файл со случаем незакрытой [> check` печатает `…:3: did not find expected ',' or ']'`.
- [ ] **Шаг 7: Коммит.** `fix(config): show the line of a YAML syntax error`

### Task 12: Save и Ctrl+S по одному правилу; страница после остановки сервера

Два замечания приёмки:

1. Пока в черновике есть ошибки, кнопка Save выключена, а Ctrl+S всё равно отправляет `POST /api/save`: сервер отвечает 422, файл не меняется, но Chrome пишет в консоль «Failed to load resource: the server responded with a status of 422». Причина: `save()` в `app.js` проверяет `dirty`, `stale` и `saving`, а ошибки проверяет только кнопка (`canSave`).
2. После баннера «graft ui stopped» страница по-прежнему каждые 2 с опрашивает остановленный сервер (консоль копит `net::ERR_CONNECTION_REFUSED`), а Save остаётся активной. Новый `graft ui` поднимается на другом порту и с другим токеном, так что к этой вкладке сервер уже не вернётся.

**Files:**
- Modify: `internal/ui/static/draft.js`, `internal/ui/static/app.js`, `internal/ui/jstest/draft.test.mjs`, `internal/ui/jstest/smoke.mjs`, `docs/superpowers/specs/2026-10-15-graft-ui-design.md`

Требования:

- `draft.js`: `export function canSave({ dirty, stale, saving, stopped, errors })` — `true` только при `dirty`, когда нет ничего из: `stale`, `saving`, `stopped`, `errors > 0` (`errors` — число ошибок последней проверки). Комментарий: это правило кнопки Save, и Ctrl+S следует ему же.
- `app.js`:
  - состояние `stopped`, вначале `false`; `trouble(e)` для ошибки не `ApiError` ставит его вместе с баннером `'stopped'`;
  - `canSave` считается новой функцией (`errors: check.errors.length`) и кладётся в `now.current`; кнопка Save берёт его, как сейчас; `save()` сразу выходит, если `!now.current.canSave`, так что Ctrl+S делает ровно то, что кнопка;
  - при `stopped` страница больше не шлёт запросов: опрос версии и проверка черновика через 300 мс не вызывают `call`. Формы остаются редактируемыми: баннер говорит, что правки остались только в этой вкладке, и их можно скопировать;
  - остальное поведение не меняется.
- Спека:
  - «Сохранение»: после «Кнопка неактивна, пока правок нет или есть ошибки.» дописать «Ctrl+S действует, только когда кнопка активна.»;
  - «Ручные правки при открытом интерфейсе», пункт об остановке: после «баннер «graft ui stopped, your unsaved changes are only in this tab».» дописать «Больше страница к серверу не обращается, а Save выключена.».

- [ ] **Шаг 1: Node-тест** `canSave follows the Save button` в `draft.test.mjs`: `true` для `{ dirty: true, stale: false, saving: false, stopped: false, errors: 0 }`; `false`, когда `dirty` ложно; `false` для каждого из `stale`, `saving`, `stopped`, `errors: 1` по отдельности. Запустить: падает (нет экспорта `canSave`).
- [ ] **Шаг 2: Проверки smoke** (`smoke.mjs`), до правки `app.js`:
  - в `editExample` сразу после проверки 'Save is off while there are errors': запомнить текст файла и `problems.length`, отправить Ctrl+S тем же `window.dispatchEvent(new KeyboardEvent('keydown', { key: 's', ctrlKey: true }))`, что в проверке 'Ctrl+S saves the change', подождать 700 мс и проверить `'Ctrl+S saves nothing while there are errors'`: файл и `problems.length` не изменились;
  - новый сценарий `stopServer(send, page)` — после проверки 'the page logged no errors': запрос, по которому страница узнаёт об остановке, тоже пишет в консоль `ERR_CONNECTION_REFUSED`, поэтому раньше этой проверки сценарий стоять не может. Свой `graft ui` на копии `config.example.yaml`, страница, меню, несохранённая правка (например, effort профиля); дождаться, пока Save включится (проверка правки вернулась), затем SIGINT этому `graft ui` и проверка `'a stopped server shows the banner'` (`.banner.bad` за 6 с). Затем запомнить `problems.length`, подождать 4500 мс (два опроса) и проверить `'the page stops asking a stopped server, and Save is off'`: `problems.length` не изменился, кнопка Save выключена;
  - `startUI` возвращает и процесс (`{ url, output, proc }`), чтобы сценарий остановил именно его; `finish()` по-прежнему гасит все процессы.

  `make ui-smoke` (с `TMPDIR` по умолчанию: с длинным путём Chrome не успевает стартовать): новые проверки 'Ctrl+S saves nothing while there are errors' и 'the page stops asking a stopped server, and Save is off' падают, остальные проходят. Вывод — в отчёт.
- [ ] **Шаг 3: Реализация** в `draft.js` и `app.js`. `node --test internal/ui/jstest/draft.test.mjs` — всё проходит; `make ui-smoke` — 25 из 25.
- [ ] **Шаг 4: Спека** — две фразы выше.
- [ ] **Шаг 5: Проверки.** `node --test internal/ui/jstest/draft.test.mjs`, `make ui-smoke`, `go test -race ./internal/ui/`.
- [ ] **Шаг 6: Коммиты.** `fix(ui): save on Ctrl+S only when Save is on, and stop asking a stopped server` (код и тесты), `docs: say what graft ui does once its server stops` (спека).

### Task 13: Мелочи задач 11–12

Владелец принял этап и решил одной задачей исправить дешёвые мелочи, которые ревью задач 11 и 12 отложили. Поведение страницы и CLI не меняется; баннер остановки держится на состоянии `stopped`, а тесты закрепляют больше случаев.

**Files:**
- Modify: `internal/config/yamlerr.go` (комментарий), `internal/config/yamlerr_test.go` (`TestParseSyntaxLines`), `internal/ui/static/app.js`, `internal/ui/jstest/smoke.mjs`

Изменения:

1. `yamlerr.go`, комментарий о подсчёте строк: сейчас он говорит «(is_break and skip_line in scannerc.go)», а `is_break` лежит в `yamlprivateh.go`; `skip_line` и `read_line` — в `scannerc.go`. Указать каждое на своём месте.
2. `TestParseSyntaxLines`, новые строки с настоящим yaml.v3; номер — строка, на которую надо смотреть (у `'-' indicator` — начало списка: так выбирает yaml.v3, и строка это закрепляет):
   - CRLF: `"providers:\r\n  x:\r\n    url: [https://a\r\n    key: {file: k}\r\n"` → 3, `did not find expected ',' or ']'`;
   - кириллица и эмодзи до ошибки, как в `TestApplyUnicodeAndCRLF` (UTF-8 прямо в строке): `"# комментарий 😀\nmodels:\n  m:\n    label: Кими 🚀\n    id: [a\n    context: 200000\n"` → 5, `did not find expected ',' or ']'`;
   - ошибка сканера в конце текста: `"a: \"unterminated\n"` → 1, `found unexpected end of stream`;
   - `"top:\n  - a\n  b: 1\n"` → 2, `did not find expected '-' indicator`;
   - `"\"a\"\nb\n"` → 2, `did not find expected <document start>`.

   Строки закрепляют уже верное поведение и проходят сразу. Если номер или сообщение какой-то строки другие, остановиться и сообщить, а не подгонять ожидание.
3. `app.js`: из `now.current` убрать `check` — его никто не читает.
4. `app.js`: баннер остановки берётся из `stopped`: `<Banner kind=${stopped ? 'stopped' : banner} …>`; `trouble()` ставит только `stopped`; комментарий у `banner` — «'changed' or null». Тогда ни один поздний `setBanner` не спрячет остановку.
5. `smoke.mjs`:
   - два ожидания с именем 'the check of the change' (в `editExample` перед Ctrl+S и в `stopServer`) ждут только, пока включится Save: переименовать в 'Save to turn on';
   - в `stopServer` после этого ожидания `await sleep(700)` с комментарием, как у проверки переименования в `editExample`: проверка правки отвечает через 300 мс после неё, и остановка приходит уже после ответа;
   - в `stopServer` после баннера и запомненного `problems.length`, до ожидания 4500 мс, отправить Ctrl+S тем же `window.dispatchEvent(new KeyboardEvent('keydown', { key: 's', ctrlKey: true }))`; проверка называется 'the page stops asking a stopped server, and Save and Ctrl+S are off'. Проверок по-прежнему 25.

- [ ] **Шаг 1: Строки теста** (п. 2). `go test -race -count=1 ./internal/config/ -run TestParseSyntaxLines` — проходит; вывод с `-v` для новых строк — в отчёт.
- [ ] **Шаг 2: Комментарий** (п. 1).
- [ ] **Шаг 3: Smoke** (п. 5). Проверка мутацией: если временно убрать `stopped` из правила в `canSave` (`draft.js`), новая часть проверки 'the page stops asking a stopped server, and Save and Ctrl+S are off' падает; вывод — в отчёт, мутацию вернуть.
- [ ] **Шаг 4: `app.js`** (п. 3 и 4).
- [ ] **Шаг 5: Проверки.** `test -z "$(gofmt -l .)"`, `go vet ./...`, `GOOS=darwin go vet ./...`, `go test -race ./...`, `node --test internal/ui/jstest/draft.test.mjs` (21 из 21), `make ui-smoke` (25 из 25, с `TMPDIR` по умолчанию).
- [ ] **Шаг 6: Коммиты.** `test(config): pin the lines of more YAML errors` (п. 1 и 2), `fix(ui): show the stop banner from the stopped state` (п. 3, 4 и 5).
