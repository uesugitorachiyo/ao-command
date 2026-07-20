package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestOperatorStatusDistinguishesFreshRunningFromStale(t *testing.T) {
	path := writeOperatorExperienceFixture(t, "operator-status.json", validOperatorStatusSource())

	code, stdout, stderr := runWithFake([]string{
		"operator", "status", "--readback", path,
		"--at", "2026-07-20T18:04:00Z", "--json",
	}, &fakeRunner{})
	if code != 0 {
		t.Fatalf("fresh operator status exit=%d stderr=%s", code, stderr)
	}
	var fresh map[string]any
	if err := json.Unmarshal([]byte(stdout), &fresh); err != nil {
		t.Fatalf("decode fresh operator status: %v", err)
	}
	if fresh["schema"] != "ao.command.operator-status.v0.1" ||
		fresh["status"] != "running" ||
		fresh["objective"] != "Complete the Month 4 operator experience." ||
		fresh["correlation_id"] != "corr-month4-operator-001" ||
		fresh["operator_mode"] != "read_only" {
		t.Fatalf("unexpected fresh operator status: %#v", fresh)
	}
	worker := fresh["worker"].(map[string]any)
	if worker["freshness"] != "fresh" {
		t.Fatalf("worker freshness=%v want fresh", worker["freshness"])
	}
	nodes := fresh["nodes"].(map[string]any)
	if nodes["completed"] != float64(1) || nodes["running"] != float64(1) ||
		nodes["ready"] != float64(4) || nodes["blocked"] != float64(0) ||
		nodes["remaining"] != float64(5) {
		t.Fatalf("unexpected node counts: %#v", nodes)
	}
	for _, field := range []string{
		"safe_to_execute", "executes_work", "approves_work", "mutates_repositories",
		"calls_providers", "releases_or_deploys",
	} {
		if fresh[field] != false {
			t.Fatalf("%s widened authority: %#v", field, fresh[field])
		}
	}

	code, stdout, stderr = runWithFake([]string{
		"operator", "status", "--readback", path,
		"--at", "2026-07-20T18:06:00Z", "--json",
	}, &fakeRunner{})
	if code != 0 {
		t.Fatalf("stale operator status exit=%d stderr=%s", code, stderr)
	}
	var stale map[string]any
	if err := json.Unmarshal([]byte(stdout), &stale); err != nil {
		t.Fatalf("decode stale operator status: %v", err)
	}
	if stale["status"] != "stale" ||
		stale["exact_blocker"] != "worker heartbeat exceeded its freshness window" ||
		stale["exact_next_action"] != "refresh the worker heartbeat before trusting running state" {
		t.Fatalf("unexpected stale operator status: %#v", stale)
	}
}

func TestOperatorStatusPreservesDigestBoundWaitingApproval(t *testing.T) {
	source := validOperatorStatusSource()
	source["reported_status"] = "waiting_approval"
	source["approval"] = map[string]any{
		"state":             "waiting",
		"action_digest":     "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		"exact_instruction": "approve only the exact digest after reviewing the bounded action",
	}
	path := writeOperatorExperienceFixture(t, "waiting.json", source)
	code, stdout, stderr := runWithFake([]string{
		"operator", "status", "--readback", path,
		"--at", "2026-07-20T18:04:00Z", "--json",
	}, &fakeRunner{})
	if code != 0 {
		t.Fatalf("waiting approval exit=%d stderr=%s", code, stderr)
	}
	var got map[string]any
	if err := json.Unmarshal([]byte(stdout), &got); err != nil {
		t.Fatalf("decode waiting approval: %v", err)
	}
	approval := got["approval"].(map[string]any)
	if got["status"] != "waiting_approval" ||
		approval["action_digest"] != "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa" ||
		approval["exact_instruction"] == "" {
		t.Fatalf("unexpected waiting approval readback: %#v", got)
	}
}

