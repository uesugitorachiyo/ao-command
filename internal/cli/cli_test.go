package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"
)

type fakeRunner struct {
	stdout []byte
	stderr []byte
	err    error
	calls  []Command
}

func (f *fakeRunner) Run(_ context.Context, cmd Command) ([]byte, []byte, error) {
	f.calls = append(f.calls, cmd)
	return f.stdout, f.stderr, f.err
}

func runWithFake(args []string, fake *fakeRunner) (int, string, string) {
	var stdout, stderr bytes.Buffer
	app := App{Runner: fake, Stdout: &stdout, Stderr: &stderr}
	code := app.Run(context.Background(), args)
	return code, stdout.String(), stderr.String()
}

func TestStatusReadsAOForgeProductionReadiness(t *testing.T) {
	fake := &fakeRunner{stdout: []byte(`{
		"status": "passed",
		"readiness_percent": 100,
		"passed_gates": 12,
		"total_gates": 12,
		"next_actions": []
	}`)}

	code, stdout, stderr := runWithFake([]string{"status", "--forge", "/repo/ao-forge"}, fake)
	if code != 0 {
		t.Fatalf("status exit=%d stderr=%s", code, stderr)
	}
	for _, want := range []string{
		"ao_command_status=passed",
		"forge=/repo/ao-forge",
		"readiness_percent=100",
		"gates=12/12",
		"next_actions=0",
		"required_next_actions=0",
		"production_ready=true",
		"operator_mode=read_only",
		"release_governance=blocked_pending_operator_approval",
	} {
		if !strings.Contains(stdout, want) {
			t.Fatalf("status stdout missing %q:\n%s", want, stdout)
		}
	}
	if len(fake.calls) != 1 {
		t.Fatalf("expected one call, got %d", len(fake.calls))
	}
	call := fake.calls[0]
	if call.Dir != "/repo/ao-forge" || call.Name != "go" {
		t.Fatalf("unexpected command: %+v", call)
	}
	wantArgs := []string{"run", "./cmd/forge", "production-readiness", "audit", "--json"}
	if !reflect.DeepEqual(call.Args, wantArgs) {
		t.Fatalf("args = %#v, want %#v", call.Args, wantArgs)
	}
}

func TestStatusJSONIncludesOperatorSignals(t *testing.T) {
	fake := &fakeRunner{stdout: []byte(`{
		"status": "blocked",
		"readiness_percent": 91,
		"passed_gates": 11,
		"total_gates": 12,
		"next_actions": [
			{"action_id":"refresh-evidence","description":"Refresh stale release evidence.","required":true},
			{"action_id":"inspect-ui","description":"Inspect operator UI polish.","required":false}
		]
	}`)}

	code, stdout, stderr := runWithFake([]string{"status", "--forge", "/repo/ao-forge", "--json"}, fake)
	if code != 0 {
		t.Fatalf("status exit=%d stderr=%s", code, stderr)
	}
	var got struct {
		CommandSchemaVersion string `json:"command_schema_version"`
		Forge                string `json:"forge"`
		Status               string `json:"status"`
		ReadinessPercent     int    `json:"readiness_percent"`
		RequiredNextActions  int    `json:"required_next_actions"`
		ProductionReady      bool   `json:"production_ready"`
		OperatorMode         string `json:"operator_mode"`
		ReleaseGovernance    string `json:"release_governance"`
	}
	if err := json.Unmarshal([]byte(stdout), &got); err != nil {
		t.Fatalf("invalid status JSON: %v\n%s", err, stdout)
	}
	if got.CommandSchemaVersion != "ao.command.v0.1" ||
		got.Forge != "/repo/ao-forge" ||
		got.Status != "blocked" ||
		got.ReadinessPercent != 91 ||
		got.RequiredNextActions != 1 ||
		got.ProductionReady ||
		got.OperatorMode != "read_only" ||
		got.ReleaseGovernance != "blocked_pending_operator_approval" {
		t.Fatalf("unexpected status summary: %+v", got)
	}
}

