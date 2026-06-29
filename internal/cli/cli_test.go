package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
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

func TestAtlasStatusReadsFoundryObserverArtifact(t *testing.T) {
	status := writeAtlasStatusFixture(t, false)
	code, stdout, stderr := runWithFake([]string{"atlas", "status", "--status", status}, &fakeRunner{})
	if code != 0 {
		t.Fatalf("atlas status exit=%d stderr=%s", code, stderr)
	}
	for _, want := range []string{
		"ao_command_atlas_status=ready",
		"foundry_status=" + status,
		"mode=fixture_only_readback",
		"registry_id=atlas-demo-stack",
		"workgraph_id=atlas-readiness-workgraph",
		"target_instance=demo-stack",
		"task_id=atlas-readiness-task",
		"operator_mode=read_only",
		"orchestration_owner=ao-foundry",
		"atlas_authority=compile_only",
		"schedules_work=false",
		"executes_work=false",
		"approves_work=false",
		"mutates_repositories=false",
	} {
		if !strings.Contains(stdout, want) {
			t.Fatalf("atlas status stdout missing %q:\n%s", want, stdout)
		}
	}
}

func TestAtlasStatusJSONReportsReadOnlyBoundaries(t *testing.T) {
	status := writeAtlasStatusFixture(t, false)
	code, stdout, stderr := runWithFake([]string{"atlas", "status", "--status", status, "--json"}, &fakeRunner{})
	if code != 0 {
		t.Fatalf("atlas status exit=%d stderr=%s", code, stderr)
	}
	var got struct {
		SchemaVersion        string `json:"schema_version"`
		CommandSchemaVersion string `json:"command_schema_version"`
		Status               string `json:"status"`
		FoundryStatus        string `json:"foundry_status"`
		Mode                 string `json:"mode"`
		RegistryID           string `json:"registry_id"`
		WorkgraphID          string `json:"workgraph_id"`
		TargetInstance       string `json:"target_instance"`
		TaskID               string `json:"task_id"`
		OperatorMode         string `json:"operator_mode"`
		OrchestrationOwner   string `json:"orchestration_owner"`
		AtlasAuthority       string `json:"atlas_authority"`
		SchedulesWork        bool   `json:"schedules_work"`
		ExecutesWork         bool   `json:"executes_work"`
		ApprovesWork         bool   `json:"approves_work"`
		MutatesRepositories  bool   `json:"mutates_repositories"`
	}
	if err := json.Unmarshal([]byte(stdout), &got); err != nil {
		t.Fatalf("invalid atlas status JSON: %v\n%s", err, stdout)
	}
	if got.SchemaVersion != "ao.command.atlas-status.v0.1" ||
		got.CommandSchemaVersion != "ao.command.v0.1" ||
		got.Status != "ready" ||
		got.FoundryStatus != status ||
		got.Mode != "fixture_only_readback" ||
		got.RegistryID != "atlas-demo-stack" ||
		got.WorkgraphID != "atlas-readiness-workgraph" ||
		got.TargetInstance != "demo-stack" ||
		got.TaskID != "atlas-readiness-task" ||
		got.OperatorMode != "read_only" ||
		got.OrchestrationOwner != "ao-foundry" ||
		got.AtlasAuthority != "compile_only" ||
		got.SchedulesWork ||
		got.ExecutesWork ||
		got.ApprovesWork ||
		got.MutatesRepositories {
		t.Fatalf("unexpected atlas status summary: %+v", got)
	}
}

func TestAtlasStatusRejectsAuthorityDrift(t *testing.T) {
	status := writeAtlasStatusFixture(t, true)
	code, _, stderr := runWithFake([]string{"atlas", "status", "--status", status}, &fakeRunner{})
	if code != 1 {
		t.Fatalf("atlas status exit=%d, want 1", code)
	}
	if !strings.Contains(stderr, "must remain observer-only") {
		t.Fatalf("stderr missing observer-only boundary reason: %s", stderr)
	}
}

func TestRSIHealthReportsNewAssuranceFamilies(t *testing.T) {
	paths := writeRSIHealthFixtures(t, true)
	code, stdout, stderr := runWithFake([]string{
		"rsi", "health",
		"--arena-gate", paths.arena,
		"--crucible-gate", paths.crucible,
		"--sentinel-verdict", paths.sentinel,
		"--promoter-gate", paths.promoter,
		"--foundry-gate", paths.foundry,
		"--foundry-candidate", paths.foundryCandidate,
		"--foundry-next-task", paths.foundryNextTask,
		"--forge-retained-gate", paths.forgeRetainedGate,
		"--forge-retained-candidate", paths.forgeRetainedCandidate,
		"--forge-retained-next-task", paths.forgeRetainedNextTask,
		"--forge-retained-command-health", paths.forgeRetainedCommandHealth,
	}, &fakeRunner{})
	if code != 0 {
		t.Fatalf("rsi health exit=%d stderr=%s", code, stderr)
	}
	for _, want := range []string{
		"ao_command_rsi_health=passed",
		"rsi_mode=governed_fixture_local",
		"operator_mode=read_only",
		"family=ao-arena status=passed",
		"family=ao-crucible status=passed",
		"family=ao-sentinel status=clear",
		"family=ao-promoter status=passed",
		"family=ao-foundry status=passed",
		"rsi_capability=demonstrated_local_fixture_loop",
		"claim_level=bounded_governed_rsi decision=allowed status=passed",
		"claim_level=full_autonomous_self_mutating_rsi decision=denied status=blocked",
	} {
		if !strings.Contains(stdout, want) {
			t.Fatalf("rsi health stdout missing %q:\n%s", want, stdout)
		}
	}
}

func TestRSIHealthJSONIncludesEvidencePathsAndNoMutation(t *testing.T) {
	paths := writeRSIHealthFixtures(t, true)
	code, stdout, stderr := runWithFake([]string{
		"rsi", "health",
		"--arena-gate", paths.arena,
		"--crucible-gate", paths.crucible,
		"--sentinel-verdict", paths.sentinel,
		"--promoter-gate", paths.promoter,
		"--foundry-gate", paths.foundry,
		"--foundry-candidate", paths.foundryCandidate,
		"--foundry-next-task", paths.foundryNextTask,
		"--forge-retained-gate", paths.forgeRetainedGate,
		"--forge-retained-candidate", paths.forgeRetainedCandidate,
		"--forge-retained-next-task", paths.forgeRetainedNextTask,
		"--forge-retained-command-health", paths.forgeRetainedCommandHealth,
		"--json",
	}, &fakeRunner{})
	if code != 0 {
		t.Fatalf("rsi health json exit=%d stderr=%s", code, stderr)
	}
	var got struct {
		SchemaVersion        string `json:"schema_version"`
		CommandSchemaVersion string `json:"command_schema_version"`
		Status               string `json:"status"`
		RSIMode              string `json:"rsi_mode"`
		RSICapability        string `json:"rsi_capability"`
		OperatorMode         string `json:"operator_mode"`
		MutatesRepositories  bool   `json:"mutates_repositories"`
		ClaimLevels          []struct {
			Claim    string `json:"claim"`
			Decision string `json:"decision"`
			Status   string `json:"status"`
			Reason   string `json:"reason"`
		} `json:"claim_levels"`
		Families []struct {
			Family   string `json:"family"`
			Status   string `json:"status"`
			Passed   bool   `json:"passed"`
			Evidence string `json:"evidence"`
		} `json:"families"`
	}
	if err := json.Unmarshal([]byte(stdout), &got); err != nil {
		t.Fatalf("invalid rsi health JSON: %v\n%s", err, stdout)
	}
	if got.SchemaVersion != "ao.command.rsi-health.v0.1" ||
		got.CommandSchemaVersion != "ao.command.v0.1" ||
		got.Status != "passed" ||
		got.RSIMode != "governed_fixture_local" ||
		got.RSICapability != "demonstrated_local_fixture_loop" ||
		got.OperatorMode != "read_only" ||
		got.MutatesRepositories ||
		len(got.ClaimLevels) != 2 ||
		len(got.Families) != 5 {
		t.Fatalf("unexpected rsi health summary: %+v", got)
	}
	if got.ClaimLevels[0].Claim != "bounded_governed_rsi" ||
		got.ClaimLevels[0].Decision != "allowed" ||
		got.ClaimLevels[0].Status != "passed" ||
		!strings.Contains(got.ClaimLevels[0].Reason, "5 percent") {
		t.Fatalf("unexpected bounded RSI claim level: %+v", got.ClaimLevels)
	}
	if got.ClaimLevels[1].Claim != "full_autonomous_self_mutating_rsi" ||
		got.ClaimLevels[1].Decision != "denied" ||
		got.ClaimLevels[1].Status != "blocked" ||
		!strings.Contains(got.ClaimLevels[1].Reason, "mutation authority") ||
		!strings.Contains(got.ClaimLevels[1].Reason, "rollback") ||
		!strings.Contains(got.ClaimLevels[1].Reason, "live self-change") {
		t.Fatalf("unexpected full RSI claim level: %+v", got.ClaimLevels)
	}
	for _, family := range got.Families {
		if !family.Passed || family.Evidence == "" {
			t.Fatalf("family missing pass/evidence: %+v", family)
		}
	}
}

func TestRSIHealthBindsFoundryCandidateToImprovementGate(t *testing.T) {
	paths := writeRSIHealthFixtures(t, true)
	code, stdout, stderr := runWithFake([]string{
		"rsi", "health",
		"--arena-gate", paths.arena,
		"--crucible-gate", paths.crucible,
		"--sentinel-verdict", paths.sentinel,
		"--promoter-gate", paths.promoter,
		"--foundry-gate", paths.foundry,
		"--foundry-candidate", paths.foundryCandidate,
		"--foundry-next-task", paths.foundryNextTask,
		"--forge-retained-gate", paths.forgeRetainedGate,
		"--forge-retained-candidate", paths.forgeRetainedCandidate,
		"--forge-retained-next-task", paths.forgeRetainedNextTask,
		"--forge-retained-command-health", paths.forgeRetainedCommandHealth,
		"--json",
	}, &fakeRunner{})
	if code != 0 {
		t.Fatalf("rsi health candidate binding exit=%d stdout=%s stderr=%s", code, stdout, stderr)
	}
	var got struct {
		Status                  string `json:"status"`
		RSICapability           string `json:"rsi_capability"`
		FoundryCandidateBinding struct {
			Status                string `json:"status"`
			Passed                bool   `json:"passed"`
			CandidateEvidence     string `json:"candidate_evidence"`
			GateEvidence          string `json:"gate_evidence"`
			MatchedEvalResultPath string `json:"matched_eval_result_path"`
			MutatesRepositories   bool   `json:"mutates_repositories"`
		} `json:"foundry_candidate_binding"`
	}
	if err := json.Unmarshal([]byte(stdout), &got); err != nil {
		t.Fatalf("invalid rsi health candidate binding JSON: %v\n%s", err, stdout)
	}
	if got.Status != "passed" ||
		got.RSICapability != "demonstrated_local_fixture_loop" ||
		got.FoundryCandidateBinding.Status != "passed" ||
		!got.FoundryCandidateBinding.Passed ||
		got.FoundryCandidateBinding.CandidateEvidence != paths.foundryCandidate ||
		got.FoundryCandidateBinding.GateEvidence != paths.foundry ||
		got.FoundryCandidateBinding.MatchedEvalResultPath != "tmp/pulse-rsi-verify/eval-result.json" ||
		got.FoundryCandidateBinding.MutatesRepositories {
		t.Fatalf("unexpected Foundry candidate binding: %+v", got)
	}
}

