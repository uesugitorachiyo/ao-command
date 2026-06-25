#!/usr/bin/env bash
set -euo pipefail

forge=""
foundry=""
covenant=""
out="tmp/rsi-evidence-chain-smoke"

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
    --covenant)
      covenant="${2:-}"
      shift 2
      ;;
    --out)
      out="${2:-}"
      shift 2
      ;;
    *)
      echo "rsi-evidence-chain-smoke: unknown argument $1" >&2
      exit 2
      ;;
  esac
done

if [[ -z "$forge" || -z "$foundry" || -z "$covenant" || -z "$out" ]]; then
  echo "rsi-evidence-chain-smoke: --forge, --foundry, --covenant, and --out are required" >&2
  exit 2
fi

json_escape() {
  printf '%s' "$1" | sed 's/\\/\\\\/g; s/"/\\"/g'
}

root="$(pwd)"
forge="$(cd "$forge" && pwd)"
foundry="$(cd "$foundry" && pwd)"
covenant="$(cd "$covenant" && pwd)"
mkdir -p "$out"
out="$(cd "$out" && pwd)"

assurance_dir="$out/assurance"
foundry_pulse_dir="$out/foundry-pulse"
covenant_work_dir="$out/covenant-work"
mkdir -p "$assurance_dir" "$foundry_pulse_dir" "$covenant_work_dir"

cat > "$assurance_dir/arena-promotion-gate.json" <<'JSON'
{
  "schema_version": "ao.arena.promotion-gate.v0.1",
  "status": "passed",
  "winner": "ao-orchestration"
}
JSON

cat > "$assurance_dir/crucible-hardening-gate.json" <<'JSON'
{
  "schema_version": "ao.crucible.hardening-gate.v0.1",
  "status": "passed",
  "score": 97
}
JSON

cat > "$assurance_dir/sentinel-verdict.json" <<'JSON'
{
  "schema_version": "ao.sentinel.verdict.v0.1",
  "verdict": "clear",
  "safety_status": "passed",
  "regression_status": "passed",
  "promoter_hold_required": false,
  "mutates_live_state": false
}
JSON

cat > "$assurance_dir/promoter-gate.json" <<'JSON'
{
  "schema_version": "ao.promoter.gate.v0.1",
  "status": "passed",
  "promotion_allowed": true,
  "activation_plan_allowed": true,
  "blockers": []
}
JSON

(
  cd "$foundry"
  go run ./cmd/foundry pulse run \
    --out "$foundry_pulse_dir" \
    --rsi-baseline examples/evals/rsi-baseline.eval-result.json \
    --rsi-min-improvement 5 > "$out/foundry-pulse.txt"
)

proof_dir="$forge/docs/evidence/goals/ao2-weekend-hardening/20260619T180000Z-verification"
go run ./cmd/ao-command rsi health \
  --arena-gate "$assurance_dir/arena-promotion-gate.json" \
  --crucible-gate "$assurance_dir/crucible-hardening-gate.json" \
  --sentinel-verdict "$assurance_dir/sentinel-verdict.json" \
  --promoter-gate "$assurance_dir/promoter-gate.json" \
  --foundry-gate "$foundry_pulse_dir/rsi-improvement-gate.json" \
  --foundry-candidate "$foundry_pulse_dir/rsi-candidate.json" \
  --foundry-next-task "$foundry_pulse_dir/rsi-next-improvement-task.json" \
  --forge-retained-gate "$proof_dir/ao-foundry-rsi-improvement-gate-retention-proof.json" \
  --forge-retained-candidate "$proof_dir/ao-foundry-rsi-candidate-retention-proof.json" \
  --forge-retained-next-task "$proof_dir/ao-foundry-rsi-next-improvement-task-retention-proof.json" \
  --forge-retained-command-health "$proof_dir/ao-command-rsi-health-retention-proof.json" \
  --bundle-out "$out/rsi-health-bundle.json" \
  --json > "$out/ao-command-rsi-health.json"

grep -q '"status": "passed"' "$out/ao-command-rsi-health.json"
grep -q '"rsi_capability": "demonstrated_local_fixture_loop"' "$out/ao-command-rsi-health.json"
grep -q '"mutates_repositories": false' "$out/ao-command-rsi-health.json"

cat > "$covenant_work_dir/brief.md" <<'EOF_BRIEF'
Attempt to publish full autonomous self-mutating RSI without the required
mutation authority, rollback, and live self-change evidence.
EOF_BRIEF

