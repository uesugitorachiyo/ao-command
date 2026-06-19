#!/usr/bin/env bash
set -euo pipefail

repo=""
forge="../ao-forge"
out="tmp/public-readiness-audit.json"
skip_gates=0
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
    --out)
      out="${2:-}"
      shift 2
      ;;
    --skip-gates)
      skip_gates=1
      shift
      ;;
    *)
      echo "public-readiness-audit: unknown argument $1" >&2
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

tracked_files() {
  git ls-files
}

require_no_tracked_match() {
  local check_id="$1"
  local summary="$2"
  local pattern="$3"
  local matches
  if matches="$(tracked_files | xargs rg -n --pcre2 "$pattern" -- 2>/dev/null)"; then
    add_check "$check_id" "failed" "$summary: $matches"
  else
    add_check "$check_id" "passed" "$summary"
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
  if matches="$(printf '%s\n' "$files" | xargs rg -n --pcre2 "$pattern" -- 2>/dev/null)"; then
    add_check "$check_id" "failed" "$summary: $matches"
  else
    add_check "$check_id" "passed" "$summary"
  fi
}

if [[ -n "$(git status --porcelain)" ]]; then
  add_check "clean_worktree" "failed" "working tree must be clean before publication audit"
else
  add_check "clean_worktree" "passed" "working tree is clean"
fi

if [[ -n "$repo" ]]; then
  if visibility="$(gh repo view "$repo" --json isPrivate --jq '.isPrivate' 2>&1)" && [[ "$visibility" == "true" ]]; then
    add_check "repository_private" "passed" "$repo is still private"
  else
    add_check "repository_private" "failed" "$repo must still be private before publication approval: $visibility"
  fi
else
  add_check "repository_private" "skipped" "--repo not provided"
fi

public_scan_files="$(git ls-files | grep -v '^scripts/public-readiness-audit.sh$' | grep -v '^scripts/production-readiness-audit.sh$' | grep -v '^scripts/release-governance-dry-run.sh$' || true)"
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

workflow_and_scripts="$(git ls-files .github scripts | grep -v '^scripts/public-readiness-audit.sh$' | grep -v '^scripts/production-readiness-audit.sh$' | grep -v '^scripts/release-governance-dry-run.sh$' || true)"
require_no_tracked_match_in_files \
  "ci_artifact_uploads" \
  "workflows and scripts do not upload CI artifacts by default" \
  '(actions/upload-artifact|upload-artifact@|gh release upload)' \
  "$workflow_and_scripts"

command_surface_files="$(git ls-files .github cmd internal scripts | grep -v '^scripts/public-readiness-audit.sh$' | grep -v '^scripts/production-readiness-audit.sh$' | grep -v '^scripts/release-governance-dry-run.sh$' || true)"
require_no_tracked_match_in_files \
  "dangerous_write_surface" \
  "command surface has no public-switch, release-publish, production-promotion, or destructive git operations" \
  '(gh repo edit .*--visibility|release[ -]publish|production[ -]promotion|git push --force|git reset --hard|rm -rf /)' \
  "$command_surface_files"

if rg -q "operator-approved public-readiness audit" README.md \
  && rg -q "PUBLICATION-CHECKLIST.md" README.md \
  && rg -q "public after passing the v0.1 publication audit" SECURITY.md; then
  add_check "publication_docs" "passed" "README and SECURITY document private/public boundaries"
else
  add_check "publication_docs" "failed" "README and SECURITY must document private/public boundaries and link the publication checklist"
fi

if [[ "$skip_gates" -eq 0 ]]; then
  run_check "go_test" "Go tests pass" go test ./... -count=1
  run_check "go_vet" "go vet passes" go vet ./...
  run_check "go_build" "ao-command builds" go build -o bin/ao-command ./cmd/ao-command
  run_check "workflow_lint" "GitHub workflow lint passes" go run github.com/rhysd/actionlint/cmd/actionlint@latest
  run_check "integration_smoke" "AO Forge-backed ao-command smoke passes" scripts/ao-command-smoke.sh --forge "$forge" --out tmp/ao-command-smoke

  forge="$(cd "$forge" && pwd)"
  mkdir -p "$(dirname "$out")"
  if (
    cd "$forge"
    go run ./cmd/forge production-readiness audit --json > "$root/tmp/ao-forge-public-readiness.json"
  ); then
    if rg -q '"status": "passed"' tmp/ao-forge-public-readiness.json && rg -q '"readiness_percent": 100' tmp/ao-forge-public-readiness.json; then
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

status="passed"
ready="true"
if [[ "$failed" -gt 0 ]]; then
  status="failed"
  ready="false"
fi

mkdir -p "$(dirname "$out")"
{
  echo "{"
  echo '  "schema_version": "ao.command.public-readiness-audit.v0.1",'
  echo "  \"status\": \"$status\","
  echo "  \"ready_to_request_publication\": $ready,"
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

echo "public_readiness_audit=$status"
echo "failed_checks=$failed"
echo "audit=$out"

if [[ "$status" != "passed" ]]; then
  exit 1
fi