func TestRSIHealthBindsFoundryNextTaskToCandidateAndGate(t *testing.T) {
	paths := writeRSIHealthFixtures(t, true)
	code, stdout, stderr := runWithFake([]string{
		"rsi", "health",
		"--arena-gate", paths.arena,
		"--crucible-gate", paths.crucible,
		"--sentinel-verdict", paths.sentinel,
		"--promoter-gate", paths.promoter,
		"--foundry-gate", paths.foundry,
		"--foundry-candidate", paths.foundryCandidate,
		"--foundry-next-task", paths.foundryNextTask,
		"--forge-retained-gate", paths.forgeRetainedGate,
		"--forge-retained-candidate", paths.forgeRetainedCandidate,
		"--forge-retained-next-task", paths.forgeRetainedNextTask,
		"--forge-retained-command-health", paths.forgeRetainedCommandHealth,
		"--json",
	}, &fakeRunner{})
	if code != 0 {
		t.Fatalf("rsi health next-task binding exit=%d stdout=%s stderr=%s", code, stdout, stderr)
	}
	var got struct {
		Status                 string `json:"status"`
		RSICapability          string `json:"rsi_capability"`
		FoundryNextTaskBinding struct {
			Status                     string  `json:"status"`
			Passed                     bool    `json:"passed"`
			NextTaskEvidence           string  `json:"next_task_evidence"`
			CandidateEvidence          string  `json:"candidate_evidence"`
			GateEvidence               string  `json:"gate_evidence"`
			RequiredImprovementPercent float64 `json:"required_improvement_percent"`
			ActualImprovementPercent   float64 `json:"actual_improvement_percent"`
			AutonomousClaim            string  `json:"autonomous_claim"`
			MutatesRepositories        bool    `json:"mutates_repositories"`
		} `json:"foundry_next_task_binding"`
	}
	if err := json.Unmarshal([]byte(stdout), &got); err != nil {
		t.Fatalf("invalid rsi health next-task binding JSON: %v\n%s", err, stdout)
	}
	if got.Status != "passed" ||
		got.RSICapability != "demonstrated_local_fixture_loop" ||
		got.FoundryNextTaskBinding.Status != "passed" ||
		!got.FoundryNextTaskBinding.Passed ||
		got.FoundryNextTaskBinding.NextTaskEvidence != paths.foundryNextTask ||
		got.FoundryNextTaskBinding.CandidateEvidence != paths.foundryCandidate ||
		got.FoundryNextTaskBinding.GateEvidence != paths.foundry ||
		got.FoundryNextTaskBinding.RequiredImprovementPercent != 5 ||
		got.FoundryNextTaskBinding.ActualImprovementPercent < got.FoundryNextTaskBinding.RequiredImprovementPercent ||
		got.FoundryNextTaskBinding.AutonomousClaim != "derived_local_next_improvement" ||
		got.FoundryNextTaskBinding.MutatesRepositories {
		t.Fatalf("unexpected Foundry next-task binding: %+v", got)
	}
}

func TestRSIHealthBindsForgeRetainedEvidenceChain(t *testing.T) {
	paths := writeRSIHealthFixtures(t, true)
	code, stdout, stderr := runWithFake([]string{
		"rsi", "health",
		"--arena-gate", paths.arena,
		"--crucible-gate", paths.crucible,
		"--sentinel-verdict", paths.sentinel,
		"--promoter-gate", paths.promoter,
		"--foundry-gate", paths.foundry,
		"--foundry-candidate", paths.foundryCandidate,
		"--foundry-next-task", paths.foundryNextTask,
		"--forge-retained-gate", paths.forgeRetainedGate,
		"--forge-retained-candidate", paths.forgeRetainedCandidate,
		"--forge-retained-next-task", paths.forgeRetainedNextTask,
		"--forge-retained-command-health", paths.forgeRetainedCommandHealth,
		"--json",
	}, &fakeRunner{})
	if code != 0 {
		t.Fatalf("rsi health forge retention binding exit=%d stdout=%s stderr=%s", code, stdout, stderr)
	}
	var got struct {
		Status                string `json:"status"`
		RSICapability         string `json:"rsi_capability"`
		ForgeRetentionBinding struct {
			Status              string   `json:"status"`
			Passed              bool     `json:"passed"`
			GoalID              string   `json:"goal_id"`
			Iteration           string   `json:"iteration"`
			Phase               string   `json:"phase"`
			RetainedEvidence    []string `json:"retained_evidence"`
			RetainedOutputCount int      `json:"retained_output_count"`
			MutatesRepositories bool     `json:"mutates_repositories"`
		} `json:"forge_retention_binding"`
	}
	if err := json.Unmarshal([]byte(stdout), &got); err != nil {
		t.Fatalf("invalid rsi health forge retention binding JSON: %v\n%s", err, stdout)
	}
	if got.Status != "passed" ||
		got.RSICapability != "demonstrated_local_fixture_loop" ||
		got.ForgeRetentionBinding.Status != "passed" ||
		!got.ForgeRetentionBinding.Passed ||
		got.ForgeRetentionBinding.GoalID != "ao2-weekend-hardening" ||
		got.ForgeRetentionBinding.Iteration != "20260619T180000Z-verification" ||
		got.ForgeRetentionBinding.Phase != "verification" ||
		len(got.ForgeRetentionBinding.RetainedEvidence) != 4 ||
		got.ForgeRetentionBinding.RetainedOutputCount != 4 ||
		got.ForgeRetentionBinding.MutatesRepositories {
		t.Fatalf("unexpected Forge retention binding: %+v", got)
	}
}

func TestRSIHealthFailsClosedWhenForgeRetentionDoesNotBind(t *testing.T) {
	paths := writeRSIHealthFixtures(t, true)
	if err := os.WriteFile(paths.forgeRetainedNextTask, []byte(`{
  "schema_version": "ao.forge.goal-run-retained-evidence.v0.1",
  "goal_id": "ao2-weekend-hardening",
  "iteration": "20260619T180000Z-verification",
  "phase": "verification",
  "captured_outputs": [
    {
      "label": "ao-foundry-rsi-next-improvement-task",
      "command": "foundry pulse run",
      "schema_version": "ao.foundry.rsi-next-improvement-task.v0.1",
      "status": "ready",
      "required_improvement_percent": 5,
      "actual_improvement_percent": 4,
      "autonomous_claim": "derived_local_next_improvement",
      "mutates_repositories": false
    }
  ],
  "retention_policy": {
    "temporary_paths_allowed": false,
    "minimum_retention_days_after_terminal_phase": 90
  },
  "retention_metadata": {
    "retention_class": "loop_evidence",
    "retain_while_goal_active": true,
    "deletion_requires_review": true
  }
}`), 0o644); err != nil {
		t.Fatalf("write mismatched forge retained next task: %v", err)
	}
	code, stdout, stderr := runWithFake([]string{
		"rsi", "health",
		"--arena-gate", paths.arena,
		"--crucible-gate", paths.crucible,
		"--sentinel-verdict", paths.sentinel,
		"--promoter-gate", paths.promoter,
		"--foundry-gate", paths.foundry,
		"--foundry-candidate", paths.foundryCandidate,
		"--foundry-next-task", paths.foundryNextTask,
		"--forge-retained-gate", paths.forgeRetainedGate,
		"--forge-retained-candidate", paths.forgeRetainedCandidate,
		"--forge-retained-next-task", paths.forgeRetainedNextTask,
		"--forge-retained-command-health", paths.forgeRetainedCommandHealth,
		"--json",
	}, &fakeRunner{})
	if code != 1 {
		t.Fatalf("rsi health mismatched forge retention exit=%d want 1 stdout=%s stderr=%s", code, stdout, stderr)
	}
	var got struct {
		Status                string `json:"status"`
		RSICapability         string `json:"rsi_capability"`
		ForgeRetentionBinding struct {
			Status string `json:"status"`
			Passed bool   `json:"passed"`
		} `json:"forge_retention_binding"`
	}
	if err := json.Unmarshal([]byte(stdout), &got); err != nil {
		t.Fatalf("invalid rsi health mismatched forge retention JSON: %v\n%s", err, stdout)
	}
	if got.Status != "blocked" ||
		got.RSICapability != "not_demonstrated" ||
		got.ForgeRetentionBinding.Status != "blocked" ||
		got.ForgeRetentionBinding.Passed {
		t.Fatalf("unexpected mismatched Forge retention status: %+v", got)
	}
	if !strings.Contains(stderr, "RSI health blocked") {
		t.Fatalf("stderr missing blocked message: %s", stderr)
	}
}

func TestRSIHealthFailsClosedWhenForgeRetainedProofViolatesSchema(t *testing.T) {
	paths := writeRSIHealthFixtures(t, true)
	if err := os.WriteFile(paths.forgeRetainedGate, []byte(`{
  "schema_version": "ao.forge.goal-run-retained-evidence.v0.1",
  "goal_id": "ao2-weekend-hardening",
  "iteration": "20260619T180000Z-verification",
  "phase": "verification",
  "summary": "Schema-invalid retained AO Foundry RSI improvement gate.",
  "captured_outputs": [
    {
      "label": "ao-foundry-rsi-improvement-gate",
      "command": "foundry pulse run",
      "schema_version": "ao.foundry.rsi-improvement-gate.v0.1",
      "status": "passed",
      "baseline_score": 90,
      "candidate_score": 100,
      "required_improvement_percent": 5,
      "actual_improvement_percent": 10,
      "autonomous_claim": "measured_local_improvement",
      "mutates_repositories": false
    }
  ],
  "retention_policy": {
    "layout": "docs/evidence/goals/<goal_id>/<YYYYMMDDTHHMMSSZ>-<phase>/",
    "temporary_paths_allowed": false,
    "minimum_retention_days_after_terminal_phase": 90
  },
  "retention_metadata": {
    "retained_at": "2026-06-19T18:00:00Z",
    "retention_class": "loop_evidence",
    "retain_while_goal_active": true,
    "deletion_requires_review": true
  }
}`), 0o644); err != nil {
		t.Fatalf("write schema-invalid forge retained gate: %v", err)
	}

	code, stdout, stderr := runWithFake([]string{
		"rsi", "health",
		"--arena-gate", paths.arena,
		"--crucible-gate", paths.crucible,
		"--sentinel-verdict", paths.sentinel,
		"--promoter-gate", paths.promoter,
		"--foundry-gate", paths.foundry,
		"--foundry-candidate", paths.foundryCandidate,
		"--foundry-next-task", paths.foundryNextTask,
		"--forge-retained-gate", paths.forgeRetainedGate,
		"--forge-retained-candidate", paths.forgeRetainedCandidate,
		"--forge-retained-next-task", paths.forgeRetainedNextTask,
		"--forge-retained-command-health", paths.forgeRetainedCommandHealth,
		"--json",
	}, &fakeRunner{})
	if code != 1 {
		t.Fatalf("rsi health schema-invalid forge retention exit=%d want 1 stdout=%s stderr=%s", code, stdout, stderr)
	}
	var got struct {
		Status                string `json:"status"`
		RSICapability         string `json:"rsi_capability"`
		ForgeRetentionBinding struct {
			Status string `json:"status"`
			Passed bool   `json:"passed"`
		} `json:"forge_retention_binding"`
	}
	if err := json.Unmarshal([]byte(stdout), &got); err != nil {
		t.Fatalf("invalid rsi health schema-invalid forge retention JSON: %v\n%s", err, stdout)
	}
	if got.Status != "blocked" ||
		got.RSICapability != "not_demonstrated" ||
		got.ForgeRetentionBinding.Status != "blocked" ||
		got.ForgeRetentionBinding.Passed {
		t.Fatalf("unexpected schema-invalid Forge retention status: %+v", got)
	}
	if !strings.Contains(stderr, "RSI health blocked") {
		t.Fatalf("stderr missing blocked message: %s", stderr)
	}
}

