package cli

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

const windowsDefaultEnvironmentFixture = `
on:
  push:
    branches: [main, codex/**]
jobs:
  windows-default-environment:
    name: windows-default-environment
    runs-on: windows-2025
    permissions:
      contents: read
    env:
      PYTHONUTF8: "0"
    defaults:
      run:
        shell: pwsh
        working-directory: AO Command Default Windows
    steps:
      - name: Configure checkout conversion
        shell: pwsh
        working-directory: .
        run: git config --global core.autocrlf true
      - name: Checkout
        uses: actions/checkout@3d3c42e5aac5ba805825da76410c181273ba90b1
        with:
          path: AO Command Default Windows
          persist-credentials: false
      - name: Setup Go
        uses: actions/setup-go@924ae3a1cded613372ab5595356fb5720e22ba16
        with:
          go-version-file: AO Command Default Windows/go.mod
          cache: false
      - name: Setup Python
        uses: actions/setup-python@a309ff8b426b58ec0e2a45f0f869d46889d02405
        with:
          python-version: "3.12"
      - name: Verify defaults
        run: |
          if ((Get-Location).Path -notmatch ' ') { throw 'checkout path must contain spaces' }
          if ((git config --global --get core.autocrlf) -ne 'true') { throw 'core.autocrlf must be true' }
          if ($env:PYTHONUTF8 -ne '0') { throw 'PYTHONUTF8 must be 0' }
          $claude = [System.IO.File]::ReadAllBytes('CLAUDE.md')
          if ([Convert]::ToHexString($claude) -ne '404147454E54532E6D640A') { throw 'CLAUDE.md bytes differ' }
          $eol = git ls-files --eol
          if ($LASTEXITCODE -ne 0) { exit $LASTEXITCODE }
          $goEOL = @($eol | Where-Object { $_ -match '\.go$' })
          $bashEOL = @($eol | Where-Object { $_ -match '\.sh$' })
          $claudeEOL = @($eol | Where-Object { $_ -match 'CLAUDE\.md$' })
          $digestEOL = @($eol | Where-Object { $_ -match 'examples/mission/artifacts/sha256/' })
          if (@($goEOL | Where-Object { $_ -notmatch 'w/lf\s+attr/text eol=lf\s+' })) { throw 'Go files must be w/lf' }
          if (@($bashEOL | Where-Object { $_ -notmatch 'w/lf\s+attr/text eol=lf\s+' })) { throw 'Bash files must be w/lf' }
          if (@($claudeEOL | Where-Object { $_ -notmatch 'w/lf\s+attr/text eol=lf\s+' })) { throw 'CLAUDE.md must be w/lf' }
          if (@($digestEOL | Where-Object { $_ -notmatch 'attr/-text\s+' })) { throw 'digest fixtures must be attr/-text' }
      - name: Format
        run: |
          $unformatted = gofmt -l .
          if ($LASTEXITCODE -ne 0) { exit $LASTEXITCODE }
          if ($unformatted) { $unformatted; exit 1 }
      - name: Test
        run: go test ./... -count=1
      - name: Vet
        run: go vet ./...
      - name: Build
        run: go build -o (Join-Path $env:RUNNER_TEMP 'ao-command.exe') ./cmd/ao-command
      - name: Diff check
        run: git diff --check
`

func TestWindowsDefaultEnvironmentWorkflow(t *testing.T) {
	workflow, err := readWorkflowTestFile(filepath.Join("..", "..", ".github", "workflows", "ci.yml"))
	if err != nil {
		t.Fatal(err)
	}
	if err := validateWindowsDefaultEnvironmentWorkflow(workflow); err != nil {
		t.Fatal(err)
	}
}

