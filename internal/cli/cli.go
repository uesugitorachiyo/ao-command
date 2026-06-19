package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

const (
	commandSchemaVersion = "ao.command.v0.1"
	operatorMode         = "read_only"
	releaseGovernance    = "blocked_pending_operator_approval"
)

type Command struct {
	Dir  string
	Env  []string
	Name string
	Args []string
}

type Runner interface {
	Run(ctx context.Context, cmd Command) ([]byte, []byte, error)
}

type execRunner struct{}

func (execRunner) Run(ctx context.Context, cmd Command) ([]byte, []byte, error) {
	c := exec.CommandContext(ctx, cmd.Name, cmd.Args...)
	c.Dir = cmd.Dir
	c.Env = append(os.Environ(), cmd.Env...)
	var stdout, stderr bytes.Buffer
	c.Stdout = &stdout
	c.Stderr = &stderr
	err := c.Run()
	return stdout.Bytes(), stderr.Bytes(), err
}

type App struct {
	Runner Runner
	Stdout io.Writer
	Stderr io.Writer
}

func Main(args []string, stdout, stderr io.Writer) int {
	app := App{Runner: execRunner{}, Stdout: stdout, Stderr: stderr}
	return app.Run(context.Background(), args)
}

func (a App) Run(ctx context.Context, args []string) int {
	if a.Runner == nil {
		a.Runner = execRunner{}
	}
	if a.Stdout == nil {
		a.Stdout = io.Discard
	}
	if a.Stderr == nil {
		a.Stderr = io.Discard
	}
	if len(args) == 0 {
		a.printHelp()
		return 0
	}

	switch args[0] {
	case "help", "--help", "-h":
		a.printHelp()
		return 0
	case "status":
		return a.status(ctx, args[1:])
	case "next":
		return a.next(ctx, args[1:])
	case "goals":
		return a.goals(ctx, args[1:])
	case "evidence":
		return a.evidence(ctx, args[1:])
	case "rehearse":
		return a.rehearse(ctx, args[1:])
	default:
		fmt.Fprintf(a.Stderr, "ao-command: unknown command %q\n", args[0])
		a.printHelp()
		return 2
	}
}

func (a App) printHelp() {
	fmt.Fprintln(a.Stdout, `ao-command is the read-only operator command surface for AO Forge.

Usage:
  ao-command status [--forge PATH] [--forge-bin PATH] [--json]
  ao-command next [--forge PATH] [--forge-bin PATH] [--json]
  ao-command goals --goal-run PATH [--forge PATH] [--forge-bin PATH] [--json]
  ao-command evidence --schema PATH --document PATH [--forge PATH] [--forge-bin PATH] [--json]
  ao-command rehearse --tag TAG --out DIR [--forge PATH] [--forge-bin PATH] [--json]

Commands are read-only by default. Rehearsal writes only dry-run evidence to the
operator-provided output directory and relies on AO Forge release-preview proofs.`)
}

type commonFlags struct {
	forgeDir string
	forgeBin string
	jsonOut  bool
	timeout  time.Duration
}

func addCommonFlags(fs *flag.FlagSet, c *commonFlags) {
	fs.StringVar(&c.forgeDir, "forge", defaultForgeDir(), "path to the ao-forge checkout")
	fs.StringVar(&c.forgeBin, "forge-bin", os.Getenv("AO_FORGE_BIN"), "path to a built forge binary")
	fs.BoolVar(&c.jsonOut, "json", false, "emit JSON")
	fs.DurationVar(&c.timeout, "timeout", 2*time.Minute, "command timeout")
}

func defaultForgeDir() string {
	if dir := os.Getenv("AO_FORGE_REPO"); dir != "" {
		return dir
	}
	return filepath.Clean(filepath.Join("..", "ao-forge"))
}

func (a App) status(ctx context.Context, args []string) int {
	var flags commonFlags
	fs := flag.NewFlagSet("status", flag.ContinueOnError)
	fs.SetOutput(a.Stderr)
	addCommonFlags(fs, &flags)
	if err := fs.Parse(args); err != nil {
		return 2
	}

	audit, code := a.readReadiness(ctx, flags)
	if code != 0 {
		return code
	}
	if flags.jsonOut {
		return a.writeJSON(statusSummaryFromAudit(flags.forgeDir, audit))
	}
	fmt.Fprintf(a.Stdout, "ao_command_status=%s\n", audit.Status)
	fmt.Fprintf(a.Stdout, "forge=%s\n", flags.forgeDir)
	fmt.Fprintf(a.Stdout, "readiness_percent=%d\n", audit.ReadinessPercent)
	fmt.Fprintf(a.Stdout, "gates=%d/%d\n", audit.PassedGates, audit.TotalGates)
	fmt.Fprintf(a.Stdout, "next_actions=%d\n", len(audit.NextActions))
	fmt.Fprintf(a.Stdout, "required_next_actions=%d\n", requiredNextActionCount(audit.NextActions))
	fmt.Fprintf(a.Stdout, "production_ready=%t\n", productionReady(audit))
	fmt.Fprintf(a.Stdout, "operator_mode=%s\n", operatorMode)
	fmt.Fprintf(a.Stdout, "release_governance=%s\n", releaseGovernance)
	return 0
}