func TestRSIHealthAcceptsFoundryPulseScorePercentGate(t *testing.T) {
	paths := writeRSIHealthFixtures(t, true)
	if err := os.WriteFile(paths.foundry, []byte(`{
  "schema_version": "ao.foundry.rsi-improvement-gate.v0.1",
  "status": "passed",
  "baseline_score_percent": 90,
  "candidate_score_percent": 100,
  "required_improvement_percent": 5,
  "actual_improvement_percent": 10,
  "autonomous_claim": "measured_local_improvement",
  "mutates_repositories": false,
  "evidence": [
    {
      "label": "baseline",
      "path": "examples/evals/rsi-baseline.eval-result.json",
      "schema_version": "ao.foundry.eval-result.v0.1",
      "status": "ready",
      "score": 90,
      "max_score": 100,
      "sha256": "e5824cee9ef1455fcdc74dfecc7e30710edb5ef67cb939eff92d57283dfdc52e"
    },
    {
      "label": "candidate",
      "path": "tmp/pulse-rsi-verify/eval-result.json",
      "schema_version": "ao.foundry.eval-result.v0.1",
      "status": "ready",
      "score": 100,
      "max_score": 100,
      "sha256": "cf3f99d1b1639ef2fd2ba24cb75481211c0c4b0ad8e81605be5fbd6e3f7f39ec"
    }
  ],
  "next_actions": []
}`), 0o644); err != nil {
		t.Fatalf("write foundry pulse score-percent gate: %v", err)
	}

	code, stdout, stderr := runWithFake([]string{
		"rsi", "health",
		"--arena-gate", paths.arena,
		"--crucible-gate", paths.crucible,
		"--sentinel-verdict", paths.sentinel,
		"--promoter-gate", paths.promoter,
		"--foundry-gate", paths.foundry,
		"--foundry-candidate", paths.foundryCandidate,
		"--foundry-next-task", paths.foundryNextTask,
		"--forge-retained-gate", paths.forgeRetainedGate,
		"--forge-retained-candidate", paths.forgeRetainedCandidate,
		"--forge-retained-next-task", paths.forgeRetainedNextTask,
		"--forge-retained-command-health", paths.forgeRetainedCommandHealth,
	}, &fakeRunner{})
	if code != 0 {
		t.Fatalf("rsi health with Foundry pulse score-percent gate exit=%d stdout=%s stderr=%s", code, stdout, stderr)
	}
	if !strings.Contains(stdout, "ao_command_rsi_health=passed") ||
		!strings.Contains(stdout, "rsi_capability=demonstrated_local_fixture_loop") {
		t.Fatalf("stdout missing passed health:\n%s", stdout)
	}
}

func TestRSIHealthFailsClosedWhenFoundryNextTaskDoesNotBind(t *testing.T) {
	paths := writeRSIHealthFixtures(t, true)
	if err := os.WriteFile(paths.foundryNextTask, []byte(`{
  "schema_version": "ao.foundry.rsi-next-improvement-task.v0.1",
  "status": "ready",
  "generated_by": "foundry pulse run",
  "candidate_evidence_path": "tmp/wrong-candidate.json",
  "gate_evidence_path": "`+paths.foundry+`",
  "required_improvement_percent": 5,
  "actual_improvement_percent": 10,
  "autonomous_claim": "derived_local_next_improvement",
  "mutates_repositories": false,
  "next_actions": [
    "retain rsi_next_improvement_task with RSI candidate and gate evidence"
  ]
}`), 0o644); err != nil {
		t.Fatalf("write mismatched foundry next task: %v", err)
	}
	code, stdout, stderr := runWithFake([]string{
		"rsi", "health",
		"--arena-gate", paths.arena,
		"--crucible-gate", paths.crucible,
		"--sentinel-verdict", paths.sentinel,
		"--promoter-gate", paths.promoter,
		"--foundry-gate", paths.foundry,
		"--foundry-candidate", paths.foundryCandidate,
		"--foundry-next-task", paths.foundryNextTask,
		"--forge-retained-gate", paths.forgeRetainedGate,
		"--forge-retained-candidate", paths.forgeRetainedCandidate,
		"--forge-retained-next-task", paths.forgeRetainedNextTask,
		"--forge-retained-command-health", paths.forgeRetainedCommandHealth,
		"--json",
	}, &fakeRunner{})
	if code != 1 {
		t.Fatalf("rsi health mismatched next-task exit=%d want 1 stdout=%s stderr=%s", code, stdout, stderr)
	}
	var got struct {
		Status                 string `json:"status"`
		RSICapability          string `json:"rsi_capability"`
		FoundryNextTaskBinding struct {
			Status string `json:"status"`
			Passed bool   `json:"passed"`
		} `json:"foundry_next_task_binding"`
	}
	if err := json.Unmarshal([]byte(stdout), &got); err != nil {
		t.Fatalf("invalid rsi health mismatched next-task JSON: %v\n%s", err, stdout)
	}
	if got.Status != "blocked" ||
		got.RSICapability != "not_demonstrated" ||
		got.FoundryNextTaskBinding.Status != "blocked" ||
		got.FoundryNextTaskBinding.Passed {
		t.Fatalf("unexpected mismatched Foundry next-task status: %+v", got)
	}
	if !strings.Contains(stderr, "RSI health blocked") {
		t.Fatalf("stderr missing blocked message: %s", stderr)
	}
}

func TestRSIHealthWritesCanonicalBundle(t *testing.T) {
	paths := writeRSIHealthFixtures(t, true)
	bundleOut := filepath.Join(t.TempDir(), "rsi-health-bundle.json")
	code, stdout, stderr := runWithFake([]string{
		"rsi", "health",
		"--arena-gate", paths.arena,
		"--crucible-gate", paths.crucible,
		"--sentinel-verdict", paths.sentinel,
		"--promoter-gate", paths.promoter,
		"--foundry-gate", paths.foundry,
		"--foundry-candidate", paths.foundryCandidate,
		"--foundry-next-task", paths.foundryNextTask,
		"--forge-retained-gate", paths.forgeRetainedGate,
		"--forge-retained-candidate", paths.forgeRetainedCandidate,
		"--forge-retained-next-task", paths.forgeRetainedNextTask,
		"--forge-retained-command-health", paths.forgeRetainedCommandHealth,
		"--bundle-out", bundleOut,
	}, &fakeRunner{})
	if code != 0 {
		t.Fatalf("rsi health bundle exit=%d stdout=%s stderr=%s", code, stdout, stderr)
	}
	if !strings.Contains(stdout, "bundle="+bundleOut) {
		t.Fatalf("rsi health stdout missing bundle path:\n%s", stdout)
	}

	bytes, err := os.ReadFile(bundleOut)
	if err != nil {
		t.Fatalf("read bundle: %v", err)
	}
	var got struct {
		SchemaVersion        string `json:"schema_version"`
		CommandSchemaVersion string `json:"command_schema_version"`
		Status               string `json:"status"`
		RSIMode              string `json:"rsi_mode"`
		RSICapability        string `json:"rsi_capability"`
		OperatorMode         string `json:"operator_mode"`
		MutatesRepositories  bool   `json:"mutates_repositories"`
		ClaimLevels          []struct {
			Claim    string `json:"claim"`
			Decision string `json:"decision"`
			Status   string `json:"status"`
		} `json:"claim_levels"`
		Families []struct {
			Family   string `json:"family"`
			Status   string `json:"status"`
			Passed   bool   `json:"passed"`
			Evidence string `json:"evidence"`
			SHA256   string `json:"sha256"`
		} `json:"families"`
	}
	if err := json.Unmarshal(bytes, &got); err != nil {
		t.Fatalf("invalid bundle JSON: %v\n%s", err, string(bytes))
	}
	if got.SchemaVersion != "ao.command.rsi-health-bundle.v0.1" ||
		got.CommandSchemaVersion != "ao.command.v0.1" ||
		got.Status != "passed" ||
		got.RSIMode != "governed_fixture_local" ||
		got.RSICapability != "demonstrated_local_fixture_loop" ||
		got.OperatorMode != "read_only" ||
		got.MutatesRepositories ||
		len(got.ClaimLevels) != 2 ||
		len(got.Families) != 5 {
		t.Fatalf("unexpected bundle: %+v", got)
	}
	if got.ClaimLevels[0].Claim != "bounded_governed_rsi" ||
		got.ClaimLevels[0].Decision != "allowed" ||
		got.ClaimLevels[0].Status != "passed" ||
		got.ClaimLevels[1].Claim != "full_autonomous_self_mutating_rsi" ||
		got.ClaimLevels[1].Decision != "denied" ||
		got.ClaimLevels[1].Status != "blocked" {
		t.Fatalf("unexpected bundle claim levels: %+v", got.ClaimLevels)
	}
	for _, family := range got.Families {
		if !family.Passed || family.Evidence == "" || len(family.SHA256) != 64 {
			t.Fatalf("family missing pass/evidence/hash: %+v", family)
		}
	}
}

func TestRSIHealthFailsClosedWhenAssuranceFamilyBlocks(t *testing.T) {
	paths := writeRSIHealthFixtures(t, false)
	code, stdout, stderr := runWithFake([]string{
		"rsi", "health",
		"--arena-gate", paths.arena,
		"--crucible-gate", paths.crucible,
		"--sentinel-verdict", paths.sentinel,
		"--promoter-gate", paths.promoter,
		"--foundry-gate", paths.foundry,
		"--foundry-candidate", paths.foundryCandidate,
		"--foundry-next-task", paths.foundryNextTask,
		"--forge-retained-gate", paths.forgeRetainedGate,
		"--forge-retained-candidate", paths.forgeRetainedCandidate,
		"--forge-retained-next-task", paths.forgeRetainedNextTask,
		"--forge-retained-command-health", paths.forgeRetainedCommandHealth,
	}, &fakeRunner{})
	if code != 1 {
		t.Fatalf("rsi health blocked exit=%d want 1 stdout=%s stderr=%s", code, stdout, stderr)
	}
	if !strings.Contains(stderr, "RSI health blocked") {
		t.Fatalf("stderr missing blocked message: %s", stderr)
	}
	if !strings.Contains(stdout, "ao_command_rsi_health=blocked") ||
		!strings.Contains(stdout, "family=ao-sentinel status=incident") ||
		!strings.Contains(stdout, "rsi_capability=not_demonstrated") {
		t.Fatalf("blocked stdout missing expected signals:\n%s", stdout)
	}
}

