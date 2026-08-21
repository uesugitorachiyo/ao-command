package cli

import (
	"bytes"
	"os"
	"path/filepath"
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
	for _, name := range []string{"AGENTS.md", "REFERENCE.md"} {
		document, err := os.ReadFile(filepath.Join(root, name))
		if err != nil {
			t.Fatal(err)
		}
		for _, want := range []string{
			`Git\bin\bash.exe`,
			"Repository `.sh` gates run in Git for Windows Bash",
			"Ruby is not required.",
		} {
			if !bytes.Contains(document, []byte(want)) {
				t.Errorf("%s missing Windows source-shell contract %q", name, want)
			}
		}
	}

	reference, err := os.ReadFile(filepath.Join(root, "REFERENCE.md"))
	if err != nil {
		t.Fatal(err)
	}
	want := "scripts/ao-command-smoke.sh --forge ../ao-forge --foundry ../ao-foundry --out tmp/ao-command-smoke"
	if !bytes.Contains(reference, []byte(want)) {
		t.Errorf("REFERENCE.md missing AO Command smoke command %q", want)
	}
}
