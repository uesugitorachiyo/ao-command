package cli

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

const githubIssueRepairFixture = "../../examples/github-issue-repair/discovery-result.valid.json"

func TestGitHubIssueRepairReadbackStableOutput(t *testing.T) {
	fake := &fakeRunner{}
	code, stdout, stderr := runWithFake([]string{
		"github-issue", "repair-readback", "--discovery", githubIssueRepairFixture,
	}, fake)
	if code != 0 || stderr != "" {
		t.Fatalf("exit=%d stderr=%q", code, stderr)
	}
	want := "" +
		"ao_command_github_issue_repair_readback=candidate_selected\n" +
		"command_schema_version=ao.command.v0.1\n" +
		"schema=ao.command.github-issue-repair-readback.v1\n" +
		"source_schema=ao.architecture.autonomous-issue-repair.discovery-result.v1\n" +
		"source_contract_commit=b8c64860003238ab45fe7c76d7e8950f80a4043b\n" +
		"source_schema_sha256=f53c8ab36753cc645c48f391d8538ddb0b26cd9fe72edfd149e653e9975b3547\n" +
		"run_id=repair-run-20260728\n" +
		"repository=example/repair-fixture\n" +
		"head_sha=1111111111111111111111111111111111111111\n" +
		"completed_at=2026-07-27T23:00:00Z\n" +
		"snapshot_count=2\n" +
		"candidate_count=1\n" +
		"exclusion_count=1\n" +
		"selected_issue=101\n" +
		"status=candidate_selected\n" +
		"operator_mode=read_only\n" +
		"safe_to_execute=false\n" +
		"approves_work=false\n" +
		"mutates_github=false\n" +
		"exact_next_action=Continue only through downstream governance; AO Command grants no mutation authority.\n"
	if stdout != want {
		t.Fatalf("stdout changed\nwant:\n%s\ngot:\n%s", want, stdout)
	}
	if len(fake.calls) != 0 {
		t.Fatalf("readback invoked Runner: %#v", fake.calls)
	}
}

func TestGitHubIssueRepairReadbackStableJSON(t *testing.T) {
	fake := &fakeRunner{}
	code, stdout, stderr := runWithFake([]string{
		"github-issue", "repair-readback", "--discovery", githubIssueRepairFixture, "--json",
	}, fake)
	if code != 0 || stderr != "" {
		t.Fatalf("exit=%d stderr=%q", code, stderr)
	}
	var got map[string]any
	if err := json.Unmarshal([]byte(stdout), &got); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	want := map[string]any{
		"command_schema_version": "ao.command.v0.1",
		"schema":                 "ao.command.github-issue-repair-readback.v1",
		"source_schema":          "ao.architecture.autonomous-issue-repair.discovery-result.v1",
		"source_contract_commit": "b8c64860003238ab45fe7c76d7e8950f80a4043b",
		"source_schema_sha256":   "f53c8ab36753cc645c48f391d8538ddb0b26cd9fe72edfd149e653e9975b3547",
		"run_id":                 "repair-run-20260728",
		"repository":             "example/repair-fixture",
		"head_sha":               "1111111111111111111111111111111111111111",
		"completed_at":           "2026-07-27T23:00:00Z",
		"snapshot_count":         float64(2),
		"candidate_count":        float64(1),
		"exclusion_count":        float64(1),
		"selected_issue":         float64(101),
		"status":                 "candidate_selected",
		"operator_mode":          "read_only",
		"safe_to_execute":        false,
		"approves_work":          false,
		"mutates_github":         false,
		"exact_next_action":      "Continue only through downstream governance; AO Command grants no mutation authority.",
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("JSON mismatch\nwant: %#v\ngot:  %#v", want, got)
	}
	if len(fake.calls) != 0 {
		t.Fatalf("readback invoked Runner: %#v", fake.calls)
	}
	for _, forbidden := range []string{
		"credential", "credentials", "token", "provider", "runner",
		"repository_mutation", "execute", "approval", "approval_digest",
	} {
		if _, exists := got[forbidden]; exists {
			t.Fatalf("JSON contains forbidden field %q: %s", forbidden, stdout)
		}
	}
}