func TestWindowsDefaultEnvironmentWorkflowRejectsDecoys(t *testing.T) {
	if err := validateWindowsDefaultEnvironmentWorkflow(windowsDefaultEnvironmentFixture); err != nil {
		t.Fatalf("valid fixture rejected: %v", err)
	}
	configStep := "      - name: Configure checkout conversion\n        shell: pwsh\n        working-directory: .\n        run: git config --global core.autocrlf true\n"
	checkoutStep := "      - name: Checkout\n        uses: actions/checkout@3d3c42e5aac5ba805825da76410c181273ba90b1\n        with:\n          path: AO Command Default Windows\n          persist-credentials: false\n"
	withoutConfig := strings.Replace(windowsDefaultEnvironmentFixture, configStep, "", 1)
	configAfterCheckout := strings.Replace(withoutConfig, checkoutStep, checkoutStep+configStep, 1)
	tests := []struct {
		name     string
		document string
	}{
		{"UTF-8 forced on", strings.Replace(windowsDefaultEnvironmentFixture, `PYTHONUTF8: "0"`, `PYTHONUTF8: "1"`, 1)},
		{"conversion configured after checkout", configAfterCheckout},
		{"checkout path has no spaces", strings.ReplaceAll(windowsDefaultEnvironmentFixture, "AO Command Default Windows", "ao-command-default-windows")},
		{"commands only in comments", strings.Replace(windowsDefaultEnvironmentFixture, "        run: go vet ./...", "        run: |\n          # go vet ./...\n          Write-Output skipped", 1)},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if test.document == windowsDefaultEnvironmentFixture {
				t.Fatal("decoy setup did not change fixture")
			}
			if test.name == "conversion configured after checkout" &&
				(strings.Count(test.document, configStep) != 1 || strings.Count(test.document, checkoutStep) != 1) {
				t.Fatal("ordering decoy must retain exactly one checkout and configuration step")
			}
			if err := validateWindowsDefaultEnvironmentWorkflow(test.document); err == nil {
				t.Fatal("invalid workflow was accepted")
			}
		})
	}
}

func validateWindowsDefaultEnvironmentWorkflow(text string) error {
	document, err := parseWorkflowTestYAML(text)
	if err != nil {
		return fmt.Errorf("parse CI workflow: %w", err)
	}
	on, ok := document["on"].(map[string]any)
	if !ok {
		return fmt.Errorf("on must be a mapping")
	}
	push, ok := on["push"].(map[string]any)
	if !ok {
		return fmt.Errorf("push must be a mapping")
	}
	branches, ok := push["branches"].([]any)
	if !ok || fmt.Sprint(branches) != "[main codex/**]" {
		return fmt.Errorf("push branches must be exactly main and codex/**")
	}
	jobs, ok := document["jobs"].(map[string]any)
	if !ok {
		return fmt.Errorf("jobs must be a mapping")
	}
	job, ok := jobs["windows-default-environment"].(map[string]any)
	if !ok {
		return fmt.Errorf("windows-default-environment job is missing")
	}
	for key, want := range map[string]string{"name": "windows-default-environment", "runs-on": "windows-2025"} {
		if job[key] != want {
			return fmt.Errorf("windows-default-environment %s = %v, want %q", key, job[key], want)
		}
	}
	permissions, _ := job["permissions"].(map[string]any)
	if len(permissions) != 1 || permissions["contents"] != "read" {
		return fmt.Errorf("Windows job permissions must be exactly contents: read")
	}
	environment, _ := job["env"].(map[string]any)
	if len(environment) != 1 || environment["PYTHONUTF8"] != "0" {
		return fmt.Errorf("Windows job env must be exactly PYTHONUTF8 string 0")
	}
	defaults, _ := job["defaults"].(map[string]any)
	runDefaults, _ := defaults["run"].(map[string]any)
	workingDirectory, _ := runDefaults["working-directory"].(string)
	if !strings.Contains(workingDirectory, " ") || runDefaults["shell"] != "pwsh" {
		return fmt.Errorf("PowerShell working directory must contain spaces")
	}
	steps, ok := job["steps"].([]any)
	if !ok {
		return fmt.Errorf("Windows job steps are missing")
	}
	checkoutIndex := -1
	configIndex := -1
	runs := make([]string, 0, len(steps))
	uses := map[string]map[string]any{}
	for index, rawStep := range steps {
		step, _ := rawStep.(map[string]any)
		if action, _ := step["uses"].(string); action != "" {
			with, _ := step["with"].(map[string]any)
			uses[action] = with
			if strings.HasPrefix(action, "actions/checkout@") {
				checkoutIndex = index
			}
		}
		if run, _ := step["run"].(string); run != "" {
			executable := executableWorkflowLines(run)
			runs = append(runs, executable)
			if strings.Contains(executable, "git config --global core.autocrlf true") {
				configIndex = index
				if step["working-directory"] != "." {
					return fmt.Errorf("core.autocrlf must be configured before checkout from the workspace root")
				}
			}
		}
	}
	checkout := uses["actions/checkout@3d3c42e5aac5ba805825da76410c181273ba90b1"]
	if checkoutIndex < 0 || configIndex < 0 || configIndex >= checkoutIndex || checkout["path"] != workingDirectory || checkout["persist-credentials"] != false {
		return fmt.Errorf("pinned credential-free checkout must follow global core.autocrlf configuration and use the spaced path")
	}
	setupGo := uses["actions/setup-go@924ae3a1cded613372ab5595356fb5720e22ba16"]
	if setupGo["go-version-file"] != workingDirectory+"/go.mod" || setupGo["cache"] != false {
		return fmt.Errorf("pinned setup-go must use the spaced go.mod path with cache disabled")
	}
	setupPython := uses["actions/setup-python@a309ff8b426b58ec0e2a45f0f869d46889d02405"]
	if setupPython["python-version"] != "3.12" {
		return fmt.Errorf("pinned setup-python must select Python 3.12")
	}
	body, _ := json.Marshal(job)
	if strings.Contains(string(body), "secrets.") || strings.Contains(string(body), "upload-artifact") || strings.Contains(string(body), "contents\":\"write") {
		return fmt.Errorf("Windows job must not use secrets, uploads, or write permissions")
	}
	commands := strings.Join(runs, "\n")
	for _, want := range []string{
		"(Get-Location).Path -notmatch ' '",
		"git config --global --get core.autocrlf",
		"$env:PYTHONUTF8 -ne '0'",
		"[System.IO.File]::ReadAllBytes('CLAUDE.md')",
		"404147454E54532E6D640A",
		"git ls-files --eol",
		"$_ -match '\\.go$'",
		"$_ -match '\\.sh$'",
		"$_ -match 'CLAUDE\\.md$'",
		"$_ -match 'examples/mission/artifacts/sha256/'",
		"$_ -notmatch 'w/lf\\s+attr/text eol=lf\\s+'",
		"$_ -notmatch 'attr/-text\\s+'",
		"$unformatted = gofmt -l .",
		"if ($unformatted)",
		"go test ./... -count=1",
		"go vet ./...",
		"go build -o (Join-Path $env:RUNNER_TEMP 'ao-command.exe') ./cmd/ao-command",
		"git diff --check",
	} {
		if !strings.Contains(commands, want) {
			return fmt.Errorf("Windows job executable commands missing %q", want)
		}
	}
	return nil
}

