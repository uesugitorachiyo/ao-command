#!/usr/bin/env bash
set -euo pipefail

out="tmp/release-governance"
tag="v0.1.0"
release_preview_audit=""
install_verify_audit=""

while [[ $# -gt 0 ]]; do
  case "$1" in
    --out)
      out="${2:-}"
      shift 2
      ;;
    --tag)
      tag="${2:-}"
      shift 2
      ;;
    --release-preview-audit)
      release_preview_audit="${2:-}"
      shift 2
      ;;
    --install-verify-audit)
      install_verify_audit="${2:-}"
      shift 2
      ;;
    *)
      echo "release-governance-dry-run: unknown argument $1" >&2
      exit 2
      ;;
  esac
done

if [[ -z "$out" || -z "$tag" || -z "$release_preview_audit" || -z "$install_verify_audit" ]]; then
  echo "release-governance-dry-run: --out, --tag, --release-preview-audit, and --install-verify-audit are required" >&2
  exit 2
fi

status_output="$(git status --porcelain -- ':!tmp' ':!ao-forge' ':!ao-foundry' ':!ao-covenant' ':!bin' 2>&1)"
if [[ -n "$status_output" ]]; then
  echo "release-governance-dry-run: working tree must be clean except tmp, ao-forge, ao-foundry, ao-covenant, and bin outputs" >&2
  printf '%s\n' "$status_output" >&2
  exit 1
fi

mkdir -p "$out"
out="$(cd "$out" && pwd)"
release_preview_audit="$(cd "$(dirname "$release_preview_audit")" && pwd)/$(basename "$release_preview_audit")"
install_verify_audit="$(cd "$(dirname "$install_verify_audit")" && pwd)/$(basename "$install_verify_audit")"
audit="$out/release-governance-audit.json"

grep -q '"schema_version": "ao.command.release-preview-audit.v0.1"' "$release_preview_audit"
grep -q '"status": "passed"' "$release_preview_audit"
grep -q '"mutates_releases": false' "$release_preview_audit"
grep -q '"uploads_artifacts": false' "$release_preview_audit"

grep -q '"schema_version": "ao.command.install-verify-audit.v0.1"' "$install_verify_audit"
grep -q '"status": "passed"' "$install_verify_audit"
grep -q '"mutates_releases": false' "$install_verify_audit"
grep -q '"uploads_artifacts": false' "$install_verify_audit"

json_escape() {
  printf '%s' "$1" | sed 's/\\/\\\\/g; s/"/\\"/g'
}

generated_at_utc="$(date -u '+%Y-%m-%dT%H:%M:%SZ')"
head_commit="$(git rev-parse HEAD)"

{
  echo "{"
  echo '  "schema_version": "ao.command.release-governance-audit.v0.1",'
  echo '  "status": "blocked",'
  echo "  \"generated_at_utc\": \"$generated_at_utc\","
  echo "  \"tag\": \"$(json_escape "$tag")\","
  echo "  \"head_commit\": \"$head_commit\","
  echo '  "decision": "blocked_pending_operator_approval",'
  echo '  "would_create_release": false,'
  echo '  "mutates_repositories": false,'
  echo '  "mutates_releases": false,'
  echo '  "uploads_artifacts": false,'
  echo '  "network_required": false,'
  echo '  "required_evidence": ['
  echo "    {\"evidence_id\":\"release-preview-dry-run\",\"path\":\"$(json_escape "$release_preview_audit")\",\"status\":\"present\"},"
  echo "    {\"evidence_id\":\"install-verify-dry-run\",\"path\":\"$(json_escape "$install_verify_audit")\",\"status\":\"present\"}"
  echo '  ],'
  echo '  "checks": ['
  echo '    {"check_id":"release_preview_evidence","status":"passed","summary":"release-preview dry-run evidence is present and read-only"},'
  echo '    {"check_id":"install_verify_evidence","status":"passed","summary":"install verification dry-run evidence is present and read-only"},'
  echo '    {"check_id":"operator_approval_required","status":"passed","summary":"release creation remains blocked pending explicit operator approval"},'
  echo '    {"check_id":"no_network_or_upload","status":"passed","summary":"dry run does not require network access or artifact uploads"}'
  echo '  ],'
  echo '  "next_actions": ['
  echo '    {"action_id":"operator-approve-release-publication","description":"Add explicit release publication policy before creating a GitHub release.","required":true}'
  echo '  ]'
  echo "}"
} > "$audit"

echo "release_governance_audit=$audit"