func TestGitHubIssueRepairReadbackNoEligibleIssue(t *testing.T) {
	document := loadGitHubIssueRepairDocument(t)
	document["candidates"] = []any{}
	document["selected_issue_number"] = nil
	document["exclusion_ledger"] = []any{
		exclusion(101, "not_eligible", strings.Repeat("8", 64)),
		exclusion(102, "already_fixed_current_head", strings.Repeat("7", 64)),
	}
	path := writeGitHubIssueRepairDocument(t, document)
	code, stdout, stderr := runWithFake([]string{
		"github-issue", "repair-readback", "--discovery", path, "--json",
	}, &fakeRunner{})
	if code != 0 || stderr != "" {
		t.Fatalf("exit=%d stderr=%q", code, stderr)
	}
	if !strings.Contains(stdout, `"status": "no_eligible_issue"`) ||
		!strings.Contains(stdout, `"selected_issue": null`) {
		t.Fatalf("unexpected no-candidate JSON: %s", stdout)
	}
}

func TestGitHubIssueRepairReadbackRejectsMalformedBoundary(t *testing.T) {
	body := readGitHubIssueRepairFixture(t)
	tests := map[string]string{
		"duplicate key":          strings.Replace(body, `"mutation_performed": false`, `"mutation_performed": false, "mutation_performed": false`, 1),
		"unknown write field":    strings.Replace(body, `"mutation_performed": false`, `"execute": false, "mutation_performed": false`, 1),
		"case variant field":     strings.Replace(body, `"run_id": "repair-run-20260728"`, `"run_id": "repair-run-20260728", "Run_ID": "repair-run-attacker"`, 1),
		"missing mutation false": strings.Replace(body, `  "mutation_performed": false,`+"\n", "", 1),
		"null mutation false":    strings.Replace(body, `"mutation_performed": false`, `"mutation_performed": null`, 1),
		"malformed":              body[:len(body)-3],
		"trailing JSON":          body + "\n{}",
	}
	for name, content := range tests {
		t.Run(name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "discovery.json")
			if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
				t.Fatal(err)
			}
			code, _, stderr := runWithFake([]string{
				"github-issue", "repair-readback", "--discovery", path,
			}, &fakeRunner{})
			if code == 0 || strings.TrimSpace(stderr) == "" {
				t.Fatalf("unsafe document accepted: exit=%d stderr=%q", code, stderr)
			}
		})
	}
}

func TestGitHubIssueRepairReadbackRejectsMissingOrNullRequiredFields(t *testing.T) {
	required := []string{
		"schema", "run_id", "repository", "default_branch", "head_sha", "source_url",
		"snapshot_limit", "candidate_limit", "selected_limit", "page_count",
		"response_digests", "issues", "candidates", "selected_issue_number",
		"exclusion_ledger", "mutation_performed", "completed_at",
	}
	for _, field := range required {
		t.Run(field+"/missing", func(t *testing.T) {
			document := loadGitHubIssueRepairDocument(t)
			delete(document, field)
			assertGitHubIssueRepairRejected(t, document)
		})
		if field != "selected_issue_number" {
			t.Run(field+"/null", func(t *testing.T) {
				document := loadGitHubIssueRepairDocument(t)
				document[field] = nil
				assertGitHubIssueRepairRejected(t, document)
			})
		}
	}
}