func executableWorkflowLines(script string) string {
	lines := strings.Split(strings.ReplaceAll(script, "\r\n", "\n"), "\n")
	executable := lines[:0]
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed != "" && !strings.HasPrefix(trimmed, "#") {
			executable = append(executable, trimmed)
		}
	}
	return strings.Join(executable, "\n")
}

func TestWindowsCheckoutEOLContract(t *testing.T) {
	root := filepath.Join("..", "..")
	attributes, err := os.ReadFile(filepath.Join(root, ".gitattributes"))
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.ReplaceAll(string(attributes), "\r\n", "\n"), "\n")
	for _, want := range []string{
		"*.go text eol=lf",
		"*.sh text eol=lf",
		"CLAUDE.md text eol=lf",
	} {
		found := false
		for _, line := range lines {
			if line == want {
				found = true
				break
			}
		}
		if !found {
			t.Errorf(".gitattributes missing exact line %q", want)
		}
	}

	alias, err := os.ReadFile(filepath.Join(root, "CLAUDE.md"))
	if err != nil {
		t.Fatal(err)
	}
	if want := []byte("@AGENTS.md\n"); !bytes.Equal(alias, want) {
		t.Errorf("CLAUDE.md bytes = %q, want %q", alias, want)
	}
}

func TestWindowsSourceShellContract(t *testing.T) {
	root := filepath.Join("..", "..")
	agents := readTestDocument(t, filepath.Join(root, "AGENTS.md"))
	reference := readTestDocument(t, filepath.Join(root, "REFERENCE.md"))

	agentSection := markdownSection(agents, "## Windows source-shell contract")
	for _, want := range []string{
		`Git\bin\bash.exe`,
		"Repository `.sh` gates run in Git for Windows Bash",
		"Ruby is not required.",
		"The AO Command binary has no Bash dependency.",
		"[REFERENCE.md](REFERENCE.md#windows-source-shell-contract)",
	} {
		if !strings.Contains(agentSection, want) {
			t.Errorf("AGENTS.md Windows source-shell section missing %q", want)
		}
	}
	if strings.Contains(agentSection, "```powershell") {
		t.Error("AGENTS.md Windows source-shell section must point to REFERENCE.md instead of duplicating the stateful example")
	}

	referenceSection := markdownSection(reference, "## Windows source-shell contract")
	for _, want := range []string{
		`Git\bin\bash.exe`,
		"Repository `.sh` gates run in Git for Windows Bash",
		"Ruby is not required.",
		"The AO Command binary has no Bash dependency.",
		"$hadAoCommandRoot = Test-Path Env:AO_COMMAND_ROOT",
		"$previousAoCommandRoot = $env:AO_COMMAND_ROOT",
		"try {",
		"finally {",
		"if ($hadAoCommandRoot)",
		"$env:AO_COMMAND_ROOT = $previousAoCommandRoot",
		"Remove-Item Env:AO_COMMAND_ROOT -ErrorAction SilentlyContinue",
		`$bashCommand = 'cd "$(cygpath -u "$AO_COMMAND_ROOT")" && scripts/ao-command-smoke.sh --forge ../ao-forge --foundry ../ao-foundry --out tmp/ao-command-smoke'`,
		"$bashArgument = $bashCommand",
		"if ($PSVersionTable.PSVersion.Major -lt 7)",
		"& $gitBash -lc $bashArgument",
		"$commandExit = $LASTEXITCODE",
		"if ($commandExit -ne 0) { exit $commandExit }",
		"scripts/ao-command-smoke.sh --forge ../ao-forge --foundry ../ao-foundry --out tmp/ao-command-smoke",
	} {
		if !strings.Contains(referenceSection, want) {
			t.Errorf("REFERENCE.md Windows source-shell section missing %q", want)
		}
	}
	if strings.Contains(referenceSection, `\"`) {
		t.Error(`REFERENCE.md Windows source-shell section must not backslash-escape double quotes inside the PowerShell single-quoted bashCommand`)
	}

	brokenReference := strings.Replace(reference, referenceSection, "## Windows source-shell contract\n", 1)
	if !strings.Contains(brokenReference, "scripts/ao-command-smoke.sh --forge ../ao-forge --foundry ../ao-foundry") {
		t.Fatal("test setup must retain the Unix smoke command outside the Windows section")
	}
	if brokenSection := markdownSection(brokenReference, "## Windows source-shell contract"); strings.Contains(brokenSection, "--forge ../ao-forge --foundry ../ao-foundry") {
		t.Error("section-scoped check accepted a smoke command that exists only outside the Windows section")
	}
}

