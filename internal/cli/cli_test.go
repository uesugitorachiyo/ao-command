package cli

import (
	"bytes"
	"context"
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
		{name: "security public", doc: security, want: "public after passing the v0.1 publication audit"},
		{name: "security no secrets", doc: security, want: "Do not commit secrets"},
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
		{name: "production readiness schema version", doc: productionReadinessSchema, want: "ao.command.production-readiness-audit.v0.1"},
		{name: "production readiness schema strict", doc: productionReadinessSchema, want: "\"additionalProperties\": false"},
		{name: "release preview schema version", doc: releasePreviewSchema, want: "ao.command.release-preview-audit.v0.1"},
		{name: "release preview schema read-only", doc: releasePreviewSchema, want: "\"mutates_releases\""},
		{name: "release preview dry run read-only", doc: releasePreviewDryRun, want: "\"mutates_releases\": false"},
		{name: "install verify schema version", doc: installVerifySchema, want: "ao.command.install-verify-audit.v0.1"},
		{name: "install verify schema read-only", doc: installVerifySchema, want: "\"mutates_repositories\""},
		{name: "install verify dry run read-only", doc: installVerifyDryRun, want: "\"mutates_repositories\": false"},
		{name: "public readiness audit repo private check", doc: publicReadinessAudit, want: "repository_private"},
		{name: "public readiness audit no artifacts", doc: publicReadinessAudit, want: "ci_artifact_uploads"},
		{name: "public readiness audit no dangerous writes", doc: publicReadinessAudit, want: "dangerous_write_surface"},
		{name: "production readiness audit schema", doc: productionReadinessAudit, want: "ao.command.production-readiness-audit.v0.1"},
		{name: "production readiness audit contract check", doc: productionReadinessAudit, want: "readiness_contract"},
		{name: "production readiness audit release preview contract", doc: productionReadinessAudit, want: "release_preview_contract"},
		{name: "production readiness audit release preview dry-run", doc: productionReadinessAudit, want: "release_preview_dry_run"},
		{name: "production readiness audit install verify contract", doc: productionReadinessAudit, want: "install_verify_contract"},
		{name: "production readiness audit install verify dry-run", doc: productionReadinessAudit, want: "install_verify_dry_run"},
		{name: "production readiness audit retained evidence policy", doc: productionReadinessAudit, want: "retained_evidence_policy"},
		{name: "production readiness audit operator closeout", doc: productionReadinessAudit, want: "operator_closeout"},
		{name: "production readiness audit public repo", doc: productionReadinessAudit, want: "repository_public"},
		{name: "production readiness audit secret scanning", doc: productionReadinessAudit, want: "secret_scanning"},
		{name: "production readiness audit branch protection", doc: productionReadinessAudit, want: "branch_protection"},
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
	} {
		if !strings.Contains(check.doc, check.want) {
			t.Fatalf("%s missing %q", check.name, check.want)
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

func contains(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}
