package cli

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

const protectedPublisherCondition = "${{ inputs.dry_run == false && needs.validate-inputs.result == 'success' && needs.live-preflight.result == 'success' && needs.assemble-plan.result == 'success' && inputs.exact_confirmation == format('publish-ao-command-{0}-{1}-{2}-{3}-{4}-{5}', inputs.version, inputs.tag, inputs.source_commit, inputs.approved_manifest_digest, needs.validate-inputs.outputs.release_notes_digest, inputs.expected_plan_digest) }}"

func TestCIArtifactUploadPolicyAcceptsStrictManualReadOnlyEvidence(t *testing.T) {
	root := repoRoot(t)
	for _, workflow := range []string{
		".github/workflows/release-rehearsal.yml",
		".github/workflows/native-artifacts.yml",
	} {
		output, err := runCIArtifactUploadPolicy(t, root, filepath.Join(root, workflow))
		if err != nil {
			t.Fatalf("strict manual evidence workflow %s rejected: %v\n%s", workflow, err, output)
		}
	}
}

func TestCIArtifactUploadPolicyRejectsDefaultMixedAndWritableWorkflows(t *testing.T) {
	tests := []struct {
		name     string
		workflow string
		want     string
	}{
		{
			name: "pull_request",
			workflow: workflowWithUpload(`
  pull_request:
`),
			want: "workflow_dispatch-only",
		},
		{
			name: "push",
			workflow: workflowWithUpload(`
  push:
    branches: [main]
`),
			want: "workflow_dispatch-only",
		},
		{
			name: "schedule",
			workflow: workflowWithUpload(`
  schedule:
    - cron: "0 0 * * *"
`),
			want: "workflow_dispatch-only",
		},
		{
			name: "mixed_manual_and_default",
			workflow: workflowWithUpload(`
  workflow_dispatch:
  pull_request:
`),
			want: "workflow_dispatch-only",
		},
		{
			name: "global_write",
			workflow: strings.Replace(
				workflowWithUpload("\n  workflow_dispatch:\n"),
				"contents: read",
				"contents: write",
				1,
			),
			want: "workflow permissions must be explicit read-only",
		},
		{
			name: "job_write",
			workflow: strings.Replace(
				workflowWithUpload("\n  workflow_dispatch:\n"),
				"    steps:",
				"    permissions:\n      contents: write\n    steps:",
				1,
			),
			want: "job evidence artifact uploads require contents: read and no writes",
		},
		{
			name: "missing_permissions",
			workflow: strings.Replace(
				workflowWithUpload("\n  workflow_dispatch:\n"),
				"permissions:\n  contents: read\n\n",
				"",
				1,
			),
			want: "workflow permissions must be explicit read-only",
		},
		{
			name: "public_release_upload",
			workflow: strings.Replace(
				workflowWithUpload("\n  workflow_dispatch:\n"),
				"uses: actions/upload-artifact@0123456789012345678901234567890123456789",
				"run: gh release upload v1 evidence.json",
				1,
			),
			want: "public release command requires protected publisher",
		},
		{
			name: "separate_writer_bypass",
			workflow: workflowWithUpload("\n  workflow_dispatch:\n") + `
  writer:
    runs-on: ubuntu-latest
    permissions:
      contents: write
    steps:
      - run: echo separate writer
`,
			want: "job writer has forbidden write permissions",
		},
		{
			name: "release_create_assets_bypass",
			workflow: workflowWithUpload("\n  workflow_dispatch:\n") + `
  release:
    runs-on: ubuntu-latest
    permissions:
      contents: read
    steps:
      - run: gh release create v1 evidence.tar.gz
`,
			want: "public release command requires protected publisher",
		},
		{
			name:     "malformed",
			workflow: "name: broken\non: [",
			want:     "malformed workflow",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "workflow.yml")
			if err := os.WriteFile(path, []byte(test.workflow), 0o644); err != nil {
				t.Fatal(err)
			}
			output, err := runCIArtifactUploadPolicy(t, repoRoot(t), path)
			if err == nil {
				t.Fatalf("unsafe workflow unexpectedly accepted:\n%s", output)
			}
			if !strings.Contains(output, test.want) {
				t.Fatalf("failure output missing %q:\n%s", test.want, output)
			}
		})
	}
}

func TestCIArtifactUploadPolicyAcceptsIsolatedEvidenceAndProtectedPublisher(t *testing.T) {
	path := filepath.Join(t.TempDir(), "workflow.yml")
	if err := os.WriteFile(path, []byte(workflowWithProtectedPublisher()), 0o644); err != nil {
		t.Fatal(err)
	}
	output, err := runCIArtifactUploadPolicy(t, repoRoot(t), path)
	if err != nil {
		t.Fatalf("isolated evidence and protected publisher rejected: %v\n%s", err, output)
	}
}

