package cli

import (
	"archive/zip"
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

var windowsCandidateMembers = []string{
	"ao-command.exe",
	"LICENSE",
	"functional-smoke.json",
	"help-smoke.txt",
	"provenance.json",
	"sbom.json",
	"version-readback.json",
}

func TestWindowsCandidateQualification(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("Windows candidate qualification requires Windows")
	}

	binaryBytes := buildWindowsQualificationBinary(t)
	for _, shell := range []string{"powershell.exe", "pwsh.exe"} {
		shell := shell
		t.Run(shell, func(t *testing.T) {
			path, err := exec.LookPath(shell)
			if err != nil {
				if shell == "pwsh.exe" {
					t.Skip("PowerShell 7 is not installed locally")
				}
				t.Fatal("powershell.exe is required on Windows")
			}
			fixture := newWindowsQualificationFixture(t, binaryBytes)
			report, output, err := fixture.run(t, path)
			if err != nil {
				t.Fatalf("qualification failed: %v\n%s", err, output)
			}
			if report.SchemaVersion != "ao.command.windows-candidate-qualification.v0.1" || report.Status != "passed" {
				t.Fatalf("unexpected report identity: %+v", report)
			}
			expectedEdition := "Desktop"
			if shell == "pwsh.exe" {
				expectedEdition = "Core"
			}
			if report.PowerShellEdition != expectedEdition || report.PowerShellVersion == "" {
				t.Fatalf("unexpected PowerShell identity: %+v", report)
			}
			if report.Archive != fixture.archive || report.ArchiveSHA256 != fixture.archiveDigest || report.SourceCommit != releaseSource || report.Version != releaseVersion {
				t.Fatalf("unexpected candidate binding: %+v", report)
			}
			if !report.InstallPathContainsSpaces || !report.CleanupVerified || report.ProviderCalls {
				t.Fatalf("unexpected qualification boundary: %+v", report)
			}
			if report.Doctor.Status != "not_applicable" || report.Doctor.ReplacementDiagnostic != "version_and_mission_status" {
				t.Fatalf("unexpected doctor readback: %+v", report.Doctor)
			}
			if report.Authority != (windowsQualificationAuthority{}) {
				t.Fatalf("qualification granted authority: %+v", report.Authority)
			}
		})
	}
}

func TestWindowsCandidateQualificationRejectsInvalidCandidates(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("Windows candidate qualification requires Windows")
	}
	powershell, err := exec.LookPath("powershell.exe")
	if err != nil {
		t.Fatal("powershell.exe is required on Windows")
	}

	binaryBytes := buildWindowsQualificationBinary(t)
	tests := []struct {
		name   string
		mutate func(*testing.T, *windowsQualificationFixture)
	}{
		{name: "substituted_archive_bytes", mutate: func(t *testing.T, f *windowsQualificationFixture) {
			writeTestFile(t, filepath.Join(f.candidateDir, f.archive), []byte("substituted"))
		}},
		{name: "mismatched_checksum", mutate: func(t *testing.T, f *windowsQualificationFixture) {
			writeTestFile(t, filepath.Join(f.candidateDir, "SHA256SUMS"), []byte(strings.Repeat("0", 64)+"  "+f.archive+"\n"))
		}},
		{name: "malformed_checksum", mutate: func(t *testing.T, f *windowsQualificationFixture) {
			writeTestFile(t, filepath.Join(f.candidateDir, "SHA256SUMS"), []byte(f.archiveDigest+" *"+f.archive+"\n"))
		}},
		{name: "duplicate_checksum_entry", mutate: func(t *testing.T, f *windowsQualificationFixture) {
			line := f.archiveDigest + "  " + f.archive + "\n"
			writeTestFile(t, filepath.Join(f.candidateDir, "SHA256SUMS"), []byte(line+line))
		}},
		{name: "version_mismatch", mutate: func(_ *testing.T, f *windowsQualificationFixture) { f.expectedVersion = "9.9.9" }},
		{name: "source_mismatch", mutate: func(_ *testing.T, f *windowsQualificationFixture) { f.expectedSource = strings.Repeat("9", 40) }},
		{name: "unexpected_inventory", mutate: func(t *testing.T, f *windowsQualificationFixture) {
			f.rewriteArchive(t, map[string][]byte{"unexpected.txt": []byte("unexpected")})
		}},
		{name: "traversal_member", mutate: func(t *testing.T, f *windowsQualificationFixture) {
			f.rewriteArchive(t, map[string][]byte{"../outside.txt": []byte("unsafe")})
		}},
		{name: "absolute_member", mutate: func(t *testing.T, f *windowsQualificationFixture) {
			f.rewriteArchive(t, map[string][]byte{"C:/outside.txt": []byte("unsafe")})
		}},
		{name: "duplicate_archive_member", mutate: func(t *testing.T, f *windowsQualificationFixture) {
			f.rewriteArchiveWithDuplicate(t, "LICENSE")
		}},
		{name: "output_outside_candidate", mutate: func(t *testing.T, f *windowsQualificationFixture) {
			f.outputPath = filepath.Join(t.TempDir(), "outside-report.json")
		}},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			fixture := newWindowsQualificationFixture(t, binaryBytes)
			test.mutate(t, fixture)
			_, output, err := fixture.run(t, powershell)
			if err == nil {
				t.Fatalf("invalid candidate unexpectedly passed:\n%s", output)
			}
		})
	}
}

