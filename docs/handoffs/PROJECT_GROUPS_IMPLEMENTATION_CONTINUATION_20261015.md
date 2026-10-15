# Handoff №2 — Project Groups: реализация полноценной multi-repo работы в одном чате

**Дата:** 15 октября 2026. **Репозиторий:** https://github.com/benice2me11/codexify-go
**Статус:** PROPOSED / CODE NOT IMPLEMENTED / PROJECT GROUPS ACCEPTANCE GATES NOT RUN.
**Предыдущее ТЗ:** [Project Groups — Implementation and Acceptance Protocol](../superpowers/plans/2026-10-14-project-groups-implementation-and-acceptance.md). Этот handoff уточняет его после реального Windows Go-only cutover и Setup v3, не заменяя его полный набор PG-A00–PG-A08.

## 1. Требуемый результат

Пользователь должен **в одном ChatGPT conversation** одновременно работать с несколькими независимыми Git-репозиториями: читать/редактировать файлы, запускать команды и тесты, просматривать и создавать независимые Git diff/commits/push, пользоваться AGENTS.md и отдельными source checkout/managed worktrees. Нельзя требовать переподключения коннектора, нового чата или глобального set_project_root для перехода между репозиториями.

Первый настоящий use case — группа **job-search**:

| Alias | Project | Source checkout Windows | Предпочтительный режим |
|---|---|---|---|
| work (primary) | work-search | C:\Users\FoxOS_User\work-search | Source |
| hh | hh-mcp-server | C:\Users\FoxOS_User\hh-mcp-server | Source |
| habr | habr-career-mcp | C:\Users\FoxOS_User\habr-career-mcp | Source |

Пути — ориентиры для существующей Windows-конфигурации: перед использованием удостовериться, что локальные checkouts существуют, принадлежат разрешённому catalog и имеют каноническую идентичность. На macOS путь корня проекта обычно ~/prjctx, на Linux иной; конфигурация хоста задаётся отдельно. Нельзя копировать Windows absolute paths между ОС.

**Результат должен быть настоящей маршрутизацией функций**, а не UI-списком репозиториев и не временным switch_project_root между вызовами. Каждая операция выбирает одну из разрешённых корневых директорий по стабильному alias и исполняется там, сохраняя binding всей группы.

## 2. Что уже готово в main и не должно переписываться

- Windows Go-only SCM service, пользовательский worker, управляемый tunnel runtime, G03 PollWatcher и Go-only G10 watcher. Rust автооткат из-за VPN/403 не должен возвращаться.
- Setup v3 c conversation-scoped uiContext (middleware в internal/mcpserver/setup_context.go), новый resource URI setup/v3 с legacy v2, обработка isError и host globals без бесконечных tool calls.
- Explicit Auto / Source checkout / Managed worktree: nil createWorktree следует политике, false запрещает создание, true создаёт managed. Смена выбранного типа требует явного Switch, существующие checkout/dirty files остаются.
- internal/workspace/root.go + Windows canonicalExistingPath: запрет traversal/escape, нормализация extended paths и 8.3 alias, защита от junction/symlink при файловых операциях.
- GitHub CI native Windows/Linux/macOS, race, cross-build, compiled Setup v3 MCP smoke, Windows Go-only operator fixtures.
- Инструменты upstream MCP hh_local, habr_career и node_repl уже доступны через существующий bridge: discovery источников **не эквивалентен** файловому доступу к трём проектам.
- В production Windows уже установлен новый Setup v3 кандидат cdf8c98, а source repo main к моменту первичного handoff был 7c66828. После первого handoff HEAD может измениться: ПЕРВЫМ ШАГОМ получить новый реальный HEAD и runbook, не считать старые SHA текущими.
- Код Project Groups ещё не внедрён. Не выдавать план или успешный multi-project каталог за завершённую multi-repo реализацию.

## 3. Исходный single-project bottleneck: точные точки изменения

