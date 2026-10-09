# Linux Setup parity → main: handoff для коммита и переноса

Дата подготовки: 2026-10-09. Целевой репозиторий: `benice2me11/codexify-go`, ветка `main`.

## Решение

**Не сливать `codex/linux-setup-parity-20261009` непосредственно в `main`.**
Сделать новую интеграционную ветку от актуальной `upstream/main` и перенести исправления **Setup / MCP UI / контекста разговора** выборочно. Изменения жизненного цикла туннеля и Windows cutover оставить для отдельных решений/PR. Это предотвращает случайное удаление Windows-отчётов и перезапись уже принятого диагностического кода из `main`.

**Граница выполнения этого handoff:** подготовить чистую интеграционную ветку, коммиты и проверенный PR в `main`. Установка нового бинарника, изменение systemd unit, конфигурации или rollback-снимка в задачу Git-слияния **не входят**.

## Проверенная база и состояние

- `upstream/main` = `e3e9ca1353516daefdc60bdc44315e95a190ca1b` на момент проверки.
- Источник Linux-кандидата: `codex/linux-setup-parity-20261009` = `3fee7be82a8436a32bb9ccace11261c9be59c0a0` **до добавления настоящего handoff**; при работе использовать свежий HEAD ветки.
- Испытанный исходный код кандидата: `b37ec0ed7716bfb70b33b7407f4470ec2e2ec6f6`. Следующие за ним коммиты в основном документируют Linux-проверки и установку.
- По графу Git: `main` имеет 1 уникальный коммит, кандидат — 33 уникальных (до этого handoff). По фактическому `git diff` между вершинами отличаются **56 файлов** — меньше, чем список изменений относительно общего предка.
- Linux source checkout `/home/whtvr/codexify-go` (локальная `main`) на момент диагностики чистый и отстаёт от удалённого состояния. Не обновлять его `git pull` ради этого переноса.
- Отдельный тестовый worktree `.codexify-go/worktrees/linux-setup-parity-test-20261009` чистый, содержит проверенный код `b37ec0e`; его локальная ветка может отставать от удалённой по документационным коммитам.
- На Linux: `origin = git@github.com:1AAk/codexify-go.git`; **`upstream = https://github.com/benice2me11/codexify-go.git`**. Целевой `main` именно в `upstream`. Не пушить в `origin/main`.

## Реальные конфликты

Выполнен безопасный `git merge-tree --write-tree --messages upstream/main upstream/codex/linux-setup-parity-20261009`. Код возврата 1, **12 конфликтов**. Проверка не изменяла текущие checkout, индекс или ветки. Сводка сохранена локально в игнорируемом `.codexify-go/main-merge-analysis/merge-tree.txt`.

| Группа | Точные пути | Конфликт | Приоритет |
| --- | --- | --- | --- |
| Windows отчёты (6) | `docs/handoffs/WINDOWS_CUTOVER_V2_ACCEPTANCE_20260930.md`; `WINDOWS_CUTOVER_V2_G00_G01_20261001.md`; `WINDOWS_G02_BINDING_AND_GUARD_20261001.md`; `WINDOWS_G02_LIVE_REPORT_20261002.md`; `WINDOWS_G02_READINESS_CRITERIA_20261001.md`; `WINDOWS_G03_CLIENT_REPORT_20261002.md` (все под `docs/handoffs/`) | add/add | Сохранить `main`, не объединять в Linux Setup PR |
| Windows план (1) | `docs/superpowers/plans/2026-10-01-windows-g02-preparation.md` | add/add | Сохранить `main`, рассмотреть отдельно |
| MCP диагностика (2) | `internal/mcpdiag/recorder.go`; `internal/mcpdiag/recorder_test.go` | add/add | Сохранить базу `main`; отдельно изучить кандидатные изменения, ничего не выбрасывать без ревью |
| MCP setup (1) | `internal/mcpserver/server.go` | content | `main` — основа, вручную интегрировать Setup и его middleware из кандидата |
| Setup UI (1) | `internal/ui/resources.go` | content | Перенести `v5` + `v1/v2` aliases + обработку ошибок/контекста; не терять существующие компоненты `main` |
| UI harness (1) | `internal/ui/widget_runtime_test.mjs` | add/add | Объединить оба набора тестов, не принимать усечённую версию |

Важно: `git diff upstream/main upstream/candidate` также выявляет **удаления** Windows-отчётов и evidence в кандидате. Эти удаления не переносить автоматически. Простой `git merge`, `git checkout --theirs .` или замена целого каталога `docs` недопустимы.

