package cli

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

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

func TestWindowsSourceShellPowerShellRestoresEnvironment(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("PowerShell regression is Windows-specific")
	}
	reference := readTestDocument(t, filepath.Join("..", "..", "REFERENCE.md"))
	block := fencedBlock(markdownSection(reference, "## Windows source-shell contract"), "powershell")
	smoke := `$bashCommand = 'cd "$(cygpath -u "$AO_COMMAND_ROOT")" && scripts/ao-command-smoke.sh --forge ../ao-forge --foundry ../ao-foundry --out tmp/ao-command-smoke'`
	roundTrip := `$bashCommand = 'cd "$(cygpath -u "$AO_COMMAND_ROOT")" && test -d .'`
	shells := []string{"powershell.exe", "pwsh.exe"}
	for _, shell := range shells {
		if _, err := exec.LookPath(shell); err != nil {
			t.Fatalf("required supported shell %s is missing: %v", shell, err)
		}
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
