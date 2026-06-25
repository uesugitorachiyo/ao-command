#!/usr/bin/env bash
set -euo pipefail

repo="uesugitorachiyo/ao-command"
forge="../ao-forge"
foundry="../ao-foundry"
covenant="../ao-covenant"
architecture="../ao-architecture"
out="tmp/production-readiness-audit.json"
skip_gates=0
skip_remote_admin=0
root="$(pwd)"

while [[ $# -gt 0 ]]; do
  case "$1" in
    --repo)
      repo="${2:-}"
      shift 2
      ;;
    --forge)
      forge="${2:-}"
      shift 2
      ;;
    --foundry)
      foundry="${2:-}"
      shift 2
      ;;
    --covenant)
      covenant="${2:-}"
      shift 2
      ;;
    --architecture)
      architecture="${2:-}"
      shift 2
      ;;
    --out)
      out="${2:-}"
      shift 2
      ;;
    --skip-gates)
      skip_gates=1
      shift
      ;;
    --skip-remote-admin)
      skip_remote_admin=1
      shift
      ;;
    *)
      echo "production-readiness-audit: unknown argument $1" >&2
      exit 2
      ;;
  esac
done

json_escape() {
  printf '%s' "$1" | sed 's/\\/\\\\/g; s/"/\\"/g'
}

checks=()
failed=0
passed_count=0
counted=0

add_check() {
  local check_id="$1"
  local status="$2"
  local summary
  summary="$(printf '%s' "$3" | tr '\n\r' '  ')"
  checks+=("{\"check_id\":\"$(json_escape "$check_id")\",\"status\":\"$(json_escape "$status")\",\"summary\":\"$(json_escape "$summary")\"}")
  if [[ "$status" == "passed" ]]; then
    counted=$((counted + 1))
    passed_count=$((passed_count + 1))
  elif [[ "$status" == "failed" ]]; then
    counted=$((counted + 1))
    failed=$((failed + 1))
  fi
}

run_check() {
  local check_id="$1"
  local summary="$2"
  shift 2
  local output
  if output="$("$@" 2>&1)"; then
    add_check "$check_id" "passed" "$summary"
  else
    add_check "$check_id" "failed" "$summary: $output"
  fi
}

require_no_tracked_match_in_files() {
  local check_id="$1"
  local summary="$2"
  local pattern="$3"
  local files="$4"
  local matches
  if [[ -z "$files" ]]; then
    add_check "$check_id" "passed" "$summary"
    return
  fi
  if matches="$(printf '%s\n' "$files" | xargs grep -nE -- "$pattern" 2>/dev/null)"; then
    add_check "$check_id" "failed" "$summary: $matches"
  else
    add_check "$check_id" "passed" "$summary"
  fi
}

public_scan_files="$(git ls-files | grep -v '^scripts/production-readiness-audit.sh$' | grep -v '^scripts/public-readiness-audit.sh$' | grep -v '^scripts/release-preview-dry-run.sh$' | grep -v '^scripts/install-verify-dry-run.sh$' | grep -v '^scripts/release-governance-dry-run.sh$' || true)"
workflow_and_scripts="$(git ls-files .github scripts | grep -v '^scripts/production-readiness-audit.sh$' | grep -v '^scripts/public-readiness-audit.sh$' | grep -v '^scripts/release-preview-dry-run.sh$' | grep -v '^scripts/install-verify-dry-run.sh$' | grep -v '^scripts/release-governance-dry-run.sh$' || true)"
command_surface_files="$(git ls-files .github cmd internal scripts | grep -v '^scripts/production-readiness-audit.sh$' | grep -v '^scripts/public-readiness-audit.sh$' | grep -v '^scripts/release-preview-dry-run.sh$' | grep -v '^scripts/install-verify-dry-run.sh$' | grep -v '^scripts/release-governance-dry-run.sh$' || true)"

status_output="$(git status --porcelain -- ':!tmp' ':!ao-forge' ':!ao-foundry' ':!ao-covenant' ':!ao-architecture' ':!bin' 2>&1)"
if [[ -n "$status_output" ]]; then
  add_check "clean_worktree" "failed" "working tree must be clean for production readiness: $status_output"