func TestStackReadsFoundryActiveStackLedger(t *testing.T) {
	ledger := writeStackLedgerFixture(t)
	code, stdout, stderr := runWithFake([]string{"stack", "--ledger", ledger}, &fakeRunner{})
	if code != 0 {
		t.Fatalf("stack exit=%d stderr=%s", code, stderr)
	}
	for _, want := range []string{
		"ao_command_stack=ready",
		"ledger=" + ledger,
		"active_repositories=6",
		"release_handoff=ready",
		"operator_mode=read_only",
		"orchestration_owner=ao-foundry",
		"gate=foundry-release-candidate status=ready required_before_promotion=true",
		"gate=forge-release-candidate-handoff status=ready required_before_promotion=true",
		"gate=covenant-policy-spine status=ready required_before_promotion=true",
		"out_of_scope=ao-operator,ao-runtime,ao-control-plane,ao-conductor,agy-swarms,codex-cron",
	} {
		if !strings.Contains(stdout, want) {
			t.Fatalf("stack stdout missing %q:\n%s", want, stdout)
		}
	}
	for _, excluded := range []string{"orchestration_started", "release_published", "branch_mutated"} {
		if strings.Contains(stdout, excluded) {
			t.Fatalf("stack output contains mutation signal %q:\n%s", excluded, stdout)
		}
	}
}

func TestStackJSONReportsReadOnlyActiveStack(t *testing.T) {
	ledger := writeStackLedgerFixture(t)
	code, stdout, stderr := runWithFake([]string{"stack", "--ledger", ledger, "--json"}, &fakeRunner{})
	if code != 0 {
		t.Fatalf("stack exit=%d stderr=%s", code, stderr)
	}
	var got struct {
		CommandSchemaVersion string `json:"command_schema_version"`
		Ledger               string `json:"ledger"`
		Status               string `json:"status"`
		OperatorMode         string `json:"operator_mode"`
		OrchestrationOwner   string `json:"orchestration_owner"`
		ReleaseHandoff       struct {
			Status string `json:"status"`
			Gates  []struct {
				Name                    string `json:"name"`
				Status                  string `json:"status"`
				RequiredBeforePromotion bool   `json:"required_before_promotion"`
			} `json:"gates"`
		} `json:"release_handoff"`
		ActiveRepositories []struct {
			ID     string `json:"id"`
			Status string `json:"status"`
		} `json:"active_repositories"`
		OutOfScope []string `json:"out_of_scope"`
	}
	if err := json.Unmarshal([]byte(stdout), &got); err != nil {
		t.Fatalf("invalid stack JSON: %v\n%s", err, stdout)
	}
	if got.CommandSchemaVersion != "ao.command.v0.1" ||
		got.Ledger != ledger ||
		got.Status != "ready" ||
		got.OperatorMode != "read_only" ||
		got.OrchestrationOwner != "ao-foundry" ||
		got.ReleaseHandoff.Status != "ready" ||
		len(got.ReleaseHandoff.Gates) != 3 ||
		len(got.ActiveRepositories) != 6 {
		t.Fatalf("unexpected stack summary: %+v", got)
	}
	for _, want := range []string{"ao2", "ao2-control-plane", "ao-foundry", "ao-forge", "ao-command", "ao-covenant"} {
		if !stackRepoPresent(got.ActiveRepositories, want) {
			t.Fatalf("stack JSON missing active repo %q: %+v", want, got.ActiveRepositories)
		}
	}
	for _, want := range []string{"ao-operator", "ao-runtime", "ao-control-plane", "ao-conductor", "agy-swarms", "codex-cron"} {
		if !contains(got.OutOfScope, want) {
			t.Fatalf("stack JSON missing out-of-scope repo %q: %+v", want, got.OutOfScope)
		}
	}
}

func TestNextUsesAOForgeNextActionsWhenPresent(t *testing.T) {
	fake := &fakeRunner{stdout: []byte(`{
		"status": "blocked",
		"readiness_percent": 91,
		"passed_gates": 11,
		"total_gates": 12,
		"next_actions": [
			{"action_id":"fix-release-preview","description":"Refresh release preview evidence.","required":true}
		]
	}`)}

	code, stdout, stderr := runWithFake([]string{"next"}, fake)
	if code != 0 {
		t.Fatalf("next exit=%d stderr=%s", code, stderr)
	}
	for _, want := range []string{
		"ao_command_next=blocked",
		"readiness_percent=91",
		"next_action=fix-release-preview required=true",
		"summary=Refresh release preview evidence.",
	} {
		if !strings.Contains(stdout, want) {
			t.Fatalf("next stdout missing %q:\n%s", want, stdout)
		}
	}
}

