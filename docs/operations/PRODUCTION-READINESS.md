# Production Readiness

AO Command production readiness is measured by
`scripts/production-readiness-audit.sh`.

## Current Gate

The audit is passing only when all gates pass:

- repository is public and deletes merged branches;
- main branch protection denies force-push and deletion, enforces linear
  history, and requires strict `License policy`, `Go`, `Workflow lint`, and
  `Production readiness audit` checks;
- secret scanning and push protection are enabled;
- vulnerability alerts are enabled;
- tracked files contain no obvious tokens, private keys, provider secrets, or
  user home absolute paths;
- workflows and scripts do not upload artifacts by default;
- command surface contains no public-switch, release-publish,
  production-promotion, forced-push, hard-reset, or destructive delete path;
- README, SECURITY, and publication record document the public readiness state;
- CI defines a production readiness audit job and manual dispatch;
- the generated audit validates against
  `docs/contracts/production-readiness-audit-v0.1.schema.json`;
- release-preview dry-run evidence validates against
  `docs/contracts/release-preview-audit-v0.1.schema.json`;
- install verification dry-run evidence validates against
  `docs/contracts/install-verify-audit-v0.1.schema.json`;
- release governance dry-run evidence validates against
  `docs/contracts/release-governance-audit-v0.1.schema.json`;
- RSI health JSON and bundle evidence validate against
  `docs/contracts/rsi-health-v0.1.schema.json` and
  `docs/contracts/rsi-health-bundle-v0.1.schema.json`;
- AO Foundry active-stack handoff status is visible through read-only
  `ao-command stack --ledger` output;
- AO Foundry Atlas observer status is visible through read-only
  `ao-command atlas status --status` output and preserves
  `schedules_work=false`, `executes_work=false`, `approves_work=false`, and
  `atlas_authority=compile_only`;
- the governed RSI evidence-chain smoke passes from `foundry pulse run` through
  AO Forge retained proofs, `ao-command rsi health`, and AO Covenant's denied
  full autonomous self-mutating RSI claim boundary;
- RSI health output reports
  `claim_level=bounded_governed_rsi decision=allowed` only for the bounded
  governed local chain and
  `claim_level=full_autonomous_self_mutating_rsi decision=denied` for the full
  autonomous self-mutating RSI claim until mutation authority, rollback, live
  self-change evidence, and Covenant approval exist;
- AO Architecture's RSI claim evidence manifest validates through read-only
  `ao-command rsi manifest --manifest`, preserves the same bounded/full
  claim-level decisions, and includes ao2-control-plane's
  `ao2.cp-ao2-rsi-claim-readiness-readback.v1` observer readback plus AO2's
  `ao2.rsi-governed-self-change-dry-run.v1` producer and ao2-control-plane's
  `ao2.cp-ao2-rsi-self-change-dry-run-readback.v1` observer readback, including
  `rollback_rehearsal.status=passed` from AO2 PR #200 and ao2-control-plane PR
  #72, plus AO Forge PR #143's retained
  `ao-command-rsi-manifest-retention-proof.json`, AO Forge PR #144's
  `goalrun.architecture_rsi_pin_readback` evidence, and AO Covenant PR #57's
  `rollback-retained.contract.json` denial fixture plus AO Covenant PR #58's
  `covenant.live-self-change-authority.v1` authority packet schema. The
  manifest validator also fail-closes unless AO Architecture pins AO2 PR #201's
  dry-run authority-packet candidate with
  `schema_valid_for_claim_publish=false` and ao2-control-plane PR #73's
  `ao2.cp-ao2-rsi-authority-packet-readback.v1` observer readback;
- retained dry-run evidence is governed by
  `docs/operations/RETAINED-EVIDENCE.md` and
  `docs/operations/public-provenance-manifest.json`;
- v0.1.1 operator closeout and release notes are documented in
  `docs/release/V0.1.1-OPERATOR-CLOSEOUT.md`;
- tests, vet, build, workflow lint, AO Forge-backed smoke, and AO Forge
  production readiness all pass.

## Local Command

```sh
scripts/release-preview-dry-run.sh --forge ../ao-forge --out tmp/release-preview --tag v0.1.1-preview
go run ./cmd/ao-command evidence --forge ../ao-forge --schema "$PWD/docs/contracts/release-preview-audit-v0.1.schema.json" --document "$PWD/tmp/release-preview/release-preview-audit.json"
scripts/install-verify-dry-run.sh --forge ../ao-forge --out tmp/install-verify
go run ./cmd/ao-command evidence --forge ../ao-forge --schema "$PWD/docs/contracts/install-verify-audit-v0.1.schema.json" --document "$PWD/tmp/install-verify/install-verify-audit.json"
scripts/release-governance-dry-run.sh --out tmp/release-governance --tag v0.1.1 --release-preview-audit tmp/release-preview/release-preview-audit.json --install-verify-audit tmp/install-verify/install-verify-audit.json
go run ./cmd/ao-command evidence --forge ../ao-forge --schema "$PWD/docs/contracts/release-governance-audit-v0.1.schema.json" --document "$PWD/tmp/release-governance/release-governance-audit.json"
go run ./cmd/ao-command stack --ledger ../ao-foundry/examples/readiness/active-stack-readiness.ledger.json
go run ./cmd/ao-command atlas status --status ../ao-foundry/examples/contract-fixtures/valid/foundry-atlas-status-v0.1.json
scripts/rsi-evidence-chain-smoke.sh --forge ../ao-forge --foundry ../ao-foundry --covenant ../ao-covenant --out tmp/rsi-evidence-chain-smoke
go run ./cmd/ao-command rsi manifest --manifest ../ao-architecture/overview/rsi-claim-evidence-manifest.json
go run ./cmd/ao-command evidence --forge ../ao-forge --schema "$PWD/docs/contracts/rsi-health-v0.1.schema.json" --document "$PWD/tmp/rsi-evidence-chain-smoke/ao-command-rsi-health.json"
go run ./cmd/ao-command evidence --forge ../ao-forge --schema "$PWD/docs/contracts/rsi-health-bundle-v0.1.schema.json" --document "$PWD/tmp/rsi-evidence-chain-smoke/rsi-health-bundle.json"
scripts/production-readiness-audit.sh --repo uesugitorachiyo/ao-command --forge ../ao-forge --foundry ../ao-foundry --covenant ../ao-covenant --architecture ../ao-architecture --out tmp/production-readiness-audit.json
go run ./cmd/ao-command evidence --forge ../ao-forge --schema "$PWD/docs/contracts/production-readiness-audit-v0.1.schema.json" --document "$PWD/tmp/production-readiness-audit.json"
scripts/verify-branch-protection.sh
```

The audit emits `ao.command.production-readiness-audit.v0.1` JSON with
`readiness_percent`, `passed_gates`, `total_gates`, and per-gate evidence.

## CI Mode

Public GitHub Actions runs the same audit with `--skip-remote-admin` because the
workflow token cannot read every branch-protection and security setting. Skipped
admin-only checks are excluded from the CI percentage. The full local/operator
audit above remains the authoritative operator score.

## Branch Protection Drift

`docs/operations/BRANCH-PROTECTION.md` defines the required live protection for
`main`. The read-only `Production Readiness Ops` workflow runs
`scripts/verify-branch-protection.sh` daily and by manual dispatch in
`AO_COMMAND_BRANCH_PROTECTION_MODE=limited`, while local maintainer runs default
to full administrative verification.
