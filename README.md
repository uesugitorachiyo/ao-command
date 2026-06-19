# AO Command

AO Command is the read-only operator command surface for the AO stack. It makes
AO Forge, AO2, ao2-control-plane, and AO Covenant evidence inspectable from one
daily command center.

## Status

v0.1 is a local scaffold. It reads AO Forge production-readiness, GoalRun,
release-preview, and contract evidence through AO Forge-owned commands. It does
not publish releases, promote production, mutate provider state, or replace AO
Forge policy decisions.

This is the first slice toward AO Command Foundry, not the full Foundry system
yet. Foundry is the later persistent engineering operations layer across many
repos, branches, releases, incidents, dependency updates, docs drift, CI health,
security reviews, and roadmap slices. v0.1 keeps the command surface read-only
so AO Forge remains the trusted factory brain before autonomous cross-repo work
is enabled.

## Commands

```sh
go run ./cmd/ao-command status --forge ../ao-forge
go run ./cmd/ao-command next --forge ../ao-forge
go run ./cmd/ao-command goals --forge ../ao-forge --goal-run ../ao-forge/examples/goals/ao2-weekend-hardening.goal-run.json
go run ./cmd/ao-command evidence --forge ../ao-forge --schema docs/contracts/production-readiness-audit-v0.1.schema.json --document /tmp/ao-forge-production-readiness.json
```

Use `--json` on any command for machine-readable output when available.

For an existing release tag, rehearse from an AO Forge checkout whose HEAD
matches that tag:

```sh
git -C ../ao-forge worktree add /tmp/ao-forge-v0.1.3 v0.1.3
go run ./cmd/ao-command rehearse --forge /tmp/ao-forge-v0.1.3 --tag v0.1.3 --out /tmp/ao-command-v013-rehearsal
```

## Safety

- Private by default until the operator explicitly approves publication.
- Read-only by default.
- AO Forge remains the source of truth for readiness percentages, release gates,
  GoalRun state, retained evidence, and Covenant decisions.
- `rehearse` only runs AO Forge release-preview dry-run evidence and then
  inspects the produced audit.
- Dangerous writes are intentionally out of scope for v0.1.
- CI does not upload artifacts by default while the repository is private.

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
```

Private-repo operating guardrails are tracked in
`docs/operations/PRIVATE-REPO-GUARDRAILS.md`.
The pre-publication operator gate is tracked in
`docs/operations/PUBLICATION-CHECKLIST.md`.
