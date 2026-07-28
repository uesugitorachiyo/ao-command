package cli

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"testing"
)

type commandBinding struct {
	Name    string
	Handler string
}

func TestMissionCommandOutputCharacterization(t *testing.T) {
	missionFixture := func(name string) string {
		return filepath.Join("..", "..", "examples", "mission", name)
	}
	tests := []struct {
		name string
		args []string
		want string
	}{
		{
			name: "status",
			args: []string{"mission", "status", "--status", missionFixture("command-status.ready.json")},
			want: "ao_command_mission_status=ready\n" +
				"mission_id=mission-demo\n" +
				"current_route=ao-atlas\n" +
				"current_phase=atlas_import_required\n" +
				"operator_mode=read_only\n" +
				"safe_to_execute=false\n" +
				"executes_work=false\n" +
				"approves_work=false\n" +
				"mutates_repositories=false\n" +
				"exact_next_action=AO Atlas compiles mission context before AO Foundry import\n",
		},
		{
			name: "timeline",
			args: []string{"mission", "timeline", "--readback", missionFixture("mission-status-timeline.ready.json")},
			want: "ao_command_mission_timeline=ready\n" +
				"mission_id=ao-stack-month2-compatibility-workgraph\n" +
				"mission_status=active\n" +
				"current_route=ao-atlas\n" +
				"current_phase=wave3_compatibility\n" +
				"timeline_event_count=3\n" +
				"latest_event=operator_timeline\n" +
				"tested_edge_count=2\n" +
				"full_stack_compatibility_complete=false\n" +
				"operator_mode=read_only\n" +
				"safe_to_execute=false\n" +
				"executes_work=false\n" +
				"approves_work=false\n" +
				"mutates_repositories=false\n" +
				"exact_next_action=continue Wave 3 through Command operator timeline consumer test\n",
		},
		{
			name: "dashboard compact",
			args: []string{"mission", "dashboard", "--dashboard", missionFixture("dashboard.ready.json"), "--compact"},
			want: "compact_mission_status=mission=mission-demo status=active route=ao-atlas latest_route=ao-atlas events=2\n" +
				"safe_to_execute=false\n" +
				"executes_work=false\n" +
				"approves_work=false\n" +
				"mutates_repositories=false\n" +
				"exact_next_action=send authorized pack to AO Atlas\n",
		},
		{
			name: "terminal card",
			args: []string{"mission", "dashboard", "--dashboard", missionFixture("dashboard.terminal-ready.json"), "--terminal-card"},
			want: "terminal_rollup_card=mission=mission-terminal-rollup status=terminal_handoff_ready route=complete nodes=40/40 ready=0 blocked=0 failed=0\n" +
				"terminal_rollup_evidence=foundry=completed promoter=no_promotion_requested command=readback_agrees_no_promotion return_gate=final_response_allowed final_response_allowed=true\n" +
				"terminal_rollup_safety=promotion_claimed=false rsi_remains_denied=true safe_to_execute=false executes_work=false approves_work=false mutates_repositories=false\n" +
				"exact_next_action=Use the next Month 6 recommendation wave prompt; no promotion requested and RSI remains denied.\n",
		},
		{
			name: "aggregate watch",
			args: []string{
				"mission", "aggregate",
				"--status", missionFixture("command-status.ready.json"),
				"--atlas-metadata", missionFixture("atlas-workgraph-metadata.ready.json"),
				"--foundry-smoke", missionFixture("foundry-e2e-smoke.ready.json"),
				"--watch", "--iterations", "2", "--compact",
			},
			want: "compact_summary=mission=mission-demo status=ready route=ao-atlas provenance=artifact_manifest timeline_compaction_bound=true iterations=2\n" +
				"safe_to_execute=false\n" +
				"executes_work=false\n" +
				"approves_work=false\n" +
				"exact_next_action=watch Mission aggregate readback only; do not schedule or execute work from Command\n",
		},
		{
			name: "approvals",
			args: []string{"mission", "approvals", "--inbox", missionFixture("approval-inbox.ready.json")},
			want: "ao_command_mission_approvals=ready\n" +
				"mission_id=mission-demo\n" +
				"approval_count=3\n" +
				"pending_count=1\n" +
				"approved_count=1\n" +
				"denied_count=1\n" +
				"operator_mode=read_only\n" +
				"safe_to_execute=false\n" +
				"executes_work=false\n" +
				"approves_work=false\n" +
				"mutates_repositories=false\n" +
				"exact_next_action=review pending approval tickets through Covenant before any gated live action\n",
		},
		{
			name: "next",
			args: []string{"mission", "next", "--decision", missionFixture("route-decision.ready.json")},
			want: "ao_command_mission_next=ready\n" +
				"mission_id=mission-demo\n" +
				"route=ao-atlas\n" +
				"reason=objective requires workgraph, context, or long-running task management\n" +
				"safe_to_request=true\n" +
				"safe_to_execute=false\n" +
				"executes_work=false\n" +
				"approves_work=false\n" +
				"mutates_repositories=false\n" +
				"exact_next_action=AO Atlas compiles mission context before AO Foundry import\n",
		},
		{
			name: "artifacts",
			args: []string{"mission", "artifacts", "--manifest", missionFixture("artifact-manifest.ready.json")},
			want: "ao_command_mission_artifacts=ready\n" +
				"mission_id=mission-demo\n" +
				"artifact_count=3\n" +
				"operator_mode=read_only\n" +
				"safe_to_execute=false\n" +
				"executes_work=false\n" +
				"approves_work=false\n" +
				"mutates_repositories=false\n" +
				"artifact=route_readback:examples/mission/route-decision.ready.json\n" +
				"artifact=command_status:examples/mission/command-status.ready.json\n" +
				"artifact=governance_snapshot:examples/mission/governance-snapshot.ready.json\n" +
				"exact_next_action=review AO Mission artifacts as read-only evidence\n",
		},
		{
			name: "history",
			args: []string{"mission", "history", "--history", missionFixture("route-history.ready.json")},
			want: "ao_command_mission_history=ready\n" +
				"mission_id=mission-demo\n" +
				"route_count=2\n" +
				"latest_route=ao-atlas\n" +
				"operator_mode=read_only\n" +
				"safe_to_execute=false\n" +
				"executes_work=false\n" +
				"approves_work=false\n" +
				"mutates_repositories=false\n" +
				"exact_next_action=AO Atlas compiles mission context before AO Foundry import\n",
		},
		{
			name: "readiness",
			args: []string{"mission", "readiness", "--bundle", missionFixture("readiness-bundle.ready.json")},
			want: "ao_command_mission_readiness=ready\n" +
				"repo_count=2\n" +
				"ready_repos=2\n" +
				"blocked_repos=0\n" +
				"operator_mode=read_only\n" +
				"safe_to_execute=false\n" +
				"executes_work=false\n" +
				"approves_work=false\n" +
				"mutates_repositories=false\n" +
				"exact_next_action=readiness bundle verified locally; remote PR lifecycle remains operator-controlled\n",
		},
		{
			name: "gateway",
			args: []string{"mission", "gateway", "--readback", missionFixture("gateway-intent-ledger.ready.json")},
			want: "ao_command_mission_gateway=ready\n" +
				"mission_id=mission-demo\n" +
				"gateway_count=2\n" +
				"total=7\n" +
				"intent_recorded=4\n" +
				"denied=1\n" +
				"invalid=2\n" +
				"operator_mode=read_only\n" +
				"safe_to_execute=false\n" +
				"executes_work=false\n" +
				"approves_work=false\n" +
				"mutates_repositories=false\n" +
				"exact_next_action=inspect gateway intent ledger; continue through AO Mission routing, not Telegram or A2A authority\n",
		},
		{
			name: "evidence",
			args: []string{"mission", "evidence", "--readback", missionFixture("scheduler-recovery-readback.ready.json")},
			want: "ao_command_mission_evidence=ready\n" +
				"mission_id=mission-demo\n" +
				"evidence_kind=scheduler_recovery\n" +
				"operator_mode=read_only\n" +
				"safe_to_execute=false\n" +
				"schedules_work=false\n" +
				"executes_work=false\n" +
				"approves_work=false\n" +
				"mutates_repositories=false\n" +
				"exact_next_action=review scheduler recovery evidence and continue through governed AO Mission routing\n",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			code, stdout, stderr := runWithFake(test.args, &fakeRunner{})
			if code != 0 || stderr != "" {
				t.Fatalf("exit=%d stderr=%q", code, stderr)
			}
			if stdout != test.want {
				t.Fatalf("stdout changed\nwant:\n%s\ngot:\n%s", test.want, stdout)
			}
		})
	}
}

