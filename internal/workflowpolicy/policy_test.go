package workflowpolicy

import (
	"path/filepath"
	"strings"
	"testing"
)

const protectedPublisherCondition = "${{ inputs.dry_run == false && needs.validate-inputs.result == 'success' && needs.live-preflight.result == 'success' && needs.assemble-plan.result == 'success' && inputs.exact_confirmation == format('publish-ao-command-{0}-{1}-{2}-{3}-{4}-{5}', inputs.version, inputs.tag, inputs.source_commit, inputs.approved_manifest_digest, needs.validate-inputs.outputs.release_notes_digest, inputs.expected_plan_digest) }}"

func TestValidateFilesAcceptsRepositoryWorkflows(t *testing.T) {
	for _, path := range []string{
		filepath.Join("..", "..", ".github", "workflows", "release-rehearsal.yml"),
		filepath.Join("..", "..", ".github", "workflows", "native-artifacts.yml"),
	} {
		if err := ValidateFiles([]string{path}); err != nil {
			t.Fatalf("strict manual evidence workflow %s rejected: %v", path, err)
		}
	}
}

func TestValidateBytesAcceptsLFCRLFAndOnKey(t *testing.T) {
	workflow := workflowWithUpload("\n  workflow_dispatch:\n")
	for _, body := range []string{workflow, strings.ReplaceAll(workflow, "\n", "\r\n")} {
		if err := ValidateBytes("workflow.yml", []byte(body)); err != nil {
			t.Fatalf("valid workflow rejected: %v", err)
		}
	}
}

func TestValidateBytesRejectsDefaultMixedAndWritableWorkflows(t *testing.T) {
	tests := []struct {
		name     string
		workflow string
		want     string
	}{
		{name: "pull_request", workflow: workflowWithUpload("\n  pull_request:\n"), want: "workflow_dispatch-only"},
		{name: "push", workflow: workflowWithUpload("\n  push:\n    branches: [main]\n"), want: "workflow_dispatch-only"},
		{name: "schedule", workflow: workflowWithUpload("\n  schedule:\n    - cron: \"0 0 * * *\"\n"), want: "workflow_dispatch-only"},
		{name: "mixed_manual_and_default", workflow: workflowWithUpload("\n  workflow_dispatch:\n  pull_request:\n"), want: "workflow_dispatch-only"},
		{
			name:     "global_write",
			workflow: strings.Replace(workflowWithUpload("\n  workflow_dispatch:\n"), "contents: read", "contents: write", 1),
			want:     "workflow permissions must be explicit read-only",
		},
		{
			name:     "job_write",
			workflow: strings.Replace(workflowWithUpload("\n  workflow_dispatch:\n"), "    steps:", "    permissions:\n      contents: write\n    steps:", 1),
			want:     "job evidence artifact uploads require contents: read and no writes",
		},
		{
			name:     "missing_permissions",
			workflow: strings.Replace(workflowWithUpload("\n  workflow_dispatch:\n"), "permissions:\n  contents: read\n\n", "", 1),
			want:     "workflow permissions must be explicit read-only",
		},
		{
			name:     "public_release_upload",
			workflow: strings.Replace(workflowWithUpload("\n  workflow_dispatch:\n"), "uses: actions/upload-artifact@0123456789012345678901234567890123456789", "run: gh release upload v1 evidence.json", 1),
			want:     "public release command requires protected publisher",
		},
		{
			name:     "continued_public_release_upload",
			workflow: strings.Replace(workflowWithUpload("\n  workflow_dispatch:\n"), "uses: actions/upload-artifact@0123456789012345678901234567890123456789", "run: gh \\\n        release upload v1 evidence.json", 1),
			want:     "public release command requires protected publisher",
		},
		{
			name:     "separate_writer_bypass",
			workflow: workflowWithUpload("\n  workflow_dispatch:\n") + "\n  writer:\n    runs-on: ubuntu-latest\n    permissions:\n      contents: write\n    steps:\n      - run: echo separate writer\n",
			want:     "job writer has forbidden write permissions",
		},
		{
			name:     "release_create_assets_bypass",
			workflow: workflowWithUpload("\n  workflow_dispatch:\n") + "\n  release:\n    runs-on: ubuntu-latest\n    permissions:\n      contents: read\n    steps:\n      - run: gh release create v1 evidence.tar.gz\n",
			want:     "public release command requires protected publisher",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if test.name == "continued_public_release_upload" {
				test.workflow = strings.Replace(workflowWithUpload("\n  workflow_dispatch:\n"), "uses: actions/upload-artifact@0123456789012345678901234567890123456789", "run: |\n          gh \\\n            release upload v1 evidence.json", 1)
			}
			assertViolation(t, ValidateBytes("workflow.yml", []byte(test.workflow)), "workflow.yml", test.want)
		})
	}
}

