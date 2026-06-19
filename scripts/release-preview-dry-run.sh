#!/usr/bin/env bash
set -euo pipefail

forge="../ao-forge"
out="tmp/release-preview"
tag="v0.0.0-preview"

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
    --tag)
      tag="${2:-}"
      shift 2
      ;;
    *)
      echo "release-preview-dry-run: unknown argument $1" >&2
      exit 2
      ;;
  esac
done

if [[ -z "$forge" || -z "$out" || -z "$tag" ]]; then
  echo "release-preview-dry-run: --forge, --out, and --tag are required" >&2
  exit 2
fi

root="$(pwd)"
forge="$(cd "$forge" && pwd)"
mkdir -p "$out"
out="$(cd "$out" && pwd)"

status_output="$(git status --porcelain -- ':!tmp' ':!ao-forge' ':!bin' 2>&1)"
if [[ -n "$status_output" ]]; then
  echo "release-preview-dry-run: working tree must be clean except tmp, ao-forge, and bin outputs" >&2
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

bin="$out/ao-command"
smoke_dir="$out/smoke"
audit="$out/release-preview-audit.json"
checksums="$out/checksums.txt"

go test ./... -count=1
go vet ./...
go build -o "$bin" ./cmd/ao-command
scripts/ao-command-smoke.sh --forge "$forge" --out "$smoke_dir"

shasum -a 256 "$bin" "$smoke_dir/ao-command-smoke.json" > "$checksums"

generated_at_utc="$(date -u '+%Y-%m-%dT%H:%M:%SZ')"
head_commit="$(git rev-parse HEAD)"

{
  echo "{"
  echo '  "schema_version": "ao.command.release-preview-audit.v0.1",'
  echo '  "status": "passed",'
  echo "  \"generated_at_utc\": \"$generated_at_utc\","
  echo "  \"tag\": \"$(json_escape "$tag")\","
  echo "  \"head_commit\": \"$head_commit\","
  echo '  "mutates_releases": false,'
  echo '  "uploads_artifacts": false,'
  echo '  "network_required": false,'
  echo '  "checks": ['
  echo '    {"check_id":"go_test","status":"passed","summary":"Go tests passed"},'
  echo '    {"check_id":"go_vet","status":"passed","summary":"go vet passed"},'
  echo '    {"check_id":"go_build","status":"passed","summary":"ao-command release binary built"},'
  echo '    {"check_id":"integration_smoke","status":"passed","summary":"AO Forge-backed smoke evidence passed"},'
  echo '    {"check_id":"checksum_manifest","status":"passed","summary":"release-preview artifacts were checksummed"},'
  echo '    {"check_id":"read_only_preview","status":"passed","summary":"dry run does not mutate releases or upload artifacts"}'
  echo '  ],'
  echo '  "artifacts": ['
  echo "    $(artifact_json "$bin"),"
  echo "    $(artifact_json "$smoke_dir/ao-command-smoke.json"),"
  echo "    $(artifact_json "$checksums")"
  echo '  ],'
  echo '  "next_actions": []'
  echo "}"
} > "$audit"

echo "release_preview_audit=$audit"