func TestCommandDispatchCharacterization(t *testing.T) {
	files := parseProductionGoFiles(t)
	wantRoot := []commandBinding{
		{Name: "help", Handler: "printHelp"},
		{Name: "--help", Handler: "printHelp"},
		{Name: "-h", Handler: "printHelp"},
		{Name: "version", Handler: "version"},
		{Name: "status", Handler: "status"},
		{Name: "stack", Handler: "stack"},
		{Name: "atlas", Handler: "atlas"},
		{Name: "pulse", Handler: "pulse"},
		{Name: "blueprint-atlas-foundry", Handler: "blueprintAtlasFoundry"},
		{Name: "complex-refactor", Handler: "complexRefactor"},
		{Name: "live-mutation", Handler: "liveMutation"},
		{Name: "mission", Handler: "mission"},
		{Name: "control-plane", Handler: "controlPlane"},
		{Name: "controlled-loop", Handler: "controlledLoop"},
		{Name: "operator", Handler: "operator"},
		{Name: "covenant", Handler: "covenant"},
		{Name: "github-issue", Handler: "githubIssue"},
		{Name: "forge", Handler: "forge"},
		{Name: "promoter", Handler: "promoter"},
		{Name: "rsi", Handler: "rsi"},
		{Name: "next", Handler: "next"},
		{Name: "goals", Handler: "goals"},
		{Name: "evidence", Handler: "evidence"},
		{Name: "rehearse", Handler: "rehearse"},
	}
	wantMission := []commandBinding{
		{Name: "aggregate", Handler: "missionAggregate"},
		{Name: "approvals", Handler: "missionApprovals"},
		{Name: "artifacts", Handler: "missionArtifacts"},
		{Name: "dashboard", Handler: "missionDashboard"},
		{Name: "evidence", Handler: "missionEvidence"},
		{Name: "gateway", Handler: "missionGateway"},
		{Name: "history", Handler: "missionHistory"},
		{Name: "next", Handler: "missionNext"},
		{Name: "readiness", Handler: "missionReadiness"},
		{Name: "status", Handler: "missionStatus"},
		{Name: "timeline", Handler: "missionTimeline"},
	}

	if got := commandBindings(t, files, "Run", "rootCommandRegistry"); !reflect.DeepEqual(got, wantRoot) {
		t.Fatalf("root command bindings changed\nwant: %#v\ngot:  %#v", wantRoot, got)
	}
	if got := commandBindings(t, files, "mission", "missionCommandRegistry"); !reflect.DeepEqual(got, wantMission) {
		t.Fatalf("mission command bindings changed\nwant: %#v\ngot:  %#v", wantMission, got)
	}
}

