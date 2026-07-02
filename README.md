# AO Command

AO Command is the read-only operator command surface for the AO stack. It makes
AO Atlas, AO Foundry, AO Forge, AO2, ao2-control-plane, and AO Covenant evidence
inspectable from one daily command center.

## AO Stack Architecture

This repository is part of the AO agent orchestration stack. Start with the
central architecture guide at
[uesugitorachiyo/ao-architecture](https://github.com/uesugitorachiyo/ao-architecture);
the AO Command-specific architecture page is
[ao-command](https://github.com/uesugitorachiyo/ao-architecture/tree/main/ao-command).

## Status

v0.1 is a public scaffold after passing the AO Command publication audit. It
reads AO Forge production-readiness, GoalRun, release-preview, and contract
evidence through AO Forge-owned commands. It does not publish releases, promote
production, mutate provider state, or replace AO Forge policy decisions.

AO Command's live stack boundary is AO Blueprint for requirements sufficiency
and build-authorization evidence, AO Atlas for stack-instance/workgraph
compile evidence, AO Foundry for active-stack and Atlas observer status, AO
Forge for readiness and GoalRun truth, AO2 for governed execution,
ao2-control-plane for evidence readback, and AO Covenant for allow, deny, and
block decisions. Deprecated standalone runtime, operator, conductor, and
subscription-backed swarm surfaces are out of scope.
The newer assurance families extend the read-only view: AO Arena supplies
benchmark promotion gates, AO Crucible supplies hardening gates, AO Sentinel
supplies regression and safety verdicts, and AO Promoter supplies dry-run
promotion gates.

AO Foundry is the persistent engineering operations layer for many repos,
branches, releases, dependency updates, docs drift, CI health, security
reviews, and roadmap slices. AO Command reads AO Foundry's active-stack ledger
and AO Forge evidence for humans; it does not become the factory brain or the
operations scheduler. v0.1 keeps the command surface read-only so AO Forge
remains the trusted factory brain and AO Foundry remains the orchestration
owner.

## Commands

```sh
go run ./cmd/ao-command status --forge ../ao-forge
go run ./cmd/ao-command stack --ledger ../ao-foundry/examples/readiness/active-stack-readiness.ledger.json
go run ./cmd/ao-command atlas status --status ../ao-foundry/examples/contract-fixtures/valid/foundry-atlas-status-v0.1.json
go run ./cmd/ao-command atlas authority-ladder --mission-status examples/authority-ladder/status.blocked.json
go run ./cmd/ao-command pulse status --preflight ../ao-foundry/examples/pulse-overnight-start-gate/ready.intake-preflight.json --lifecycle ../ao-foundry/examples/pulse-lifecycle/ready-to-start-next-slice.json --start-gate ../ao-foundry/examples/pulse-overnight-start-gate/ready.json
go run ./cmd/ao-command complex-refactor status --summary examples/complex-refactor/ready-summary.json
go run ./cmd/ao-command live-mutation status --authority examples/live-mutation/covenant-authority.ready.json --request examples/live-mutation/foundry-request.ready.json --forge-plan examples/live-mutation/forge-plan.ready.json --ao2-packet examples/live-mutation/ao2-packet.ready.json --isolation examples/live-mutation/worktree-isolation.ready.json --rollback examples/live-mutation/rollback-rehearsal.ready.json --kill-switch examples/live-mutation/kill-switch.armed.json
go run ./cmd/ao-command live-mutation status --authority examples/live-mutation/covenant-authority.low-risk-dry-run-ready.json --request examples/live-mutation/foundry-request.low-risk-dry-run-ready.json --forge-plan examples/live-mutation/forge-plan.low-risk-dry-run-ready.json --ao2-packet examples/live-mutation/ao2-packet.low-risk-dry-run-ready.json --isolation examples/live-mutation/worktree-isolation.low-risk-dry-run-ready.json --rollback examples/live-mutation/rollback-rehearsal.low-risk-dry-run-ready.json --kill-switch examples/live-mutation/kill-switch.armed.json --sentinel-hold examples/live-mutation/sentinel-hold.low-risk-code-clear.json
go run ./cmd/ao-command live-mutation approval --request examples/live-docs-approval/request.json --ticket examples/live-docs-approval/ticket-approved.json
go run ./cmd/ao-command live-mutation pr-rehearsal --gate examples/live-docs-pr-rehearsal/gate-ready.json
go run ./cmd/ao-command rsi health --arena-gate ../ao-arena/tmp/arena-promotion-gate.json --crucible-gate ../ao-crucible/tmp/crucible-hardening-gate.json --sentinel-verdict ../ao-sentinel/tmp/sentinel-verdict.json --promoter-gate ../ao-promoter/tmp/promotion-gate.json --foundry-gate ../ao-foundry/tmp/pulse-rsi-verify/rsi-improvement-gate.json --foundry-candidate ../ao-foundry/tmp/pulse-rsi-verify/rsi-candidate.json --foundry-next-task ../ao-foundry/tmp/pulse-rsi-verify/rsi-next-improvement-task.json --forge-retained-gate ../ao-forge/docs/evidence/goals/ao2-weekend-hardening/20260619T180000Z-verification/ao-foundry-rsi-improvement-gate-retention-proof.json --forge-retained-candidate ../ao-forge/docs/evidence/goals/ao2-weekend-hardening/20260619T180000Z-verification/ao-foundry-rsi-candidate-retention-proof.json --forge-retained-next-task ../ao-forge/docs/evidence/goals/ao2-weekend-hardening/20260619T180000Z-verification/ao-foundry-rsi-next-improvement-task-retention-proof.json --forge-retained-command-health ../ao-forge/docs/evidence/goals/ao2-weekend-hardening/20260619T180000Z-verification/ao-command-rsi-health-retention-proof.json --bundle-out tmp/rsi-health-bundle.json
go run ./cmd/ao-command rsi manifest --manifest ../ao-architecture/overview/rsi-claim-evidence-manifest.json
go run ./cmd/ao-command next --forge ../ao-forge
go run ./cmd/ao-command goals --forge ../ao-forge --goal-run ../ao-forge/examples/goals/ao2-weekend-hardening.goal-run.json
go run ./cmd/ao-command evidence --forge ../ao-forge --schema docs/contracts/production-readiness-audit-v0.1.schema.json --document /tmp/ao-forge-production-readiness.json
scripts/rsi-evidence-chain-smoke.sh --forge ../ao-forge --foundry ../ao-foundry --covenant ../ao-covenant --out tmp/rsi-evidence-chain-smoke
```

Use `--json` on any command for machine-readable output when available.
`status` reports the AO Forge readiness percentage, gate count, required next
action count, derived `production_ready` decision, `operator_mode=read_only`,
and release governance state.

`stack` reads the AO Foundry active-stack readiness ledger and reports the
active repository count, release handoff gates, `operator_mode=read_only`, and
`orchestration_owner=ao-foundry`. It does not schedule work, mutate branches,
publish releases, or write control-plane records.

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
`safe_to_execute=false`, required evidence, and denied higher classes. The
checked-in `test_only` fixture can be requested only as a live rehearsal
readback. The checked low-risk-code dry-run fixture can be requested only as a
dry-run design with `safe_to_execute=false`; Command also emits a
`low_risk_code_denial_audit` explaining the missing live policy promotion,
rollback proof, Sentinel clear verdict, Promoter promotion, Command readback,
PR CI evidence, and exact next action. The checked multi-repo low-risk fixture
reports repo-by-repo dependency order and rollback readiness, still with
`safe_to_execute=false`. It also reports `highest_proven_live_class`,
`low_risk_code_live_evidence_status`, `next_denied_class`, and
`next_denied_reason`. Earlier fixtures kept the highest proven live class at
`test_only` until completed `low_risk_code` live evidence existed; later merged
evidence records `fully_unsupervised_complex_mutation` as proven for the governed
26-node first non-planning rehearsal boundary, and newer bounded application
evidence records `bounded_rsi_self_improvement_application` as the highest
proven live class for the exact private readback/eval rubric rehearsal. Later
exact safe public claim wording evidence records
`exact_safe_public_claim_wording_conservative_readback_evidence` as prior conservative public-safe tracked readback evidence. Each
repo readback includes its serialized order, planned dry-run PR placeholder,
rollback scope, dependencies, and merge-after constraints. It also emits
`multi_repo_live_rehearsal_denial` when the dry-run chain is requestable but
live multi-repo execution is blocked by missing lower-class live evidence.
Broader RSI remains denied.
When `--sentinel-hold` is supplied, Command reads the AO Sentinel
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

`rsi health` reads local fixture evidence from AO Arena, AO Crucible, AO
Sentinel, AO Promoter, AO Foundry's RSI improvement gate, AO Foundry's RSI
candidate evidence, and AO Foundry's RSI next improvement task evidence. It
verifies the candidate eval result is the same candidate evidence used by the
improvement gate, then verifies the next-task artifact binds to both the
candidate and gate evidence. It also verifies AO Forge retained evidence for
the Foundry gate, Foundry candidate, Foundry next task, and AO Command health
output against the AO Forge retained-evidence contract before trusting the
semantic proof fields. It reports whether the governed fixture/local RSI chain
is demonstrated from Foundry pulse through Forge retention to Command health
while keeping `operator_mode=read_only` and
`mutates_repositories=false`. The text, JSON, and bundle outputs also publish
claim-level decisions: `claim_level=bounded_governed_rsi decision=allowed` only
for the bounded governed local evidence chain, and
`claim_level=full_autonomous_self_mutating_rsi decision=denied` until mutation
authority, rollback, live self-change evidence, and an AO Covenant
`claim.publish` boundary allow that stronger claim. Use `--bundle-out` to write
the canonical `ao.command.rsi-health-bundle.v0.1` JSON artifact with the source
evidence paths, claim levels, and SHA-256 hashes retained in one portable file.
Validate machine-readable RSI health output against
`docs/contracts/rsi-health-v0.1.schema.json` and bundles against
`docs/contracts/rsi-health-bundle-v0.1.schema.json`.

`rsi manifest` reads AO Architecture's
`overview/rsi-claim-evidence-manifest.json` and fail-closes unless the
manifest preserves the bounded/full RSI claim boundary. It requires
`claim_level=bounded_governed_rsi decision=allowed`, requires
`claim_level=full_autonomous_self_mutating_rsi decision=denied`, confirms the
active repositories and deprecated or out-of-scope repositories are represented,
requires AO2's claim-readiness producer plus ao2-control-plane's
`ao2.cp-ao2-rsi-claim-readiness-readback.v1` observer readback, requires AO2's
governed self-change dry-run producer
`ao2.rsi-governed-self-change-dry-run.v1` plus ao2-control-plane's
`ao2.cp-ao2-rsi-self-change-dry-run-readback.v1` observer readback, requires
AO2 rollback rehearsal evidence with `rollback_rehearsal.status=passed`, and
requires ao2-control-plane to read back the same rollback rehearsal evidence
from PR #72 after AO2 PR #200. It also fails closed unless AO Architecture pins
AO2 PR #201's dry-run `covenant.live-self-change-authority.v1` authority-packet
candidate with `schema_valid_for_claim_publish=false` and ao2-control-plane PR
#73's `ao2.cp-ao2-rsi-authority-packet-readback.v1` observer readback,
AO Forge PR #143's retained `ao-command-rsi-manifest-retention-proof.json` and
AO Forge PR #144's `goalrun.architecture_rsi_pin_readback` evidence plus
`ao-architecture-rsi-pin-readback.json`, and unless Architecture pins
AO Covenant PR #57's `rollback-retained.contract.json` denial fixture plus
AO Covenant PR #58's `covenant.live-self-change-authority.v1` schema and
`live-self-change-authority.packet.json` fixture. It
reports `operator_mode=read_only` with `mutates_repositories=false`.

`scripts/rsi-evidence-chain-smoke.sh` exercises the governed RSI chain end to
end: it runs `foundry pulse run`, verifies the pulse evidence against AO Forge
retained RSI proofs through `ao-command rsi health`, and confirms AO Covenant
denies `claim.publish` for `full-autonomous-self-mutating-rsi` unless mutation
authority, rollback, and live self-change evidence exist. The smoke also pins
the AO Forge aggregate proof at
`../ao-forge/docs/evidence/goals/ao2-weekend-hardening/20260619T180000Z-verification/bounded-rsi-improvement-chain-retention-proof.json`
and the AO Covenant claim-boundary fixtures at
`../ao-covenant/examples/full-rsi-claim-boundary/`.

The final bounded evidence readback accepts only the bounded class decision:
`decision=promote_bounded_rsi_evidence_rehearsal_keep_broad_rsi_denied`.
`bounded_rsi_evidence_rehearsal` may be reported as live-proven, while broad
RSI, hidden self-modification, and unrestricted self-modification remain
denied. This prior bounded-evidence readback does not schedule, execute,
approve, publish, or mutate anything, and it is superseded by the bounded
self-improvement application class readback below for the current highest live
class and next denied class.

The bounded self-improvement application readback accepts only the exact bounded
class decision:
`class_decision=bounded_rsi_self_improvement_application_proven`.
`bounded_rsi_self_improvement_application` may be reported as proven only for
the exact private readback/eval rubric rehearsal. Command reads back
`highest_proven_live_class=bounded_rsi_self_improvement_application` and
`next_denied_class=broad_RSI`, while `broad_RSI`, unrestricted
self-modification, hidden instruction mutation, policy/auth/secret/provider/
deploy/release/config/dependency expansion, and policy-changing autonomy remain
denied. This is readback only; it does not schedule, execute, approve, publish,
or mutate anything.

The exact safe public claim wording readback accepts only the conservative
public wording evidence:
`class_decision=exact_safe_public_claim_wording_conservative_readback_evidence_proven`.
The approved public wording is exactly: "AO has public-safe tracked readback
evidence for bounded improvement-claim review and retraction rehearsal; stronger
recursive-improvement claims remain denied." This remains prior readback
evidence while `broad_RSI`, unrestricted self-modification, hidden instruction
mutation, policy-changing autonomy, and stronger recursive-improvement claims
remain denied. Stronger recursive-improvement wording remains denied. This is
readback only; it
does not schedule, execute, approve, publish, or mutate anything.

The causal-review evidence-selection guidance readback accepts only the narrow
public-safe guidance evidence:
`class_decision=public_safe_causal_review_evidence_selection_guidance_proven`.
The approved public wording is exactly: "AO has public-safe causal-review
evidence that prior bounded evidence can guide later evidence-selection and
blocker prioritization under independent review gates; stronger
recursive-improvement wording and broad_RSI remain denied." Command reads back
this as prior evidence while `broad_RSI`, stronger
recursive-improvement wording, unrestricted self-modification, hidden instruction
mutation, and policy-changing autonomy remain denied. The guard is explicit:
stronger recursive-improvement wording remains denied. This is readback only; it
does not schedule, execute, approve, publish, or mutate anything.

The guided evidence-application readback accepts only the narrow public-safe
guided application evidence:
`class_decision=public_safe_guided_evidence_application_four_attempts_proven`.
The approved public wording is exactly: "AO has public-safe guided
evidence-application evidence showing causal-review guidance can select and
prioritize later bounded evidence attempts under independent gates; stronger
recursive-improvement wording and broad_RSI remain denied." Command reads back
`highest_proven_live_class=public_safe_bounded_recursive_improvement_review_durability_evidence`
and `next_denied_class=broad_RSI`, while `broad_RSI`, stronger
recursive-improvement wording, unrestricted self-modification, hidden instruction
mutation, and policy-changing autonomy remain denied. This is readback only; it
does not schedule, execute, approve, publish, or mutate anything.

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
- `rsi health` reads assurance-family JSON evidence and does not run providers,
  promote candidates, apply activation plans, or mutate repositories.
- `rsi manifest` reads the architecture manifest and does not mutate
  repositories, publish claims, or approve the full RSI claim.
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
go run ./cmd/ao-command rsi health --arena-gate ../ao-arena/tmp/arena-promotion-gate.json --crucible-gate ../ao-crucible/tmp/crucible-hardening-gate.json --sentinel-verdict ../ao-sentinel/tmp/sentinel-verdict.json --promoter-gate ../ao-promoter/tmp/promotion-gate.json --foundry-gate ../ao-foundry/tmp/pulse-rsi-verify/rsi-improvement-gate.json --foundry-candidate ../ao-foundry/tmp/pulse-rsi-verify/rsi-candidate.json --foundry-next-task ../ao-foundry/tmp/pulse-rsi-verify/rsi-next-improvement-task.json --forge-retained-gate ../ao-forge/docs/evidence/goals/ao2-weekend-hardening/20260619T180000Z-verification/ao-foundry-rsi-improvement-gate-retention-proof.json --forge-retained-candidate ../ao-forge/docs/evidence/goals/ao2-weekend-hardening/20260619T180000Z-verification/ao-foundry-rsi-candidate-retention-proof.json --forge-retained-next-task ../ao-forge/docs/evidence/goals/ao2-weekend-hardening/20260619T180000Z-verification/ao-foundry-rsi-next-improvement-task-retention-proof.json --forge-retained-command-health ../ao-forge/docs/evidence/goals/ao2-weekend-hardening/20260619T180000Z-verification/ao-command-rsi-health-retention-proof.json --bundle-out tmp/rsi-health-bundle.json --json
scripts/ao-command-smoke.sh --forge ../ao-forge --foundry ../ao-foundry --out tmp/ao-command-smoke
scripts/rsi-evidence-chain-smoke.sh --forge ../ao-forge --foundry ../ao-foundry --covenant ../ao-covenant --out tmp/rsi-evidence-chain-smoke
go run ./cmd/ao-command evidence --forge ../ao-forge --schema "$PWD/docs/contracts/rsi-health-v0.1.schema.json" --document "$PWD/tmp/rsi-evidence-chain-smoke/ao-command-rsi-health.json"
go run ./cmd/ao-command evidence --forge ../ao-forge --schema "$PWD/docs/contracts/rsi-health-bundle-v0.1.schema.json" --document "$PWD/tmp/rsi-evidence-chain-smoke/rsi-health-bundle.json"
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

`public_safe_intermediate_causal_review_claim_evidence` remains prior evidence
from AO Foundry PR #189, commit
`860e3f353ab833c4a671b9d0ee6d8101ece2815c`, with tracked public evidence under
`docs/evidence/recursive-improvement-safe-intermediate-claim/`. The approved public wording is exactly: "AO has public-safe intermediate causal-review evidence that bounded improvement evidence can guide and constrain later claim review across independent roles; stronger recursive-improvement wording and broad_RSI remain denied." Stronger recursive-improvement wording remains denied, `broad_RSI` remains denied, unrestricted self-modification remains denied, hidden instruction mutation remains denied, and policy-changing autonomy remains denied.

`public_safe_causal_review_evidence_selection_guidance` is proven from AO Foundry
PR #191, commit `413b70f15d8f3d0203dc7be076914a2f3b539881`, with tracked public
evidence under `docs/evidence/recursive-improvement-evidence-selection-guidance/`.
The approved public wording is exactly: "AO has public-safe causal-review
evidence that prior bounded evidence can guide later evidence-selection and
blocker prioritization under independent review gates; stronger
recursive-improvement wording and broad_RSI remain denied." This remains prior
evidence. Stronger recursive-improvement wording remains denied, `broad_RSI`
remains denied, unrestricted self-modification remains denied, hidden
instruction mutation remains denied, and policy-changing autonomy remains
denied.

`public_safe_guided_evidence_application_four_attempts` is proven from AO Foundry
PR #193, commit `4ec509fd64d1fc1ea41ea7f22aae900ba79e09a1`, with tracked public
evidence under `docs/evidence/recursive-improvement-guided-evidence-application/`.
The approved public wording is exactly: "AO has public-safe guided
evidence-application evidence showing causal-review guidance can select and
prioritize later bounded evidence attempts under independent gates; stronger
recursive-improvement wording and broad_RSI remain denied." The highest proven
live class is `public_safe_recursive_improvement_claim_threshold_calibration_evidence` and the
next denied class is `broad_RSI`. Stronger recursive-improvement wording
remains denied, `broad_RSI` remains denied, unrestricted self-modification
remains denied, hidden instruction mutation remains denied, and policy-changing
autonomy remains denied.

## Public-Safe Reviewer-Approved Bounded Wording Evidence

`public_safe_reviewer_approved_bounded_recursive_improvement_wording_evidence` is proven from AO Foundry PR #195, commit `0f742738324c185ba7243bc53ee2f1bc81804ef6`, with tracked public evidence under `docs/evidence/recursive-improvement-reviewer-approved-wording/`. The approved public wording is exactly: "AO has public-safe reviewer-approved bounded recursive-improvement wording evidence showing guided evidence application can improve later evidence attempts under independent review gates; broad_RSI remains denied." This remains prior evidence; the current highest proven live class is `public_safe_broad_RSI_governed_campaign_segment_07_evidence` and the next denied class is `broad_RSI`.

This does not prove `broad_RSI`, unrestricted self-modification, hidden instruction mutation, policy-changing autonomy, policy/auth/secret/provider/deploy/release/config/dependency expansion, or unbounded stronger recursive-improvement claims.
`public_safe_bounded_recursive_improvement_wording_generality_evidence` is proven from AO Foundry PR #197, commit `166398641b655f0da97817659acc771026b204e7`, with tracked public evidence under `docs/evidence/recursive-improvement-bounded-wording-generality/`. The approved public wording is exactly: "AO has public-safe bounded recursive-improvement wording generality evidence showing reviewer-approved bounded wording can transfer across additional public-safe review tasks under independent gates; broad_RSI remains denied." This remains prior evidence; the current highest proven live class is `public_safe_broad_RSI_governed_campaign_segment_07_evidence` and the next denied class is `broad_RSI`.

This does not prove `broad_RSI`, unrestricted self-modification, hidden instruction mutation, policy-changing autonomy, policy/auth/secret/provider/deploy/release/config/dependency expansion, or unbounded stronger recursive-improvement claims.
### Review Durability Evidence Readback

`public_safe_bounded_recursive_improvement_review_durability_evidence` is proven from AO Foundry PR #199, commit `12d524b60c200cab643e44f9105169b045602798`, with tracked public evidence under `docs/evidence/recursive-improvement-review-durability/`. The approved public wording is exactly: "AO has public-safe bounded recursive-improvement review durability evidence showing bounded recursive-improvement wording remains stable across delayed re-review, adversarial drift checks, stale-language sweeps, and reproducibility retests under independent gates; broad_RSI remains denied." This remains prior evidence; the current highest proven live class is `public_safe_broad_RSI_governed_campaign_segment_07_evidence` and the next denied class is `broad_RSI`.


`public_safe_recursive_improvement_claim_threshold_calibration_evidence` is proven from AO Foundry PR #201, commit `3e3d1101da112fa5ff0aca26f8ab2933652f3502`, with tracked public evidence under
`docs/evidence/recursive-improvement-claim-threshold-calibration/`. The approved public wording is exactly: "AO has public-safe recursive-improvement claim threshold calibration evidence showing stronger bounded recursive-improvement claims can be evaluated against reproducible threshold, public-reader, adversarial wording, Covenant, Sentinel, rollback, and retraction gates; broad_RSI remains denied." This remains prior evidence; the current highest proven live class is `public_safe_broad_RSI_governed_campaign_segment_07_evidence` and the next denied class is `broad_RSI`.

This does not prove `broad_RSI`, unrestricted self-modification, hidden instruction mutation, policy-changing autonomy, policy/auth/secret/provider/deploy/release/config/dependency expansion, or unbounded stronger recursive-improvement claims.
This does not prove `broad_RSI`, unrestricted self-modification, hidden instruction mutation, policy-changing autonomy, policy/auth/secret/provider/deploy/release/config/dependency expansion, or unbounded stronger recursive-improvement claims.

## Broad RSI Ten-Day Governed Campaign First Segment Readback

`public_safe_broad_RSI_governed_campaign_first_segment_state_evidence` is proven from AO Foundry PR #203, commit `b7523031d61b11df374e2203bdf44927e2d8432a`, with tracked public evidence under `docs/evidence/broad-rsi-ten-day-governed-evidence-campaign/`. The approved public wording is exactly: "AO has public-safe broad_RSI governed campaign first-segment state evidence showing a 10-day evidence campaign can start from mission-state, no-repeat, sufficiency, Pulse reliability, context-repack, rollback, and claim-gate readbacks while broad_RSI remains denied." This remains prior evidence; the current highest proven live class is `public_safe_broad_RSI_governed_campaign_segment_07_evidence` and the next denied class is `broad_RSI`.

This does not prove `broad_RSI`, full 10-day campaign completion, final repeated independent broad evidence, final cross-repo generality proof for `broad_RSI`, exact `broad_RSI` public-reader approval, exact `broad_RSI` Covenant or Architecture approval, unrestricted self-modification, hidden instruction mutation, policy-changing autonomy, policy/auth/secret/provider/deploy/release/config/dependency expansion, release/deploy/publish/upload/tag/provider calls, credential use, direct main mutation, concurrent mutation, or unbounded stronger recursive-improvement claims.

## Broad RSI Ten-Day Governed Campaign Segment 07 Readback

The segment-07 readback accepts only the narrow class decision:
`public_safe_broad_RSI_governed_campaign_segment_07_evidence_proven_broad_RSI_denied`.
It is proven from AO Foundry PR #210, commit
`8f8ac5f8f74d942c7a02a6c2dd39a7c974872bb6`, with tracked public evidence under
`docs/evidence/broad-rsi-ten-day-campaign-segment-07/`. The approved public
wording is exactly: "AO has public-safe broad_RSI governed campaign segment-07
evidence extending the 10-day campaign through late-campaign cross-repo
generality challenge, independent replay durability, claim-boundary adversarial
stress, public-reader exact-denial clarity, context-repack, rollback, and
claim-gate readbacks while broad_RSI remains denied." Command reads back
`highest_proven_live_class=public_safe_broad_RSI_governed_campaign_segment_07_evidence`
and `next_denied_class=broad_RSI`.

This is readback only. It does not schedule, execute, approve, publish, release,
deploy, call providers, use credentials, update dependencies, widen
policy/auth/config, expose secrets, mutate direct main, allow concurrent
mutation, prove full 10-day campaign completion, or prove `broad_RSI`.
