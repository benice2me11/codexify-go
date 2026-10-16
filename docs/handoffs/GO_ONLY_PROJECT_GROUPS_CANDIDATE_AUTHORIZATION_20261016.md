# Go-only Project Groups canary — separate candidate authorization

**Date:** 2026-10-16
**Status:** implemented in development; production cutover **not requested or executed**.

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

## How the risk exception is enforced

- The new `Get-GoOnlyTrialAcceptedRiskEvidence` validates the old-Go scope/hash, exact accepted HOLD-reason categories, fresh controlled `CONTINUE`, unique 2 Go/1 tunnel/0 Rust topology, absence of new unaccepted `HOLD` events and absence of inspector warnings.
- It does **not** return permission to mutate SCM. Without a candidate-specific authorization, the read-only preparer returns `decision=HOLD`, `blockReasons=candidate_specific_approval_required` and `g10Decision=HOLD`.
- `Approve-GoOnlyTrialCandidate.ps1` defaults to read-only `PLAN_ONLY`. A separate `-Authorize -OperatorApproved -ExpectedCandidateSHA256 <64-hex-sha>` creates a new private `candidate-approval.json`, valid for at most 30 minutes, using `FileMode.CreateNew`. No service, tunnel or Task Scheduler changes are made by this authorization step.
- Frozen `Prepare`, `Invoke` and `New-GoOnlyTrialOperatorTask` carry explicit `-UseAcceptedG10` mode, pinned into the protected snapshot. All live checks are repeated immediately before execution. Only a valid, unexpired, candidate-specific record can produce `READY_FOR_INDEPENDENT_OPERATOR_WITH_ACCEPTED_RISK` when technical G10 is HOLD.
- Safety gates **cannot** be waived: running Go owner identity, two Go processes/one tunnel/zero Rust, one pinned Go-only watcher, Rust disabled, exact original SCM/task XML/immutable executable/config hashes, full production config parity (except explicitly added Project Groups catalog), no foreign process or G10 BLOCKED, independent elevated on-demand task, and previous-Go-only recovery.
- The existing strict `LOCAL_READY_FOR_OPERATOR_REVIEW` path remains unchanged. There is no Rust fallback, and transient HTTP 403/429/503 alone cannot trigger binary rollback.

## Next operator action

After review and native CI, create a **new immutable operator snapshot** (e.g. `operator-trial-v6`) from the exact approved source; previous `operator-trial-v5` is a different pinned script version and must not be executed with new switches.

Read-only preparation and readiness:

```powershell
$stage = Join-Path $env:USERPROFILE '.codexify-go\staging\project-groups-v1-cd6b5bd59dea'
$run = Join-Path $env:USERPROFILE '.codexify-go\cutover-v2\20261015T101000Z-go-only-upgrade'
$tx = Join-Path $stage 'operator-trial-v6'
./tools/windows/cutover-v2/Prepare-GoOnlyTrialUpgrade.ps1 -StagingDirectory $stage -CurrentG10RunDirectory $run -TransactionDirectory $tx -UseAcceptedG10
```

Only after a **distinct explicit go/no-go for the new candidate** may the prepared `Approve-GoOnlyTrialCandidate.ps1` be used with `-Authorize -OperatorApproved -ExpectedCandidateSHA256`, followed by `New-GoOnlyTrialOperatorTask.ps1 -UseAcceptedG10 -Register -OperatorApproved` in the appropriate elevated operator context. Registration does not start the task. Starting the independent on-demand task would be a separately authorized production action.

No attempt should treat the old Go acceptance marker as an authorization for `projectGroupsEnabled=true`; actually enabling and testing the hosted Setup v4 is a subsequent, independently verified configuration change. G11 and `v0.9.0` stable release are separate gates.

**Do not include actual private approval records, config JSON, G10 watcher task exports or authentication secrets in Git.**
