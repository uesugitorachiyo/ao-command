# AO Command

AO Command is the read-only operator interface for AO. It turns Mission lifecycle state, portfolio and run status, policy decisions, and stored evidence into terminal or JSON views without becoming another source of domain truth. Use it when an operator needs current status, the next action, or a compact explanation of records owned by other AO components.

## How it fits in AO

- **Primary responsibility:** Read-only operator presentation.
- **Inputs:** Mission records, Atlas and Foundry status, Forge GoalRun evidence, Covenant decisions, AO2 evidence, and Control Plane readbacks.
- **Outputs:** Human-readable status, compact timelines, next-action views, validation summaries, and JSON readbacks.
- **Upstream:** AO Mission, AO Atlas, AO Foundry, AO Forge, AO Covenant, AO2, and AO2 Control Plane.
- **Downstream:** Operators and automation that consume read-only status.

See the
[AO Architecture guide](https://github.com/uesugitorachiyo/ao-architecture)
and the
[AO Command component page](https://github.com/uesugitorachiyo/ao-architecture/blob/main/components/ao-command.md)
for the cross-repository flow.

<!--
Legacy documentation-test compatibility tokens (not rendered):
Deprecated standalone runtime
go run ./cmd/ao-command rsi health
go run ./cmd/ao-command rsi manifest --manifest ../ao-architecture/overview/rsi-claim-evidence-manifest.json
--foundry-candidate ../ao-foundry/tmp/pulse-rsi-verify/rsi-candidate.json
--foundry-next-task ../ao-foundry/tmp/pulse-rsi-verify/rsi-next-improvement-task.json
--forge-retained-gate ../ao-forge/docs/evidence/goals/ao2-weekend-hardening/20260619T180000Z-verification/ao-foundry-rsi-improvement-gate-retention-proof.json
--forge-retained-command-health ../ao-forge/docs/evidence/goals/ao2-weekend-hardening/20260619T180000Z-verification/ao-command-rsi-health-retention-proof.json
--bundle-out tmp/rsi-health-bundle.json
scripts/rsi-evidence-chain-smoke.sh --forge ../ao-forge --foundry ../ao-foundry --covenant ../ao-covenant
mutates_repositories=false
claim_level=bounded_governed_rsi decision=allowed
claim_level=full_autonomous_self_mutating_rsi decision=denied
docs/contracts/rsi-health-v0.1.schema.json
docs/contracts/rsi-health-bundle-v0.1.schema.json
`rsi manifest` reads AO Architecture's
ao2.cp-ao2-rsi-claim-readiness-readback.v1
ao2.rsi-governed-self-change-dry-run.v1
ao2.cp-ao2-rsi-self-change-dry-run-readback.v1
rollback_rehearsal.status=passed
AO2 PR #200
ao-command-rsi-manifest-retention-proof.json
goalrun.architecture_rsi_pin_readback
ao-architecture-rsi-pin-readback.json
rollback-retained.contract.json
covenant.live-self-change-authority.v1
live-self-change-authority.packet.json
AO2 PR #201
schema_valid_for_claim_publish=false
ao2.cp-ao2-rsi-authority-packet-readback.v1
bounded-rsi-improvement-chain-retention-proof.json
examples/full-rsi-claim-boundary/
-->

## Commands

```sh
go run ./cmd/ao-command status --forge ../ao-forge
go run ./cmd/ao-command mission status --status examples/mission/command-status.ready.json
go run ./cmd/ao-command mission next --decision examples/mission/route-decision.ready.json
go run ./cmd/ao-command mission history --history examples/mission/route-history.ready.json
go run ./cmd/ao-command mission history --history examples/mission/route-history.ready.json --route ao-atlas --query "Foundry import" --compact
go run ./cmd/ao-command mission artifacts --manifest examples/mission/artifact-manifest.ready.json
go run ./cmd/ao-command mission dashboard --dashboard examples/mission/dashboard.ready.json
go run ./cmd/ao-command mission dashboard --dashboard examples/mission/dashboard.ready.json --compact
go run ./cmd/ao-command mission readiness --bundle examples/mission/readiness-bundle.ready.json
go run ./cmd/ao-command mission gateway --readback examples/mission/gateway-intent-ledger.ready.json
go run ./cmd/ao-command mission gateway --readback examples/mission/gateway-replay-bundle.ready.json
go run ./cmd/ao-command mission evidence --readback examples/mission/scheduler-recovery-readback.ready.json
go run ./cmd/ao-command mission evidence --readback examples/mission/scheduler-recovery-readback.ready.json --json
go run ./cmd/ao-command mission evidence --readback examples/mission/ledger-compaction-readback.ready.json
go run ./cmd/ao-command mission evidence --readback examples/mission/ledger-compaction-readback.ready.json --json
go run ./cmd/ao-command mission evidence --readback examples/mission/timeline-compaction-readback.ready.json
go run ./cmd/ao-command mission evidence --readback examples/mission/timeline-compaction-readback.ready.json --json
go run ./cmd/ao-command mission aggregate --status examples/mission/command-status.ready.json --atlas-metadata examples/mission/atlas-workgraph-metadata.ready.json --foundry-smoke examples/mission/foundry-e2e-smoke.ready.json
go run ./cmd/ao-command mission aggregate --status examples/mission/command-status.ready.json --atlas-metadata examples/mission/atlas-workgraph-metadata.ready.json --foundry-smoke examples/mission/foundry-e2e-smoke.ready.json --watch --iterations 3 --jsonl
go run ./cmd/ao-command mission aggregate --status examples/mission/command-status.ready.json --atlas-metadata examples/mission/atlas-workgraph-metadata.ready.json --foundry-smoke examples/mission/foundry-e2e-smoke.ready.json --watch --iterations 3 --compact
go run ./cmd/ao-command stack --ledger ../ao-foundry/examples/readiness/active-stack-readiness.ledger.json
go run ./cmd/ao-command control-plane qualification-progress --readback examples/control-plane/windows-qualification-progress.ready.json
go run ./cmd/ao-command atlas status --status ../ao-foundry/examples/contract-fixtures/valid/foundry-atlas-status-v0.1.json
go run ./cmd/ao-command atlas authority-ladder --mission-status examples/authority-ladder/status.blocked.json
go run ./cmd/ao-command pulse status --preflight ../ao-foundry/examples/pulse-overnight-start-gate/ready.intake-preflight.json --lifecycle ../ao-foundry/examples/pulse-lifecycle/ready-to-start-next-slice.json --start-gate ../ao-foundry/examples/pulse-overnight-start-gate/ready.json
go run ./cmd/ao-command complex-refactor status --summary examples/complex-refactor/ready-summary.json
go run ./cmd/ao-command live-mutation status --authority examples/live-mutation/covenant-authority.ready.json --request examples/live-mutation/foundry-request.ready.json --forge-plan examples/live-mutation/forge-plan.ready.json --ao2-packet examples/live-mutation/ao2-packet.ready.json --isolation examples/live-mutation/worktree-isolation.ready.json --rollback examples/live-mutation/rollback-rehearsal.ready.json --kill-switch examples/live-mutation/kill-switch.armed.json
go run ./cmd/ao-command live-mutation status --authority examples/live-mutation/covenant-authority.low-risk-dry-run-ready.json --request examples/live-mutation/foundry-request.low-risk-dry-run-ready.json --forge-plan examples/live-mutation/forge-plan.low-risk-dry-run-ready.json --ao2-packet examples/live-mutation/ao2-packet.low-risk-dry-run-ready.json --isolation examples/live-mutation/worktree-isolation.low-risk-dry-run-ready.json --rollback examples/live-mutation/rollback-rehearsal.low-risk-dry-run-ready.json --kill-switch examples/live-mutation/kill-switch.armed.json --sentinel-hold examples/live-mutation/sentinel-hold.low-risk-code-clear.json
go run ./cmd/ao-command live-mutation approval --request examples/live-docs-approval/request.json --ticket examples/live-docs-approval/ticket-approved.json
go run ./cmd/ao-command live-mutation pr-rehearsal --gate examples/live-docs-pr-rehearsal/gate-ready.json
go run ./cmd/ao-command next --forge ../ao-forge
go run ./cmd/ao-command goals --forge ../ao-forge --goal-run ../ao-forge/examples/goals/ao2-weekend-hardening.goal-run.json
go run ./cmd/ao-command evidence --forge ../ao-forge --schema docs/contracts/production-readiness-audit-v0.1.schema.json --document /tmp/ao-forge-production-readiness.json
```

Use `--json` on any command for machine-readable output when available.
`status` reports the AO Forge readiness percentage, gate count, required next
action count, derived `production_ready` decision, `operator_mode=read_only`,
and release governance state.

`mission status` reads AO Mission's `ao.command.mission-status.v0.1` readback
and reports the mission route, phase, next action, and authority boundary. It
also preserves an optional `correlation_id` for objective correlation when it
matches `[A-Za-z0-9][A-Za-z0-9._:-]{0,127}`. It rejects any packet that claims
execution, approval, or repository mutation authority.

`mission next` reads AO Mission's `ao.mission.route-decision.v0.1` next-action
readback and reports the next route, reason, and exact next action in
`operator_mode=read_only`. It rejects any packet that claims execution,
approval, or repository mutation authority.

`mission history` reads AO Mission route-history exports and reports route
count, latest route, and exact next action in `operator_mode=read_only`. It
rejects any route-history entry that claims execution, approval, or repository
mutation authority. Compact timeline output can be narrowed with `--route`,
`--status-filter`, and `--query` without changing read-only authority.

`mission artifacts` reads AO Mission's `ao.mission.artifact-manifest.v0.1`
artifact manifest and reports the artifact count and refs in
`operator_mode=read_only`. It rejects any manifest that claims execution,
approval, or repository mutation authority.

`mission dashboard` reads AO Mission's `ao.mission.dashboard-readback.v0.1`
compact operator readback and reports current route, recent-event count, and
event index digest in `operator_mode=read_only`. It rejects any dashboard packet
that claims execution, approval, or repository mutation authority. Use
`--compact` for one-screen long-run Mission status readback with mission status,
route, latest route, event count, exact next action, and authority flags.

`mission readiness` reads AO Mission's
`ao.mission.readiness-bundle-readback.v0.1` and reports repo readiness counts in
`operator_mode=read_only`. It rejects any readiness packet that claims
execution, approval, or repository mutation authority.

`mission gateway` reads AO Mission Telegram/A2A gateway replay or
`ao.mission.gateway-intent-ledger.v0.1` readbacks, including
`ao.mission.gateway-replay-bundle-readback.v0.1`, and reports intent counts in
`operator_mode=read_only`. It rejects any gateway replay or intent record that
claims execution, approval, or repository mutation authority; Telegram and A2A
intents cannot grant mutation authority.

`mission evidence` reads AO Mission scheduler-recovery, ledger-compaction, and timeline-compaction
readbacks and reports them as `ao.command.mission-evidence.v0.1` in
`operator_mode=read_only`. It rejects any evidence packet that claims scheduling,
execution, approval, repository mutation, provider, release, credential,
direct-main, or concurrent mutation authority. Recovery and compaction evidence
can support readback and Atlas provenance, but it does not authorize work.

`mission aggregate` reads AO Mission command status, AO Atlas Mission workgraph
metadata, and AO Foundry Mission e2e smoke output into
`ao.command.mission-aggregate.v0.1`. It binds mission ID, Atlas workgraph ID,
primary Mission provenance, and Foundry smoke status while preserving
`safe_to_execute=false`, `executes_work=false`, `approves_work=false`, and
`mutates_repositories=false`.
With `--watch --jsonl`, it emits one bounded read-only aggregate object per
iteration. With `--watch --compact`, it emits a terse operator summary for
terminal polling. Watch modes do not schedule, approve, execute, mutate
repositories, publish, call providers, or widen Mission authority.

`stack` reads the AO Foundry active-stack readiness ledger and reports the
active repository count, release handoff gates, `operator_mode=read_only`, and
`orchestration_owner=ao-foundry`. It does not schedule work, mutate branches,
publish releases, or write control-plane records.

`control-plane qualification-progress` reads AO2 Control Plane Windows
qualification progress readbacks and reports request state, shard counts, cache
counters, bounded ETA, and global deadline in `operator_mode=read_only`. It
rejects any vector that claims release readiness, approval, credential use,
provider calls, repository mutation, Control Plane mutation, release, or deploy
authority.

`atlas status` reads AO Foundry's `ao.foundry.atlas-status.v0.1` observer
artifact and reports Atlas stack-instance/workgraph readback in AO Command's
read-only operator format. It requires `mode=fixture_only_readback`,
`schedules_work=false`, `executes_work=false`, and `approves_work=false`, then
reports `atlas_authority=compile_only`, `operator_mode=read_only`, and
`mutates_repositories=false`. It does not schedule work, execute work, approve
claims, call providers, mutate repositories, or replace AO Foundry scheduling.

`atlas authority-ladder` reads AO Atlas mission status evidence containing the
mutation-class authority ladder. It reports the current proven live class, the
next denied class, blockers, required evidence, do-not-advance gates, and
denial reasons for higher classes while preserving
`operator_mode=read_only`, `schedules_work=false`, `executes_work=false`, and
`mutates_repositories=false`. It does not schedule, request, approve, execute,
or promote mutation work.

`pulse status` reads AO Foundry's Pulse intake preflight, PR lifecycle state,
and overnight start gate artifacts. It reports whether the autonomous loop may
start, block for Blueprint clarification, or stop on failed evidence while
keeping `operator_mode=read_only` and `mutates_repositories=false`. It does not
start loops, create branches, merge PRs, mutate repositories, publish releases,
upload artifacts, call providers, or replace AO Foundry Pulse gates.

`complex-refactor status` reads AO Foundry/Atlas complex-refactor rehearsal
summary evidence and reports task counts, the next recommended factory task,
blocked-node repair status, needs-context repack status, first failing check,
and blocking next actions in AO Command's read-only format. It fail-closes
unless the summary is fixture-only, digest-bound, public-safe, and explicitly
denies scheduling, execution, approval, provider calls, and repository mutation.

`live-mutation status` reads governed live-mutation readiness evidence from
Covenant, Foundry, Forge, AO2, Foundry worktree isolation, Foundry rollback
rehearsal, and an operator kill-switch state artifact. It reports whether the
first tiny live mutation class is ready to request, blocked, or failed while
preserving `operator_mode=read_only`, `mutates_repositories=false`, and no
scheduling, execution, approval, provider, release, or publish authority. This
is readback only; it does not grant or perform live mutation.
When the evidence carries class fields, the readback also reports
`current_mutation_class`, `next_mutation_class`, `safe_to_request`,
`safe_to_execute=false`, required evidence, and denied higher classes. When `--sentinel-hold` is supplied, Command reads the AO Sentinel
`ao.sentinel.live-mutation-hold.v0.1` packet, reports the class hold verdict,
and blocks the status if Sentinel still holds. This is readback only and does
not grant scheduling, execution, approval, or mutation authority.

`live-mutation approval` reads the first docs-only live mutation approval
request and Covenant ticket. It reports `safe_to_request`, `safe_to_execute`,
approval state, request id, ticket id, and read-only boundaries. It never calls
Covenant, starts a branch, mutates a repository, publishes, uploads, or calls a
provider. If the ticket is pending, denied, stale, consumed, missing an
approver, or mismatched to the request, the readback reports the blocker instead
of trying to repair or override the approval state.

`live-mutation pr-rehearsal` reads AO Foundry's
`ao.foundry.live-docs-pr-rehearsal-gate.v0.1` decision. It reports whether the
first docs-only branch/PR rehearsal may start or whether the operator must
request approval first. AO Command remains read-only: it does not create
branches, create worktrees, open PRs, merge, mutate repositories, execute work,
approve work, call providers, publish, upload, tag, or release.

Reading `safe_to_execute=true` is not the same as executing. AO Command only
explains that the exact-scope first docs-only PR rehearsal gate is ready for an
operator-controlled action. It does not grant broad live mutation authority,
does not approve fully unsupervised complex repository mutation, and does not
convert readback evidence into permission.

For an existing release tag, rehearse from an AO Forge checkout whose HEAD
matches that tag:

```sh
git -C ../ao-forge worktree add /tmp/ao-forge-v0.1.3 v0.1.3
go run ./cmd/ao-command rehearse --forge /tmp/ao-forge-v0.1.3 --tag v0.1.3 --out /tmp/ao-command-v013-rehearsal
```

## Safety

- Published only after the operator-approved public-readiness audit passed.
- Read-only by default.
- AO Forge remains the source of truth for readiness percentages, release gates,
  GoalRun state, retained evidence, and Covenant decisions.
- AO2 is the governed execution path; AO Command reads evidence about that path
  rather than invoking deprecated standalone runtime or operator repositories.
- `atlas status` reads AO Foundry Atlas observer evidence and does not schedule,
  execute, approve, call providers, or mutate repositories.
- `atlas authority-ladder` reads Atlas mission-status evidence and does not
  schedule, request, approve, execute, promote, or mutate repository work.
- `pulse status` reads AO Foundry Pulse gate evidence and does not start loops,
  merge PRs, create branches, call providers, publish, or mutate repositories.
- `blueprint-atlas-foundry status` reads the Blueprint pack status from Atlas,
  the Atlas import status from Foundry preflight evidence, and the Foundry gate
  status without starting, approving, or executing work.
- `rehearse` only runs AO Forge release-preview dry-run evidence and then
  inspects the produced audit.
- Dangerous writes are intentionally out of scope for v0.1.
- CI does not upload artifacts by default.

## Foundry Boundary

The current stack boundary is:

1. AO Blueprint: requirements interview, sufficiency audit, blueprint pack, and
   build-authorization front door.
2. AO Atlas: stack-instance/workgraph/context-pack compile evidence over one
   shared AO toolchain.
3. AO Foundry: persistent active-stack operations ledger, Atlas status
   observer, release handoff, and cross-repo readiness loop.
4. AO Forge: trusted factory brain, release gates, GoalRun state, readiness, and
   verified evidence.
5. AO Command v0.1: human/operator command center over Blueprint, Atlas,
   Foundry, Forge, Covenant, AO2, and readback evidence, including the
   Blueprint -> Atlas -> Foundry status chain for oversized and live-mutation
   work.
6. AO Covenant, AO2, and ao2-control-plane: policy, governed execution, and
   evidence readback.

Historical AO Command Foundry design notes remain in
`docs/design/AO-COMMAND-FOUNDRY.md`, but they are not the active ownership
model. New persistent operations work belongs in AO Foundry unless a future
AO Covenant-approved design moves that boundary.

## Verification

```sh
go test ./...
go vet ./...
go build -o bin/ao-command ./cmd/ao-command
go run ./cmd/ao-command atlas status --status ../ao-foundry/examples/contract-fixtures/valid/foundry-atlas-status-v0.1.json --json
go run ./cmd/ao-command pulse status --preflight ../ao-foundry/examples/pulse-overnight-start-gate/ready.intake-preflight.json --lifecycle ../ao-foundry/examples/pulse-lifecycle/ready-to-start-next-slice.json --start-gate ../ao-foundry/examples/pulse-overnight-start-gate/ready.json --json
# After generating Foundry's Blueprint -> Atlas -> Pulse dry-run evidence:
go run ./cmd/ao-command blueprint-atlas-foundry status --atlas-blueprint-import ../ao-foundry/examples/atlas/blueprint-import.low-risk-code.json --preflight ../ao-foundry/docs/evidence/pulse/blueprint-atlas-pulse-e2e-local/ready/pulse-intake-preflight.json --foundry-gate ../ao-foundry/docs/evidence/pulse/blueprint-atlas-pulse-e2e-local/ready/pulse-overnight-start-gate.json --json
scripts/ao-command-smoke.sh --forge ../ao-forge --foundry ../ao-foundry --out tmp/ao-command-smoke
scripts/release-preview-dry-run.sh --forge ../ao-forge --out tmp/release-preview --tag v0.1.0-preview
go run ./cmd/ao-command evidence --forge ../ao-forge --schema "$PWD/docs/contracts/release-preview-audit-v0.1.schema.json" --document "$PWD/tmp/release-preview/release-preview-audit.json"
scripts/install-verify-dry-run.sh --forge ../ao-forge --out tmp/install-verify
go run ./cmd/ao-command evidence --forge ../ao-forge --schema "$PWD/docs/contracts/install-verify-audit-v0.1.schema.json" --document "$PWD/tmp/install-verify/install-verify-audit.json"
scripts/release-governance-dry-run.sh --out tmp/release-governance --tag v0.1.0 --release-preview-audit tmp/release-preview/release-preview-audit.json --install-verify-audit tmp/install-verify/install-verify-audit.json
go run ./cmd/ao-command evidence --forge ../ao-forge --schema "$PWD/docs/contracts/release-governance-audit-v0.1.schema.json" --document "$PWD/tmp/release-governance/release-governance-audit.json"
scripts/production-readiness-audit.sh --repo uesugitorachiyo/ao-command --forge ../ao-forge --foundry ../ao-foundry --covenant ../ao-covenant --architecture ../ao-architecture --out tmp/production-readiness-audit.json
go run ./cmd/ao-command evidence --forge ../ao-forge --schema "$PWD/docs/contracts/production-readiness-audit-v0.1.schema.json" --document "$PWD/tmp/production-readiness-audit.json"
scripts/verify-branch-protection.sh
```

Historical private-repo operating guardrails are tracked in
`docs/operations/PRIVATE-REPO-GUARDRAILS.md`.
The pre-publication operator gate and publication evidence are tracked in
`docs/operations/PUBLICATION-CHECKLIST.md`.
Production Readiness is tracked in
`docs/operations/PRODUCTION-READINESS.md`, with publication evidence in
`docs/operations/PUBLICATION-RECORD-2026-06-19.md`.
Branch protection requirements and drift verification are tracked in
`docs/operations/BRANCH-PROTECTION.md`.
The AO Command readiness audit contract is tracked in
`docs/contracts/production-readiness-audit-v0.1.schema.json`.
The read-only AO Command release-preview dry-run contract is tracked in
`docs/contracts/release-preview-audit-v0.1.schema.json`.
The install verification dry-run contract is tracked in
`docs/contracts/install-verify-audit-v0.1.schema.json`.
The release governance dry-run contract is tracked in
`docs/contracts/release-governance-audit-v0.1.schema.json`.
Public-safe retained evidence rules are tracked in
`docs/operations/RETAINED-EVIDENCE.md` and
`docs/operations/public-provenance-manifest.json`.
The v0.1.0 operator closeout is tracked in
`docs/release/V0.1.0-OPERATOR-CLOSEOUT.md`.

## License

AO Command is licensed under `Apache-2.0`. See `LICENSE`.