func TestGitHubIssueRepairReadbackRejectsRedigestedSemanticViolations(t *testing.T) {
	tests := map[string]func(map[string]any){
		"repository source mismatch": func(d map[string]any) {
			d["source_url"] = "https://github.com/other/repository/issues"
		},
		"snapshot over declared limit": func(d map[string]any) {
			d["snapshot_limit"] = float64(1)
		},
		"candidate over declared limit": func(d map[string]any) {
			d["candidate_limit"] = float64(1)
			d["candidates"] = append(d["candidates"].([]any), candidate(102, 2, strings.Repeat("9", 64)))
			d["exclusion_ledger"] = []any{}
		},
		"duplicate issue number": func(d map[string]any) {
			issues := d["issues"].([]any)
			issues[1].(map[string]any)["number"] = float64(101)
		},
		"duplicate issue digest": func(d map[string]any) {
			issues := d["issues"].([]any)
			issues[1].(map[string]any)["content_digest"] = issues[0].(map[string]any)["content_digest"]
		},
		"duplicate candidate digest": func(d map[string]any) {
			d["candidates"] = append(d["candidates"].([]any), candidate(102, 2, "706b3dd556fb8a76e20d90569c4b39e7843f396feba95bac279de4aac9540ca5"))
			d["exclusion_ledger"] = []any{}
		},
		"noncontiguous rank": func(d map[string]any) {
			d["candidates"].([]any)[0].(map[string]any)["rank"] = float64(2)
		},
		"candidate outside snapshot": func(d map[string]any) {
			d["candidates"].([]any)[0].(map[string]any)["issue_number"] = float64(999)
		},
		"selected not rank one": func(d map[string]any) {
			d["selected_issue_number"] = float64(102)
		},
		"exclusion outside snapshot": func(d map[string]any) {
			d["exclusion_ledger"].([]any)[0].(map[string]any)["issue_number"] = float64(999)
		},
		"candidate exclusion overlap": func(d map[string]any) {
			d["exclusion_ledger"].([]any)[0].(map[string]any)["issue_number"] = float64(101)
		},
		"noncandidate not excluded": func(d map[string]any) {
			d["exclusion_ledger"] = []any{}
		},
		"noncandidate excluded twice": func(d map[string]any) {
			d["exclusion_ledger"] = append(d["exclusion_ledger"].([]any), exclusion(102, "duplicate", strings.Repeat("8", 64)))
		},
		"completed offset not UTC": func(d map[string]any) {
			d["completed_at"] = "2026-07-27T16:00:00-07:00"
		},
		"mutation true": func(d map[string]any) {
			d["mutation_performed"] = true
		},
		"bad schema": func(d map[string]any) {
			d["schema"] = "ao.architecture.autonomous-issue-repair.discovery-result.v2"
		},
		"response digest duplicate": func(d map[string]any) {
			d["response_digests"] = []any{strings.Repeat("2", 64), strings.Repeat("2", 64)}
		},
	}
	for name, mutate := range tests {
		t.Run(name, func(t *testing.T) {
			document := loadGitHubIssueRepairDocument(t)
			mutate(document)
			assertGitHubIssueRepairRejected(t, document)
		})
	}
}

func TestGitHubIssueRepairReadbackRejectsNestedUnknownMissingAndNull(t *testing.T) {
	tests := map[string]func(map[string]any){
		"issue unknown": func(d map[string]any) {
			d["issues"].([]any)[0].(map[string]any)["approval"] = false
		},
		"issue case variant": func(d map[string]any) {
			d["issues"].([]any)[0].(map[string]any)["State"] = "closed"
		},
		"issue missing": func(d map[string]any) {
			delete(d["issues"].([]any)[0].(map[string]any), "state")
		},
		"issue null": func(d map[string]any) {
			d["issues"].([]any)[0].(map[string]any)["updated_at"] = nil
		},
		"candidate unknown": func(d map[string]any) {
			d["candidates"].([]any)[0].(map[string]any)["execute"] = false
		},
		"candidate missing": func(d map[string]any) {
			delete(d["candidates"].([]any)[0].(map[string]any), "rank")
		},
		"candidate null": func(d map[string]any) {
			d["candidates"].([]any)[0].(map[string]any)["decision_digest"] = nil
		},
		"exclusion unknown": func(d map[string]any) {
			d["exclusion_ledger"].([]any)[0].(map[string]any)["approval_digest"] = strings.Repeat("1", 64)
		},
		"exclusion missing": func(d map[string]any) {
			delete(d["exclusion_ledger"].([]any)[0].(map[string]any), "reason_codes")
		},
		"exclusion null": func(d map[string]any) {
			d["exclusion_ledger"].([]any)[0].(map[string]any)["evidence_digests"] = nil
		},
	}
	for name, mutate := range tests {
		t.Run(name, func(t *testing.T) {
			document := loadGitHubIssueRepairDocument(t)
			mutate(document)
			assertGitHubIssueRepairRejected(t, document)
		})
	}
}

func TestGitHubIssueRepairReadbackUsageIsBounded(t *testing.T) {
	tests := [][]string{
		{"github-issue"},
		{"github-issue", "repair-readback"},
		{"github-issue", "repair-readback", "--discovery", githubIssueRepairFixture, "extra"},
	}
	for _, args := range tests {
		code, _, stderr := runWithFake(args, &fakeRunner{})
		if code != 2 || strings.TrimSpace(stderr) == "" {
			t.Fatalf("args=%q exit=%d stderr=%q", args, code, stderr)
		}
	}
}

