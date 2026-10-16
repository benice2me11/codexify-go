# G11 Windows steady-state — handoff для нового чата (16 октября 2026)

> **Назначение:** продолжить завершение перехода Rust → Go на Windows, не прерывая уже успешно работающий `codexify-go`, и отдельно определить следующий этап Project Groups. Это handoff **для выполнения G11**, а не отчёт о пройденном G11. Основной приоритет — постоянный Go-only мониторинг до истечения временного 27-часового G10 observer.

## 0. Стартовый статус и решение пользователя

- **Источник:** `github.com/benice2me11/codexify-go`, локальный source checkout `C:\Users\FoxOS_User\codexify-go`, `main` = `67b94e1b1642bc8322c9a7b3f6a3c0d87eb6085b`, чистый, синхронизирован с `origin/main` на момент аудита. CI main: [run 38872570386](https://github.com/benice2me11/codexify-go/actions/runs/38872570386), **success / 11 из 11**.
- **Текущий Windows production:** `CodexifyGo` — `Running`, `Automatic`; Go supervisor + один worker (всего **2 Go**), **один** `tunnel-client-runtime`, **0 Rust**. Сервис после успешного Go→Go cutover работает на SHA256 **`F5AF36CEA944688CB8BF6145F8047722CA23F991E077DA1D7897D74B2D3A0DB2`**, исходный код бинарника — `cd6b5bd59dea0be35e3107446dfddce322f41ad2`.
- **Новый G10:** локально `CONTINUE / within_limits`, свежий guard, `LOCAL_OBSERVED`. Это **не** новый 24-часовой `G10 PASS`; протокол исходного оператора зафиксировал `RESET_AND_RUNNING_NOT_ACCEPTED`.
- **Предыдущее операторское G10-исключение:** `G10_OPERATOR_ACCEPTED_WITH_DEVIATIONS` относится к **предыдущему** Go-бинарнику, а не автоматически к нынешнему. Пользователь отдельно разрешил бинарный Go→Go cutover на точный SHA, и он успешно завершён 16 октября в 07:59:56 UTC.
- **G11:** **PENDING / NOT ACCEPTED**. Не маркировать G11 `PASS` просто потому, что новый Go запущен и отвечает.
- **Project Groups:** реализация PG-I1..PG-I7 в коде и уже в новом запущенном бинарнике, но **`mcp.projectGroupsEnabled=false`**. Живой ChatGPT всё ещё получает **Setup v3**, не Setup v4. Live feature canary PG-A00..A08 не закрыт.
- **Stable `v0.9.0`:** **HOLD**. Project Groups — отдельный rollout, не автоматическое условие/разрешение стабильного релиза.
- **Rust:** обе задачи `Codexify` / `Codexify Watchdog` — **Disabled**, Rust-процессов нет. **НЕ включать Rust fallback.**
- **Сторонняя работа:** `Vacancy Hourly Watch` оставить выключенным, отклики на вакансии не отправлять, репозитории `work-search` / `hh-mcp-server` / `habr-career-mcp` не изменять ради G11.

Контрольный read-only snapshot: 2026-10-16 ~08:42 UTC / ~11:42 по московскому времени — service `Running`, единственный новый Go supervisor, 1 worker, 1 tunnel child, 0 Rust, G10 watcher task `Running`, текущий `CONTINUE`, а весь Git-код `main` чистый. У сервиса уже настроен Windows SCM recovery: **RESTART через 5, 15, 60 секунд**, сброс счётчика — 86400 секунд (`sc.exe qfailure CodexifyGo`). **Это нужно учитывать до написания второго watchdog.**

## 1. Критичный дедлайн: временный Go-only наблюдатель

Сейчас `CodexifyGo-G10Observers` — Windows Task Scheduler, `BootTrigger`, `Highest`, `MultipleInstances=IgnoreNew`. Его действие запускает **`Watch-GoTrial.ps1`** из приватного `operator-trial-v6` с:

- `-ArmGoRecovery`, `-IntervalSeconds 10`, `-DurationHours 27`.
- Внутренний deadline наблюдателя — 27 часов от старта процесса. Он запущен примерно **2026-10-16 07:59:55 UTC** и, если не перезапустится, сам закончит работу приблизительно **2026-10-17 10:59:55 UTC / 13:59:55 по Москве**. Перезагрузка/новый запуск перезапустит это окно: проверять фактический PID и время запуска.
- Windows Task имеет `ExecutionTimeLimit=PT72H`, но **это НЕ делает 27-часовой скрипт постоянным**. Если сам скрипт завершится, задача с одним BootTrigger не обязана перезапуститься до следующего запуска Windows.
- Текущий `Watch-GoTrial` проверяет образ SCM, 2 Go / 1 worker / 1 managed tunnel, 0 Rust; пишет `probe-observations.jsonl` каждые ~10 секунд и метрики ~60 секунд.
- Два вида защитных событий: `BLOCKED` при Rust, чужом SCM ImagePath, лишних Go владельцах — **выход без переключения владельца**; `RECOVER_GO` только после шести последовательных наблюдений отсутствующего/неработающего **Go supervisor**, и только на предыдущий проверенный Go. Отсутствие tunnel/worker → `HOLD`, а не Rust или автоматический откат всего Go.
- Любые исключения записи observer логируются как `watcher_observation_failed`, но их нельзя выдавать за непрерывный G10 PASS.
- **Особый нюанс recovery:** `Invoke-GoOnlyRecovery.ps1` при восстановлении предыдущего Go отключает G10 task и создаёт `observer.stop`. Поэтому постоянный мониторинг не должен считать старый recovery watcher вечно доступным или бесконтрольно перезапускать его после смены SCM-образа.

**P0 новой задачи:** обеспечить постоянный контролируемый Go-only мониторинг **до истечения наблюдателя**. Сначала подготовить и протестировать решение; **не выключать текущий G10 watcher до готового безопасного handover**.

## 2. Зафиксированные производственные артефакты

Приватный корень, **не копировать в Git, не прикладывать секреты и исходные JSON конфигурации**:

`C:\Users\FoxOS_User\.codexify-go\staging\project-groups-v1-cd6b5bd59dea\`

- Новый кандидат: `candidate\codexify-go.exe` — SHA256 `F5AF36CEA944688CB8BF6145F8047722CA23F991E077DA1D7897D74B2D3A0DB2`.
- Production config: `config\project-groups-disabled.json` — SHA256 `9453FF3D13DCD97287112D13AB5A3E8ABDC64AAAC72C910AD238AA339D6D72F1`.
- Frozen `operator-trial-v6\manifest.json`; `upgrade-result.json` = `GO_UPGRADED`; `operator\` содержит 10 защищённых операторских файлов.
- Предыдущий Go: `operator-trial-v6\previous-go\codexify-go.exe` — SHA256 `8BAA786E6BEFDCE5C746419B5A9277D3531A824938396DF8DDDA3FB7200B5880`.
- Предыдущая конфигурация: `operator-trial-v6\previous-go\production.json` — SHA256 `47DF2D446BAF84D9DC9CE0243AFD79A22B803525884BFB94E6D3D79B6B03830D`.
- Task XML до переключения, события `operator-trial-v6\evidence\operator-events.jsonl`, активный guard `...\evidence\probe-observations.jsonl`, sample/metrics logs.
- `CodexifyGo-PGCanary-cd6b5bd59dea` — завершённая **однократная** независимая задача с `LastTaskResult=0`, `State=Ready`, 0 автоматических триггеров. **Не запускать повторно** — транзакция уже исполнена; lock оставлен намеренно.
- Предыдущие `operator-trial-v2..v5` — только исторический материал. Старые executable оператора были сделаны инертными.

**Важное:** при G11 хранить копию предыдущего **Go** для восстановления, не удалять её, не переделывать Rust в автоматический recovery. Сохранённые Rust-артефакты — только forensic/ручной аварийный архив: исторический G11 протокол предусматривал хранение как минимум 7 суток и три нормальные рабочие сессии **после фактической приёмки**, удаление требует отдельного решения.

## 3. G11 — требования и что сейчас уже выполнено

Первоисточник — `docs/handoffs/WINDOWS_CUTOVER_V2_ACCEPTANCE_20260930.md`, секция `G11 - Closure and recovery retention`.

| Обязательство G11 | Состояние на 16 октября | Что делать |
|---|---|---|
| Один проверенный Go, точные binary/config SHA256, автозапуск; Rust не работает | **PASS (live)** | Снять финальный read-only snapshot и повторить после всех изменений |
| Реальный SCM restart при падении сервиса | **Настроен 5/15/60 c** | Изолированно проверить сохранение правил и отсутствие второго владельца; не устраивать ненужных production-crash тестов |
| Проверенный предыдущий Go и Go-only recovery | **Snapshots PASS, живой откат не потребовался** | Тестировать на disposable SCM fixture; production rollback только при реальном подтверждённом дефекте/санкции |
| Постоянный мониторинг после G10 | **НЕ СДЕЛАН**; watcher ~27 ч | **P0:** спроектировать, протестировать, безопасно перевести на steady-state Go-only supervision |
| Перевести production на обычный профиль без временной диагностики | **НЕ СДЕЛАН** | Сейчас `log.level=info`, но `mcp.diagnostics.enabled=true`. Подготовить новую нормальную конфигурацию с `mcp.diagnostics.enabled=false`, остальные параметры не менять |
| Финальный контролируемый рестарт и 30 минут наблюдения | **НЕ СДЕЛАН для G11** | Только после проверки постоянного мониторинга и одобрения окна переключения; подтвердить MCP/ChatGPT и рабочие bindings |
| Удалить/отключить только временные задачи и watchers | **НЕ СДЕЛАН** | Не трогать `CodexifyGo-G10Observers` заранее; сохранять evidence. Однократную operator task выводить только с отдельной фиксацией |
| Финальный отчёт / scope / риски / операторская приёмка | **НЕ СДЕЛАН** | Зафиксировать G10 нового кандидата отдельно; закрыть G11 только с честным PASS или явным операторским исключением |

**Граница G10/G11:** прежнее пользовательское принятие G10 с отклонениями было привязано к **старому** SHA256. Новая версия только начала собственный G10. Не переносить старое `PASS` или исключение на новую версию; если строгие 24 ч не набраны, нужна отдельная формулировка решения пользователя о принятом риске **для нового** Go, с сохранением фактического статуса измерений.

## 4. План работы в новом чате (приоритеты)

### G11-00 — read-only базовая линия (P0)

1. Вызвать `get_environment` и `get_agent_brief` один раз; убедиться, что активен **исходный checkout** `C:\Users\FoxOS_User\codexify-go`, `managedWorktree=false`. Не менять `set_project_root` без необходимости.
2. `git fetch origin`, `git status --short --branch`, `git rev-parse HEAD`; сохранить сторонние изменения, работать в отдельной feature-ветке, без reset/force-push.
3. Проверить: SCM `CodexifyGo` Running/Automatic; точный SHA нового Go; 2 Go, 1 worker, 1 tunnel, 0 Rust; ``Codexify`/`Codexify Watchdog` Disabled; `sc.exe qfailure CodexifyGo`; активный G10 observer, один watcher PID, свежий `CONTINUE`, реальный uptime.
4. Зафиксировать task XML/hash, существующий `-DurationHours 27` и время завершения, текущий config SHA, диагностику, приватный предыдущий Go snapshot. Не пересоздавать или переподписывать `operator-trial-v6`.

Полезные **только read-only** команды Windows:

```powershell
Get-CimInstance Win32_Service -Filter "Name='CodexifyGo'"
sc.exe qfailure CodexifyGo
Get-ScheduledTask -TaskName 'CodexifyGo-G10Observers','Codexify','Codexify Watchdog'
Get-ScheduledTaskInfo -TaskName 'CodexifyGo-G10Observers'
Get-CimInstance Win32_Process | Where-Object {
  $_.Name -in @('codexify-go.exe','tunnel-client-runtime.exe','codexify.exe')
}
$tx = Join-Path $env:USERPROFILE '.codexify-go\staging\project-groups-v1-cd6b5bd59dea\operator-trial-v6'
& (Join-Path $tx 'operator\Inspect-G10Runtime.ps1') -RunDirectory $tx
git status --short --branch
```

Не публиковать полные командные строки с токенами, `config`, экспортированные task XML, `.env` или исходные логи — только отредактированные статусы/хэши.

### G11-01 — архитектура постоянного Go-only контроля (P0)

**Избежать второго конкурирующего watchdog:**

- **SCM** уже перезапускает упавший supervisor (5/15/60 секунд). Сам Go supervisor должен управлять worker и туннелем. Не добавлять второй независимый процесс, который автоматически запускает ещё одну копию `service run`, worker или tunnel.
- Дополнительный Go-only monitor должен различать живой/неживой SCM, зависший локальный `/health`, неожиданный бинарник, два Go-владельца, чужой Rust, потерю observer, локальные ошибки чтения/записи. Контроль может быть периодическим отдельным Task Scheduler, а не бесконечным скриптом — конкретная форма требует тестов.
- **Обычная сетевуха:** VPN down, временные HTTP 403/429/503/timeout control-plane и краткое self-heal туннеля — регистрировать и давать текущему Go восстановить соединение. Не откатывать бинарник и тем более не включать Rust.
- **Настоящая локальная поломка:** после SCM-grace и нескольких проверок подтверждённого отказа — alert/операторская диагностика; Go-only recovery на предыдущий проверенный SHA допускается по документированному локальному сценарию, а не из-за одного HTTP-кода.
- **BLOCKED / foreign ImagePath / duplicate owners / Rust owner / secret exposure:** fail closed и записать инцидент. Не перезапускать чужой процесс, не создавать петлю «watchdog → restart → watchdog».
- **Recovery outcome:** если произошёл контролируемый fallback на предыдущий Go, permanent monitor обязан распознать смену образа и не атаковать успешный recovery своей «проверкой нового SHA». Требуется явное состояние `GO_PREVIOUS_RECOVERED` с записью причины и уведомлением.
- Не оставлять немониторируемого интервала: подготовить новый read-only monitor, проверить отсутствие повторяющихся процессов, только после этого переключать восстановительную полномочность и завершать старый G10 watcher. Не держать **два одновременно recovery-armed** observer.

Выбрать предел логов: файл/каталог с ограничением размера, ротация, redaction секретов, UTC timestamps и события только при изменении состояния; безопасная защита от log spam. Отдельно спроектировать reboot/login, включая случай, когда задания с Interactive principal зависят от входа пользователя.

### G11-02 — реализация и тесты (P0)

Реализовать **отдельно от production** и закоммитить:

- PowerShell/Go-only state policy, Windows Scheduled Task installation/removal/inspect в режиме `PLAN_ONLY` по умолчанию и явном `-Execute` для оператора; безопасный rollback task XML.
- Fixture-тесты: нормальный Go, падение worker, временно отсутствующий tunnel, 403/429/503, остановленный supervisor при ещё работающем SCM recovery, два Go owner, Rust, чужой SCM ImagePath, устаревший watchdog PID, ошибки записи логов, logout/reboot, ручной previous-Go recovery.
- Специальный тест защиты от **дублирующей автоматической задачи и recovery-loop** и тест того, что выполнение monitor не зависит от активного ChatGPT MCP worker.
- Go tests/vet, PowerShell AST/offline policy, Windows native CI, Linux/macOS CI для затронутых Go-компонентов, race и cross-build; никакого живого SCM cutover до завершения тестов.
- Сам watcher на production не заменять во время написания тестов.

### G11-03 — ввод постоянного мониторинга (P0; отдельная мутация)

После готового кандидата и разрешённого перехода: snapshot исходной G10 task XML, текущих guard logs и recovery assets; install read-only monitor без второго recovery owner → наблюдение достаточного контрольного интервала → контролируемая передача recovery права (ровно один активный authority) → удостовериться, что новый monitor переживает повторный запуск и не запускает Rust. Старые G10 события не удалять. Не допускать периода с двумя одинаково вооружёнными rollback-скриптами.

Если новой мониторинговой системы до deadline не будет — **не игнорировать 27-часовой лимит**: подготовить безопасное продление с отдельным операторским согласованием, без дублирования задач и без изменения сервиса. Это запасной план, а не молчаливый restart любой ценой.

### G11-04 — обычный production профиль и финальный restart (P1)

Сейчас production `mcp.diagnostics.enabled=true`, `log.level=info`. Сформировать новый versioned config **с выключенной временной MCP-диагностикой**, сохранив byte/semantic parity остальных полей (кроме явно разрешённых diagnostics-only ключей). **`projectGroupsEnabled=false`**, те же tunnel ID/auth/upstreams, workspace, bindings, scan, service и supervisor.

После утверждённого окна: independent operator → controlled restart текущего Go (без Rust) → проверить exact config hash, SCM, single owner, локальный `/health`, Setup v3, диалоговый binding, source checkout, `mcp_list_sources`, безопасные `hh_local.get_my_resumes` и `habr_career.habr_whoami`. Провести **30 минут** наблюдения без неожиданных owner restarts или MCP burst. Сервис не рестартовать просто ради «красивого» документа.

### G11-05 — приёмка и закрытие (P1)

Создать `docs/handoffs/WINDOWS_G11_ACCEPTANCE_YYYYMMDD.md` с чеклистом, time window, source Git SHA, binary/config SHA, screenshots только без секретов, task definitions/hash, sample count/gaps, SCM recovery, rollback procedure, residual risks, Win service и Rust-disabled состоянием, а также **явным операторским решением**. В status ledger различать:

- `G11_READY_FOR_REVIEW` — техническая работа готова, но нет операционного решения.
- `G11_ACCEPTED` — все обязательные G11 шаги действительно подтверждены.
- `G11_ACCEPTED_WITH_EXCEPTIONS` — только если пользователь явно согласовал конкретные непрошедшие требования; не выдавать технические тесты за PASS.
- `G11_HOLD` — не выполнены критичные безопасность/ownership/monitoring/recovery проверки.

Stable release и тег `v0.9.0` без отдельного решения не трогать.

## 5. Project Groups: текущее состояние и отдельный план

**Итог по реализации: PG-I1..PG-I7 реализованы и включены в Git-историю `main`**, объединены через PR [#29](https://github.com/benice2me11/codexify-go/pull/29), после чего прошли native compiled-worker/SDK tests и binary-only Go→Go cutover. Код новой функции есть в текущем запущенном Go-бинарнике **`F5AF36CE…D3A0DB2`**.

| Компонент | Выполнено | Что ещё остаётся |
|---|---|---|
| PG-I1: каталог группы/валидатор корней | Реализовано | Проверить live включённый каталог |
| PG-I2: binding v3 + Source/Managed по членам | Fixture и compiled-worker restart PASS | Проверить живую сохранность после двух реальных restart |
| PG-I3: repo-aware MCP, активный alias, отдельные Git/diff | Fixture + SDK PASS | Реальная host маршрутизация 3 репозиториев |
| PG-I4: scoped resources, AGENTS/exec/artifact, security | Негативные и SDK тесты PASS | Hosted `resources/read` и реальные negative cases |
| PG-I5: Setup v4 GUI, переключение checkout | Реализовано, UI host-event fixture PASS | Реальная карточка Setup v4 в ChatGPT, UX и focus |
| PG-I6: native compiled-worker/compat | Windows/Linux/macOS CI PASS | Live service tests с группой включённой |
| PG-I7: Go-only rollout preflight | Реализовано, Go→Go бинарный cutover PASS | Feature-enable acceptance ещё не выполнена |

**Группа, уже добавленная в production JSON, но неактивная:**

| Alias | Source checkout |
|---|---|
| `work` (primary) | `C:\Users\FoxOS_User\work-search` |
| `hh` | `C:\Users\FoxOS_User\hh-mcp-server` |
| `habr` | `C:\Users\FoxOS_User\habr-career-mcp` |

Для каждого члена изначально `checkout=source`, **без автоматического managed worktree**. Текущий флаг **`mcp.projectGroupsEnabled=false`**, поэтому host advertises Setup v3 и legacy single-project flow продолжает работать. В проведённом cutover подтверждены 22 доступных проекта, включая все три repo; `hh_local` / `habr_career` read-only upstreams отвечают, но это **не** проверка маршрутизации Project Groups в живом ChatGPT.

**PG-A00..PG-A08:** базовая часть A00 и A08 (точный production SHA + controlled бинарный cutover + сохранённый previous-Go backup) уже имеет evidence, но **целиком PG-A08 не PASS**, так как **группа не включалась**. A01..A07 имеют сильные offline/native fixture результаты, но live hosted service/UX/negative cases пока не закрыты. Не писать «Project Groups полностью принято».

### Отдельный будущий feature canary после/параллельно с G11

1. Создать отдельный изменённый config с **только `mcp.projectGroupsEnabled=true`** (и необходимой явно проверенной семантикой), versioned previous config backup и операторским разрешением. Не совмещать включение группы с G11 diagnostics-off restart без явного совместного плана; предпочтительнее **два независимых контролируемых изменения**.
2. Controlled Go-only config switch с одним сервисом/туннелем и немедленным здоровьем. **Не запускать ещё один Go executable**, не включать Rust.
3. Получить ChatGPT Setup v4: схема `+project-groups-v1` и `+setup-v4` (либо compact version), после необходимости сделать connector refresh, не выдавая старый Setup v3 за новый.
4. В **отдельном тестовом conversation**, не разрушая привязку текущего: `list_project_groups` → `set_project_group(job-search, checkouts=source/source/source)` → `get_project_group_status` → `set_active_repository` → repo-specific read/Git/AGENTS/test (только неразрушающие операции в реальных репозиториях).
5. Проверить отдельно Managed checkout для одного члена на безопасном тестовом материале, переключение Source → Managed → Source с сохранением dirty state, без вмешательства в рабочие изменения.
6. Проверить две реальные перезагрузки worker/service, binding v3, независимые Git/diff, owned stdin и artifact `resources/read`; от другого разговора — **DENY**. Никакой несанкционированной работы с чужим root.
7. Live MCP upstream `hh_local` и `habr_career` read-only, без burst/429 amplification; сеть/VPN/403 не считать автоматически багом группы.
8. Если feature fails: выключить **только feature flag** и вернуть прежнюю конфигурацию контролируемо, сохранить binding v3 для возможного resume; старый Go/Rust не включать автоматически.

**Ограничения, которые нельзя скрывать:** в `projects.Manager` есть Windows in-process locking для binding JSON, **нет межпроцессного durable CAS**. А `exec_command` запускает команды с правами Windows пользователя (Trusted Workspace), это **не OS sandbox**; repo-scoped file-tool containment и ограничения CLI shell — разные вещи.

**Вывод:** Project Groups — **код готов, бинарник работает, feature rollout не начат**. В контексте G11 ничего не требуется переписывать или повторно объединять. Нужен отдельный feature flag rollout + PG-A00..A08 live приемка.

## 6. Файлы и Git-история для нового агента

Читайте в таком порядке:

1. `docs/handoffs/WINDOWS_PROJECT_GROUPS_BINARY_CUTOVER_20261016.md` — **фактический текущий runtime** и private evidence.
2. **Этот файл** — актуальный G11 P0-P1 handoff и границы PG.
3. `docs/handoffs/WINDOWS_CUTOVER_V2_ACCEPTANCE_20260930.md` — первоначальные точные G11/G10 требования.
4. `tools/windows/cutover-v2/Watch-GoTrial.ps1`, `Invoke-GoOnlyRecovery.ps1`, `GoOnlyUpgrade.psm1` — точная Go-only watchdog/recovery policy.
5. `docs/handoffs/WINDOWS_G10_OPERATOR_RISK_ACCEPTANCE_20261016.md` — предыдущее **операторское** исключение для старого Go.
6. `docs/handoffs/PROJECT_GROUPS_PG_I6_I7_NATIVE_WORKER_AND_ROLLOUT_20261016.md` и `docs/handoffs/PROJECT_GROUPS_PG_I5_SETUP_V4_20261015.md` — PG fixture/native evidence и pending gates.
7. `docs/superpowers/plans/2026-10-14-project-groups-implementation-and-acceptance.md` — детальная матрица PG-A00..PG-A08.

GitHub source baseline: `main=67b94e1b1642bc8322c9a7b3f6a3c0d87eb6085b`, PR [#35](https://github.com/benice2me11/codexify-go/pull/35), CI [38872570386](https://github.com/benice2me11/codexify-go/actions/runs/38872570386) 11/11 PASS. Держать Git чистым; рабочие исправления коммитить и пушить отдельными PR после тестов, не делать blanket reset/rebase/force-push. Другие локальные Git worktrees/форензические ветки не удалять под видом G11 cleanup без их отдельной сверки.

## 7. Сообщение для нового чата (можно вставить дословно)

> Продолжи `codexify-go` на Windows по `docs/handoffs/WINDOWS_G11_STEADY_STATE_AND_PROJECT_GROUPS_HANDOFF_20261016.md`. Начни с read-only проверки текущего Go-сервиса, `main`, Task Scheduler и приватного `operator-trial-v6`. Приоритет G11-00—G11-03: спроектируй и протестируй **постоянный Go-only мониторинг** до завершения 27-часового watcher; учти существующий SCM recovery 5/15/60 секунд, исключи двойной watchdog, MCP burst и Rust fallback. Не трогай текущий живой G10 watcher, SCM, туннель или конфиг до проверок и явного разрешения на мутацию. Далее подготовь нормальный `mcp.diagnostics.enabled=false` конфиг и план финального 30-минутного G11 smoke. Проверь тесты, коммить и пушь готовые исправления. Project Groups сейчас есть в бинарнике, но флаг выключен: **не включай** без отдельного согласия; подготовь отдельный live acceptance план Setup v4 / PG-A00..PG-A08. Сохрани previous-Go recovery, Rust Disabled, Vacancy Hourly Watch OFF и не отправляй отклики.

**Не выполнено при создании handoff:** production restart, остановка/замена watcher, новая scheduled task, включение Project Groups, G11 PASS, релиз 0.9.0. Этот документ — план следующего исполнения.
