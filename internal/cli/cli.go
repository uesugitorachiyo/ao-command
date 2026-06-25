package cli

import (
	"bytes"
	"context"
	"crypto/sha256"
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
	case "stack":
		return a.stack(args[1:])
	case "rsi":
		return a.rsi(args[1:])
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
	fmt.Fprintln(a.Stdout, `ao-command is the read-only operator command surface for the AO2-first AO stack.

Usage:
  ao-command status [--forge PATH] [--forge-bin PATH] [--json]
  ao-command stack --ledger PATH [--json]
  ao-command rsi health --arena-gate PATH --crucible-gate PATH --sentinel-verdict PATH --promoter-gate PATH --foundry-gate PATH --foundry-candidate PATH --foundry-next-task PATH --forge-retained-gate PATH --forge-retained-candidate PATH --forge-retained-next-task PATH --forge-retained-command-health PATH [--bundle-out PATH] [--json]
  ao-command next [--forge PATH] [--forge-bin PATH] [--json]
  ao-command goals --goal-run PATH [--forge PATH] [--forge-bin PATH] [--json]
  ao-command evidence --schema PATH --document PATH [--forge PATH] [--forge-bin PATH] [--json]
  ao-command rehearse --tag TAG --out DIR [--forge PATH] [--forge-bin PATH] [--json]

Commands are read-only by default. Rehearsal writes only dry-run evidence to the
operator-provided output directory and relies on AO Forge release-preview proofs.
AO Forge provides readiness truth, AO2 executes governed work, ao2-control-plane
stores evidence, and AO Covenant owns allow, deny, and block decisions.`)
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

func (a App) stack(args []string) int {
	var ledgerPath string
	var jsonOut bool
	fs := flag.NewFlagSet("stack", flag.ContinueOnError)
	fs.SetOutput(a.Stderr)
	fs.StringVar(&ledgerPath, "ledger", "", "path to AO Foundry active-stack readiness ledger")
	fs.BoolVar(&jsonOut, "json", false, "emit JSON")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if strings.TrimSpace(ledgerPath) == "" {
		fmt.Fprintln(a.Stderr, "ao-command stack: --ledger is required")
		return 2
	}
	ledger, err := readActiveStackLedger(ledgerPath)
	if err != nil {
		fmt.Fprintf(a.Stderr, "ao-command stack: %v\n", err)
		return 1
	}
	summary := stackSummaryFromLedger(ledgerPath, ledger)
	if jsonOut {
		return a.writeJSON(summary)
	}
	fmt.Fprintf(a.Stdout, "ao_command_stack=%s\n", summary.Status)
	fmt.Fprintf(a.Stdout, "ledger=%s\n", summary.Ledger)
	fmt.Fprintf(a.Stdout, "active_repositories=%d\n", len(summary.ActiveRepositories))
	fmt.Fprintf(a.Stdout, "release_handoff=%s\n", summary.ReleaseHandoff.Status)
	fmt.Fprintf(a.Stdout, "operator_mode=%s\n", summary.OperatorMode)
	fmt.Fprintf(a.Stdout, "orchestration_owner=%s\n", summary.OrchestrationOwner)
	for _, gate := range summary.ReleaseHandoff.Gates {
		fmt.Fprintf(a.Stdout, "gate=%s status=%s required_before_promotion=%t\n", gate.Name, gate.Status, gate.RequiredBeforePromotion)
	}
	fmt.Fprintf(a.Stdout, "out_of_scope=%s\n", strings.Join(summary.OutOfScope, ","))
	return 0
}

func (a App) rsi(args []string) int {
	if len(args) == 0 || args[0] != "health" {
		fmt.Fprintln(a.Stderr, "ao-command rsi: usage: ao-command rsi health --arena-gate PATH --crucible-gate PATH --sentinel-verdict PATH --promoter-gate PATH --foundry-gate PATH --foundry-candidate PATH --foundry-next-task PATH --forge-retained-gate PATH --forge-retained-candidate PATH --forge-retained-next-task PATH --forge-retained-command-health PATH [--bundle-out PATH] [--json]")
		return 2
	}
	var arenaGate, crucibleGate, sentinelVerdict, promoterGate, foundryGate, foundryCandidate, foundryNextTask, forgeRetainedGate, forgeRetainedCandidate, forgeRetainedNextTask, forgeRetainedCommandHealth, bundleOut string
	var jsonOut bool
	fs := flag.NewFlagSet("rsi health", flag.ContinueOnError)
	fs.SetOutput(a.Stderr)
	fs.StringVar(&arenaGate, "arena-gate", "", "path to AO Arena promotion gate JSON")
	fs.StringVar(&crucibleGate, "crucible-gate", "", "path to AO Crucible hardening gate JSON")
	fs.StringVar(&sentinelVerdict, "sentinel-verdict", "", "path to AO Sentinel verdict JSON")
	fs.StringVar(&promoterGate, "promoter-gate", "", "path to AO Promoter gate JSON")
	fs.StringVar(&foundryGate, "foundry-gate", "", "path to AO Foundry RSI improvement gate JSON")
	fs.StringVar(&foundryCandidate, "foundry-candidate", "", "path to AO Foundry RSI candidate JSON")
	fs.StringVar(&foundryNextTask, "foundry-next-task", "", "path to AO Foundry RSI next improvement task JSON")
	fs.StringVar(&forgeRetainedGate, "forge-retained-gate", "", "path to AO Forge retained Foundry RSI improvement gate proof JSON")
	fs.StringVar(&forgeRetainedCandidate, "forge-retained-candidate", "", "path to AO Forge retained Foundry RSI candidate proof JSON")
	fs.StringVar(&forgeRetainedNextTask, "forge-retained-next-task", "", "path to AO Forge retained Foundry RSI next task proof JSON")
	fs.StringVar(&forgeRetainedCommandHealth, "forge-retained-command-health", "", "path to AO Forge retained AO Command RSI health proof JSON")
	fs.StringVar(&bundleOut, "bundle-out", "", "write canonical RSI health bundle JSON to path")
	fs.BoolVar(&jsonOut, "json", false, "emit JSON")
	if err := fs.Parse(args[1:]); err != nil {
		return 2
	}
	if arenaGate == "" || crucibleGate == "" || sentinelVerdict == "" || promoterGate == "" || foundryGate == "" || foundryCandidate == "" || foundryNextTask == "" || forgeRetainedGate == "" || forgeRetainedCandidate == "" || forgeRetainedNextTask == "" || forgeRetainedCommandHealth == "" {
		fmt.Fprintln(a.Stderr, "ao-command rsi health: all evidence flags are required")
		return 2
	}
	summary, err := readRSIHealth(arenaGate, crucibleGate, sentinelVerdict, promoterGate, foundryGate, foundryCandidate, foundryNextTask, forgeRetainedGate, forgeRetainedCandidate, forgeRetainedNextTask, forgeRetainedCommandHealth)
	if err != nil {
		fmt.Fprintf(a.Stderr, "ao-command rsi health: %v\n", err)
		return 1
	}
	if strings.TrimSpace(bundleOut) != "" {
		if err := writeRSIHealthBundle(bundleOut, summary); err != nil {
			fmt.Fprintf(a.Stderr, "ao-command rsi health: write bundle: %v\n", err)
			return 1
		}
	}
	if jsonOut {
		if code := a.writeJSON(summary); code != 0 {
			return code
		}
		if summary.Status != "passed" {
			fmt.Fprintln(a.Stderr, "ao-command rsi health: RSI health blocked")
			return 1
		}
		return 0
	}
	fmt.Fprintf(a.Stdout, "ao_command_rsi_health=%s\n", summary.Status)
	fmt.Fprintf(a.Stdout, "rsi_mode=%s\n", summary.RSIMode)
	fmt.Fprintf(a.Stdout, "operator_mode=%s\n", summary.OperatorMode)
	fmt.Fprintf(a.Stdout, "mutates_repositories=%t\n", summary.MutatesRepositories)
	for _, family := range summary.Families {
		fmt.Fprintf(a.Stdout, "family=%s status=%s passed=%t evidence=%s\n", family.Family, family.Status, family.Passed, family.Evidence)
	}
	if summary.FoundryCandidateBinding != nil {
		fmt.Fprintf(a.Stdout, "foundry_candidate_bound=%t matched_eval_result_path=%s candidate_evidence=%s gate_evidence=%s\n",
			summary.FoundryCandidateBinding.Passed,
			summary.FoundryCandidateBinding.MatchedEvalResultPath,
			summary.FoundryCandidateBinding.CandidateEvidence,
			summary.FoundryCandidateBinding.GateEvidence)
	}
	if summary.FoundryNextTaskBinding != nil {
		fmt.Fprintf(a.Stdout, "foundry_next_task_bound=%t next_task_evidence=%s candidate_evidence=%s gate_evidence=%s\n",
			summary.FoundryNextTaskBinding.Passed,
			summary.FoundryNextTaskBinding.NextTaskEvidence,
			summary.FoundryNextTaskBinding.CandidateEvidence,
			summary.FoundryNextTaskBinding.GateEvidence)
	}
	if summary.ForgeRetentionBinding != nil {
		fmt.Fprintf(a.Stdout, "forge_retention_bound=%t goal_id=%s iteration=%s retained_evidence=%d\n",
			summary.ForgeRetentionBinding.Passed,
			summary.ForgeRetentionBinding.GoalID,
			summary.ForgeRetentionBinding.Iteration,
			len(summary.ForgeRetentionBinding.RetainedEvidence))
	}
	if strings.TrimSpace(bundleOut) != "" {
		fmt.Fprintf(a.Stdout, "bundle=%s\n", bundleOut)
	}
	fmt.Fprintf(a.Stdout, "rsi_capability=%s\n", summary.RSICapability)
	if summary.Status != "passed" {
		fmt.Fprintln(a.Stderr, "ao-command rsi health: RSI health blocked")
		return 1
	}
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

type activeStackLedger struct {
	SchemaVersion  string                  `json:"schema_version"`
	RegistryID     string                  `json:"registry_id"`
	Status         string                  `json:"status"`
	Repositories   []activeStackRepository `json:"repositories"`
	ReleaseHandoff releaseHandoff          `json:"release_handoff"`
}

type activeStackRepository struct {
	ID     string `json:"id"`
	Name   string `json:"name"`
	Role   string `json:"role"`
	Status string `json:"status"`
}

type releaseHandoff struct {
	Status string               `json:"status"`
	Gates  []releaseHandoffGate `json:"gates"`
}

type releaseHandoffGate struct {
	Name                    string `json:"name"`
	Status                  string `json:"status"`
	RequiredBeforePromotion bool   `json:"required_before_promotion"`
}

type stackSummary struct {
	CommandSchemaVersion string                  `json:"command_schema_version"`
	Ledger               string                  `json:"ledger"`
	Status               string                  `json:"status"`
	OperatorMode         string                  `json:"operator_mode"`
	OrchestrationOwner   string                  `json:"orchestration_owner"`
	ActiveRepositories   []activeStackRepository `json:"active_repositories"`
	ReleaseHandoff       releaseHandoff          `json:"release_handoff"`
	OutOfScope           []string                `json:"out_of_scope"`
}

type rsiFamilyStatus struct {
	Family   string `json:"family"`
	Status   string `json:"status"`
	Passed   bool   `json:"passed"`
	Evidence string `json:"evidence"`
}

type rsiHealthSummary struct {
	CommandSchemaVersion    string                         `json:"command_schema_version"`
	Status                  string                         `json:"status"`
	RSIMode                 string                         `json:"rsi_mode"`
	RSICapability           string                         `json:"rsi_capability"`
	OperatorMode            string                         `json:"operator_mode"`
	MutatesRepositories     bool                           `json:"mutates_repositories"`
	Families                []rsiFamilyStatus              `json:"families"`
	FoundryCandidateBinding *foundryCandidateBindingStatus `json:"foundry_candidate_binding,omitempty"`
	FoundryNextTaskBinding  *foundryNextTaskBindingStatus  `json:"foundry_next_task_binding,omitempty"`
	ForgeRetentionBinding   *forgeRetentionBindingStatus   `json:"forge_retention_binding,omitempty"`
}

type rsiHealthBundle struct {
	SchemaVersion           string                         `json:"schema_version"`
	CommandSchemaVersion    string                         `json:"command_schema_version"`
	Status                  string                         `json:"status"`
	RSIMode                 string                         `json:"rsi_mode"`
	RSICapability           string                         `json:"rsi_capability"`
	OperatorMode            string                         `json:"operator_mode"`
	MutatesRepositories     bool                           `json:"mutates_repositories"`
	Families                []rsiBundleFamilyStatus        `json:"families"`
	FoundryCandidateBinding *foundryCandidateBindingStatus `json:"foundry_candidate_binding,omitempty"`
	FoundryNextTaskBinding  *foundryNextTaskBindingStatus  `json:"foundry_next_task_binding,omitempty"`
	ForgeRetentionBinding   *forgeRetentionBindingStatus   `json:"forge_retention_binding,omitempty"`
}

type rsiBundleFamilyStatus struct {
	Family   string `json:"family"`
	Status   string `json:"status"`
	Passed   bool   `json:"passed"`
	Evidence string `json:"evidence"`
	SHA256   string `json:"sha256"`
}

type foundryCandidateBindingStatus struct {
	Status                string `json:"status"`
	Passed                bool   `json:"passed"`
	CandidateEvidence     string `json:"candidate_evidence"`
	GateEvidence          string `json:"gate_evidence"`
	MatchedEvalResultPath string `json:"matched_eval_result_path"`
	MutatesRepositories   bool   `json:"mutates_repositories"`
}

type foundryNextTaskBindingStatus struct {
	Status                     string  `json:"status"`
	Passed                     bool    `json:"passed"`
	NextTaskEvidence           string  `json:"next_task_evidence"`
	CandidateEvidence          string  `json:"candidate_evidence"`
	GateEvidence               string  `json:"gate_evidence"`
	RequiredImprovementPercent float64 `json:"required_improvement_percent"`
	ActualImprovementPercent   float64 `json:"actual_improvement_percent"`
	AutonomousClaim            string  `json:"autonomous_claim"`
	MutatesRepositories        bool    `json:"mutates_repositories"`
}

type forgeRetentionBindingStatus struct {
	Status              string   `json:"status"`
	Passed              bool     `json:"passed"`
	GoalID              string   `json:"goal_id"`
	Iteration           string   `json:"iteration"`
	Phase               string   `json:"phase"`
	RetainedEvidence    []string `json:"retained_evidence"`
	RetainedOutputCount int      `json:"retained_output_count"`
	MutatesRepositories bool     `json:"mutates_repositories"`
}

type foundryEvalResultRef struct {
	Label         string  `json:"label,omitempty"`
	Path          string  `json:"path"`
	SchemaVersion string  `json:"schema_version"`
	Status        string  `json:"status"`
	Score         float64 `json:"score"`
	MaxScore      float64 `json:"max_score"`
	SHA256        string  `json:"sha256"`
}

type foundryRSIImprovementGate struct {
	SchemaVersion              string                 `json:"schema_version"`
	Status                     string                 `json:"status"`
	BaselineScore              float64                `json:"baseline_score"`
	CandidateScore             float64                `json:"candidate_score"`
	RequiredImprovementPercent float64                `json:"required_improvement_percent"`
	ActualImprovementPercent   float64                `json:"actual_improvement_percent"`
	AutonomousClaim            string                 `json:"autonomous_claim"`
	MutatesRepositories        bool                   `json:"mutates_repositories"`
	Evidence                   []foundryEvalResultRef `json:"evidence"`
}

type foundryRSICandidate struct {
	SchemaVersion       string               `json:"schema_version"`
	Status              string               `json:"status"`
	GeneratedBy         string               `json:"generated_by"`
	BaselineEvalResult  foundryEvalResultRef `json:"baseline_eval_result"`
	CandidateEvalResult foundryEvalResultRef `json:"candidate_eval_result"`
	MutatesRepositories bool                 `json:"mutates_repositories"`
}

type foundryRSINextImprovementTask struct {
	SchemaVersion              string   `json:"schema_version"`
	Status                     string   `json:"status"`
	GeneratedBy                string   `json:"generated_by"`
	CandidateEvidencePath      string   `json:"candidate_evidence_path"`
	GateEvidencePath           string   `json:"gate_evidence_path"`
	RequiredImprovementPercent float64  `json:"required_improvement_percent"`
	ActualImprovementPercent   float64  `json:"actual_improvement_percent"`
	AutonomousClaim            string   `json:"autonomous_claim"`
	MutatesRepositories        bool     `json:"mutates_repositories"`
	NextActions                []string `json:"next_actions"`
}

type forgeRetainedEvidence struct {
	SchemaVersion     string                 `json:"schema_version"`
	GoalID            string                 `json:"goal_id"`
	Iteration         string                 `json:"iteration"`
	Phase             string                 `json:"phase"`
	CapturedOutputs   []forgeRetainedOutput  `json:"captured_outputs"`
	RetentionPolicy   forgeRetentionPolicy   `json:"retention_policy"`
	RetentionMetadata forgeRetentionMetadata `json:"retention_metadata"`
}

type forgeRetainedOutput struct {
	Label                      string              `json:"label"`
	Command                    string              `json:"command"`
	SchemaVersion              string              `json:"schema_version,omitempty"`
	Status                     string              `json:"status"`
	GeneratedBy                string              `json:"generated_by,omitempty"`
	BaselineScore              float64             `json:"baseline_score,omitempty"`
	CandidateScore             float64             `json:"candidate_score,omitempty"`
	RequiredImprovementPercent float64             `json:"required_improvement_percent,omitempty"`
	ActualImprovementPercent   float64             `json:"actual_improvement_percent,omitempty"`
	AutonomousClaim            string              `json:"autonomous_claim,omitempty"`
	RSIMode                    string              `json:"rsi_mode,omitempty"`
	RSICapability              string              `json:"rsi_capability,omitempty"`
	OperatorMode               string              `json:"operator_mode,omitempty"`
	MutatesRepositories        bool                `json:"mutates_repositories"`
	Families                   []retainedRSIFamily `json:"families,omitempty"`
}

type retainedRSIFamily struct {
	Family string `json:"family"`
	Status string `json:"status"`
	Passed bool   `json:"passed"`
}

type forgeRetentionPolicy struct {
	TemporaryPathsAllowed                  bool `json:"temporary_paths_allowed"`
	MinimumRetentionDaysAfterTerminalPhase int  `json:"minimum_retention_days_after_terminal_phase"`
}

type forgeRetentionMetadata struct {
	RetentionClass         string `json:"retention_class"`
	RetainWhileGoalActive  bool   `json:"retain_while_goal_active"`
	DeletionRequiresReview bool   `json:"deletion_requires_review"`
}

func readActiveStackLedger(path string) (activeStackLedger, error) {
	var ledger activeStackLedger
	bytes, err := os.ReadFile(path)
	if err != nil {
		return ledger, fmt.Errorf("read ledger: %w", err)
	}
	if err := json.Unmarshal(bytes, &ledger); err != nil {
		return ledger, fmt.Errorf("invalid ledger JSON: %w", err)
	}
	if ledger.SchemaVersion != "ao.foundry.active-stack-readiness.v0.1" {
		return ledger, errors.New("invalid active-stack readiness schema_version")
	}
	if ledger.Status == "" || len(ledger.Repositories) == 0 {
		return ledger, errors.New("active-stack ledger requires status and repositories")
	}
	if ledger.ReleaseHandoff.Status == "" || len(ledger.ReleaseHandoff.Gates) == 0 {
		return ledger, errors.New("active-stack ledger requires release_handoff gates")
	}
	return ledger, nil
}

func stackSummaryFromLedger(path string, ledger activeStackLedger) stackSummary {
	return stackSummary{
		CommandSchemaVersion: commandSchemaVersion,
		Ledger:               path,
		Status:               ledger.Status,
		OperatorMode:         operatorMode,
		OrchestrationOwner:   "ao-foundry",
		ActiveRepositories:   ledger.Repositories,
		ReleaseHandoff:       ledger.ReleaseHandoff,
		OutOfScope: []string{
			"ao-operator",
			"ao-runtime",
			"ao-control-plane",
			"ao-conductor",
			"agy-swarms",
			"codex-cron",
		},
	}
}

func readRSIHealth(arenaGatePath, crucibleGatePath, sentinelVerdictPath, promoterGatePath, foundryGatePath, foundryCandidatePath, foundryNextTaskPath, forgeRetainedGatePath, forgeRetainedCandidatePath, forgeRetainedNextTaskPath, forgeRetainedCommandHealthPath string) (rsiHealthSummary, error) {
	arena, err := readArenaGate(arenaGatePath)
	if err != nil {
		return rsiHealthSummary{}, err
	}
	crucible, err := readCrucibleGate(crucibleGatePath)
	if err != nil {
		return rsiHealthSummary{}, err
	}
	sentinel, err := readSentinelVerdict(sentinelVerdictPath)
	if err != nil {
		return rsiHealthSummary{}, err
	}
	promoter, err := readPromoterGate(promoterGatePath)
	if err != nil {
		return rsiHealthSummary{}, err
	}
	foundry, foundryGate, err := readFoundryRSIImprovementGate(foundryGatePath)
	if err != nil {
		return rsiHealthSummary{}, err
	}
	binding, err := readFoundryRSICandidateBinding(foundryCandidatePath, foundryGatePath, foundryGate)
	if err != nil {
		return rsiHealthSummary{}, err
	}
	foundryCandidateBinding := &binding
	nextTaskBinding, err := readFoundryRSINextTaskBinding(foundryNextTaskPath, foundryCandidatePath, foundryGatePath, foundryGate)
	if err != nil {
		return rsiHealthSummary{}, err
	}
	foundryNextTaskBinding := &nextTaskBinding
	families := []rsiFamilyStatus{arena, crucible, sentinel, promoter, foundry}
	retentionBinding, err := readForgeRSIRetentionBinding(forgeRetainedGatePath, forgeRetainedCandidatePath, forgeRetainedNextTaskPath, forgeRetainedCommandHealthPath, foundryGate, foundryCandidatePath, foundryNextTaskPath, families)
	if err != nil {
		return rsiHealthSummary{}, err
	}
	forgeRetentionBinding := &retentionBinding
	status := "passed"
	capability := "demonstrated_local_fixture_loop"
	for _, family := range families {
		if !family.Passed {
			status = "blocked"
			capability = "not_demonstrated"
			break
		}
	}
	if foundryCandidateBinding != nil && !foundryCandidateBinding.Passed {
		status = "blocked"
		capability = "not_demonstrated"
	}
	if foundryNextTaskBinding != nil && !foundryNextTaskBinding.Passed {
		status = "blocked"
		capability = "not_demonstrated"
	}
	if forgeRetentionBinding != nil && !forgeRetentionBinding.Passed {
		status = "blocked"
		capability = "not_demonstrated"
	}
	return rsiHealthSummary{
		CommandSchemaVersion:    commandSchemaVersion,
		Status:                  status,
		RSIMode:                 "governed_fixture_local",
		RSICapability:           capability,
		OperatorMode:            operatorMode,
		MutatesRepositories:     false,
		Families:                families,
		FoundryCandidateBinding: foundryCandidateBinding,
		FoundryNextTaskBinding:  foundryNextTaskBinding,
		ForgeRetentionBinding:   forgeRetentionBinding,
	}, nil
}

func writeRSIHealthBundle(path string, summary rsiHealthSummary) error {
	bundle, err := rsiHealthBundleFromSummary(summary)
	if err != nil {
		return err
	}
	bytes, err := json.MarshalIndent(bundle, "", "  ")
	if err != nil {
		return err
	}
	bytes = append(bytes, '\n')
	if dir := filepath.Dir(path); dir != "." {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return err
		}
	}
	return os.WriteFile(path, bytes, 0o644)
}

func rsiHealthBundleFromSummary(summary rsiHealthSummary) (rsiHealthBundle, error) {
	bundle := rsiHealthBundle{
		SchemaVersion:           "ao.command.rsi-health-bundle.v0.1",
		CommandSchemaVersion:    summary.CommandSchemaVersion,
		Status:                  summary.Status,
		RSIMode:                 summary.RSIMode,
		RSICapability:           summary.RSICapability,
		OperatorMode:            summary.OperatorMode,
		MutatesRepositories:     summary.MutatesRepositories,
		Families:                make([]rsiBundleFamilyStatus, 0, len(summary.Families)),
		FoundryCandidateBinding: summary.FoundryCandidateBinding,
		FoundryNextTaskBinding:  summary.FoundryNextTaskBinding,
		ForgeRetentionBinding:   summary.ForgeRetentionBinding,
	}
	for _, family := range summary.Families {
		hash, err := sha256File(family.Evidence)
		if err != nil {
			return rsiHealthBundle{}, fmt.Errorf("hash %s evidence: %w", family.Family, err)
		}
		bundle.Families = append(bundle.Families, rsiBundleFamilyStatus{
			Family:   family.Family,
			Status:   family.Status,
			Passed:   family.Passed,
			Evidence: family.Evidence,
			SHA256:   hash,
		})
	}
	return bundle, nil
}

func sha256File(path string) (string, error) {
	bytes, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("%x", sha256.Sum256(bytes)), nil
}

func readArenaGate(path string) (rsiFamilyStatus, error) {
	var gate struct {
		SchemaVersion string `json:"schema_version"`
		Status        string `json:"status"`
		Winner        string `json:"winner"`
	}
	if err := readJSONFile(path, &gate); err != nil {
		return rsiFamilyStatus{}, fmt.Errorf("read arena gate: %w", err)
	}
	passed := gate.SchemaVersion == "ao.arena.promotion-gate.v0.1" && gate.Status == "passed" && gate.Winner != ""
	return rsiFamilyStatus{Family: "ao-arena", Status: gate.Status, Passed: passed, Evidence: path}, nil
}

func readCrucibleGate(path string) (rsiFamilyStatus, error) {
	var gate struct {
		SchemaVersion string `json:"schema_version"`
		Status        string `json:"status"`
		Score         int    `json:"score"`
	}
	if err := readJSONFile(path, &gate); err != nil {
		return rsiFamilyStatus{}, fmt.Errorf("read crucible gate: %w", err)
	}
	passed := gate.SchemaVersion == "ao.crucible.hardening-gate.v0.1" && gate.Status == "passed" && gate.Score >= 85
	return rsiFamilyStatus{Family: "ao-crucible", Status: gate.Status, Passed: passed, Evidence: path}, nil
}

func readSentinelVerdict(path string) (rsiFamilyStatus, error) {
	var verdict struct {
		SchemaVersion        string `json:"schema_version"`
		Verdict              string `json:"verdict"`
		SafetyStatus         string `json:"safety_status"`
		RegressionStatus     string `json:"regression_status"`
		PromoterHoldRequired bool   `json:"promoter_hold_required"`
		MutatesLiveState     bool   `json:"mutates_live_state"`
	}
	if err := readJSONFile(path, &verdict); err != nil {
		return rsiFamilyStatus{}, fmt.Errorf("read sentinel verdict: %w", err)
	}
	passed := verdict.SchemaVersion == "ao.sentinel.verdict.v0.1" &&
		verdict.Verdict == "clear" &&
		verdict.SafetyStatus == "passed" &&
		verdict.RegressionStatus == "passed" &&
		!verdict.PromoterHoldRequired &&
		!verdict.MutatesLiveState
	return rsiFamilyStatus{Family: "ao-sentinel", Status: verdict.Verdict, Passed: passed, Evidence: path}, nil
}

func readPromoterGate(path string) (rsiFamilyStatus, error) {
	var gate struct {
		SchemaVersion         string   `json:"schema_version"`
		Status                string   `json:"status"`
		PromotionAllowed      bool     `json:"promotion_allowed"`
		ActivationPlanAllowed bool     `json:"activation_plan_allowed"`
		Blockers              []string `json:"blockers"`
	}
	if err := readJSONFile(path, &gate); err != nil {
		return rsiFamilyStatus{}, fmt.Errorf("read promoter gate: %w", err)
	}
	passed := gate.SchemaVersion == "ao.promoter.gate.v0.1" &&
		gate.Status == "passed" &&
		gate.PromotionAllowed &&
		gate.ActivationPlanAllowed &&
		len(gate.Blockers) == 0
	return rsiFamilyStatus{Family: "ao-promoter", Status: gate.Status, Passed: passed, Evidence: path}, nil
}

func readFoundryRSIImprovementGate(path string) (rsiFamilyStatus, foundryRSIImprovementGate, error) {
	var gate foundryRSIImprovementGate
	if err := readJSONFile(path, &gate); err != nil {
		return rsiFamilyStatus{}, foundryRSIImprovementGate{}, fmt.Errorf("read foundry RSI improvement gate: %w", err)
	}
	passed := gate.SchemaVersion == "ao.foundry.rsi-improvement-gate.v0.1" &&
		gate.Status == "passed" &&
		gate.CandidateScore >= gate.BaselineScore &&
		gate.ActualImprovementPercent >= gate.RequiredImprovementPercent &&
		gate.RequiredImprovementPercent > 0 &&
		gate.AutonomousClaim == "measured_local_improvement" &&
		!gate.MutatesRepositories
	return rsiFamilyStatus{Family: "ao-foundry", Status: gate.Status, Passed: passed, Evidence: path}, gate, nil
}

func readFoundryRSICandidateBinding(candidatePath, gatePath string, gate foundryRSIImprovementGate) (foundryCandidateBindingStatus, error) {
	var candidate foundryRSICandidate
	if err := readJSONFile(candidatePath, &candidate); err != nil {
		return foundryCandidateBindingStatus{}, fmt.Errorf("read foundry RSI candidate: %w", err)
	}
	gateCandidate, ok := foundryGateCandidateEvidence(gate)
	passed := ok &&
		candidate.SchemaVersion == "ao.foundry.rsi-candidate.v0.1" &&
		candidate.Status == "ready" &&
		candidate.GeneratedBy == "foundry pulse run" &&
		candidate.CandidateEvalResult.SchemaVersion == "ao.foundry.eval-result.v0.1" &&
		candidate.CandidateEvalResult.Status == "ready" &&
		candidate.CandidateEvalResult.Path != "" &&
		candidate.CandidateEvalResult.MaxScore > 0 &&
		candidate.CandidateEvalResult.SHA256 != "" &&
		candidate.CandidateEvalResult.Path == gateCandidate.Path &&
		candidate.CandidateEvalResult.SchemaVersion == gateCandidate.SchemaVersion &&
		candidate.CandidateEvalResult.Status == gateCandidate.Status &&
		candidate.CandidateEvalResult.Score == gateCandidate.Score &&
		candidate.CandidateEvalResult.MaxScore == gateCandidate.MaxScore &&
		candidate.CandidateEvalResult.SHA256 == gateCandidate.SHA256 &&
		!candidate.MutatesRepositories &&
		!gate.MutatesRepositories
	status := "passed"
	if !passed {
		status = "blocked"
	}
	return foundryCandidateBindingStatus{
		Status:                status,
		Passed:                passed,
		CandidateEvidence:     candidatePath,
		GateEvidence:          gatePath,
		MatchedEvalResultPath: candidate.CandidateEvalResult.Path,
		MutatesRepositories:   candidate.MutatesRepositories || gate.MutatesRepositories,
	}, nil
}

func readFoundryRSINextTaskBinding(nextTaskPath, candidatePath, gatePath string, gate foundryRSIImprovementGate) (foundryNextTaskBindingStatus, error) {
	var nextTask foundryRSINextImprovementTask
	if err := readJSONFile(nextTaskPath, &nextTask); err != nil {
		return foundryNextTaskBindingStatus{}, fmt.Errorf("read foundry RSI next task: %w", err)
	}
	passed := nextTask.SchemaVersion == "ao.foundry.rsi-next-improvement-task.v0.1" &&
		nextTask.Status == "ready" &&
		nextTask.GeneratedBy == "foundry pulse run" &&
		nextTask.CandidateEvidencePath == candidatePath &&
		nextTask.GateEvidencePath == gatePath &&
		nextTask.RequiredImprovementPercent == gate.RequiredImprovementPercent &&
		nextTask.ActualImprovementPercent == gate.ActualImprovementPercent &&
		nextTask.RequiredImprovementPercent > 0 &&
		nextTask.ActualImprovementPercent >= nextTask.RequiredImprovementPercent &&
		nextTask.AutonomousClaim == "derived_local_next_improvement" &&
		!nextTask.MutatesRepositories &&
		!gate.MutatesRepositories &&
		len(nextTask.NextActions) > 0
	status := "passed"
	if !passed {
		status = "blocked"
	}
	return foundryNextTaskBindingStatus{
		Status:                     status,
		Passed:                     passed,
		NextTaskEvidence:           nextTaskPath,
		CandidateEvidence:          candidatePath,
		GateEvidence:               gatePath,
		RequiredImprovementPercent: nextTask.RequiredImprovementPercent,
		ActualImprovementPercent:   nextTask.ActualImprovementPercent,
		AutonomousClaim:            nextTask.AutonomousClaim,
		MutatesRepositories:        nextTask.MutatesRepositories || gate.MutatesRepositories,
	}, nil
}

func readForgeRSIRetentionBinding(gateProofPath, candidateProofPath, nextTaskProofPath, commandHealthProofPath string, gate foundryRSIImprovementGate, candidatePath, nextTaskPath string, families []rsiFamilyStatus) (forgeRetentionBindingStatus, error) {
	var candidate foundryRSICandidate
	if err := readJSONFile(candidatePath, &candidate); err != nil {
		return forgeRetentionBindingStatus{}, fmt.Errorf("read foundry RSI candidate for retention binding: %w", err)
	}
	var nextTask foundryRSINextImprovementTask
	if err := readJSONFile(nextTaskPath, &nextTask); err != nil {
		return forgeRetentionBindingStatus{}, fmt.Errorf("read foundry RSI next task for retention binding: %w", err)
	}
	gateProof, err := readForgeRetainedEvidence(gateProofPath)
	if err != nil {
		return forgeRetentionBindingStatus{}, fmt.Errorf("read forge retained gate proof: %w", err)
	}
	candidateProof, err := readForgeRetainedEvidence(candidateProofPath)
	if err != nil {
		return forgeRetentionBindingStatus{}, fmt.Errorf("read forge retained candidate proof: %w", err)
	}
	nextTaskProof, err := readForgeRetainedEvidence(nextTaskProofPath)
	if err != nil {
		return forgeRetentionBindingStatus{}, fmt.Errorf("read forge retained next task proof: %w", err)
	}
	commandHealthProof, err := readForgeRetainedEvidence(commandHealthProofPath)
	if err != nil {
		return forgeRetentionBindingStatus{}, fmt.Errorf("read forge retained command health proof: %w", err)
	}

	goalID := gateProof.GoalID
	iteration := gateProof.Iteration
	phase := gateProof.Phase
	proofs := []forgeRetainedEvidence{gateProof, candidateProof, nextTaskProof, commandHealthProof}
	passed := goalID != "" && iteration != "" && phase == "verification"
	outputCount := 0
	mutatesRepositories := false
	for _, proof := range proofs {
		outputCount += len(proof.CapturedOutputs)
		passed = passed && forgeRetentionBasePassed(proof, goalID, iteration, phase)
		for _, output := range proof.CapturedOutputs {
			mutatesRepositories = mutatesRepositories || output.MutatesRepositories
		}
	}

	gateOutput, gateOutputOK := retainedOutput(gateProof, "ao-foundry-rsi-improvement-gate")
	candidateOutput, candidateOutputOK := retainedOutput(candidateProof, "ao-foundry-rsi-candidate")
	nextTaskOutput, nextTaskOutputOK := retainedOutput(nextTaskProof, "ao-foundry-rsi-next-improvement-task")
	commandHealthOutput, commandHealthOutputOK := retainedOutput(commandHealthProof, "ao-command-rsi-health")

	passed = passed &&
		gateOutputOK &&
		candidateOutputOK &&
		nextTaskOutputOK &&
		commandHealthOutputOK &&
		retainedFoundryGatePassed(gateOutput, gate) &&
		retainedFoundryCandidatePassed(candidateOutput, candidate) &&
		retainedFoundryNextTaskPassed(nextTaskOutput, nextTask) &&
		retainedCommandHealthPassed(commandHealthOutput, families) &&
		!mutatesRepositories

	status := "passed"
	if !passed {
		status = "blocked"
	}
	return forgeRetentionBindingStatus{
		Status:              status,
		Passed:              passed,
		GoalID:              goalID,
		Iteration:           iteration,
		Phase:               phase,
		RetainedEvidence:    []string{gateProofPath, candidateProofPath, nextTaskProofPath, commandHealthProofPath},
		RetainedOutputCount: outputCount,
		MutatesRepositories: mutatesRepositories,
	}, nil
}

func readForgeRetainedEvidence(path string) (forgeRetainedEvidence, error) {
	var retained forgeRetainedEvidence
	if err := readJSONFile(path, &retained); err != nil {
		return forgeRetainedEvidence{}, err
	}
	return retained, nil
}

func forgeRetentionBasePassed(proof forgeRetainedEvidence, goalID, iteration, phase string) bool {
	return proof.SchemaVersion == "ao.forge.goal-run-retained-evidence.v0.1" &&
		proof.GoalID == goalID &&
		proof.Iteration == iteration &&
		proof.Phase == phase &&
		len(proof.CapturedOutputs) == 1 &&
		!proof.RetentionPolicy.TemporaryPathsAllowed &&
		proof.RetentionPolicy.MinimumRetentionDaysAfterTerminalPhase >= 90 &&
		proof.RetentionMetadata.RetentionClass == "loop_evidence" &&
		proof.RetentionMetadata.RetainWhileGoalActive &&
		proof.RetentionMetadata.DeletionRequiresReview
}

func retainedOutput(proof forgeRetainedEvidence, label string) (forgeRetainedOutput, bool) {
	for _, output := range proof.CapturedOutputs {
		if output.Label == label {
			return output, true
		}
	}
	return forgeRetainedOutput{}, false
}

func retainedFoundryGatePassed(output forgeRetainedOutput, gate foundryRSIImprovementGate) bool {
	return output.Command == "foundry pulse run" &&
		output.SchemaVersion == "ao.foundry.rsi-improvement-gate.v0.1" &&
		output.Status == gate.Status &&
		output.BaselineScore == gate.BaselineScore &&
		output.CandidateScore == gate.CandidateScore &&
		output.RequiredImprovementPercent == gate.RequiredImprovementPercent &&
		output.ActualImprovementPercent == gate.ActualImprovementPercent &&
		output.AutonomousClaim == gate.AutonomousClaim &&
		!output.MutatesRepositories &&
		!gate.MutatesRepositories
}

func retainedFoundryCandidatePassed(output forgeRetainedOutput, candidate foundryRSICandidate) bool {
	return output.Command == "foundry pulse run" &&
		output.SchemaVersion == "ao.foundry.rsi-candidate.v0.1" &&
		output.Status == candidate.Status &&
		output.GeneratedBy == candidate.GeneratedBy &&
		output.BaselineScore == candidate.BaselineEvalResult.Score &&
		output.CandidateScore == candidate.CandidateEvalResult.Score &&
		!output.MutatesRepositories &&
		!candidate.MutatesRepositories
}

func retainedFoundryNextTaskPassed(output forgeRetainedOutput, nextTask foundryRSINextImprovementTask) bool {
	return output.Command == "foundry pulse run" &&
		output.SchemaVersion == "ao.foundry.rsi-next-improvement-task.v0.1" &&
		output.Status == nextTask.Status &&
		output.RequiredImprovementPercent == nextTask.RequiredImprovementPercent &&
		output.ActualImprovementPercent == nextTask.ActualImprovementPercent &&
		output.AutonomousClaim == nextTask.AutonomousClaim &&
		!output.MutatesRepositories &&
		!nextTask.MutatesRepositories
}

func retainedCommandHealthPassed(output forgeRetainedOutput, families []rsiFamilyStatus) bool {
	return output.Command == "ao-command rsi health" &&
		output.Status == "passed" &&
		output.RSIMode == "governed_fixture_local" &&
		output.RSICapability == "demonstrated_local_fixture_loop" &&
		output.OperatorMode == operatorMode &&
		!output.MutatesRepositories &&
		retainedFamiliesMatch(output.Families, families)
}

func retainedFamiliesMatch(retained []retainedRSIFamily, current []rsiFamilyStatus) bool {
	if len(retained) != len(current) {
		return false
	}
	byFamily := make(map[string]retainedRSIFamily, len(retained))
	for _, family := range retained {
		byFamily[family.Family] = family
	}
	for _, family := range current {
		retainedFamily, ok := byFamily[family.Family]
		if !ok || retainedFamily.Status != family.Status || retainedFamily.Passed != family.Passed {
			return false
		}
	}
	return true
}

func foundryGateCandidateEvidence(gate foundryRSIImprovementGate) (foundryEvalResultRef, bool) {
	for _, evidence := range gate.Evidence {
		if evidence.Label == "candidate" {
			return evidence, true
		}
	}
	return foundryEvalResultRef{}, false
}

func readJSONFile(path string, target any) error {
	bytes, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	if err := json.Unmarshal(bytes, target); err != nil {
		return fmt.Errorf("invalid JSON: %w", err)
	}
	return nil
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
		ActionID:    "inspect-active-stack-handoff",
		Description: "AO Forge is production-ready; inspect the AO Foundry active-stack readiness ledger before release handoff.",
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