func TestWindowsSourceShellSelection(t *testing.T) {
	tests := []struct {
		name      string
		available map[string]bool
		want      []string
		wantErr   bool
	}{
		{"both available", map[string]bool{"powershell.exe": true, "pwsh.exe": true}, []string{"powershell.exe", "pwsh.exe"}, false},
		{"optional pwsh missing", map[string]bool{"powershell.exe": true}, []string{"powershell.exe"}, false},
		{"required powershell missing", map[string]bool{"pwsh.exe": true}, nil, true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, err := selectWindowsSourceShells(func(name string) (string, error) {
				if test.available[name] {
					return name, nil
				}
				return "", exec.ErrNotFound
			})
			if (err != nil) != test.wantErr {
				t.Fatalf("selectWindowsSourceShells() error = %v, wantErr %t", err, test.wantErr)
			}
			if strings.Join(got, ",") != strings.Join(test.want, ",") {
				t.Fatalf("selectWindowsSourceShells() = %v, want %v", got, test.want)
			}
		})
	}
}

func selectWindowsSourceShells(lookup func(string) (string, error)) ([]string, error) {
	shells := make([]string, 0, 2)
	for _, shell := range []struct {
		name     string
		required bool
	}{
		{"powershell.exe", true},
		{"pwsh.exe", false},
	} {
		if _, err := lookup(shell.name); err != nil {
			if shell.required {
				return nil, fmt.Errorf("required supported shell %s is missing: %w", shell.name, err)
			}
			continue
		}
		shells = append(shells, shell.name)
	}
	return shells, nil
}

