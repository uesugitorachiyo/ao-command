package workflowpolicy

import (
	"os"
	"strings"
	"testing"
)

func TestSymlinkSetupOnlySkipsWindowsPrivilegeError(t *testing.T) {
	body, err := os.ReadFile("document_test.go")
	if err != nil {
		t.Fatal(err)
	}
	source := string(body)
	for _, want := range []string{
		`runtime.GOOS == "windows" && errors.Is(err, syscall.Errno(1314))`,
		`t.Skip("Windows symlink privilege is not held")`,
		"t.Fatal(err)",
	} {
		if !strings.Contains(source, want) {
			t.Fatalf("symlink setup missing %q", want)
		}
	}
	if strings.Contains(source, "symlink creation unavailable on this host") {
		t.Fatal("symlink setup must not swallow unexpected errors")
	}
}
