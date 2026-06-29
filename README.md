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
go run ./cmd/ao-command pulse status --preflight ../ao-foundry/examples/pulse-overnight-start-gate/ready.intake-preflight.json --lifecycle ../ao-foundry/examples/pulse-lifecycle/ready-to-start-next-slice.json --start-gate ../ao-foundry/examples/pulse-overnight-start-gate/ready.json
go run ./cmd/ao-command complex-refactor status --summary examples/complex-refactor/ready-summary.json
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
- `pulse status` reads AO Foundry Pulse gate evidence and does not start loops,
  merge PRs, create branches, call providers, publish, or mutate repositories.
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
   Foundry, Forge, Covenant, AO2, and readback evidence.
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
