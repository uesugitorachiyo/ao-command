# Production Readiness

AO Command production readiness is measured by
`scripts/production-readiness-audit.sh`.

## Current Gate

The audit is passing only when all gates pass:

- repository is public and deletes merged branches;
- main branch protection denies force-push and deletion, enforces linear
  history, and requires strict `Go`, `Workflow lint`, and
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
- retained dry-run evidence is governed by
  `docs/operations/RETAINED-EVIDENCE.md` and
  `docs/operations/public-provenance-manifest.json`;
- v0.1.0 operator closeout is documented in
  `docs/release/V0.1.0-OPERATOR-CLOSEOUT.md`;
- tests, vet, build, workflow lint, AO Forge-backed smoke, and AO Forge
  production readiness all pass.

## Local Command

```sh
scripts/release-preview-dry-run.sh --forge ../ao-forge --out tmp/release-preview --tag v0.1.0-preview
go run ./cmd/ao-command evidence --forge ../ao-forge --schema "$PWD/docs/contracts/release-preview-audit-v0.1.schema.json" --document "$PWD/tmp/release-preview/release-preview-audit.json"
scripts/install-verify-dry-run.sh --forge ../ao-forge --out tmp/install-verify
go run ./cmd/ao-command evidence --forge ../ao-forge --schema "$PWD/docs/contracts/install-verify-audit-v0.1.schema.json" --document "$PWD/tmp/install-verify/install-verify-audit.json"
scripts/production-readiness-audit.sh --repo uesugitorachiyo/ao-command --forge ../ao-forge --out tmp/production-readiness-audit.json
go run ./cmd/ao-command evidence --forge ../ao-forge --schema "$PWD/docs/contracts/production-readiness-audit-v0.1.schema.json" --document "$PWD/tmp/production-readiness-audit.json"
```

The audit emits `ao.command.production-readiness-audit.v0.1` JSON with
`readiness_percent`, `passed_gates`, `total_gates`, and per-gate evidence.

## CI Mode

Public GitHub Actions runs the same audit with `--skip-remote-admin` because the
workflow token cannot read every branch-protection and security setting. Skipped
admin-only checks are excluded from the CI percentage. The full local/operator
audit above remains the authoritative operator score.