| Код | Что сейчас ограничивает | Что нужно |
|---|---|---|
| internal/config/config.go | MCPConfig описывает projects, worktrees, upstreams и один workspaceRoot | Opt-in projectGroups catalog и валидация членов/alias/primary, изолированная для каждой ОС |
| internal/projects/manager.go | Workspace/Select/Status возвращают один active project root | Групповой resolver с atomic per-call выбором member, без перебиндивания conversation |
| internal/projects/bindings.go | bindingVersion=2, одна SourceProjectRoot/ProjectRoot | Аддитивный tagged Binding v3 для group, с сохранением v1/v2 |
| internal/projects/lifecycle.go, catalog.go | Lifecycle и discovery per single project/worktree | Per-member checkout identity, source/managed выбор, restore/switch без удаления чужих деревьев |
| internal/mcpserver/server.go | workspaceFor(req) выбирает один workspace Root | Repository-aware selection для file/search/patch/git/diff/exec/context, явные member params |
| internal/agenttools/files.go, git.go | Root задаётся для одной инстанции | Конструировать инструменты с выбранным group member Root |
| internal/execsession/manager.go | Session по ID, workdir привязан к Root в момент запуска | Жёсткая owner tuple conversation + group + repository + immutable session root для write_stdin/cancel/poll |
| internal/diff/* | Checkpoint и owner принадлежат отдельному checkout | Независимый курсор/owner на каждый member и его checkout |
| internal/projectdoc/*, memory/*, skills/*, markdownchat/*, artifacts/* | Контекст и state связываются с workspace | Обязательный контракт per-repo vs per-group; исключить утечки AGENTS.md и memory |
| internal/ui/resources.go + connectorschema/* | Setup v3 один project picker и current schema | Group picker, member selector, Source/Managed per member; новая версия UI/schema с безопасным refresh |

**Проверить фактические имена типов и функций на HEAD** до кодирования. В старом плане могут быть более ранние названия. Не менять signature сразу десятков обработчиков: сначала добавить resolver и тесты на его контракт, затем расширять API по группам.

## 4. Предлагаемый контракт (ещё не существующая конфигурация)

~~~json
{
  "mcp": {
    "multiProject": true,
    "projectGroupsEnabled": false,
    "projects": [
      {"name": "work-search", "path": "C:/Users/FoxOS_User/work-search"},
      {"name": "hh-mcp-server", "path": "C:/Users/FoxOS_User/hh-mcp-server"},
      {"name": "habr-career-mcp", "path": "C:/Users/FoxOS_User/habr-career-mcp"}
    ],
    "projectGroups": [
      {
        "name": "job-search",
        "primary": "work",
        "repositories": [
          {"alias": "work", "project": "work-search", "checkout": "source"},
          {"alias": "hh", "project": "hh-mcp-server", "checkout": "source"},
          {"alias": "habr", "project": "habr-career-mcp", "checkout": "source"}
        ]
      }
    ]
  }
}
~~~

**JSON — эскиз ТЗ, а не готовый синтаксис работающего config.json.** Поля необходимо согласовать с Go Config/MCPConfig и тестами загрузки. Включение флага только явно, default false; если multiProject=false, попытку добавить group отклонять. Не поддерживать nested groups в первой версии.

**Идентичность member:** alias уникален и стабилен в пределах группы; primary принадлежит членам; project должен быть уже разрешён в configured catalog. Проверять canonical roots, OS-specific case/symlink/8.3/UNC, запрещать duplicate/overlapping roots. Изменение membership/path после binding должно требовать явного rebinding; никаких молчаливых расширений прав.

**Checkout per member:** source vs managed независимо. Показывать source path, active path, Git root, branch/worktree и effective checkout mode для каждого. Existing dirty files не переносить автоматически, не создавать managed worktree при явном source=false; auto-политика должна оставаться backward compatible.

## 5. Persisted state / semantics

Рекомендуемая модель **Binding v3 mode=group**: conversation identity/scope, group name, primary alias, active alias, validated membershipFingerprint и список member records с sourceRoot, activeRoot, checkout kind, managed flag, worktree metadata и optimistic generation. Старые v1/v2 binding не мигрировать без надобности и никогда не трактовать как group. В случае disabled feature v3 сохранять как данные, но fail closed при попытке использовать group с понятным сообщением, не заменять workspace домашним каталогом.

**activeRepository** отвечает только за implicit default, меняется явным set_active_repository. **Explicit repository parameter** в конкретном вызове не меняет activeRepository. Для двух параллельных вызовов это исключает гонку «сначала switch global, затем выполнить command», один handler всегда получает заранее проверенный immutable resolved member.

Предлагаемые инструменты:
- list_project_groups — только доступные конфигурации;
- set_project_group(name) — одноразовая привязка группы к conversation;
- get_project_group_status — current member roots, checkout modes и mode/flags;
- set_active_repository(repository) — меняет default alias без изменения списка member или создания нового чата;
- optional repository string в repository-scoped tool inputs, отсутствие означает active alias, а в legacy chat — прежний single root.

~~~text
set_project_group(name="job-search")
read_file(repository="hh", path="pyproject.toml")
exec_command(repository="habr", cmd="git status --short")
git_status(repository="work")
set_active_repository(repository="hh")
git_diff()    # теперь hh, но explicit выбор выше не изменял default
read_file(repository="work", path="README.md") # не меняет default с hh
~~~

Именно такая пара возможностей (explicit routing + persisted active default) нужна для полной работы с несколькими репозиториями без перепривязки.

## 6. Полный охват инструментов

- **Файлы:** read_file, write_file, glob, grep, apply_patch. Для patch относительные пути обязаны пройти confinement выбранного Root; запрещать «одной заплаткой» писать сразу в два репозитория без двух независимо проверенных операций.
- **Ingress/egress и resources:** import_host_file, export_host_file, связанные артефакты, скачиваемые file resources — метки member/owner, ограничения и срок жизни токенов. Никакого implicit home-root.
- **Exec:** exec_command запускается в выбранном member workdir; write_stdin сохраняет исходную owner tuple даже после смены active alias. Stop/poll/cancel и reset аналогично. Foreign conversation/session ID — отказ.
- **Git и diff:** git_status, git_diff, git_log, show_diff, list_worktrees; отдельные Git roots, HEAD/status/diff checkpoint и build/test commands. Не запускать Git для не-Git member. Коммит/пуш должен относиться только к repo selected, никаких общих staging областей.
- **Context:** get_project_doc, get_agent_brief, AGENTS.override.md/AGENTS.md — читать инструкцию выбранного member; group orchestration brief не должен незаметно подменять per-repo instructions. Для remember/recall, skills, CHAT.md, session memory и ticket state определить explicit scope и регрессионные тесты.
- **Upstream:** mcp_list_sources, mcp_search_tools, mcp_call_tool для hh_local/habr_career должны оставаться независимыми от alias; изменение group/active не должно создавать бурст новых MCP-сессий, менять upstream paths или выполнять job applications.

Ответ каждого repository-scoped tool должен по возможности включать **effective alias + canonical source/active path + checkout**, чтобы пользователь понимал, с каким деревом работал агент. Не изобретать общего Git status на разные репозитории.

## 7. Безопасность: от чего защищаем, от чего нет

Файловые операции через workspace.Root должны сохранять реальные canonical containment проверки на точке доступа: reject ../, absolute paths, symlink/junction escapes, Windows 8.3 short names, UNC/extended-length aliases, path swap/reparse points, отсутствующие каталоги и подменённые roots. Нельзя сделать member.Root равным C:\Users\FoxOS_User или ~/prjctx и доверять лишь alias.

**exec_command НЕ является OS sandbox.** workdir задаёт лишь начальный каталог обычного shell, который наследует аккаунт, PATH, переменные окружения и доступ к остальным файлам ОС. Это Trusted Workspace с явно заявленной областью доверия; полноценный sandbox на уровне токенов Windows/контейнера/OS permissions — отдельная будущая функция с отдельными тестами. Не описывать Path confinement в FileTools как изоляцию произвольной команды shell.

Per-conversation uiContext должен подтверждаться сервером и иметь TTL; запрет использовать этот token в file/exec/tools сохраняется. Группа также требует проверки membershipFingerprint + binding owner на каждом mutating вызове. Журналы не сохраняют пароли .env, настройки tunnel или private пользовательские файлы.

## 8. Этапы реализации и reviewable PR

| Stage | Deliverable | PASS до следующего этапа |
|---|---|---|
| PG-I0 | Актуальный baseline, карты инструментов/схемы, tests legacy, безопасный go-only rollback | Исключения, контракты и acceptance ledger зафиксированы |
| PG-I1 | Config flag + catalog groups, canonical root registry | Позитивные/отрицательные path/alias tests, Windows junction/short-path |
| PG-I2 | Binding v3, membership fingerprint, persist active alias и per-member checkout | v1/v2 regression и два контролируемых restart smoke |
| PG-I3 | Files/patch/exec/Git/diff/session routing | Одновременные 3 repo, per-member diff, cross-chat session denied |
| PG-I4 | Context/memory/skills/agent docs/artifacts/upstream bridging | AGENTS и contexts не смешиваются; no tool burst |
| PG-I5 | Setup Group UI, mode selector, group overview + schema version bump | Устойчивые hosted cards, stale schema guidance, keyboard/a11y smoke |
| PG-I6 | CI native Windows/Linux/macOS, feature-flag canary | G10 Go-only guard непрерывный, old single-project mode не регрессирует |
| PG-I7 | Rollout + acceptance ledger + rollback without Rust | PG-A00..PG-A08 PASS, только затем закрывать feature |

**Не делать гигантский PR.** Минимально нужны отдельные reviewable PR: config/binding, per-tool routing, context/session safety, UI/schema, cross-platform runtime/tests. Не смешивать с Linux Setup PR #3, Windows G10 guard hardening или GitHub branch cleanup.

## 9. Mandatory acceptance (полный список — в исходном Project Groups плане)

| Gate | Необходимый результат |
|---|---|
| PG-A00 | baseline + rollback + exact legacy behavior |
| PG-A01 | group config/aliases/paths canonical, reject overlaps/unknown/stale member |
| PG-A02 | v1/v2 chats unchanged; v3 persistent after two restarts; no worktree/data loss |
| PG-A03 | file/search/patch/exec in 3 repos, implicit vs explicit routing, concurrent calls do not race |
| PG-A04 | Git status/diff/log/show_diff independent; member-specific AGENTS/skills/artifacts/memory |
| PG-A05 | traversal/symlink/junction/UNC/8.3 + cross-member/foreign-session negative cases; trusted-shell caveat |
| PG-A06 | user uses one live conversation, changes member/mode without rebinding; no MCP bursts/RATE_LIMITED loop; upstream unchanged |
| PG-A07 | Go tests, vet, race, Windows/Linux/macOS native smoke, no single-project regressions |
| PG-A08 | controlled canary/restart/reboot, operator logs, Go-only rollback rehearsed, feature-flag disable validated |

Ни один gate нельзя отмечать PASS по документу или скриншоту, если соответствующий тест не выполнен. Тесты файлообмена и Git-пушей делать только в disposable fixture repos/ветках, не в live work-search/hh-mcp-server/habr-career-mcp с реальными uncommitted changes.

### End-to-end сценарий для приёмки

1. Feature flag enabled в isolated fixture; в одном чате выбрать group job-search, primary=work и три Source checkouts, без автоматического managed worktree.
2. Прочитать по файлу и запустить свой безопасный test/git status в work, hh и habr. Сравнить actual roots/aliases.
3. Создать две независимые fixture правки и убедиться, что show_diff/commit в hh не затрагивает habr; push исключительно на тестовые ветки/репозитории.
4. Поменять activeRepository hh → habr → work, при этом explicit repository override должен работать независимо. Ни одного глобального set_project_root, refresh connector или нового чата.
5. Начать harmless длительный exec в hh, выбрать habr, продолжить hh session по write_stdin; из другого разговора попытка обратиться к session должна завершиться отказом.
6. Проверить negative cases: ../ соседний репозиторий, junction/symlink escapes, изменённые roots, одинаковый alias с другим case, stale fingerprint, отказ подставить home-root.
7. Перезапустить Go worker/service дважды на разрешённом стенде, затем проверить alias, checkout modes, binding, diff checkpoints, независимость двух conversations и upstream.
8. Отключить feature flag: legacy single-project функционирует, данные v3 не теряются, явное reenable/resume восстанавливает группу.
9. Проверить UI/AGENTS/context leakage, логи MCP-burst/429 и actual live upstream hh_local/habr_career **только read-only**.
10. Зафиксировать evidence и gate results. Выполнять Windows production rollout отдельно через существующий Go→Go operator flow, с правильным watcher и сохранённым старым Go-бинарником.

## 10. Release boundary и продолжение

Рекомендуемая последовательность:
1. Закрыть или точно классифицировать всё из handoff №1, принять/выпустить v0.9.0 **без Project Groups**, если эксплуатационные gates выполнены.
2. От свежего main открыть PG-I0/I1. Согласовать API/config schema до изменений в десятках MCP tools.
3. Реализовать и принять PG-I2–I6 отдельными PR, дав feature flag default false.
4. Отдельно выбрать номер будущей версии с Project Groups; не утверждать, что это часть уже запущенной v0.9.0. Для включённой возможности нужен новый version marker в connector schema (например +project-groups-v1), обновлённый Setup resource URI и ясный refresh path для старых cards.
5. После PG-A00–PG-A08 принять go-only canary и закрыть feature. Не включать Vacancy Hourly Watch и не отправлять HH/Хабр отклики без отдельного согласия.

**Суть задачи следующему агенту:** не просто показывать 3 репозитория в UI, а сделать per-call и persistent active-member routing во всех инструментах при сохранении изоляции FileTools, доверенной модели Exec, старых чатов и Go-only устойчивости.