func TestWindowsCandidateQualificationRejectsCleanupFailure(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("Windows candidate qualification requires Windows")
	}
	powershell, err := exec.LookPath("powershell.exe")
	if err != nil {
		t.Fatal("powershell.exe is required on Windows")
	}
	fixture := newWindowsQualificationFixture(t, buildWindowsQualificationBinary(t))
	tempRoot := filepath.Join(t.TempDir(), "Qualification Temp With Spaces")
	installRoot := filepath.Join(tempRoot, "AO Command Candidate Install With Spaces")
	if err := os.MkdirAll(installRoot, 0o755); err != nil {
		t.Fatal(err)
	}
	deny := exec.Command("icacls.exe", installRoot, "/deny", "*S-1-1-0:(OI)(CI)(D,DC)")
	if output, err := deny.CombinedOutput(); err != nil {
		t.Fatalf("configure cleanup denial: %v\n%s", err, output)
	}
	t.Cleanup(func() {
		reset := exec.Command("icacls.exe", installRoot, "/remove:d", "*S-1-1-0", "/T", "/C")
		if output, err := reset.CombinedOutput(); err != nil {
			t.Errorf("restore cleanup ACL: %v\n%s", err, output)
		}
	})
	fixture.environment = map[string]string{"TEMP": tempRoot, "TMP": tempRoot}
	_, output, err := fixture.run(t, powershell)
	if err == nil || !strings.Contains(output, "candidate install cleanup could not be verified") {
		t.Fatalf("cleanup failure = %v, output %q", err, output)
	}
}

type windowsQualificationReport struct {
	SchemaVersion             string                        `json:"schema_version"`
	Status                    string                        `json:"status"`
	Archive                   string                        `json:"archive"`
	ArchiveSHA256             string                        `json:"archive_sha256"`
	SourceCommit              string                        `json:"source_commit"`
	Version                   string                        `json:"version"`
	PowerShellEdition         string                        `json:"powershell_edition"`
	PowerShellVersion         string                        `json:"powershell_version"`
	InstallPathContainsSpaces bool                          `json:"install_path_contains_spaces"`
	ProviderCalls             bool                          `json:"provider_calls"`
	CleanupVerified           bool                          `json:"cleanup_verified"`
	Doctor                    windowsQualificationDoctor    `json:"doctor"`
	Authority                 windowsQualificationAuthority `json:"authority"`
}

type windowsQualificationDoctor struct {
	Status                string `json:"status"`
	ReplacementDiagnostic string `json:"replacement_diagnostic"`
}

type windowsQualificationAuthority struct {
	SafeToExecute       bool `json:"safe_to_execute"`
	ExecutesWork        bool `json:"executes_work"`
	ApprovesWork        bool `json:"approves_work"`
	MutatesRepositories bool `json:"mutates_repositories"`
	ReleasesOrDeploys   bool `json:"releases_or_deploys"`
}

type windowsQualificationFixture struct {
	repoRoot        string
	candidateDir    string
	archive         string
	archiveDigest   string
	expectedSource  string
	expectedVersion string
	outputPath      string
	members         map[string][]byte
	environment     map[string]string
}

func buildWindowsQualificationBinary(t *testing.T) []byte {
	t.Helper()
	repoRoot, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	if err := os.MkdirAll(root, 0o755); err != nil {
		t.Fatal(err)
	}
	binary := filepath.Join(root, "ao-command.exe")
	ldflags := fmt.Sprintf("-X github.com/uesugitorachiyo/ao-command/internal/cli.buildVersion=%s -X github.com/uesugitorachiyo/ao-command/internal/cli.buildSourceCommit=%s", releaseVersion, releaseSource)
	command := exec.Command("go", "build", "-trimpath", "-ldflags", ldflags, "-o", binary, "./cmd/ao-command")
	command.Dir = repoRoot
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("build Windows candidate: %v\n%s", err, output)
	}
	binaryBytes, err := os.ReadFile(binary)
	if err != nil {
		t.Fatal(err)
	}
	return binaryBytes
}

