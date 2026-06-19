# Publication Checklist

AO Command was made public only after this checklist passed and the operator
explicitly approved publication.

## Required Evidence

- `scripts/public-readiness-audit.sh --repo uesugitorachiyo/ao-command --forge ../ao-forge --out tmp/public-readiness-audit.json`
  reports `status=passed`.
- The audit reports `ready_to_request_publication=true`.
- AO Forge production readiness is 100 percent.
- The latest local smoke evidence reports `mutates_repositories=false` and
  `uploads_artifacts=false`.
- The repository reported `visibility=PRIVATE` before publication approval.

## Public Safety Review

Confirm no tracked file contains:

- secrets, tokens, private keys, or provider credentials;
- private operational evidence;
- private incident or security review details;
- private control-plane exports;
- user home absolute paths;
- CI artifact upload defaults;
- dangerous branch, release, provider, or control-plane write commands.

## Operator Approval

Only after the required evidence and public safety review pass:

1. Record explicit operator approval.
2. Change repository visibility from private to public.
3. Run public GitHub Actions.
4. Enable required CI branch protection after public Actions pass.
5. Re-enable signed-commit enforcement after GitHub verifies the signing key.

Publication approval was granted after the audit passed with zero failed checks.