func TestRSIManifestReportsArchitectureClaimBoundary(t *testing.T) {
	manifest := writeRSIManifestFixture(t, true)
	code, stdout, stderr := runWithFake([]string{"rsi", "manifest", "--manifest", manifest}, &fakeRunner{})
	if code != 0 {
		t.Fatalf("rsi manifest exit=%d stderr=%s", code, stderr)
	}
	for _, want := range []string{
		"ao_command_rsi_manifest=passed",
		"schema_version=ao.architecture.rsi-claim-evidence-manifest.v0.1",
		"manifest=" + manifest,
		"operator_mode=read_only",
		"mutates_repositories=false",
		"claim_level=bounded_governed_rsi decision=allowed status=supported_when_chain_passes",
		"claim_level=full_autonomous_self_mutating_rsi decision=denied status=missing_required_full_claim_evidence",
		"active_repositories=6",
		"out_of_scope_repositories=5",
		"full_claim_required_evidence=6",
	} {
		if !strings.Contains(stdout, want) {
			t.Fatalf("rsi manifest stdout missing %q:\n%s", want, stdout)
		}
	}
}

func TestRSIManifestJSONReportsArchitectureClaimBoundary(t *testing.T) {
	manifest := writeRSIManifestFixture(t, true)
	code, stdout, stderr := runWithFake([]string{"rsi", "manifest", "--manifest", manifest, "--json"}, &fakeRunner{})
	if code != 0 {
		t.Fatalf("rsi manifest json exit=%d stderr=%s", code, stderr)
	}
	var got struct {
		SchemaVersion        string `json:"schema_version"`
		CommandSchemaVersion string `json:"command_schema_version"`
		Status               string `json:"status"`
		Manifest             string `json:"manifest"`
		OperatorMode         string `json:"operator_mode"`
		MutatesRepositories  bool   `json:"mutates_repositories"`
		ClaimLevels          []struct {
			ClaimLevel string `json:"claim_level"`
			Decision   string `json:"decision"`
			Status     string `json:"status"`
		} `json:"claim_levels"`
		ActiveRepositories                 []struct{ ID string } `json:"active_repositories"`
		DeprecatedOrOutOfScopeRepositories []struct{ ID string } `json:"deprecated_or_out_of_scope_repositories"`
		FullClaimRequiredEvidence          []string              `json:"full_claim_required_evidence"`
	}
	if err := json.Unmarshal([]byte(stdout), &got); err != nil {
		t.Fatalf("invalid rsi manifest JSON: %v\n%s", err, stdout)
	}
	if got.SchemaVersion != "ao.command.rsi-manifest.v0.1" ||
		got.CommandSchemaVersion != "ao.command.v0.1" ||
		got.Status != "passed" ||
		got.Manifest != manifest ||
		got.OperatorMode != "read_only" ||
		got.MutatesRepositories ||
		len(got.ClaimLevels) != 2 ||
		len(got.ActiveRepositories) != 6 ||
		len(got.DeprecatedOrOutOfScopeRepositories) != 5 ||
		len(got.FullClaimRequiredEvidence) != 6 {
		t.Fatalf("unexpected rsi manifest summary: %+v", got)
	}
	if got.ClaimLevels[0].ClaimLevel != "bounded_governed_rsi" || got.ClaimLevels[0].Decision != "allowed" {
		t.Fatalf("unexpected bounded claim level: %+v", got.ClaimLevels)
	}
	if got.ClaimLevels[1].ClaimLevel != "full_autonomous_self_mutating_rsi" || got.ClaimLevels[1].Decision != "denied" {
		t.Fatalf("unexpected full claim level: %+v", got.ClaimLevels)
	}
}

func TestRSIManifestFailsClosedWithoutDeniedFullClaim(t *testing.T) {
	manifest := writeRSIManifestFixture(t, false)
	code, stdout, stderr := runWithFake([]string{"rsi", "manifest", "--manifest", manifest, "--json"}, &fakeRunner{})
	if code != 1 {
		t.Fatalf("rsi manifest invalid exit=%d want 1 stdout=%s stderr=%s", code, stdout, stderr)
	}
	if !strings.Contains(stderr, "full_autonomous_self_mutating_rsi denied claim level is required") {
		t.Fatalf("stderr missing denied full claim reason: %s", stderr)
	}
}

func TestRSIManifestFailsClosedWithoutAO2ControlPlaneReadback(t *testing.T) {
	manifest := writeRSIManifestFixtureMissingAO2ControlPlaneReadback(t)
	code, stdout, stderr := runWithFake([]string{"rsi", "manifest", "--manifest", manifest, "--json"}, &fakeRunner{})
	if code != 1 {
		t.Fatalf("rsi manifest invalid exit=%d want 1 stdout=%s stderr=%s", code, stdout, stderr)
	}
	if !strings.Contains(stderr, "bounded_governed_rsi required evidence must include ao2-control-plane RSI claim-readiness readback") {
		t.Fatalf("stderr missing ao2-control-plane readback reason: %s", stderr)
	}
}

func TestRSIManifestFailsClosedWithoutAO2SelfChangeDryRunReadback(t *testing.T) {
	manifest := writeRSIManifestFixtureMissingAO2SelfChangeDryRunReadback(t)
	code, stdout, stderr := runWithFake([]string{"rsi", "manifest", "--manifest", manifest, "--json"}, &fakeRunner{})
	if code != 1 {
		t.Fatalf("rsi manifest invalid exit=%d want 1 stdout=%s stderr=%s", code, stdout, stderr)
	}
	if !strings.Contains(stderr, "bounded_governed_rsi required evidence must include AO2 self-change dry-run and control-plane readback") {
		t.Fatalf("stderr missing AO2 self-change dry-run readback reason: %s", stderr)
	}
}

func TestRSIManifestFailsClosedWithoutAO2RollbackRehearsalReadback(t *testing.T) {
	manifest := writeRSIManifestFixtureMissingAO2RollbackRehearsalReadback(t)
	code, stdout, stderr := runWithFake([]string{"rsi", "manifest", "--manifest", manifest, "--json"}, &fakeRunner{})
	if code != 1 {
		t.Fatalf("rsi manifest invalid exit=%d want 1 stdout=%s stderr=%s", code, stdout, stderr)
	}
	if !strings.Contains(stderr, "AO2 RSI rollback rehearsal evidence and control-plane readback are required") {
		t.Fatalf("stderr missing AO2 rollback rehearsal readback reason: %s", stderr)
	}
}

func TestRSIManifestFailsClosedWithoutForgeManifestRetentionPin(t *testing.T) {
	manifest := writeRSIManifestFixtureMissingForgeManifestRetentionPin(t)
	code, stdout, stderr := runWithFake([]string{"rsi", "manifest", "--manifest", manifest, "--json"}, &fakeRunner{})
	if code != 1 {
		t.Fatalf("rsi manifest invalid exit=%d want 1 stdout=%s stderr=%s", code, stdout, stderr)
	}
	if !strings.Contains(stderr, "AO Forge retained AO Command RSI manifest evidence is required") {
		t.Fatalf("stderr missing AO Forge manifest retention reason: %s", stderr)
	}
}

func TestRSIManifestFailsClosedWithoutForgeArchitectureReadbackPin(t *testing.T) {
	manifest := writeRSIManifestFixtureMissingForgeArchitectureReadbackPin(t)
	code, stdout, stderr := runWithFake([]string{"rsi", "manifest", "--manifest", manifest, "--json"}, &fakeRunner{})
	if code != 1 {
		t.Fatalf("rsi manifest invalid exit=%d want 1 stdout=%s stderr=%s", code, stdout, stderr)
	}
	if !strings.Contains(stderr, "AO Forge architecture RSI pin readback evidence is required") {
		t.Fatalf("stderr missing AO Forge architecture readback reason: %s", stderr)
	}
}

func TestRSIManifestFailsClosedWithoutCovenantRetainedRollbackBoundaryPin(t *testing.T) {
	manifest := writeRSIManifestFixtureMissingCovenantRetainedRollbackBoundaryPin(t)
	code, stdout, stderr := runWithFake([]string{"rsi", "manifest", "--manifest", manifest, "--json"}, &fakeRunner{})
	if code != 1 {
		t.Fatalf("rsi manifest invalid exit=%d want 1 stdout=%s stderr=%s", code, stdout, stderr)
	}
	if !strings.Contains(stderr, "AO Covenant retained rollback-only denial evidence is required") {
		t.Fatalf("stderr missing AO Covenant retained rollback boundary reason: %s", stderr)
	}
}

func TestRSIManifestFailsClosedWithoutCovenantLiveSelfChangeAuthorityPacketPin(t *testing.T) {
	manifest := writeRSIManifestFixtureMissingCovenantLiveSelfChangeAuthorityPacketPin(t)
	code, stdout, stderr := runWithFake([]string{"rsi", "manifest", "--manifest", manifest, "--json"}, &fakeRunner{})
	if code != 1 {
		t.Fatalf("rsi manifest invalid exit=%d want 1 stdout=%s stderr=%s", code, stdout, stderr)
	}
	if !strings.Contains(stderr, "AO Covenant live self-change authority packet evidence is required") {
		t.Fatalf("stderr missing AO Covenant live self-change authority packet reason: %s", stderr)
	}
}

func TestRSIManifestFailsClosedWithoutAO2AuthorityPacketReadbackPins(t *testing.T) {
	manifest := writeRSIManifestFixtureMissingAO2AuthorityPacketReadbackPins(t)
	code, stdout, stderr := runWithFake([]string{"rsi", "manifest", "--manifest", manifest, "--json"}, &fakeRunner{})
	if code != 1 {
		t.Fatalf("rsi manifest invalid exit=%d want 1 stdout=%s stderr=%s", code, stdout, stderr)
	}
	if !strings.Contains(stderr, "AO2 RSI authority packet candidate and control-plane readback are required") {
		t.Fatalf("stderr missing AO2 authority packet readback reason: %s", stderr)
	}
}

