package cli

import (
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"go.yaml.in/yaml/v3"
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
		"exact_next_action=Submit the selected candidate only to downstream governance; AO Command grants no mutation authority.\n"
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
		"exact_next_action":      "Submit the selected candidate only to downstream governance; AO Command grants no mutation authority.",
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

func TestGitHubIssueRepairReadbackTruthfulStatuses(t *testing.T) {
	tests := []struct {
		name       string
		mutate     func(map[string]any)
		status     string
		nextAction string
	}{
		{
			name:   "selected",
			mutate: func(map[string]any) {},
			status: "candidate_selected",
			nextAction: "Submit the selected candidate only to downstream governance; " +
				"AO Command grants no mutation authority.",
		},
		{
			name: "candidates not selected",
			mutate: func(document map[string]any) {
				document["selected_issue_number"] = nil
				document["exclusion_ledger"] = []any{
					exclusion(101, "unselected", strings.Repeat("8", 64)),
					exclusion(102, "unselected", strings.Repeat("7", 64)),
				}
			},
			status: "candidates_not_selected",
			nextAction: "Review the unselected candidates before downstream governance; " +
				"AO Command grants no mutation authority.",
		},
		{
			name: "no eligible issue",
			mutate: func(document map[string]any) {
				document["candidates"] = []any{}
				document["selected_issue_number"] = nil
				document["exclusion_ledger"] = []any{
					exclusion(101, "not_eligible", strings.Repeat("8", 64)),
					exclusion(102, "already_fixed_current_head", strings.Repeat("7", 64)),
				}
			},
			status: "no_eligible_issue",
			nextAction: "Record that discovery found no eligible issue; any future repair requires downstream governance, " +
				"and AO Command grants no mutation authority.",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			document := loadGitHubIssueRepairDocument(t)
			test.mutate(document)
			path := writeGitHubIssueRepairDocument(t, document)
			for _, jsonOut := range []bool{false, true} {
				args := []string{"github-issue", "repair-readback", "--discovery", path}
				if jsonOut {
					args = append(args, "--json")
				}
				fake := &fakeRunner{}
				code, stdout, stderr := runWithFake(args, fake)
				if code != 0 || stderr != "" {
					t.Fatalf("json=%t exit=%d stderr=%q", jsonOut, code, stderr)
				}
				if jsonOut {
					var summary map[string]any
					if err := json.Unmarshal([]byte(stdout), &summary); err != nil {
						t.Fatal(err)
					}
					if summary["status"] != test.status || summary["exact_next_action"] != test.nextAction ||
						summary["operator_mode"] != "read_only" || summary["safe_to_execute"] != false ||
						summary["approves_work"] != false || summary["mutates_github"] != false {
						t.Fatalf("unexpected JSON summary: %#v", summary)
					}
				} else {
					for _, expected := range []string{
						"ao_command_github_issue_repair_readback=" + test.status + "\n",
						"status=" + test.status + "\n",
						"operator_mode=read_only\n",
						"safe_to_execute=false\n",
						"approves_work=false\n",
						"mutates_github=false\n",
						"exact_next_action=" + test.nextAction + "\n",
					} {
						if !strings.Contains(stdout, expected) {
							t.Fatalf("text output missing %q:\n%s", expected, stdout)
						}
					}
				}
				if len(fake.calls) != 0 {
					t.Fatalf("status readback invoked Runner: %#v", fake.calls)
				}
			}
		})
	}
}

