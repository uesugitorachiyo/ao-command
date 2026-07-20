package cli

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strings"
	"testing"
)

const (
	releaseVersion     = "0.1.0"
	releaseTag         = "v0.1.0"
	releaseSource      = "1111111111111111111111111111111111111111"
	releaseNotesDigest = "2222222222222222222222222222222222222222222222222222222222222222"
)

func TestReleaseRehearsalWorkflowStructure(t *testing.T) {
	workflow := readReleaseWorkflow(t)
	document := parseReleaseWorkflow(t)

	triggers := requireObject(t, document["on"], "on")
	if len(triggers) != 1 || triggers["workflow_dispatch"] == nil {
		t.Fatalf("release rehearsal must be workflow_dispatch-only: %#v", triggers)
	}
	dispatch := requireObject(t, triggers["workflow_dispatch"], "workflow_dispatch")
	inputs := requireObject(t, dispatch["inputs"], "workflow_dispatch.inputs")
	for _, input := range []string{
		"version",
		"tag",
		"source_commit",
		"approved_manifest_base64",
		"approved_manifest_digest",
		"release_notes",
		"dry_run",
		"expected_plan_digest",
		"exact_confirmation",
	} {
		if inputs[input] == nil {
			t.Errorf("workflow_dispatch missing input %q", input)
		}
	}
	if len(inputs) > 10 {
		t.Fatalf("workflow_dispatch has %d inputs; GitHub permits at most 10", len(inputs))
	}
	dryRun := requireObject(t, inputs["dry_run"], "dry_run")
	if dryRun["default"] != true {
		t.Fatalf("dry_run must default true: %#v", dryRun)
	}
	if strings.Contains(workflow, "allow_existing_exact_release") {
		t.Fatal("existing tag or release override must not exist")
	}

	globalPermissions := requireObject(t, document["permissions"], "permissions")
	if len(globalPermissions) != 1 || globalPermissions["contents"] != "read" {
		t.Fatalf("global permissions must be contents: read: %#v", globalPermissions)
	}
	jobs := requireObject(t, document["jobs"], "jobs")
	for name, rawJob := range jobs {
		job := requireObject(t, rawJob, "job "+name)
		permissions := requireObject(t, job["permissions"], "permissions for "+name)
		want := map[string]any{"contents": "read"}
		switch name {
		case "publish":
			want = map[string]any{"contents": "write"}
		case "live-preflight":
			want = map[string]any{"actions": "read", "contents": "read"}
		}
		if !objectsEqual(permissions, want) {
			t.Errorf("job %q permissions = %#v, want %#v", name, permissions, want)
		}
	}

	publisher := requireObject(t, jobs["publish"], "publish")
	if publisher["environment"] != "protected-release" {
		t.Fatalf("publisher environment = %#v", publisher["environment"])
	}
	const publisherCondition = "${{ inputs.dry_run == false && needs.validate-inputs.result == 'success' && needs.live-preflight.result == 'success' && needs.assemble-plan.result == 'success' && inputs.exact_confirmation == format('publish-ao-command-{0}-{1}-{2}-{3}-{4}-{5}', inputs.version, inputs.tag, inputs.source_commit, inputs.approved_manifest_digest, needs.validate-inputs.outputs.release_notes_digest, inputs.expected_plan_digest) }}"
	if publisher["if"] != publisherCondition {
		t.Fatalf("publisher condition = %#v, want %q", publisher["if"], publisherCondition)
	}
	preflight := requireObject(t, jobs["live-preflight"], "live-preflight")
	if preflight["if"] != "${{ inputs.dry_run == false }}" {
		t.Fatalf("live preflight condition = %#v", preflight["if"])
	}
	preflightBody, err := json.Marshal(preflight)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"repos/$GITHUB_REPOSITORY/environments/protected-release",
		"required_reviewers",
		"protected-release environment is absent or unreadable",
		"protected-release environment requires configured reviewers",
	} {
		if !strings.Contains(string(preflightBody), want) {
			t.Errorf("live preflight missing %q", want)
		}
	}

	actionUse := regexp.MustCompile(`(?m)^\s*uses:\s+([^#\s]+)`)
	uses := actionUse.FindAllStringSubmatch(workflow, -1)
	if len(uses) == 0 {
		t.Fatal("release rehearsal workflow has no actions to validate")
	}
	fullPin := regexp.MustCompile(`^actions/[a-z0-9-]+@[0-9a-f]{40}$`)
	for _, match := range uses {
		if !fullPin.MatchString(match[1]) {
			t.Errorf("third-party action is not pinned to a full commit SHA: %s", match[1])
		}
	}

	for _, want := range []string{
		"# release-rehearsal-script: validate-tag-state",
		"python3 scripts/release-rehearsal-verify.py assemble",
		"python3 scripts/release-rehearsal-verify.py verify",
		"base64 --decode",
		"approved-release-manifest.json",
		"docs/release/V${VERSION}-OPERATOR-CLOSEOUT.md",
		"help-smoke.txt",
		"version --json",
		"version-readback.json",
		"github.com/uesugitorachiyo/ao-command/internal/cli.buildVersion",
		"github.com/uesugitorachiyo/ao-command/internal/cli.buildSourceCommit",
		"functional-smoke.json",
		`"provider_calls":false`,
		"mission status --status examples/mission/command-status.ready.json --json",
		"ubuntu-24.04",
		"macos-15",
		"windows-2025",
		"EXPECTED_RUNNER_ARCH",
		"EXPECTED_GOOS",
		"EXPECTED_GOARCH",
		"EXPECTED_EXECUTABLE_FORMAT",
		`"workflow_identity":".github/workflows/release-rehearsal.yml@%s"`,
		`gzip.GzipFile(filename="", fileobj=output, mode="wb", mtime=0)`,
		`ZipInfo(name, date_time=(1980, 1, 1, 0, 0, 0))`,
		`rm -f "$candidate_dir/$binary"`,
		`HTTP(/[0-9.]+)? 404`,
		"gh release create",
		"gh release download",
		"Do not create or modify this environment from this workflow",
	} {
		if !strings.Contains(workflow, want) {
			t.Errorf("release rehearsal workflow missing %q", want)
		}
	}
	if strings.Count(workflow, `HTTP(/[0-9.]+)? 404`) != 2 {
		t.Error("both input and publisher remote-state checks must recognize a definitive GitHub 404")
	}
	for _, forbidden := range []string{"eval ", "secrets.", "GH_PAT", "PERSONAL_ACCESS_TOKEN", "environment create", "api repos/{owner}/{repo}/environments"} {
		if strings.Contains(workflow, forbidden) {
			t.Errorf("release rehearsal workflow must not include %q", forbidden)
		}
	}

	for name, rawJob := range jobs {
		if name == "publish" {
			continue
		}
		body, err := json.Marshal(rawJob)
		if err != nil {
			t.Fatal(err)
		}
		for _, forbidden := range []string{"gh release create", "gh release upload", "git tag", "deploy"} {
			if strings.Contains(string(body), forbidden) {
				t.Errorf("non-publisher job %q contains public mutation %q", name, forbidden)
			}
		}
	}
}