func TestRSIManifestFailsClosedWithoutAO2AuthorityPacketRequiredEvidence(t *testing.T) {
	manifest := writeRSIManifestFixtureMissingAO2AuthorityPacketRequiredEvidence(t)
	code, stdout, stderr := runWithFake([]string{"rsi", "manifest", "--manifest", manifest, "--json"}, &fakeRunner{})
	if code != 1 {
		t.Fatalf("rsi manifest invalid exit=%d want 1 stdout=%s stderr=%s", code, stdout, stderr)
	}
	if !strings.Contains(stderr, "bounded_governed_rsi required evidence must include AO2 authority packet candidate and control-plane readback") {
		t.Fatalf("stderr missing AO2 authority packet required-evidence reason: %s", stderr)
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

func TestNextFallbackStaysWithinActiveStack(t *testing.T) {
	fake := &fakeRunner{stdout: []byte(`{
		"status": "passed",
		"readiness_percent": 100,
		"passed_gates": 12,
		"total_gates": 12,
		"next_actions": []
	}`)}

	code, stdout, stderr := runWithFake([]string{"next"}, fake)
	if code != 0 {
		t.Fatalf("next exit=%d stderr=%s", code, stderr)
	}
	for _, want := range []string{
		"ao_command_next=passed",
		"next_action=inspect-active-stack-handoff required=false",
		"AO Foundry active-stack readiness ledger",
	} {
		if !strings.Contains(stdout, want) {
			t.Fatalf("next fallback stdout missing %q:\n%s", want, stdout)
		}
	}
	for _, forbidden := range []string{"ao-arena", "agy-swarms", "ao-conductor"} {
		if strings.Contains(stdout, forbidden) {
			t.Fatalf("next fallback must not mention out-of-scope project %q:\n%s", forbidden, stdout)
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
	rsiHealthSchema := read("docs", "contracts", "rsi-health-v0.1.schema.json")
	rsiHealthBundleSchema := read("docs", "contracts", "rsi-health-bundle-v0.1.schema.json")
	rsiEvidenceChainSmoke := read("scripts", "rsi-evidence-chain-smoke.sh")
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
		{name: "README Atlas status command", doc: readme, want: "go run ./cmd/ao-command atlas status --status ../ao-foundry/examples/contract-fixtures/valid/foundry-atlas-status-v0.1.json"},
		{name: "README Atlas observer schema", doc: readme, want: "ao.foundry.atlas-status.v0.1"},
		{name: "README Atlas compile-only boundary", doc: readme, want: "atlas_authority=compile_only"},
		{name: "README Atlas no scheduling", doc: readme, want: "schedules_work=false"},
		{name: "README Atlas read-only mutation boundary", doc: readme, want: "mutates_repositories=false"},
		{name: "README RSI health command", doc: readme, want: "go run ./cmd/ao-command rsi health"},
		{name: "README RSI manifest command", doc: readme, want: "go run ./cmd/ao-command rsi manifest --manifest ../ao-architecture/overview/rsi-claim-evidence-manifest.json"},
		{name: "README RSI health Foundry candidate", doc: readme, want: "--foundry-candidate ../ao-foundry/tmp/pulse-rsi-verify/rsi-candidate.json"},
		{name: "README RSI health Foundry next task", doc: readme, want: "--foundry-next-task ../ao-foundry/tmp/pulse-rsi-verify/rsi-next-improvement-task.json"},
		{name: "README RSI health Forge retained gate", doc: readme, want: "--forge-retained-gate ../ao-forge/docs/evidence/goals/ao2-weekend-hardening/20260619T180000Z-verification/ao-foundry-rsi-improvement-gate-retention-proof.json"},
		{name: "README RSI health Forge retained command", doc: readme, want: "--forge-retained-command-health ../ao-forge/docs/evidence/goals/ao2-weekend-hardening/20260619T180000Z-verification/ao-command-rsi-health-retention-proof.json"},
		{name: "README RSI health bundle", doc: readme, want: "--bundle-out tmp/rsi-health-bundle.json"},
		{name: "README RSI evidence chain smoke", doc: readme, want: "scripts/rsi-evidence-chain-smoke.sh --forge ../ao-forge --foundry ../ao-foundry --covenant ../ao-covenant"},
		{name: "README RSI health read-only", doc: readme, want: "mutates_repositories=false"},
		{name: "README RSI bounded claim", doc: readme, want: "claim_level=bounded_governed_rsi decision=allowed"},
		{name: "README RSI full claim denied", doc: readme, want: "claim_level=full_autonomous_self_mutating_rsi decision=denied"},
		{name: "README RSI health schema", doc: readme, want: "docs/contracts/rsi-health-v0.1.schema.json"},
		{name: "README RSI health bundle schema", doc: readme, want: "docs/contracts/rsi-health-bundle-v0.1.schema.json"},
		{name: "README RSI manifest read-only", doc: readme, want: "`rsi manifest` reads AO Architecture's"},
		{name: "README RSI manifest readback schema", doc: readme, want: "ao2.cp-ao2-rsi-claim-readiness-readback.v1"},
		{name: "README RSI self-change dry-run schema", doc: readme, want: "ao2.rsi-governed-self-change-dry-run.v1"},
		{name: "README RSI self-change readback schema", doc: readme, want: "ao2.cp-ao2-rsi-self-change-dry-run-readback.v1"},
		{name: "README RSI rollback rehearsal status", doc: readme, want: "rollback_rehearsal.status=passed"},
		{name: "README RSI rollback rehearsal PRs", doc: readme, want: "AO2 PR #200"},
		{name: "README RSI Forge manifest retention pin", doc: readme, want: "ao-command-rsi-manifest-retention-proof.json"},
		{name: "README RSI Forge architecture readback pin", doc: readme, want: "goalrun.architecture_rsi_pin_readback"},
		{name: "README RSI Forge architecture readback document", doc: readme, want: "ao-architecture-rsi-pin-readback.json"},
		{name: "README RSI Covenant rollback-retained pin", doc: readme, want: "rollback-retained.contract.json"},
		{name: "README RSI Covenant authority packet schema pin", doc: readme, want: "covenant.live-self-change-authority.v1"},
		{name: "README RSI Covenant authority packet fixture pin", doc: readme, want: "live-self-change-authority.packet.json"},
		{name: "README RSI AO2 authority packet PR", doc: readme, want: "AO2 PR #201"},
		{name: "README RSI authority packet not publish valid", doc: readme, want: "schema_valid_for_claim_publish=false"},
		{name: "README RSI control-plane authority packet readback", doc: readme, want: "ao2.cp-ao2-rsi-authority-packet-readback.v1"},
		{name: "README RSI manifest no mutation", doc: readme, want: "mutates_repositories=false"},
		{name: "README RSI Forge aggregate proof", doc: readme, want: "bounded-rsi-improvement-chain-retention-proof.json"},
		{name: "README RSI Covenant fixture", doc: readme, want: "examples/full-rsi-claim-boundary/"},
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
		{name: "production readiness docs RSI health schema", doc: productionReadiness, want: "rsi-health-v0.1.schema.json"},
		{name: "production readiness docs RSI health bundle schema", doc: productionReadiness, want: "rsi-health-bundle-v0.1.schema.json"},
		{name: "production readiness docs active stack command", doc: productionReadiness, want: "ao-command stack --ledger"},
		{name: "production readiness docs active stack gate", doc: productionReadiness, want: "active-stack handoff"},
		{name: "production readiness docs Atlas status command", doc: productionReadiness, want: "ao-command atlas status --status"},
		{name: "production readiness docs Atlas compile-only boundary", doc: productionReadiness, want: "atlas_authority=compile_only"},
		{name: "production readiness docs bounded RSI claim", doc: productionReadiness, want: "claim_level=bounded_governed_rsi decision=allowed"},
		{name: "production readiness docs full RSI claim denied", doc: productionReadiness, want: "claim_level=full_autonomous_self_mutating_rsi decision=denied"},
		{name: "production readiness docs RSI readback schema", doc: productionReadiness, want: "ao2.cp-ao2-rsi-claim-readiness-readback.v1"},
		{name: "production readiness docs RSI self-change dry-run schema", doc: productionReadiness, want: "ao2.rsi-governed-self-change-dry-run.v1"},
		{name: "production readiness docs RSI self-change readback schema", doc: productionReadiness, want: "ao2.cp-ao2-rsi-self-change-dry-run-readback.v1"},
		{name: "production readiness docs RSI rollback rehearsal status", doc: productionReadiness, want: "rollback_rehearsal.status=passed"},
		{name: "production readiness docs RSI rollback rehearsal PRs", doc: productionReadiness, want: "ao2-control-plane PR\n  #72"},
		{name: "production readiness docs RSI Forge manifest retention pin", doc: productionReadiness, want: "ao-command-rsi-manifest-retention-proof.json"},
		{name: "production readiness docs RSI Forge architecture readback pin", doc: productionReadiness, want: "goalrun.architecture_rsi_pin_readback"},
		{name: "production readiness docs RSI Covenant rollback-retained pin", doc: productionReadiness, want: "rollback-retained.contract.json"},
		{name: "production readiness docs RSI Covenant authority packet schema pin", doc: productionReadiness, want: "covenant.live-self-change-authority.v1"},
		{name: "production readiness docs RSI AO2 authority packet PR", doc: productionReadiness, want: "AO2 PR #201"},
		{name: "production readiness docs RSI authority packet not publish valid", doc: productionReadiness, want: "schema_valid_for_claim_publish=false"},
		{name: "production readiness docs RSI control-plane authority packet readback", doc: productionReadiness, want: "ao2.cp-ao2-rsi-authority-packet-readback.v1"},
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
		{name: "RSI health schema version", doc: rsiHealthSchema, want: "ao.command.rsi-health.v0.1"},
		{name: "RSI health schema claim levels", doc: rsiHealthSchema, want: "\"claim_levels\""},
		{name: "RSI health schema bounded claim", doc: rsiHealthSchema, want: "bounded_governed_rsi"},
		{name: "RSI health schema full claim", doc: rsiHealthSchema, want: "full_autonomous_self_mutating_rsi"},
		{name: "RSI health schema strict", doc: rsiHealthSchema, want: "\"additionalProperties\": false"},
		{name: "RSI health bundle schema version", doc: rsiHealthBundleSchema, want: "ao.command.rsi-health-bundle.v0.1"},
		{name: "RSI health bundle schema hashes", doc: rsiHealthBundleSchema, want: "\"sha256\""},
		{name: "RSI health bundle schema strict", doc: rsiHealthBundleSchema, want: "\"additionalProperties\": false"},
		{name: "RSI evidence chain smoke Foundry pulse", doc: rsiEvidenceChainSmoke, want: "foundry pulse run"},
		{name: "RSI evidence chain smoke Command health", doc: rsiEvidenceChainSmoke, want: "ao-command rsi health"},
		{name: "RSI evidence chain smoke Covenant claim boundary", doc: rsiEvidenceChainSmoke, want: "full-autonomous-self-mutating-rsi"},
		{name: "RSI evidence chain smoke read-only", doc: rsiEvidenceChainSmoke, want: "\"mutates_repositories\": false"},
		{name: "RSI evidence chain smoke claim levels", doc: rsiEvidenceChainSmoke, want: "\"claim_levels\""},
		{name: "RSI evidence chain smoke bounded claim", doc: rsiEvidenceChainSmoke, want: "\"claim\": \"bounded_governed_rsi\""},
		{name: "RSI evidence chain smoke full claim denied", doc: rsiEvidenceChainSmoke, want: "\"claim\": \"full_autonomous_self_mutating_rsi\""},
		{name: "RSI evidence chain smoke Forge aggregate proof", doc: rsiEvidenceChainSmoke, want: "bounded-rsi-improvement-chain-retention-proof.json"},
		{name: "RSI evidence chain smoke Covenant fixture", doc: rsiEvidenceChainSmoke, want: "examples/full-rsi-claim-boundary"},
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
		{name: "production readiness audit RSI evidence chain smoke", doc: productionReadinessAudit, want: "rsi_evidence_chain_smoke"},
		{name: "production readiness audit RSI claim manifest", doc: productionReadinessAudit, want: "rsi_claim_manifest"},
		{name: "production readiness audit RSI health contract", doc: productionReadinessAudit, want: "rsi_health_contract_validate"},
		{name: "production readiness audit RSI health bundle contract", doc: productionReadinessAudit, want: "rsi_health_bundle_contract_validate"},
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
		{name: "workflow RSI health step", doc: workflow, want: "RSI health dry-run"},
		{name: "workflow RSI health command", doc: workflow, want: "bin/ao-command rsi health"},
		{name: "workflow RSI health Foundry candidate", doc: workflow, want: "--foundry-candidate tmp/rsi-health/foundry-rsi-candidate.json"},
		{name: "workflow RSI health Foundry next task", doc: workflow, want: "--foundry-next-task tmp/rsi-health/foundry-rsi-next-improvement-task.json"},
		{name: "workflow RSI health Forge retained gate", doc: workflow, want: "--forge-retained-gate tmp/rsi-health/forge-retained-foundry-rsi-improvement-gate.json"},
		{name: "workflow RSI health Forge retained command", doc: workflow, want: "--forge-retained-command-health tmp/rsi-health/forge-retained-command-rsi-health.json"},
		{name: "workflow RSI health bundle", doc: workflow, want: "--bundle-out tmp/rsi-health/rsi-health-bundle.json"},
		{name: "workflow RSI health schema validation", doc: workflow, want: "Validate RSI health contract"},
		{name: "workflow RSI health bundle schema validation", doc: workflow, want: "Validate RSI health bundle contract"},
		{name: "workflow RSI evidence chain smoke", doc: workflow, want: "scripts/rsi-evidence-chain-smoke.sh --forge ao-forge --foundry ao-foundry --covenant ao-covenant"},
		{name: "workflow RSI claim manifest", doc: workflow, want: "RSI claim manifest"},
	} {
		if !strings.Contains(check.doc, check.want) {
			t.Fatalf("%s missing %q", check.name, check.want)
		}
	}
}

func TestDryRunCleanTreeAllowlistIncludesReadOnlyFixtureCheckouts(t *testing.T) {
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
		if !strings.Contains(string(content), "':!ao-covenant'") {
			t.Fatalf("%s clean-tree allowlist must include the read-only ao-covenant fixture checkout", script)
		}
		if !strings.Contains(string(content), "':!ao-architecture'") {
			t.Fatalf("%s clean-tree allowlist must include the read-only ao-architecture fixture checkout", script)
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

func writeAtlasStatusFixture(t *testing.T, schedulesWork bool) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "foundry-atlas-status.json")
	status := fmt.Sprintf(`{
  "schema_version": "ao.foundry.atlas-status.v0.1",
  "status": "ready",
  "mode": "fixture_only_readback",
  "registry_id": "atlas-demo-stack",
  "import_id": "atlas-readiness-workgraph-foundry-import",
  "workgraph_id": "atlas-readiness-workgraph",
  "target_instance": "demo-stack",
  "readback_status": "ready",
  "task_id": "atlas-readiness-task",
  "task_digest": "sha256:7a3df442c6a8268de6e7b963bb55759aa15039e724f3291b7bf902a37cd43d99",
  "run_link_digest": "sha256:bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb",
  "schedules_work": %t,
  "executes_work": false,
  "approves_work": false,
  "evidence": {
    "foundry": "evidence/foundry/atlas-readiness.json"
  },
  "next_actions": [
    "keep Atlas status as observer-only readback"
  ]
}`, schedulesWork)
	if err := os.WriteFile(path, []byte(status), 0o644); err != nil {
		t.Fatalf("write atlas status fixture: %v", err)
	}
	return path
}

type rsiHealthFixturePaths struct {
	arena                      string
	crucible                   string
	sentinel                   string
	promoter                   string
	foundry                    string
	foundryCandidate           string
	foundryNextTask            string
	forgeRetainedGate          string
	forgeRetainedCandidate     string
	forgeRetainedNextTask      string
	forgeRetainedCommandHealth string
}

func writeRSIHealthFixtures(t *testing.T, clear bool) rsiHealthFixturePaths {
	t.Helper()
	dir := t.TempDir()
	write := func(name string, body string) string {
		t.Helper()
		path := filepath.Join(dir, name)
		if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
			t.Fatalf("write %s: %v", name, err)
		}
		return path
	}
	quote := func(value string) string {
		t.Helper()
		data, err := json.Marshal(value)
		if err != nil {
			t.Fatalf("quote path %q: %v", value, err)
		}
		return string(data)
	}
	sentinelVerdict := `"clear"`
	sentinelSafety := `"passed"`
	sentinelRegression := `"passed"`
	sentinelHold := `false`
	if !clear {
		sentinelVerdict = `"incident"`
		sentinelSafety = `"failed"`
		sentinelRegression = `"failed"`
		sentinelHold = `true`
	}
	return rsiHealthFixturePaths{
		arena: write("arena-promotion-gate.json", `{
  "schema_version": "ao.arena.promotion-gate.v0.1",
  "suite_id": "ao-arena-v0.1",
  "status": "passed",
  "reasons": [],
  "winner": "ao-orchestration"
}`),
		crucible: write("crucible-hardening-gate.json", `{
  "schema_version": "ao.crucible.hardening-gate.v0.1",
  "gate_id": "hardening-gate",
  "status": "passed",
  "score": 97,
  "reasons": ["resilience score meets threshold"]
}`),
		sentinel: write("sentinel-verdict.json", `{
  "schema_version": "ao.sentinel.verdict.v0.1",
  "target_id": "local-ao-stack",
  "verdict": `+sentinelVerdict+`,
  "safety_status": `+sentinelSafety+`,
  "regression_status": `+sentinelRegression+`,
  "promoter_hold_required": `+sentinelHold+`,
  "mutates_live_state": false,
  "blockers": []
}`),
		promoter: write("promoter-gate.json", `{
  "schema_version": "ao.promoter.gate.v0.1",
  "target_stack_id": "ao-stack-local",
  "candidate_id": "ao-foundry",
  "status": "passed",
  "promotion_allowed": true,
  "activation_plan_allowed": true,
  "blockers": [],
  "gate_results": [
    {"role": "arena_promotion_gate", "status": "passed", "accepted_status": "passed", "passed": true},
    {"role": "crucible_hardening_gate", "status": "passed", "accepted_status": "passed", "passed": true}
  ]
}`),
		foundry: write("foundry-rsi-improvement-gate.json", `{
  "schema_version": "ao.foundry.rsi-improvement-gate.v0.1",
  "status": "passed",
  "baseline_score": 90,
  "candidate_score": 100,
  "required_improvement_percent": 5,
  "actual_improvement_percent": 10,
  "autonomous_claim": "measured_local_improvement",
  "mutates_repositories": false,
  "evidence": [
    {
      "label": "baseline",
      "path": "examples/evals/rsi-baseline.eval-result.json",
      "schema_version": "ao.foundry.eval-result.v0.1",
      "status": "ready",
      "score": 90,
      "max_score": 100,
      "sha256": "e5824cee9ef1455fcdc74dfecc7e30710edb5ef67cb939eff92d57283dfdc52e"
    },
    {
      "label": "candidate",
      "path": "tmp/pulse-rsi-verify/eval-result.json",
      "schema_version": "ao.foundry.eval-result.v0.1",
      "status": "ready",
      "score": 100,
      "max_score": 100,
      "sha256": "cf3f99d1b1639ef2fd2ba24cb75481211c0c4b0ad8e81605be5fbd6e3f7f39ec"
    }
  ]
}`),
		foundryCandidate: write("foundry-rsi-candidate.json", `{
  "schema_version": "ao.foundry.rsi-candidate.v0.1",
  "status": "ready",
  "generated_by": "foundry pulse run",
  "improvement_hypothesis": "Local pulse generated the candidate eval result from the current Foundry run before measuring the RSI improvement gate.",
  "baseline_eval_result": {
    "path": "examples/evals/rsi-baseline.eval-result.json",
    "schema_version": "ao.foundry.eval-result.v0.1",
    "status": "ready",
    "score": 90,
    "max_score": 100,
    "sha256": "e5824cee9ef1455fcdc74dfecc7e30710edb5ef67cb939eff92d57283dfdc52e"
  },
  "candidate_eval_result": {
    "path": "tmp/pulse-rsi-verify/eval-result.json",
    "schema_version": "ao.foundry.eval-result.v0.1",
    "status": "ready",
    "score": 100,
    "max_score": 100,
    "sha256": "cf3f99d1b1639ef2fd2ba24cb75481211c0c4b0ad8e81605be5fbd6e3f7f39ec"
  },
  "mutates_repositories": false,
  "next_actions": []
}`),
		foundryNextTask: write("foundry-rsi-next-improvement-task.json", `{
  "schema_version": "ao.foundry.rsi-next-improvement-task.v0.1",
  "status": "ready",
  "generated_by": "foundry pulse run",
  "goal_id": "ao-foundry-production-readiness",
  "recommended_task_id": "rsi-next-example",
  "recommended_action": "retain the next bounded RSI improvement task as governed evidence",
  "improvement_rationale": "The local pulse produced an RSI candidate and a passing improvement gate, so the next bounded task can be retained as governed evidence before delegation.",
  "candidate_evidence_path": `+quote(filepath.Join(dir, "foundry-rsi-candidate.json"))+`,
  "gate_evidence_path": `+quote(filepath.Join(dir, "foundry-rsi-improvement-gate.json"))+`,
  "required_improvement_percent": 5,
  "actual_improvement_percent": 10,
  "autonomous_claim": "derived_local_next_improvement",
  "mutates_repositories": false,
  "next_actions": [
    "retain rsi_next_improvement_task with RSI candidate and gate evidence"
  ]
}`),
		forgeRetainedGate: write("forge-retained-foundry-rsi-improvement-gate.json", `{
  "schema_version": "ao.forge.goal-run-retained-evidence.v0.1",
  "goal_id": "ao2-weekend-hardening",
  "iteration": "20260619T180000Z-verification",
  "phase": "verification",
  "summary": "Retained AO Foundry RSI improvement gate.",
  "captured_outputs": [
    {
      "label": "ao-foundry-rsi-improvement-gate",
      "command": "foundry pulse run",
      "schema_version": "ao.foundry.rsi-improvement-gate.v0.1",
      "status": "passed",
      "baseline_score": 90,
      "candidate_score": 100,
      "required_improvement_percent": 5,
      "actual_improvement_percent": 10,
      "autonomous_claim": "measured_local_improvement",
      "mutates_repositories": false
    }
  ],
  "retention_policy": {
    "layout": "docs/evidence/goals/<goal_id>/<YYYYMMDDTHHMMSSZ>-<phase>/",
    "temporary_paths_allowed": false,
    "minimum_retention_days_after_terminal_phase": 90
  },
  "retention_metadata": {
    "retained_at": "2026-06-19T18:00:00Z",
    "retention_class": "loop_evidence",
    "retain_while_goal_active": true,
    "deletion_requires_review": true,
    "cleanup_change_must_name": ["goal_id", "iteration", "reason"]
  }
}`),
		forgeRetainedCandidate: write("forge-retained-foundry-rsi-candidate.json", `{
  "schema_version": "ao.forge.goal-run-retained-evidence.v0.1",
  "goal_id": "ao2-weekend-hardening",
  "iteration": "20260619T180000Z-verification",
  "phase": "verification",
  "summary": "Retained AO Foundry RSI candidate evidence.",
  "captured_outputs": [
    {
      "label": "ao-foundry-rsi-candidate",
      "command": "foundry pulse run",
      "schema_version": "ao.foundry.rsi-candidate.v0.1",
      "status": "ready",
      "generated_by": "foundry pulse run",
      "baseline_score": 90,
      "candidate_score": 100,
      "mutates_repositories": false
    }
  ],
  "retention_policy": {
    "layout": "docs/evidence/goals/<goal_id>/<YYYYMMDDTHHMMSSZ>-<phase>/",
    "temporary_paths_allowed": false,
    "minimum_retention_days_after_terminal_phase": 90
  },
  "retention_metadata": {
    "retained_at": "2026-06-19T18:00:00Z",
    "retention_class": "loop_evidence",
    "retain_while_goal_active": true,
    "deletion_requires_review": true,
    "cleanup_change_must_name": ["goal_id", "iteration", "reason"]
  }
}`),
		forgeRetainedNextTask: write("forge-retained-foundry-rsi-next-improvement-task.json", `{
  "schema_version": "ao.forge.goal-run-retained-evidence.v0.1",
  "goal_id": "ao2-weekend-hardening",
  "iteration": "20260619T180000Z-verification",
  "phase": "verification",
  "summary": "Retained AO Foundry RSI next improvement task.",
  "captured_outputs": [
    {
      "label": "ao-foundry-rsi-next-improvement-task",
      "command": "foundry pulse run",
      "schema_version": "ao.foundry.rsi-next-improvement-task.v0.1",
      "status": "ready",
      "required_improvement_percent": 5,
      "actual_improvement_percent": 10,
      "autonomous_claim": "derived_local_next_improvement",
      "mutates_repositories": false
    }
  ],
  "retention_policy": {
    "layout": "docs/evidence/goals/<goal_id>/<YYYYMMDDTHHMMSSZ>-<phase>/",
    "temporary_paths_allowed": false,
    "minimum_retention_days_after_terminal_phase": 90
  },
  "retention_metadata": {
    "retained_at": "2026-06-19T18:00:00Z",
    "retention_class": "loop_evidence",
    "retain_while_goal_active": true,
    "deletion_requires_review": true,
    "cleanup_change_must_name": ["goal_id", "iteration", "reason"]
  }
}`),
		forgeRetainedCommandHealth: write("forge-retained-command-rsi-health.json", `{
  "schema_version": "ao.forge.goal-run-retained-evidence.v0.1",
  "goal_id": "ao2-weekend-hardening",
  "iteration": "20260619T180000Z-verification",
  "phase": "verification",
  "summary": "Retained AO Command RSI health output.",
  "captured_outputs": [
    {
      "label": "ao-command-rsi-health",
      "command": "ao-command rsi health",
      "status": "passed",
      "rsi_mode": "governed_fixture_local",
      "rsi_capability": "demonstrated_local_fixture_loop",
      "operator_mode": "read_only",
      "mutates_repositories": false,
      "families": [
        {"family": "ao-arena", "status": "passed", "passed": true},
        {"family": "ao-crucible", "status": "passed", "passed": true},
        {"family": "ao-sentinel", "status": "clear", "passed": true},
        {"family": "ao-promoter", "status": "passed", "passed": true},
        {"family": "ao-foundry", "status": "passed", "passed": true}
      ]
    }
  ],
  "retention_policy": {
    "layout": "docs/evidence/goals/<goal_id>/<YYYYMMDDTHHMMSSZ>-<phase>/",
    "temporary_paths_allowed": false,
    "minimum_retention_days_after_terminal_phase": 90
  },
  "retention_metadata": {
    "retained_at": "2026-06-19T18:00:00Z",
    "retention_class": "loop_evidence",
    "retain_while_goal_active": true,
    "deletion_requires_review": true,
    "cleanup_change_must_name": ["goal_id", "iteration", "reason"]
  }
}`),
	}
}

func writeRSIManifestFixture(t *testing.T, includeDeniedFullClaim bool) string {
	t.Helper()
	return writeRSIManifestFixtureWithPins(t, includeDeniedFullClaim, true, true, true, true, true, true, true, true, true)
}

func writeRSIManifestFixtureWithReadbacks(t *testing.T, includeDeniedFullClaim bool, includeClaimReadinessReadback bool, includeSelfChangeReadback bool, includeRollbackRehearsalReadback bool) string {
	t.Helper()
	return writeRSIManifestFixtureWithPins(t, includeDeniedFullClaim, includeClaimReadinessReadback, includeSelfChangeReadback, includeRollbackRehearsalReadback, true, true, true, true, true, true)
}

func writeRSIManifestFixtureWithPins(t *testing.T, includeDeniedFullClaim bool, includeClaimReadinessReadback bool, includeSelfChangeReadback bool, includeRollbackRehearsalReadback bool, includeForgeManifestRetentionPin bool, includeForgeArchitectureReadbackPin bool, includeCovenantRetainedRollbackBoundaryPin bool, includeCovenantLiveSelfChangeAuthorityPacketPin bool, includeAO2AuthorityPacketPin bool, includeAO2ControlPlaneAuthorityPacketReadbackPin bool) string {
	t.Helper()
	fullClaim := ""
	if includeDeniedFullClaim {
		fullClaim = `,
    {
      "claim_level": "full_autonomous_self_mutating_rsi",
      "decision": "denied",
      "status": "missing_required_full_claim_evidence",
      "required_before_allowed": [
        "covenant_claim_publish_approval",
        "mutation_authority",
        "rollback_evidence",
        "live_self_change_evidence",
        "observer_readback",
        "updated_retained_claim_level_evidence"
      ]
    }`
	}
	requiredEvidence := ""
	if includeClaimReadinessReadback {
		requiredEvidence += `,
        "ao2_control_plane_rsi_claim_readiness_readback"`
	}
	if includeSelfChangeReadback {
		requiredEvidence += `,
        "ao2_rsi_self_change_dry_run",
        "ao2_control_plane_rsi_self_change_dry_run_readback"`
	}
	if includeAO2AuthorityPacketPin {
		requiredEvidence += `,
        "ao2_rsi_authority_packet_dry_run_candidate"`
	}
	if includeAO2ControlPlaneAuthorityPacketReadbackPin {
		requiredEvidence += `,
        "ao2_control_plane_rsi_authority_packet_readback"`
	}
	if includeForgeArchitectureReadbackPin {
		requiredEvidence += `,
        "ao_forge_architecture_rsi_pin_readback"`
	}
	ao2Repo := `{"id": "ao2", "role": "governed_execution_and_evidence_runtime"}`
	if includeSelfChangeReadback {
		rollbackRehearsalEvidence := ""
		rollbackRehearsalPR := ""
		rollbackRehearsalOutput := ""
		if includeRollbackRehearsalReadback {
			rollbackRehearsalEvidence = `,
	        "target/rsi-self-change-dry-run/latest/rollback-rehearsal/worktree/scripts/rsi-claim-readiness-audit.sh",
	        "rollback_rehearsal.status=passed"`
			rollbackRehearsalPR = `,
	        {
	          "number": 200,
	          "title": "Add RSI rollback rehearsal evidence",
	          "url": "https://github.com/uesugitorachiyo/ao2/pull/200",
	          "merge_commit": "6c5d383c78362168fe50851bb6063a63114f1cee"
	        }`
			rollbackRehearsalOutput = `,
	        "rollback_rehearsal=passed"`
		}
		authorityPacketEvidence := ""
		authorityPacketPR := ""
		authorityPacketOutput := ""
		if includeAO2AuthorityPacketPin {
			authorityPacketEvidence = `,
	        "target/rsi-self-change-dry-run/latest/live-self-change-authority.packet.json",
	        "covenant.live-self-change-authority.v1",
	        "mutation_authority_packet.mode=dry_run_candidate",
	        "mutation_authority_packet.schema_valid_for_claim_publish=false"`
			authorityPacketPR = `,
	        {
	          "number": 201,
	          "title": "Emit RSI authority packet dry-run evidence",
	          "url": "https://github.com/uesugitorachiyo/ao2/pull/201",
	          "merge_commit": "8b232431bbeb007330ebf1acfb025b2a73ba98d3"
	        }`
			authorityPacketOutput = `,
	        "mutation_authority_packet=dry_run_candidate",
	        "schema_valid_for_claim_publish=false"`
		}
		ao2Repo = `{
	      "id": "ao2",
	      "role": "governed_execution_and_evidence_runtime",
      "evidence": [
        "scripts/rsi-claim-readiness-audit.sh",
        "scripts/rsi-governed-self-change-dry-run.sh",
        "tests/test_ao2_rsi_claim_readiness.py",
        "tests/test_ao2_rsi_governed_self_change_dry_run.py",
        "target/rsi-claim-readiness/latest/summary.json",
        "target/rsi-self-change-dry-run/latest/summary.json",
	        "target/rsi-self-change-dry-run/latest/proposed-self-change.patch",
	        "target/rsi-self-change-dry-run/latest/rollback-self-change.patch",
	        "ao2.rsi-claim-readiness-audit.v1",
	        "ao2.rsi-governed-self-change-dry-run.v1"` + authorityPacketEvidence + rollbackRehearsalEvidence + `
	      ],
	      "known_prs": [
	        {
          "number": 198,
          "title": "Add AO2 RSI claim readiness audit",
          "url": "https://github.com/uesugitorachiyo/ao2/pull/198",
          "merge_commit": "af86093758b13303402b825bf3b5849da88cf501"
        },
        {
          "number": 199,
	          "title": "Add AO2 RSI self-change dry-run evidence",
	          "url": "https://github.com/uesugitorachiyo/ao2/pull/199",
	          "merge_commit": "204078604b8bb52b606ed2bf35ff5c5dd9654b21"
	        }` + rollbackRehearsalPR + authorityPacketPR + `
	      ],
	      "claim_output": [
	        "self_change_dry_run=passed"` + rollbackRehearsalOutput + authorityPacketOutput + `,
	        "claim_level=bounded_governed_rsi decision=allowed",
	        "claim_level=full_autonomous_self_mutating_rsi decision=denied"
	      ],
      "boundary": "execution_and_evidence_mechanics_only_for_current_rsi_claim"
    }`
	}
	ao2ControlPlaneRepo := `{"id": "ao2-control-plane", "role": "read_only_observer_readback"}`
	if includeClaimReadinessReadback || includeSelfChangeReadback {
		extraReadbackEvidence := ""
		extraKnownPR := ""
		extraClaimOutput := ""
		boundary := "observer_only_no_claim_approval_no_repository_mutation"
		if includeSelfChangeReadback {
			rollbackReadbackEvidence := ""
			rollbackReadbackPR := ""
			if includeRollbackRehearsalReadback {
				rollbackReadbackEvidence = `,
	        "rollback_rehearsal.status=passed"`
				rollbackReadbackPR = `,
	        {
	          "number": 72,
	          "title": "Require AO2 RSI rollback rehearsal readback",
	          "url": "https://github.com/uesugitorachiyo/ao2-control-plane/pull/72",
	          "merge_commit": "3f81bba3a897101e2a56ba76de9a59a7d027f464"
	        }`
			}
			extraReadbackEvidence = `,
	        "scripts/verify_ao2_rsi_self_change_dry_run.py",
	        "tests/test_ao2_rsi_self_change_dry_run_readback.py",
	        "target/ao2-rsi-self-change-dry-run-readback/summary.json",
	        "ao2.cp-ao2-rsi-self-change-dry-run-readback.v1"` + rollbackReadbackEvidence
			extraKnownPR = `,
	        {
	          "number": 71,
	          "title": "Add AO2 RSI self-change dry-run readback",
	          "url": "https://github.com/uesugitorachiyo/ao2-control-plane/pull/71",
	          "merge_commit": "9a54ac1ffad95080a92a82096a90c1b7bc9c1700"
	        }` + rollbackReadbackPR
			extraClaimOutput = `,
	        "control_plane_ao2_rsi_self_change_dry_run_readback=passed"`
			boundary = "observer_only_no_claim_approval_no_patch_application_no_repository_mutation"
		}
		if includeAO2ControlPlaneAuthorityPacketReadbackPin {
			extraReadbackEvidence += `,
	        "scripts/verify_ao2_rsi_authority_packet.py",
	        "tests/test_ao2_rsi_authority_packet_readback.py",
	        "target/ao2-rsi-authority-packet-readback/summary.json",
	        "ao2.cp-ao2-rsi-authority-packet-readback.v1",
	        "covenant.live-self-change-authority.v1",
	        "live-self-change-authority.packet.json",
	        "schema_valid_for_claim_publish=false"`
			extraKnownPR += `,
	        {
	          "number": 73,
	          "title": "Add AO2 RSI authority packet readback",
	          "url": "https://github.com/uesugitorachiyo/ao2-control-plane/pull/73",
	          "merge_commit": "6b83330c8a673b2bf210818c080ba4361062cf8f"
	        }`
			extraClaimOutput += `,
	        "control_plane_ao2_rsi_authority_packet_readback=passed"`
			boundary = "observer_only_no_claim_approval_no_patch_application_no_repository_mutation_no_claim_publish"
		}
		ao2ControlPlaneRepo = `{
      "id": "ao2-control-plane",
      "role": "read_only_observer_readback",
      "evidence": [
        "scripts/verify_ao2_rsi_claim_readiness.py",
        "tests/test_ao2_rsi_claim_readiness_readback.py",
        "target/ao2-rsi-claim-readiness-readback/summary.json",
        "ao2.cp-ao2-rsi-claim-readiness-readback.v1"` + extraReadbackEvidence + `
      ],
      "known_prs": [
        {
          "number": 70,
          "title": "Add AO2 RSI claim readiness readback",
          "url": "https://github.com/uesugitorachiyo/ao2-control-plane/pull/70",
          "merge_commit": "1f80530ca8430f810fbd2c7f21daa70076c689a0"
        }` + extraKnownPR + `
      ],
      "claim_output": [
        "control_plane_ao2_rsi_claim_readiness_readback=passed"` + extraClaimOutput + `,
        "claim_level=bounded_governed_rsi decision=allowed",
        "claim_level=full_autonomous_self_mutating_rsi decision=denied"
      ],
      "boundary": "` + boundary + `"
    }`
	}
	aoForgeEvidence := `
        "docs/contracts/goal-run-retained-evidence-v0.1.schema.json",
        "docs/evidence/goals/ao2-weekend-hardening/20260619T180000Z-verification/ao-command-rsi-health-retention-proof.json",
        "docs/evidence/goals/ao2-weekend-hardening/20260619T180000Z-verification/bounded-rsi-improvement-chain-retention-proof.json"`
	aoForgePRs := `
        {
          "number": 142,
          "title": "Retain RSI claim levels",
          "url": "https://github.com/uesugitorachiyo/ao-forge/pull/142",
          "merge_commit": "037f505a30bcff2536175b76021cfdd5f5f5a562"
        }`
	if includeForgeManifestRetentionPin {
		aoForgeEvidence += `,
        "docs/evidence/goals/ao2-weekend-hardening/20260619T180000Z-verification/ao-command-rsi-manifest-retention-proof.json",
        "ao-command-rsi-manifest",
        "rollback_rehearsal.status=passed"`
		aoForgePRs += `,
        {
          "number": 143,
          "title": "Retain AO Command RSI manifest evidence",
          "url": "https://github.com/uesugitorachiyo/ao-forge/pull/143",
          "merge_commit": "966a3022c66635ab03b0029cd6cf68aefccd11b4"
        }`
	}
	if includeForgeArchitectureReadbackPin {
		aoForgeEvidence += `,
        "docs/contracts/architecture-rsi-pin-readback-v0.1.schema.json",
        "docs/evidence/architecture/ao-architecture-rsi-pin-readback.json",
        "goalrun.architecture_rsi_pin_readback"`
		aoForgePRs += `,
        {
          "number": 144,
          "title": "Require architecture RSI pin readback readiness",
          "url": "https://github.com/uesugitorachiyo/ao-forge/pull/144",
          "merge_commit": "c196384c854448ee327f8ce4dbeb346c84ab649a"
        }`
	}
	aoForgeRepo := `{
      "id": "ao-forge",
      "role": "goalrun_retention_and_aggregate_proof",
      "evidence": [` + aoForgeEvidence + `
      ],
      "known_prs": [` + aoForgePRs + `
      ]
    }`
	aoCovenantEvidence := `
        "examples/full-rsi-claim-boundary/denied.contract.json",
        "examples/full-rsi-claim-boundary/evidence-approved.contract.json",
        "internal/policy/evaluator.go",
        "internal/policy/explain.go",
        "internal/policy/spine.go"`
	aoCovenantPRs := `
        {
          "number": 55,
          "title": "Add full RSI claim boundary fixtures",
          "url": "https://github.com/uesugitorachiyo/ao-covenant/pull/55",
          "merge_commit": "c5ff915d65b6159bc64df88805b52959052fd397"
        },
        {
          "number": 56,
          "title": "Define RSI claim level vocabulary",
          "url": "https://github.com/uesugitorachiyo/ao-covenant/pull/56",
          "merge_commit": "60f5b4a45c0b420c9224075edd258170a549416d"
        }`
	if includeCovenantRetainedRollbackBoundaryPin {
		aoCovenantEvidence += `,
        "examples/full-rsi-claim-boundary/rollback-retained.contract.json",
        "examples/full-rsi-claim-boundary/rollback-retained-ticket.json",
        "retained rollback rehearsal alone is insufficient"`
		aoCovenantPRs += `,
        {
          "number": 57,
          "title": "Deny full RSI with retained rollback only",
          "url": "https://github.com/uesugitorachiyo/ao-covenant/pull/57",
          "merge_commit": "3a47e3845e0a0c6a2db196a00e339de425cc6c6c"
        }`
	}
	if includeCovenantLiveSelfChangeAuthorityPacketPin {
		aoCovenantEvidence += `,
        "examples/full-rsi-claim-boundary/live-self-change-authority.packet.json",
        "schemas/covenant.live-self-change-authority.v1.schema.json",
        "covenant.live-self-change-authority.v1"`
		aoCovenantPRs += `,
        {
          "number": 58,
          "title": "Add live self-change authority packet schema",
          "url": "https://github.com/uesugitorachiyo/ao-covenant/pull/58",
          "merge_commit": "2606a00a6643c99fe46775d8b6238d5915a49206"
        }`
	}
	aoCovenantRepo := `{
      "id": "ao-covenant",
      "role": "claim_publication_policy_gate",
      "evidence": [` + aoCovenantEvidence + `
      ],
      "known_prs": [` + aoCovenantPRs + `
      ]
    }`
	path := filepath.Join(t.TempDir(), "rsi-claim-evidence-manifest.json")
	manifest := `{
  "schema_version": "ao.architecture.rsi-claim-evidence-manifest.v0.1",
  "generated_date": "2026-06-25",
  "claim_levels": [
    {
      "claim_level": "bounded_governed_rsi",
      "decision": "allowed",
      "status": "supported_when_chain_passes",
      "required_evidence": [
        "ao_foundry_rsi_candidate_evidence",
        "ao_foundry_rsi_improvement_gate",
        "ao_foundry_rsi_next_improvement_task",
        "ao_forge_retained_foundry_evidence",
        "ao_command_rsi_health",
        "ao_covenant_full_claim_boundary_denial"` + requiredEvidence + `
      ]
    }` + fullClaim + `
  ],
  "active_repositories": [
    {"id": "ao-foundry", "role": "pulse_candidate_and_improvement_gate_producer"},
    ` + aoForgeRepo + `,
    {"id": "ao-command", "role": "read_only_rsi_health_verifier"},
    ` + aoCovenantRepo + `,
    ` + ao2Repo + `,
    ` + ao2ControlPlaneRepo + `
  ],
  "deprecated_or_out_of_scope_repositories": [
    {"id": "ao-operator", "status": "deprecated", "replacement": "ao2", "rsi_evidence_scope": "not_active_rsi_evidence"},
    {"id": "ao-runtime", "status": "deprecated", "replacement": "ao2", "rsi_evidence_scope": "not_active_rsi_evidence"},
    {"id": "ao-control-plane", "status": "deprecated", "replacement": "ao2-control-plane", "rsi_evidence_scope": "not_active_rsi_evidence"},
    {"id": "ao-conductor", "status": "out_of_scope", "replacement": null, "rsi_evidence_scope": "not_active_rsi_evidence"},
    {"id": "agy-swarms", "status": "out_of_scope", "replacement": null, "rsi_evidence_scope": "not_active_rsi_evidence"}
  ],
  "full_claim_required_evidence": [
    "covenant-approved claim.publish ticket for full-autonomous-self-mutating-rsi",
    "mutation authority packet using covenant.live-self-change-authority.v1 naming repository, branch, allowed write surface, exact digest, approval identity, and expiry",
    "rollback evidence for the same change class",
    "live self-change evidence over an active planning, execution, policy, or verification path",
    "observer readback that preserves observer-only authority",
    "AO Command and AO Forge retained evidence with updated claim-level decisions"
  ]
}`
	if err := os.WriteFile(path, []byte(manifest), 0o644); err != nil {
		t.Fatalf("write rsi manifest fixture: %v", err)
	}
	return path
}

func writeRSIManifestFixtureMissingAO2ControlPlaneReadback(t *testing.T) string {
	t.Helper()
	return writeRSIManifestFixtureWithReadbacks(t, true, false, true, true)
}

func writeRSIManifestFixtureMissingAO2SelfChangeDryRunReadback(t *testing.T) string {
	t.Helper()
	return writeRSIManifestFixtureWithReadbacks(t, true, true, false, true)
}

func writeRSIManifestFixtureMissingAO2RollbackRehearsalReadback(t *testing.T) string {
	t.Helper()
	return writeRSIManifestFixtureWithReadbacks(t, true, true, true, false)
}

func writeRSIManifestFixtureMissingForgeManifestRetentionPin(t *testing.T) string {
	t.Helper()
	return writeRSIManifestFixtureWithPins(t, true, true, true, true, false, true, true, true, true, true)
}

func writeRSIManifestFixtureMissingForgeArchitectureReadbackPin(t *testing.T) string {
	t.Helper()
	return writeRSIManifestFixtureWithPins(t, true, true, true, true, true, false, true, true, true, true)
}

func writeRSIManifestFixtureMissingCovenantRetainedRollbackBoundaryPin(t *testing.T) string {
	t.Helper()
	return writeRSIManifestFixtureWithPins(t, true, true, true, true, true, true, false, true, true, true)
}

func writeRSIManifestFixtureMissingCovenantLiveSelfChangeAuthorityPacketPin(t *testing.T) string {
	t.Helper()
	return writeRSIManifestFixtureWithPins(t, true, true, true, true, true, true, true, false, true, true)
}

func writeRSIManifestFixtureMissingAO2AuthorityPacketReadbackPins(t *testing.T) string {
	t.Helper()
	path := writeRSIManifestFixture(t, true)
	bytes, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read rsi manifest fixture: %v", err)
	}
	manifest := strings.ReplaceAll(string(bytes), `"number": 201`, `"number": 202`)
	manifest = strings.ReplaceAll(manifest, `"number": 73`, `"number": 74`)
	if err := os.WriteFile(path, []byte(manifest), 0o644); err != nil {
		t.Fatalf("write rsi manifest fixture without AO2 authority packet pins: %v", err)
	}
	return path
}

func writeRSIManifestFixtureMissingAO2AuthorityPacketRequiredEvidence(t *testing.T) string {
	t.Helper()
	return writeRSIManifestFixtureWithPins(t, true, true, true, true, true, true, true, true, false, false)
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
