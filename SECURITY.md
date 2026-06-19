# Security

AO Command is public after passing the v0.1 publication audit. Do not commit
private operational evidence, credentials, provider payloads, or control-plane
exports.

## Public Repository Rules

- Do not commit secrets, tokens, credentials, private keys, provider payloads,
  or local machine paths.
- Do not upload CI artifacts by default unless a release gate explicitly
  requires reviewed public evidence.
- Keep all commands read-only by default.
- Require explicit operator approval before introducing any command that mutates
  branches, releases, provider state, task queues, or control-plane records.
- Require a fresh public-readiness review before adding any cross-repo autonomous
  job scheduler, release publisher, production promoter, or provider mutation
  path. Use `scripts/public-readiness-audit.sh` as the baseline local gate.

## Reporting

For now, report security issues privately to the repository owner. Do not open
public issues or publish proof-of-concept details for suspected vulnerabilities
until a fix is available.