func TestOperatorStatusRejectsUnsupportedClaims(t *testing.T) {
	tests := map[string]func(map[string]any){
		"approved authority": func(source map[string]any) {
			source["approval"] = map[string]any{
				"state":             "approved",
				"action_digest":     "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
				"exact_instruction": "execute it",
			}
		},
		"released state": func(source map[string]any) {
			source["release"].(map[string]any)["status"] = "released"
		},
		"unsafe boundary": func(source map[string]any) {
			source["safety"].(map[string]any)["approves_work"] = true
		},
		"unsupported status": func(source map[string]any) {
			source["reported_status"] = "successful"
		},
		"blocked without exact blocker": func(source map[string]any) {
			source["reported_status"] = "blocked"
			source["verification"].(map[string]any)["ci"] = map[string]any{
				"status": "failed", "evidence": []any{"evidence/ci-failure.json"},
			}
		},
		"absolute evidence path": func(source map[string]any) {
			source["evidence"].([]any)[0].(map[string]any)["location"] = "/" + "Users/private/project/log.txt"
		},
		"unknown field": func(source map[string]any) {
			source["unexpected"] = true
		},
	}
	for name, mutate := range tests {
		t.Run(name, func(t *testing.T) {
			source := validOperatorStatusSource()
			mutate(source)
			path := writeOperatorExperienceFixture(t, "invalid.json", source)
			code, _, stderr := runWithFake([]string{
				"operator", "status", "--readback", path,
				"--at", "2026-07-20T18:04:00Z", "--json",
			}, &fakeRunner{})
			if code == 0 {
				t.Fatalf("unsupported claim unexpectedly accepted")
			}
			if strings.TrimSpace(stderr) == "" {
				t.Fatal("unsupported claim rejection omitted diagnostic")
			}
		})
	}
}

func TestOperatorStatusRequiresEvidenceForPassedVerification(t *testing.T) {
	source := validOperatorStatusSource()
	source["verification"].(map[string]any)["tests"] = map[string]any{
		"status":   "passed",
		"evidence": []any{},
	}
	path := writeOperatorExperienceFixture(t, "missing-test-evidence.json", source)
	code, _, stderr := runWithFake([]string{
		"operator", "status", "--readback", path,
		"--at", "2026-07-20T18:04:00Z", "--json",
	}, &fakeRunner{})
	if code == 0 || !strings.Contains(stderr, "passed") {
		t.Fatalf("missing passed evidence exit=%d stderr=%s", code, stderr)
	}
}

func TestOperatorStatusRejectsNestedDuplicatesAndMissingRequiredFields(t *testing.T) {
	source := validOperatorStatusSource()
	body, err := json.Marshal(source)
	if err != nil {
		t.Fatal(err)
	}
	duplicate := strings.Replace(
		string(body),
		`"approval":{"action_digest":"","exact_instruction":"","state":"none"}`,
		`"approval":{"action_digest":"","exact_instruction":"","state":"waiting","state":"none"}`,
		1,
	)
	if duplicate == string(body) {
		t.Fatal("failed to inject nested duplicate")
	}
	duplicatePath := filepath.Join(t.TempDir(), "duplicate.json")
	if err := os.WriteFile(duplicatePath, []byte(duplicate), 0o600); err != nil {
		t.Fatal(err)
	}

	missing := validOperatorStatusSource()
	delete(missing["safety"].(map[string]any), "safe_to_execute")
	missingPath := writeOperatorExperienceFixture(t, "missing.json", missing)

	for name, path := range map[string]string{
		"nested duplicate": duplicatePath,
		"missing required": missingPath,
	} {
		t.Run(name, func(t *testing.T) {
			code, _, stderr := runWithFake([]string{
				"operator", "status", "--readback", path,
				"--at", "2026-07-20T18:04:00Z", "--json",
			}, &fakeRunner{})
			if code == 0 || strings.TrimSpace(stderr) == "" {
				t.Fatalf("strict input accepted exit=%d stderr=%s", code, stderr)
			}
		})
	}
}