func TestReleaseRehearsalPublisherCreatesAndVerifiesExactTag(t *testing.T) {
	document := parseReleaseWorkflow(t)
	jobs := requireObject(t, document["jobs"], "jobs")
	publisherBody := workflowNodeText(jobs["publish"])
	independentBody := workflowNodeText(jobs["verify-published-release"])

	for _, want := range []string{
		`gh api --method POST "repos/$GITHUB_REPOSITORY/git/refs"`,
		`-f ref="refs/tags/$TAG"`,
		`-f sha="$SOURCE_COMMIT"`,
		`created_tag_commit`,
		`[[ "$created_tag_commit" == "$SOURCE_COMMIT" ]]`,
		`gh release create "$TAG" --verify-tag`,
	} {
		if !strings.Contains(publisherBody, want) {
			t.Errorf("publisher missing atomic exact-tag contract %q", want)
		}
	}
	if strings.Contains(publisherBody, `gh release create "$TAG" --target`) {
		t.Error("publisher must not ask gh release create to synthesize or retarget a tag")
	}
	for _, want := range []string{
		`published_tag_commit`,
		`refs/tags/$TAG`,
		`[[ "$published_tag_commit" == "$SOURCE_COMMIT" ]]`,
	} {
		if !strings.Contains(independentBody, want) {
			t.Errorf("independent verifier missing exact tag-source check %q", want)
		}
	}
}

func TestReleaseRehearsalPublisherRequiresEveryPrerequisiteSuccess(t *testing.T) {
	document := parseReleaseWorkflow(t)
	jobs := requireObject(t, document["jobs"], "jobs")
	publisher := requireObject(t, jobs["publish"], "publish")
	condition, ok := publisher["if"].(string)
	if !ok {
		t.Fatalf("publisher condition is not a string: %#v", publisher["if"])
	}
	if strings.Contains(condition, "always()") {
		t.Fatalf("publisher condition bypasses failed needs: %s", condition)
	}
	for _, prerequisite := range []string{"validate-inputs", "live-preflight", "assemble-plan"} {
		want := "needs." + prerequisite + ".result == 'success'"
		if !strings.Contains(condition, want) {
			t.Errorf("publisher condition missing %q: %s", want, condition)
		}
	}
}

func TestReleaseRehearsalLivePreflightHasBoundedActionsRead(t *testing.T) {
	document := parseReleaseWorkflow(t)
	jobs := requireObject(t, document["jobs"], "jobs")
	preflight := requireObject(t, jobs["live-preflight"], "live-preflight")
	permissions := requireObject(t, preflight["permissions"], "live-preflight.permissions")
	want := map[string]any{"actions": "read", "contents": "read"}
	if !objectsEqual(permissions, want) {
		t.Fatalf("live preflight permissions = %#v, want %#v", permissions, want)
	}
}

func TestReleaseRehearsalCandidateManifestHashIsPortable(t *testing.T) {
	document := parseReleaseWorkflow(t)
	jobs := requireObject(t, document["jobs"], "jobs")
	buildBody := workflowNodeText(jobs["build-candidate"])
	const portableHash = `hashlib.sha256(pathlib.Path("approved-manifest/approved-release-manifest.json").read_bytes()).hexdigest()`
	if !strings.Contains(buildBody, portableHash) {
		t.Fatalf("candidate manifest hashing missing portable Python SHA-256: %s", buildBody)
	}
	if strings.Contains(buildBody, "sha256sum approved-manifest/approved-release-manifest.json") {
		t.Fatal("candidate manifest hashing must not require GNU sha256sum on macOS")
	}
	if strings.Contains(buildBody, `sha256sum "$candidate_dir/$archive"`) ||
		strings.Contains(buildBody, `shasum -a 256 "$candidate_dir/$archive"`) {
		t.Fatal("candidate checksum sidecar must not inherit platform-specific filename formatting")
	}
	if !strings.Contains(buildBody, `(root / "SHA256SUMS").write_bytes(f"{digest}  {archive}\n".encode("ascii"))`) {
		t.Fatal("candidate checksum sidecar must use a canonical digest and archive basename record")
	}
}

