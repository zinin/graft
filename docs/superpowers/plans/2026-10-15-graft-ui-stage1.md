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

✅ Done — accepted by the owner on 2026-10-15, run in Chrome through the Playwright MCP; no commits (its remarks became Tasks 11–13)

### Task 11: Строка синтаксической ошибки YAML

✅ Done — see commit(s): `65cd3ab`, `6e40cac`, `8fc36d9`

### Task 12: Save и Ctrl+S по одному правилу; страница после остановки сервера

✅ Done — see commit(s): `281bac2`, `84970ab`

### Task 13: Мелочи задач 11–12

✅ Done — see commit(s): `4462a75`, `c55fd2d`