func TestGoalsInspectsGoalRunThroughAOForge(t *testing.T) {
	fake := &fakeRunner{stdout: []byte(`{
		"goal_run": "examples/goals/ao2-weekend-hardening.goal-run.json",
		"goal_id": "ao2-weekend-hardening",
		"repo": "ao2",
		"current_phase": "implementation",
		"next_task": "Harden the next slice.",
		"last_iteration_status": "passed",
		"next_action_guard": {"enabled": true, "on_mismatch": "backoff_or_stop"}
	}`)}

	code, stdout, stderr := runWithFake([]string{"goals", "--goal-run", "goal.json", "--forge-bin", "forge"}, fake)
	if code != 0 {
		t.Fatalf("goals exit=%d stderr=%s", code, stderr)
	}
	for _, want := range []string{
		"goal_id=ao2-weekend-hardening",
		"repo=ao2",
		"current_phase=implementation",
		"next_task=Harden the next slice.",
		"next_action_guard=true",
		"last_iteration_status=passed",
	} {
		if !strings.Contains(stdout, want) {
			t.Fatalf("goals stdout missing %q:\n%s", want, stdout)
		}
	}
	call := fake.calls[0]
	if call.Name != "forge" {
		t.Fatalf("expected forge binary, got %+v", call)
	}
	wantArgs := []string{"goal", "inspect", "--goal-run", "goal.json", "--json"}
	if !reflect.DeepEqual(call.Args, wantArgs) {
		t.Fatalf("args = %#v, want %#v", call.Args, wantArgs)
	}
}

func TestEvidenceRequiresSchemaAndDocument(t *testing.T) {
	code, _, stderr := runWithFake([]string{"evidence", "--schema", "schema.json"}, &fakeRunner{})
	if code != 2 {
		t.Fatalf("evidence exit=%d, want 2", code)
	}
	if !strings.Contains(stderr, "--schema and --document are required") {
		t.Fatalf("stderr missing required flag message: %s", stderr)
	}
}

func TestRehearseRunsDryRunAndInspect(t *testing.T) {
	fake := &fakeRunner{stdout: []byte(`{"status":"passed"}`)}

	code, stdout, stderr := runWithFake([]string{"rehearse", "--tag", "v0.1.3", "--out", "/tmp/rehearse", "--forge", "/repo/ao-forge"}, fake)
	if code != 0 {
		t.Fatalf("rehearse exit=%d stderr=%s", code, stderr)
	}
	if len(fake.calls) != 2 {
		t.Fatalf("expected two calls, got %d", len(fake.calls))
	}
	if fake.calls[0].Name != "scripts/release-preview-dry-run.sh" || fake.calls[0].Dir != "/repo/ao-forge" {
		t.Fatalf("unexpected rehearsal command: %+v", fake.calls[0])
	}
	for _, want := range []string{
		"AO_FORGE_RELEASE_PREVIEW_TAG=v0.1.3",
		"AO_FORGE_RELEASE_PREVIEW_OUT=/tmp/rehearse",
		"AO_FORGE_RELEASE_NOTES_PATH=docs/release/V0.1.3-RELEASE-NOTES.md",
	} {
		if !contains(fake.calls[0].Env, want) {
			t.Fatalf("rehearsal env missing %q: %#v", want, fake.calls[0].Env)
		}
	}
	if !reflect.DeepEqual(fake.calls[1].Args, []string{"run", "./cmd/forge", "release-preview", "inspect", "--audit", "/tmp/rehearse/release-preview-audit.json", "--json"}) {
		t.Fatalf("unexpected inspect args: %#v", fake.calls[1].Args)
	}
	if !strings.Contains(stdout, "ao_command_rehearse=passed") {
		t.Fatalf("rehearse stdout missing status:\n%s", stdout)
	}
}

func TestCommandFailureReportsStderr(t *testing.T) {
	fake := &fakeRunner{stderr: []byte("forge failed"), err: errors.New("exit 1")}
	code, _, stderr := runWithFake([]string{"status"}, fake)
	if code != 1 {
		t.Fatalf("status exit=%d, want 1", code)
	}
	if !strings.Contains(stderr, "forge failed") {
		t.Fatalf("stderr missing command stderr: %s", stderr)
	}
}