func TestGitHubIssueRepairReadbackRejectsMalformedBoundary(t *testing.T) {
	body := readGitHubIssueRepairFixture(t)
	tests := map[string]string{
		"unknown write field":    strings.Replace(body, `"mutation_performed": false`, `"execute": false, "mutation_performed": false`, 1),
		"case variant field":     strings.Replace(body, `"run_id": "repair-run-20260728"`, `"run_id": "repair-run-20260728", "Run_ID": "repair-run-attacker"`, 1),
		"missing mutation false": strings.Replace(body, `  "mutation_performed": false,`+"\n", "", 1),
		"null mutation false":    strings.Replace(body, `"mutation_performed": false`, `"mutation_performed": null`, 1),
		"malformed":              body[:len(body)-3],
		"trailing JSON":          body + "\n{}",
		"invalid UTF-8":          strings.Replace(body, `"master"`, "\"ma\xffster\"", 1),
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

func TestGitHubIssueRepairReadbackMatchesArchitectureDuplicateKeyHandling(t *testing.T) {
	body := readGitHubIssueRepairFixture(t)
	body = strings.Replace(body, `"mutation_performed": false`,
		`"mutation_performed": false, "mutation_performed": false`, 1)
	path := filepath.Join(t.TempDir(), "duplicate.json")
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	code, _, stderr := runWithFake([]string{
		"github-issue", "repair-readback", "--discovery", path,
	}, &fakeRunner{})
	if code != 0 {
		t.Fatalf("Architecture-compatible duplicate rejected: exit=%d stderr=%q", code, stderr)
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
		"noncontiguous rank": func(d map[string]any) {
			d["candidates"].([]any)[0].(map[string]any)["rank"] = float64(2)
		},
		"candidate outside snapshot": func(d map[string]any) {
			d["candidates"].([]any)[0].(map[string]any)["issue_number"] = float64(999)
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
		"mutation true": func(d map[string]any) {
			d["mutation_performed"] = true
		},
		"bad schema": func(d map[string]any) {
			d["schema"] = "ao.architecture.autonomous-issue-repair.discovery-result.v2"
		},
		"response digest duplicate": func(d map[string]any) {
			d["response_digests"] = []any{strings.Repeat("2", 64), strings.Repeat("2", 64)}
		},
		"response page mismatch": func(d map[string]any) {
			d["page_count"] = float64(2)
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
	}
	for name, mutate := range tests {
		t.Run(name, func(t *testing.T) {
			document := loadGitHubIssueRepairDocument(t)
			mutate(document)
			assertGitHubIssueRepairRejected(t, document)
		})
	}
}

func TestGitHubIssueRepairReadbackAcceptsAuthoritativeValidVariants(t *testing.T) {
	tests := map[string]func(map[string]any){
		"source URL independent of repository": func(d map[string]any) {
			d["source_url"] = "https://github.com/other/repository/issues"
		},
		"RFC3339 offset": func(d map[string]any) {
			d["completed_at"] = "2026-07-27T16:00:00-07:00"
		},
		"extreme valid offsets": func(d map[string]any) {
			d["completed_at"] = "2026-07-27T23:00:00+22:99"
			d["issues"].([]any)[0].(map[string]any)["updated_at"] = "2026-07-27T23:00:00-22:99"
		},
		"shared issue content digest": func(d map[string]any) {
			issues := d["issues"].([]any)
			issues[1].(map[string]any)["content_digest"] = issues[0].(map[string]any)["content_digest"]
		},
		"rank two selected": func(d map[string]any) {
			d["candidates"] = append(d["candidates"].([]any), candidate(102, 2, strings.Repeat("9", 64)))
			d["selected_issue_number"] = float64(102)
			d["exclusion_ledger"] = []any{exclusion(101, "unselected", strings.Repeat("8", 64))}
		},
		"rank two excluded": func(d map[string]any) {
			d["candidates"] = append(d["candidates"].([]any), candidate(102, 2, strings.Repeat("9", 64)))
		},
		"shared candidate decision digest": func(d map[string]any) {
			first := d["candidates"].([]any)[0].(map[string]any)["decision_digest"].(string)
			d["candidates"] = append(d["candidates"].([]any), candidate(102, 2, first))
		},
		"nil selection with candidates": func(d map[string]any) {
			d["selected_issue_number"] = nil
			d["exclusion_ledger"] = []any{
				exclusion(101, "unselected", strings.Repeat("8", 64)),
				exclusion(102, "unselected", strings.Repeat("7", 64)),
			}
		},
		"shared evidence digest across exclusions": func(d map[string]any) {
			addThirdSnapshotIssue(d)
			d["exclusion_ledger"] = []any{
				exclusion(102, "unselected", strings.Repeat("7", 64)),
				exclusion(103, "unselected", strings.Repeat("7", 64)),
			}
		},
		"unicode branch character length": func(d map[string]any) {
			d["default_branch"] = strings.Repeat("界", 255)
		},
		"unicode reason character length": func(d map[string]any) {
			d["exclusion_ledger"].([]any)[0].(map[string]any)["reason_codes"] = []any{strings.Repeat("界", 128)}
		},
		"unbounded issue number": func(d map[string]any) {
			huge := json.Number("9223372036854775808")
			d["issues"].([]any)[0].(map[string]any)["number"] = huge
			d["candidates"].([]any)[0].(map[string]any)["issue_number"] = huge
			d["selected_issue_number"] = huge
		},
	}
	for name, mutate := range tests {
		t.Run(name, func(t *testing.T) {
			document := loadGitHubIssueRepairDocument(t)
			mutate(document)
			path := writeGitHubIssueRepairDocument(t, document)
			code, _, stderr := runWithFake([]string{
				"github-issue", "repair-readback", "--discovery", path,
			}, &fakeRunner{})
			if code != 0 {
				t.Fatalf("authoritative valid variant rejected: %s", stderr)
			}
		})
	}
}

func TestGitHubIssueRepairConsumerParityWithPinnedArchitectureValidator(t *testing.T) {
	corpus := []struct {
		name   string
		mutate func(map[string]any)
	}{
		{name: "valid"},
		{name: "response count mismatch", mutate: func(d map[string]any) { d["page_count"] = float64(2) }},
		{name: "issues exceed snapshot limit", mutate: func(d map[string]any) { d["snapshot_limit"] = float64(1) }},
		{name: "candidates exceed candidate limit", mutate: func(d map[string]any) {
			d["candidate_limit"] = float64(1)
			d["candidates"] = append(d["candidates"].([]any), candidate(102, 2, strings.Repeat("9", 64)))
		}},
		{name: "duplicate issue number", mutate: func(d map[string]any) {
			d["issues"].([]any)[1].(map[string]any)["number"] = float64(101)
		}},
		{name: "duplicate candidate number", mutate: func(d map[string]any) {
			d["candidates"] = append(d["candidates"].([]any), candidate(101, 2, strings.Repeat("9", 64)))
		}},
		{name: "candidate outside snapshot", mutate: func(d map[string]any) {
			d["candidates"].([]any)[0].(map[string]any)["issue_number"] = float64(999)
		}},
		{name: "duplicate rank", mutate: func(d map[string]any) {
			d["candidates"] = append(d["candidates"].([]any), candidate(102, 1, strings.Repeat("9", 64)))
		}},
		{name: "noncontiguous rank", mutate: func(d map[string]any) {
			d["candidates"].([]any)[0].(map[string]any)["rank"] = float64(2)
		}},
		{name: "selected outside candidates", mutate: func(d map[string]any) { d["selected_issue_number"] = float64(102) }},
		{name: "duplicate exclusion", mutate: func(d map[string]any) {
			d["exclusion_ledger"] = append(d["exclusion_ledger"].([]any), exclusion(102, "again", strings.Repeat("8", 64)))
		}},
		{name: "missing unselected exclusion", mutate: func(d map[string]any) { d["exclusion_ledger"] = []any{} }},
		{name: "nil selection missing exclusion", mutate: func(d map[string]any) {
			d["selected_issue_number"] = nil
		}},
		{name: "rank two selected", mutate: func(d map[string]any) {
			d["candidates"] = append(d["candidates"].([]any), candidate(102, 2, strings.Repeat("9", 64)))
			d["selected_issue_number"] = float64(102)
			d["exclusion_ledger"] = []any{exclusion(101, "unselected", strings.Repeat("8", 64))}
		}},
		{name: "rank two excluded", mutate: func(d map[string]any) {
			d["candidates"] = append(d["candidates"].([]any), candidate(102, 2, strings.Repeat("9", 64)))
		}},
		{name: "nil selection excludes all", mutate: func(d map[string]any) {
			d["selected_issue_number"] = nil
			d["exclusion_ledger"] = []any{
				exclusion(101, "unselected", strings.Repeat("8", 64)),
				exclusion(102, "unselected", strings.Repeat("7", 64)),
			}
		}},
		{name: "shared unpinned digests", mutate: func(d map[string]any) {
			addThirdSnapshotIssue(d)
			d["issues"].([]any)[1].(map[string]any)["content_digest"] =
				d["issues"].([]any)[0].(map[string]any)["content_digest"]
			first := d["candidates"].([]any)[0].(map[string]any)["decision_digest"].(string)
			d["candidates"] = append(d["candidates"].([]any), candidate(102, 2, first))
			d["exclusion_ledger"] = []any{
				exclusion(102, "unselected", strings.Repeat("7", 64)),
				exclusion(103, "unselected", strings.Repeat("7", 64)),
			}
		}},
		{name: "offset timestamps", mutate: func(d map[string]any) {
			d["completed_at"] = "2026-07-27T16:00:00-07:00"
			d["issues"].([]any)[0].(map[string]any)["updated_at"] = "2026-07-27T14:00:00-07:00"
		}},
		{name: "independent source URL", mutate: func(d map[string]any) {
			d["source_url"] = "https://github.com/other/repository/issues"
		}},
		{name: "unbounded issue number", mutate: func(d map[string]any) {
			huge := json.Number("9223372036854775808")
			d["issues"].([]any)[0].(map[string]any)["number"] = huge
			d["candidates"].([]any)[0].(map[string]any)["issue_number"] = huge
			d["selected_issue_number"] = huge
		}},
		{name: "missing required field", mutate: func(d map[string]any) { delete(d, "head_sha") }},
		{name: "unknown top field", mutate: func(d map[string]any) { d["execute"] = false }},
		{name: "null required field", mutate: func(d map[string]any) { d["mutation_performed"] = nil }},
		{name: "invalid run pattern", mutate: func(d map[string]any) { d["run_id"] = "short" }},
		{name: "invalid source pattern", mutate: func(d map[string]any) { d["source_url"] = "http://example.invalid" }},
		{name: "schema maximum", mutate: func(d map[string]any) { d["snapshot_limit"] = float64(51) }},
		{name: "schema unique array", mutate: func(d map[string]any) {
			d["response_digests"] = []any{strings.Repeat("2", 64), strings.Repeat("2", 64)}
			d["page_count"] = float64(2)
		}},
		{name: "nested const", mutate: func(d map[string]any) {
			d["issues"].([]any)[0].(map[string]any)["state"] = "closed"
		}},
		{name: "nested unknown field", mutate: func(d map[string]any) {
			d["candidates"].([]any)[0].(map[string]any)["approval"] = false
		}},
		{name: "array minimum", mutate: func(d map[string]any) {
			d["exclusion_ledger"].([]any)[0].(map[string]any)["reason_codes"] = []any{}
		}},
		{name: "invalid date-time", mutate: func(d map[string]any) { d["completed_at"] = "yesterday" }},
		{name: "comma fractional date-time", mutate: func(d map[string]any) {
			d["completed_at"] = "2026-07-27T23:00:00,5Z"
		}},
		{name: "completed year zero", mutate: func(d map[string]any) {
			d["completed_at"] = "0000-01-01T00:00:00Z"
		}},
		{name: "issue year zero", mutate: func(d map[string]any) {
			d["issues"].([]any)[0].(map[string]any)["updated_at"] = "0000-01-01T00:00:00Z"
		}},
		{name: "completed positive offset hour 24", mutate: func(d map[string]any) {
			d["completed_at"] = "2026-07-27T23:00:00+24:00"
		}},
		{name: "issue negative offset hour 24", mutate: func(d map[string]any) {
			d["issues"].([]any)[0].(map[string]any)["updated_at"] = "2026-07-27T23:00:00-24:00"
		}},
		{name: "completed positive offset minute 60", mutate: func(d map[string]any) {
			d["completed_at"] = "2026-07-27T23:00:00+23:60"
		}},
		{name: "issue negative offset minute 60", mutate: func(d map[string]any) {
			d["issues"].([]any)[0].(map[string]any)["updated_at"] = "2026-07-27T23:00:00-23:60"
		}},
	}

	validator := pinnedArchitectureDiscoveryValidator(t)
	for _, test := range corpus {
		t.Run(test.name, func(t *testing.T) {
			document := loadGitHubIssueRepairDocument(t)
			if test.mutate != nil {
				test.mutate(document)
			}
			path := writeGitHubIssueRepairDocument(t, document)
			_, consumerErr := readGitHubIssueRepairDiscovery(path)
			architectureAccepts := validator(path)
			if (consumerErr == nil) != architectureAccepts {
				t.Fatalf("acceptance mismatch: consumer_err=%v architecture_accepts=%t", consumerErr, architectureAccepts)
			}
		})
	}
}

func TestGitHubIssueRepairDuplicateKeysMatchPinnedArchitecture(t *testing.T) {
	body := readGitHubIssueRepairFixture(t)
	tests := []struct {
		name    string
		body    string
		accepts bool
	}{
		{
			name: "top invalid earlier valid final",
			body: strings.Replace(body, `"snapshot_limit": 50`,
				`"snapshot_limit": "invalid", "snapshot_limit": 50`, 1),
			accepts: true,
		},
		{
			name: "nested invalid earlier valid final",
			body: strings.Replace(body, `"number": 101`,
				`"number": "invalid", "number": 101`, 1),
			accepts: true,
		},
		{
			name: "top valid earlier invalid final",
			body: strings.Replace(body, `"snapshot_limit": 50`,
				`"snapshot_limit": 50, "snapshot_limit": "invalid"`, 1),
			accepts: false,
		},
		{
			name: "nested valid earlier invalid final",
			body: strings.Replace(body, `"number": 101`,
				`"number": 101, "number": "invalid"`, 1),
			accepts: false,
		},
	}
	validator := pinnedArchitectureDiscoveryValidator(t)
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "duplicate.json")
			if err := os.WriteFile(path, []byte(test.body), 0o600); err != nil {
				t.Fatal(err)
			}
			_, consumerErr := readGitHubIssueRepairDiscovery(path)
			architectureAccepts := validator(path)
			if architectureAccepts != test.accepts || (consumerErr == nil) != architectureAccepts {
				t.Fatalf("acceptance mismatch: expected=%t architecture=%t consumer_err=%v",
					test.accepts, architectureAccepts, consumerErr)
			}
		})
	}
}

