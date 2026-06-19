# Security

AO Command is a private repository during v0.1 development. Do not make this
repository public until the operator, security, and release gates explicitly
approve publication.

## Private-by-Default Rules

- Keep the GitHub repository private.
- Do not commit secrets, tokens, credentials, private keys, provider payloads,
  or local machine paths.
- Do not upload CI artifacts by default while the repository is private and the
  evidence model is still being hardened.
- Do not assume private GitHub Actions are available until billing or
  spending-limit settings allow private runner usage; use the local verification
  gate in `docs/operations/PRIVATE-REPO-GUARDRAILS.md` while Actions are blocked.
- Keep all commands read-only by default.
- Require explicit operator approval before introducing any command that mutates
  branches, releases, provider state, task queues, or control-plane records.
- Require `docs/operations/PUBLICATION-CHECKLIST.md` and
  `scripts/public-readiness-audit.sh` to pass before making the repository
  public.

## Reporting

For now, report security issues privately to the repository owner. Do not open
public issues or publish proof-of-concept details while the repository remains
private.