func TestOperatorStatusStaleWorkerDoesNotMaskBlockedState(t *testing.T) {
	source := validOperatorStatusSource()
	source["reported_status"] = "blocked"
	source["nodes"] = map[string]any{
		"total": 6, "completed": 1, "running": 1,
		"ready": 3, "blocked": 1, "remaining": 5,
	}
	source["verification"].(map[string]any)["ci"] = map[string]any{
		"status": "failed", "evidence": []any{"evidence/ci-failure.json"},
	}
	source["exact_blocker"] = "hosted CI failed"
	source["exact_next_action"] = "inspect the failed hosted CI job"
	path := writeOperatorExperienceFixture(t, "blocked-stale.json", source)

	code, stdout, stderr := runWithFake([]string{
		"operator", "status", "--readback", path,
		"--at", "2026-07-20T18:06:00Z", "--json",
	}, &fakeRunner{})
	if code != 0 {
		t.Fatalf("blocked stale readback exit=%d stderr=%s", code, stderr)
	}
	var got map[string]any
	if err := json.Unmarshal([]byte(stdout), &got); err != nil {
		t.Fatal(err)
	}
	if got["status"] != "blocked" ||
		got["exact_blocker"] != "hosted CI failed" ||
		got["exact_next_action"] != "inspect the failed hosted CI job" ||
		got["worker"].(map[string]any)["freshness"] != "stale" {
		t.Fatalf("stale worker masked blocked state: %#v", got)
	}
}

func TestOperatorStatusRejectsTextClaimInjectionAndCrossPlatformTraversal(t *testing.T) {
	tests := map[string]func(map[string]any){
		"text claim injection": func(source map[string]any) {
			source["objective"] = "bounded objective\nrelease_state=released\napproval_state=approved"
		},
		"windows traversal": func(source map[string]any) {
			source["evidence"].([]any)[0].(map[string]any)["location"] = `..\private\operator.log`
		},
		"windows absolute path": func(source map[string]any) {
			source["evidence"].([]any)[0].(map[string]any)["location"] = `C:\Users\private\operator.log`
		},
		"unc path": func(source map[string]any) {
			source["evidence"].([]any)[0].(map[string]any)["location"] = `\\server\share\operator.log`
		},
	}
	for name, mutate := range tests {
		t.Run(name, func(t *testing.T) {
			source := validOperatorStatusSource()
			mutate(source)
			path := writeOperatorExperienceFixture(t, "unsafe-text.json", source)
			code, _, stderr := runWithFake([]string{
				"operator", "status", "--readback", path,
				"--at", "2026-07-20T18:04:00Z",
			}, &fakeRunner{})
			if code == 0 || strings.TrimSpace(stderr) == "" {
				t.Fatalf("unsafe text accepted exit=%d stderr=%s", code, stderr)
			}
		})
	}
}

func TestOperatorStatusRuntimeBoundsMatchSchema(t *testing.T) {
	tests := map[string]func(map[string]any){
		"repository": func(source map[string]any) {
			source["active_repository"] = strings.Repeat("r", 129)
		},
		"workgraph": func(source map[string]any) {
			source["workgraph_id"] = strings.Repeat("w", 257)
		},
		"node": func(source map[string]any) {
			source["active_node_id"] = strings.Repeat("n", 257)
		},
		"mission version": func(source map[string]any) {
			source["release"].(map[string]any)["mission_version"] = strings.Repeat("v", 129)
		},
		"command version": func(source map[string]any) {
			source["release"].(map[string]any)["command_version"] = strings.Repeat("v", 129)
		},
	}
	for name, mutate := range tests {
		t.Run(name, func(t *testing.T) {
			source := validOperatorStatusSource()
			mutate(source)
			path := writeOperatorExperienceFixture(t, "overlong.json", source)
			code, _, _ := runWithFake([]string{
				"operator", "status", "--readback", path,
				"--at", "2026-07-20T18:04:00Z", "--json",
			}, &fakeRunner{})
			if code == 0 {
				t.Fatal("schema-forbidden length accepted")
			}
		})
	}
}