type workflowActionPin struct {
	sha     string
	version string
}

type workflowUsesReference struct {
	reference string
	comment   string
	line      int
}

type workflowYAMLScanLimits struct {
	maxDocuments   int
	maxUniqueNodes int
}

const (
	workflowYAMLMaxDocuments   = 16
	workflowYAMLMaxUniqueNodes = 10_000
)

type workflowYAMLScanState struct {
	limits   workflowYAMLScanLimits
	visited  map[*yaml.Node]bool
	active   map[*yaml.Node]bool
	resolved map[*yaml.Node]*yaml.Node
	nodes    int
}

func scanWorkflowUsesDocuments(body []byte) ([]workflowUsesReference, error) {
	return scanWorkflowUsesDocumentsWithLimits(body, workflowYAMLScanLimits{
		maxDocuments:   workflowYAMLMaxDocuments,
		maxUniqueNodes: workflowYAMLMaxUniqueNodes,
	})
}

func scanWorkflowUsesDocumentsWithLimits(
	body []byte,
	limits workflowYAMLScanLimits,
) ([]workflowUsesReference, error) {
	if limits.maxDocuments <= 0 || limits.maxUniqueNodes <= 0 {
		return nil, errors.New("workflow YAML scan limits must be positive")
	}
	decoder := yaml.NewDecoder(bytes.NewReader(body))
	state := workflowYAMLScanState{
		limits:   limits,
		visited:  make(map[*yaml.Node]bool),
		active:   make(map[*yaml.Node]bool),
		resolved: make(map[*yaml.Node]*yaml.Node),
	}
	var references []workflowUsesReference
	for document := 1; ; document++ {
		var root yaml.Node
		if err := decoder.Decode(&root); err != nil {
			if errors.Is(err, io.EOF) {
				return references, nil
			}
			return references, fmt.Errorf("parse workflow YAML document %d: %w", document, err)
		}
		if document > limits.maxDocuments {
			return references, fmt.Errorf(
				"workflow YAML document budget exceeded: maximum %d", limits.maxDocuments)
		}
		if err := state.walk(&root, &references); err != nil {
			return references, fmt.Errorf("workflow YAML document %d: %w", document, err)
		}
	}
}