else
  add_check "clean_worktree" "passed" "working tree is clean"
fi

if repo_meta="$(gh repo view "$repo" --json isPrivate,visibility,deleteBranchOnMerge --jq '.')" \
  && printf '%s' "$repo_meta" | grep -qE '"isPrivate":false' \
  && printf '%s' "$repo_meta" | grep -qE '"visibility":"PUBLIC"' \
  && printf '%s' "$repo_meta" | grep -qE '"deleteBranchOnMerge":true'; then
  add_check "repository_public" "passed" "$repo is public and deletes merged branches"
else
  add_check "repository_public" "failed" "$repo must be public with delete-branch-on-merge enabled: ${repo_meta:-unavailable}"
fi

if [[ "$skip_remote_admin" -eq 1 ]]; then
  add_check "branch_protection" "skipped" "remote admin gate skipped in non-admin CI token mode"
else
  if protection="$(gh api "repos/$repo/branches/main/protection" 2>&1)" \
    && printf '%s' "$protection" | grep -qE '"strict":true' \
    && printf '%s' "$protection" | grep -qE '"context":"License policy"' \
    && printf '%s' "$protection" | grep -qE '"context":"Go"' \
    && printf '%s' "$protection" | grep -qE '"context":"Workflow lint"' \
    && printf '%s' "$protection" | grep -qE '"context":"Production readiness audit"' \
    && printf '%s' "$protection" | grep -qE '"enforce_admins".*"enabled":true' \
    && printf '%s' "$protection" | grep -qE '"required_linear_history".*"enabled":true' \
    && printf '%s' "$protection" | grep -qE '"allow_force_pushes".*"enabled":false' \
    && printf '%s' "$protection" | grep -qE '"allow_deletions".*"enabled":false'; then
    add_check "branch_protection" "passed" "main requires strict License policy, Go, Workflow lint, and Production readiness audit checks and denies force-push/delete"
  else
    add_check "branch_protection" "failed" "main branch protection is incomplete: ${protection:-unavailable}"
  fi
fi

if [[ "$skip_remote_admin" -eq 1 ]]; then
  add_check "secret_scanning" "skipped" "remote admin gate skipped in non-admin CI token mode"
else
  if security="$(gh api "repos/$repo" --jq '.security_and_analysis' 2>&1)" \
    && printf '%s' "$security" | grep -qE '"secret_scanning".*"enabled"' \
    && printf '%s' "$security" | grep -qE '"secret_scanning_push_protection".*"enabled"'; then
    add_check "secret_scanning" "passed" "secret scanning and push protection are enabled"
  else
    add_check "secret_scanning" "failed" "secret scanning and push protection must be enabled: ${security:-unavailable}"
  fi
fi

if [[ "$skip_remote_admin" -eq 1 ]]; then
  add_check "vulnerability_alerts" "skipped" "remote admin gate skipped in non-admin CI token mode"
else
  if gh api "repos/$repo/vulnerability-alerts" -i >/tmp/ao-command-vulnerability-alerts.headers 2>&1 \
    && grep -qE "204 No Content" /tmp/ao-command-vulnerability-alerts.headers; then
    add_check "vulnerability_alerts" "passed" "vulnerability alerts are enabled"
  else
    add_check "vulnerability_alerts" "failed" "vulnerability alerts must be enabled"
  fi
fi

require_no_tracked_match_in_files \
  "secret_patterns" \
  "tracked files contain no obvious tokens, private keys, or provider secrets" \
  '(ghp_[A-Za-z0-9_]{20,}|github_pat_[A-Za-z0-9_]{20,}|sk-[A-Za-z0-9_-]{20,}|AKIA[0-9A-Z]{16}|-----BEGIN (RSA |OPENSSH |EC |DSA )?PRIVATE KEY-----|xox[baprs]-[A-Za-z0-9-]{10,})' \
  "$public_scan_files"

require_no_tracked_match_in_files \
  "machine_local_paths" \
  "tracked files contain no user home absolute paths" \
  '(/Users/[^[:space:]")]+|/home/[^[:space:]")]+|C:/Users/[^[:space:]")]+)' \
  "$public_scan_files"