func TestDocsDeclarePrivateReadOnlyBoundary(t *testing.T) {
	root := repoRoot(t)
	read := func(path ...string) string {
		t.Helper()
		content, err := os.ReadFile(filepath.Join(append([]string{root}, path...)...))
		if err != nil {
			t.Fatalf("read %s: %v", filepath.Join(path...), err)
		}
		return string(content)
	}

	readme := read("README.md")
	security := read("SECURITY.md")
	foundry := read("docs", "design", "AO-COMMAND-FOUNDRY.md")
	privateGuardrails := read("docs", "operations", "PRIVATE-REPO-GUARDRAILS.md")
	publicationChecklist := read("docs", "operations", "PUBLICATION-CHECKLIST.md")
	publicationRecord := read("docs", "operations", "PUBLICATION-RECORD-2026-06-19.md")
	productionReadiness := read("docs", "operations", "PRODUCTION-READINESS.md")
	operatorCloseout := read("docs", "release", "V0.1.0-OPERATOR-CLOSEOUT.md")
	retainedEvidence := read("docs", "operations", "RETAINED-EVIDENCE.md")
	publicProvenanceManifest := read("docs", "operations", "public-provenance-manifest.json")
	productionReadinessSchema := read("docs", "contracts", "production-readiness-audit-v0.1.schema.json")
	releasePreviewSchema := read("docs", "contracts", "release-preview-audit-v0.1.schema.json")
	releasePreviewDryRun := read("scripts", "release-preview-dry-run.sh")
	installVerifySchema := read("docs", "contracts", "install-verify-audit-v0.1.schema.json")
	installVerifyDryRun := read("scripts", "install-verify-dry-run.sh")
	releaseGovernanceSchema := read("docs", "contracts", "release-governance-audit-v0.1.schema.json")
	releaseGovernanceDryRun := read("scripts", "release-governance-dry-run.sh")
	publicReadinessAudit := read("scripts", "public-readiness-audit.sh")
	productionReadinessAudit := read("scripts", "production-readiness-audit.sh")
	workflow := read(".github", "workflows", "ci.yml")
	for _, check := range []struct {
		name string
		doc  string
		want string
	}{
		{name: "README publication audit", doc: readme, want: "operator-approved public-readiness audit passed"},
		{name: "README no dangerous writes", doc: readme, want: "Dangerous writes are intentionally out of scope"},
		{name: "README AO2 execution boundary", doc: readme, want: "AO2 is the governed execution path"},
		{name: "README active stack command", doc: readme, want: "go run ./cmd/ao-command stack --ledger ../ao-foundry/examples/readiness/active-stack-readiness.ledger.json"},
		{name: "README Foundry owner", doc: readme, want: "orchestration_owner=ao-foundry"},
		{name: "README deprecated repos out of scope", doc: readme, want: "Deprecated standalone runtime"},
		{name: "security public", doc: security, want: "public after passing the v0.1 publication audit"},
		{name: "security no secrets", doc: security, want: "Do not commit secrets"},
		{name: "foundry legacy note", doc: foundry, want: "legacy product note"},
		{name: "foundry owned by ao-foundry", doc: foundry, want: "AO Foundry owns\nthe active-stack operations ledger"},
		{name: "foundry no autonomous writes", doc: foundry, want: "intentionally avoids\nautonomous writes"},
		{name: "README publication checklist", doc: readme, want: "PUBLICATION-CHECKLIST.md"},
		{name: "security publication audit", doc: security, want: "scripts/public-readiness-audit.sh"},
		{name: "private guardrails local gate", doc: privateGuardrails, want: "scripts/public-readiness-audit.sh"},
		{name: "publication checklist operator approval", doc: publicationChecklist, want: "explicitly approved publication"},
		{name: "publication checklist private before approval", doc: publicationChecklist, want: "visibility=PRIVATE"},
		{name: "publication record public", doc: publicationRecord, want: "visibility=PUBLIC"},
		{name: "publication record no leaks", doc: publicationRecord, want: "reported no\n  leaks"},
		{name: "production readiness docs title", doc: productionReadiness, want: "# Production Readiness"},
		{name: "production readiness docs command", doc: productionReadiness, want: "scripts/production-readiness-audit.sh"},
		{name: "production readiness docs contract", doc: productionReadiness, want: "production-readiness-audit-v0.1.schema.json"},
		{name: "production readiness docs release preview", doc: productionReadiness, want: "release-preview-audit-v0.1.schema.json"},
		{name: "production readiness docs install verify", doc: productionReadiness, want: "install-verify-audit-v0.1.schema.json"},
		{name: "production readiness docs release governance", doc: productionReadiness, want: "release-governance-audit-v0.1.schema.json"},
		{name: "production readiness docs active stack command", doc: productionReadiness, want: "ao-command stack --ledger"},
		{name: "production readiness docs active stack gate", doc: productionReadiness, want: "active-stack handoff"},
		{name: "production readiness docs retained evidence", doc: productionReadiness, want: "public-provenance-manifest.json"},
		{name: "production readiness docs operator closeout", doc: productionReadiness, want: "V0.1.0-OPERATOR-CLOSEOUT.md"},
		{name: "operator closeout title", doc: operatorCloseout, want: "AO Command v0.1.0 Operator Closeout"},
		{name: "operator closeout read-only", doc: operatorCloseout, want: "read-only operator command surface"},
		{name: "operator closeout required evidence", doc: operatorCloseout, want: "readiness_percent=100"},
		{name: "operator closeout remaining actions", doc: operatorCloseout, want: "Rerun the full admin-mode readiness audit"},
		{name: "retained evidence no uploads", doc: retainedEvidence, want: "Do not upload CI artifacts by default"},
		{name: "retained evidence no secrets", doc: retainedEvidence, want: "Do not retain"},
		{name: "provenance manifest schema", doc: publicProvenanceManifest, want: "ao.command.public-provenance-manifest.v0.1"},
		{name: "provenance manifest release preview", doc: publicProvenanceManifest, want: "release-preview-dry-run"},
		{name: "provenance manifest install verify", doc: publicProvenanceManifest, want: "install-verify-dry-run"},
		{name: "provenance manifest release governance", doc: publicProvenanceManifest, want: "release-governance-dry-run"},
		{name: "production readiness schema version", doc: productionReadinessSchema, want: "ao.command.production-readiness-audit.v0.1"},
		{name: "production readiness schema strict", doc: productionReadinessSchema, want: "\"additionalProperties\": false"},
		{name: "release preview schema version", doc: releasePreviewSchema, want: "ao.command.release-preview-audit.v0.1"},
		{name: "release preview schema read-only", doc: releasePreviewSchema, want: "\"mutates_releases\""},
		{name: "release preview dry run read-only", doc: releasePreviewDryRun, want: "\"mutates_releases\": false"},
		{name: "install verify schema version", doc: installVerifySchema, want: "ao.command.install-verify-audit.v0.1"},
		{name: "install verify schema read-only", doc: installVerifySchema, want: "\"mutates_repositories\""},
		{name: "install verify dry run read-only", doc: installVerifyDryRun, want: "\"mutates_repositories\": false"},
		{name: "release governance schema version", doc: releaseGovernanceSchema, want: "ao.command.release-governance-audit.v0.1"},
		{name: "release governance schema blocked", doc: releaseGovernanceSchema, want: "blocked_pending_operator_approval"},
		{name: "release governance dry run blocked", doc: releaseGovernanceDryRun, want: "blocked_pending_operator_approval"},
		{name: "release governance dry run no release create", doc: releaseGovernanceDryRun, want: "\"would_create_release\": false"},
		{name: "public readiness audit repo private check", doc: publicReadinessAudit, want: "repository_private"},
		{name: "public readiness audit no artifacts", doc: publicReadinessAudit, want: "ci_artifact_uploads"},
		{name: "public readiness audit no dangerous writes", doc: publicReadinessAudit, want: "dangerous_write_surface"},
		{name: "production readiness audit schema", doc: productionReadinessAudit, want: "ao.command.production-readiness-audit.v0.1"},
		{name: "production readiness audit contract check", doc: productionReadinessAudit, want: "readiness_contract"},
		{name: "production readiness audit release preview contract", doc: productionReadinessAudit, want: "release_preview_contract"},
		{name: "production readiness audit release preview dry-run", doc: productionReadinessAudit, want: "release_preview_dry_run"},
		{name: "production readiness audit install verify contract", doc: productionReadinessAudit, want: "install_verify_contract"},
		{name: "production readiness audit install verify dry-run", doc: productionReadinessAudit, want: "install_verify_dry_run"},
		{name: "production readiness audit release governance contract", doc: productionReadinessAudit, want: "release_governance_contract"},
		{name: "production readiness audit release governance dry-run", doc: productionReadinessAudit, want: "release_governance_dry_run"},
		{name: "production readiness audit active stack status", doc: productionReadinessAudit, want: "active_stack_status"},
		{name: "production readiness audit retained evidence policy", doc: productionReadinessAudit, want: "retained_evidence_policy"},
		{name: "production readiness audit operator closeout", doc: productionReadinessAudit, want: "operator_closeout"},
		{name: "production readiness audit public repo", doc: productionReadinessAudit, want: "repository_public"},
		{name: "production readiness audit secret scanning", doc: productionReadinessAudit, want: "secret_scanning"},
		{name: "production readiness audit branch protection", doc: productionReadinessAudit, want: "branch_protection"},
		{name: "production readiness audit branch protection requires license policy", doc: productionReadinessAudit, want: "\"context\":\"License policy\""},
		{name: "production readiness audit branch protection requires readiness", doc: productionReadinessAudit, want: "\"context\":\"Production readiness audit\""},
		{name: "production readiness audit skip admin mode", doc: productionReadinessAudit, want: "skip_remote_admin"},
		{name: "production readiness audit no dangerous writes", doc: productionReadinessAudit, want: "dangerous_write_surface"},
		{name: "workflow production readiness job", doc: workflow, want: "name: Production readiness audit"},
		{name: "workflow production readiness script", doc: workflow, want: "scripts/production-readiness-audit.sh"},
		{name: "workflow production readiness schema", doc: workflow, want: "Validate production readiness contract"},
		{name: "workflow release preview dry-run", doc: workflow, want: "Release preview dry-run"},
		{name: "workflow release preview schema", doc: workflow, want: "Validate release preview contract"},
		{name: "workflow install verify dry-run", doc: workflow, want: "Install verification dry-run"},
		{name: "workflow install verify schema", doc: workflow, want: "Validate install verification contract"},
		{name: "workflow release governance dry-run", doc: workflow, want: "Release governance dry-run"},
		{name: "workflow release governance schema", doc: workflow, want: "Validate release governance contract"},
		{name: "workflow active stack checkout", doc: workflow, want: "Checkout ao-foundry active-stack fixture"},
		{name: "workflow active stack status", doc: workflow, want: "Active stack status"},
	} {
		if !strings.Contains(check.doc, check.want) {
			t.Fatalf("%s missing %q", check.name, check.want)
		}
	}
}