func TestBuiltBinaryVersionCommandReportsInjectedIdentity(t *testing.T) {
	root := filepath.Join("..", "..")
	binary := filepath.Join(t.TempDir(), "ao-command")
	if runtime.GOOS == "windows" {
		binary += ".exe"
	}
	ldflags := fmt.Sprintf(
		"-X github.com/uesugitorachiyo/ao-command/internal/cli.buildVersion=%s -X github.com/uesugitorachiyo/ao-command/internal/cli.buildSourceCommit=%s",
		releaseVersion,
		releaseSource,
	)
	build := exec.Command("go", "build", "-trimpath", "-ldflags", ldflags, "-o", binary, "./cmd/ao-command")
	build.Dir = root
	if output, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build injected binary: %v\n%s", err, output)
	}
	command := exec.Command(binary, "version", "--json")
	output, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("run injected version command: %v\n%s", err, output)
	}
	var got struct {
		Version       string `json:"version"`
		SourceCommit  string `json:"source_commit"`
		ProviderCalls bool   `json:"provider_calls"`
	}
	if err := json.Unmarshal(output, &got); err != nil {
		t.Fatalf("decode version output: %v\n%s", err, output)
	}
	if got.Version != releaseVersion || got.SourceCommit != releaseSource || got.ProviderCalls {
		t.Fatalf("unexpected injected version identity: %+v", got)
	}
}

func TestAssemblePlanAcceptsBoundManifestAndCandidates(t *testing.T) {
	fixture := newReleaseFixture(t)
	output, err := runReleaseVerifier("assemble", fixture.root, fixture.env)
	if err != nil {
		t.Fatalf("valid release fixture rejected: %v\n%s", err, output)
	}
	for _, name := range []string{"release-rehearsal-plan.json", "release-rehearsal-plan.sha256", "dry-run-boundary.json"} {
		if _, err := os.Stat(filepath.Join(fixture.root, "target", "release-rehearsal-plan", name)); err != nil {
			t.Fatalf("missing plan output %s: %v", name, err)
		}
	}
}

func TestAssemblePlanRejectsNegativeFixtures(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*testing.T, *releaseFixture)
		want   string
	}{
		{
			name: "manifest_digest_mismatch",
			mutate: func(_ *testing.T, fixture *releaseFixture) {
				fixture.env["APPROVED_MANIFEST_DIGEST"] = strings.Repeat("0", 64)
			},
			want: "approved manifest digest mismatch",
		},
		{
			name: "malformed_manifest",
			mutate: func(t *testing.T, fixture *releaseFixture) {
				writeTestFile(t, fixture.manifestPath, []byte("{"))
				fixture.refreshManifestDigest(t)
			},
			want: "approved manifest malformed",
		},
		{
			name: "oversized_manifest",
			mutate: func(t *testing.T, fixture *releaseFixture) {
				writeTestFile(t, fixture.manifestPath, bytes.Repeat([]byte(" "), 64*1024+1))
				fixture.refreshManifestDigest(t)
			},
			want: "approved manifest exceeds bounded size",
		},
		{
			name: "manifest_inventory_mismatch",
			mutate: func(t *testing.T, fixture *releaseFixture) {
				manifest := readJSONObject(t, fixture.manifestPath)
				candidates := manifest["candidates"].([]any)
				candidates[0].(map[string]any)["archive_sha256"] = strings.Repeat("f", 64)
				writeJSONFile(t, fixture.manifestPath, manifest)
				fixture.refreshManifestDigest(t)
				fixture.syncCandidateManifestDigest(t)
			},
			want: "approved manifest candidate mismatch",
		},
		{
			name: "missing_candidate",
			mutate: func(t *testing.T, fixture *releaseFixture) {
				if err := os.RemoveAll(fixture.candidateDirs["linux-x86_64"]); err != nil {
					t.Fatal(err)
				}
			},
			want: "unexpected artifact inventory",
		},
		{
			name: "duplicate_candidate",
			mutate: func(t *testing.T, fixture *releaseFixture) {
				copyTestDir(t, fixture.candidateDirs["linux-x86_64"], filepath.Join(fixture.root, "downloaded-candidates", "duplicate"))
			},
			want: "unexpected artifact inventory",
		},
		{
			name: "stale_candidate",
			mutate: func(t *testing.T, fixture *releaseFixture) {
				path := filepath.Join(fixture.candidateDirs["macos-aarch64"], "candidate-summary.json")
				summary := readJSONObject(t, path)
				summary["source_commit"] = strings.Repeat("9", 40)
				writeJSONFile(t, path, summary)
			},
			want: "source_commit drift",
		},
		{
			name: "substituted_archive",
			mutate: func(t *testing.T, fixture *releaseFixture) {
				path := filepath.Join(fixture.candidateDirs["windows-x86_64"], "ao-command-0.1.0-windows-x86_64.zip")
				writeTestFile(t, path, []byte("substituted"))
			},
			want: "candidate archive checksum mismatch",
		},
		{
			name: "target_identity_substitution",
			mutate: func(t *testing.T, fixture *releaseFixture) {
				path := filepath.Join(fixture.candidateDirs["macos-aarch64"], "candidate-summary.json")
				summary := readJSONObject(t, path)
				summary["goarch"] = "amd64"
				writeJSONFile(t, path, summary)
			},
			want: "candidate target identity mismatch",
		},
		{
			name: "archive_architecture_substitution",
			mutate: func(t *testing.T, fixture *releaseFixture) {
				fixture.replaceArchiveExecutable(t, "macos-aarch64", executableFixture("elf"))
			},
			want: "archive executable format or architecture mismatch",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			fixture := newReleaseFixture(t)
			test.mutate(t, fixture)
			output, err := runReleaseVerifier("assemble", fixture.root, fixture.env)
			if err == nil {
				t.Fatalf("negative fixture unexpectedly passed:\n%s", output)
			}
			if !strings.Contains(output, test.want) {
				t.Fatalf("failure output missing %q:\n%s", test.want, output)
			}
		})
	}
}