func (a App) next(ctx context.Context, args []string) int {
	var flags commonFlags
	fs := flag.NewFlagSet("next", flag.ContinueOnError)
	fs.SetOutput(a.Stderr)
	addCommonFlags(fs, &flags)
	if err := fs.Parse(args); err != nil {
		return 2
	}

	audit, code := a.readReadiness(ctx, flags)
	if code != 0 {
		return code
	}
	summary := nextSummaryFromAudit(flags.forgeDir, audit)
	if flags.jsonOut {
		return a.writeJSON(summary)
	}
	fmt.Fprintf(a.Stdout, "ao_command_next=%s\n", summary.Status)
	fmt.Fprintf(a.Stdout, "readiness_percent=%d\n", summary.ReadinessPercent)
	fmt.Fprintf(a.Stdout, "next_action=%s required=%t\n", summary.NextAction.ActionID, summary.NextAction.Required)
	fmt.Fprintf(a.Stdout, "summary=%s\n", summary.NextAction.Description)
	return 0
}

func (a App) goals(ctx context.Context, args []string) int {
	var flags commonFlags
	var goalRun string
	fs := flag.NewFlagSet("goals", flag.ContinueOnError)
	fs.SetOutput(a.Stderr)
	addCommonFlags(fs, &flags)
	fs.StringVar(&goalRun, "goal-run", "", "GoalRun JSON path")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if strings.TrimSpace(goalRun) == "" {
		fmt.Fprintln(a.Stderr, "ao-command goals: --goal-run is required")
		return 2
	}

	stdout, stderr, err := a.runForge(ctx, flags, "goal", "inspect", "--goal-run", goalRun, "--json")
	if err != nil {
		return a.commandError("goal inspect", stderr, err)
	}
	var inspect goalInspect
	if err := json.Unmarshal(stdout, &inspect); err != nil {
		fmt.Fprintf(a.Stderr, "ao-command goals: invalid AO Forge goal inspect JSON: %v\n", err)
		return 1
	}
	if flags.jsonOut {
		return a.writeJSON(goalSummaryFromInspect(flags.forgeDir, inspect))
	}
	fmt.Fprintf(a.Stdout, "goal_id=%s\n", inspect.GoalID)
	fmt.Fprintf(a.Stdout, "repo=%s\n", inspect.Repo)
	fmt.Fprintf(a.Stdout, "current_phase=%s\n", inspect.CurrentPhase)
	fmt.Fprintf(a.Stdout, "next_task=%s\n", inspect.NextTask)
	fmt.Fprintf(a.Stdout, "next_action_guard=%t\n", inspect.NextActionGuard.Enabled)
	fmt.Fprintf(a.Stdout, "last_iteration_status=%s\n", inspect.LastIterationStatus)
	return 0
}

func (a App) evidence(ctx context.Context, args []string) int {
	var flags commonFlags
	var schema, document string
	fs := flag.NewFlagSet("evidence", flag.ContinueOnError)
	fs.SetOutput(a.Stderr)
	addCommonFlags(fs, &flags)
	fs.StringVar(&schema, "schema", "", "schema path")
	fs.StringVar(&document, "document", "", "document path")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if strings.TrimSpace(schema) == "" || strings.TrimSpace(document) == "" {
		fmt.Fprintln(a.Stderr, "ao-command evidence: --schema and --document are required")
		return 2
	}

	forgeArgs := []string{"contract", "validate", "--schema", schema, "--document", document}
	if flags.jsonOut {
		forgeArgs = append(forgeArgs, "--json")
	}
	stdout, stderr, err := a.runForge(ctx, flags, forgeArgs...)
	if err != nil {
		return a.commandError("contract validate", stderr, err)
	}
	_, _ = a.Stdout.Write(stdout)
	return 0
}