func TestDryRunCleanTreeAllowlistIncludesFoundryFixture(t *testing.T) {
	root := repoRoot(t)
	for _, script := range []string{
		"scripts/release-preview-dry-run.sh",
		"scripts/install-verify-dry-run.sh",
		"scripts/release-governance-dry-run.sh",
		"scripts/production-readiness-audit.sh",
	} {
		content, err := os.ReadFile(filepath.Join(root, script))
		if err != nil {
			t.Fatalf("read %s: %v", script, err)
		}
		if !strings.Contains(string(content), "':!ao-foundry'") {
			t.Fatalf("%s clean-tree allowlist must include the read-only ao-foundry fixture checkout", script)
		}
	}
}

func TestWorkflowUsesCurrentNodeRuntimeActions(t *testing.T) {
	content, err := os.ReadFile(filepath.Join(repoRoot(t), ".github", "workflows", "ci.yml"))
	if err != nil {
		t.Fatalf("read workflow: %v", err)
	}
	workflow := string(content)
	for _, deprecated := range []string{
		"actions/checkout@v4",
		"actions/setup-go@v5",
	} {
		if strings.Contains(workflow, deprecated) {
			t.Fatalf("workflow must not use deprecated Node 20 action %q", deprecated)
		}
	}
	for _, current := range []string{
		"actions/checkout@v7",
		"actions/setup-go@v6",
	} {
		if !strings.Contains(workflow, current) {
			t.Fatalf("workflow must use current action %q", current)
		}
	}
}