cat > "$covenant_work_dir/full-rsi-claim.contract.json" <<'JSON'
{
  "schema_version": "covenant.contract.v1",
  "objective": "Attempt to publish full autonomous self-mutating RSI claim without required evidence.",
  "workspace": {
    "root": ".",
    "reads": ["brief.md"],
    "writes": []
  },
  "tasks": [
    {
      "id": "publish_rsi_claim",
      "kind": "scripted",
      "adapter": "process",
      "depends_on": [],
      "obligations": ["obl_rsi_claim_boundary"],
      "declared_side_effects": [
        {
          "type": "claim.publish",
          "resource": "full-autonomous-self-mutating-rsi"
        }
      ],
      "timeout_seconds": 60
    }
  ],
  "obligations": [
    {
      "id": "obl_rsi_claim_boundary",
      "text": "Do not publish full autonomous self-mutating RSI without stronger evidence.",
      "required": true
    }
  ],
  "policy": {
    "mode": "strict"
  },
  "approvals": [],
  "evaluator": {
    "required_obligations": ["obl_rsi_claim_boundary"]
  }
}
JSON

set +e
(
  cd "$covenant"
  go run ./cmd/covenant run \
    --contract "$covenant_work_dir/full-rsi-claim.contract.json" \
    --workspace "$covenant_work_dir" \
    --out "$covenant_work_dir/.covenant/runs" \
    --run-id rsi-claim-boundary \
    --json > "$out/covenant-run.json" 2> "$out/covenant-run.stderr.txt"
)
covenant_run_code=$?
set -e

if [[ "$covenant_run_code" -eq 0 ]]; then
  echo "rsi-evidence-chain-smoke: Covenant unexpectedly allowed the full RSI claim" >&2
  exit 1
fi

(
  cd "$covenant"
  go run ./cmd/covenant policy explain \
    --json \
    --evidence "$covenant_work_dir/.covenant/runs/rsi-claim-boundary/evidence-pack.json" > "$out/covenant-policy-explain.json"
)

grep -q '"decision": "deny"' "$out/covenant-policy-explain.json"
grep -q '"effect_type": "claim.publish"' "$out/covenant-policy-explain.json"
grep -q '"resource": "full-autonomous-self-mutating-rsi"' "$out/covenant-policy-explain.json"
grep -q 'mutation authority' "$out/covenant-policy-explain.json"
grep -q 'rollback' "$out/covenant-policy-explain.json"
grep -q 'live self-change' "$out/covenant-policy-explain.json"

shasum -a 256 \
  "$out/foundry-pulse/rsi-candidate.json" \
  "$out/foundry-pulse/rsi-improvement-gate.json" \
  "$out/foundry-pulse/rsi-next-improvement-task.json" \
  "$out/ao-command-rsi-health.json" \
  "$out/rsi-health-bundle.json" \
  "$out/covenant-run.stderr.txt" \
  "$out/covenant-policy-explain.json" > "$out/checksums.txt"

cat > "$out/rsi-evidence-chain-smoke.json" <<JSON
{
  "schema_version": "ao.command.rsi-evidence-chain-smoke.v0.1",
  "status": "passed",
  "foundry": "$(json_escape "$foundry")",
  "forge": "$(json_escape "$forge")",
  "covenant": "$(json_escape "$covenant")",
  "chain": [
    "foundry pulse run",
    "ao-forge retained RSI proofs",
    "ao-command rsi health",
    "ao-covenant claim.publish denial for full-autonomous-self-mutating-rsi"
  ],
  "claim_boundary": "bounded governed RSI only; full autonomous self-mutating RSI remains denied without mutation authority, rollback, and live self-change evidence",
  "mutates_repositories": false,
  "artifacts": [
    "$(json_escape "$out/foundry-pulse/rsi-candidate.json")",
    "$(json_escape "$out/foundry-pulse/rsi-improvement-gate.json")",
    "$(json_escape "$out/foundry-pulse/rsi-next-improvement-task.json")",
    "$(json_escape "$out/ao-command-rsi-health.json")",
    "$(json_escape "$out/rsi-health-bundle.json")",
    "$(json_escape "$out/covenant-run.stderr.txt")",
    "$(json_escape "$out/covenant-policy-explain.json")",
    "$(json_escape "$out/checksums.txt")"
  ]
}
JSON

cd "$root"
echo "rsi_evidence_chain_smoke=passed"
echo "evidence=$out/rsi-evidence-chain-smoke.json"