func (state *workflowYAMLScanState) walk(
	node *yaml.Node,
	references *[]workflowUsesReference,
) error {
	if node == nil {
		return nil
	}
	if state.active[node] {
		return fmt.Errorf("cyclic YAML alias at line %d", node.Line)
	}
	// A node's uses semantics depend only on its immutable YAML content.
	if state.visited[node] {
		return nil
	}
	if state.nodes >= state.limits.maxUniqueNodes {
		return fmt.Errorf(
			"workflow YAML unique-node budget exceeded: maximum %d",
			state.limits.maxUniqueNodes,
		)
	}
	state.visited[node] = true
	state.nodes++
	state.active[node] = true
	defer delete(state.active, node)

	switch node.Kind {
	case yaml.DocumentNode, yaml.SequenceNode:
		for _, child := range node.Content {
			if err := state.walk(child, references); err != nil {
				return err
			}
		}
	case yaml.MappingNode:
		if len(node.Content)%2 != 0 {
			return fmt.Errorf("mapping at line %d has an unmatched key", node.Line)
		}
		for index := 0; index < len(node.Content); index += 2 {
			keyNode := node.Content[index]
			valueNode := node.Content[index+1]
			if err := state.walk(keyNode, references); err != nil {
				return err
			}
			if err := state.walk(valueNode, references); err != nil {
				return err
			}
			key, err := state.resolve(keyNode)
			if err != nil {
				return err
			}
			if key.Kind == yaml.ScalarNode && key.Tag == "!!str" && key.Value == "uses" {
				resolvedValue, err := state.resolve(valueNode)
				if err != nil {
					return err
				}
				if resolvedValue.Kind != yaml.ScalarNode || resolvedValue.Tag != "!!str" {
					return fmt.Errorf("uses value at line %d must be a string scalar", valueNode.Line)
				}
				*references = append(*references, workflowUsesReference{
					reference: resolvedValue.Value,
					comment: strings.TrimSpace(strings.TrimPrefix(
						strings.TrimSpace(valueNode.LineComment), "#")),
					line: valueNode.Line,
				})
			}
		}
	case yaml.AliasNode:
		if node.Alias == nil {
			return fmt.Errorf("unresolved YAML alias at line %d", node.Line)
		}
		return state.walk(node.Alias, references)
	}
	return nil
}