func TestProductionReadinessOpsWorkflowRunsBranchProtectionVerifier(t *testing.T) {
	root := repoRoot(t)
	read := func(path ...string) string {
		t.Helper()
		content, err := os.ReadFile(filepath.Join(append([]string{root}, path...)...))
		if err != nil {
			t.Fatalf("read %s: %v", filepath.Join(path...), err)
		}
		return string(content)
	}

	workflow := read(".github", "workflows", "production-readiness-ops.yml")
	verifier := read("scripts", "verify-branch-protection.sh")
	runbook := read("docs", "operations", "BRANCH-PROTECTION.md")

	for _, check := range []struct {
		name string
		doc  string
		want string
	}{
		{name: "workflow name", doc: workflow, want: "name: Production Readiness Ops"},
		{name: "manual dispatch", doc: workflow, want: "workflow_dispatch:"},
		{name: "daily schedule", doc: workflow, want: `cron: "31 10 * * *"`},
		{name: "read-only permissions", doc: workflow, want: "contents: read"},
		{name: "token wiring", doc: workflow, want: "GH_TOKEN: ${{ github.token }}"},
		{name: "limited token mode", doc: workflow, want: "AO_COMMAND_BRANCH_PROTECTION_MODE: limited"},
		{name: "verifier step", doc: workflow, want: "scripts/verify-branch-protection.sh"},
		{name: "verifier full mode", doc: verifier, want: `mode="${AO_COMMAND_BRANCH_PROTECTION_MODE:-full}"`},
		{name: "verifier limited branch endpoint", doc: verifier, want: `repos/${repository}/branches/${branch}`},
		{name: "runbook command", doc: runbook, want: "scripts/verify-branch-protection.sh"},
		{name: "runbook limited mode", doc: runbook, want: "AO_COMMAND_BRANCH_PROTECTION_MODE=limited"},
	} {
		if !strings.Contains(check.doc, check.want) {
			t.Fatalf("%s missing %q", check.name, check.want)
		}
	}

	for _, forbidden := range []string{
		"contents: write",
		"pull-requests: write",
		"id-token: write",
	} {
		if strings.Contains(workflow, forbidden) {
			t.Fatalf("production readiness ops workflow must stay read-only, found %q", forbidden)
		}
	}
}