func TestValidateBytesDoesNotSkipJobsOrStepsWithNonStringKeys(t *testing.T) {
	tests := []struct {
		name string
		body string
		want string
	}{
		{
			name: "write permissions",
			body: "name: unsafe\non: workflow_dispatch\npermissions:\n  contents: read\njobs:\n  unsafe:\n    1: extra\n    permissions: write-all\n",
			want: "job unsafe has forbidden write permissions",
		},
		{
			name: "artifact upload",
			body: "name: unsafe\non: workflow_dispatch\npermissions:\n  contents: read\njobs:\n  unsafe:\n    1: extra\n    permissions: write-all\n    steps:\n      - 2: extra\n        uses: actions/upload-artifact@v4\n",
			want: "job unsafe artifact uploads require contents: read and no writes",
		},
		{
			name: "public release command",
			body: "name: unsafe\non: workflow_dispatch\npermissions:\n  contents: read\njobs:\n  unsafe:\n    1: extra\n    steps:\n      - 2: extra\n        run: gh release create v1\n",
			want: "job unsafe public release command requires protected publisher",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			assertViolation(t, ValidateBytes("workflow.yml", []byte(test.body)), "workflow.yml", test.want)
		})
	}
}

func TestValidateBytesAcceptsProtectedPublisher(t *testing.T) {
	if err := ValidateBytes("workflow.yml", []byte(workflowWithProtectedPublisher())); err != nil {
		t.Fatalf("isolated evidence and protected publisher rejected: %v", err)
	}
}

func TestValidateBytesRejectsIncompleteOrTautologicalPublisherGuards(t *testing.T) {
	tests := []struct {
		name string
		old  string
		bad  string
	}{
		{name: "omitted_immutable_plan_prerequisite", old: "needs: [validate-inputs, live-preflight, assemble-plan]", bad: "needs: [validate-inputs, live-preflight]"},
		{name: "dry_run_tautology", old: "inputs.dry_run == false", bad: "(inputs.dry_run == false || true)"},
		{name: "prerequisite_tautology", old: "needs.assemble-plan.result == 'success'", bad: "(needs.assemble-plan.result == 'success' || true)"},
		{name: "confirmation_tautology", old: protectedPublisherCondition, bad: strings.TrimSuffix(protectedPublisherCondition, " }}") + " || true }}"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			assertViolation(t, ValidateBytes("workflow.yml", []byte(strings.Replace(workflowWithProtectedPublisher(), test.old, test.bad, 1))), "workflow.yml", "job publish has forbidden write permissions")
		})
	}
}

func TestValidateBytesRejectsBrokenProtectedPublisherDependencyChain(t *testing.T) {
	tests := []struct{ name, old, bad string }{
		{name: "assemble_plan_omits_build_candidate", old: "needs: [validate-inputs, build-candidate]", bad: "needs: validate-inputs"},
		{name: "build_candidate_omits_validation", old: "  build-candidate:\n    needs: validate-inputs", bad: "  build-candidate:"},
		{name: "live_preflight_omits_validation", old: "  live-preflight:\n    needs: validate-inputs", bad: "  live-preflight:"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			assertViolation(t, ValidateBytes("workflow.yml", []byte(strings.Replace(workflowWithProtectedPublisher(), test.old, test.bad, 1))), "workflow.yml", "job publish has forbidden write permissions")
		})
	}
}

func workflowWithUpload(triggers string) string {
	return "name: evidence\non:\n" + strings.TrimPrefix(triggers, "\n") + `
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
	workflow := strings.Replace(workflowWithUpload("\n  workflow_dispatch:\n"), "  evidence:", "  assemble-plan:", 1)
	workflow = strings.Replace(workflow, "  assemble-plan:\n    runs-on:", "  assemble-plan:\n    needs: [validate-inputs, build-candidate]\n    runs-on:", 1)
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