func TestTagStateValidatorAllowsReadOnlyRehearsalAndFailsClosedForLiveUse(t *testing.T) {
	script := extractReleaseWorkflowScript(t, "validate-tag-state")
	tests := []struct {
		name    string
		env     map[string]string
		wantErr string
	}{
		{
			name: "live_tag_and_release_absent",
			env:  map[string]string{"DRY_RUN": "false", "REMOTE_TAG_COMMIT": "", "RELEASE_EXISTS": "false"},
		},
		{
			name: "dry_run_existing_tag_and_release",
			env:  map[string]string{"DRY_RUN": "true", "REMOTE_TAG_COMMIT": releaseSource, "RELEASE_EXISTS": "true"},
		},
		{
			name:    "tag_source_drift",
			env:     map[string]string{"DRY_RUN": "false", "REMOTE_TAG_COMMIT": strings.Repeat("9", 40), "RELEASE_EXISTS": "false"},
			wantErr: "remote tag already exists",
		},
		{
			name:    "existing_exact_tag",
			env:     map[string]string{"DRY_RUN": "false", "REMOTE_TAG_COMMIT": releaseSource, "RELEASE_EXISTS": "false"},
			wantErr: "remote tag already exists",
		},
		{
			name:    "existing_release",
			env:     map[string]string{"DRY_RUN": "false", "REMOTE_TAG_COMMIT": "", "RELEASE_EXISTS": "true"},
			wantErr: "remote release already exists",
		},
		{
			name:    "malformed_mode",
			env:     map[string]string{"DRY_RUN": "maybe", "REMOTE_TAG_COMMIT": "", "RELEASE_EXISTS": "false"},
			wantErr: "dry-run state is malformed",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			env := map[string]string{"SOURCE_COMMIT": releaseSource}
			for key, value := range test.env {
				env[key] = value
			}
			output, err := runReleaseWorkflowScript(script, t.TempDir(), env)
			if test.wantErr == "" {
				if err != nil {
					t.Fatalf("valid tag state rejected: %v\n%s", err, output)
				}
				return
			}
			if err == nil || !strings.Contains(output, test.wantErr) {
				t.Fatalf("tag state failure = %v, output %q, want %q", err, output, test.wantErr)
			}
		})
	}
}

func TestEnvironmentValidatorRequiresConfiguredReviewers(t *testing.T) {
	script := extractReleaseWorkflowScript(t, "validate-environment")
	tests := []struct {
		name     string
		document string
		wantErr  string
	}{
		{
			name:     "protected",
			document: `{"name":"protected-release","protection_rules":[{"type":"required_reviewers","reviewers":[{"type":"User","reviewer":{"id":1}}]}]}`,
		},
		{
			name:     "wrong_environment",
			document: `{"name":"other","protection_rules":[{"type":"required_reviewers","reviewers":[{"type":"User","reviewer":{"id":1}}]}]}`,
			wantErr:  "protected-release environment identity mismatch",
		},
		{
			name:     "no_required_reviewers",
			document: `{"name":"protected-release","protection_rules":[]}`,
			wantErr:  "protected-release environment requires configured reviewers",
		},
		{
			name:     "empty_reviewer_set",
			document: `{"name":"protected-release","protection_rules":[{"type":"required_reviewers","reviewers":[]}]}`,
			wantErr:  "protected-release environment requires configured reviewers",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			output, err := runReleaseWorkflowScript(script, t.TempDir(), map[string]string{"ENVIRONMENT_JSON": test.document})
			if test.wantErr == "" {
				if err != nil {
					t.Fatalf("protected environment rejected: %v\n%s", err, output)
				}
				return
			}
			if err == nil || !strings.Contains(output, test.wantErr) {
				t.Fatalf("environment failure = %v, output %q, want %q", err, output, test.wantErr)
			}
		})
	}
}