## Обязательный объём переноса — Setup / MCP UI

Интегрировать как один логический комплект со всеми тестами:

1. `internal/mcpserver/setup_context.go` и `setup_context_test.go`: защищённый контекст для app-only вызовов, срок жизни/изоляция разных разговоров, отсутствие утечки в model-visible результат и запрет использования в обычных файловых инструментах.
2. `internal/mcpserver/server.go`: модельный инструмент `setup`, регистрация receiving middleware, передача `uiContext`, единая функция `setupStatus`, корректная публикация `list_projects`/`set_project_root` и совместимые ресурсы старых карточек. **Не заменять файл целиком версией кандидата**: сохранить текущие диагностические и другие изменения `main`.
3. `internal/projects/catalog.go`: поля `Workspace`, `Selected`, `AwaitingSelection` в `ListOutput`, нужные для пассивной отрисовки Setup без нового запроса.
4. `internal/ui/resources.go`: Setup `v5`, aliases `v1` и `v2`, осмысленный вывод MCP `isError` / ошибок-объектов, обработка `openai:set_globals` без циклических вызовов, защита от одновременных кликов, обновление `uiContext`.
5. `internal/mcpserver/server_test.go` и `internal/ui/widget_runtime_test.mjs`: тесты каталога инструментов, старых resource URI, отказов переключения, контекстов двух разговоров, ошибок `[object Object]` и отсутствия бесконечных повторных вызовов. Сохранить также тесты из `main`.

`internal/projects/manager.go` и `internal/tunnel/runtime.go` между вершинами содержательно совпадают; переписывать их не требуется. Проверить итоговый diff против свежего `main` после интеграции.

## Отдельный объём — НЕ включать незаметно в Setup PR

- **Tunnel PollWatcher:** `internal/tunnel/pollwatch.go` + тест, `internal/tunnel/factory.go`, `internal/app/app.go`, `internal/health/composite.go`, конфигурационные поля в `internal/config/config.go`. Это отдельная логика перезапуска туннеля; требует собственного review, проверки дефолтов и recovery/soak. Тесты Setup не доказывают корректность этой подсистемы.
- **MCP diagnostics:** различия `internal/mcpdiag/recorder*` имеют самостоятельный жизненный цикл. Проверять отдельно с сохранением уже принятой реализации `main`.
- **Windows cutover**: `tools/windows/cutover-v2/**`, исторические отчёты, evidence и планы — вне Linux Setup PR. Никаких массовых удалений этих файлов.
- **Локальные артефакты:** `.codexify-go/**`, бинарники, приватные rollback-копии, `config.json` с реквизитами, credentials, tunnel logs, временные файлы и реальные ID токенов — никогда не добавлять в коммит.

## Рекомендуемый Git-процесс

Из Linux-корня репозитория; **не выполнять merge на рабочей `main`**:

```bash
cd /home/whtvr/codexify-go
git status --short --branch
git fetch --no-tags upstream \
  +refs/heads/main:refs/remotes/upstream/main \
  +refs/heads/codex/linux-setup-parity-20261009:refs/remotes/upstream/codex/linux-setup-parity-20261009
git check-ignore -v .codexify-go/worktrees
git worktree list --porcelain
```

Если путь/ветка интеграции ещё отсутствуют:

```bash
git worktree add -b codex/linux-setup-main-integration-20261009 \
  .codexify-go/worktrees/linux-setup-main-integration-20261009 \
  upstream/main
cd .codexify-go/worktrees/linux-setup-main-integration-20261009
```

Не делать `git merge candidate` целиком. Переносить новые Setup-файлы через выборочный `git restore --source=<candidate-ref> -- <path>`, а существующие конфликтующие файлы `server.go`, `resources.go` и `widget_runtime_test.mjs` — отдельными точечными патчами. Сначала тесты, затем реализация. Проверять `git diff upstream/main -- ...` после каждой группы; не выбирать «ours/theirs» для конфликтов без анализа.

Рекомендуемая логическая разбивка:

1. `feat(mcp): restore setup tool and conversation-scoped UI context` — server, context store, catalog, тесты изоляции;
2. `fix(ui): make Setup v5 stable and preserve legacy v1/v2 cards` — UI и regression harness;
3. `docs(linux): record Setup acceptance and safe mainline integration` — handoff и evidence, без секретов.

Перед каждым коммитом:

```bash
git status --short
git diff --check
git diff --cached --check
git diff --cached --stat
git diff --cached
```

