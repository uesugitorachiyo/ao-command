# Publication Checklist

AO Command must remain private until this checklist passes and the operator
explicitly approves making the repository public.

## Required Evidence

- `scripts/public-readiness-audit.sh --repo uesugitorachiyo/ao-command --forge ../ao-forge --out tmp/public-readiness-audit.json`
  reports `status=passed`.
- The audit reports `ready_to_request_publication=true`.
- AO Forge production readiness is 100 percent.
- The latest local smoke evidence reports `mutates_repositories=false` and
  `uploads_artifacts=false`.
- The repository still reports `visibility=PRIVATE` before the publication
  approval.

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

Until the operator asks for publication, do not change repository visibility.
