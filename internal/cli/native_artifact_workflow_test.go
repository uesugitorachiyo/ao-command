package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestNativeArtifactWorkflowContract(t *testing.T) {
	workflow, err := readWorkflowTestFile(filepath.Join("..", "..", ".github", "workflows", "native-artifacts.yml"))
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"ubuntu-latest",
		"macos-latest",
		"windows-latest",
		"linux-x86_64",
		"macos-aarch64",
		"windows-x86_64",
		"actions/upload-artifact",
		"ao-command-native-artifact-${{ matrix.target_label }}-${{ github.sha }}",
		"native-artifact-summary.json",
		"SHA256SUMS",
		"LICENSE",
		"NOTICE",
		"./cmd/ao-command",
		"--help",
		"contents: read",
		"4c501b4f1e55cb9b926709e19d496edf41984fb1",
	} {
		if !strings.Contains(workflow, want) {
			t.Fatalf("native artifact workflow missing %q", want)
		}
	}
	for _, forbidden := range []string{"contents: write", "gh release", "actions/create-release", "softprops/action-gh-release"} {
		if strings.Contains(workflow, forbidden) {
			t.Fatalf("native artifact workflow must not include %q", forbidden)
		}
	}
	triggerStart := strings.Index(workflow, "on:\n")
	permissionsStart := strings.Index(workflow, "\npermissions:")
	if triggerStart < 0 || permissionsStart < 0 || triggerStart >= permissionsStart {
		t.Fatal("native artifact workflow must declare a bounded trigger block")
	}
	triggerBlock := strings.TrimSpace(workflow[triggerStart+len("on:\n") : permissionsStart])
	if triggerBlock != "workflow_dispatch:" {
		t.Fatalf("native artifact workflow must be workflow_dispatch-only, got %q", triggerBlock)
	}

	nativeBuild := strings.Index(workflow, "go build -trimpath")
	policyCheckout := strings.Index(workflow, "repository: uesugitorachiyo/ao-architecture")
	metadataReader := strings.Index(workflow, "scripts/read_go_binary_metadata.go")
	builder := strings.Index(workflow, "scripts/build_go_supply_chain_candidate.py")
	verifier := strings.Index(workflow, "scripts/verify_supply_chain_policy.py")
	if nativeBuild < 0 || policyCheckout < 0 || metadataReader < 0 || builder < 0 || verifier < 0 ||
		!(nativeBuild < policyCheckout && policyCheckout < metadataReader && metadataReader < builder && builder < verifier) {
		t.Fatal("native build, policy checkout, metadata reader, builder, and verifier are required in order")
	}
	hasExactLine := func(section, want string) bool {
		for _, line := range strings.Split(section, "\n") {
			if strings.TrimSpace(line) == want {
				return true
			}
		}
		return false
	}
	if !hasExactLine(workflow[builder:verifier], `--workspace-root . \`) ||
		!hasExactLine(workflow[verifier:], `--workspace-root "$supply_chain_dir" \`) {
		t.Fatal("builder must use the repository root and verifier must use the bundle root")
	}
}

func TestProductionReadinessAuditClassifiesNativeArtifactWorkflowUploads(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("..", "..", "scripts", "production-readiness-audit.sh"))
	if err != nil {
		t.Fatal(err)
	}
	script := string(data)
	for _, want := range []string{
		"go run ./cmd/ci-artifact-upload-policy",
		"ci_artifact_uploads",
	} {
		if !strings.Contains(script, want) {
			t.Fatalf("production readiness audit must structurally classify artifact uploads, missing %q", want)
		}
	}
	if strings.Contains(script, "native-artifacts\\.yml") {
		t.Fatal("production readiness audit must not exempt native artifacts by filename")
	}
}