func TestGitHubIssueRepairReadbackRejectsSchemaConstraintViolations(t *testing.T) {
	tests := map[string]func(map[string]any){
		"short run id":          func(d map[string]any) { d["run_id"] = "short" },
		"invalid repository":    func(d map[string]any) { d["repository"] = "example" },
		"empty branch":          func(d map[string]any) { d["default_branch"] = "" },
		"long branch":           func(d map[string]any) { d["default_branch"] = strings.Repeat("x", 256) },
		"invalid head":          func(d map[string]any) { d["head_sha"] = strings.Repeat("A", 40) },
		"snapshot limit zero":   func(d map[string]any) { d["snapshot_limit"] = float64(0) },
		"snapshot limit high":   func(d map[string]any) { d["snapshot_limit"] = float64(51) },
		"candidate limit zero":  func(d map[string]any) { d["candidate_limit"] = float64(0) },
		"candidate limit high":  func(d map[string]any) { d["candidate_limit"] = float64(11) },
		"selected limit":        func(d map[string]any) { d["selected_limit"] = float64(2) },
		"page count":            func(d map[string]any) { d["page_count"] = float64(0) },
		"empty response digest": func(d map[string]any) { d["response_digests"] = []any{} },
		"bad response digest":   func(d map[string]any) { d["response_digests"] = []any{"bad"} },
		"issue number":          func(d map[string]any) { d["issues"].([]any)[0].(map[string]any)["number"] = float64(0) },
		"issue state":           func(d map[string]any) { d["issues"].([]any)[0].(map[string]any)["state"] = "closed" },
		"issue timestamp":       func(d map[string]any) { d["issues"].([]any)[0].(map[string]any)["updated_at"] = "yesterday" },
		"issue digest":          func(d map[string]any) { d["issues"].([]any)[0].(map[string]any)["content_digest"] = "bad" },
		"candidate digest":      func(d map[string]any) { d["candidates"].([]any)[0].(map[string]any)["decision_digest"] = "bad" },
		"empty reasons":         func(d map[string]any) { d["exclusion_ledger"].([]any)[0].(map[string]any)["reason_codes"] = []any{} },
		"duplicate reasons": func(d map[string]any) {
			d["exclusion_ledger"].([]any)[0].(map[string]any)["reason_codes"] = []any{"same", "same"}
		},
		"long reason": func(d map[string]any) {
			d["exclusion_ledger"].([]any)[0].(map[string]any)["reason_codes"] = []any{strings.Repeat("x", 129)}
		},
		"empty evidence": func(d map[string]any) {
			d["exclusion_ledger"].([]any)[0].(map[string]any)["evidence_digests"] = []any{}
		},
		"bad evidence digest": func(d map[string]any) {
			d["exclusion_ledger"].([]any)[0].(map[string]any)["evidence_digests"] = []any{"bad"}
		},
		"duplicate evidence digest": func(d map[string]any) {
			d["exclusion_ledger"].([]any)[0].(map[string]any)["evidence_digests"] = []any{strings.Repeat("7", 64), strings.Repeat("7", 64)}
		},
		"candidate without selection": func(d map[string]any) { d["selected_issue_number"] = nil },
	}
	for name, mutate := range tests {
		t.Run(name, func(t *testing.T) {
			document := loadGitHubIssueRepairDocument(t)
			mutate(document)
			assertGitHubIssueRepairRejected(t, document)
		})
	}
}

func TestGitHubIssueRepairReadbackRejectsOversizedInput(t *testing.T) {
	body := readGitHubIssueRepairFixture(t)
	body = strings.Replace(body, `"run_id": "repair-run-20260728"`,
		`"run_id": "repair-run-20260728", "padding": "`+strings.Repeat("x", (1<<20))+`"`, 1)
	path := filepath.Join(t.TempDir(), "oversized.json")
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	code, _, stderr := runWithFake([]string{
		"github-issue", "repair-readback", "--discovery", path,
	}, &fakeRunner{})
	if code == 0 || !strings.Contains(stderr, "input exceeds") {
		t.Fatalf("oversized input accepted: exit=%d stderr=%q", code, stderr)
	}
}

