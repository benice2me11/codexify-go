# Handoff №1 — аудит PR/14 веток, консолидация main и подготовка v0.9.0

**Дата:** 15 октября 2026. **Репозиторий:** https://github.com/benice2me11/codexify-go
**Состояние:** PLAN / NOT EXECUTED. Этот документ НЕ разрешает массовые merge, удаление веток, выпуск тега или замену работающего Windows-сервиса.
**Проверенная база:** main = 7c66828b8c620dfb33a51f1810e8cbec01b9ded0, чистый source checkout Windows; GitHub main защищён.
**Цель:** принять только действительно нужные изменения, устранить остаточные расхождения Linux/Windows, закрыть или архивировать устаревшие refs, затем оценить выпуск стабильного v0.9.0 с проверенными артефактами и без регрессии Windows Go-only cutover.

## A. Подтверждённый baseline

- На GitHub **14 веток, включая main**, то есть 13 дополнительных remote heads на момент аудита. Количество локальных веток больше; их инвентаризировать отдельно. Часть локальных веток занята worktrees.
- **Открытый PR ровно один:** [#3 — Integrate Linux Setup parity](https://github.com/benice2me11/codexify-go/pull/3), Draft, HEAD 9cc7c302, **из форка 1AAk/codexify-go**, базировался на устаревшем e3e9ca1. GitHub сообщил mergeable=false / dirty; 8 изменённых файлов. PR #3 **не является одной из 14 веток целевого репозитория**.
- Все недавние Windows/Setup PR #4, #5, #7–#12, #14, #15 слиты через GitHub, в основном squash. Поэтому команда git branch --merged main НЕ является достаточным доказательством отсутствия уникальных правок.
- Последние релизы GitHub: [v0.8.1](https://github.com/benice2me11/codexify-go/releases/tag/v0.8.1) и v0.8.0, оба от 28.09.2026. Тега v0.9.0 нет. internal/buildinfo/buildinfo.go по умолчанию содержит Version = 0.8.2-dev; release.yml правильно переопределяет Version с помощью -ldflags -X.
- Последний main CI на SHA 7c66828: [успешный run 38721722171](https://github.com/benice2me11/codexify-go/actions/runs/38721722171). Это тесты GitHub Actions, а не закрытие эксплуатационного G10/G11.
- Live Windows: 15.10.2026 13:26 Москва, Go→Go транзакция дала GO_UPGRADED. В работе Setup v3 бинарник из cdf8c98, SHA-256 8BAA786E6BEFDCE5C746419B5A9277D3531A824938396DF8DDDA3FB7200B5880; previous Go сохранён. Rust/Watchdog выключены; новый Go-only G10 watcher наблюдает. На 14:01:08 Москва G10 выдавал CONTINUE, служба была Running, но **24 часа G10 и G11 не приняты**. Не подменять эту проверку более поздними неподтверждёнными выводами.
- На Windows закреплён исходный checkout C:\Users\FoxOS_User\codexify-go, не managed worktree; три исторических worktree: cutover-v2-clean-20260930, g03-checkpoint-20261002, g10-guard-hardening-20261013. Не удалять эти деревья при очистке веток.

### A1. Все 14 remote-веток и предварительное решение

| # | Ветка на benice2me11/codexify-go | HEAD | Статус относительно main / решение |
|---|---|---|---|
| 1 | main | 7c66828 | Защищённая база; сохранять |
| 2 | codex/linux-setup-parity-20261009 | df0180d | **REVIEW/P0**: Linux Setup v5, v1/v2 aliases и тесты; НЕ сливать весь divergent branch |
| 3 | codex/windows-cutover-g03 | 6387f54 | **HOLD**: проверенный исторический Windows-кандидат; PollWatcher перенесён в main (#8), но эта ветка остаётся важным provenance/rollback checkpoint |
| 4 | codex/windows-g10-guard-hardening-20261013 | e4e36a7 | **REVIEW/P0**: четыре дополнительных коммита старого watcher; проверить точечные полезные тесты/форензику, НЕ возвращать Rust-capable rollback в live |
| 5 | docs/g10-go-only-live-result-20261015 | 8d45f69 | **MERGED-SQUASH #15**, cleanup candidate после проверки содержимого |
| 6 | docs/project-groups-plan-20261014 | ed2f278 | **MERGED-SQUASH #4**, план уже в main; cleanup candidate |
| 7 | feat/g10-go-only-upgrade-20261015 | 2d236bf | **MERGED-SQUASH #14**, рабочий Go-only upgrade уже в main; cleanup candidate |
| 8 | feat/g10-offline-live-inspector-20261015 | ec72e4d | **MERGED-SQUASH #11**, cleanup candidate |
| 9 | fix/g03-setup-context-parity-20261014 | c9c3768 | **MERGED-SQUASH #9**, Setup v3 в main; cleanup candidate |
| 10 | fix/g03-tunnel-selfheal-main-20261014 | 7e92527 | **MERGED-SQUASH #8**, G03 PollWatcher в main; cleanup candidate |
| 11 | fix/setup-ui-version-20261015 | 14f1945 | **MERGED-SQUASH #10**, v3 URI/schema в main; cleanup candidate |
| 12 | fix/windows-path-identity-20261014 | 170291d | **MERGED-SQUASH #7**, Windows short-name/junction fix в main; cleanup candidate |
| 13 | fix/workspace-mode-selection-20261014 | 716d117 | **MERGED-SQUASH #5**, Auto/Source/Managed в main; cleanup candidate |
| 14 | test/setup-v3-binary-smoke-20261015 | 9455910 | **MERGED-SQUASH #12**, Windows compiled-binary MCP smoke в main; cleanup candidate |

**MERGED-SQUASH означает предварительное подтверждение PR/назначения изменений, не разрешение немедленно удалять ref.** Для каждой из десяти веток ещё нужно сделать content/patch parity и проверить отсутствие зависимых worktree/PR. Изменения под чужим/сдвинутым базовым коммитом не проверять сравнением всей файловой системы ветки с современным main: это выдаст тысячи ложных «потерь» из-за истории.

### A2. Локальные refs — отдельная инвентаризация

На Windows существуют дополнительные локальные ветки, например backup/pre-github-reconcile-20260928, cutover/windows-v2-minimal, codexify-go/* session bindings, local/g10-offline-inspector-20261015 и три живых worktree. Их нет в списке 14 GitHub-веток. Не выполнять git branch -D и git worktree remove -f без просмотра git worktree list --porcelain, статуса каждой рабочей директории и назначения binding. Проверить также локальные, не опубликованные refs на macOS/Linux.

## B. Два содержательных направления перед очисткой

### B1. Linux Setup parity и PR #3 (P0)

- Ветка целевого репо codex/linux-setup-parity-20261009 содержит Linux-кандидат df0180d и исторический handoff LINUX_SETUP_MAIN_INTEGRATION_HANDOFF_20261009.md. PR #3 приходит **из форка 1AAk**, не из этой ветки; интеграционная ветка в PR базировалась на e3e9ca1, поэтому сейчас конфликтует с main.
- В PR #3 есть Setup tool + conversation-scoped uiContext + tests + Setup v5, а также совместимость legacy resource URI **v1/v2**. В main уже есть проверенные setup/context, Setup **v3**, legacy **v2**, явный Source/Managed, compiled-binary smoke.
- Главная потенциальная дельта: Linux legacy **v1** resource compatibility, некоторые UI race/error tests или hosting behavior. Нельзя копировать целиком internal/ui/resources.go из PR #3 — можно потерять новый picker или schema +setup-v3.
- Сделать таблицу функционального сравнения: setup tool metadata; v1/v2/v3 URI; initial project list; error rendering; uiContext TTL/cross-chat protection; busy click suppression; Linux browser/host; selected Source/Managed; old conversation refresh behavior.
- Создать **новую минимальную интеграционную ветку от свежего main**, если остались уникальные полезные изменения. Переносить только недостающий функционал/тесты с отрицательным/положительным тестом. Запрещено merge всей Linux candidate ветки с её старыми Windows/tunnel/docs.
- PR #3: после классификации обновить или закрыть с ссылкой на интеграционный PR и тесты. Не удалять fork branch — она принадлежит 1AAk; нельзя считать её одной из удаляемых веток benice2me11.
- Проверить native Linux: свежая сборка Setup, legacy v1/v2/v3 cards, новый разговор, Switch на mikrotik и обратно, одновременные A/B conversation, сохранение bindings/systemd --user, test/go vet/race/Node harness.

### B2. Windows G10 guard-hardening (P0)

- Против codex/windows-cutover-g03 добавлены e4e36a7, d25db52, 70fa4a9 (docs), 8c6c8cb. Меняются tools/windows/cutover-v2/Watch-Cutover.ps1, CutoverGuard.psm1 и их тесты, плюс WINDOWS_G10_ROLLBACK_ROOT_CAUSE_20261013.md.
- Это изменения **старого механизма Rust-capable cutover watcher**. Live-путь после PR #14 иной: Watch-GoTrial.ps1 и Invoke-GoOnlyRecovery.ps1, без network-triggered Rust rollback.
- Проверить полезность ограниченного чтения логов, классификации VPN outages, инструментальных ошибок и тестов; портировать только актуальные safeguards в Go-only watcher/тесты по отдельному PR. Не возвращать ArmRollback → Rust, не заменять Go-only task script.
- Исторические документы об инцидентах сохранить. Ветку и G03 checkpoint НЕ удалять до принятого решения по G10/G11 и резервным снимкам.

## C. Фазы выполнения

### R00 — Зафиксировать состояние и создать ledger

- Записать дату/OS/source HEAD, origin URL, git status, полный git branch -a -vv, git worktree list --porcelain, все remote refs и SHA, PR status/base/head, tags/releases, CI/workflow versions, active G10 result и исполняемый Go SHA.
- Запустить на macOS/Linux инвентаризацию локальных refs/worktrees; не считать Windows список исчерпывающим.
- Создать docs/handoffs/BRANCH_RECONCILIATION_LEDGER_20261015.md: для каждого ref status MERGED_EQUIVALENT / UNMERGED_REQUIRED / FORENSIC_HOLD / DELETE_APPROVED / BLOCKED, evidence и проверяющий; до явного решения всё HOLD.
- Не выводить в docs секретные значения tunnel, profiles, хранилища токенов или полный environment.

### R01 — Сверить десять squash-merged refs

- Для каждой записи MERGED-SQUASH подтвердить GitHub merged PR + сравнить изменения соответствующего логического набора файлов/тестов с main: присутствие функций, тестов и эффекта; при необходимости использовать git range-diff, patch-id, git show / git diff по конкретным путям.
- Недоказанное «уже в main» переводить в UNMERGED_REQUIRED и открывать узкий PR; не делать blanket merge с устаревшей базой.
- Зафиксировать в ledger commit и evidence. Ни одну remote-ветку не удалять до принятия отдельно.

### R02 — Разобрать PR #3 и G10 hardening

- Использовать B1/B2; запускать dry-run merge-tree или diff в выделенных временных worktree, **не** в работающем source checkout.
- После узких изменений повторить tests/go vet/race, Node UI и compiled-binary smoke, MCP tool catalog/schema checks, отсутствие orphan/sessions burst.
- Проверить пользовательский GitHub fork и права перед любым обновлением PR #3.

### R03 — Операционная готовность стабильной версии

- Граница G10: новая Go-only сборка установлена 15.10.2026 в 13:26 Москва; 24-часовое окно может оцениваться не ранее **16.10 после 13:26**, при полной непрерывности метрик; перезапуски, observation gaps и host outages отдельно классифицируются.
- Граница G11: только после G10 PASS. Сверить исходные требования WINDOWS_CUTOVER_V2_ACCEPTANCE_20260930.md и GO_TO_GO_G10_TRANSACTION_20261015.md. При HOLD/BLOCKED нельзя называть production acceptance закрытой.
- Linux: native systemd --user install/start/stop/restart, tunnel identity, upstream MCP, binding persistence, compiled Setup UI, startup/logout/reboot в предусмотренной приёмке.
- macOS: native launchd release binary + configuration checks; cross-build сам по себе **не** нативная приёмка.
- Windows: нового single-owner Go нельзя автоматически заменить release binary без отдельного Go→Go operator gate и сохранения Go-only watcher; HTTP 403 не должен запускать Rust.
- Результаты CI — необходимое, но не достаточное свидетельство. Release может быть отложен; RC допустим раньше стабильного тега.

### R04 — Подготовка RC 0.9.0-rc.1 (без публикации stable)

- Из main после R02 и зелёного CI выполнить **workflow_dispatch Release Candidate** с явными ref=<проверенный SHA/ветка> и version=0.9.0-rc.1. Не использовать дефолт 0.8.2-rc.1.
- Проверить 6 нативных платформ/архитектур (Windows/Linux/macOS × amd64/arm64), version через -ldflags, распаковку и запуск smoke на Windows/Linux/macOS amd64, checksums и корректную структуру архивов. Дополнительно проверить Windows Setup v3 native binary smoke, Go-only upgrade fixtures, Linux systemd/launchd smoke в своих ОС.
- Проверить GitHub artifacts retention, версии и архитектуры, package manifests, отсутствие секретов и посторонних файлов. Не выдавать SHA dev-candidate за SHA release binary: release создаёт новый бинарник с version=0.9.0.
- Release Candidate workflow публикует artifacts для проверки, **не** создаёт стабильный GitHub Release.

### R05 — Решение о v0.9.0

- GO возможен только при R00–R04 PASS, G10/G11 в принятом состоянии либо при отдельно согласованном release scope с честно зафиксированными эксплуатационными ограничениями; риск скрывать нельзя.
- В release notes перечислить: Go-only Windows cutover и восстановление tunnel-child, Setup v3 + Source/Managed switching, Windows path identity fix, Linux/macOS service work, diagnostics and CI, security caveat trusted exec (not OS sandbox), upgrade/rollback steps. **Project Groups в этой сборке НЕ реализованы.**
- docs README/ARCHITECTURE/config.example должны соответствовать реально выпускаемому коду; проверить требования Go toolchain (go.mod), Go buildinfo fallback и совместимость updater.
- Файл .github/workflows/release.yml **автоматически публикует GitHub Release при push тега формата vX.Y.Z**. Тег v0.9.0 создавать только после отдельного GO/Release Approval: он не является безвредной меткой.
- Не переписывать/перемещать уже опубликованный stable tag; tag должен точно указывать на принятый commit. Проверить GitHub Actions Release после тега, итоговые checksums и ссылки.
- Текущий workflow рассчитывает **11 release assets**: 2 Windows ZIP + 4 Unix tar.gz + 4 Unix совместимых ZIP + checksums.txt. Проверить реальный asset count, имена, SHA-256 и наличие обоих goos/goarch.
- При сбое стабильной публикации фиксировать статус отдельно, не утверждать релиз завершённым.

### R06 — Очистка после подтверждения эквивалентности

- Удалять remote refs только из DELETE_APPROVED и только по **точному имени и сохранённому SHA**; до этого — архивировать решения в ledger. Никаких массовых фильтров и force delete.
- Проверить, что branch не является источником открытого PR, незаменимого rollback checkpoint, dirty worktree или связанного conversation.
- Сначала закрыть/обновить PR #3, решить Linux candidate и G10 forensic ветки. Если остаются блокеры, сохранить их и честно зафиксировать итоговое количество веток вместо формальной цели «оставить только main».
- Удалять локальные worktree/ветки только после сохранения изменений и проверки git status каждого дерева. Rust snapshot и Go binary candidate не затрагивать.
- После cleanup заново выполнить github branches inventory, PR list, git status на Windows/Linux/macOS и сверить итог с ledger.

## D. Evidence/acceptance gates

| Gate | Критерий PASS | Сейчас |
|---|---|---|
| R-A00 | Все 14 веток + локальные refs/worktrees/PR/tag/CI в ledger со SHA | PARTIAL — инвентаризация Windows/GitHub есть |
| R-A01 | Каждый squash-merged ref имеет доказательство функциональной эквивалентности в main | NOT RUN |
| R-A02 | Linux Setup #3 разобран по дельте, v1/v2/v3 и контексты протестированы, конфликт разрешён | BLOCKED/REVIEW |
| R-A03 | G10 hardening классифицирован без возвращения Rust rollback | BLOCKED/REVIEW |
| R-A04 | main чистый, все принятые интеграции проходят native Go/Node/CI | NOT RUN для финального candidate |
| R-A05 | G10/G11 и Linux/macOS runtime readiness документированы | G10 RUNNING, остальное не подтверждено |
| R-A06 | 0.9.0-rc.1 assets/compatibility/test/checksums прошли | NOT RUN |
| R-A07 | Релиз v0.9.0 одобрен, tag SHA/Release/11 assets/notes проверены | NOT RUN — релиз не опубликован |
| R-A08 | Remote/local ветки очищены только по DELETE_APPROVED; backups/PR/worktrees целы | NOT RUN |

## E. Команды только для инвентаризации

~~~bash
git status --short --branch
git fetch --prune origin
git branch -a -vv
git worktree list --porcelain
git for-each-ref refs/remotes/origin --format='%(refname:short) %(objectname)'
git tag -l --sort=-version:refname
git log --oneline origin/main..origin/codex/windows-g10-guard-hardening-20261013
git diff --name-status origin/codex/windows-cutover-g03 origin/codex/windows-g10-guard-hardening-20261013 -- tools/windows/cutover-v2
~~~

GitHub source of truth: https://github.com/benice2me11/codexify-go/branches ; PR #3 — https://github.com/benice2me11/codexify-go/pull/3 ; релизы — https://github.com/benice2me11/codexify-go/releases .

**Рекомендация следующему агенту:** R00 → R01 → R02 → R03 → RC; только затем решение о stable v0.9.0 и удаление refs. Release scope не расширять незавершёнными Project Groups. Все изменения готовить отдельными PR; обновление существующего Windows Go — только Go→Go с отдельной контролируемой приёмкой.
