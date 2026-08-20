package workflowpolicy

import (
	"bytes"
	"fmt"
	"io"
	"os"

	"go.yaml.in/yaml/v3"
)

const (
	maxFiles      = 256
	maxFileBytes  = 1 << 20
	maxTotalBytes = 8 << 20
)

// Violation describes the first workflow policy violation.
type Violation struct {
	Path    string
	Message string
}

func (v Violation) Error() string {
	if v.Path == "" {
		return v.Message
	}
	return v.Path + ": " + v.Message
}

// ValidateFiles validates workflow files using bounded, race-resistant reads.
func ValidateFiles(paths []string) error {
	if len(paths) > maxFiles {
		return Violation{Message: "workflow file count limit exceeded"}
	}
	total := 0
	for _, path := range paths {
		body, err := readFile(path)
		if err != nil {
			return err
		}
		total += len(body)
		if total > maxTotalBytes {
			return Violation{Path: path, Message: "workflow total byte limit exceeded"}
		}
		if err := validateDocument(path, body); err != nil {
			return err
		}
	}
	return nil
}

// ValidateBytes validates one in-memory workflow document.
func ValidateBytes(path string, body []byte) error {
	if len(body) > maxFileBytes {
		return Violation{Path: path, Message: "workflow file size limit exceeded"}
	}
	return validateDocument(path, body)
}

func readFile(path string) ([]byte, error) {
	before, err := os.Lstat(path)
	if err != nil {
		return nil, Violation{Path: path, Message: "workflow cannot be inspected: " + err.Error()}
	}
	if before.Mode()&os.ModeSymlink != 0 {
		return nil, Violation{Path: path, Message: "workflow must not be a symlink"}
	}
	if !before.Mode().IsRegular() {
		return nil, Violation{Path: path, Message: "workflow must be a regular file"}
	}
	if before.Size() > maxFileBytes {
		return nil, Violation{Path: path, Message: "workflow file size limit exceeded"}
	}

	file, err := os.Open(path)
	if err != nil {
		return nil, Violation{Path: path, Message: "workflow cannot be opened: " + err.Error()}
	}
	defer file.Close()
	opened, err := file.Stat()
	if err != nil || !opened.Mode().IsRegular() || !sameFileState(before, opened) {
		return nil, Violation{Path: path, Message: "workflow changed while being inspected"}
	}
	body, err := io.ReadAll(io.LimitReader(file, maxFileBytes+1))
	if err != nil {
		return nil, Violation{Path: path, Message: "workflow cannot be read: " + err.Error()}
	}
	if len(body) > maxFileBytes {
		return nil, Violation{Path: path, Message: "workflow file size limit exceeded"}
	}
	afterHandle, handleErr := file.Stat()
	afterPath, pathErr := os.Lstat(path)
	if handleErr != nil || pathErr != nil || afterPath.Mode()&os.ModeSymlink != 0 ||
		!afterPath.Mode().IsRegular() || !sameFileState(before, afterHandle) || !sameFileState(before, afterPath) ||
		afterHandle.Size() != int64(len(body)) || afterPath.Size() != int64(len(body)) {
		return nil, Violation{Path: path, Message: "workflow changed while being inspected"}
	}
	return body, nil
}

func sameFileState(before, after os.FileInfo) bool {
	return os.SameFile(before, after) && before.Mode() == after.Mode() && before.Size() == after.Size() && before.ModTime() == after.ModTime()
}

func validateDocument(path string, body []byte) error {
	var document yaml.Node
	decoder := yaml.NewDecoder(bytes.NewReader(body))
	if err := decoder.Decode(&document); err != nil {
		return Violation{Path: path, Message: "malformed workflow: " + err.Error()}
	}
	if hasAlias(&document) {
		return Violation{Path: path, Message: "malformed workflow: YAML aliases are forbidden"}
	}
	if len(document.Content) != 1 || document.Content[0].Kind != yaml.MappingNode {
		return Violation{Path: path, Message: "malformed workflow: root must be a mapping"}
	}
	var workflow map[string]any
	if err := document.Content[0].Decode(&workflow); err != nil {
		return Violation{Path: path, Message: "malformed workflow: " + err.Error()}
	}
	if err := validateWorkflow(workflow); err != nil {
		return Violation{Path: path, Message: err.Error()}
	}
	return nil
}

func hasAlias(node *yaml.Node) bool {
	if node == nil {
		return false
	}
	if node.Kind == yaml.AliasNode {
		return true
	}
	for _, child := range node.Content {
		if hasAlias(child) {
			return true
		}
	}
	return false
}

func malformed(format string, args ...any) error {
	return fmt.Errorf(format, args...)
}
