#!/usr/bin/env bash
set -euo pipefail

forge=""
foundry=""
out=""

while [[ $# -gt 0 ]]; do
  case "$1" in
    --forge)
      forge="${2:-}"
      shift 2
      ;;
    --foundry)
      foundry="${2:-}"
      shift 2
      ;;
    --out)
      out="${2:-}"
      shift 2
      ;;
    *)
      echo "ao-command-smoke: unknown argument $1" >&2
      exit 2
      ;;
  esac
done

if [[ -z "$forge" || -z "$out" ]]; then
  echo "ao-command-smoke: --forge and --out are required" >&2
  exit 2
fi

forge="$(cd "$forge" && pwd)"
if [[ -z "$foundry" ]]; then
  if [[ -d "ao-foundry" ]]; then
    foundry="ao-foundry"
  else
    foundry="../ao-foundry"
  fi
fi
foundry="$(cd "$foundry" && pwd)"
mkdir -p "$out"
out="$(cd "$out" && pwd)"

go run ./cmd/ao-command status --forge "$forge" --json > "$out/status.json"
go run ./cmd/ao-command atlas status --status "$foundry/examples/contract-fixtures/valid/foundry-atlas-status-v0.1.json" --json > "$out/atlas-status.json"
go run ./cmd/ao-command next --forge "$forge" --json > "$out/next.json"
go run ./cmd/ao-command goals \
  --forge "$forge" \
  --goal-run examples/goals/ao2-weekend-hardening.goal-run.json \
  --json > "$out/goal.json"

(
  cd "$forge"
  go run ./cmd/forge production-readiness audit --json > "$out/ao-forge-production-readiness.json"
)

go run ./cmd/ao-command evidence \
  --forge "$forge" \
  --schema docs/contracts/production-readiness-audit-v0.1.schema.json \
  --document "$out/ao-forge-production-readiness.json" > "$out/evidence.txt"

shasum -a 256 "$out"/status.json "$out"/atlas-status.json "$out"/next.json "$out"/goal.json "$out"/ao-forge-production-readiness.json "$out"/evidence.txt > "$out/checksums.txt"

cat > "$out/ao-command-smoke.json" <<JSON
{
  "schema_version": "ao.command.smoke.v0.1",
  "status": "passed",
  "forge": "$forge",
  "artifacts": [
    "$out/status.json",
    "$out/atlas-status.json",
    "$out/next.json",
    "$out/goal.json",
    "$out/ao-forge-production-readiness.json",
    "$out/evidence.txt",
    "$out/checksums.txt"
  ],
  "mutates_repositories": false,
  "uploads_artifacts": false
}
JSON

echo "ao_command_smoke=passed"
echo "evidence=$out/ao-command-smoke.json"