func TestRepresentativeCLIErrorCharacterization(t *testing.T) {
	code, stdout, stderr := runWithFake([]string{"mission", "status"}, &fakeRunner{})
	if code != 2 || stdout != "" || stderr != "ao-command mission status: --status is required\n" {
		t.Fatalf("mission status error changed: exit=%d stdout=%q stderr=%q", code, stdout, stderr)
	}

	code, stdout, stderr = runWithFake([]string{"unknown-command"}, &fakeRunner{})
	if code != 2 || stderr != "ao-command: unknown command \"unknown-command\"\n" {
		t.Fatalf("unknown command error changed: exit=%d stderr=%q", code, stderr)
	}
	if stdout == "" {
		t.Fatal("unknown command must retain root help on stdout")
	}
}

func parseProductionGoFiles(t *testing.T) []*ast.File {
	t.Helper()
	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatal(err)
	}
	fset := token.NewFileSet()
	var files []*ast.File
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || filepath.Ext(name) != ".go" || len(name) >= 8 && name[len(name)-8:] == "_test.go" {
			continue
		}
		file, err := parser.ParseFile(fset, name, nil, 0)
		if err != nil {
			t.Fatalf("parse %s: %v", name, err)
		}
		files = append(files, file)
	}
	return files
}

func commandBindings(t *testing.T, files []*ast.File, methodName, registryName string) []commandBinding {
	t.Helper()
	if bindings := switchCommandBindings(files, methodName); len(bindings) > 0 {
		return bindings
	}
	bindings := registryCommandBindings(files, registryName)
	if len(bindings) == 0 {
		t.Fatalf("no dispatch switch or %s declaration found", registryName)
	}
	return bindings
}