func TestPublisherVerifierBindsCompletePlan(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*testing.T, *releaseFixture, map[string]any)
		want   string
	}{
		{
			name: "confirmation_mismatch",
			mutate: func(_ *testing.T, fixture *releaseFixture, _ map[string]any) {
				fixture.env["EXACT_CONFIRMATION"] = "wrong"
			},
			want: "exact confirmation mismatch",
		},
		{
			name: "plan_digest_mismatch",
			mutate: func(_ *testing.T, fixture *releaseFixture, _ map[string]any) {
				fixture.env["EXPECTED_PLAN_DIGEST"] = strings.Repeat("0", 64)
			},
			want: "plan digest mismatch",
		},
		{
			name: "schema_mismatch",
			mutate: func(_ *testing.T, _ *releaseFixture, plan map[string]any) {
				plan["schema_version"] = "wrong"
			},
			want: "plan schema mismatch",
		},
		{
			name: "mutable_plan",
			mutate: func(_ *testing.T, _ *releaseFixture, plan map[string]any) {
				plan["immutable"] = false
			},
			want: "plan must be immutable",
		},
		{
			name: "source_drift",
			mutate: func(_ *testing.T, _ *releaseFixture, plan map[string]any) {
				plan["source_commit"] = strings.Repeat("8", 40)
			},
			want: "plan source_commit mismatch",
		},
		{
			name: "version_drift",
			mutate: func(_ *testing.T, _ *releaseFixture, plan map[string]any) {
				plan["version"] = "9.9.9"
			},
			want: "plan version mismatch",
		},
		{
			name: "tag_drift",
			mutate: func(_ *testing.T, _ *releaseFixture, plan map[string]any) {
				plan["tag"] = "v9.9.9"
			},
			want: "plan tag mismatch",
		},
		{
			name: "manifest_digest_drift",
			mutate: func(_ *testing.T, _ *releaseFixture, plan map[string]any) {
				plan["approved_manifest_digest"] = strings.Repeat("7", 64)
			},
			want: "plan approved_manifest_digest mismatch",
		},
		{
			name: "release_notes_digest_drift",
			mutate: func(_ *testing.T, _ *releaseFixture, plan map[string]any) {
				plan["release_notes_digest"] = strings.Repeat("6", 64)
			},
			want: "plan release_notes_digest mismatch",
		},
		{
			name: "duplicate_candidate",
			mutate: func(_ *testing.T, _ *releaseFixture, plan map[string]any) {
				candidates := plan["candidates"].([]any)
				plan["candidates"] = append(candidates, candidates[0])
			},
			want: "plan candidate inventory mismatch",
		},
		{
			name: "archive_digest_substitution",
			mutate: func(t *testing.T, fixture *releaseFixture, _ map[string]any) {
				path := filepath.Join(fixture.candidateDirs["linux-x86_64"], "ao-command-0.1.0-linux-x86_64.tar.gz")
				writeTestFile(t, path, []byte("substituted after plan assembly"))
			},
			want: "candidate archive checksum mismatch",
		},
		{
			name: "inventory_substitution",
			mutate: func(t *testing.T, fixture *releaseFixture, _ map[string]any) {
				path := filepath.Join(fixture.candidateDirs["macos-aarch64"], "LICENSE")
				writeTestFile(t, path, []byte("substituted after plan assembly"))
			},
			want: "candidate inventory mismatch",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			fixture := newReleaseFixture(t)
			if output, err := runReleaseVerifier("assemble", fixture.root, fixture.env); err != nil {
				t.Fatalf("fixture assembly failed: %v\n%s", err, output)
			}
			planPath := filepath.Join(fixture.root, "target", "release-rehearsal-plan", "release-rehearsal-plan.json")
			plan := readJSONObject(t, planPath)
			planChanged := test.name != "confirmation_mismatch" && test.name != "plan_digest_mismatch"
			test.mutate(t, fixture, plan)
			if planChanged {
				writeJSONFile(t, planPath, plan)
				fixture.bindPlanDigest(t, planPath)
			} else {
				fixture.bindPlanDigest(t, planPath)
				test.mutate(t, fixture, plan)
			}
			output, err := runReleaseVerifier("verify", fixture.root, fixture.env)
			if err == nil {
				t.Fatalf("negative publisher fixture unexpectedly passed:\n%s", output)
			}
			if !strings.Contains(output, test.want) {
				t.Fatalf("publisher failure missing %q:\n%s", test.want, output)
			}
		})
	}
}

func TestPublisherVerifierAcceptsCompleteBoundPlan(t *testing.T) {
	fixture := newReleaseFixture(t)
	if output, err := runReleaseVerifier("assemble", fixture.root, fixture.env); err != nil {
		t.Fatalf("fixture assembly failed: %v\n%s", err, output)
	}
	planPath := filepath.Join(fixture.root, "target", "release-rehearsal-plan", "release-rehearsal-plan.json")
	fixture.bindPlanDigest(t, planPath)
	output, err := runReleaseVerifier("verify", fixture.root, fixture.env)
	if err != nil {
		t.Fatalf("complete bound plan rejected: %v\n%s", err, output)
	}
}

func TestPublisherVerifierRejectsMalformedPlan(t *testing.T) {
	fixture := newReleaseFixture(t)
	if output, err := runReleaseVerifier("assemble", fixture.root, fixture.env); err != nil {
		t.Fatalf("fixture assembly failed: %v\n%s", err, output)
	}
	planPath := filepath.Join(fixture.root, "target", "release-rehearsal-plan", "release-rehearsal-plan.json")
	writeTestFile(t, planPath, []byte("{"))
	fixture.bindPlanDigest(t, planPath)
	output, err := runReleaseVerifier("verify", fixture.root, fixture.env)
	if err == nil || !strings.Contains(output, "plan malformed") {
		t.Fatalf("malformed plan failure = %v, output %q", err, output)
	}
}

type releaseFixture struct {
	root          string
	manifestPath  string
	candidateDirs map[string]string
	env           map[string]string
}