func (state *workflowYAMLScanState) resolve(node *yaml.Node) (*yaml.Node, error) {
	seen := make(map[*yaml.Node]bool)
	var aliases []*yaml.Node
	for node != nil && node.Kind == yaml.AliasNode {
		if resolved, exists := state.resolved[node]; exists {
			node = resolved
			break
		}
		if seen[node] || node.Alias == nil {
			return nil, fmt.Errorf("invalid YAML alias at line %d", node.Line)
		}
		seen[node] = true
		aliases = append(aliases, node)
		node = node.Alias
	}
	if node == nil {
		return nil, errors.New("nil YAML node")
	}
	for _, alias := range aliases {
		state.resolved[alias] = node
	}
	return node, nil
}

func validateWorkflowActionReferences(references []workflowUsesReference, allowed map[string]workflowActionPin) error {
	for _, reference := range references {
		parts := strings.Split(reference.reference, "@")
		if len(parts) != 2 || len(parts[1]) != 40 {
			return fmt.Errorf("line %d: non-40-hex action ref %q", reference.line, reference.reference)
		}
		for _, character := range parts[1] {
			isDigit := character >= '0' && character <= '9'
			isLowerHex := character >= 'a' && character <= 'f'
			if !isDigit && !isLowerHex {
				return fmt.Errorf("line %d: non-40-hex action ref %q", reference.line, reference.reference)
			}
		}
		expected, exists := allowed[parts[0]]
		if !exists || parts[1] != expected.sha {
			return fmt.Errorf("line %d: unapproved action ref %q", reference.line, reference.reference)
		}
		if expected.version != "" && reference.comment != expected.version {
			return fmt.Errorf("line %d: action ref %q lacks exact comment # %s",
				reference.line, reference.reference, expected.version)
		}
	}
	return nil
}

