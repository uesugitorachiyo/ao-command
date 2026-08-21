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
  workflow_dispatch:
  pull_request:
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
        run: |
          git config --global core.autocrlf true
          if ($LASTEXITCODE -ne 0) { exit $LASTEXITCODE }
      - name: Checkout ao-command
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
      - name: Verify default Windows checkout
        run: |
          if ((Get-Location).Path -notmatch ' ') { throw 'checkout path must contain spaces' }
          $autocrlf = git config --global --get core.autocrlf
          if ($LASTEXITCODE -ne 0) { exit $LASTEXITCODE }
          if ($autocrlf -ne 'true') { throw 'core.autocrlf must be true' }
          if ($env:PYTHONUTF8 -ne '0') { throw 'PYTHONUTF8 must be 0' }
          $claude = [System.IO.File]::ReadAllBytes('CLAUDE.md')
          if ([Convert]::ToHexString($claude) -ne '404147454E54532E6D640A') { throw 'CLAUDE.md must contain exact ASCII @AGENTS.md plus LF' }
          $eol = @(git ls-files --eol -- '*.go' '*.sh' 'CLAUDE.md' 'examples/mission/artifacts/sha256/*')
          if ($LASTEXITCODE -ne 0) { exit $LASTEXITCODE }
          $goEOL = @($eol | Where-Object { $_ -match '\.go$' })
          $bashEOL = @($eol | Where-Object { $_ -match '\.sh$' })
          $claudeEOL = @($eol | Where-Object { $_ -match 'CLAUDE\.md$' })
          $digestEOL = @($eol | Where-Object { $_ -match 'examples/mission/artifacts/sha256/' })
          if ($goEOL.Count -eq 0 -or @($goEOL | Where-Object { $_ -notmatch 'w/lf\s+attr/text eol=lf\s+' }).Count -ne 0) { throw 'Go files must be w/lf with attr/text eol=lf' }
          if ($bashEOL.Count -eq 0 -or @($bashEOL | Where-Object { $_ -notmatch 'w/lf\s+attr/text eol=lf\s+' }).Count -ne 0) { throw 'Bash files must be w/lf with attr/text eol=lf' }
          if ($claudeEOL.Count -ne 1 -or $claudeEOL[0] -notmatch 'w/lf\s+attr/text eol=lf\s+') { throw 'CLAUDE.md must be w/lf with attr/text eol=lf' }
          if ($digestEOL.Count -eq 0 -or @($digestEOL | Where-Object { $_ -notmatch 'attr/-text\s+' }).Count -ne 0) { throw 'digest fixtures must be attr/-text' }
      - name: Check formatting
        run: |
          $unformatted = gofmt -l .
          if ($LASTEXITCODE -ne 0) { exit $LASTEXITCODE }
          if ($unformatted) {
            $unformatted
            exit 1
          }
      - name: Test
        run: |
          go test ./... -count=1
          if ($LASTEXITCODE -ne 0) { exit $LASTEXITCODE }
      - name: Vet
        run: |
          go vet ./...
          if ($LASTEXITCODE -ne 0) { exit $LASTEXITCODE }
      - name: Build
        run: |
          go build -o (Join-Path $env:RUNNER_TEMP 'ao-command.exe') ./cmd/ao-command
          if ($LASTEXITCODE -ne 0) { exit $LASTEXITCODE }
      - name: Check diff
        run: |
          git diff --check
          if ($LASTEXITCODE -ne 0) { exit $LASTEXITCODE }
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
	configStep := "      - name: Configure checkout conversion\n        shell: pwsh\n        working-directory: .\n        run: |\n          git config --global core.autocrlf true\n          if ($LASTEXITCODE -ne 0) { exit $LASTEXITCODE }\n"
	checkoutStep := "      - name: Checkout ao-command\n        uses: actions/checkout@3d3c42e5aac5ba805825da76410c181273ba90b1\n        with:\n          path: AO Command Default Windows\n          persist-credentials: false\n"
	setupGoStep := "      - name: Setup Go\n        uses: actions/setup-go@924ae3a1cded613372ab5595356fb5720e22ba16\n        with:\n          go-version-file: AO Command Default Windows/go.mod\n          cache: false\n"
	testStep := "      - name: Test\n        run: |\n          go test ./... -count=1\n          if ($LASTEXITCODE -ne 0) { exit $LASTEXITCODE }\n"
	vetStep := "      - name: Vet\n        run: |\n          go vet ./...\n          if ($LASTEXITCODE -ne 0) { exit $LASTEXITCODE }\n"
	buildStep := "      - name: Build\n        run: |\n          go build -o (Join-Path $env:RUNNER_TEMP 'ao-command.exe') ./cmd/ao-command\n          if ($LASTEXITCODE -ne 0) { exit $LASTEXITCODE }\n"
	diffStep := "      - name: Check diff\n        run: |\n          git diff --check\n          if ($LASTEXITCODE -ne 0) { exit $LASTEXITCODE }\n"
	withoutConfig := strings.Replace(windowsDefaultEnvironmentFixture, configStep, "", 1)
	configAfterCheckout := strings.Replace(withoutConfig, checkoutStep, checkoutStep+configStep, 1)
	withoutSetupGo := strings.Replace(windowsDefaultEnvironmentFixture, setupGoStep, "", 1)
	setupBeforeCheckout := strings.Replace(withoutSetupGo, checkoutStep, setupGoStep+checkoutStep, 1)
	gatesOutOfOrder := strings.Replace(windowsDefaultEnvironmentFixture, testStep+vetStep+buildStep+diffStep, diffStep+buildStep+vetStep+testStep, 1)
	collapsedGates := strings.Replace(windowsDefaultEnvironmentFixture, testStep+vetStep, "      - name: Arbitrary gates\n        run: |\n          go test ./... -count=1\n          if ($LASTEXITCODE -ne 0) { exit $LASTEXITCODE }\n          go vet ./...\n          if ($LASTEXITCODE -ne 0) { exit $LASTEXITCODE }\n", 1)
	diffInBuild := strings.Replace(windowsDefaultEnvironmentFixture, buildStep+diffStep, "      - name: Build\n        run: |\n          go build -o (Join-Path $env:RUNNER_TEMP 'ao-command.exe') ./cmd/ao-command\n          if ($LASTEXITCODE -ne 0) { exit $LASTEXITCODE }\n          git diff --check\n          if ($LASTEXITCODE -ne 0) { exit $LASTEXITCODE }\n      - name: Check diff\n        run: Write-Output skipped\n", 1)
	duplicateTestCommand := strings.Replace(windowsDefaultEnvironmentFixture, testStep, testStep+"      - name: Extra test\n        run: |\n          go test ./... -count=1\n          if ($LASTEXITCODE -ne 0) { exit $LASTEXITCODE }\n", 1)
	tests := []struct {
		name     string
		document string
	}{
		{"UTF-8 forced on", strings.Replace(windowsDefaultEnvironmentFixture, `PYTHONUTF8: "0"`, `PYTHONUTF8: "1"`, 1)},
		{"conversion configured after checkout", configAfterCheckout},
		{"checkout path has no spaces", strings.ReplaceAll(windowsDefaultEnvironmentFixture, "AO Command Default Windows", "ao-command-default-windows")},
		{"commands only in comments", strings.Replace(windowsDefaultEnvironmentFixture, "          go vet ./...", "          # go vet ./...\n          Write-Output skipped", 1)},
		{"quoted config command", strings.Replace(windowsDefaultEnvironmentFixture, "          git config --global core.autocrlf true", "          Write-Output 'git config --global core.autocrlf true'", 1)},
		{"quoted vet command", strings.Replace(windowsDefaultEnvironmentFixture, "          go vet ./...", "          Write-Output 'go vet ./...'", 1)},
		{"quoted test command", strings.Replace(windowsDefaultEnvironmentFixture, "          go test ./... -count=1", "          Write-Output 'go test ./... -count=1'", 1)},
		{"quoted build command", strings.Replace(windowsDefaultEnvironmentFixture, "          go build -o (Join-Path $env:RUNNER_TEMP 'ao-command.exe') ./cmd/ao-command", "          Write-Output \"go build -o (Join-Path $env:RUNNER_TEMP 'ao-command.exe') ./cmd/ao-command\"", 1)},
		{"quoted diff command", strings.Replace(windowsDefaultEnvironmentFixture, "          git diff --check", "          Write-Output 'git diff --check'", 1)},
		{"duplicate checkout", strings.Replace(windowsDefaultEnvironmentFixture, checkoutStep, checkoutStep+checkoutStep, 1)},
		{"duplicate config", strings.Replace(windowsDefaultEnvironmentFixture, configStep, configStep+configStep, 1)},
		{"test exit guard removed", strings.Replace(windowsDefaultEnvironmentFixture, "          go test ./... -count=1\n          if ($LASTEXITCODE -ne 0) { exit $LASTEXITCODE }", "          go test ./... -count=1", 1)},
		{"vet exit guard misplaced", strings.Replace(windowsDefaultEnvironmentFixture, "          go vet ./...\n          if ($LASTEXITCODE -ne 0) { exit $LASTEXITCODE }", "          go vet ./...\n          Write-Output delayed\n          if ($LASTEXITCODE -ne 0) { exit $LASTEXITCODE }", 1)},
		{"workflow dispatch omitted", strings.Replace(windowsDefaultEnvironmentFixture, "  workflow_dispatch:\n", "", 1)},
		{"pull request omitted", strings.Replace(windowsDefaultEnvironmentFixture, "  pull_request:\n", "", 1)},
		{"extra trigger", strings.Replace(windowsDefaultEnvironmentFixture, "  push:\n", "  schedule:\n    - cron: '0 0 * * *'\n  push:\n", 1)},
		{"push paths", strings.Replace(windowsDefaultEnvironmentFixture, "    branches: [main, codex/**]\n", "    branches: [main, codex/**]\n    paths: [internal/**]\n", 1)},
		{"push paths ignore", strings.Replace(windowsDefaultEnvironmentFixture, "    branches: [main, codex/**]\n", "    branches: [main, codex/**]\n    paths-ignore: [docs/**]\n", 1)},
		{"pull request paths", strings.Replace(windowsDefaultEnvironmentFixture, "  pull_request:\n", "  pull_request:\n    paths: [internal/**]\n", 1)},
		{"pull request branches", strings.Replace(windowsDefaultEnvironmentFixture, "  pull_request:\n", "  pull_request:\n    branches: [main]\n", 1)},
		{"nonempty workflow dispatch", strings.Replace(windowsDefaultEnvironmentFixture, "  workflow_dispatch:\n", "  workflow_dispatch:\n    inputs: {}\n", 1)},
		{"setup before checkout", setupBeforeCheckout},
		{"gates out of order", gatesOutOfOrder},
		{"commands collapsed into arbitrary step", collapsedGates},
		{"required step renamed", strings.Replace(windowsDefaultEnvironmentFixture, "      - name: Setup Python\n", "      - name: Python toolchain\n", 1)},
		{"correct diff command in wrong step", diffInBuild},
		{"duplicate test command in extra step", duplicateTestCommand},
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

type windowsWorkflowStep struct {
	index  int
	name   string
	action string
	with   map[string]any
	lines  []string
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
	if len(on) != 3 {
		return fmt.Errorf("triggers must be exactly workflow_dispatch, pull_request, and push")
	}
	for _, trigger := range []string{"workflow_dispatch", "pull_request", "push"} {
		if _, ok := on[trigger]; !ok {
			return fmt.Errorf("required trigger %q is missing", trigger)
		}
	}
	for _, trigger := range []string{"workflow_dispatch", "pull_request"} {
		value := on[trigger]
		mapping, isMapping := value.(map[string]any)
		if value != nil && (!isMapping || len(mapping) != 0) {
			return fmt.Errorf("trigger %q must be null or an empty mapping", trigger)
		}
	}
	push, ok := on["push"].(map[string]any)
	if !ok || len(push) != 1 {
		return fmt.Errorf("push must contain only branches")
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
	parsedSteps := make([]windowsWorkflowStep, 0, len(steps))
	for index, rawStep := range steps {
		step, ok := rawStep.(map[string]any)
		if !ok {
			return fmt.Errorf("Windows job step %d must be a mapping", index)
		}
		action, _ := step["uses"].(string)
		with, _ := step["with"].(map[string]any)
		run, _ := step["run"].(string)
		parsedSteps = append(parsedSteps, windowsWorkflowStep{
			index:  index,
			name:   fmt.Sprint(step["name"]),
			action: action,
			with:   with,
			lines:  executableWorkflowLines(run),
		})
	}
	requiredNames := []string{
		"Configure checkout conversion",
		"Checkout ao-command",
		"Setup Go",
		"Setup Python",
		"Verify default Windows checkout",
		"Check formatting",
		"Test",
		"Vet",
		"Build",
		"Check diff",
	}
	actionSteps := map[string]bool{
		"Checkout ao-command": true,
		"Setup Go":            true,
		"Setup Python":        true,
	}
	requiredSteps := make(map[string]windowsWorkflowStep, len(requiredNames))
	previousIndex := -1
	for _, name := range requiredNames {
		matches := make([]windowsWorkflowStep, 0, 1)
		for _, step := range parsedSteps {
			if step.name == name {
				matches = append(matches, step)
			}
		}
		if len(matches) != 1 || matches[0].index <= previousIndex {
			return fmt.Errorf("required Windows step %q must appear exactly once in order", name)
		}
		step := matches[0]
		if actionSteps[name] {
			if step.action == "" || len(step.lines) != 0 {
				return fmt.Errorf("required Windows step %q must be an action step", name)
			}
		} else if step.action != "" || len(step.lines) == 0 {
			return fmt.Errorf("required Windows step %q must be a run step", name)
		}
		requiredSteps[name] = step
		previousIndex = step.index
	}
	findAction := func(prefix, exact string) (windowsWorkflowStep, error) {
		matches := make([]windowsWorkflowStep, 0, 1)
		for _, step := range parsedSteps {
			if strings.HasPrefix(step.action, prefix+"@") {
				matches = append(matches, step)
			}
		}
		if len(matches) != 1 || matches[0].action != exact {
			return windowsWorkflowStep{}, fmt.Errorf("Windows job must contain one exact %s action", exact)
		}
		return matches[0], nil
	}
	checkout, err := findAction("actions/checkout", "actions/checkout@3d3c42e5aac5ba805825da76410c181273ba90b1")
	if err != nil {
		return err
	}
	setupGo, err := findAction("actions/setup-go", "actions/setup-go@924ae3a1cded613372ab5595356fb5720e22ba16")
	if err != nil {
		return err
	}
	setupPython, err := findAction("actions/setup-python", "actions/setup-python@a309ff8b426b58ec0e2a45f0f869d46889d02405")
	if err != nil {
		return err
	}
	if checkout.index != requiredSteps["Checkout ao-command"].index ||
		setupGo.index != requiredSteps["Setup Go"].index ||
		setupPython.index != requiredSteps["Setup Python"].index {
		return fmt.Errorf("pinned setup actions must be bound to their required named steps")
	}
	configCommand := "git config --global core.autocrlf true"
	configStep := requiredSteps["Configure checkout conversion"]
	if workflowLineCount(parsedSteps, configCommand) != 1 || !equalWorkflowLines(configStep.lines, []string{configCommand, workflowExitGuard}) {
		return fmt.Errorf("one exact core.autocrlf command must run before checkout with immediate exit handling")
	}
	rawConfigStep, _ := steps[configStep.index].(map[string]any)
	if rawConfigStep["working-directory"] != "." {
		return fmt.Errorf("core.autocrlf must be configured from the workspace root")
	}
	if checkout.with["path"] != workingDirectory || checkout.with["persist-credentials"] != false {
		return fmt.Errorf("pinned credential-free checkout must follow global core.autocrlf configuration and use the spaced path")
	}
	if setupGo.with["go-version-file"] != workingDirectory+"/go.mod" || setupGo.with["cache"] != false {
		return fmt.Errorf("pinned setup-go must use the spaced go.mod path with cache disabled")
	}
	if setupPython.with["python-version"] != "3.12" {
		return fmt.Errorf("pinned setup-python must select Python 3.12")
	}
	body, _ := json.Marshal(job)
	if strings.Contains(string(body), "secrets.") || strings.Contains(string(body), "upload-artifact") || strings.Contains(string(body), "contents\":\"write") {
		return fmt.Errorf("Windows job must not use secrets, uploads, or write permissions")
	}
	verification := []string{
		"if ((Get-Location).Path -notmatch ' ') { throw 'checkout path must contain spaces' }",
		"$autocrlf = git config --global --get core.autocrlf",
		workflowExitGuard,
		"if ($autocrlf -ne 'true') { throw 'core.autocrlf must be true' }",
		"if ($env:PYTHONUTF8 -ne '0') { throw 'PYTHONUTF8 must be 0' }",
		"$claude = [System.IO.File]::ReadAllBytes('CLAUDE.md')",
		"if ([Convert]::ToHexString($claude) -ne '404147454E54532E6D640A') { throw 'CLAUDE.md must contain exact ASCII @AGENTS.md plus LF' }",
		"$eol = @(git ls-files --eol -- '*.go' '*.sh' 'CLAUDE.md' 'examples/mission/artifacts/sha256/*')",
		workflowExitGuard,
		"$goEOL = @($eol | Where-Object { $_ -match '\\.go$' })",
		"$bashEOL = @($eol | Where-Object { $_ -match '\\.sh$' })",
		"$claudeEOL = @($eol | Where-Object { $_ -match 'CLAUDE\\.md$' })",
		"$digestEOL = @($eol | Where-Object { $_ -match 'examples/mission/artifacts/sha256/' })",
		"if ($goEOL.Count -eq 0 -or @($goEOL | Where-Object { $_ -notmatch 'w/lf\\s+attr/text eol=lf\\s+' }).Count -ne 0) { throw 'Go files must be w/lf with attr/text eol=lf' }",
		"if ($bashEOL.Count -eq 0 -or @($bashEOL | Where-Object { $_ -notmatch 'w/lf\\s+attr/text eol=lf\\s+' }).Count -ne 0) { throw 'Bash files must be w/lf with attr/text eol=lf' }",
		"if ($claudeEOL.Count -ne 1 -or $claudeEOL[0] -notmatch 'w/lf\\s+attr/text eol=lf\\s+') { throw 'CLAUDE.md must be w/lf with attr/text eol=lf' }",
		"if ($digestEOL.Count -eq 0 -or @($digestEOL | Where-Object { $_ -notmatch 'attr/-text\\s+' }).Count -ne 0) { throw 'digest fixtures must be attr/-text' }",
	}
	if !equalWorkflowLines(requiredSteps["Verify default Windows checkout"].lines, verification) {
		return fmt.Errorf("Windows job must contain the exact verification commands")
	}
	formatting := []string{
		"$unformatted = gofmt -l .",
		workflowExitGuard,
		"if ($unformatted) {",
		"$unformatted",
		"exit 1",
		"}",
	}
	if workflowLineCount(parsedSteps, "$unformatted = gofmt -l .") != 1 || !equalWorkflowLines(requiredSteps["Check formatting"].lines, formatting) {
		return fmt.Errorf("Windows job must contain the exact formatting commands")
	}
	for name, command := range map[string]string{
		"Test":       "go test ./... -count=1",
		"Vet":        "go vet ./...",
		"Build":      "go build -o (Join-Path $env:RUNNER_TEMP 'ao-command.exe') ./cmd/ao-command",
		"Check diff": "git diff --check",
	} {
		if err := requireGuardedWorkflowCommand(parsedSteps, requiredSteps[name], command); err != nil {
			return err
		}
	}
	return nil
}

const workflowExitGuard = "if ($LASTEXITCODE -ne 0) { exit $LASTEXITCODE }"

func executableWorkflowLines(script string) []string {
	lines := strings.Split(strings.ReplaceAll(script, "\r\n", "\n"), "\n")
	executable := lines[:0]
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed != "" && !strings.HasPrefix(trimmed, "#") {
			executable = append(executable, trimmed)
		}
	}
	return executable
}

func equalWorkflowLines(got, want []string) bool {
	return strings.Join(got, "\n") == strings.Join(want, "\n")
}

func workflowLineCount(steps []windowsWorkflowStep, want string) int {
	count := 0
	for _, step := range steps {
		for _, line := range step.lines {
			if line == want {
				count++
			}
		}
	}
	return count
}

func requireGuardedWorkflowCommand(steps []windowsWorkflowStep, step windowsWorkflowStep, command string) error {
	if workflowLineCount(steps, command) != 1 || !equalWorkflowLines(step.lines, []string{command, workflowExitGuard}) {
		return fmt.Errorf("step %q must contain exact command %q with an immediate exit guard", step.name, command)
	}
	return nil
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
