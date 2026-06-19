# Publication Record - 2026-06-19

AO Command was made public on 2026-06-19 after an explicit operator request and
a passing local publication gate.

## Pre-Publication Evidence

- Repository before switch: `visibility=PRIVATE`.
- `scripts/public-readiness-audit.sh --repo uesugitorachiyo/ao-command --forge ../ao-forge --out tmp/public-readiness-audit.json`
  reported `status=passed`.
- `ready_to_request_publication=true`.
- `failed_checks=0`.
- Tracked-file scan found no obvious tokens, private keys, provider secrets, or
  user home absolute paths.
- Git history scan over all five pushed commits found no tracked token,
  private-key, provider-secret, or user home absolute path matches.
- `gitleaks detect --redact --source .` scanned five commits and reported no
  leaks.
- `go test ./... -count=1`, `go vet ./...`, `go build`, `actionlint`, and AO
  Forge-backed smoke checks passed.
- AO Forge production readiness was 100 percent with 12 of 12 gates passing.

## Publication Action

- Repository after switch: `visibility=PUBLIC`.
- Existing branch protection remains enabled for linear history, admin
  enforcement, force-push denial, and branch deletion denial.
- CI remains artifact-free by default.

## Follow-Up

- Run public GitHub Actions on the public repository.
- Add required public CI status checks after the public run passes.
- Revisit GitHub secret scanning and signed-commit enforcement after GitHub
  reports those features available and verified for this repository.