require_no_tracked_match_in_files \
  "ci_artifact_uploads" \
  "workflows and scripts do not upload CI artifacts by default" \
  '(actions/upload-artifact|upload-artifact@|gh release upload)' \
  "$workflow_and_scripts"

require_no_tracked_match_in_files \
  "dangerous_write_surface" \
  "command surface has no public-switch, release-publish, production-promotion, or destructive git operations" \
  '(gh repo edit .*--visibility|release[ -]publish|production[ -]promotion|git push --force|git reset --hard|rm -rf /)' \
  "$command_surface_files"

if grep -qE "Production Readiness" README.md \
  && grep -qE "PRODUCTION-READINESS.md" README.md \
  && grep -qE "PUBLICATION-RECORD-2026-06-19.md" README.md \
  && grep -qE "public after passing the v0.1 publication audit" SECURITY.md; then
  add_check "readiness_docs" "passed" "README and SECURITY document production/public readiness"
else
  add_check "readiness_docs" "failed" "README and SECURITY must document production/public readiness"
fi

if grep -qE "production-readiness-audit.sh" .github/workflows/ci.yml \
  && grep -qE "name: Production readiness audit" .github/workflows/ci.yml \
  && grep -qE "workflow_dispatch" .github/workflows/ci.yml; then
  add_check "ci_readiness_job" "passed" "CI defines a production readiness audit job and manual dispatch"
else
  add_check "ci_readiness_job" "failed" "CI must define production readiness audit and manual dispatch"
fi

if grep -qE "ao.command.production-readiness-audit.v0.1" docs/contracts/production-readiness-audit-v0.1.schema.json \
  && grep -qE '"additionalProperties": false' docs/contracts/production-readiness-audit-v0.1.schema.json \
  && grep -qE "Validate production readiness contract" .github/workflows/ci.yml \
  && grep -qE "production-readiness-audit-v0.1.schema.json" .github/workflows/ci.yml; then
  add_check "readiness_contract" "passed" "production readiness audit has a schema contract validated in CI"
else
  add_check "readiness_contract" "failed" "production readiness audit must have a schema contract validated in CI"
fi

if grep -qE "ao.command.release-preview-audit.v0.1" docs/contracts/release-preview-audit-v0.1.schema.json \
  && grep -qE '"mutates_releases"' docs/contracts/release-preview-audit-v0.1.schema.json \
  && grep -qE "release-preview-dry-run.sh" .github/workflows/ci.yml; then
  add_check "release_preview_contract" "passed" "release preview dry-run contract is present and wired into CI"
else
  add_check "release_preview_contract" "failed" "release preview dry-run contract must be present and wired into CI"
fi

if grep -qE "ao.command.install-verify-audit.v0.1" docs/contracts/install-verify-audit-v0.1.schema.json \
  && grep -qE '"mutates_repositories"' docs/contracts/install-verify-audit-v0.1.schema.json \
  && grep -qE "install-verify-dry-run.sh" .github/workflows/ci.yml; then
  add_check "install_verify_contract" "passed" "install verification dry-run contract is present and wired into CI"
else
  add_check "install_verify_contract" "failed" "install verification dry-run contract must be present and wired into CI"
fi

if grep -qE "ao.command.release-governance-audit.v0.1" docs/contracts/release-governance-audit-v0.1.schema.json \
  && grep -qE '"would_create_release"' docs/contracts/release-governance-audit-v0.1.schema.json \
  && grep -qE "release-governance-dry-run.sh" .github/workflows/ci.yml; then
  add_check "release_governance_contract" "passed" "release governance dry-run contract is present and wired into CI"
else
  add_check "release_governance_contract" "failed" "release governance dry-run contract must be present and wired into CI"
fi