func TestGitHubIssueRepairProvenanceManifestReplaysFixtureDigest(t *testing.T) {
	type manifest struct {
		Schema                  string `json:"schema"`
		ArchitectureRepository  string `json:"architecture_repository"`
		ArchitectureCommit      string `json:"architecture_commit"`
		SourceSchemaID          string `json:"source_schema_id"`
		SourceSchemaPath        string `json:"source_schema_path"`
		SourceSchemaSHA256      string `json:"source_schema_sha256"`
		ConsumerFixturePath     string `json:"consumer_fixture_path"`
		ConsumerFixtureSHA256   string `json:"consumer_fixture_sha256"`
		OperatorMode            string `json:"operator_mode"`
		NetworkRequired         bool   `json:"network_required"`
		ShellRequired           bool   `json:"shell_required"`
		GrantsMutationAuthority bool   `json:"grants_mutation_authority"`
	}
	body, err := os.ReadFile("../../examples/github-issue-repair/provenance-manifest.json")
	if err != nil {
		t.Fatal(err)
	}
	var got manifest
	decoder := json.NewDecoder(strings.NewReader(string(body)))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&got); err != nil {
		t.Fatalf("invalid provenance manifest: %v", err)
	}
	if got.Schema != "ao.command.github-issue-repair-readback-provenance.v1" ||
		got.ArchitectureRepository != "uesugitorachiyo/ao-architecture" ||
		got.ArchitectureCommit != githubIssueRepairSourceCommit ||
		got.SourceSchemaID != githubIssueRepairSourceSchema ||
		got.SourceSchemaPath != "stack/schemas/github-issue-repair/bounded-discovery-result-v1.schema.json" ||
		got.SourceSchemaSHA256 != githubIssueRepairSourceDigest ||
		got.ConsumerFixturePath != "examples/github-issue-repair/discovery-result.valid.json" ||
		got.OperatorMode != operatorMode || got.NetworkRequired || got.ShellRequired || got.GrantsMutationAuthority {
		t.Fatalf("unexpected provenance manifest: %+v", got)
	}
	fixture, err := os.ReadFile(githubIssueRepairFixture)
	if err != nil {
		t.Fatal(err)
	}
	if digest := fmt.Sprintf("%x", sha256.Sum256(fixture)); digest != got.ConsumerFixtureSHA256 {
		t.Fatalf("fixture digest=%s want=%s", digest, got.ConsumerFixtureSHA256)
	}
}

func assertGitHubIssueRepairRejected(t *testing.T, document map[string]any) {
	t.Helper()
	path := writeGitHubIssueRepairDocument(t, document)
	fake := &fakeRunner{}
	code, stdout, stderr := runWithFake([]string{
		"github-issue", "repair-readback", "--discovery", path,
	}, fake)
	if code == 0 || stdout != "" || strings.TrimSpace(stderr) == "" {
		t.Fatalf("unsafe document accepted: exit=%d stdout=%q stderr=%q", code, stdout, stderr)
	}
	if len(fake.calls) != 0 {
		t.Fatalf("rejected readback invoked Runner: %#v", fake.calls)
	}
}

func loadGitHubIssueRepairDocument(t *testing.T) map[string]any {
	t.Helper()
	var document map[string]any
	if err := json.Unmarshal([]byte(readGitHubIssueRepairFixture(t)), &document); err != nil {
		t.Fatal(err)
	}
	return document
}

func readGitHubIssueRepairFixture(t *testing.T) string {
	t.Helper()
	body, err := os.ReadFile(githubIssueRepairFixture)
	if err != nil {
		t.Fatal(err)
	}
	return string(body)
}

func writeGitHubIssueRepairDocument(t *testing.T, document map[string]any) string {
	t.Helper()
	body, err := json.MarshalIndent(document, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "discovery.json")
	if err := os.WriteFile(path, append(body, '\n'), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func candidate(issue, rank int, digest string) map[string]any {
	return map[string]any{
		"issue_number":    issue,
		"rank":            rank,
		"decision_digest": digest,
	}
}

func exclusion(issue int, reason, digest string) map[string]any {
	return map[string]any{
		"issue_number":     issue,
		"reason_codes":     []any{reason},
		"evidence_digests": []any{digest},
	}
}