func newReleaseFixture(t *testing.T) *releaseFixture {
	t.Helper()
	root := t.TempDir()
	fixture := &releaseFixture{
		root:          root,
		manifestPath:  filepath.Join(root, "approved-manifest", "approved-release-manifest.json"),
		candidateDirs: make(map[string]string),
		env: map[string]string{
			"VERSION":              releaseVersion,
			"TAG":                  releaseTag,
			"SOURCE_COMMIT":        releaseSource,
			"RELEASE_NOTES_DIGEST": releaseNotesDigest,
			"DRY_RUN":              "true",
		},
	}
	type target struct {
		name             string
		extension        string
		binary           string
		runnerArch       string
		goos             string
		goarch           string
		executableFormat string
	}
	targets := []target{
		{name: "linux-x86_64", extension: "tar.gz", binary: "ao-command", runnerArch: "X64", goos: "linux", goarch: "amd64", executableFormat: "elf"},
		{name: "macos-aarch64", extension: "tar.gz", binary: "ao-command", runnerArch: "ARM64", goos: "darwin", goarch: "arm64", executableFormat: "macho"},
		{name: "windows-x86_64", extension: "zip", binary: "ao-command.exe", runnerArch: "X64", goos: "windows", goarch: "amd64", executableFormat: "pe"},
	}
	manifestCandidates := make([]map[string]any, 0, len(targets))
	for _, target := range targets {
		archive := fmt.Sprintf("ao-command-%s-%s.%s", releaseVersion, target.name, target.extension)
		dir := filepath.Join(root, "downloaded-candidates", "ao-command-release-candidate-"+target.name+"-"+releaseSource)
		fixture.candidateDirs[target.name] = dir
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
		provenance := map[string]any{
			"executable_format":    target.executableFormat,
			"goarch":               target.goarch,
			"goos":                 target.goos,
			"provider_calls":       false,
			"release_notes_digest": releaseNotesDigest,
			"repository":           "ao-command",
			"runner_arch":          target.runnerArch,
			"schema_version":       "ao.command.release-rehearsal-provenance.v0.1",
			"source_commit":        releaseSource,
			"target":               target.name,
			"version":              releaseVersion,
			"workflow_identity":    ".github/workflows/release-rehearsal.yml@" + releaseSource,
		}
		provenanceBytes := marshalJSONBytes(t, provenance)
		archiveMembers := map[string][]byte{
			target.binary:           executableFixture(target.executableFormat),
			"LICENSE":               []byte("license"),
			"help-smoke.txt":        []byte("help"),
			"version-readback.json": []byte(`{"provider_calls":false,"schema_version":"ao.command.version.v0.1","source_commit":"` + releaseSource + `","version":"` + releaseVersion + `"}`),
			"functional-smoke.json": []byte(`{"provider_calls":false,"readback":{"command_schema_version":"ao.command.v0.1","operator_mode":"read_only","safe_to_execute":false,"status":"ready"},"schema_version":"ao.command.release-rehearsal-functional-smoke.v0.1","status":"passed"}`),
			"sbom.json":             []byte(`{"modules":[]}`),
			"provenance.json":       provenanceBytes,
		}
		archiveBytes := makeArchiveFixture(t, target.extension, archiveMembers)
		archiveDigest := digestBytes(archiveBytes)
		files := map[string][]byte{
			archive:                 archiveBytes,
			"LICENSE":               archiveMembers["LICENSE"],
			"help-smoke.txt":        archiveMembers["help-smoke.txt"],
			"version-readback.json": archiveMembers["version-readback.json"],
			"functional-smoke.json": archiveMembers["functional-smoke.json"],
			"sbom.json":             archiveMembers["sbom.json"],
			"provenance.json":       archiveMembers["provenance.json"],
			"SHA256SUMS":            []byte(archiveDigest + "  " + archive + "\n"),
		}
		inventory := make([]map[string]string, 0, len(files))
		names := make([]string, 0, len(files))
		for name := range files {
			names = append(names, name)
		}
		sort.Strings(names)
		for _, name := range names {
			writeTestFile(t, filepath.Join(dir, name), files[name])
			inventory = append(inventory, map[string]string{"name": name, "sha256": digestBytes(files[name])})
		}
		summary := map[string]any{
			"approved_manifest_digest": "",
			"archive":                  archive,
			"archive_sha256":           archiveDigest,
			"executable":               target.binary,
			"executable_format":        target.executableFormat,
			"goarch":                   target.goarch,
			"goos":                     target.goos,
			"inventory":                inventory,
			"provider_calls":           false,
			"release_notes_digest":     releaseNotesDigest,
			"repository":               "ao-command",
			"runner_arch":              target.runnerArch,
			"schema_version":           "ao.command.release-rehearsal-candidate.v0.3",
			"smoke": map[string]any{
				"functional":     "passed",
				"help":           "passed",
				"provider_calls": false,
				"version":        "passed",
			},
			"source_commit": releaseSource,
			"tag":           releaseTag,
			"target":        target.name,
			"version":       releaseVersion,
		}
		writeJSONFile(t, filepath.Join(dir, "candidate-summary.json"), summary)
		manifestCandidates = append(manifestCandidates, map[string]any{
			"archive":        archive,
			"archive_sha256": archiveDigest,
			"target":         target.name,
		})
	}
	manifest := map[string]any{
		"candidates":           manifestCandidates,
		"immutable":            true,
		"release_notes_digest": releaseNotesDigest,
		"repository":           "ao-command",
		"schema_version":       "ao.command.approved-release-manifest.v0.1",
		"source_commit":        releaseSource,
		"tag":                  releaseTag,
		"version":              releaseVersion,
	}
	writeJSONFile(t, fixture.manifestPath, manifest)
	fixture.refreshManifestDigest(t)
	for _, dir := range fixture.candidateDirs {
		path := filepath.Join(dir, "candidate-summary.json")
		summary := readJSONObject(t, path)
		summary["approved_manifest_digest"] = fixture.env["APPROVED_MANIFEST_DIGEST"]
		writeJSONFile(t, path, summary)
	}
	return fixture
}