Использовать **явное `git add` по выбранным файлам** — без `git add -A`, пока не проверен состав. Если создан worktree из `upstream/main`, то коммиты Setup должны содержать минимальный diff, а не повторно добавлять всю Windows ветку. В PR приложить ссылки на выполненные тесты и результаты Linux UI.

После приёмки:

```bash
git push -u upstream codex/linux-setup-main-integration-20261009
```

Открыть PR **из интеграционной ветки в `benice2me11/codexify-go:main`**. Merge только после review/CI и выполнения критериев ниже. Не пушить force, не изменять установленный systemd unit для проверки Git-мерджа.

## Критерии приёмки

- [ ] На интеграционной ветке нет неожиданных удалений Windows-документации, логов/схем диагностики, пользовательских данных и исходников Linux systemd.
- [ ] `git diff --check` без замечаний, рабочее дерево чистое, финальная `git diff upstream/main...HEAD` содержит только одобренный scope.
- [ ] `go test -count=1 ./...` — PASS.
- [ ] `go vet ./...` — PASS.
- [ ] `go test -race -count=1 ./internal/mcpserver ./internal/projects ./internal/ui` — PASS.
- [ ] `node internal/ui/widget_runtime_test.mjs --source internal/ui/resources.go` — PASS, нет циклических host events, повторной отправки mutation и сырой ошибки `[object Object]`.
- [ ] Linux `amd64` сборка проходит, `go version -m` показывает точный HEAD интеграции (`vcs.modified=false`). Для вложенных worktree использовать явные `GIT_DIR="$(git rev-parse --absolute-git-dir)" GIT_WORK_TREE="$PWD"` перед `go build -buildvcs=true`; ранее обычный билд ошибочно маркировал внешний checkout.
- [ ] CI GitHub Actions на PR зелёный, включая все поддерживаемые платформы/сборки; отдельно проверить схему инструментов MCP.
- [ ] В **новом чате** Setup действительно открывается — пользователь это уже подтвердил на установленном Linux-кандидате. Но это **не означает**, что проверен Switch.
- [ ] Live-тест `Switch project → mikrotik` и обратно: правильный `get_environment`, `list_worktrees` и `git_status`, без ошибок и мерцания карточки.
- [ ] Второй параллельный разговор остаётся привязан к своему проекту. Токен UI-разговора A нельзя использовать для B.
- [ ] Ошибки `isError` видимы текстом; отсутствуют циклы вызовов, ``Setup`, `Refresh` и `Select` корректно прекращают работу после ошибки.
- [ ] Ошибки `RATE_LIMITED` фиксируются отдельно: они наблюдались до и после установки; сами по себе не доказывают регрессию Setup.
- [ ] Защита отката установленного сервиса остаётся доступной. Слияние Git не требует обновления работающего бинарника. Новая установка из будущего `main` — отдельная контролируемая операция.

## Проверенная Linux-база и границы доказательств

Кандидат `b37ec0e` ранее прошёл на Linux Node UI harness, `go test ./...`, `go vet`, целевые race-тесты. Собранный Linux-бинарник с корректным VCS stamp имеет SHA-256 `ab004c3cab72fc1122db7ef580be09487e2b7fd7d4219abfdffe700d6b45a4bf`.

После установки сервис вернулся с HTTP 200 и корректным MCP-доступом; резервная копия сохранена, независимый таймер rollback подтвердил стабильный сервис и откат не потребовался. Пользователь сообщил, что Setup **работает в новом чате**. Однако старый чат получал ошибку открытия и полный live-тест переключения проектов/изоляции A/B **ещё не принят**.

Критично: код будущего выборочного переноса в `main` **не равен байт-в-байт** установленному кандидату из Windows ветки. Поэтому результаты установленного кандидата нельзя автоматически переносить на интеграционную сборку; тесты и контроль VCS-штампа нужно повторить после разрешения конфликтов.

## Итог и handoff следующему исполнителю

1. Создать интеграционный worktree от свежей `upstream/main` (не трогать рабочий `main`).
2. Выборочно перенести Setup, UI и tests, сохранить приоритет `main` в diagnostics и исторических Windows файлах.
3. Проверить diff/тесты/линтер/race/сборку и live Setup в двух разговорах.
4. Закоммитить ограниченный scope, запушить интеграционную ветку в `upstream`, открыть PR в `main`.
5. Сливать только после выполнения критериев; Windows/PollWatcher/diagnostics развивать отдельными PR.

**Состояние при составлении документа:** handoff подготовлен, **интеграционный перенос в `main` ещё не выполнен**.