func (a App) rehearse(ctx context.Context, args []string) int {
	var flags commonFlags
	var tag, outDir, notesPath string
	fs := flag.NewFlagSet("rehearse", flag.ContinueOnError)
	fs.SetOutput(a.Stderr)
	addCommonFlags(fs, &flags)
	fs.StringVar(&tag, "tag", "", "release tag to rehearse")
	fs.StringVar(&outDir, "out", "", "dry-run evidence output directory")
	fs.StringVar(&notesPath, "notes", "", "release notes path")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if strings.TrimSpace(tag) == "" || strings.TrimSpace(outDir) == "" {
		fmt.Fprintln(a.Stderr, "ao-command rehearse: --tag and --out are required")
		return 2
	}
	if notesPath == "" {
		notesPath = filepath.ToSlash(filepath.Join("docs", "release", "V"+strings.ToUpper(strings.TrimPrefix(tag, "v"))+"-RELEASE-NOTES.md"))
	}

	env := []string{
		"AO_FORGE_RELEASE_PREVIEW_TAG=" + tag,
		"AO_FORGE_RELEASE_PREVIEW_OUT=" + outDir,
		"AO_FORGE_RELEASE_NOTES_PATH=" + notesPath,
	}
	ctx, cancel := context.WithTimeout(ctx, flags.timeout)
	defer cancel()
	stdout, stderr, err := a.Runner.Run(ctx, Command{
		Dir:  flags.forgeDir,
		Env:  env,
		Name: filepath.ToSlash(filepath.Join("scripts", "release-preview-dry-run.sh")),
	})
	if err != nil {
		return a.commandError("release preview rehearsal", append(stderr, stdout...), err)
	}

	auditPath := filepath.Join(outDir, "release-preview-audit.json")
	inspect, inspectErr, err := a.runForge(ctx, flags, "release-preview", "inspect", "--audit", auditPath, "--json")
	if err != nil {
		return a.commandError("release preview inspect", inspectErr, err)
	}
	if flags.jsonOut {
		_, _ = a.Stdout.Write(inspect)
		return 0
	}
	fmt.Fprintf(a.Stdout, "ao_command_rehearse=passed\n")
	fmt.Fprintf(a.Stdout, "tag=%s\n", tag)
	fmt.Fprintf(a.Stdout, "out=%s\n", outDir)
	_, _ = a.Stdout.Write(stdout)
	return 0
}

func (a App) readReadiness(ctx context.Context, flags commonFlags) (productionReadinessAudit, int) {
	stdout, stderr, err := a.runForge(ctx, flags, "production-readiness", "audit", "--json")
	if err != nil {
		return productionReadinessAudit{}, a.commandError("production-readiness audit", stderr, err)
	}
	var audit productionReadinessAudit
	if err := json.Unmarshal(stdout, &audit); err != nil {
		fmt.Fprintf(a.Stderr, "ao-command status: invalid AO Forge production-readiness JSON: %v\n", err)
		return productionReadinessAudit{}, 1
	}
	return audit, 0
}

func (a App) runForge(ctx context.Context, flags commonFlags, args ...string) ([]byte, []byte, error) {
	ctx, cancel := context.WithTimeout(ctx, flags.timeout)
	defer cancel()
	if flags.forgeBin != "" {
		return a.Runner.Run(ctx, Command{Name: flags.forgeBin, Args: args})
	}
	forgeArgs := append([]string{"run", "./cmd/forge"}, args...)
	return a.Runner.Run(ctx, Command{Dir: flags.forgeDir, Name: "go", Args: forgeArgs})
}

func (a App) commandError(label string, stderr []byte, err error) int {
	stderrText := strings.TrimSpace(string(stderr))
	if stderrText != "" {
		fmt.Fprintf(a.Stderr, "ao-command %s failed: %s\n", label, stderrText)
	} else if errors.Is(err, context.DeadlineExceeded) {
		fmt.Fprintf(a.Stderr, "ao-command %s timed out\n", label)
	} else {
		fmt.Fprintf(a.Stderr, "ao-command %s failed: %v\n", label, err)
	}
	return 1
}

func (a App) writeJSON(v any) int {
	enc := json.NewEncoder(a.Stdout)
	enc.SetIndent("", "  ")
	if err := enc.Encode(v); err != nil {
		fmt.Fprintf(a.Stderr, "ao-command: write JSON: %v\n", err)
		return 1
	}
	return 0
}

type nextAction struct {
	ActionID    string `json:"action_id"`
	Description string `json:"description"`
	Required    bool   `json:"required"`
}