if grep -qE "ao.command.rsi-health.v0.1" docs/contracts/rsi-health-v0.1.schema.json \
  && grep -qE "ao.command.rsi-health-bundle.v0.1" docs/contracts/rsi-health-bundle-v0.1.schema.json \
  && grep -qE '"claim_levels"' docs/contracts/rsi-health-v0.1.schema.json \
  && grep -qE '"sha256"' docs/contracts/rsi-health-bundle-v0.1.schema.json \
  && grep -qE "rsi manifest --manifest" README.md \
  && grep -qE "rsi manifest --manifest" docs/operations/PRODUCTION-READINESS.md \
  && grep -qE "Validate RSI health contract" .github/workflows/ci.yml \
  && grep -qE "Validate RSI health bundle contract" .github/workflows/ci.yml \
  && grep -qE "RSI claim manifest" .github/workflows/ci.yml; then
  add_check "rsi_health_contract" "passed" "RSI health, bundle, and claim manifest checks are present and wired into CI"
else
  add_check "rsi_health_contract" "failed" "RSI health, bundle, and claim manifest checks must be present and wired into CI"
fi

if grep -qE "ao.command.public-provenance-manifest.v0.1" docs/operations/public-provenance-manifest.json \
  && grep -qE "release-preview-dry-run" docs/operations/public-provenance-manifest.json \
  && grep -qE "install-verify-dry-run" docs/operations/public-provenance-manifest.json \
  && grep -qE "release-governance-dry-run" docs/operations/public-provenance-manifest.json \
  && grep -qE "rsi-health" docs/operations/public-provenance-manifest.json \
  && grep -qE "rsi-health-bundle" docs/operations/public-provenance-manifest.json \
  && grep -qE '"default_ci_artifact_uploads": false' docs/operations/public-provenance-manifest.json \
  && grep -qE "Do not upload CI artifacts by default" docs/operations/RETAINED-EVIDENCE.md; then
  add_check "retained_evidence_policy" "passed" "public-safe retained evidence policy and manifest are present"
else
  add_check "retained_evidence_policy" "failed" "public-safe retained evidence policy and manifest must cover dry-run evidence"
fi

if grep -qE "AO Command v0.1.0 Operator Closeout" docs/release/V0.1.0-OPERATOR-CLOSEOUT.md \
  && grep -qE "Required Evidence Before Tagging" docs/release/V0.1.0-OPERATOR-CLOSEOUT.md \
  && grep -qE "readiness_percent=100" docs/release/V0.1.0-OPERATOR-CLOSEOUT.md \
  && grep -qE "public provenance manifest" docs/release/V0.1.0-OPERATOR-CLOSEOUT.md; then
  add_check "operator_closeout" "passed" "v0.1.0 operator closeout documents ready scope, evidence, and remaining actions"
else
  add_check "operator_closeout" "failed" "v0.1.0 operator closeout must document ready scope, evidence, and remaining actions"
fi

