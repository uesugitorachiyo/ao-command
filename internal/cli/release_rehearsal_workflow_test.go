package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestReleaseRehearsalWorkflowContract(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("..", "..", ".github", "workflows", "release-rehearsal.yml"))
	if err != nil {
		t.Fatal(err)
	}
	workflow := string(data)

	for _, want := range []string{
		"name: release-rehearsal",
		"workflow_dispatch:",
		"description: Required only for a future live release with every bound value.",
		"version:",
		"tag:",
		"source_commit:",
		"approved_manifest_digest:",
		"dry_run:",
		"exact_confirmation:",
		"contents: read",
		"ubuntu-latest",
		"macos-latest",
		"windows-latest",
		"linux-x86_64",
		"macos-aarch64",
		"windows-x86_64",
		"actions/upload-artifact@v7",
		"actions/download-artifact@v7",
		"overwrite: false",
		"approved_manifest_digest",
		"release-rehearsal-plan.json",
		"release-rehearsal-plan.sha256",
		"dry-run-boundary.json",
		"tag_creation_attempted",
		"release_creation_attempted",
		"public_upload_attempted",
		"publication_performed",
		"unexpected artifact inventory",
		"candidate archive checksum mismatch",
		"environment: protected-release",
		"contents: write",
		"inputs.dry_run == false",
		"inputs.exact_confirmation == format('publish-ao-command-{0}-{1}-{2}-{3}', inputs.version, inputs.tag, inputs.source_commit, inputs.approved_manifest_digest)",
		"gh release create",
		"gh release download",
	} {
		if !strings.Contains(workflow, want) {
			t.Fatalf("release rehearsal workflow missing %q", want)
		}
	}

	if strings.Contains(workflow, "push:") || strings.Contains(workflow, "pull_request:") {
		t.Fatal("release rehearsal workflow must be dispatch-only")
	}
	if strings.Contains(workflow, "secrets.") || strings.Contains(workflow, "GH_PAT") || strings.Contains(workflow, "PERSONAL_ACCESS_TOKEN") {
		t.Fatal("release rehearsal workflow must use only the workflow-scoped token")
	}
	if strings.Count(workflow, "contents: write") != 1 {
		t.Fatal("release rehearsal workflow must grant contents: write only to the publisher job")
	}
}
