# Go-only Project Groups canary — separate candidate authorization

**Operational follow-up (2026-10-16 10:59 Moscow):** The user subsequently gave explicit authorization for the exact candidate SHA256, and the independently scheduled Go→Go binary switch completed successfully. The operator reported `GO_UPGRADED`; the new Go-only watcher is Running. **Project Groups remains disabled.** See `docs/handoffs/WINDOWS_PROJECT_GROUPS_BINARY_CUTOVER_20261016.md` for the authoritative live result and remaining acceptance gates. Historical pre-cutover `HOLD` and approval requirements below are retained as the transaction's audit context.

**Date:** 2026-10-16
**Status:** implemented and merged through PR #33, `main` at `893f5ec6959354b50ae9c0f02d06ce3816771f1d`. Native PR CI [38864012738](https://github.com/benice2me11/codexify-go/actions/runs/38864012738) and main CI [38864561202](https://github.com/benice2me11/codexify-go/actions/runs/38864561202) both passed **11/11**. Production cutover **not requested or executed**.

## Distinguish the three independent decisions

1. **Historical technical G10 result**: remains `HOLD` because of incomplete contiguous observation, reboot-related gaps and two `tunnel_child_recovery` decisions. `Assess-ProjectGroupsCanaryReadiness.ps1` still reports the actual technical evidence. No thresholds or log files are rewritten.
2. **User's operator decision for the currently deployed Go**: `G10_OPERATOR_ACCEPTED_WITH_DEVIATIONS`. The signed-off scope is only the already running binary SHA256 `8BAA786E6BEFDCE5C746419B5A9277D3531A824938396DF8DDDA3FB7200B5880`, recorded privately in `g10-operator-acceptance-20261016.json` with SHA256 `0BA09B150E44FD57C1C41005ECE2290775A1AA7EC559311B584ABD7C8D74E42B`.
3. **Separate explicit go/no-go for a different binary**: requires a new candidate-specific, time-limited authorization bound to the staged Go SHA256, config SHA256, source commit and previous G10 acceptance marker SHA256. It is **not** implied by decision #2 or general instructions to continue development.

## Candidate and protected staging

The previously isolated compiled candidate remains:

- Source commit `cd6b5bd59dea0be35e3107446dfddce322f41ad2`.
- Candidate binary SHA256 `F5AF36CEA944688CB8BF6145F8047722CA23F991E077DA1D7897D74B2D3A0DB2`.
- Staged config SHA256 `9453FF3D13DCD97287112D13AB5A3E8ABDC64AAAC72C910AD238AA339D6D72F1`.
- New group `job-search` contains `work`, `hh`, `habr`, all in source mode; `projectGroupsEnabled=false`.
- Previous verified Go and its original config, G10 observer XML, private bindings and tunnel secrets are preserved outside the Git checkout under `%USERPROFILE%\.codexify-go\staging\project-groups-v1-cd6b5bd59dea`.
- The private **`operator-trial-v6` was prepared after green PR CI**, from exact operator source commit `898ef12bda1ae52866ab46a35292f3cad7c0e441`, with **10/10** frozen operator-file hashes verified and **10/10** Git-normalized files matching merged `main`. Previous-Go executable/profile snapshots and risk-record SHA256 match the manifest. The old `operator-trial-v5` executor was made inert after verifying the v6 successor, preserving its previous snapshots only for forensics.
- Frozen read-only `Prepare`, `Invoke`, `Approve` and task-builder checks all returned the same expected status: **previous Go accepted, technical G10 HOLD, `candidate_specific_approval_required`**, no candidate approval record, no operator task registered and no service changes.

## How the risk exception is enforced

- The new `Get-GoOnlyTrialAcceptedRiskEvidence` validates the old-Go scope/hash, exact accepted HOLD-reason categories, fresh controlled `CONTINUE`, unique 2 Go/1 tunnel/0 Rust topology, absence of new unaccepted `HOLD` events and absence of inspector warnings.
- It does **not** return permission to mutate SCM. Without a candidate-specific authorization, the read-only preparer returns `decision=HOLD`, `blockReasons=candidate_specific_approval_required` and `g10Decision=HOLD`.
- `Approve-GoOnlyTrialCandidate.ps1` defaults to read-only `PLAN_ONLY`. A separate `-Authorize -OperatorApproved -ExpectedCandidateSHA256 <64-hex-sha>` creates a new private `candidate-approval.json`, valid for at most 30 minutes, using `FileMode.CreateNew`. No service, tunnel or Task Scheduler changes are made by this authorization step.
- Frozen `Prepare`, `Invoke` and `New-GoOnlyTrialOperatorTask` carry explicit `-UseAcceptedG10` mode, pinned into the protected snapshot. All live checks are repeated immediately before execution. Only a valid, unexpired, candidate-specific record can produce `READY_FOR_INDEPENDENT_OPERATOR_WITH_ACCEPTED_RISK` when technical G10 is HOLD.
- Safety gates **cannot** be waived: running Go owner identity, two Go processes/one tunnel/zero Rust, one pinned Go-only watcher, Rust disabled, exact original SCM/task XML/immutable executable/config hashes, full production config parity (except explicitly added Project Groups catalog), no foreign process or G10 BLOCKED, independent elevated on-demand task, and previous-Go-only recovery.
- The existing strict `LOCAL_READY_FOR_OPERATOR_REVIEW` path remains unchanged. There is no Rust fallback, and transient HTTP 403/429/503 alone cannot trigger binary rollback.

## Next operator action

The exact approved operator scripts are already frozen in private **`operator-trial-v6`**. Do not re-run `-Prepare` over that directory or use an earlier snapshot. If the running Go owner, watcher identity, G10 status, candidate files or source configuration changes, reprepare into a **new versioned** transaction directory after appropriate verification.

Read-only preparation and readiness:

```powershell
$stage = Join-Path $env:USERPROFILE '.codexify-go\staging\project-groups-v1-cd6b5bd59dea'
$run = Join-Path $env:USERPROFILE '.codexify-go\cutover-v2\20261015T101000Z-go-only-upgrade'
$tx = Join-Path $stage 'operator-trial-v6'
& (Join-Path $tx 'operator\Prepare-GoOnlyTrialUpgrade.ps1') -StagingDirectory $stage -CurrentG10RunDirectory $run -TransactionDirectory $tx -UseAcceptedG10
& (Join-Path $tx 'operator\Invoke-GoOnlyTrialUpgrade.ps1') -TransactionDirectory $tx -UseAcceptedG10
& (Join-Path $tx 'operator\Approve-GoOnlyTrialCandidate.ps1') -TransactionDirectory $tx
& (Join-Path $tx 'operator\New-GoOnlyTrialOperatorTask.ps1') -TransactionDirectory $tx -UseAcceptedG10
```

Only after a **distinct explicit go/no-go for the new candidate** may the prepared `Approve-GoOnlyTrialCandidate.ps1` be used with `-Authorize -OperatorApproved -ExpectedCandidateSHA256`, followed by `New-GoOnlyTrialOperatorTask.ps1 -UseAcceptedG10 -Register -OperatorApproved` in the appropriate elevated operator context. Registration does not start the task. Starting the independent on-demand task would be a separately authorized production action.

No attempt should treat the old Go acceptance marker as an authorization for `projectGroupsEnabled=true`; actually enabling and testing the hosted Setup v4 is a subsequent, independently verified configuration change. G11 and `v0.9.0` stable release are separate gates.

**Do not include actual private approval records, config JSON, G10 watcher task exports or authentication secrets in Git.**
