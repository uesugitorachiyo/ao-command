package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestNativeArtifactWorkflowContract(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("..", "..", ".github", "workflows", "native-artifacts.yml"))
	if err != nil {
		t.Fatal(err)
	}
	workflow := string(data)
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
	for _, forbiddenTrigger := range []string{"pull_request:", "push:", "schedule:"} {
		if strings.Contains(workflow, forbiddenTrigger) {
			t.Fatalf("native artifact uploads must not run by default, found trigger %q", forbiddenTrigger)
		}
	}
	if !strings.Contains(workflow, "workflow_dispatch:") {
		t.Fatal("native artifact workflow must be explicitly manual")
	}

	nativeBuild := strings.Index(workflow, "go build -trimpath")
	policyCheckout := strings.Index(workflow, "repository: uesugitorachiyo/ao-architecture")
	if nativeBuild < 0 || policyCheckout < 0 || nativeBuild >= policyCheckout {
		t.Fatal("native artifact must be built before the policy checkout modifies the source tree")
	}
	builder := strings.Index(workflow, "scripts/build_go_supply_chain_candidate.py")
	verifier := strings.Index(workflow, "scripts/verify_supply_chain_policy.py")
	if builder < 0 || verifier < 0 || builder >= verifier {
		t.Fatal("supply-chain builder and verifier steps are required in order")
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
		"scripts/ci-artifact-upload-policy.rb",
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
