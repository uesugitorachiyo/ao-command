package cli

import (
	"encoding/json"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

type readbackArchitectureBaseline struct {
	SchemaVersion string `json:"schema_version"`
	SourceTree    string `json:"source_tree"`
	Measurements  map[string]struct {
		Lines        int      `json:"lines"`
		Declarations []string `json:"owned_declarations"`
	} `json:"measurements"`
}

func TestStackReadbackArchitectureRatchet(t *testing.T) {
	const sourceMovementTree = "b8a88c4b90867bc4b17a8d97cd289a914d65531c"

	root := filepath.Clean(filepath.Join("..", ".."))
	body, err := os.ReadFile(filepath.Join(root, ".github", "architecture-baseline.json"))
	if err != nil {
		t.Fatalf("read architecture baseline: %v", err)
	}
	var baseline readbackArchitectureBaseline
	if err := json.Unmarshal(body, &baseline); err != nil {
		t.Fatalf("decode architecture baseline: %v", err)
	}
	if baseline.SchemaVersion != "ao-command.go-architecture-baseline.v1" {
		t.Fatalf("unexpected architecture baseline schema: %q", baseline.SchemaVersion)
	}
	if baseline.SourceTree != sourceMovementTree {
		t.Fatalf("architecture baseline source tree drifted: %q != %q", baseline.SourceTree, sourceMovementTree)
	}
	if len(baseline.Measurements) == 0 {
		t.Fatal("architecture baseline has no measurements")
	}

	expectedOwnership := make(map[string][]string, len(baseline.Measurements))
	for relative, expected := range baseline.Measurements {
		if len(expected.Declarations) == 0 {
			t.Errorf("%s has no expected readback declarations", relative)
		}
		source, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(relative)))
		if err != nil {
			t.Errorf("read measured source %s: %v", relative, err)
			continue
		}
		if lines := strings.Count(string(source), "\n"); lines > expected.Lines {
			t.Errorf("%s grew above architecture ratchet: %d > %d lines", relative, lines, expected.Lines)
		}
		expectedOwnership[relative] = expected.Declarations
	}

	actualOwnership := map[string][]string{}
	cliDir := filepath.Join(root, "internal", "cli")
	entries, err := os.ReadDir(cliDir)
	if err != nil {
		t.Fatalf("read CLI package: %v", err)
	}
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".go") || strings.HasSuffix(entry.Name(), "_test.go") {
			continue
		}
		path := filepath.Join(cliDir, entry.Name())
		parsed, err := parser.ParseFile(token.NewFileSet(), path, nil, 0)
		if err != nil {
			t.Errorf("parse CLI source %s: %v", entry.Name(), err)
			continue
		}
		declarations := measuredReadbackDeclarations(parsed)
		if len(declarations) != 0 {
			relative := filepath.ToSlash(filepath.Join("internal", "cli", entry.Name()))
			actualOwnership[relative] = declarations
		}
	}
	if !equalReadbackOwnership(actualOwnership, expectedOwnership) {
		t.Errorf("stack readback ownership drifted:\nactual:   %v\nexpected: %v", actualOwnership, expectedOwnership)
	}
}

func measuredReadbackDeclarations(file *ast.File) []string {
	var result []string
	for _, declaration := range file.Decls {
		switch value := declaration.(type) {
		case *ast.FuncDecl:
			if measuredReadbackName(value.Name.Name) {
				result = append(result, "func:"+value.Name.Name)
			}
		case *ast.GenDecl:
			for _, spec := range value.Specs {
				typeSpec, ok := spec.(*ast.TypeSpec)
				if ok && measuredReadbackName(typeSpec.Name.Name) {
					result = append(result, "type:"+typeSpec.Name.Name)
				}
			}
		}
	}
	sort.Strings(result)
	return result
}

func measuredReadbackName(name string) bool {
	prefixes := []string{
		"activeStack", "atlasStatus", "atlasAuthority", "foundryAtlas",
		"pulseGate", "foundryPulse", "pulseSource", "atlasBlueprint",
		"atlasSource", "blueprintAtlas", "foundryComplexRefactor",
		"complexRefactor", "liveMutation", "lowRiskCode", "multiRepoLive",
		"readActiveStack", "readAtlas", "validateFoundryAtlas",
		"validateAtlasAuthority", "readPulse", "readBlueprint",
		"validatePulse", "validateAtlasBlueprint", "validateAtlasSource",
		"validateBlueprint", "readComplexRefactor", "deriveComplexRefactor",
		"validateComplexRefactor", "readLiveMutation", "validateLiveMutation",
		"newLowRisk", "newMultiRepo", "previousLiveMutation",
	}
	for _, prefix := range prefixes {
		if strings.HasPrefix(name, prefix) {
			return true
		}
	}
	return name == "isPulseStatus" || name == "isHexSHA256" || name == "jsonEquivalent"
}

func equalReadbackOwnership(left, right map[string][]string) bool {
	if len(left) != len(right) {
		return false
	}
	for path, declarations := range left {
		expected := right[path]
		if len(declarations) != len(expected) {
			return false
		}
		for index := range declarations {
			if declarations[index] != expected[index] {
				return false
			}
		}
	}
	return true
}
