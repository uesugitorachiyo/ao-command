package cli

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"go.yaml.in/yaml/v3"
)

const workflowTestFileLimit = 1 << 20

func readWorkflowTestFile(path string) (string, error) {
	info, err := os.Lstat(path)
	if err != nil {
		return "", err
	}
	if !info.Mode().IsRegular() {
		return "", fmt.Errorf("workflow must be a regular file")
	}
	if info.Size() > workflowTestFileLimit {
		return "", fmt.Errorf("workflow exceeds %d-byte limit", workflowTestFileLimit)
	}

	file, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer file.Close()
	data, err := io.ReadAll(io.LimitReader(file, workflowTestFileLimit+1))
	if err != nil {
		return "", err
	}
	if len(data) > workflowTestFileLimit {
		return "", fmt.Errorf("workflow exceeds %d-byte limit", workflowTestFileLimit)
	}
	data = bytes.ReplaceAll(data, []byte("\r\n"), []byte("\n"))
	if bytes.ContainsRune(data, '\r') {
		return "", fmt.Errorf("workflow contains a lone carriage return")
	}
	return string(data), nil
}

func parseWorkflowTestYAML(text string) (map[string]any, error) {
	var document map[string]any
	if err := yaml.Unmarshal([]byte(text), &document); err != nil {
		return nil, err
	}
	if document == nil {
		return nil, fmt.Errorf("workflow YAML must be a mapping")
	}
	return document, nil
}

func TestWorkflowTestReaderNormalizesLFAndCRLFEqually(t *testing.T) {
	dir := t.TempDir()
	lfPath := filepath.Join(dir, "lf.yml")
	crlfPath := filepath.Join(dir, "crlf.yml")
	lf := []byte("on:\n  workflow_dispatch:\njobs:\n  verify:\n    runs-on: windows-latest\n")
	if err := os.WriteFile(lfPath, lf, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(crlfPath, []byte("on:\r\n  workflow_dispatch:\r\njobs:\r\n  verify:\r\n    runs-on: windows-latest\r\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	lfText, err := readWorkflowTestFile(lfPath)
	if err != nil {
		t.Fatal(err)
	}
	crlfText, err := readWorkflowTestFile(crlfPath)
	if err != nil {
		t.Fatal(err)
	}
	if lfText != crlfText {
		t.Fatalf("normalized workflow text differs:\nLF: %q\nCRLF: %q", lfText, crlfText)
	}
	lfDocument, err := parseWorkflowTestYAML(lfText)
	if err != nil {
		t.Fatal(err)
	}
	crlfDocument, err := parseWorkflowTestYAML(crlfText)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(lfDocument, crlfDocument) {
		t.Fatalf("parsed workflow documents differ:\nLF: %#v\nCRLF: %#v", lfDocument, crlfDocument)
	}
}

func TestWorkflowTestReaderRejectsLoneCarriageReturn(t *testing.T) {
	path := filepath.Join(t.TempDir(), "lone-cr.yml")
	if err := os.WriteFile(path, []byte("on:\rworkflow_dispatch:\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := readWorkflowTestFile(path); err == nil {
		t.Fatal("workflow reader accepted a lone carriage return")
	}
}