func (fixture *releaseFixture) replaceArchiveExecutable(t *testing.T, target string, executable []byte) {
	t.Helper()
	dir := fixture.candidateDirs[target]
	summaryPath := filepath.Join(dir, "candidate-summary.json")
	summary := readJSONObject(t, summaryPath)
	archive := summary["archive"].(string)
	members := map[string][]byte{
		summary["executable"].(string): executable,
	}
	for _, name := range []string{
		"LICENSE",
		"functional-smoke.json",
		"help-smoke.txt",
		"provenance.json",
		"sbom.json",
		"version-readback.json",
	} {
		data, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil {
			t.Fatal(err)
		}
		members[name] = data
	}
	extension := "tar.gz"
	if strings.HasSuffix(archive, ".zip") {
		extension = "zip"
	}
	archiveBytes := makeArchiveFixture(t, extension, members)
	archiveDigest := digestBytes(archiveBytes)
	writeTestFile(t, filepath.Join(dir, archive), archiveBytes)
	checksumBytes := []byte(archiveDigest + "  " + archive + "\n")
	writeTestFile(t, filepath.Join(dir, "SHA256SUMS"), checksumBytes)
	summary["archive_sha256"] = archiveDigest
	for _, rawItem := range summary["inventory"].([]any) {
		item := rawItem.(map[string]any)
		switch item["name"] {
		case archive:
			item["sha256"] = archiveDigest
		case "SHA256SUMS":
			item["sha256"] = digestBytes(checksumBytes)
		}
	}
	writeJSONFile(t, summaryPath, summary)

	manifest := readJSONObject(t, fixture.manifestPath)
	for _, rawCandidate := range manifest["candidates"].([]any) {
		candidate := rawCandidate.(map[string]any)
		if candidate["target"] == target {
			candidate["archive_sha256"] = archiveDigest
		}
	}
	writeJSONFile(t, fixture.manifestPath, manifest)
	fixture.refreshManifestDigest(t)
	fixture.syncCandidateManifestDigest(t)
}

func (fixture *releaseFixture) refreshManifestDigest(t *testing.T) {
	t.Helper()
	data, err := os.ReadFile(fixture.manifestPath)
	if err != nil {
		t.Fatal(err)
	}
	fixture.env["APPROVED_MANIFEST_DIGEST"] = digestBytes(data)
}

func (fixture *releaseFixture) bindPlanDigest(t *testing.T, planPath string) {
	t.Helper()
	data, err := os.ReadFile(planPath)
	if err != nil {
		t.Fatal(err)
	}
	digest := digestBytes(data)
	checksumPath := filepath.Join(filepath.Dir(planPath), "release-rehearsal-plan.sha256")
	writeTestFile(t, checksumPath, []byte(digest+"  release-rehearsal-plan.json\n"))
	fixture.env["EXPECTED_PLAN_DIGEST"] = digest
	fixture.env["EXACT_CONFIRMATION"] = fmt.Sprintf(
		"publish-ao-command-%s-%s-%s-%s-%s-%s",
		fixture.env["VERSION"],
		fixture.env["TAG"],
		fixture.env["SOURCE_COMMIT"],
		fixture.env["APPROVED_MANIFEST_DIGEST"],
		fixture.env["RELEASE_NOTES_DIGEST"],
		digest,
	)
}

func (fixture *releaseFixture) syncCandidateManifestDigest(t *testing.T) {
	t.Helper()
	for _, dir := range fixture.candidateDirs {
		path := filepath.Join(dir, "candidate-summary.json")
		summary := readJSONObject(t, path)
		summary["approved_manifest_digest"] = fixture.env["APPROVED_MANIFEST_DIGEST"]
		writeJSONFile(t, path, summary)
	}
}

func readReleaseWorkflow(t *testing.T) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("..", "..", ".github", "workflows", "release-rehearsal.yml"))
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

func parseReleaseWorkflow(t *testing.T) map[string]any {
	t.Helper()
	path := filepath.Join("..", "..", ".github", "workflows", "release-rehearsal.yml")
	script := `document = YAML.safe_load(File.read(ARGV[0]), aliases: true); document["on"] = document.delete(true) if document.key?(true); puts JSON.generate(document)`
	command := exec.Command("ruby", "-ryaml", "-rjson", "-e", script, path)
	output, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("parse workflow YAML: %v\n%s", err, output)
	}
	var document map[string]any
	if err := json.Unmarshal(output, &document); err != nil {
		t.Fatalf("decode parsed workflow: %v\n%s", err, output)
	}
	return document
}

func requireObject(t *testing.T, value any, label string) map[string]any {
	t.Helper()
	object, ok := value.(map[string]any)
	if !ok {
		t.Fatalf("%s is not an object: %#v", label, value)
	}
	return object
}

func objectsEqual(left, right map[string]any) bool {
	leftJSON, leftErr := json.Marshal(left)
	rightJSON, rightErr := json.Marshal(right)
	return leftErr == nil && rightErr == nil && bytes.Equal(leftJSON, rightJSON)
}

