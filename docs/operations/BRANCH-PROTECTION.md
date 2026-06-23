# Branch Protection

This runbook records the live `main` branch protection expected for the public
AO Command repository.

## Required Settings

Configure `main` with these controls:

- Require pull requests before merge.
- Require status checks to pass before merge.
- Require branches to be up to date before merge.
- Include administrators.
- Require linear history.
- Block force pushes.
- Block branch deletion.

## Required Checks

Require these status checks:

- `Go`
- `Workflow lint`
- `Production readiness audit`
- `License policy`

The checks come from `.github/workflows/ci.yml`. `Production readiness audit`
keeps the operator command surface read-only, validates release-preview,
install-verification, release-governance, and production-readiness evidence
contracts, and confirms AO Foundry active-stack status remains visible through
`ao-command stack --ledger`.

## Live Verification

After changing branch protection or renaming CI jobs, run:

```sh
scripts/verify-branch-protection.sh
```

The verifier is read-only and defaults to `AO_COMMAND_BRANCH_PROTECTION_MODE=full`,
which checks the administrative branch-protection endpoint. The scheduled/manual
`Production Readiness Ops` workflow uses:

```sh
AO_COMMAND_BRANCH_PROTECTION_MODE=limited scripts/verify-branch-protection.sh
```

Limited mode is used because the default GitHub Actions token cannot read every
administrative branch-protection field. It still verifies that `main` is
protected and that the required status checks are enforced for everyone.