func TestGitHubIssueRepairParityIsMandatoryInHostedCI(t *testing.T) {
	body, err := os.ReadFile("../../.github/workflows/ci.yml")
	if err != nil {
		t.Fatal(err)
	}
	workflow := string(body)
	for _, expected := range []string{
		"name: Checkout pinned AO Architecture parity oracle",
		"repository: uesugitorachiyo/ao-architecture",
		"ref: b8c64860003238ab45fe7c76d7e8950f80a4043b",
		"path: ao-architecture-parity",
		"AO_ARCHITECTURE_REPO: ${{ github.workspace }}/ao-architecture-parity",
		`AO_ARCHITECTURE_PARITY_REQUIRED: "true"`,
		`test "$(git -C "$AO_ARCHITECTURE_REPO" rev-parse HEAD)" = "b8c64860003238ab45fe7c76d7e8950f80a4043b"`,
	} {
		if !strings.Contains(workflow, expected) {
			t.Fatalf("CI parity oracle wiring missing %q", expected)
		}
	}
	allowed := map[string]workflowActionPin{
		"actions/checkout": {
			sha:     "3d3c42e5aac5ba805825da76410c181273ba90b1",
			version: "v7",
		},
		"actions/setup-go": {
			sha:     "924ae3a1cded613372ab5595356fb5720e22ba16",
			version: "v6",
		},
	}
	references, err := scanWorkflowUsesDocuments(body)
	if err != nil {
		t.Fatalf("CI workflow YAML: %v", err)
	}
	if err := validateWorkflowActionReferences(references, allowed); err != nil {
		t.Fatalf("CI workflow action reference: %v", err)
	}
	if len(references) == 0 {
		t.Fatal("CI workflow contains no action references")
	}
}

func TestWorkflowActionReferencesArePinnedRepoWide(t *testing.T) {
	allowed := map[string]workflowActionPin{
		"actions/checkout": {
			sha:     "3d3c42e5aac5ba805825da76410c181273ba90b1",
			version: "v7",
		},
		"actions/setup-go": {
			sha:     "924ae3a1cded613372ab5595356fb5720e22ba16",
			version: "v6",
		},
		"actions/upload-artifact": {
			sha:     "043fb46d1a93c77aae656e7c1c64a875d1fc6a0a",
			version: "v7",
		},
		"actions/download-artifact": {
			sha:     "37930b1c2abaa49bbe596cd826c3c89aef350131",
			version: "v7",
		},
	}
	workflowRoot := filepath.Clean("../../.github/workflows")
	entries, err := os.ReadDir(workflowRoot)
	if err != nil {
		t.Fatal(err)
	}
	seen := make(map[string]bool, len(allowed))
	usesCount := 0
	for _, entry := range entries {
		extension := filepath.Ext(entry.Name())
		if entry.IsDir() || extension != ".yml" && extension != ".yaml" {
			continue
		}
		body, err := os.ReadFile(filepath.Join(workflowRoot, entry.Name()))
		if err != nil {
			t.Fatal(err)
		}
		references, err := scanWorkflowUsesDocuments(body)
		if err != nil {
			t.Fatalf("%s: %v", entry.Name(), err)
		}
		if err := validateWorkflowActionReferences(references, allowed); err != nil {
			t.Fatalf("%s: %v", entry.Name(), err)
		}
		usesCount += len(references)
		for _, reference := range references {
			action := strings.SplitN(reference.reference, "@", 2)[0]
			seen[action] = true
		}
	}
	if usesCount == 0 {
		t.Fatal("workflow directory contains no action references")
	}
	for action := range allowed {
		if !seen[action] {
			t.Fatalf("workflow action allowlist entry %q is not exercised", action)
		}
	}
}