func newWindowsQualificationFixture(t *testing.T, binaryBytes []byte) *windowsQualificationFixture {
	t.Helper()
	repoRoot, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	root := filepath.Join(t.TempDir(), "Candidate Inputs With Spaces")
	if err := os.MkdirAll(root, 0o755); err != nil {
		t.Fatal(err)
	}
	provenance := []byte(`{"executable_format":"pe","goarch":"amd64","goos":"windows","provider_calls":false,"repository":"ao-command","runner_arch":"X64","schema_version":"ao.command.release-rehearsal-provenance.v0.1","source_commit":"` + releaseSource + `","target":"windows-x86_64","version":"` + releaseVersion + `"}`)
	members := map[string][]byte{
		"ao-command.exe":        binaryBytes,
		"LICENSE":               []byte("license"),
		"functional-smoke.json": []byte(`{"provider_calls":false,"status":"passed"}`),
		"help-smoke.txt":        []byte("help"),
		"provenance.json":       provenance,
		"sbom.json":             []byte(`{"modules":[]}`),
		"version-readback.json": []byte(`{"provider_calls":false}`),
	}
	archive := "ao-command-" + releaseVersion + "-windows-x86_64.zip"
	f := &windowsQualificationFixture{
		repoRoot: repoRoot, candidateDir: root, archive: archive,
		expectedSource: releaseSource, expectedVersion: releaseVersion,
		outputPath: filepath.Join(root, "Qualification Reports With Spaces", "report.json"), members: members,
	}
	f.writeArchive(t, members, "")
	return f
}

func (f *windowsQualificationFixture) run(t *testing.T, shell string) (windowsQualificationReport, string, error) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(f.outputPath), 0o755); err != nil {
		t.Fatal(err)
	}
	script := filepath.Join(f.repoRoot, "scripts", "qualify-windows-candidate.ps1")
	command := exec.Command(shell, "-NoLogo", "-NoProfile", "-NonInteractive", "-File", script,
		"-CandidateDirectory", f.candidateDir,
		"-Archive", f.archive,
		"-ExpectedSourceCommit", f.expectedSource,
		"-ExpectedVersion", f.expectedVersion,
		"-Output", f.outputPath,
	)
	command.Dir = f.repoRoot
	command.Env = os.Environ()
	for key, value := range f.environment {
		command.Env = append(command.Env, key+"="+value)
	}
	output, err := command.CombinedOutput()
	var report windowsQualificationReport
	if data, readErr := os.ReadFile(f.outputPath); readErr == nil {
		_ = json.Unmarshal(bytes.TrimPrefix(data, []byte{0xef, 0xbb, 0xbf}), &report)
	}
	return report, string(output), err
}

func (f *windowsQualificationFixture) rewriteArchive(t *testing.T, extra map[string][]byte) {
	t.Helper()
	members := make(map[string][]byte, len(f.members)+len(extra))
	for name, data := range f.members {
		members[name] = data
	}
	for name, data := range extra {
		members[name] = data
	}
	f.writeArchive(t, members, "")
}

func (f *windowsQualificationFixture) rewriteArchiveWithDuplicate(t *testing.T, duplicate string) {
	t.Helper()
	f.writeArchive(t, f.members, duplicate)
}

func (f *windowsQualificationFixture) writeArchive(t *testing.T, members map[string][]byte, duplicate string) {
	t.Helper()
	var buffer bytes.Buffer
	packageWriter := zip.NewWriter(&buffer)
	for _, name := range windowsCandidateMembers {
		data, ok := members[name]
		if !ok {
			continue
		}
		writer, err := packageWriter.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := writer.Write(data); err != nil {
			t.Fatal(err)
		}
	}
	for name, data := range members {
		found := false
		for _, standard := range windowsCandidateMembers {
			if name == standard {
				found = true
			}
		}
		if found {
			continue
		}
		writer, err := packageWriter.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := writer.Write(data); err != nil {
			t.Fatal(err)
		}
	}
	if duplicate != "" {
		writer, err := packageWriter.Create(duplicate)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := writer.Write(members[duplicate]); err != nil {
			t.Fatal(err)
		}
	}
	if err := packageWriter.Close(); err != nil {
		t.Fatal(err)
	}
	writeTestFile(t, filepath.Join(f.candidateDir, f.archive), buffer.Bytes())
	f.archiveDigest = digestBytes(buffer.Bytes())
	writeTestFile(t, filepath.Join(f.candidateDir, "SHA256SUMS"), []byte(f.archiveDigest+"  "+f.archive+"\n"))
}
