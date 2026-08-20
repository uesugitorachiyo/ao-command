package cli

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/uesugitorachiyo/ao-command/internal/workflowpolicy"
)

func TestCIArtifactUploadPolicyAcceptsRepositoryWorkflowsDirectly(t *testing.T) {
	root := repoRoot(t)
	err := workflowpolicy.ValidateFiles([]string{
		filepath.Join(root, ".github", "workflows", "release-rehearsal.yml"),
		filepath.Join(root, ".github", "workflows", "native-artifacts.yml"),
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestCIArtifactUploadPolicyCommandExitCodes(t *testing.T) {
	root := repoRoot(t)
	binary := filepath.Join(t.TempDir(), "ci-artifact-upload-policy")
	if runtime.GOOS == "windows" {
		binary += ".exe"
	}
	build := exec.Command("go", "build", "-o", binary, "./cmd/ci-artifact-upload-policy")
	build.Dir = root
	if output, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build command: %v\n%s", err, output)
	}
	valid := filepath.Join(root, ".github", "workflows", "native-artifacts.yml")
	invalid := filepath.Join(t.TempDir(), "invalid.yml")
	if err := os.WriteFile(invalid, []byte("jobs: &jobs {}\ncopy: *jobs\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		name string
		args []string
		code int
		want string
	}{
		{name: "usage", code: 2, want: "usage: ci-artifact-upload-policy WORKFLOW..."},
		{name: "success", args: []string{valid}, code: 0},
		{name: "violation", args: []string{invalid}, code: 1, want: "YAML aliases are forbidden"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			command := exec.Command(binary, test.args...)
			command.Dir = root
			output, err := command.CombinedOutput()
			code := 0
			if err != nil {
				exitErr, ok := err.(*exec.ExitError)
				if !ok {
					t.Fatalf("command failed without exit status: %v\n%s", err, output)
				}
				code = exitErr.ExitCode()
			}
			if code != test.code || !strings.Contains(string(output), test.want) {
				t.Fatalf("exit/output = %d, %q; want exit %d containing %q", code, output, test.code, test.want)
			}
		})
	}
}

func TestReadinessAuditsUseGoCIArtifactUploadPolicy(t *testing.T) {
	root := repoRoot(t)
	for _, path := range []string{
		"scripts/production-readiness-audit.sh",
		"scripts/public-readiness-audit.sh",
	} {
		data, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(path)))
		if err != nil {
			t.Fatal(err)
		}
		script := string(data)
		for _, want := range []string{
			`go run ./cmd/ci-artifact-upload-policy "${workflow_files[@]}"`,
			"internal/workflowpolicy/policy\\.go",
			"ci_artifact_uploads",
		} {
			if !strings.Contains(script, want) {
				t.Errorf("%s missing Go upload policy %q", path, want)
			}
		}
		if strings.Contains(script, "scripts/ci-artifact-upload-policy.rb") || strings.Contains(script, "native-artifacts\\.yml") {
			t.Errorf("%s retains a Ruby policy reference or filename whitelist", path)
		}
	}
}
