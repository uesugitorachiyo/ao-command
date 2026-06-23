#!/usr/bin/env bash
set -euo pipefail

forge="../ao-forge"
out="tmp/install-verify"

while [[ $# -gt 0 ]]; do
  case "$1" in
    --forge)
      forge="${2:-}"
      shift 2
      ;;
    --out)
      out="${2:-}"
      shift 2
      ;;
    *)
      echo "install-verify-dry-run: unknown argument $1" >&2
      exit 2
      ;;
  esac
done

if [[ -z "$forge" || -z "$out" ]]; then
  echo "install-verify-dry-run: --forge and --out are required" >&2
  exit 2
fi

root="$(pwd)"
forge="$(cd "$forge" && pwd)"
mkdir -p "$out/bin" "$out/commands"
out="$(cd "$out" && pwd)"

status_output="$(git status --porcelain -- ':!tmp' ':!ao-forge' ':!ao-foundry' ':!bin' 2>&1)"
if [[ -n "$status_output" ]]; then
  echo "install-verify-dry-run: working tree must be clean except tmp, ao-forge, ao-foundry, and bin outputs" >&2
  printf '%s\n' "$status_output" >&2
  exit 1
fi

json_escape() {
  printf '%s' "$1" | sed 's/\\/\\\\/g; s/"/\\"/g'
}

artifact_json() {
  local path="$1"
  local checksum size
  checksum="$(shasum -a 256 "$path" | awk '{print $1}')"
  size="$(wc -c < "$path" | tr -d ' ')"
  printf '{"path":"%s","sha256":"%s","size_bytes":%s}' "$(json_escape "$path")" "$checksum" "$size"
}

bin="$out/bin/ao-command"
help_out="$out/commands/help.txt"
status_json="$out/commands/status.json"
next_json="$out/commands/next.json"
checksums="$out/checksums.txt"
audit="$out/install-verify-audit.json"

go build -o "$bin" ./cmd/ao-command
"$bin" --help > "$help_out"
"$bin" status --forge "$forge" --json > "$status_json"
"$bin" next --forge "$forge" --json > "$next_json"

grep -q "ao-command is the read-only operator command surface" "$help_out"
grep -q '"command_schema_version": "ao.command.v0.1"' "$status_json"
grep -q '"command_schema_version": "ao.command.v0.1"' "$next_json"

shasum -a 256 "$bin" "$help_out" "$status_json" "$next_json" > "$checksums"

generated_at_utc="$(date -u '+%Y-%m-%dT%H:%M:%SZ')"
head_commit="$(git rev-parse HEAD)"

{
  echo "{"
  echo '  "schema_version": "ao.command.install-verify-audit.v0.1",'
  echo '  "status": "passed",'
  echo "  \"generated_at_utc\": \"$generated_at_utc\","
  echo "  \"head_commit\": \"$head_commit\","
  echo "  \"installed_binary\": \"$(json_escape "$bin")\","
  echo '  "mutates_repositories": false,'
  echo '  "mutates_releases": false,'
  echo '  "uploads_artifacts": false,'
  echo '  "network_required": false,'
  echo '  "checks": ['
  echo '    {"check_id":"binary_build","status":"passed","summary":"ao-command binary built into the dry-run install directory"},'
  echo '    {"check_id":"help_command","status":"passed","summary":"installed binary renders operator help"},'
  echo '    {"check_id":"status_command","status":"passed","summary":"installed binary reads AO Forge readiness through status --json"},'
  echo '    {"check_id":"next_command","status":"passed","summary":"installed binary reads AO Forge next action through next --json"},'
  echo '    {"check_id":"checksum_manifest","status":"passed","summary":"install verification artifacts were checksummed"},'
  echo '    {"check_id":"read_only_install_verify","status":"passed","summary":"dry run does not mutate repositories, releases, or provider state"}'
  echo '  ],'
  echo '  "artifacts": ['
  echo "    $(artifact_json "$bin"),"
  echo "    $(artifact_json "$help_out"),"
  echo "    $(artifact_json "$status_json"),"
  echo "    $(artifact_json "$next_json"),"
  echo "    $(artifact_json "$checksums")"
  echo '  ],'
  echo '  "next_actions": []'
  echo "}"
} > "$audit"

echo "install_verify_audit=$audit"
