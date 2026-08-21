package workflowpolicy

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
	"testing"
	"time"
)

func TestValidateBytesRejectsMalformedAliasAndNonMappingDocuments(t *testing.T) {
	tests := []struct {
		name string
		body string
		want string
	}{
		{name: "malformed", body: "name: broken\non: [", want: "malformed workflow"},
		{name: "alias", body: "jobs: &jobs {}\ncopy: *jobs\n", want: "YAML aliases are forbidden"},
		{name: "sequence root", body: "- workflow_dispatch\n", want: "malformed workflow: root must be a mapping"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := ValidateBytes("workflow.yml", []byte(test.body))
			assertViolation(t, err, "workflow.yml", test.want)
		})
	}
}

func TestValidateBytesRejectsNonStringMappingKeysDeterministically(t *testing.T) {
	tests := []struct {
		name string
		body string
	}{
		{name: "root", body: "1: value\n"},
		{name: "nested", body: "jobs:\n  unsafe:\n    1: extra\n    permissions: write-all\n"},
		{
			name: "colliding jobs keys",
			body: "name: collision\non: workflow_dispatch\npermissions:\n  contents: read\njobs:\n  1:\n    permissions: write-all\n  \"\\0int:1\":\n    steps: []\n",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			for range 20 {
				err := ValidateBytes("workflow.yml", []byte(test.body))
				violation, ok := err.(Violation)
				if !ok || violation.Path != "workflow.yml" || violation.Message != "malformed workflow: mapping keys must be strings" {
					t.Fatalf("violation = %#v, want stable non-string-key rejection", err)
				}
			}
		})
	}
}

func TestValidateBytesRejectsOversizeDocument(t *testing.T) {
	err := ValidateBytes("workflow.yml", []byte(strings.Repeat("#", maxFileBytes+1)))
	assertViolation(t, err, "workflow.yml", "workflow file size limit exceeded")
}

func TestValidateFilesRejectsCountAndTotalLimits(t *testing.T) {
	paths := make([]string, maxFiles+1)
	for i := range paths {
		paths[i] = fmt.Sprintf("workflow-%03d.yml", i)
	}
	assertViolation(t, ValidateFiles(paths), "", "workflow file count limit exceeded")

	dir := t.TempDir()
	body := []byte("jobs: {}\n" + strings.Repeat("#", maxFileBytes-len("jobs: {}\n")))
	paths = nil
	for i := 0; i < maxTotalBytes/maxFileBytes+1; i++ {
		path := filepath.Join(dir, fmt.Sprintf("workflow-%02d.yml", i))
		if err := os.WriteFile(path, body[:maxFileBytes], 0o644); err != nil {
			t.Fatal(err)
		}
		paths = append(paths, path)
	}
	assertViolation(t, ValidateFiles(paths), paths[len(paths)-1], "workflow total byte limit exceeded")
}

func TestValidateFilesRejectsSymlink(t *testing.T) {
	dir := t.TempDir()
	regular := filepath.Join(dir, "regular.yml")
	if err := os.WriteFile(regular, []byte("jobs: {}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	symlink := filepath.Join(dir, "link.yml")
	if err := os.Symlink(regular, symlink); err != nil {
		if runtime.GOOS == "windows" && errors.Is(err, syscall.Errno(1314)) {
			t.Skip("Windows symlink privilege is not held")
		}
		t.Fatal(err)
	}
	assertViolation(t, ValidateFiles([]string{symlink}), symlink, "workflow must not be a symlink")
}

func TestValidateFilesRejectsNonRegularAndOversizeFiles(t *testing.T) {
	dir := t.TempDir()
	assertViolation(t, ValidateFiles([]string{dir}), dir, "workflow must be a regular file")

	oversize := filepath.Join(dir, "oversize.yml")
	if err := os.WriteFile(oversize, []byte(strings.Repeat("#", maxFileBytes+1)), 0o644); err != nil {
		t.Fatal(err)
	}
	assertViolation(t, ValidateFiles([]string{oversize}), oversize, "workflow file size limit exceeded")
}

func TestValidateFilesRejectsMissingFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "missing.yml")
	assertViolation(t, ValidateFiles([]string{path}), path, "workflow cannot be inspected")
}

func TestSameFileStateRejectsSameLengthMetadataChange(t *testing.T) {
	path := filepath.Join(t.TempDir(), "workflow.yml")
	if err := os.WriteFile(path, []byte("jobs: {}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	before, err := os.Lstat(path)
	if err != nil {
		t.Fatal(err)
	}
	changed := before.ModTime().Add(time.Second)
	if err := os.Chtimes(path, changed, changed); err != nil {
		t.Fatal(err)
	}
	after, err := os.Lstat(path)
	if err != nil {
		t.Fatal(err)
	}
	if sameFileState(before, after) {
		t.Fatal("same-length metadata change treated as stable")
	}
}

func assertViolation(t *testing.T, err error, path, message string) {
	t.Helper()
	if err == nil {
		t.Fatal("unsafe workflow unexpectedly accepted")
	}
	violation, ok := err.(Violation)
	if !ok {
		t.Fatalf("error type = %T, want Violation: %v", err, err)
	}
	if violation.Path != path || !strings.Contains(violation.Message, message) {
		t.Fatalf("violation = %#v, want path %q and message containing %q", violation, path, message)
	}
}