func repoRoot(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime caller unavailable")
	}
	return filepath.Clean(filepath.Join(filepath.Dir(file), "..", ".."))
}

func writeStackLedgerFixture(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "active-stack-readiness.ledger.json")
	ledger := `{
  "schema_version": "ao.foundry.active-stack-readiness.v0.1",
  "registry_id": "local-ao-stack",
  "generated_from_registry": "examples/registry/local-ao-stack.foundry-registry.json",
  "last_sweep_date": "2026-06-23",
  "status": "ready",
  "repositories": [
    {"id": "ao-foundry", "name": "AO Foundry", "role": "operations-factory", "status": "ready", "verification_evidence": ["go test ./..."]},
    {"id": "ao-forge", "name": "AO Forge", "role": "factory-brain", "status": "ready", "verification_evidence": ["release candidate handoff"]},
    {"id": "ao-command", "name": "AO Command", "role": "operator-command", "status": "ready", "verification_evidence": ["read-only status"]},
    {"id": "ao2", "name": "AO2", "role": "execution-engine", "status": "ready", "verification_evidence": ["npm run verify"]},
    {"id": "ao2-control-plane", "name": "AO2 Control Plane", "role": "evidence-observer", "status": "ready", "verification_evidence": ["cargo test --workspace"]},
    {"id": "ao-covenant", "name": "AO Covenant", "role": "policy-kernel", "status": "ready", "verification_evidence": ["covenant policy spine --json"]}
  ],
  "release_handoff": {
    "status": "ready",
    "gates": [
      {"name": "foundry-release-candidate", "status": "ready", "required_before_promotion": true, "evidence": ["foundry candidate validation"]},
      {"name": "forge-release-candidate-handoff", "status": "ready", "required_before_promotion": true, "evidence": ["forge release-candidate validate"]},
      {"name": "covenant-policy-spine", "status": "ready", "required_before_promotion": true, "evidence": ["covenant.policy-spine-result.v1"]}
    ]
  },
  "next_actions": ["Keep release handoff gates attached to the active-stack readiness ledger"]
}`
	if err := os.WriteFile(path, []byte(ledger), 0o644); err != nil {
		t.Fatalf("write stack ledger fixture: %v", err)
	}
	return path
}

func stackRepoPresent(repos []struct {
	ID     string `json:"id"`
	Status string `json:"status"`
}, id string) bool {
	for _, repo := range repos {
		if repo.ID == id && repo.Status == "ready" {
			return true
		}
	}
	return false
}

func contains(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}