func TestWindowsSourceShellPowerShellRestoresEnvironment(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("PowerShell regression is Windows-specific")
	}
	reference := readTestDocument(t, filepath.Join("..", "..", "REFERENCE.md"))
	block := fencedBlock(markdownSection(reference, "## Windows source-shell contract"), "powershell")
	smoke := `$bashCommand = 'cd "$(cygpath -u "$AO_COMMAND_ROOT")" && scripts/ao-command-smoke.sh --forge ../ao-forge --foundry ../ao-foundry --out tmp/ao-command-smoke'`
	roundTrip := `$bashCommand = 'cd "$(cygpath -u "$AO_COMMAND_ROOT")" && test -d .'`
	shells, err := selectWindowsSourceShells(exec.LookPath)
	if err != nil {
		t.Fatal(err)
	}
	if len(shells) == 1 {
		t.Run("pwsh", func(t *testing.T) {
			t.Skip("optional pwsh.exe is not installed")
		})
	}

	for _, shell := range shells {
		t.Run(strings.TrimSuffix(shell, ".exe"), func(t *testing.T) {
			for _, test := range []struct {
				name        string
				setup       string
				command     string
				assertState string
			}{
				{"pre-existing value", `$env:AO_COMMAND_ROOT = 'preexisting-value'`, roundTrip, `if ($env:AO_COMMAND_ROOT -ne 'preexisting-value') { throw 'AO_COMMAND_ROOT was not restored' }`},
				{"initially absent", `Remove-Item Env:AO_COMMAND_ROOT -ErrorAction SilentlyContinue`, roundTrip, `if (Test-Path Env:AO_COMMAND_ROOT) { throw 'AO_COMMAND_ROOT was not removed' }`},
				{"thrown command", `$env:AO_COMMAND_ROOT = 'preexisting-value'`, `throw 'forced command failure'`, `if (-not $caught -or $env:AO_COMMAND_ROOT -ne 'preexisting-value') { throw 'command failure did not restore AO_COMMAND_ROOT' }`},
			} {
				t.Run(test.name, func(t *testing.T) {
					if block == "" || !strings.Contains(block, smoke) {
						t.Fatal("REFERENCE.md Windows PowerShell block or smoke invocation not found")
					}
					body := strings.Replace(block, smoke, test.command, 1)
					script := test.setup + "\n"
					if test.name == "thrown command" {
						script += "$caught = $false\ntry {\n" + body + "\n} catch { $caught = $true }\n"
					} else {
						script += body + "\n"
					}
					script += test.assertState
					scriptPath := filepath.Join(t.TempDir(), "source-shell-regression.ps1")
					if err := os.WriteFile(scriptPath, []byte(script), 0o600); err != nil {
						t.Fatal(err)
					}
					command := exec.Command(shell, "-NoProfile", "-NonInteractive", "-ExecutionPolicy", "Bypass", "-File", scriptPath)
					if output, err := command.CombinedOutput(); err != nil {
						t.Fatalf("documented PowerShell state wrapper failed: %v\n%s", err, output)
					}
				})
			}

			t.Run("nonzero command exit", func(t *testing.T) {
				body := strings.Replace(block, smoke, `$bashCommand = 'exit 23'`, 1)
				scriptPath := filepath.Join(t.TempDir(), "source-shell-exit-regression.ps1")
				if err := os.WriteFile(scriptPath, []byte("Remove-Item Env:AO_COMMAND_ROOT -ErrorAction SilentlyContinue\n"+body), 0o600); err != nil {
					t.Fatal(err)
				}
				command := exec.Command(shell, "-NoProfile", "-NonInteractive", "-ExecutionPolicy", "Bypass", "-File", scriptPath)
				err := command.Run()
				exitError, ok := err.(*exec.ExitError)
				if !ok || exitError.ExitCode() != 23 {
					t.Fatalf("documented PowerShell wrapper exit = %v, want 23", err)
				}
			})
		})
	}
}

func readTestDocument(t *testing.T, path string) string {
	t.Helper()
	document, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return strings.ReplaceAll(string(document), "\r\n", "\n")
}

func markdownSection(document, heading string) string {
	start := strings.Index(document, heading)
	if start < 0 {
		return ""
	}
	section := document[start:]
	if end := strings.Index(section[len(heading):], "\n## "); end >= 0 {
		section = section[:len(heading)+end]
	}
	return section
}

func fencedBlock(section, language string) string {
	open := fmt.Sprintf("```%s\n", language)
	start := strings.Index(section, open)
	if start < 0 {
		return ""
	}
	block := section[start+len(open):]
	if end := strings.Index(block, "\n```"); end >= 0 {
		return block[:end]
	}
	return ""
}
