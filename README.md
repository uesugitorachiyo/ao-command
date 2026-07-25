# AO Command

AO Command is the read-only operator interface for AO. It turns Mission,
Atlas, Foundry, Forge, Covenant, AO2, and Control Plane records into terminal
or JSON views without becoming another source of domain truth.

## Role In AO

- **Inputs:** Mission lifecycle state, portfolio status, GoalRun evidence,
  policy decisions, AO2 evidence, and Control Plane readbacks.
- **Outputs:** Status, timelines, next actions, validation summaries, and JSON
  readbacks.
- **Upstream:** AO Mission, AO Atlas, AO Foundry, AO Forge, AO Covenant, AO2,
  and AO2 Control Plane.
- **Downstream:** Operators and read-only automation.

See the [AO Architecture guide](https://github.com/uesugitorachiyo/ao-architecture)
and the
[AO Command component page](https://github.com/uesugitorachiyo/ao-architecture/blob/main/components/ao-command.md)
for the cross-repository flow.

## Common Commands

```sh
go run ./cmd/ao-command status --forge ../ao-forge
go run ./cmd/ao-command mission status \
  --status examples/mission/command-status.ready.json
go run ./cmd/ao-command mission next \
  --decision examples/mission/route-decision.ready.json
go run ./cmd/ao-command stack \
  --ledger ../ao-foundry/examples/readiness/active-stack-readiness.ledger.json
go run ./cmd/ao-command atlas status \
  --status ../ao-foundry/examples/contract-fixtures/valid/foundry-atlas-status-v0.1.json
go run ./cmd/ao-command pulse status \
  --preflight ../ao-foundry/examples/pulse-overnight-start-gate/ready.intake-preflight.json \
  --lifecycle ../ao-foundry/examples/pulse-lifecycle/ready-to-start-next-slice.json \
  --start-gate ../ao-foundry/examples/pulse-overnight-start-gate/ready.json
```

Use `--json` when a command supports machine-readable output. The
[full command reference](REFERENCE.md) documents every status, evidence,
rehearsal, release, and live-mutation readback.

## Safety Boundary

AO Command reads records owned by other components. It does not start loops,
approve work, execute patches, merge pull requests, call providers, publish
releases, or mutate repositories. Commands reject input packets that claim
authority outside their documented contract.

AO Forge remains the source of truth for GoalRun and release state. AO
Covenant owns policy decisions. AO2 owns governed execution. AO Foundry owns
portfolio coordination.

## Documentation

- [AO Command Foundry Design](docs/design/AO-COMMAND-FOUNDRY.md)
- [Production Readiness](docs/operations/PRODUCTION-READINESS.md)
- [Branch Protection](docs/operations/BRANCH-PROTECTION.md)
- [Retained Evidence](docs/operations/RETAINED-EVIDENCE.md)
- [Publication Checklist](docs/operations/PUBLICATION-CHECKLIST.md)
- [v0.1.1 Operator Closeout](docs/release/V0.1.1-OPERATOR-CLOSEOUT.md)
- [Full Reference](REFERENCE.md)

## Verification

```sh
go test ./...
go vet ./...
go build -o bin/ao-command ./cmd/ao-command
scripts/ao-command-smoke.sh \
  --forge ../ao-forge \
  --foundry ../ao-foundry \
  --out tmp/ao-command-smoke
```

## License

AO Command is licensed under Apache 2.0. See [LICENSE](LICENSE).