if [[ "$skip_gates" -eq 0 ]]; then
  run_check "go_test" "Go tests pass" go test ./... -count=1
  run_check "go_vet" "go vet passes" go vet ./...
  run_check "go_build" "ao-command builds" go build -o bin/ao-command ./cmd/ao-command
  run_check "workflow_lint" "GitHub workflow lint passes" go run github.com/rhysd/actionlint/cmd/actionlint@latest
  run_check "integration_smoke" "AO Forge-backed ao-command smoke passes" scripts/ao-command-smoke.sh --forge "$forge" --out tmp/ao-command-smoke
  run_check "release_preview_dry_run" "AO Command release preview dry-run passes" scripts/release-preview-dry-run.sh --forge "$forge" --out tmp/ao-command-release-preview --tag v0.1.0-preview
  run_check "release_preview_contract_validate" "AO Command release preview audit validates against its schema" go run ./cmd/ao-command evidence --forge "$forge" --schema "$root/docs/contracts/release-preview-audit-v0.1.schema.json" --document "$root/tmp/ao-command-release-preview/release-preview-audit.json"
  run_check "install_verify_dry_run" "AO Command install verification dry-run passes" scripts/install-verify-dry-run.sh --forge "$forge" --out tmp/ao-command-install-verify
  run_check "install_verify_contract_validate" "AO Command install verification audit validates against its schema" go run ./cmd/ao-command evidence --forge "$forge" --schema "$root/docs/contracts/install-verify-audit-v0.1.schema.json" --document "$root/tmp/ao-command-install-verify/install-verify-audit.json"
  run_check "release_governance_dry_run" "AO Command release governance dry-run passes" scripts/release-governance-dry-run.sh --out tmp/ao-command-release-governance --tag v0.1.0 --release-preview-audit tmp/ao-command-release-preview/release-preview-audit.json --install-verify-audit tmp/ao-command-install-verify/install-verify-audit.json
  run_check "release_governance_contract_validate" "AO Command release governance audit validates against its schema" go run ./cmd/ao-command evidence --forge "$forge" --schema "$root/docs/contracts/release-governance-audit-v0.1.schema.json" --document "$root/tmp/ao-command-release-governance/release-governance-audit.json"
  run_check "active_stack_status" "AO Command reads AO Foundry active-stack handoff status without orchestration" go run ./cmd/ao-command stack --ledger "$foundry/examples/readiness/active-stack-readiness.ledger.json"
  run_check "rsi_evidence_chain_smoke" "Foundry pulse, Forge retained proofs, Command health, and Covenant RSI claim boundary pass" scripts/rsi-evidence-chain-smoke.sh --forge "$forge" --foundry "$foundry" --covenant "$covenant" --out tmp/rsi-evidence-chain-smoke
  run_check "rsi_claim_manifest" "AO Command validates AO Architecture RSI claim manifest without mutation" go run ./cmd/ao-command rsi manifest --manifest "$architecture/overview/rsi-claim-evidence-manifest.json"
  run_check "rsi_health_contract_validate" "AO Command RSI health JSON validates against its schema" go run ./cmd/ao-command evidence --forge "$forge" --schema "$root/docs/contracts/rsi-health-v0.1.schema.json" --document "$root/tmp/rsi-evidence-chain-smoke/ao-command-rsi-health.json"
  run_check "rsi_health_bundle_contract_validate" "AO Command RSI health bundle validates against its schema" go run ./cmd/ao-command evidence --forge "$forge" --schema "$root/docs/contracts/rsi-health-bundle-v0.1.schema.json" --document "$root/tmp/rsi-evidence-chain-smoke/rsi-health-bundle.json"

  forge="$(cd "$forge" && pwd)"
  mkdir -p "$root/tmp"
  if (
    cd "$forge"
    go run ./cmd/forge production-readiness audit --json > "$root/tmp/ao-forge-production-readiness.json"
  ); then
    if grep -qE '"status": "passed"' tmp/ao-forge-production-readiness.json && grep -qE '"readiness_percent": 100' tmp/ao-forge-production-readiness.json; then
      add_check "ao_forge_readiness" "passed" "AO Forge production readiness is 100 percent"
    else
      add_check "ao_forge_readiness" "failed" "AO Forge readiness audit did not report 100 percent"
    fi
  else
    add_check "ao_forge_readiness" "failed" "AO Forge readiness audit failed"
  fi
else
  add_check "local_gates" "skipped" "--skip-gates requested"
fi

total=$counted
passed=$passed_count
readiness_percent=0
if [[ "$total" -gt 0 ]]; then
  readiness_percent=$((passed * 100 / total))
fi

status="passed"
if [[ "$failed" -gt 0 ]]; then
  status="failed"
fi

mkdir -p "$(dirname "$out")"
{
  echo "{"
  echo '  "schema_version": "ao.command.production-readiness-audit.v0.1",'
  echo "  \"status\": \"$status\","
  echo "  \"readiness_percent\": $readiness_percent,"
  echo "  \"passed_gates\": $passed,"
  echo "  \"total_gates\": $total,"
  echo "  \"failed_checks\": $failed,"
  echo '  "checks": ['
  for i in "${!checks[@]}"; do
    suffix=","
    if [[ "$i" -eq $((${#checks[@]} - 1)) ]]; then
      suffix=""
    fi
    echo "    ${checks[$i]}$suffix"
  done
  echo "  ]"
  echo "}"
} > "$out"

echo "production_readiness_audit=$status"
echo "readiness_percent=$readiness_percent"
echo "gates=$passed/$total"
echo "audit=$out"

if [[ "$status" != "passed" ]]; then
  exit 1
fi