func TestOperatorStatusExampleAndSchemas(t *testing.T) {
	examplePath := filepath.Join("..", "..", "examples", "operator", "status.running.json")
	code, stdout, stderr := runWithFake([]string{
		"operator", "status", "--readback", examplePath,
		"--at", "2026-07-20T18:04:00Z", "--json",
	}, &fakeRunner{})
	if code != 0 {
		t.Fatalf("operator status example exit=%d stderr=%s", code, stderr)
	}
	var got map[string]any
	if err := json.Unmarshal([]byte(stdout), &got); err != nil {
		t.Fatalf("decode operator status example: %v", err)
	}
	if got["schema"] != "ao.command.operator-status.v0.1" || got["status"] != "running" {
		t.Fatalf("unexpected example readback: %#v", got)
	}

	for name, wantID := range map[string]string{
		"operator-status-source-v0.1.schema.json": "https://github.com/uesugitorachiyo/ao-command/blob/main/docs/contracts/operator-status-source-v0.1.schema.json",
		"operator-status-v0.1.schema.json":        "https://github.com/uesugitorachiyo/ao-command/blob/main/docs/contracts/operator-status-v0.1.schema.json",
	} {
		body, err := os.ReadFile(filepath.Join("..", "..", "docs", "contracts", name))
		if err != nil {
			t.Fatalf("read %s: %v", name, err)
		}
		var schema map[string]any
		if err := json.Unmarshal(body, &schema); err != nil {
			t.Fatalf("decode %s: %v", name, err)
		}
		if schema["$schema"] != "https://json-schema.org/draft/2020-12/schema" ||
			schema["$id"] != wantID ||
			schema["additionalProperties"] != false {
			t.Fatalf("unexpected %s contract: %#v", name, schema)
		}
	}
}

func validOperatorStatusSource() map[string]any {
	digest := "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	return map[string]any{
		"schema":            "ao.operator.status-source.v0.1",
		"reported_status":   "running",
		"objective":         "Complete the Month 4 operator experience.",
		"correlation_id":    "corr-month4-operator-001",
		"active_repository": "ao-command",
		"workgraph_id":      "month1-6-productization",
		"active_node_id":    "m4-n01-operator-status-schema",
		"nodes": map[string]any{
			"total": 6, "completed": 1, "running": 1,
			"ready": 4, "blocked": 0, "remaining": 5,
		},
		"approval": map[string]any{
			"state": "none", "action_digest": "", "exact_instruction": "",
		},
		"verification": map[string]any{
			"tests": map[string]any{"status": "passed", "evidence": []any{"evidence/tests.json"}},
			"ci":    map[string]any{"status": "running", "evidence": []any{"https://github.com/uesugitorachiyo/ao-command/actions/runs/1"}},
		},
		"worker": map[string]any{
			"state": "running", "heartbeat_at": "2026-07-20T18:00:00Z",
			"fresh_for_seconds": 300,
		},
		"started_at":       "2026-07-20T17:00:00Z",
		"progress_percent": 16,
		"release": map[string]any{
			"status": "candidate_only", "mission_version": "0.1.0",
			"command_version": "0.1.0", "publicly_available": false,
			"publication_attempted": false,
		},
		"exact_blocker":     "",
		"exact_next_action": "continue the active Month 4 workgraph node",
		"evidence": []any{
			map[string]any{"name": "operator-tests", "location": "evidence/tests.json", "sha256": digest},
			map[string]any{"name": "hosted-ci", "location": "https://github.com/uesugitorachiyo/ao-command/actions/runs/1", "sha256": digest},
		},
		"final_report": map[string]any{"available": false, "location": "", "sha256": ""},
		"safety": map[string]any{
			"operator_mode": "read_only", "safe_to_execute": false,
			"executes_work": false, "approves_work": false,
			"mutates_repositories": false, "calls_providers": false,
			"releases_or_deploys": false,
		},
	}
}

func writeOperatorExperienceFixture(t *testing.T, name string, value any) string {
	t.Helper()
	body, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		t.Fatalf("marshal fixture: %v", err)
	}
	path := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(path, append(body, '\n'), 0o600); err != nil {
		t.Fatalf("write fixture: %v", err)
	}
	return path
}