func switchCommandBindings(files []*ast.File, methodName string) []commandBinding {
	var bindings []commandBinding
	for _, file := range files {
		for _, decl := range file.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok || fn.Name.Name != methodName || fn.Recv == nil {
				continue
			}
			ast.Inspect(fn.Body, func(node ast.Node) bool {
				switchStmt, ok := node.(*ast.SwitchStmt)
				if !ok {
					return true
				}
				for _, statement := range switchStmt.Body.List {
					clause := statement.(*ast.CaseClause)
					handler := calledAppMethod(clause.Body)
					for _, expression := range clause.List {
						literal, ok := expression.(*ast.BasicLit)
						if !ok || literal.Kind != token.STRING || handler == "" {
							continue
						}
						name, _ := strconv.Unquote(literal.Value)
						bindings = append(bindings, commandBinding{Name: name, Handler: handler})
					}
				}
				return false
			})
		}
	}
	return bindings
}

func registryCommandBindings(files []*ast.File, registryName string) []commandBinding {
	handlers := map[string]string{}
	for _, file := range files {
		for _, decl := range file.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if ok && fn.Recv == nil && fn.Body != nil {
				handlers[fn.Name.Name] = calledAppMethod(fn.Body.List)
			}
		}
	}

	var bindings []commandBinding
	for _, file := range files {
		for _, decl := range file.Decls {
			gen, ok := decl.(*ast.GenDecl)
			if !ok || gen.Tok != token.VAR {
				continue
			}
			for _, spec := range gen.Specs {
				valueSpec := spec.(*ast.ValueSpec)
				if len(valueSpec.Names) != 1 || valueSpec.Names[0].Name != registryName || len(valueSpec.Values) != 1 {
					continue
				}
				registry := valueSpec.Values[0].(*ast.CompositeLit)
				for _, element := range registry.Elts {
					entry := element.(*ast.CompositeLit)
					var names []string
					var handler string
					for _, field := range entry.Elts {
						pair := field.(*ast.KeyValueExpr)
						key := pair.Key.(*ast.Ident).Name
						switch key {
						case "names":
							list := pair.Value.(*ast.CompositeLit)
							for _, item := range list.Elts {
								value, _ := strconv.Unquote(item.(*ast.BasicLit).Value)
								names = append(names, value)
							}
						case "handler":
							handler = handlers[pair.Value.(*ast.Ident).Name]
						}
					}
					for _, name := range names {
						bindings = append(bindings, commandBinding{Name: name, Handler: handler})
					}
				}
			}
		}
	}
	return bindings
}

func calledAppMethod(statements []ast.Stmt) string {
	var handler string
	for _, statement := range statements {
		ast.Inspect(statement, func(node ast.Node) bool {
			call, ok := node.(*ast.CallExpr)
			if !ok {
				return true
			}
			selector, ok := call.Fun.(*ast.SelectorExpr)
			if !ok {
				return true
			}
			receiver, ok := selector.X.(*ast.Ident)
			if ok && receiver.Name == "a" {
				handler = selector.Sel.Name
				return false
			}
			return true
		})
		if handler != "" {
			return handler
		}
	}
	return ""
}