func TestWorkflowUsesScannerTraversesYAMLStructure(t *testing.T) {
	checkoutSHA := "3d3c42e5aac5ba805825da76410c181273ba90b1"
	allowed := map[string]workflowActionPin{
		"actions/checkout": {sha: checkoutSHA},
	}
	tests := []struct {
		name      string
		document  string
		wantCount int
		wantErr   bool
	}{
		{
			name:      "block form",
			document:  "step:\n  uses: actions/checkout@" + checkoutSHA + "\n",
			wantCount: 1,
		},
		{
			name:      "nested list form",
			document:  "jobs:\n  test:\n    steps:\n      - uses: actions/checkout@" + checkoutSHA + "\n",
			wantCount: 1,
		},
		{
			name:      "quoted key",
			document:  "step:\n  \"uses\": actions/checkout@" + checkoutSHA + "\n",
			wantCount: 1,
		},
		{
			name:      "spacing and comment",
			document:  "step:\n  uses : actions/checkout@" + checkoutSHA + " # v7\n",
			wantCount: 1,
		},
		{
			name:      "flow map",
			document:  "step: {uses: actions/checkout@" + checkoutSHA + "}\n",
			wantCount: 1,
		},
		{
			name: "multiple documents",
			document: "step: {run: echo first}\n---\n" +
				"step: {uses: actions/checkout@" + checkoutSHA + "}\n",
			wantCount: 1,
		},
		{
			name: "scalar alias value",
			document: "action: &checkout actions/checkout@" + checkoutSHA + "\n" +
				"step:\n  uses: *checkout\n",
			wantCount: 1,
		},
		{
			name: "scalar alias key",
			document: "uses_key: &uses_key uses\n" +
				"step:\n  *uses_key: actions/checkout@" + checkoutSHA + "\n",
			wantCount: 1,
		},
		{
			name: "duplicate uses keys are both scanned",
			document: "step:\n  uses: actions/checkout@" + checkoutSHA + "\n" +
				"  uses: actions/checkout@v7\n",
			wantCount: 2,
			wantErr:   true,
		},
		{
			name:      "mutable version",
			document:  "step: {uses: actions/checkout@v7}\n",
			wantCount: 1,
			wantErr:   true,
		},
		{
			name:      "unapproved action",
			document:  "step: {uses: other/action@" + checkoutSHA + "}\n",
			wantCount: 1,
			wantErr:   true,
		},
		{
			name:      "malformed reference",
			document:  "step: {uses: actions/checkout}\n",
			wantCount: 1,
			wantErr:   true,
		},
		{
			name:     "missing reference",
			document: "step:\n  uses:\n",
			wantErr:  true,
		},
		{
			name:     "non-string uses value",
			document: "step: {uses: [actions/checkout@" + checkoutSHA + "]}\n",
			wantErr:  true,
		},
		{
			name:     "malformed YAML",
			document: "step: [\n",
			wantErr:  true,
		},
		{
			name: "unrelated uses-like text",
			document: "step:\n  run: 'echo uses: actions/checkout@v7'\n" +
				"uses_note: uses\n",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			references, err := scanWorkflowUsesDocuments([]byte(test.document))
			if err == nil {
				err = validateWorkflowActionReferences(references, allowed)
			}
			if len(references) != test.wantCount {
				t.Fatalf("references=%d want=%d (%+v)", len(references), test.wantCount, references)
			}
			if (err != nil) != test.wantErr {
				t.Fatalf("error=%v wantErr=%t", err, test.wantErr)
			}
		})
	}
}

func TestWorkflowUsesScannerMemoizesAliasDAGAndFindsLaterMutableReference(t *testing.T) {
	var document strings.Builder
	document.WriteString("level0: &level0 [{run: echo bounded}]\n")
	for level := 1; level <= 24; level++ {
		fmt.Fprintf(&document, "level%d: &level%d [*level%d, *level%d]\n",
			level, level, level-1, level-1)
	}
	document.WriteString("expanded: *level24\n")
	document.WriteString("late_step: {uses: actions/checkout@v7}\n")

	references, err := scanWorkflowUsesDocumentsWithLimits(
		[]byte(document.String()),
		workflowYAMLScanLimits{maxDocuments: 2, maxUniqueNodes: 256},
	)
	if err != nil {
		t.Fatalf("bounded alias DAG rejected before action validation: %v", err)
	}
	if len(references) != 1 || references[0].reference != "actions/checkout@v7" {
		t.Fatalf("later mutable reference not found: %+v", references)
	}
	allowed := map[string]workflowActionPin{
		"actions/checkout": {sha: "3d3c42e5aac5ba805825da76410c181273ba90b1"},
	}
	if err := validateWorkflowActionReferences(references, allowed); err == nil {
		t.Fatal("later mutable action reference accepted")
	}
}

