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
	publicReadinessAudit := read("scripts", "public-readiness-audit.sh")
	for _, check := range []struct {
		name string
		doc  string
		want string
	}{
		{name: "README private", doc: readme, want: "Private by default"},
		{name: "README no dangerous writes", doc: readme, want: "Dangerous writes are intentionally out of scope"},
		{name: "security private", doc: security, want: "private repository"},
		{name: "security no secrets", doc: security, want: "Do not commit secrets"},
		{name: "foundry no autonomous writes", doc: foundry, want: "intentionally avoids\nautonomous writes"},
		{name: "README publication checklist", doc: readme, want: "PUBLICATION-CHECKLIST.md"},
		{name: "security publication audit", doc: security, want: "scripts/public-readiness-audit.sh"},
		{name: "private guardrails local gate", doc: privateGuardrails, want: "scripts/public-readiness-audit.sh"},
		{name: "publication checklist operator approval", doc: publicationChecklist, want: "explicit operator approval"},
		{name: "publication checklist private before approval", doc: publicationChecklist, want: "visibility=PRIVATE"},
		{name: "public readiness audit repo private check", doc: publicReadinessAudit, want: "repository_private"},
		{name: "public readiness audit no artifacts", doc: publicReadinessAudit, want: "ci_artifact_uploads"},
		{name: "public readiness audit no dangerous writes", doc: publicReadinessAudit, want: "dangerous_write_surface"},
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
