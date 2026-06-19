# Private Repository Guardrails

AO Command is private during v0.1 development. Do not change repository
visibility to public until an explicit release readiness review approves it.

## Current GitHub State

- Repository visibility must remain `PRIVATE`.
- Vulnerability alerts should stay enabled.
- GitHub Actions workflows are present, but private-repo Actions may not run
  until account billing or spending-limit settings allow private runner usage.
- Do not require CI status checks in branch protection while private Actions are
  blocked, or the repository can become unable to merge routine fixes.
- Required signed commits should be enabled after the active GPG public key is
  uploaded to GitHub and GitHub reports test commits as verified. Until then,
  requiring signatures can block all pushes even when commits verify locally.

## Required Local Checks

Run these before pushing while private Actions are unavailable:

```sh
go test ./... -count=1
go vet ./...
go build -o bin/ao-command ./cmd/ao-command
go run github.com/rhysd/actionlint/cmd/actionlint@latest
scripts/ao-command-smoke.sh --forge ../ao-forge --out tmp/ao-command-smoke
scripts/public-readiness-audit.sh --repo uesugitorachiyo/ao-command --forge ../ao-forge --out tmp/public-readiness-audit.json
```

## Publication Gate

Before making AO Command public:

- confirm no secrets, tokens, private keys, private repo names, incident details,
  or provider payloads are present;
- confirm CI is running and required branch checks pass;
- confirm GitHub verifies the configured signing key and then require signed
  commits on the protected branch;
- add release-preview and release-publish gates equivalent to AO Forge;
- create an explicit operator approval record.