func TestCIArtifactUploadPolicyRejectsIncompleteOrTautologicalPublisherGuards(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(string) string
	}{
		{
			name: "omitted_immutable_plan_prerequisite",
			mutate: func(workflow string) string {
				return strings.Replace(
					workflow,
					"needs: [validate-inputs, live-preflight, assemble-plan]",
					"needs: [validate-inputs, live-preflight]",
					1,
				)
			},
		},
		{
			name: "dry_run_tautology",
			mutate: func(workflow string) string {
				return strings.Replace(
					workflow,
					"inputs.dry_run == false",
					"(inputs.dry_run == false || true)",
					1,
				)
			},
		},
		{
			name: "prerequisite_tautology",
			mutate: func(workflow string) string {
				return strings.Replace(
					workflow,
					"needs.assemble-plan.result == 'success'",
					"(needs.assemble-plan.result == 'success' || true)",
					1,
				)
			},
		},
		{
			name: "confirmation_tautology",
			mutate: func(workflow string) string {
				return strings.Replace(
					workflow,
					protectedPublisherCondition,
					strings.TrimSuffix(protectedPublisherCondition, " }}")+" || true }}",
					1,
				)
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "workflow.yml")
			if err := os.WriteFile(path, []byte(test.mutate(workflowWithProtectedPublisher())), 0o644); err != nil {
				t.Fatal(err)
			}
			output, err := runCIArtifactUploadPolicy(t, repoRoot(t), path)
			if err == nil {
				t.Fatalf("unsafe protected publisher unexpectedly accepted:\n%s", output)
			}
			if !strings.Contains(output, "job publish has forbidden write permissions") {
				t.Fatalf("unexpected failure output:\n%s", output)
			}
		})
	}
}

func TestCIArtifactUploadPolicyRejectsBrokenProtectedPublisherDependencyChain(t *testing.T) {
	tests := []struct {
		name   string
		old    string
		unsafe string
	}{
		{
			name:   "assemble_plan_omits_build_candidate",
			old:    "needs: [validate-inputs, build-candidate]",
			unsafe: "needs: validate-inputs",
		},
		{
			name:   "build_candidate_omits_validation",
			old:    "  build-candidate:\n    needs: validate-inputs",
			unsafe: "  build-candidate:",
		},
		{
			name:   "live_preflight_omits_validation",
			old:    "  live-preflight:\n    needs: validate-inputs",
			unsafe: "  live-preflight:",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			workflow := strings.Replace(workflowWithProtectedPublisher(), test.old, test.unsafe, 1)
			if workflow == workflowWithProtectedPublisher() {
				t.Fatalf("fixture mutation did not match %q", test.old)
			}
			path := filepath.Join(t.TempDir(), "workflow.yml")
			if err := os.WriteFile(path, []byte(workflow), 0o644); err != nil {
				t.Fatal(err)
			}
			output, err := runCIArtifactUploadPolicy(t, repoRoot(t), path)
			if err == nil {
				t.Fatalf("broken protected publisher dependency chain unexpectedly accepted:\n%s", output)
			}
			if !strings.Contains(output, "job publish has forbidden write permissions") {
				t.Fatalf("unexpected failure output:\n%s", output)
			}
		})
	}
}

func TestReadinessAuditsUseStructuredCIArtifactUploadPolicy(t *testing.T) {
	root := repoRoot(t)
	for _, path := range []string{
		"scripts/production-readiness-audit.sh",
		"scripts/public-readiness-audit.sh",
	} {
		data, err := os.ReadFile(filepath.Join(root, path))
		if err != nil {
			t.Fatal(err)
		}
		script := string(data)
		for _, want := range []string{
			"scripts/ci-artifact-upload-policy.rb",
			"ci_artifact_uploads",
		} {
			if !strings.Contains(script, want) {
				t.Errorf("%s missing structured upload policy %q", path, want)
			}
		}
		if strings.Contains(script, "native-artifacts\\.yml") {
			t.Errorf("%s must not whitelist native-artifacts by filename", path)
		}
	}
}

func workflowWithUpload(triggers string) string {
	return `name: evidence
on:
` + strings.TrimPrefix(triggers, "\n") + `
permissions:
  contents: read

jobs:
  evidence:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/upload-artifact@0123456789012345678901234567890123456789
        with:
          name: private-evidence
          path: evidence.json
`
}

func workflowWithProtectedPublisher() string {
	workflow := strings.Replace(
		workflowWithUpload("\n  workflow_dispatch:\n"),
		"  evidence:",
		"  assemble-plan:",
		1,
	)
	workflow = strings.Replace(
		workflow,
		"  assemble-plan:\n    runs-on:",
		"  assemble-plan:\n    needs: [validate-inputs, build-candidate]\n    runs-on:",
		1,
	)
	return workflow + `
  validate-inputs:
    runs-on: ubuntu-latest
    permissions:
      contents: read
    steps:
      - run: echo validated
  build-candidate:
    needs: validate-inputs
    runs-on: ubuntu-latest
    permissions:
      contents: read
    steps:
      - run: echo built
  live-preflight:
    needs: validate-inputs
    runs-on: ubuntu-latest
    permissions:
      contents: read
    steps:
      - run: echo protected
  publish:
    needs: [validate-inputs, live-preflight, assemble-plan]
    if: ` + protectedPublisherCondition + `
    runs-on: ubuntu-latest
    environment: protected-release
    permissions:
      contents: write
    steps:
      - run: gh release create "$TAG" --verify-tag evidence.tar.gz
`
}

func runCIArtifactUploadPolicy(t *testing.T, root string, workflows ...string) (string, error) {
	t.Helper()
	args := append([]string{filepath.Join(root, "scripts", "ci-artifact-upload-policy.rb")}, workflows...)
	command := exec.Command("ruby", args...)
	command.Dir = root
	output, err := command.CombinedOutput()
	return string(output), err
}