func TestWorkflowUsesScannerVisitsAliasedMappingOnce(t *testing.T) {
	sha := "3d3c42e5aac5ba805825da76410c181273ba90b1"
	document := "shared: &shared {uses: actions/checkout@" + sha + "}\n" +
		"steps: [*shared, *shared]\n"
	references, err := scanWorkflowUsesDocuments([]byte(document))
	if err != nil {
		t.Fatal(err)
	}
	if len(references) != 1 || references[0].reference != "actions/checkout@"+sha {
		t.Fatalf("aliased mapping was skipped or revisited: %+v", references)
	}
}

func TestWorkflowUsesScannerRejectsResourceBudgets(t *testing.T) {
	t.Run("document budget", func(t *testing.T) {
		_, err := scanWorkflowUsesDocumentsWithLimits(
			[]byte("{}\n---\n{}\n"),
			workflowYAMLScanLimits{maxDocuments: 1, maxUniqueNodes: 32},
		)
		if err == nil || !strings.Contains(err.Error(), "document budget") {
			t.Fatalf("error=%v", err)
		}
	})

	t.Run("unique node budget", func(t *testing.T) {
		_, err := scanWorkflowUsesDocumentsWithLimits(
			[]byte("steps: [{run: one}, {run: two}, {run: three}]\n"),
			workflowYAMLScanLimits{maxDocuments: 1, maxUniqueNodes: 8},
		)
		if err == nil || !strings.Contains(err.Error(), "unique-node budget") {
			t.Fatalf("error=%v", err)
		}
	})
}

func TestWorkflowUsesScannerRejectsCyclicAlias(t *testing.T) {
	_, err := scanWorkflowUsesDocuments([]byte("loop: &loop [*loop]\n"))
	if err == nil || !strings.Contains(err.Error(), "cyclic YAML alias") {
		t.Fatalf("error=%v", err)
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

func addThirdSnapshotIssue(document map[string]any) {
	document["issues"] = append(document["issues"].([]any), map[string]any{
		"number":         103,
		"state":          "open",
		"updated_at":     "2026-07-27T19:00:00Z",
		"content_digest": strings.Repeat("5", 64),
	})
}

func pinnedArchitectureDiscoveryValidator(t *testing.T) func(string) bool {
	t.Helper()
	required := os.Getenv("AO_ARCHITECTURE_PARITY_REQUIRED") == "true"
	repository := os.Getenv("AO_ARCHITECTURE_REPO")
	if repository == "" {
		if required {
			t.Fatal("AO_ARCHITECTURE_REPO is required when AO_ARCHITECTURE_PARITY_REQUIRED=true")
		}
		workingDirectory, err := os.Getwd()
		if err != nil {
			t.Fatal(err)
		}
		repository = filepath.Clean(filepath.Join(workingDirectory, "..", "..", "..", "..", "public", "ao-architecture"))
	}
	commit := githubIssueRepairSourceCommit
	if err := exec.Command("git", "-C", repository, "cat-file", "-e", commit+"^{commit}").Run(); err != nil {
		if required {
			t.Fatalf("required pinned AO Architecture checkout unavailable at %s: %v", repository, err)
		}
		t.Skipf("pinned AO Architecture checkout unavailable at %s: %v", repository, err)
	}
	if required {
		head, err := exec.Command("git", "-C", repository, "rev-parse", "HEAD").Output()
		if err != nil {
			t.Fatalf("read required AO Architecture HEAD: %v", err)
		}
		if strings.TrimSpace(string(head)) != commit {
			t.Fatalf("AO Architecture HEAD=%s want=%s", strings.TrimSpace(string(head)), commit)
		}
	}
	show := func(path string) []byte {
		t.Helper()
		body, err := exec.Command("git", "-C", repository, "show", commit+":"+path).Output()
		if err != nil {
			t.Fatalf("read pinned Architecture %s: %v", path, err)
		}
		return body
	}
	root := t.TempDir()
	scriptDirectory := filepath.Join(root, "scripts")
	schemaDirectory := filepath.Join(root, "stack", "schemas", "github-issue-repair")
	if err := os.MkdirAll(scriptDirectory, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(schemaDirectory, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(
		filepath.Join(scriptDirectory, "github_issue_autonomous_contracts.py"),
		show("scripts/github_issue_autonomous_contracts.py"),
		0o600,
	); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(
		filepath.Join(schemaDirectory, "bounded-discovery-result-v1.schema.json"),
		show("stack/schemas/github-issue-repair/bounded-discovery-result-v1.schema.json"),
		0o600,
	); err != nil {
		t.Fatal(err)
	}
	runner := `
import json
import sys
sys.path.insert(0, sys.argv[1])
from github_issue_autonomous_contracts import validate_contract_instance
with open(sys.argv[2], encoding="utf-8") as source:
    document = json.load(source)
errors = validate_contract_instance("bounded_discovery_result", document)
sys.exit(0 if not errors else 1)
`
	return func(documentPath string) bool {
		command := exec.Command("python3", "-c", runner, scriptDirectory, documentPath)
		return command.Run() == nil
	}
}
