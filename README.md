# AO Command

AO Command is the read-only operator command surface for the AO stack. It makes
AO Forge, AO2, ao2-control-plane, and AO Covenant evidence inspectable from one
daily command center.

## Status

v0.1 is a public scaffold after passing the AO Command publication audit. It
reads AO Forge production-readiness, GoalRun, release-preview, and contract
evidence through AO Forge-owned commands. It does not publish releases, promote
production, mutate provider state, or replace AO Forge policy decisions.

AO Command's live stack boundary is AO Forge for readiness and GoalRun truth,
AO2 for governed execution, ao2-control-plane for evidence readback, and AO
Covenant for allow, deny, and block decisions. Deprecated standalone runtime,
operator, conductor, and subscription-backed swarm surfaces are out of scope.

This is the first slice toward AO Command Foundry, not the full Foundry system
yet. Foundry is the later persistent engineering operations layer across many
repos, branches, releases, incidents, dependency updates, docs drift, CI health,
security reviews, and roadmap slices. v0.1 keeps the command surface read-only
so AO Forge remains the trusted factory brain before autonomous cross-repo work
is enabled.

## Commands

```sh
go run ./cmd/ao-command status --forge ../ao-forge
go run ./cmd/ao-command stack --ledger ../ao-foundry/examples/readiness/active-stack-readiness.ledger.json
go run ./cmd/ao-command next --forge ../ao-forge
go run ./cmd/ao-command goals --forge ../ao-forge --goal-run ../ao-forge/examples/goals/ao2-weekend-hardening.goal-run.json
go run ./cmd/ao-command evidence --forge ../ao-forge --schema docs/contracts/production-readiness-audit-v0.1.schema.json --document /tmp/ao-forge-production-readiness.json
```

Use `--json` on any command for machine-readable output when available.
`status` reports the AO Forge readiness percentage, gate count, required next
action count, derived `production_ready` decision, `operator_mode=read_only`,
and release governance state.

`stack` reads the AO Foundry active-stack readiness ledger and reports the
active repository count, release handoff gates, `operator_mode=read_only`, and
`orchestration_owner=ao-foundry`. It does not schedule work, mutate branches,
publish releases, or write control-plane records.

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
- `rehearse` only runs AO Forge release-preview dry-run evidence and then
  inspects the produced audit.
- Dangerous writes are intentionally out of scope for v0.1.
- CI does not upload artifacts by default.

## Foundry Direction

The Foundry path is:

1. AO Forge: trusted factory brain, release gates, GoalRun state, readiness, and
   verified evidence.
2. AO Command v0.1: human/operator command center over AO Forge evidence.
3. AO Arena: internal quality and replay mode for comparing agent/factory runs.
4. AO Command Foundry: persistent engineering operations layer for many repos
   with a project registry, task queues, concurrency limits, overnight
   advancement, Covenant-signed job results, and control-plane evidence.

The initial registry design is in
`docs/design/FOUNDRY-REGISTRY-V0.1.md`.

## Verification

```sh
go test ./...
go vet ./...
go build -o bin/ao-command ./cmd/ao-command
scripts/ao-command-smoke.sh --forge ../ao-forge --out tmp/ao-command-smoke
scripts/release-preview-dry-run.sh --forge ../ao-forge --out tmp/release-preview --tag v0.1.0-preview
go run ./cmd/ao-command evidence --forge ../ao-forge --schema "$PWD/docs/contracts/release-preview-audit-v0.1.schema.json" --document "$PWD/tmp/release-preview/release-preview-audit.json"
scripts/install-verify-dry-run.sh --forge ../ao-forge --out tmp/install-verify
go run ./cmd/ao-command evidence --forge ../ao-forge --schema "$PWD/docs/contracts/install-verify-audit-v0.1.schema.json" --document "$PWD/tmp/install-verify/install-verify-audit.json"
scripts/release-governance-dry-run.sh --out tmp/release-governance --tag v0.1.0 --release-preview-audit tmp/release-preview/release-preview-audit.json --install-verify-audit tmp/install-verify/install-verify-audit.json
go run ./cmd/ao-command evidence --forge ../ao-forge --schema "$PWD/docs/contracts/release-governance-audit-v0.1.schema.json" --document "$PWD/tmp/release-governance/release-governance-audit.json"
scripts/production-readiness-audit.sh --repo uesugitorachiyo/ao-command --forge ../ao-forge --foundry ../ao-foundry --out tmp/production-readiness-audit.json
go run ./cmd/ao-command evidence --forge ../ao-forge --schema "$PWD/docs/contracts/production-readiness-audit-v0.1.schema.json" --document "$PWD/tmp/production-readiness-audit.json"
```

Historical private-repo operating guardrails are tracked in
`docs/operations/PRIVATE-REPO-GUARDRAILS.md`.
The pre-publication operator gate and publication evidence are tracked in
`docs/operations/PUBLICATION-CHECKLIST.md`.
Production Readiness is tracked in
`docs/operations/PRODUCTION-READINESS.md`, with publication evidence in
`docs/operations/PUBLICATION-RECORD-2026-06-19.md`.
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