type productionReadinessAudit struct {
	Status           string       `json:"status"`
	ReadinessPercent int          `json:"readiness_percent"`
	PassedGates      int          `json:"passed_gates"`
	TotalGates       int          `json:"total_gates"`
	NextActions      []nextAction `json:"next_actions"`
}

type statusSummary struct {
	CommandSchemaVersion string       `json:"command_schema_version"`
	Forge                string       `json:"forge"`
	Status               string       `json:"status"`
	ReadinessPercent     int          `json:"readiness_percent"`
	PassedGates          int          `json:"passed_gates"`
	TotalGates           int          `json:"total_gates"`
	RequiredNextActions  int          `json:"required_next_actions"`
	ProductionReady      bool         `json:"production_ready"`
	OperatorMode         string       `json:"operator_mode"`
	ReleaseGovernance    string       `json:"release_governance"`
	NextActions          []nextAction `json:"next_actions"`
}

func statusSummaryFromAudit(forge string, audit productionReadinessAudit) statusSummary {
	return statusSummary{
		CommandSchemaVersion: commandSchemaVersion,
		Forge:                forge,
		Status:               audit.Status,
		ReadinessPercent:     audit.ReadinessPercent,
		PassedGates:          audit.PassedGates,
		TotalGates:           audit.TotalGates,
		RequiredNextActions:  requiredNextActionCount(audit.NextActions),
		ProductionReady:      productionReady(audit),
		OperatorMode:         operatorMode,
		ReleaseGovernance:    releaseGovernance,
		NextActions:          audit.NextActions,
	}
}

func requiredNextActionCount(actions []nextAction) int {
	count := 0
	for _, action := range actions {
		if action.Required {
			count++
		}
	}
	return count
}

func productionReady(audit productionReadinessAudit) bool {
	return audit.Status == "passed" &&
		audit.ReadinessPercent == 100 &&
		audit.TotalGates > 0 &&
		audit.PassedGates == audit.TotalGates &&
		requiredNextActionCount(audit.NextActions) == 0
}

type nextSummary struct {
	CommandSchemaVersion string     `json:"command_schema_version"`
	Forge                string     `json:"forge"`
	Status               string     `json:"status"`
	ReadinessPercent     int        `json:"readiness_percent"`
	NextAction           nextAction `json:"next_action"`
}

func nextSummaryFromAudit(forge string, audit productionReadinessAudit) nextSummary {
	action := nextAction{
		ActionID:    "build-ao-command-v0.1-read-only-surface",
		Description: "AO Forge is production-ready; continue AO Command v0.1 read-only operator surface work before ao-arena.",
		Required:    false,
	}
	if len(audit.NextActions) > 0 {
		action = audit.NextActions[0]
	}
	return nextSummary{
		CommandSchemaVersion: commandSchemaVersion,
		Forge:                forge,
		Status:               audit.Status,
		ReadinessPercent:     audit.ReadinessPercent,
		NextAction:           action,
	}
}

type goalInspect struct {
	GoalRun             string `json:"goal_run"`
	GoalID              string `json:"goal_id"`
	Repo                string `json:"repo"`
	CurrentPhase        string `json:"current_phase"`
	NextTask            string `json:"next_task"`
	LastVerifiedAt      string `json:"last_verified_at"`
	LastIterationStatus string `json:"last_iteration_status"`
	NextActionGuard     struct {
		Enabled    bool   `json:"enabled"`
		OnMismatch string `json:"on_mismatch"`
	} `json:"next_action_guard"`
}

type goalSummary struct {
	CommandSchemaVersion string      `json:"command_schema_version"`
	Forge                string      `json:"forge"`
	GoalRun              string      `json:"goal_run"`
	GoalID               string      `json:"goal_id"`
	Repo                 string      `json:"repo"`
	CurrentPhase         string      `json:"current_phase"`
	NextTask             string      `json:"next_task"`
	LastVerifiedAt       string      `json:"last_verified_at"`
	LastIterationStatus  string      `json:"last_iteration_status"`
	NextActionGuard      interface{} `json:"next_action_guard"`
}

func goalSummaryFromInspect(forge string, inspect goalInspect) goalSummary {
	return goalSummary{
		CommandSchemaVersion: commandSchemaVersion,
		Forge:                forge,
		GoalRun:              inspect.GoalRun,
		GoalID:               inspect.GoalID,
		Repo:                 inspect.Repo,
		CurrentPhase:         inspect.CurrentPhase,
		NextTask:             inspect.NextTask,
		LastVerifiedAt:       inspect.LastVerifiedAt,
		LastIterationStatus:  inspect.LastIterationStatus,
		NextActionGuard:      inspect.NextActionGuard,
	}
}
