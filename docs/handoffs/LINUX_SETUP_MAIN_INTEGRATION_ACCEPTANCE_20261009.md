# Linux Setup → main: интеграция и критерии приёмки

Дата: 2026-10-09. Цель: `benice2me11/codexify-go:main`.

## Происхождение

- Интеграционная ветка: `codex/linux-setup-main-integration-20261009`, создана от `upstream/main` = `e3e9ca1`.
- Исходный кандидат: `codex/linux-setup-parity-20261009` = `df0180d`; подробный handoff — `docs/handoffs/LINUX_SETUP_MAIN_INTEGRATION_HANDOFF_20261009.md` **в ветке кандидата**, не в этой ветке.
- Кодовой коммит интеграции: `e1e6a8531454d6aafd8e2bc6d9058ef76de5bae7`. Семь путей отличаются от `upstream/main`: `internal/mcpserver/server.go`, `server_test.go`, `setup_context.go`, `setup_context_test.go`, `internal/projects/catalog.go`, `internal/ui/resources.go` и `widget_runtime_test.mjs`.
- Эти семь файлов перенесены в версии кандидата без изменений содержимого. Это выборочная интеграция, **не merge ветки кандидата**.

## Что реализовано

- Модельный `setup`, совместимость UI resource v1/v2, актуальный Setup v5, app-callable инструменты и начальное состояние из `list_projects`.
- Серверная проверка `uiContext`, ограниченный срок и область действия, привязка к conversation identity, запрет передачи `uiContext` файловым инструментам.
- Отрисовка Setup без автоматических повторных запросов из host events, защита от гонок UI, структурированный вывод ошибок MCP вместо `[object Object]`.
- Регрессионные тесты по двум независимым контекстам, просроченным токенам, ошибкам выбора, поздним host events и одновременным действиям.

## Автоматическая проверка

Проверки исполнены на Linux в отдельном worktree на приведённом выше кодовом коммите:

| Команда | Результат |
| --- | --- |
| `go test -count=1 ./...` | PASS |
| `go vet ./...` | PASS |
| `go test -race -count=1 ./internal/mcpserver ./internal/projects ./internal/ui` | PASS |
| `node internal/ui/widget_runtime_test.mjs --source internal/ui/resources.go` | PASS |
| `git diff --check upstream/main...HEAD` | PASS (до документирующего коммита) |
| `go build -buildvcs=true -o ... ./cmd/codexify-go` с явными `GIT_DIR` и `GIT_WORK_TREE` | PASS |

Go VCS metadata для бинарника из кодового коммита: `vcs.revision=e1e6a8531454d6aafd8e2bc6d9058ef76de5bae7`, `vcs.modified=false`, `GOOS=linux`, `GOARCH=amd64`. SHA-256 сборки: `bba31b467b9eeffc569705c829ce082faeeec59feece1809744778d211d468c8`. Бинарник сохранён **только локально** в игнорируемой `.codexify-go/builds/`, не установлен.

Тесты сначала воспроизвели исходную проблему на `main`: `TestUIResourcesAndToolMetadata` не находил `setup`; Node harness выявлял прежнюю автоматическую загрузку Setup (2 вызова вместо 0). После переноса оба набора прошли.

## Изолированный объём, не включённый в PR

Никаких изменений в `internal/tunnel/**`, `internal/app/**`, `internal/health/**`, `internal/config/**`, `internal/mcpdiag/**`, `tools/windows/**`, Windows handoff/evidence, сервисе systemd, пользовательских токенах или конфигурации. Незакоммиченные файлы другого worktree не затронуты.

## Осталось до merge

- [ ] GitHub CI и совместимость схем MCP-инструментов на опубликованном PR.
- [ ] Live проверка **этой сборки**, а не ранее установленного кандидата: Setup в новом разговоре; Switch → `mikrotik` → обратно, с `get_environment`/`git_status`/`list_worktrees`.
- [ ] Второй разговор сохраняет свою независимую project binding; токен A не может использоваться в B.
- [ ] Проверить сообщения об ошибках и отсутствие мерцания/циклов в поддерживаемых хостах (в частности в старых карточках).
- [ ] Контролируемую установку и rollback выполнить **отдельно**, без смешения с Git/PR.

**Статус:** автоматические проверки пройдены. Live-интеграция, CI и установка в этом документе **не утверждаются**. PR должен оставаться draft/не слитым до завершения критериев.