func workflowNodeText(value any) string {
	switch node := value.(type) {
	case string:
		return node + "\n"
	case []any:
		var text strings.Builder
		for _, child := range node {
			text.WriteString(workflowNodeText(child))
		}
		return text.String()
	case map[string]any:
		var text strings.Builder
		for _, child := range node {
			text.WriteString(workflowNodeText(child))
		}
		return text.String()
	default:
		return ""
	}
}

func extractReleaseWorkflowScript(t *testing.T, name string) string {
	t.Helper()
	workflow := readReleaseWorkflow(t)
	marker := "# release-rehearsal-script: " + name
	markerIndex := strings.Index(workflow, marker)
	if markerIndex < 0 {
		t.Fatalf("workflow missing executable script marker %q", marker)
	}
	heredoc := "python3 - <<'PY'\n"
	start := strings.Index(workflow[markerIndex:], heredoc)
	if start < 0 {
		t.Fatalf("workflow marker %q has no Python heredoc", marker)
	}
	start += markerIndex + len(heredoc)
	end := strings.Index(workflow[start:], "\n          PY")
	if end < 0 {
		t.Fatalf("workflow marker %q has no Python heredoc terminator", marker)
	}
	lines := strings.Split(workflow[start:start+end], "\n")
	for index := range lines {
		lines[index] = strings.TrimPrefix(lines[index], "          ")
	}
	return strings.Join(lines, "\n")
}

func runReleaseWorkflowScript(script, dir string, environment map[string]string) (string, error) {
	command := exec.Command("python3", "-c", script)
	command.Dir = dir
	command.Env = os.Environ()
	for key, value := range environment {
		command.Env = append(command.Env, key+"="+value)
	}
	output, err := command.CombinedOutput()
	return string(output), err
}

func runReleaseVerifier(mode, dir string, environment map[string]string) (string, error) {
	path, err := filepath.Abs(filepath.Join("..", "..", "scripts", "release-rehearsal-verify.py"))
	if err != nil {
		return "", err
	}
	command := exec.Command("python3", path, mode)
	command.Dir = dir
	command.Env = os.Environ()
	for key, value := range environment {
		command.Env = append(command.Env, key+"="+value)
	}
	output, err := command.CombinedOutput()
	return string(output), err
}

func readJSONObject(t *testing.T, path string) map[string]any {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var object map[string]any
	if err := json.Unmarshal(data, &object); err != nil {
		t.Fatalf("decode %s: %v", path, err)
	}
	return object
}

func writeJSONFile(t *testing.T, path string, value any) {
	t.Helper()
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	writeTestFile(t, path, append(data, '\n'))
}

func writeTestFile(t *testing.T, path string, data []byte) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatal(err)
	}
}

func copyTestDir(t *testing.T, source, destination string) {
	t.Helper()
	if err := filepath.WalkDir(source, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		relative, err := filepath.Rel(source, path)
		if err != nil {
			return err
		}
		target := filepath.Join(destination, relative)
		if entry.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		return os.WriteFile(target, data, 0o644)
	}); err != nil {
		t.Fatal(err)
	}
}

func digestBytes(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

func marshalJSONBytes(t *testing.T, value any) []byte {
	t.Helper()
	data, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func executableFixture(format string) []byte {
	switch format {
	case "elf":
		data := make([]byte, 64)
		copy(data, []byte{0x7f, 'E', 'L', 'F'})
		data[4] = 2
		data[5] = 1
		binary.LittleEndian.PutUint16(data[18:20], 62)
		return data
	case "macho":
		data := make([]byte, 32)
		copy(data, []byte{0xcf, 0xfa, 0xed, 0xfe})
		binary.LittleEndian.PutUint32(data[4:8], 0x0100000c)
		return data
	case "pe":
		data := make([]byte, 128)
		copy(data, []byte{'M', 'Z'})
		binary.LittleEndian.PutUint32(data[0x3c:0x40], 0x40)
		copy(data[0x40:0x44], []byte{'P', 'E', 0, 0})
		binary.LittleEndian.PutUint16(data[0x44:0x46], 0x8664)
		return data
	default:
		panic("unsupported executable fixture format: " + format)
	}
}

func makeArchiveFixture(t *testing.T, extension string, members map[string][]byte) []byte {
	t.Helper()
	names := make([]string, 0, len(members))
	for name := range members {
		names = append(names, name)
	}
	sort.Strings(names)
	var output bytes.Buffer
	if extension == "zip" {
		writer := zip.NewWriter(&output)
		for _, name := range names {
			header := &zip.FileHeader{Name: name, Method: zip.Deflate}
			header.SetMode(0o644)
			entry, err := writer.CreateHeader(header)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := entry.Write(members[name]); err != nil {
				t.Fatal(err)
			}
		}
		if err := writer.Close(); err != nil {
			t.Fatal(err)
		}
		return output.Bytes()
	}
	compressed := gzip.NewWriter(&output)
	writer := tar.NewWriter(compressed)
	for _, name := range names {
		data := members[name]
		header := &tar.Header{Name: name, Mode: 0o644, Size: int64(len(data))}
		if err := writer.WriteHeader(header); err != nil {
			t.Fatal(err)
		}
		if _, err := writer.Write(data); err != nil {
			t.Fatal(err)
		}
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	if err := compressed.Close(); err != nil {
		t.Fatal(err)
	}
	return output.Bytes()
}
