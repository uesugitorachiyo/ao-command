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
	"sort"
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
	case "atlas":
		return a.atlas(args[1:])
	case "pulse":
		return a.pulse(args[1:])
	case "complex-refactor":
		return a.complexRefactor(args[1:])
	case "live-mutation":
		return a.liveMutation(args[1:])
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
  ao-command atlas status --status PATH [--json]
  ao-command atlas authority-ladder --mission-status PATH [--json]
  ao-command pulse status --preflight PATH --lifecycle PATH --start-gate PATH [--json]
  ao-command complex-refactor status --summary PATH [--json]
  ao-command live-mutation status --authority PATH --request PATH --forge-plan PATH --ao2-packet PATH --isolation PATH --rollback PATH --kill-switch PATH [--json]
  ao-command live-mutation approval --request PATH --ticket PATH [--json]
  ao-command live-mutation pr-rehearsal --gate PATH [--json]
  ao-command rsi health --arena-gate PATH --crucible-gate PATH --sentinel-verdict PATH --promoter-gate PATH --foundry-gate PATH --foundry-candidate PATH --foundry-next-task PATH --forge-retained-gate PATH --forge-retained-candidate PATH --forge-retained-next-task PATH --forge-retained-command-health PATH [--bundle-out PATH] [--json]
  ao-command rsi manifest --manifest PATH [--json]
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

func (a App) atlas(args []string) int {
	if len(args) == 0 {
		fmt.Fprintln(a.Stderr, "ao-command atlas: usage: ao-command atlas <status|authority-ladder> ...")
		return 2
	}
	switch args[0] {
	case "authority-ladder":
		return a.atlasAuthorityLadder(args[1:])
	case "status":
		return a.atlasStatus(args[1:])
	default:
		fmt.Fprintln(a.Stderr, "ao-command atlas: usage: ao-command atlas <status|authority-ladder> ...")
		return 2
	}
}

func (a App) atlasAuthorityLadder(args []string) int {
	var missionStatusPath string
	var jsonOut bool
	fs := flag.NewFlagSet("atlas authority-ladder", flag.ContinueOnError)
	fs.SetOutput(a.Stderr)
	fs.StringVar(&missionStatusPath, "mission-status", "", "path to AO Atlas mission status JSON with authority_ladder readback")
	fs.BoolVar(&jsonOut, "json", false, "emit JSON")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if strings.TrimSpace(missionStatusPath) == "" {
		fmt.Fprintln(a.Stderr, "ao-command atlas authority-ladder: --mission-status is required")
		return 2
	}
	summary, err := readAtlasAuthorityLadderStatus(missionStatusPath)
	if err != nil {
		fmt.Fprintf(a.Stderr, "ao-command atlas authority-ladder: %v\n", err)
		return 1
	}
	if jsonOut {
		return a.writeJSON(summary)
	}
	fmt.Fprintf(a.Stdout, "ao_command_atlas_authority_ladder=%s\n", summary.Status)
	fmt.Fprintf(a.Stdout, "mission_status=%s\n", summary.MissionStatus)
	fmt.Fprintf(a.Stdout, "workgraph_id=%s\n", summary.WorkgraphID)
	fmt.Fprintf(a.Stdout, "target_instance=%s\n", summary.TargetInstance)
	fmt.Fprintf(a.Stdout, "current_class=%s\n", summary.CurrentClass)
	fmt.Fprintf(a.Stdout, "next_class=%s\n", summary.NextClass)
	fmt.Fprintf(a.Stdout, "operator_mode=%s\n", summary.OperatorMode)
	fmt.Fprintf(a.Stdout, "mutates_repositories=%t\n", summary.MutatesRepositories)
	fmt.Fprintf(a.Stdout, "schedules_work=%t\n", summary.SchedulesWork)
	fmt.Fprintf(a.Stdout, "executes_work=%t\n", summary.ExecutesWork)
	for _, class := range summary.ProvenLiveClasses {
		fmt.Fprintf(a.Stdout, "proven_live_class=%s\n", class)
	}
	for _, class := range summary.DryRunReadyClasses {
		fmt.Fprintf(a.Stdout, "dry_run_ready_class=%s\n", class)
	}
	for _, blocker := range summary.Blockers {
		fmt.Fprintf(a.Stdout, "blocker=%s\n", blocker)
	}
	for _, evidence := range summary.RequiredEvidence {
		fmt.Fprintf(a.Stdout, "required_evidence=%s\n", evidence)
	}
	for _, class := range sortedStringKeys(summary.DeniedHigherClasses) {
		fmt.Fprintf(a.Stdout, "denied_higher_class=%s reason=%s\n", class, summary.DeniedHigherClasses[class])
	}
	for _, gate := range summary.DoNotAdvanceGates {
		fmt.Fprintf(a.Stdout, "do_not_advance_gate=%s\n", gate)
	}
	return 0
}

func (a App) atlasStatus(args []string) int {
	var statusPath string
	var jsonOut bool
	fs := flag.NewFlagSet("atlas status", flag.ContinueOnError)
	fs.SetOutput(a.Stderr)
	fs.StringVar(&statusPath, "status", "", "path to AO Foundry atlas status JSON")
	fs.BoolVar(&jsonOut, "json", false, "emit JSON")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if strings.TrimSpace(statusPath) == "" {
		fmt.Fprintln(a.Stderr, "ao-command atlas status: --status is required")
		return 2
	}
	summary, err := readAtlasStatus(statusPath)
	if err != nil {
		fmt.Fprintf(a.Stderr, "ao-command atlas status: %v\n", err)
		return 1
	}
	if jsonOut {
		return a.writeJSON(summary)
	}
	fmt.Fprintf(a.Stdout, "ao_command_atlas_status=%s\n", summary.Status)
	fmt.Fprintf(a.Stdout, "foundry_status=%s\n", summary.FoundryStatus)
	fmt.Fprintf(a.Stdout, "mode=%s\n", summary.Mode)
	fmt.Fprintf(a.Stdout, "registry_id=%s\n", summary.RegistryID)
	fmt.Fprintf(a.Stdout, "import_id=%s\n", summary.ImportID)
	fmt.Fprintf(a.Stdout, "workgraph_id=%s\n", summary.WorkgraphID)
	fmt.Fprintf(a.Stdout, "target_instance=%s\n", summary.TargetInstance)
	fmt.Fprintf(a.Stdout, "readback_status=%s\n", summary.ReadbackStatus)
	fmt.Fprintf(a.Stdout, "task_id=%s\n", summary.TaskID)
	fmt.Fprintf(a.Stdout, "operator_mode=%s\n", summary.OperatorMode)
	fmt.Fprintf(a.Stdout, "orchestration_owner=%s\n", summary.OrchestrationOwner)
	fmt.Fprintf(a.Stdout, "atlas_authority=%s\n", summary.AtlasAuthority)
	fmt.Fprintf(a.Stdout, "schedules_work=%t\n", summary.SchedulesWork)
	fmt.Fprintf(a.Stdout, "executes_work=%t\n", summary.ExecutesWork)
	fmt.Fprintf(a.Stdout, "approves_work=%t\n", summary.ApprovesWork)
	fmt.Fprintf(a.Stdout, "mutates_repositories=%t\n", summary.MutatesRepositories)
	for _, action := range summary.NextActions {
		fmt.Fprintf(a.Stdout, "next_action=%s\n", action)
	}
	return 0
}

func (a App) pulse(args []string) int {
	if len(args) == 0 {
		fmt.Fprintln(a.Stderr, "ao-command pulse: usage: ao-command pulse status --preflight PATH --lifecycle PATH --start-gate PATH [--json]")
		return 2
	}
	switch args[0] {
	case "status":
		return a.pulseStatus(args[1:])
	default:
		fmt.Fprintln(a.Stderr, "ao-command pulse: usage: ao-command pulse status --preflight PATH --lifecycle PATH --start-gate PATH [--json]")
		return 2
	}
}

func (a App) pulseStatus(args []string) int {
	var preflightPath, lifecyclePath, startGatePath string
	var jsonOut bool
	fs := flag.NewFlagSet("pulse status", flag.ContinueOnError)
	fs.SetOutput(a.Stderr)
	fs.StringVar(&preflightPath, "preflight", "", "path to AO Foundry pulse intake preflight JSON")
	fs.StringVar(&lifecyclePath, "lifecycle", "", "path to AO Foundry pulse PR lifecycle JSON")
	fs.StringVar(&startGatePath, "start-gate", "", "path to AO Foundry pulse overnight start gate JSON")
	fs.BoolVar(&jsonOut, "json", false, "emit JSON")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if strings.TrimSpace(preflightPath) == "" || strings.TrimSpace(lifecyclePath) == "" || strings.TrimSpace(startGatePath) == "" {
		fmt.Fprintln(a.Stderr, "ao-command pulse status: --preflight, --lifecycle, and --start-gate are required")
		return 2
	}
	summary, err := readPulseGateStatus(preflightPath, lifecyclePath, startGatePath)
	if err != nil {
		fmt.Fprintf(a.Stderr, "ao-command pulse status: %v\n", err)
		return 1
	}
	if jsonOut {
		return a.writeJSON(summary)
	}
	fmt.Fprintf(a.Stdout, "ao_command_pulse_status=%s\n", summary.Status)
	fmt.Fprintf(a.Stdout, "preflight_status=%s\n", summary.PreflightStatus)
	fmt.Fprintf(a.Stdout, "lifecycle_status=%s\n", summary.LifecycleStatus)
	fmt.Fprintf(a.Stdout, "start_gate_status=%s\n", summary.StartGateStatus)
	fmt.Fprintf(a.Stdout, "allowed_next_action=%s\n", summary.AllowedNextAction)
	fmt.Fprintf(a.Stdout, "first_failing_check=%s\n", summary.FirstFailingCheck)
	fmt.Fprintf(a.Stdout, "operator_mode=%s\n", summary.OperatorMode)
	fmt.Fprintf(a.Stdout, "mutates_repositories=%t\n", summary.MutatesRepositories)
	for _, action := range summary.BlockingNextActions {
		fmt.Fprintf(a.Stdout, "blocking_next_action=%s\n", action)
	}
	for _, suggestion := range summary.MaintenanceSuggestions {
		fmt.Fprintf(a.Stdout, "maintenance_suggestion=%s\n", suggestion)
	}
	return 0
}

func (a App) complexRefactor(args []string) int {
	if len(args) == 0 {
		fmt.Fprintln(a.Stderr, "ao-command complex-refactor: usage: ao-command complex-refactor status --summary PATH [--json]")
		return 2
	}
	switch args[0] {
	case "status":
		return a.complexRefactorStatus(args[1:])
	default:
		fmt.Fprintln(a.Stderr, "ao-command complex-refactor: usage: ao-command complex-refactor status --summary PATH [--json]")
		return 2
	}
}

func (a App) complexRefactorStatus(args []string) int {
	var summaryPath string
	var jsonOut bool
	fs := flag.NewFlagSet("complex-refactor status", flag.ContinueOnError)
	fs.SetOutput(a.Stderr)
	fs.StringVar(&summaryPath, "summary", "", "path to AO Foundry complex-refactor rehearsal summary JSON")
	fs.BoolVar(&jsonOut, "json", false, "emit JSON")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if strings.TrimSpace(summaryPath) == "" {
		fmt.Fprintln(a.Stderr, "ao-command complex-refactor status: --summary is required")
		return 2
	}
	summary, err := readComplexRefactorStatus(summaryPath)
	if err != nil {
		fmt.Fprintf(a.Stderr, "ao-command complex-refactor status: %v\n", err)
		return 1
	}
	if jsonOut {
		return a.writeJSON(summary)
	}
	fmt.Fprintf(a.Stdout, "ao_command_complex_refactor_status=%s\n", summary.Status)
	fmt.Fprintf(a.Stdout, "summary=%s\n", summary.Summary)
	fmt.Fprintf(a.Stdout, "mode=%s\n", summary.Mode)
	fmt.Fprintf(a.Stdout, "next_action=%s\n", summary.NextAction)
	fmt.Fprintf(a.Stdout, "next_recommended_factory_task=%s\n", summary.NextRecommendedFactoryTask)
	fmt.Fprintf(a.Stdout, "target_factory_repo=%s\n", summary.TargetFactoryRepo)
	fmt.Fprintf(a.Stdout, "total_tasks=%d\n", summary.TaskCounts.Total)
	fmt.Fprintf(a.Stdout, "ready_tasks=%d\n", summary.TaskCounts.Ready)
	fmt.Fprintf(a.Stdout, "blocked_tasks=%d\n", summary.TaskCounts.Blocked)
	fmt.Fprintf(a.Stdout, "completed_tasks=%d\n", summary.TaskCounts.Completed)
	fmt.Fprintf(a.Stdout, "failed_tasks=%d\n", summary.TaskCounts.Failed)
	fmt.Fprintf(a.Stdout, "repair_plan_status=%s\n", summary.RepairPlan.Status)
	fmt.Fprintf(a.Stdout, "repair_task=%s\n", summary.RepairPlan.RepairTaskID)
	fmt.Fprintf(a.Stdout, "context_repack_status=%s\n", summary.ContextRepack.Status)
	fmt.Fprintf(a.Stdout, "context_repack_reason=%s\n", summary.ContextRepack.MissingContextReason)
	fmt.Fprintf(a.Stdout, "first_failing_check=%s\n", summary.FirstFailingCheck)
	fmt.Fprintf(a.Stdout, "operator_mode=%s\n", summary.OperatorMode)
	fmt.Fprintf(a.Stdout, "mutates_repositories=%t\n", summary.MutatesRepositories)
	fmt.Fprintf(a.Stdout, "schedules_work=%t\n", summary.SchedulesWork)
	fmt.Fprintf(a.Stdout, "executes_work=%t\n", summary.ExecutesWork)
	fmt.Fprintf(a.Stdout, "approves_work=%t\n", summary.ApprovesWork)
	fmt.Fprintf(a.Stdout, "calls_providers=%t\n", summary.CallsProviders)
	for _, action := range summary.BlockingNextActions {
		fmt.Fprintf(a.Stdout, "blocking_next_action=%s\n", action)
	}
	for _, suggestion := range summary.MaintenanceSuggestions {
		fmt.Fprintf(a.Stdout, "maintenance_suggestion=%s\n", suggestion)
	}
	return 0
}

func (a App) liveMutation(args []string) int {
	if len(args) == 0 {
		fmt.Fprintln(a.Stderr, "ao-command live-mutation: usage: ao-command live-mutation <status|approval|pr-rehearsal> ...")
		return 2
	}
	switch args[0] {
	case "approval":
		return a.liveMutationApproval(args[1:])
	case "pr-rehearsal":
		return a.liveMutationPRRehearsal(args[1:])
	case "status":
		return a.liveMutationStatus(args[1:])
	default:
		fmt.Fprintln(a.Stderr, "ao-command live-mutation: usage: ao-command live-mutation <status|approval|pr-rehearsal> ...")
		return 2
	}
}

func (a App) liveMutationApproval(args []string) int {
	var requestPath, ticketPath string
	var jsonOut bool
	fs := flag.NewFlagSet("live-mutation approval", flag.ContinueOnError)
	fs.SetOutput(a.Stderr)
	fs.StringVar(&requestPath, "request", "", "path to AO Foundry live docs approval request JSON")
	fs.StringVar(&ticketPath, "ticket", "", "path to AO Covenant live docs approval ticket JSON")
	fs.BoolVar(&jsonOut, "json", false, "emit JSON")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if requestPath == "" || ticketPath == "" {
		fmt.Fprintln(a.Stderr, "ao-command live-mutation approval: --request and --ticket are required")
		return 2
	}
	summary, err := readLiveMutationApproval(requestPath, ticketPath)
	if err != nil {
		fmt.Fprintf(a.Stderr, "ao-command live-mutation approval: %v\n", err)
		return 1
	}
	if jsonOut {
		return a.writeJSON(summary)
	}
	fmt.Fprintf(a.Stdout, "ao_command_live_mutation_approval=%s\n", summary.ApprovalState)
	fmt.Fprintf(a.Stdout, "status=%s\n", summary.Status)
	fmt.Fprintf(a.Stdout, "safe_to_request=%t\n", summary.SafeToRequest)
	fmt.Fprintf(a.Stdout, "safe_to_execute=%t\n", summary.SafeToExecute)
	fmt.Fprintf(a.Stdout, "approval_state=%s\n", summary.ApprovalState)
	fmt.Fprintf(a.Stdout, "request_id=%s\n", summary.RequestID)
	fmt.Fprintf(a.Stdout, "ticket_id=%s\n", summary.TicketID)
	fmt.Fprintf(a.Stdout, "first_failing_check=%s\n", summary.FirstFailingCheck)
	fmt.Fprintf(a.Stdout, "operator_mode=%s\n", summary.OperatorMode)
	fmt.Fprintf(a.Stdout, "mutates_repositories=%t\n", summary.MutatesRepositories)
	fmt.Fprintf(a.Stdout, "approves_work=%t\n", summary.ApprovesWork)
	fmt.Fprintf(a.Stdout, "executes_work=%t\n", summary.ExecutesWork)
	fmt.Fprintf(a.Stdout, "calls_providers=%t\n", summary.CallsProviders)
	return 0
}

func (a App) liveMutationPRRehearsal(args []string) int {
	var gatePath string
	var jsonOut bool
	fs := flag.NewFlagSet("live-mutation pr-rehearsal", flag.ContinueOnError)
	fs.SetOutput(a.Stderr)
	fs.StringVar(&gatePath, "gate", "", "path to AO Foundry live docs PR rehearsal gate JSON")
	fs.BoolVar(&jsonOut, "json", false, "emit JSON")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if gatePath == "" {
		fmt.Fprintln(a.Stderr, "ao-command live-mutation pr-rehearsal: --gate is required")
		return 2
	}
	summary, err := readLiveMutationPRRehearsal(gatePath)
	if err != nil {
		fmt.Fprintf(a.Stderr, "ao-command live-mutation pr-rehearsal: %v\n", err)
		return 1
	}
	if jsonOut {
		return a.writeJSON(summary)
	}
	fmt.Fprintf(a.Stdout, "ao_command_live_pr_rehearsal=%s\n", summary.Status)
	fmt.Fprintf(a.Stdout, "status=%s\n", summary.Status)
	fmt.Fprintf(a.Stdout, "first_live_class=%s\n", summary.FirstLiveClass)
	fmt.Fprintf(a.Stdout, "safe_to_request=%t\n", summary.SafeToRequest)
	fmt.Fprintf(a.Stdout, "safe_to_execute=%t\n", summary.SafeToExecute)
	fmt.Fprintf(a.Stdout, "exact_next_step=%s\n", summary.ExactNextStep)
	fmt.Fprintf(a.Stdout, "allowed_next_action=%s\n", summary.AllowedNextAction)
	fmt.Fprintf(a.Stdout, "first_failing_check=%s\n", summary.FirstFailingCheck)
	fmt.Fprintf(a.Stdout, "operator_mode=%s\n", summary.OperatorMode)
	fmt.Fprintf(a.Stdout, "mutates_repositories=%t\n", summary.MutatesRepositories)
	fmt.Fprintf(a.Stdout, "creates_branch=%t\n", summary.CreatesBranch)
	fmt.Fprintf(a.Stdout, "creates_worktree=%t\n", summary.CreatesWorktree)
	fmt.Fprintf(a.Stdout, "opens_pr=%t\n", summary.OpensPR)
	fmt.Fprintf(a.Stdout, "merges_pr=%t\n", summary.MergesPR)
	fmt.Fprintf(a.Stdout, "executes_work=%t\n", summary.ExecutesWork)
	fmt.Fprintf(a.Stdout, "approves_work=%t\n", summary.ApprovesWork)
	fmt.Fprintf(a.Stdout, "calls_providers=%t\n", summary.CallsProviders)
	return 0
}

func (a App) liveMutationStatus(args []string) int {
	var authorityPath, requestPath, forgePlanPath, ao2PacketPath, isolationPath, rollbackPath, killSwitchPath, sentinelHoldPath string
	var jsonOut bool
	fs := flag.NewFlagSet("live-mutation status", flag.ContinueOnError)
	fs.SetOutput(a.Stderr)
	fs.StringVar(&authorityPath, "authority", "", "path to AO Covenant live-mutation authority JSON")
	fs.StringVar(&requestPath, "request", "", "path to AO Foundry live-mutation request JSON")
	fs.StringVar(&forgePlanPath, "forge-plan", "", "path to AO Forge live-mutation dry-run plan JSON")
	fs.StringVar(&ao2PacketPath, "ao2-packet", "", "path to AO2 live-mutation dry-run packet JSON")
	fs.StringVar(&isolationPath, "isolation", "", "path to AO Foundry worktree isolation proof JSON")
	fs.StringVar(&rollbackPath, "rollback", "", "path to AO Foundry rollback rehearsal JSON")
	fs.StringVar(&killSwitchPath, "kill-switch", "", "path to operator kill-switch state JSON")
	fs.StringVar(&sentinelHoldPath, "sentinel-hold", "", "optional path to AO Sentinel live-mutation hold JSON")
	fs.BoolVar(&jsonOut, "json", false, "emit JSON")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if authorityPath == "" || requestPath == "" || forgePlanPath == "" || ao2PacketPath == "" || isolationPath == "" || rollbackPath == "" || killSwitchPath == "" {
		fmt.Fprintln(a.Stderr, "ao-command live-mutation status: --authority, --request, --forge-plan, --ao2-packet, --isolation, --rollback, and --kill-switch are required")
		return 2
	}
	summary, err := readLiveMutationStatus(authorityPath, requestPath, forgePlanPath, ao2PacketPath, isolationPath, rollbackPath, killSwitchPath, sentinelHoldPath)
	if err != nil {
		fmt.Fprintf(a.Stderr, "ao-command live-mutation status: %v\n", err)
		return 1
	}
	if jsonOut {
		return a.writeJSON(summary)
	}
	fmt.Fprintf(a.Stdout, "ao_command_live_mutation_status=%s\n", summary.Status)
	fmt.Fprintf(a.Stdout, "allowed_next_action=%s\n", summary.AllowedNextAction)
	fmt.Fprintf(a.Stdout, "current_mutation_class=%s\n", summary.CurrentMutationClass)
	fmt.Fprintf(a.Stdout, "next_mutation_class=%s\n", summary.NextMutationClass)
	fmt.Fprintf(a.Stdout, "highest_proven_live_class=%s\n", summary.HighestProvenLiveClass)
	fmt.Fprintf(a.Stdout, "current_class_live_evidence_status=%s\n", summary.CurrentClassLiveEvidenceStatus)
	if summary.LowRiskCodeLiveEvidenceStatus != "" {
		fmt.Fprintf(a.Stdout, "low_risk_code_live_evidence_status=%s\n", summary.LowRiskCodeLiveEvidenceStatus)
	}
	if summary.NextDeniedClass != "" {
		fmt.Fprintf(a.Stdout, "next_denied_class=%s\n", summary.NextDeniedClass)
		fmt.Fprintf(a.Stdout, "next_denied_reason=%s\n", summary.NextDeniedReason)
	}
	fmt.Fprintf(a.Stdout, "safe_to_request=%t\n", summary.SafeToRequest)
	fmt.Fprintf(a.Stdout, "safe_to_execute=%t\n", summary.SafeToExecute)
	fmt.Fprintf(a.Stdout, "first_failing_check=%s\n", summary.FirstFailingCheck)
	fmt.Fprintf(a.Stdout, "kill_switch_state=%s\n", summary.KillSwitchState)
	fmt.Fprintf(a.Stdout, "operator_mode=%s\n", summary.OperatorMode)
	fmt.Fprintf(a.Stdout, "mutates_repositories=%t\n", summary.MutatesRepositories)
	fmt.Fprintf(a.Stdout, "schedules_work=%t\n", summary.SchedulesWork)
	fmt.Fprintf(a.Stdout, "executes_work=%t\n", summary.ExecutesWork)
	fmt.Fprintf(a.Stdout, "approves_work=%t\n", summary.ApprovesWork)
	fmt.Fprintf(a.Stdout, "calls_providers=%t\n", summary.CallsProviders)
	fmt.Fprintf(a.Stdout, "release_or_publish_allowed=%t\n", summary.ReleaseOrPublishAllowed)
	for _, artifact := range summary.Artifacts {
		fmt.Fprintf(a.Stdout, "artifact=%s status=%s schema=%s path=%s\n", artifact.Name, artifact.Status, artifact.SchemaVersion, artifact.Path)
	}
	for _, action := range summary.BlockingNextActions {
		fmt.Fprintf(a.Stdout, "blocking_next_action=%s\n", action)
	}
	for _, evidence := range summary.RequiredEvidence {
		fmt.Fprintf(a.Stdout, "required_evidence=%s\n", evidence)
	}
	if summary.LowRiskCodeDenialAudit != nil {
		audit := summary.LowRiskCodeDenialAudit
		fmt.Fprintf(a.Stdout, "denial_audit_next_denied_class=%s\n", audit.NextDeniedClass)
		fmt.Fprintf(a.Stdout, "denial_audit_sentinel_state=%s\n", audit.SentinelState)
		fmt.Fprintf(a.Stdout, "denial_audit_promoter_state=%s\n", audit.PromoterState)
		fmt.Fprintf(a.Stdout, "denial_audit_exact_next_action=%s\n", audit.ExactNextAction)
		for _, evidence := range audit.MissingPolicyEvidence {
			fmt.Fprintf(a.Stdout, "denial_audit_missing_policy_evidence=%s\n", evidence)
		}
		for _, evidence := range audit.MissingRollbackEvidence {
			fmt.Fprintf(a.Stdout, "denial_audit_missing_rollback_evidence=%s\n", evidence)
		}
		for _, evidence := range audit.MissingSentinelPromoterEvidence {
			fmt.Fprintf(a.Stdout, "denial_audit_missing_sentinel_promoter_evidence=%s\n", evidence)
		}
		for _, requirement := range audit.CIRequirements {
			fmt.Fprintf(a.Stdout, "denial_audit_ci_requirement=%s\n", requirement)
		}
	}
	if summary.SentinelHold != nil {
		hold := summary.SentinelHold
		fmt.Fprintf(a.Stdout, "sentinel_hold_status=%s\n", hold.Status)
		fmt.Fprintf(a.Stdout, "sentinel_hold_required=%t\n", hold.HoldRequired)
		fmt.Fprintf(a.Stdout, "sentinel_class_verdict=%s\n", hold.ClassVerdictStatus)
		fmt.Fprintf(a.Stdout, "sentinel_class_test_coverage=%s\n", hold.TestCoverageStatus)
		fmt.Fprintf(a.Stdout, "sentinel_class_rollback=%s\n", hold.RollbackStatus)
		fmt.Fprintf(a.Stdout, "sentinel_class_diff_size=%s\n", hold.DiffSizeStatus)
		fmt.Fprintf(a.Stdout, "sentinel_class_file_class=%s\n", hold.FileClassStatus)
		fmt.Fprintf(a.Stdout, "sentinel_class_evidence_freshness=%s\n", hold.EvidenceFreshnessStatus)
		fmt.Fprintf(a.Stdout, "sentinel_class_ci=%s\n", hold.CIStatus)
	}
	for _, state := range summary.RepoStates {
		fmt.Fprintf(a.Stdout, "repo_state=%s order=%d planned_pr=%s status=%s execution_status=%s rollback=%s rollback_scope=%s depends_on=%s merge_after=%s\n", state.Repo, state.Order, state.PlannedPR, state.Status, state.ExecutionStatus, state.RollbackStatus, strings.Join(state.RollbackScope, ","), strings.Join(state.DependsOn, ","), strings.Join(state.MergeAfter, ","))
	}
	for _, class := range sortedStringKeys(summary.DeniedHigherClasses) {
		fmt.Fprintf(a.Stdout, "denied_higher_class=%s reason=%s\n", class, summary.DeniedHigherClasses[class])
	}
	for _, suggestion := range summary.MaintenanceSuggestions {
		fmt.Fprintf(a.Stdout, "maintenance_suggestion=%s\n", suggestion)
	}
	return 0
}

func (a App) rsi(args []string) int {
	if len(args) == 0 {
		fmt.Fprintln(a.Stderr, "ao-command rsi: usage: ao-command rsi health ... | ao-command rsi manifest --manifest PATH [--json]")
		return 2
	}
	switch args[0] {
	case "health":
		return a.rsiHealth(args[1:])
	case "manifest":
		return a.rsiManifest(args[1:])
	default:
		fmt.Fprintln(a.Stderr, "ao-command rsi: usage: ao-command rsi health ... | ao-command rsi manifest --manifest PATH [--json]")
		return 2
	}
}

func (a App) rsiHealth(args []string) int {
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
	if err := fs.Parse(args); err != nil {
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
	for _, claim := range summary.ClaimLevels {
		fmt.Fprintf(a.Stdout, "claim_level=%s decision=%s status=%s reason=%s\n", claim.Claim, claim.Decision, claim.Status, claim.Reason)
	}
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

func (a App) rsiManifest(args []string) int {
	var manifestPath string
	var jsonOut bool
	fs := flag.NewFlagSet("rsi manifest", flag.ContinueOnError)
	fs.SetOutput(a.Stderr)
	fs.StringVar(&manifestPath, "manifest", "", "path to AO Architecture RSI claim evidence manifest JSON")
	fs.BoolVar(&jsonOut, "json", false, "emit JSON")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if strings.TrimSpace(manifestPath) == "" {
		fmt.Fprintln(a.Stderr, "ao-command rsi manifest: --manifest is required")
		return 2
	}
	summary, err := readRSIManifest(manifestPath)
	if err != nil {
		fmt.Fprintf(a.Stderr, "ao-command rsi manifest: %v\n", err)
		return 1
	}
	if jsonOut {
		return a.writeJSON(summary)
	}
	fmt.Fprintf(a.Stdout, "ao_command_rsi_manifest=%s\n", summary.Status)
	fmt.Fprintf(a.Stdout, "schema_version=%s\n", summary.ManifestSchemaVersion)
	fmt.Fprintf(a.Stdout, "manifest=%s\n", summary.Manifest)
	fmt.Fprintf(a.Stdout, "operator_mode=%s\n", summary.OperatorMode)
	fmt.Fprintf(a.Stdout, "mutates_repositories=%t\n", summary.MutatesRepositories)
	for _, claim := range summary.ClaimLevels {
		fmt.Fprintf(a.Stdout, "claim_level=%s decision=%s status=%s\n", claim.ClaimLevel, claim.Decision, claim.Status)
	}
	fmt.Fprintf(a.Stdout, "active_repositories=%d\n", len(summary.ActiveRepositories))
	fmt.Fprintf(a.Stdout, "out_of_scope_repositories=%d\n", len(summary.DeprecatedOrOutOfScopeRepositories))
	fmt.Fprintf(a.Stdout, "full_claim_required_evidence=%d\n", len(summary.FullClaimRequiredEvidence))
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

type foundryAtlasStatus struct {
	SchemaVersion  string            `json:"schema_version"`
	Status         string            `json:"status"`
	Mode           string            `json:"mode"`
	RegistryID     string            `json:"registry_id"`
	ImportID       string            `json:"import_id"`
	WorkgraphID    string            `json:"workgraph_id"`
	TargetInstance string            `json:"target_instance"`
	ReadbackStatus string            `json:"readback_status"`
	TaskID         string            `json:"task_id"`
	TaskDigest     string            `json:"task_digest"`
	RunLinkDigest  string            `json:"run_link_digest"`
	SchedulesWork  bool              `json:"schedules_work"`
	ExecutesWork   bool              `json:"executes_work"`
	ApprovesWork   bool              `json:"approves_work"`
	Evidence       map[string]string `json:"evidence"`
	NextActions    []string          `json:"next_actions"`
}

type atlasMissionStatus struct {
	ContractVersion  string                      `json:"contract_version"`
	IntakeID         string                      `json:"intake_id"`
	WorkgraphID      string                      `json:"workgraph_id"`
	TargetInstance   string                      `json:"target_instance"`
	CompletionStatus string                      `json:"completion_status"`
	AuthorityLadder  *atlasAuthorityLadderStatus `json:"authority_ladder"`
	SchedulesWork    bool                        `json:"schedules_work"`
	ExecutesWork     bool                        `json:"executes_work"`
}

type atlasAuthorityLadderStatus struct {
	CurrentClass        string            `json:"current_class"`
	NextClass           string            `json:"next_class"`
	ProvenLiveClasses   []string          `json:"proven_live_classes"`
	DryRunReadyClasses  []string          `json:"dry_run_ready_classes"`
	Blockers            []string          `json:"blockers"`
	RequiredEvidence    []string          `json:"required_evidence"`
	DeniedHigherClasses map[string]string `json:"denied_higher_classes"`
	DoNotAdvanceGates   []string          `json:"do_not_advance_gates"`
}

type atlasAuthorityLadderSummary struct {
	SchemaVersion        string            `json:"schema_version"`
	CommandSchemaVersion string            `json:"command_schema_version"`
	Status               string            `json:"status"`
	MissionStatus        string            `json:"mission_status"`
	WorkgraphID          string            `json:"workgraph_id"`
	TargetInstance       string            `json:"target_instance"`
	CurrentClass         string            `json:"current_class"`
	NextClass            string            `json:"next_class"`
	ProvenLiveClasses    []string          `json:"proven_live_classes"`
	DryRunReadyClasses   []string          `json:"dry_run_ready_classes"`
	Blockers             []string          `json:"blockers"`
	RequiredEvidence     []string          `json:"required_evidence"`
	DeniedHigherClasses  map[string]string `json:"denied_higher_classes"`
	DoNotAdvanceGates    []string          `json:"do_not_advance_gates"`
	OperatorMode         string            `json:"operator_mode"`
	SchedulesWork        bool              `json:"schedules_work"`
	ExecutesWork         bool              `json:"executes_work"`
	MutatesRepositories  bool              `json:"mutates_repositories"`
}

type atlasStatusSummary struct {
	SchemaVersion        string            `json:"schema_version"`
	CommandSchemaVersion string            `json:"command_schema_version"`
	Status               string            `json:"status"`
	FoundryStatus        string            `json:"foundry_status"`
	Mode                 string            `json:"mode"`
	RegistryID           string            `json:"registry_id"`
	ImportID             string            `json:"import_id"`
	WorkgraphID          string            `json:"workgraph_id"`
	TargetInstance       string            `json:"target_instance"`
	ReadbackStatus       string            `json:"readback_status"`
	TaskID               string            `json:"task_id"`
	TaskDigest           string            `json:"task_digest"`
	RunLinkDigest        string            `json:"run_link_digest"`
	OperatorMode         string            `json:"operator_mode"`
	OrchestrationOwner   string            `json:"orchestration_owner"`
	AtlasAuthority       string            `json:"atlas_authority"`
	SchedulesWork        bool              `json:"schedules_work"`
	ExecutesWork         bool              `json:"executes_work"`
	ApprovesWork         bool              `json:"approves_work"`
	MutatesRepositories  bool              `json:"mutates_repositories"`
	Evidence             map[string]string `json:"evidence"`
	NextActions          []string          `json:"next_actions"`
}

type foundryPulseIntakePreflight struct {
	SchemaVersion          string        `json:"schema_version"`
	Status                 string        `json:"status"`
	BlueprintStatus        string        `json:"blueprint_status"`
	AtlasStatus            string        `json:"atlas_status"`
	FirstFailingCheck      string        `json:"first_failing_check"`
	Checks                 []pulseCheck  `json:"checks"`
	BlockingNextActions    []string      `json:"blocking_next_actions"`
	MaintenanceSuggestions []string      `json:"maintenance_suggestions"`
	SourceArtifacts        []pulseSource `json:"source_artifacts"`
}

type foundryPulsePRLifecycle struct {
	SchemaVersion     string `json:"schema_version"`
	CurrentSlice      string `json:"current_slice"`
	TargetRepo        string `json:"target_repo"`
	Branch            string `json:"branch"`
	PRNumber          int    `json:"pr_number"`
	PRURL             string `json:"pr_url"`
	PRState           string `json:"pr_state"`
	CheckState        string `json:"check_state"`
	MergeState        string `json:"merge_state"`
	CleanupState      string `json:"cleanup_state"`
	AllowedNextAction string `json:"allowed_next_action"`
	BlockerReason     string `json:"blocker_reason"`
}

type foundryPulseOvernightStartGate struct {
	SchemaVersion          string        `json:"schema_version"`
	Status                 string        `json:"status"`
	AllowedNextAction      string        `json:"allowed_next_action"`
	FirstFailingCheck      string        `json:"first_failing_check"`
	BlockingNextActions    []string      `json:"blocking_next_actions"`
	MaintenanceSuggestions []string      `json:"maintenance_suggestions"`
	SourceHashes           []pulseSource `json:"source_hashes"`
}

type pulseCheck struct {
	Name   string `json:"name"`
	Status string `json:"status"`
	Reason string `json:"reason"`
}

type pulseSource struct {
	Name          string `json:"name"`
	Path          string `json:"path"`
	SchemaVersion string `json:"schema_version"`
	Status        string `json:"status,omitempty"`
	SHA256        string `json:"sha256"`
}

type pulseGateStatusSummary struct {
	SchemaVersion          string        `json:"schema_version"`
	CommandSchemaVersion   string        `json:"command_schema_version"`
	Status                 string        `json:"status"`
	Preflight              string        `json:"preflight"`
	Lifecycle              string        `json:"lifecycle"`
	StartGate              string        `json:"start_gate"`
	PreflightStatus        string        `json:"preflight_status"`
	BlueprintStatus        string        `json:"blueprint_status"`
	AtlasStatus            string        `json:"atlas_status"`
	LifecycleStatus        string        `json:"lifecycle_status"`
	StartGateStatus        string        `json:"start_gate_status"`
	AllowedNextAction      string        `json:"allowed_next_action"`
	FirstFailingCheck      string        `json:"first_failing_check"`
	BlockingNextActions    []string      `json:"blocking_next_actions"`
	MaintenanceSuggestions []string      `json:"maintenance_suggestions"`
	SourceArtifacts        []pulseSource `json:"source_artifacts"`
	SourceHashes           []pulseSource `json:"source_hashes"`
	OperatorMode           string        `json:"operator_mode"`
	MutatesRepositories    bool          `json:"mutates_repositories"`
}

type foundryComplexRefactorSummary struct {
	SchemaVersion              string                       `json:"schema_version"`
	Status                     string                       `json:"status"`
	Mode                       string                       `json:"mode"`
	MutatesRepositories        bool                         `json:"mutates_repositories"`
	SchedulesWork              bool                         `json:"schedules_work"`
	ExecutesWork               bool                         `json:"executes_work"`
	ApprovesWork               bool                         `json:"approves_work"`
	CallsProviders             bool                         `json:"calls_providers"`
	NoDuplicatedStackFolders   bool                         `json:"no_duplicated_stack_folders"`
	TaskCounts                 complexRefactorTaskCounts    `json:"task_counts"`
	NextRecommendedFactoryTask complexRefactorFactoryTask   `json:"next_recommended_factory_task"`
	LoopDecision               complexRefactorLoopDecision  `json:"loop_decision"`
	RepairPlan                 complexRefactorRepairPlan    `json:"repair_plan"`
	ContextRepack              complexRefactorContextRepack `json:"context_repack"`
	SourceDigests              []complexRefactorSource      `json:"source_digests"`
	Artifacts                  map[string]string            `json:"artifacts"`
	BlockingNextActions        []string                     `json:"blocking_next_actions"`
	MaintenanceSuggestions     []string                     `json:"maintenance_suggestions"`
}

type complexRefactorTaskCounts struct {
	Total     int `json:"total"`
	Ready     int `json:"ready"`
	Blocked   int `json:"blocked"`
	Completed int `json:"completed"`
	Failed    int `json:"failed"`
}

type complexRefactorFactoryTask struct {
	NodeID            string `json:"node_id"`
	TaskID            string `json:"task_id"`
	TargetFactoryRepo string `json:"target_factory_repo"`
}

type complexRefactorLoopDecision struct {
	MayStartNextReadyTask    bool   `json:"may_start_next_ready_task"`
	MustNotStartBlockedTasks bool   `json:"must_not_start_blocked_tasks"`
	ReadyGateAction          string `json:"ready_gate_action"`
	BlockedBlueprintAction   string `json:"blocked_blueprint_action"`
	NextAction               string `json:"next_action"`
	FirstFailingCheck        string `json:"first_failing_check"`
	Why                      string `json:"why"`
}

type complexRefactorRepairPlan struct {
	Status        string `json:"status"`
	Path          string `json:"path"`
	RepairTaskID  string `json:"repair_task_id"`
	SchedulesWork bool   `json:"schedules_work"`
	ExecutesWork  bool   `json:"executes_work"`
	ApprovesWork  bool   `json:"approves_work"`
}

type complexRefactorContextRepack struct {
	Status               string `json:"status"`
	Path                 string `json:"path"`
	MissingContextReason string `json:"missing_context_reason"`
	SchedulesWork        bool   `json:"schedules_work"`
	ExecutesWork         bool   `json:"executes_work"`
	ApprovesWork         bool   `json:"approves_work"`
}

type complexRefactorSource struct {
	Name   string `json:"name"`
	Path   string `json:"path"`
	SHA256 string `json:"sha256"`
}

type complexRefactorStatusSummary struct {
	SchemaVersion              string                       `json:"schema_version"`
	CommandSchemaVersion       string                       `json:"command_schema_version"`
	Status                     string                       `json:"status"`
	Summary                    string                       `json:"summary"`
	Mode                       string                       `json:"mode"`
	NextAction                 string                       `json:"next_action"`
	NextRecommendedFactoryTask string                       `json:"next_recommended_factory_task"`
	NextRecommendedNode        string                       `json:"next_recommended_node"`
	TargetFactoryRepo          string                       `json:"target_factory_repo"`
	TaskCounts                 complexRefactorTaskCounts    `json:"task_counts"`
	RepairPlan                 complexRefactorRepairPlan    `json:"repair_plan"`
	ContextRepack              complexRefactorContextRepack `json:"context_repack"`
	FirstFailingCheck          string                       `json:"first_failing_check"`
	BlockingNextActions        []string                     `json:"blocking_next_actions"`
	MaintenanceSuggestions     []string                     `json:"maintenance_suggestions"`
	SourceDigests              []complexRefactorSource      `json:"source_digests"`
	Artifacts                  map[string]string            `json:"artifacts"`
	OperatorMode               string                       `json:"operator_mode"`
	MutatesRepositories        bool                         `json:"mutates_repositories"`
	SchedulesWork              bool                         `json:"schedules_work"`
	ExecutesWork               bool                         `json:"executes_work"`
	ApprovesWork               bool                         `json:"approves_work"`
	CallsProviders             bool                         `json:"calls_providers"`
}

type liveMutationArtifactSummary struct {
	Name              string `json:"name"`
	Path              string `json:"path"`
	SchemaVersion     string `json:"schema_version"`
	Status            string `json:"status"`
	SHA256            string `json:"sha256"`
	FirstFailingCheck string `json:"first_failing_check,omitempty"`
}

type liveMutationStatusSummary struct {
	SchemaVersion                  string                        `json:"schema_version"`
	CommandSchemaVersion           string                        `json:"command_schema_version"`
	Status                         string                        `json:"status"`
	AllowedNextAction              string                        `json:"allowed_next_action"`
	FirstFailingCheck              string                        `json:"first_failing_check"`
	KillSwitchState                string                        `json:"kill_switch_state"`
	Artifacts                      []liveMutationArtifactSummary `json:"artifacts"`
	BlockingNextActions            []string                      `json:"blocking_next_actions"`
	MaintenanceSuggestions         []string                      `json:"maintenance_suggestions"`
	CurrentMutationClass           string                        `json:"current_mutation_class,omitempty"`
	NextMutationClass              string                        `json:"next_mutation_class,omitempty"`
	HighestProvenLiveClass         string                        `json:"highest_proven_live_class,omitempty"`
	CurrentClassLiveEvidenceStatus string                        `json:"current_class_live_evidence_status,omitempty"`
	LowRiskCodeLiveEvidenceStatus  string                        `json:"low_risk_code_live_evidence_status,omitempty"`
	NextDeniedClass                string                        `json:"next_denied_class,omitempty"`
	NextDeniedReason               string                        `json:"next_denied_reason,omitempty"`
	SafeToRequest                  bool                          `json:"safe_to_request"`
	SafeToExecute                  bool                          `json:"safe_to_execute"`
	RequiredEvidence               []string                      `json:"required_evidence,omitempty"`
	LowRiskCodeDenialAudit         *lowRiskCodeDenialAudit       `json:"low_risk_code_denial_audit,omitempty"`
	SentinelHold                   *liveMutationSentinelHold     `json:"sentinel_hold,omitempty"`
	DeniedHigherClasses            map[string]string             `json:"denied_higher_classes,omitempty"`
	RepoStates                     []liveMutationRepoState       `json:"repo_states,omitempty"`
	OperatorMode                   string                        `json:"operator_mode"`
	MutatesRepositories            bool                          `json:"mutates_repositories"`
	SchedulesWork                  bool                          `json:"schedules_work"`
	ExecutesWork                   bool                          `json:"executes_work"`
	ApprovesWork                   bool                          `json:"approves_work"`
	CallsProviders                 bool                          `json:"calls_providers"`
	ReleaseOrPublishAllowed        bool                          `json:"release_or_publish_allowed"`
}

type liveMutationSentinelHold struct {
	Path                    string `json:"path"`
	SchemaVersion           string `json:"schema_version"`
	Status                  string `json:"status"`
	MutationClass           string `json:"mutation_class"`
	HoldRequired            bool   `json:"hold_required"`
	FirstFailingCheck       string `json:"first_failing_check"`
	ClassVerdictStatus      string `json:"class_verdict_status"`
	TestCoverageStatus      string `json:"test_coverage_status"`
	RollbackStatus          string `json:"rollback_status"`
	DiffSizeStatus          string `json:"diff_size_status"`
	FileClassStatus         string `json:"file_class_status"`
	EvidenceFreshnessStatus string `json:"evidence_freshness_status"`
	CIStatus                string `json:"ci_status"`
}

type lowRiskCodeDenialAudit struct {
	SchemaVersion                   string   `json:"schema_version"`
	Status                          string   `json:"status"`
	MutationClass                   string   `json:"mutation_class"`
	CurrentProvenLiveClass          string   `json:"current_proven_live_class"`
	NextDeniedClass                 string   `json:"next_denied_class"`
	SafeToRequest                   bool     `json:"safe_to_request"`
	SafeToExecute                   bool     `json:"safe_to_execute"`
	MissingPolicyEvidence           []string `json:"missing_policy_evidence"`
	MissingRollbackEvidence         []string `json:"missing_rollback_evidence"`
	MissingSentinelPromoterEvidence []string `json:"missing_sentinel_promoter_evidence"`
	SentinelState                   string   `json:"sentinel_state"`
	PromoterState                   string   `json:"promoter_state"`
	CIRequirements                  []string `json:"ci_requirements"`
	ExactNextAction                 string   `json:"exact_next_action"`
	DenialReason                    string   `json:"denial_reason"`
}

type liveMutationRepoState struct {
	Repo            string   `json:"repo"`
	Order           int      `json:"order"`
	PlannedPR       string   `json:"planned_pr"`
	Status          string   `json:"status"`
	ExecutionStatus string   `json:"execution_status"`
	RollbackScope   []string `json:"rollback_scope"`
	RollbackStatus  string   `json:"rollback_status"`
	DependsOn       []string `json:"depends_on"`
	MergeAfter      []string `json:"merge_after"`
}

type liveMutationApprovalSummary struct {
	SchemaVersion        string `json:"schema_version"`
	CommandSchemaVersion string `json:"command_schema_version"`
	Status               string `json:"status"`
	SafeToRequest        bool   `json:"safe_to_request"`
	SafeToExecute        bool   `json:"safe_to_execute"`
	ApprovalState        string `json:"approval_state"`
	RequestID            string `json:"request_id"`
	TicketID             string `json:"ticket_id"`
	FirstFailingCheck    string `json:"first_failing_check"`
	OperatorMode         string `json:"operator_mode"`
	MutatesRepositories  bool   `json:"mutates_repositories"`
	ApprovesWork         bool   `json:"approves_work"`
	ExecutesWork         bool   `json:"executes_work"`
	CallsProviders       bool   `json:"calls_providers"`
}

type liveMutationPRRehearsalSummary struct {
	SchemaVersion           string         `json:"schema_version"`
	CommandSchemaVersion    string         `json:"command_schema_version"`
	Status                  string         `json:"status"`
	Gate                    string         `json:"gate"`
	GateSchemaVersion       string         `json:"gate_schema_version"`
	FirstLiveClass          string         `json:"first_live_class"`
	SafeToRequest           bool           `json:"safe_to_request"`
	SafeToExecute           bool           `json:"safe_to_execute"`
	ExactNextStep           string         `json:"exact_next_step"`
	AllowedNextAction       string         `json:"allowed_next_action"`
	FirstFailingCheck       string         `json:"first_failing_check"`
	BlockingNextActions     []string       `json:"blocking_next_actions"`
	MaintenanceSuggestions  []string       `json:"maintenance_suggestions"`
	SourceHashes            []pulseSource  `json:"source_hashes"`
	OperatorMode            string         `json:"operator_mode"`
	MutatesRepositories     bool           `json:"mutates_repositories"`
	CreatesBranch           bool           `json:"creates_branch"`
	CreatesWorktree         bool           `json:"creates_worktree"`
	OpensPR                 bool           `json:"opens_pr"`
	MergesPR                bool           `json:"merges_pr"`
	SchedulesWork           bool           `json:"schedules_work"`
	ExecutesWork            bool           `json:"executes_work"`
	ApprovesWork            bool           `json:"approves_work"`
	CallsProviders          bool           `json:"calls_providers"`
	ReleaseOrPublishAllowed bool           `json:"release_or_publish_allowed"`
	RawGate                 map[string]any `json:"-"`
}

type rsiFamilyStatus struct {
	Family   string `json:"family"`
	Status   string `json:"status"`
	Passed   bool   `json:"passed"`
	Evidence string `json:"evidence"`
}

type rsiClaimLevel struct {
	Claim    string `json:"claim"`
	Decision string `json:"decision"`
	Status   string `json:"status"`
	Reason   string `json:"reason"`
}

type rsiManifestClaimLevel struct {
	ClaimLevel            string   `json:"claim_level"`
	Decision              string   `json:"decision"`
	Status                string   `json:"status"`
	RequiredEvidence      []string `json:"required_evidence,omitempty"`
	RequiredBeforeAllowed []string `json:"required_before_allowed,omitempty"`
}

type rsiManifestRepository struct {
	ID               string               `json:"id"`
	Role             string               `json:"role,omitempty"`
	Status           string               `json:"status,omitempty"`
	Replacement      *string              `json:"replacement,omitempty"`
	RSIEvidenceScope string               `json:"rsi_evidence_scope,omitempty"`
	Evidence         []string             `json:"evidence,omitempty"`
	KnownPRs         []rsiManifestKnownPR `json:"known_prs,omitempty"`
	ClaimOutput      []string             `json:"claim_output,omitempty"`
	Boundary         string               `json:"boundary,omitempty"`
}

type rsiManifestKnownPR struct {
	Number      int    `json:"number"`
	Title       string `json:"title"`
	URL         string `json:"url"`
	MergeCommit string `json:"merge_commit"`
}

type rsiArchitectureManifest struct {
	SchemaVersion                      string                  `json:"schema_version"`
	GeneratedDate                      string                  `json:"generated_date"`
	ClaimLevels                        []rsiManifestClaimLevel `json:"claim_levels"`
	ActiveRepositories                 []rsiManifestRepository `json:"active_repositories"`
	DeprecatedOrOutOfScopeRepositories []rsiManifestRepository `json:"deprecated_or_out_of_scope_repositories"`
	FullClaimRequiredEvidence          []string                `json:"full_claim_required_evidence"`
}

type rsiManifestSummary struct {
	SchemaVersion                      string                  `json:"schema_version"`
	CommandSchemaVersion               string                  `json:"command_schema_version"`
	Status                             string                  `json:"status"`
	Manifest                           string                  `json:"manifest"`
	ManifestSchemaVersion              string                  `json:"manifest_schema_version"`
	GeneratedDate                      string                  `json:"generated_date"`
	OperatorMode                       string                  `json:"operator_mode"`
	MutatesRepositories                bool                    `json:"mutates_repositories"`
	ClaimLevels                        []rsiManifestClaimLevel `json:"claim_levels"`
	ActiveRepositories                 []rsiManifestRepository `json:"active_repositories"`
	DeprecatedOrOutOfScopeRepositories []rsiManifestRepository `json:"deprecated_or_out_of_scope_repositories"`
	FullClaimRequiredEvidence          []string                `json:"full_claim_required_evidence"`
}

type rsiHealthSummary struct {
	SchemaVersion           string                         `json:"schema_version"`
	CommandSchemaVersion    string                         `json:"command_schema_version"`
	Status                  string                         `json:"status"`
	RSIMode                 string                         `json:"rsi_mode"`
	RSICapability           string                         `json:"rsi_capability"`
	OperatorMode            string                         `json:"operator_mode"`
	MutatesRepositories     bool                           `json:"mutates_repositories"`
	ClaimLevels             []rsiClaimLevel                `json:"claim_levels"`
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
	ClaimLevels             []rsiClaimLevel                `json:"claim_levels"`
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
	BaselineScorePercent       *float64               `json:"baseline_score_percent,omitempty"`
	CandidateScorePercent      *float64               `json:"candidate_score_percent,omitempty"`
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
	Summary           string                 `json:"summary"`
	CapturedOutputs   []forgeRetainedOutput  `json:"captured_outputs"`
	RetentionPolicy   forgeRetentionPolicy   `json:"retention_policy"`
	RetentionMetadata forgeRetentionMetadata `json:"retention_metadata"`
	SchemaValid       bool                   `json:"-"`
	SchemaError       string                 `json:"-"`
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
	Layout                                 string `json:"layout"`
	TemporaryPathsAllowed                  bool   `json:"temporary_paths_allowed"`
	MinimumRetentionDaysAfterTerminalPhase int    `json:"minimum_retention_days_after_terminal_phase"`
}

type forgeRetentionMetadata struct {
	RetainedAt             string   `json:"retained_at"`
	RetentionClass         string   `json:"retention_class"`
	RetainWhileGoalActive  bool     `json:"retain_while_goal_active"`
	DeletionRequiresReview bool     `json:"deletion_requires_review"`
	CleanupChangeMustName  []string `json:"cleanup_change_must_name"`
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

func readAtlasStatus(path string) (atlasStatusSummary, error) {
	var status foundryAtlasStatus
	bytes, err := os.ReadFile(path)
	if err != nil {
		return atlasStatusSummary{}, fmt.Errorf("read status: %w", err)
	}
	if err := json.Unmarshal(bytes, &status); err != nil {
		return atlasStatusSummary{}, fmt.Errorf("invalid status JSON: %w", err)
	}
	if err := validateFoundryAtlasStatus(status); err != nil {
		return atlasStatusSummary{}, err
	}
	return atlasStatusSummary{
		SchemaVersion:        "ao.command.atlas-status.v0.1",
		CommandSchemaVersion: commandSchemaVersion,
		Status:               status.Status,
		FoundryStatus:        path,
		Mode:                 status.Mode,
		RegistryID:           status.RegistryID,
		ImportID:             status.ImportID,
		WorkgraphID:          status.WorkgraphID,
		TargetInstance:       status.TargetInstance,
		ReadbackStatus:       status.ReadbackStatus,
		TaskID:               status.TaskID,
		TaskDigest:           status.TaskDigest,
		RunLinkDigest:        status.RunLinkDigest,
		OperatorMode:         operatorMode,
		OrchestrationOwner:   "ao-foundry",
		AtlasAuthority:       "compile_only",
		SchedulesWork:        status.SchedulesWork,
		ExecutesWork:         status.ExecutesWork,
		ApprovesWork:         status.ApprovesWork,
		MutatesRepositories:  false,
		Evidence:             status.Evidence,
		NextActions:          status.NextActions,
	}, nil
}

func readAtlasAuthorityLadderStatus(path string) (atlasAuthorityLadderSummary, error) {
	var status atlasMissionStatus
	if err := readPublicJSONFile(path, &status); err != nil {
		return atlasAuthorityLadderSummary{}, fmt.Errorf("read mission status: %w", err)
	}
	if err := validateAtlasAuthorityLadderStatus(status); err != nil {
		return atlasAuthorityLadderSummary{}, err
	}
	ladder := status.AuthorityLadder
	return atlasAuthorityLadderSummary{
		SchemaVersion:        "ao.command.atlas-authority-ladder.v0.1",
		CommandSchemaVersion: commandSchemaVersion,
		Status:               status.CompletionStatus,
		MissionStatus:        path,
		WorkgraphID:          status.WorkgraphID,
		TargetInstance:       status.TargetInstance,
		CurrentClass:         ladder.CurrentClass,
		NextClass:            ladder.NextClass,
		ProvenLiveClasses:    ladder.ProvenLiveClasses,
		DryRunReadyClasses:   ladder.DryRunReadyClasses,
		Blockers:             ladder.Blockers,
		RequiredEvidence:     ladder.RequiredEvidence,
		DeniedHigherClasses:  ladder.DeniedHigherClasses,
		DoNotAdvanceGates:    ladder.DoNotAdvanceGates,
		OperatorMode:         operatorMode,
		SchedulesWork:        status.SchedulesWork,
		ExecutesWork:         status.ExecutesWork,
		MutatesRepositories:  false,
	}, nil
}

func readPulseGateStatus(preflightPath, lifecyclePath, startGatePath string) (pulseGateStatusSummary, error) {
	var preflight foundryPulseIntakePreflight
	if err := readPublicJSONFile(preflightPath, &preflight); err != nil {
		return pulseGateStatusSummary{}, fmt.Errorf("read preflight: %w", err)
	}
	if err := validatePulseIntakePreflight(preflight); err != nil {
		return pulseGateStatusSummary{}, err
	}
	var lifecycle foundryPulsePRLifecycle
	if err := readPublicJSONFile(lifecyclePath, &lifecycle); err != nil {
		return pulseGateStatusSummary{}, fmt.Errorf("read lifecycle: %w", err)
	}
	if err := validatePulsePRLifecycle(lifecycle); err != nil {
		return pulseGateStatusSummary{}, err
	}
	var startGate foundryPulseOvernightStartGate
	if err := readPublicJSONFile(startGatePath, &startGate); err != nil {
		return pulseGateStatusSummary{}, fmt.Errorf("read start gate: %w", err)
	}
	if err := validatePulseOvernightStartGate(startGate); err != nil {
		return pulseGateStatusSummary{}, err
	}
	status := "ready"
	if preflight.Status == "failed" || startGate.Status == "failed" {
		status = "failed"
	} else if preflight.Status == "blocked" || startGate.Status == "blocked" || lifecycle.AllowedNextAction != "start_next_slice" {
		status = "blocked"
	}
	firstFailingCheck := firstNonEmpty(startGate.FirstFailingCheck, preflight.FirstFailingCheck)
	if status == "blocked" && firstFailingCheck == "" {
		firstFailingCheck = "pulse_gate"
	}
	blockingActions := append([]string{}, startGate.BlockingNextActions...)
	blockingActions = append(blockingActions, preflight.BlockingNextActions...)
	if lifecycle.BlockerReason != "" {
		blockingActions = append(blockingActions, lifecycle.BlockerReason)
	}
	maintenance := append([]string{}, startGate.MaintenanceSuggestions...)
	maintenance = append(maintenance, preflight.MaintenanceSuggestions...)
	return pulseGateStatusSummary{
		SchemaVersion:          "ao.command.pulse-gate-status.v0.1",
		CommandSchemaVersion:   commandSchemaVersion,
		Status:                 status,
		Preflight:              preflightPath,
		Lifecycle:              lifecyclePath,
		StartGate:              startGatePath,
		PreflightStatus:        preflight.Status,
		BlueprintStatus:        preflight.BlueprintStatus,
		AtlasStatus:            preflight.AtlasStatus,
		LifecycleStatus:        lifecycle.AllowedNextAction,
		StartGateStatus:        startGate.Status,
		AllowedNextAction:      startGate.AllowedNextAction,
		FirstFailingCheck:      firstFailingCheck,
		BlockingNextActions:    uniqueStrings(blockingActions),
		MaintenanceSuggestions: uniqueStrings(maintenance),
		SourceArtifacts:        preflight.SourceArtifacts,
		SourceHashes:           startGate.SourceHashes,
		OperatorMode:           operatorMode,
		MutatesRepositories:    false,
	}, nil
}

func readComplexRefactorStatus(summaryPath string) (complexRefactorStatusSummary, error) {
	var summary foundryComplexRefactorSummary
	if err := readPublicJSONFile(summaryPath, &summary); err != nil {
		return complexRefactorStatusSummary{}, fmt.Errorf("read summary: %w", err)
	}
	if err := validateComplexRefactorSummary(summary); err != nil {
		return complexRefactorStatusSummary{}, err
	}
	nextAction := summary.LoopDecision.NextAction
	if strings.TrimSpace(nextAction) == "" {
		nextAction = deriveComplexRefactorNextAction(summary)
	}
	firstFailingCheck := summary.LoopDecision.FirstFailingCheck
	if summary.Status != "ready" && strings.TrimSpace(firstFailingCheck) == "" {
		firstFailingCheck = "complex_refactor_rehearsal"
	}
	return complexRefactorStatusSummary{
		SchemaVersion:              "ao.command.complex-refactor-status.v0.1",
		CommandSchemaVersion:       commandSchemaVersion,
		Status:                     summary.Status,
		Summary:                    summaryPath,
		Mode:                       summary.Mode,
		NextAction:                 nextAction,
		NextRecommendedFactoryTask: summary.NextRecommendedFactoryTask.TaskID,
		NextRecommendedNode:        summary.NextRecommendedFactoryTask.NodeID,
		TargetFactoryRepo:          summary.NextRecommendedFactoryTask.TargetFactoryRepo,
		TaskCounts:                 summary.TaskCounts,
		RepairPlan:                 summary.RepairPlan,
		ContextRepack:              summary.ContextRepack,
		FirstFailingCheck:          firstFailingCheck,
		BlockingNextActions:        uniqueStrings(summary.BlockingNextActions),
		MaintenanceSuggestions:     uniqueStrings(summary.MaintenanceSuggestions),
		SourceDigests:              summary.SourceDigests,
		Artifacts:                  summary.Artifacts,
		OperatorMode:               operatorMode,
		MutatesRepositories:        false,
		SchedulesWork:              false,
		ExecutesWork:               false,
		ApprovesWork:               false,
		CallsProviders:             false,
	}, nil
}

func readLiveMutationStatus(authorityPath, requestPath, forgePlanPath, ao2PacketPath, isolationPath, rollbackPath, killSwitchPath, sentinelHoldPath string) (liveMutationStatusSummary, error) {
	specs := []struct {
		name        string
		path        string
		schema      string
		allowPassed bool
	}{
		{name: "covenant_authority", path: authorityPath, schema: "covenant.live-mutation-authority.v1"},
		{name: "foundry_request", path: requestPath, schema: "ao.foundry.live-mutation-request.v0.1"},
		{name: "forge_dry_run_plan", path: forgePlanPath, schema: "ao.forge.live-mutation-dry-run-plan.v0.1"},
		{name: "ao2_dry_run_packet", path: ao2PacketPath, schema: "ao2.live-mutation-dry-run-packet.v1"},
		{name: "worktree_isolation", path: isolationPath, schema: "ao.foundry.worktree-isolation-proof.v0.1"},
		{name: "rollback_rehearsal", path: rollbackPath, schema: "ao.foundry.live-mutation-rollback-rehearsal.v0.1"},
		{name: "operator_kill_switch", path: killSwitchPath, schema: "ao.command.live-mutation-kill-switch.v0.1"},
	}

	status := "ready"
	firstFailingCheck := ""
	killSwitchState := ""
	currentMutationClass := ""
	nextMutationClass := ""
	artifacts := []liveMutationArtifactSummary{}
	rawArtifacts := []map[string]any{}
	repoStates := []liveMutationRepoState{}
	var sentinelHold *liveMutationSentinelHold
	blockingActions := []string{}
	maintenance := []string{
		"Keep this readback observer-only; it does not grant live mutation authority.",
		"Do not request a live mutation class until Sentinel and Promoter evidence also pass.",
	}

	for _, spec := range specs {
		artifact, raw, err := readLiveMutationArtifact(spec.name, spec.path, spec.schema)
		if err != nil {
			return liveMutationStatusSummary{}, err
		}
		rawArtifacts = append(rawArtifacts, raw)
		if spec.name == "operator_kill_switch" {
			killSwitchState = artifact.Status
		}
		artifacts = append(artifacts, artifact)
		if err := validateLiveMutationArtifactBoundaries(spec.name, raw); err != nil {
			return liveMutationStatusSummary{}, err
		}
		if artifactCurrentClass := liveMutationMapString(raw, "current_mutation_class"); artifactCurrentClass != "" {
			if currentMutationClass != "" && currentMutationClass != artifactCurrentClass {
				status = "blocked"
				if firstFailingCheck == "" {
					firstFailingCheck = spec.name
				}
				blockingActions = append(blockingActions, "Repair inconsistent current mutation class evidence before proceeding.")
			} else {
				currentMutationClass = artifactCurrentClass
			}
		}
		if artifactNextClass := liveMutationMapString(raw, "next_mutation_class"); artifactNextClass != "" {
			if nextMutationClass != "" && nextMutationClass != artifactNextClass {
				status = "blocked"
				if firstFailingCheck == "" {
					firstFailingCheck = spec.name
				}
				blockingActions = append(blockingActions, "Repair inconsistent next mutation class evidence before proceeding.")
			} else {
				nextMutationClass = artifactNextClass
			}
		}
		if spec.name == "foundry_request" {
			repoStates = liveMutationRepoStates(raw)
			if blocker := validateLiveMutationRepoStates(raw, repoStates); blocker != "" {
				if status == "ready" {
					status = "blocked"
				}
				if firstFailingCheck == "" {
					firstFailingCheck = "foundry_request"
				}
				blockingActions = append(blockingActions, blocker)
			}
		}
		switch artifact.Status {
		case "ready", "approved", "armed":
		case "passed":
			if !spec.allowPassed {
				status = "blocked"
				if firstFailingCheck == "" {
					firstFailingCheck = spec.name
				}
			}
		case "blocked", "hold":
			if status == "ready" {
				status = "blocked"
			}
			if firstFailingCheck == "" {
				firstFailingCheck = firstNonEmpty(artifact.FirstFailingCheck, spec.name)
			}
		case "failed", "denied":
			status = "failed"
			if firstFailingCheck == "" {
				firstFailingCheck = firstNonEmpty(artifact.FirstFailingCheck, spec.name)
			}
		default:
			if status == "ready" {
				status = "blocked"
			}
			if firstFailingCheck == "" {
				firstFailingCheck = spec.name
			}
		}
		blockingActions = append(blockingActions, liveMutationStringSlice(raw, "blocking_next_actions")...)
		maintenance = append(maintenance, liveMutationStringSlice(raw, "maintenance_suggestions")...)
	}

	if killSwitchState != "armed" {
		if status == "ready" {
			status = "blocked"
		}
		if firstFailingCheck == "" {
			firstFailingCheck = "operator_kill_switch"
		}
		blockingActions = append(blockingActions, "Arm the operator kill switch before requesting live mutation authority.")
	}
	if sentinelHoldPath != "" {
		hold, artifact, err := readLiveMutationSentinelHold(sentinelHoldPath)
		if err != nil {
			return liveMutationStatusSummary{}, err
		}
		sentinelHold = hold
		artifacts = append(artifacts, artifact)
		if hold.MutationClass != "" && nextMutationClass != "" && hold.MutationClass != nextMutationClass {
			if status == "ready" {
				status = "blocked"
			}
			if firstFailingCheck == "" {
				firstFailingCheck = "sentinel_hold"
			}
			blockingActions = append(blockingActions, "Repair Sentinel hold mutation_class mismatch before proceeding.")
		}
		if hold.Status != "clear" || hold.HoldRequired {
			if status == "ready" {
				status = "blocked"
			}
			if firstFailingCheck == "" {
				firstFailingCheck = firstNonEmpty(hold.FirstFailingCheck, "sentinel_hold")
			}
			blockingActions = append(blockingActions, "Resolve AO Sentinel live-mutation hold before requesting the next class.")
		}
	}
	allowedNextAction := "request_first_tiny_live_mutation_class"
	if status == "blocked" {
		allowedNextAction = "repair_live_mutation_evidence"
	} else if status == "failed" {
		allowedNextAction = "stop_and_rebuild_live_mutation_evidence"
	} else if nextMutationClass == "test_only" {
		allowedNextAction = "request_test_only_live_rehearsal"
	} else if nextMutationClass == "low_risk_code" {
		allowedNextAction = "request_low_risk_code_dry_run"
	} else if nextMutationClass == "multi_repo_low_risk" {
		allowedNextAction = "request_multi_repo_low_risk_dry_run"
	}
	if status != "ready" && len(blockingActions) == 0 {
		blockingActions = append(blockingActions, "Repair the first failing live-mutation evidence check before proceeding.")
	}
	requiredEvidence := liveMutationRequiredEvidence(nextMutationClass)
	deniedHigherClasses := liveMutationDeniedHigherClasses(nextMutationClass)
	highestProvenLiveClass, currentClassLiveEvidenceStatus := liveMutationHighestProvenLiveClass(currentMutationClass, rawArtifacts)
	lowRiskCodeLiveEvidenceStatus := ""
	nextDeniedClass := ""
	nextDeniedReason := ""
	if currentMutationClass == "low_risk_code" || nextMutationClass == "multi_repo_low_risk" {
		lowRiskCodeLiveEvidenceStatus = currentClassLiveEvidenceStatus
		if lowRiskCodeLiveEvidenceStatus != "completed" {
			nextDeniedClass = "multi_repo_low_risk"
			nextDeniedReason = "denied until low_risk_code live rehearsal evidence is recorded"
		}
	}
	var lowRiskDenialAudit *lowRiskCodeDenialAudit
	if nextMutationClass == "low_risk_code" {
		lowRiskDenialAudit = newLowRiskCodeDenialAudit(status == "ready")
	}

	return liveMutationStatusSummary{
		SchemaVersion:                  "ao.command.live-mutation-status.v0.1",
		CommandSchemaVersion:           commandSchemaVersion,
		Status:                         status,
		AllowedNextAction:              allowedNextAction,
		FirstFailingCheck:              firstFailingCheck,
		KillSwitchState:                killSwitchState,
		Artifacts:                      artifacts,
		BlockingNextActions:            uniqueStrings(blockingActions),
		MaintenanceSuggestions:         uniqueStrings(maintenance),
		CurrentMutationClass:           currentMutationClass,
		NextMutationClass:              nextMutationClass,
		HighestProvenLiveClass:         highestProvenLiveClass,
		CurrentClassLiveEvidenceStatus: currentClassLiveEvidenceStatus,
		LowRiskCodeLiveEvidenceStatus:  lowRiskCodeLiveEvidenceStatus,
		NextDeniedClass:                nextDeniedClass,
		NextDeniedReason:               nextDeniedReason,
		SafeToRequest:                  status == "ready",
		SafeToExecute:                  false,
		RequiredEvidence:               requiredEvidence,
		LowRiskCodeDenialAudit:         lowRiskDenialAudit,
		SentinelHold:                   sentinelHold,
		DeniedHigherClasses:            deniedHigherClasses,
		RepoStates:                     repoStates,
		OperatorMode:                   operatorMode,
		MutatesRepositories:            false,
		SchedulesWork:                  false,
		ExecutesWork:                   false,
		ApprovesWork:                   false,
		CallsProviders:                 false,
		ReleaseOrPublishAllowed:        false,
	}, nil
}

func newLowRiskCodeDenialAudit(safeToRequest bool) *lowRiskCodeDenialAudit {
	return &lowRiskCodeDenialAudit{
		SchemaVersion:          "ao.command.low-risk-code-denial-audit.v0.1",
		Status:                 "blocked",
		MutationClass:          "low_risk_code",
		CurrentProvenLiveClass: "test_only",
		NextDeniedClass:        "low_risk_code",
		SafeToRequest:          safeToRequest,
		SafeToExecute:          false,
		MissingPolicyEvidence: []string{
			"policy:low_risk_code_live_promotion",
			"command_readback:low_risk_code_live",
		},
		MissingRollbackEvidence: []string{
			"rollback_proof:low_risk_code_live",
		},
		MissingSentinelPromoterEvidence: []string{
			"sentinel_clear:low_risk_code_live",
			"promoter_promotion:low_risk_code_live",
		},
		SentinelState:   "missing_live_no_hold",
		PromoterState:   "missing_live_promotion",
		CIRequirements:  []string{"ci_passed:low_risk_code_live"},
		ExactNextAction: "build_low_risk_code_promotion_prerequisites",
		DenialReason:    "low_risk_code live execution remains denied until policy promotion, rollback proof, Sentinel clear verdict, Promoter promotion, Command readback, and PR CI evidence all exist for the exact class scope.",
	}
}

func liveMutationHighestProvenLiveClass(currentClass string, artifacts []map[string]any) (string, string) {
	if currentClass == "" {
		return "", ""
	}
	for _, artifact := range artifacts {
		rehearsal, _ := artifact["completed_live_rehearsal"].(map[string]any)
		if liveMutationMapString(rehearsal, "status") == "completed" &&
			liveMutationMapString(rehearsal, "mutation_class") == currentClass {
			return currentClass, "completed"
		}
	}
	if currentClass == "low_risk_code" {
		return previousLiveMutationClass(currentClass), "missing"
	}
	return currentClass, "completed"
}

func previousLiveMutationClass(class string) string {
	switch class {
	case "docs_only_multi_file":
		return "docs_only_single_file"
	case "test_only":
		return "docs_only_multi_file"
	case "low_risk_code":
		return "test_only"
	case "multi_repo_low_risk":
		return "low_risk_code"
	case "complex_repo_mutation":
		return "multi_repo_low_risk"
	default:
		return ""
	}
}

func readLiveMutationSentinelHold(path string) (*liveMutationSentinelHold, liveMutationArtifactSummary, error) {
	artifact, raw, err := readLiveMutationArtifact("sentinel_hold", path, "ao.sentinel.live-mutation-hold.v0.1")
	if err != nil {
		return nil, liveMutationArtifactSummary{}, err
	}
	if err := validateLiveMutationArtifactBoundaries("sentinel_hold", raw); err != nil {
		return nil, liveMutationArtifactSummary{}, err
	}
	status := liveMutationMapString(raw, "status")
	if status != "clear" && status != "hold" {
		return nil, liveMutationArtifactSummary{}, fmt.Errorf("sentinel_hold status must be clear or hold, got %q", status)
	}
	classVerdict, _ := raw["class_hold_verdict"].(map[string]any)
	hold := &liveMutationSentinelHold{
		Path:                    path,
		SchemaVersion:           artifact.SchemaVersion,
		Status:                  status,
		MutationClass:           liveMutationMapString(raw, "mutation_class"),
		HoldRequired:            liveMutationMapBool(raw, "hold_required"),
		FirstFailingCheck:       liveMutationMapString(raw, "first_failing_check"),
		ClassVerdictStatus:      liveMutationMapString(classVerdict, "status"),
		TestCoverageStatus:      liveMutationMapString(classVerdict, "test_coverage_status"),
		RollbackStatus:          liveMutationMapString(classVerdict, "rollback_status"),
		DiffSizeStatus:          liveMutationMapString(classVerdict, "diff_size_status"),
		FileClassStatus:         liveMutationMapString(classVerdict, "file_class_status"),
		EvidenceFreshnessStatus: liveMutationMapString(classVerdict, "evidence_freshness_status"),
		CIStatus:                liveMutationMapString(classVerdict, "ci_status"),
	}
	if hold.MutationClass == "" || hold.ClassVerdictStatus == "" {
		return nil, liveMutationArtifactSummary{}, errors.New("sentinel_hold requires mutation_class and class_hold_verdict.status")
	}
	return hold, artifact, nil
}

func liveMutationRequiredEvidence(nextClass string) []string {
	switch nextClass {
	case "test_only":
		return []string{
			"covenant_class_ticket:test_only",
			"foundry_class_gate:test_only",
			"ao2_bounded_patch_packet:test_only",
			"sentinel_no_hold:test_only",
			"promoter_ready:test_only",
			"rollback_proof:test_only",
			"ci_passed:test_only",
		}
	case "low_risk_code":
		return []string{
			"test_only_success",
			"covenant_class_ticket:low_risk_code",
			"foundry_class_gate:low_risk_code",
			"ao2_dry_run_packet:low_risk_code",
			"sentinel_no_hold:low_risk_code",
			"promoter_ready:low_risk_code",
			"rollback_proof:low_risk_code",
			"ci_passed:low_risk_code",
		}
	case "multi_repo_low_risk":
		return []string{
			"low_risk_code_live_success",
			"covenant_class_ticket:multi_repo_low_risk",
			"foundry_class_gate:multi_repo_low_risk",
			"multi_repo_sequencing_plan",
			"per_repo_rollback:ao-atlas",
			"per_repo_rollback:ao-foundry",
			"per_repo_rollback:ao-command",
			"prevent_concurrent_unsafe_execution",
			"sentinel_no_hold:multi_repo_low_risk",
			"promoter_ready:multi_repo_low_risk",
			"ci_passed:multi_repo_low_risk",
		}
	default:
		return nil
	}
}

func liveMutationDeniedHigherClasses(nextClass string) map[string]string {
	switch nextClass {
	case "test_only":
		reason := "denied until test_only live rehearsal, rollback proof, CI, Sentinel, Promoter, and Command evidence complete"
		return map[string]string{
			"low_risk_code":          reason,
			"multi_repo_low_risk":    reason,
			"complex_repo_mutation":  reason,
			"fully_unsupervised_rsi": "denied until every governed lower mutation class has completed live evidence and no active holds",
		}
	case "low_risk_code":
		reason := "denied until low_risk_code dry-run is promoted, live rehearsal evidence exists, rollback proof and CI pass, and no holds remain"
		return map[string]string{
			"multi_repo_low_risk":    reason,
			"complex_repo_mutation":  reason,
			"fully_unsupervised_rsi": "denied until every governed lower mutation class has completed live evidence and no active holds",
		}
	case "multi_repo_low_risk":
		reason := "denied until multi_repo_low_risk live rehearsal evidence exists, per-repo rollback proof and CI pass, and no holds remain"
		return map[string]string{
			"complex_repo_mutation":  reason,
			"fully_unsupervised_rsi": "denied until every governed lower mutation class has completed live evidence and no active holds",
		}
	default:
		return nil
	}
}

func readLiveMutationPRRehearsal(gatePath string) (liveMutationPRRehearsalSummary, error) {
	var gate struct {
		SchemaVersion          string         `json:"schema_version"`
		Status                 string         `json:"status"`
		FirstLiveClass         string         `json:"first_live_class"`
		SafeToRequest          bool           `json:"safe_to_request"`
		SafeToExecute          bool           `json:"safe_to_execute"`
		ExactNextStep          string         `json:"exact_next_step"`
		AllowedNextAction      string         `json:"allowed_next_action"`
		FirstFailingCheck      string         `json:"first_failing_check"`
		BlockingNextActions    []string       `json:"blocking_next_actions"`
		MaintenanceSuggestions []string       `json:"maintenance_suggestions"`
		SourceHashes           []pulseSource  `json:"source_hashes"`
		AuthorityBoundaries    map[string]any `json:"authority_boundaries"`
	}
	if err := readPublicJSONFile(gatePath, &gate); err != nil {
		return liveMutationPRRehearsalSummary{}, fmt.Errorf("read gate: %w", err)
	}
	if gate.SchemaVersion != "ao.foundry.live-docs-pr-rehearsal-gate.v0.1" {
		return liveMutationPRRehearsalSummary{}, errors.New("gate schema_version must be ao.foundry.live-docs-pr-rehearsal-gate.v0.1")
	}
	if gate.Status != "ready" && gate.Status != "blocked" {
		return liveMutationPRRehearsalSummary{}, fmt.Errorf("gate status must be ready or blocked, got %q", gate.Status)
	}
	if gate.FirstLiveClass != "docs_only" {
		return liveMutationPRRehearsalSummary{}, errors.New("gate first_live_class must be docs_only")
	}
	if !gate.SafeToRequest {
		return liveMutationPRRehearsalSummary{}, errors.New("gate safe_to_request must be true")
	}
	if len(gate.SourceHashes) == 0 {
		return liveMutationPRRehearsalSummary{}, errors.New("gate requires source_hashes")
	}
	for _, source := range gate.SourceHashes {
		if source.Name == "" || source.Path == "" || source.SchemaVersion == "" || len(source.SHA256) != 64 {
			return liveMutationPRRehearsalSummary{}, errors.New("gate source_hashes must include name, path, schema_version, and sha256")
		}
	}
	if gate.Status == "ready" {
		if !gate.SafeToExecute || gate.ExactNextStep != "start_first_docs_only_live_pr_rehearsal" || gate.FirstFailingCheck != "" {
			return liveMutationPRRehearsalSummary{}, errors.New("ready gate must allow only start_first_docs_only_live_pr_rehearsal with no failing check")
		}
	} else {
		if gate.SafeToExecute || gate.FirstFailingCheck == "" {
			return liveMutationPRRehearsalSummary{}, errors.New("blocked gate must keep safe_to_execute=false and report first_failing_check")
		}
	}
	if gate.AuthorityBoundaries == nil {
		return liveMutationPRRehearsalSummary{}, errors.New("gate authority_boundaries are required")
	}
	if !liveMutationMapBool(gate.AuthorityBoundaries, "emits_decision_only") {
		return liveMutationPRRehearsalSummary{}, errors.New("gate must emit decisions only")
	}
	if liveMutationMapString(gate.AuthorityBoundaries, "first_live_class") != "docs_only" {
		return liveMutationPRRehearsalSummary{}, errors.New("gate authority_boundaries.first_live_class must be docs_only")
	}
	for _, field := range []string{
		"broad_live_mutation_allowed",
		"fully_unsupervised_complex_mutation_claimed",
		"mutates_repositories",
		"creates_branch",
		"creates_worktree",
		"opens_pr",
		"merges_pr",
		"schedules_work",
		"executes_work",
		"approves_work",
		"provider_calls_allowed",
		"release_or_publish_allowed",
	} {
		if liveMutationMapBool(gate.AuthorityBoundaries, field) {
			return liveMutationPRRehearsalSummary{}, fmt.Errorf("gate expands forbidden authority via authority_boundaries.%s", field)
		}
	}
	return liveMutationPRRehearsalSummary{
		SchemaVersion:           "ao.command.live-docs-pr-rehearsal-status.v0.1",
		CommandSchemaVersion:    commandSchemaVersion,
		Status:                  gate.Status,
		Gate:                    gatePath,
		GateSchemaVersion:       gate.SchemaVersion,
		FirstLiveClass:          gate.FirstLiveClass,
		SafeToRequest:           gate.SafeToRequest,
		SafeToExecute:           gate.SafeToExecute,
		ExactNextStep:           gate.ExactNextStep,
		AllowedNextAction:       gate.AllowedNextAction,
		FirstFailingCheck:       gate.FirstFailingCheck,
		BlockingNextActions:     uniqueStrings(gate.BlockingNextActions),
		MaintenanceSuggestions:  uniqueStrings(gate.MaintenanceSuggestions),
		SourceHashes:            gate.SourceHashes,
		OperatorMode:            operatorMode,
		MutatesRepositories:     false,
		CreatesBranch:           false,
		CreatesWorktree:         false,
		OpensPR:                 false,
		MergesPR:                false,
		SchedulesWork:           false,
		ExecutesWork:            false,
		ApprovesWork:            false,
		CallsProviders:          false,
		ReleaseOrPublishAllowed: false,
	}, nil
}

func readLiveMutationApproval(requestPath, ticketPath string) (liveMutationApprovalSummary, error) {
	var request map[string]any
	var ticket map[string]any
	if err := readJSONFile(requestPath, &request); err != nil {
		return liveMutationApprovalSummary{}, fmt.Errorf("read request: %w", err)
	}
	if err := readJSONFile(ticketPath, &ticket); err != nil {
		return liveMutationApprovalSummary{}, fmt.Errorf("read ticket: %w", err)
	}
	if err := validatePublicSafeText(requestPath); err != nil {
		return liveMutationApprovalSummary{}, err
	}
	if err := validatePublicSafeText(ticketPath); err != nil {
		return liveMutationApprovalSummary{}, err
	}
	if liveMutationMapString(request, "schema_version") != "ao.foundry.live-mutation-approval-request.v0.1" {
		return liveMutationApprovalSummary{}, errors.New("request schema_version must be ao.foundry.live-mutation-approval-request.v0.1")
	}
	if liveMutationMapString(ticket, "schema_version") != "covenant.live-docs-approval-ticket.v1" {
		return liveMutationApprovalSummary{}, errors.New("ticket schema_version must be covenant.live-docs-approval-ticket.v1")
	}
	summary := liveMutationApprovalSummary{
		SchemaVersion:        "ao.command.live-mutation-approval-status.v0.1",
		CommandSchemaVersion: commandSchemaVersion,
		Status:               "blocked",
		SafeToRequest:        liveMutationMapBool(request, "safe_to_request"),
		SafeToExecute:        false,
		ApprovalState:        liveMutationMapString(ticket, "approval_state"),
		RequestID:            liveMutationMapString(ticket, "request_id"),
		TicketID:             liveMutationMapString(ticket, "ticket_id"),
		FirstFailingCheck:    "",
		OperatorMode:         operatorMode,
		MutatesRepositories:  false,
		ApprovesWork:         false,
		ExecutesWork:         false,
		CallsProviders:       false,
	}
	if summary.RequestID != liveMutationMapString(request, "request_id") {
		summary.FirstFailingCheck = "request_id_mismatch"
		return summary, nil
	}
	if summary.ApprovalState != "approved" {
		summary.FirstFailingCheck = "approval_state"
		return summary, nil
	}
	if liveMutationMapBool(ticket, "consumed") {
		summary.FirstFailingCheck = "ticket_consumed"
		return summary, nil
	}
	expiresAt, err := time.Parse(time.RFC3339, liveMutationMapString(ticket, "expires_at"))
	if err != nil {
		return liveMutationApprovalSummary{}, fmt.Errorf("ticket expires_at must be RFC3339: %w", err)
	}
	if !expiresAt.After(time.Now().UTC()) {
		summary.FirstFailingCheck = "ticket_expired"
		return summary, nil
	}
	scope, ok := ticket["approved_scope"].(map[string]any)
	if !ok {
		return liveMutationApprovalSummary{}, errors.New("ticket approved_scope is required")
	}
	for _, field := range []string{"repo", "branch_policy", "docs_only_path_allowlist", "forbidden_paths", "max_changed_files"} {
		if !jsonEquivalent(scope[field], request[field]) {
			summary.FirstFailingCheck = "scope_mismatch"
			return summary, nil
		}
	}
	summary.Status = "approved"
	summary.SafeToExecute = true
	return summary, nil
}

func readLiveMutationArtifact(name, path, expectedSchema string) (liveMutationArtifactSummary, map[string]any, error) {
	var raw map[string]any
	if err := readPublicJSONFile(path, &raw); err != nil {
		return liveMutationArtifactSummary{}, nil, fmt.Errorf("read %s: %w", name, err)
	}
	schema := liveMutationMapString(raw, "schema_version")
	if schema != expectedSchema {
		return liveMutationArtifactSummary{}, nil, fmt.Errorf("%s has invalid schema_version %q", name, schema)
	}
	status := liveMutationMapString(raw, "status")
	if name == "operator_kill_switch" {
		status = liveMutationMapString(raw, "state")
	}
	if strings.TrimSpace(status) == "" {
		return liveMutationArtifactSummary{}, nil, fmt.Errorf("%s requires status", name)
	}
	sha, err := sha256File(path)
	if err != nil {
		return liveMutationArtifactSummary{}, nil, fmt.Errorf("hash %s: %w", name, err)
	}
	return liveMutationArtifactSummary{
		Name:              name,
		Path:              path,
		SchemaVersion:     schema,
		Status:            status,
		SHA256:            sha,
		FirstFailingCheck: liveMutationMapString(raw, "first_failing_check"),
	}, raw, nil
}

func validateLiveMutationArtifactBoundaries(name string, raw map[string]any) error {
	if err := validateLiveMutationMode(name, raw); err != nil {
		return err
	}
	for _, field := range []string{
		"mutates_live_state",
		"mutates_repositories",
		"schedules_work",
		"executes_work",
		"approves_work",
		"calls_providers",
		"provider_calls_allowed",
		"release_or_publish_allowed",
		"uploads_artifacts",
		"live_mutation_allowed",
	} {
		if liveMutationMapBool(raw, field) {
			return fmt.Errorf("%s expands forbidden authority via %s", name, field)
		}
	}
	if boundaries, ok := raw["authority_boundaries"].(map[string]any); ok {
		if !liveMutationMapBool(boundaries, "dry_run_only") {
			return fmt.Errorf("%s authority_boundaries must remain dry_run_only", name)
		}
		for _, field := range []string{
			"live_mutation_allowed",
			"mutates_repositories",
			"schedules_work",
			"executes_work",
			"approves_work",
			"calls_providers",
			"provider_calls_allowed",
			"release_or_publish_allowed",
			"sibling_repo_mutation_allowed",
		} {
			if liveMutationMapBool(boundaries, field) {
				return fmt.Errorf("%s expands forbidden authority via authority_boundaries.%s", name, field)
			}
		}
	}
	return nil
}

func validateLiveMutationMode(name string, raw map[string]any) error {
	mode := liveMutationMapString(raw, "mode")
	if mode == "" {
		return nil
	}
	switch mode {
	case "dry_run_only", "dry_run_packet", "fixture_only", "fixture_only_rehearsal":
		return nil
	default:
		return fmt.Errorf("%s has unsafe mode %q", name, mode)
	}
}

func liveMutationMapString(raw map[string]any, key string) string {
	if value, ok := raw[key].(string); ok {
		return value
	}
	return ""
}

func liveMutationMapBool(raw map[string]any, key string) bool {
	if value, ok := raw[key].(bool); ok {
		return value
	}
	return false
}

func jsonEquivalent(left any, right any) bool {
	leftBytes, leftErr := json.Marshal(left)
	rightBytes, rightErr := json.Marshal(right)
	return leftErr == nil && rightErr == nil && string(leftBytes) == string(rightBytes)
}

func liveMutationStringSlice(raw map[string]any, key string) []string {
	values, ok := raw[key].([]any)
	if !ok {
		return nil
	}
	result := []string{}
	for _, value := range values {
		if text, ok := value.(string); ok && strings.TrimSpace(text) != "" {
			result = append(result, text)
		}
	}
	return result
}

func liveMutationRepoStates(raw map[string]any) []liveMutationRepoState {
	values, ok := raw["repo_states"].([]any)
	if !ok {
		return nil
	}
	states := []liveMutationRepoState{}
	for _, value := range values {
		object, ok := value.(map[string]any)
		if !ok {
			continue
		}
		states = append(states, liveMutationRepoState{
			Repo:            liveMutationMapString(object, "repo"),
			Order:           liveMutationMapInt(object, "order"),
			PlannedPR:       liveMutationMapString(object, "planned_pr"),
			Status:          liveMutationMapString(object, "status"),
			ExecutionStatus: liveMutationMapString(object, "execution_status"),
			RollbackScope:   liveMutationStringSlice(object, "rollback_scope"),
			RollbackStatus:  liveMutationMapString(object, "rollback_status"),
			DependsOn:       liveMutationStringSlice(object, "depends_on"),
			MergeAfter:      liveMutationStringSlice(object, "merge_after"),
		})
	}
	return states
}

func validateLiveMutationRepoStates(raw map[string]any, states []liveMutationRepoState) string {
	if len(states) == 0 {
		return ""
	}
	policy, _ := raw["concurrency_policy"].(map[string]any)
	if liveMutationMapBool(policy, "concurrent_execution_allowed") ||
		liveMutationMapInt(policy, "max_active_repos") != 1 ||
		!liveMutationMapBool(policy, "required_serialized_dependency_order") {
		return "Repair multi_repo_low_risk dry-run evidence: unsafe concurrent execution is not allowed."
	}
	seen := map[string]bool{}
	readyToExecute := 0
	for i, state := range states {
		switch {
		case state.Repo == "":
			return "Repair multi_repo_low_risk dry-run evidence: repo_state missing repo."
		case seen[state.Repo]:
			return "Repair multi_repo_low_risk dry-run evidence: duplicate repo_state."
		case state.Order != i+1:
			return "Repair multi_repo_low_risk dry-run evidence: repo_state dependency order is invalid."
		case state.PlannedPR == "":
			return "Repair multi_repo_low_risk dry-run evidence: planned PR dependency is missing."
		case state.Status != "ready":
			return "Repair multi_repo_low_risk dry-run evidence: repo_state is not ready."
		case state.ExecutionStatus == "executing" || state.ExecutionStatus == "active":
			return "Repair multi_repo_low_risk dry-run evidence: unsafe concurrent execution is not allowed."
		case state.ExecutionStatus == "ready_to_execute":
			readyToExecute++
		case state.ExecutionStatus != "sequenced_dry_run_only":
			return "Repair multi_repo_low_risk dry-run evidence: repo_state is not sequenced dry-run only."
		case len(state.RollbackScope) == 0 || state.RollbackStatus != "ready":
			return "Repair multi_repo_low_risk dry-run evidence: per-repo rollback is not ready."
		}
		if !equalStringSlices(state.DependsOn, state.MergeAfter) {
			return "Repair multi_repo_low_risk dry-run evidence: merge_after must match depends_on."
		}
		for _, dependency := range state.DependsOn {
			if !seen[dependency] {
				return "Repair multi_repo_low_risk dry-run evidence: repo dependency must appear earlier in dependency order."
			}
		}
		seen[state.Repo] = true
	}
	if len(states) < 2 {
		return "Repair multi_repo_low_risk dry-run evidence: at least two repos are required."
	}
	if readyToExecute > 1 {
		return "Repair multi_repo_low_risk dry-run evidence: unsafe concurrent execution is not allowed."
	}
	return ""
}

func liveMutationMapInt(raw map[string]any, key string) int {
	switch value := raw[key].(type) {
	case float64:
		return int(value)
	case int:
		return value
	case json.Number:
		parsed, err := value.Int64()
		if err != nil {
			return 0
		}
		return int(parsed)
	default:
		return 0
	}
}

func deriveComplexRefactorNextAction(summary foundryComplexRefactorSummary) string {
	if summary.Status == "ready" && summary.LoopDecision.MayStartNextReadyTask {
		return "start_next_ready_task"
	}
	if summary.Status == "blocked" {
		return "repair_blocked_nodes"
	}
	return "stop_blocked"
}

func readPublicJSONFile(path string, target any) error {
	bytes, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	if err := validatePublicSafeText(string(bytes)); err != nil {
		return err
	}
	if err := json.Unmarshal(bytes, target); err != nil {
		return fmt.Errorf("invalid JSON: %w", err)
	}
	return nil
}

func validateComplexRefactorSummary(summary foundryComplexRefactorSummary) error {
	if summary.SchemaVersion != "ao.foundry.complex-refactor-workgraph-rehearsal.v0.1" {
		return errors.New("invalid complex-refactor rehearsal schema_version")
	}
	if !isPulseStatus(summary.Status) {
		return fmt.Errorf("invalid complex-refactor rehearsal status %q", summary.Status)
	}
	if summary.Mode != "fixture_only_rehearsal" {
		return errors.New("complex-refactor rehearsal mode must be fixture_only_rehearsal")
	}
	if summary.MutatesRepositories || summary.SchedulesWork || summary.ExecutesWork || summary.ApprovesWork || summary.CallsProviders {
		return errors.New("complex-refactor rehearsal must remain read-only and cannot schedule, execute, approve, call providers, or mutate repositories")
	}
	if !summary.NoDuplicatedStackFolders {
		return errors.New("complex-refactor rehearsal must prove no duplicated stack folders")
	}
	if err := validateComplexRefactorTaskCounts(summary.TaskCounts); err != nil {
		return err
	}
	if summary.TaskCounts.Ready > 0 {
		if strings.TrimSpace(summary.NextRecommendedFactoryTask.NodeID) == "" ||
			strings.TrimSpace(summary.NextRecommendedFactoryTask.TaskID) == "" ||
			strings.TrimSpace(summary.NextRecommendedFactoryTask.TargetFactoryRepo) == "" {
			return errors.New("complex-refactor rehearsal requires next_recommended_factory_task when ready tasks exist")
		}
	}
	if !summary.LoopDecision.MustNotStartBlockedTasks {
		return errors.New("complex-refactor rehearsal must block unsafe/blocked tasks")
	}
	if summary.Status == "ready" && summary.TaskCounts.Ready > 0 && !summary.LoopDecision.MayStartNextReadyTask {
		return errors.New("ready complex-refactor rehearsal must allow the next ready task")
	}
	if err := validateComplexRefactorRepairPlan(summary.RepairPlan); err != nil {
		return err
	}
	if err := validateComplexRefactorContextRepack(summary.ContextRepack); err != nil {
		return err
	}
	if len(summary.SourceDigests) == 0 {
		return errors.New("complex-refactor rehearsal requires source_digests")
	}
	for _, source := range summary.SourceDigests {
		if strings.TrimSpace(source.Name) == "" || strings.TrimSpace(source.Path) == "" {
			return errors.New("complex-refactor source_digests require name and path")
		}
		if !isHexSHA256(source.SHA256) {
			return errors.New("complex-refactor source_digests require 64-character sha256")
		}
		if err := validatePublicSafeText(source.Path); err != nil {
			return err
		}
	}
	if len(summary.Artifacts) == 0 {
		return errors.New("complex-refactor rehearsal requires artifacts")
	}
	for key, value := range summary.Artifacts {
		if strings.TrimSpace(key) == "" || strings.TrimSpace(value) == "" {
			return errors.New("complex-refactor artifacts require non-empty keys and paths")
		}
		if err := validatePublicSafeText(value); err != nil {
			return err
		}
	}
	return nil
}

func validateComplexRefactorRepairPlan(repair complexRefactorRepairPlan) error {
	if strings.TrimSpace(repair.Status) == "" && strings.TrimSpace(repair.Path) == "" && strings.TrimSpace(repair.RepairTaskID) == "" {
		return nil
	}
	if repair.Status != "repair_required" {
		return errors.New("complex-refactor repair_plan status must be repair_required")
	}
	if strings.TrimSpace(repair.Path) == "" || strings.TrimSpace(repair.RepairTaskID) == "" {
		return errors.New("complex-refactor repair_plan requires path and repair_task_id")
	}
	if repair.SchedulesWork || repair.ExecutesWork || repair.ApprovesWork {
		return errors.New("complex-refactor repair_plan must remain read-only")
	}
	if err := validatePublicSafeText(repair.Path); err != nil {
		return err
	}
	return nil
}

func validateComplexRefactorContextRepack(repack complexRefactorContextRepack) error {
	if strings.TrimSpace(repack.Status) == "" && strings.TrimSpace(repack.Path) == "" && strings.TrimSpace(repack.MissingContextReason) == "" {
		return nil
	}
	if repack.Status != "ready" {
		return errors.New("complex-refactor context_repack status must be ready")
	}
	if strings.TrimSpace(repack.Path) == "" || strings.TrimSpace(repack.MissingContextReason) == "" {
		return errors.New("complex-refactor context_repack requires path and missing_context_reason")
	}
	if repack.SchedulesWork || repack.ExecutesWork || repack.ApprovesWork {
		return errors.New("complex-refactor context_repack must remain read-only")
	}
	if err := validatePublicSafeText(repack.Path); err != nil {
		return err
	}
	if err := validatePublicSafeText(repack.MissingContextReason); err != nil {
		return err
	}
	return nil
}

func validateComplexRefactorTaskCounts(counts complexRefactorTaskCounts) error {
	if counts.Total < 0 || counts.Ready < 0 || counts.Blocked < 0 || counts.Completed < 0 || counts.Failed < 0 {
		return errors.New("complex-refactor task counts must not be negative")
	}
	if counts.Total == 0 {
		return errors.New("complex-refactor task counts require at least one task")
	}
	if counts.Ready+counts.Blocked+counts.Completed+counts.Failed != counts.Total {
		return errors.New("complex-refactor task counts must sum to total")
	}
	return nil
}

func validatePulseIntakePreflight(preflight foundryPulseIntakePreflight) error {
	if preflight.SchemaVersion != "ao.foundry.pulse-intake-preflight.v0.1" {
		return errors.New("invalid Pulse intake preflight schema_version")
	}
	if !isPulseStatus(preflight.Status) {
		return fmt.Errorf("invalid Pulse intake preflight status %q", preflight.Status)
	}
	if strings.TrimSpace(preflight.BlueprintStatus) == "" || strings.TrimSpace(preflight.AtlasStatus) == "" {
		return errors.New("Pulse intake preflight requires blueprint_status and atlas_status")
	}
	for _, source := range preflight.SourceArtifacts {
		if err := validatePulseSource(source, true); err != nil {
			return fmt.Errorf("Pulse intake preflight source_artifacts: %w", err)
		}
	}
	if preflight.Status == "ready" && len(preflight.SourceArtifacts) == 0 {
		return errors.New("ready Pulse intake preflight requires source_artifacts")
	}
	return nil
}

func validatePulsePRLifecycle(lifecycle foundryPulsePRLifecycle) error {
	if lifecycle.SchemaVersion != "ao.foundry.pulse-pr-lifecycle.v0.1" {
		return errors.New("invalid Pulse PR lifecycle schema_version")
	}
	for field, value := range map[string]string{
		"current_slice":       lifecycle.CurrentSlice,
		"target_repo":         lifecycle.TargetRepo,
		"branch":              lifecycle.Branch,
		"pr_state":            lifecycle.PRState,
		"check_state":         lifecycle.CheckState,
		"merge_state":         lifecycle.MergeState,
		"cleanup_state":       lifecycle.CleanupState,
		"allowed_next_action": lifecycle.AllowedNextAction,
	} {
		if strings.TrimSpace(value) == "" {
			return fmt.Errorf("Pulse PR lifecycle missing %s", field)
		}
	}
	return nil
}

func validatePulseOvernightStartGate(startGate foundryPulseOvernightStartGate) error {
	if startGate.SchemaVersion != "ao.foundry.pulse-overnight-start-gate.v0.1" {
		return errors.New("invalid Pulse overnight start gate schema_version")
	}
	if !isPulseStatus(startGate.Status) {
		return fmt.Errorf("invalid Pulse overnight start gate status %q", startGate.Status)
	}
	if strings.TrimSpace(startGate.AllowedNextAction) == "" {
		return errors.New("Pulse overnight start gate requires allowed_next_action")
	}
	if len(startGate.SourceHashes) == 0 {
		return errors.New("Pulse overnight start gate requires source_hashes")
	}
	for _, source := range startGate.SourceHashes {
		if err := validatePulseSource(source, false); err != nil {
			return fmt.Errorf("Pulse overnight start gate source_hashes: %w", err)
		}
	}
	return nil
}

func validatePulseSource(source pulseSource, requireStatus bool) error {
	if strings.TrimSpace(source.Name) == "" || strings.TrimSpace(source.Path) == "" || strings.TrimSpace(source.SchemaVersion) == "" {
		return errors.New("source requires name, path, and schema_version")
	}
	if requireStatus && strings.TrimSpace(source.Status) == "" {
		return errors.New("source requires status")
	}
	if !isHexSHA256(source.SHA256) {
		return errors.New("source requires 64-character sha256")
	}
	if err := validatePublicSafeText(source.Path); err != nil {
		return err
	}
	return nil
}

func isPulseStatus(status string) bool {
	return status == "ready" || status == "blocked" || status == "failed"
}

func isHexSHA256(value string) bool {
	if len(value) != 64 {
		return false
	}
	for _, ch := range value {
		if !((ch >= '0' && ch <= '9') || (ch >= 'a' && ch <= 'f') || (ch >= 'A' && ch <= 'F')) {
			return false
		}
	}
	return true
}

func validatePublicSafeText(value string) error {
	lower := strings.ToLower(strings.ReplaceAll(value, "\\", "/"))
	for _, marker := range []string{
		"/" + "users/",
		"/" + "home/",
		"/" + "tmp/",
		"/" + "var/folders/",
		"down" + "loads/",
		"file" + "://",
		"api" + "_key",
		"access" + "_token",
		"authorization: bearer",
		"begin " + "rsa",
		"begin " + "openssh",
	} {
		if strings.Contains(lower, marker) {
			return fmt.Errorf("unsafe public artifact value %q", value)
		}
	}
	return nil
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

func uniqueStrings(values []string) []string {
	seen := map[string]bool{}
	result := []string{}
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" || seen[value] {
			continue
		}
		seen[value] = true
		result = append(result, value)
	}
	return result
}

func equalStringSlices(left, right []string) bool {
	if len(left) != len(right) {
		return false
	}
	for i := range left {
		if left[i] != right[i] {
			return false
		}
	}
	return true
}

func sortedStringKeys(values map[string]string) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func validateFoundryAtlasStatus(status foundryAtlasStatus) error {
	if status.SchemaVersion != "ao.foundry.atlas-status.v0.1" {
		return errors.New("invalid Foundry Atlas status schema_version")
	}
	if status.Status != "ready" || status.ReadbackStatus != "ready" {
		return errors.New("Foundry Atlas status and readback_status must be ready")
	}
	if status.Mode != "fixture_only_readback" {
		return errors.New("Foundry Atlas status mode must be fixture_only_readback")
	}
	for field, value := range map[string]string{
		"registry_id":     status.RegistryID,
		"import_id":       status.ImportID,
		"workgraph_id":    status.WorkgraphID,
		"target_instance": status.TargetInstance,
		"task_id":         status.TaskID,
		"task_digest":     status.TaskDigest,
		"run_link_digest": status.RunLinkDigest,
	} {
		if strings.TrimSpace(value) == "" {
			return fmt.Errorf("Foundry Atlas status missing %s", field)
		}
	}
	if !isSHA256Digest(status.TaskDigest) || !isSHA256Digest(status.RunLinkDigest) {
		return errors.New("Foundry Atlas status requires sha256 task and run-link digests")
	}
	if status.SchedulesWork || status.ExecutesWork || status.ApprovesWork {
		return errors.New("Foundry Atlas status must remain observer-only and cannot schedule, execute, or approve work")
	}
	if len(status.Evidence) == 0 {
		return errors.New("Foundry Atlas status requires evidence")
	}
	for key, value := range status.Evidence {
		if strings.TrimSpace(key) == "" || strings.TrimSpace(value) == "" {
			return errors.New("Foundry Atlas status evidence keys and paths must not be empty")
		}
	}
	return nil
}

func validateAtlasAuthorityLadderStatus(status atlasMissionStatus) error {
	if status.ContractVersion != "ao.atlas.mission-status.v0.1" {
		return errors.New("invalid Atlas mission status contract_version")
	}
	for field, value := range map[string]string{
		"intake_id":         status.IntakeID,
		"workgraph_id":      status.WorkgraphID,
		"target_instance":   status.TargetInstance,
		"completion_status": status.CompletionStatus,
	} {
		if strings.TrimSpace(value) == "" {
			return fmt.Errorf("Atlas authority ladder missing %s", field)
		}
	}
	if status.SchedulesWork || status.ExecutesWork {
		return errors.New("Atlas authority ladder mission status must remain read-only and cannot schedule or execute work")
	}
	if status.AuthorityLadder == nil {
		return errors.New("Atlas authority ladder mission status requires authority_ladder readback")
	}
	ladder := *status.AuthorityLadder
	for field, value := range map[string]string{
		"authority_ladder.current_class": ladder.CurrentClass,
		"authority_ladder.next_class":    ladder.NextClass,
	} {
		if strings.TrimSpace(value) == "" {
			return fmt.Errorf("Atlas authority ladder missing %s", field)
		}
	}
	for field, values := range map[string][]string{
		"authority_ladder.proven_live_classes":  ladder.ProvenLiveClasses,
		"authority_ladder.blockers":             ladder.Blockers,
		"authority_ladder.required_evidence":    ladder.RequiredEvidence,
		"authority_ladder.do_not_advance_gates": ladder.DoNotAdvanceGates,
	} {
		if len(values) == 0 {
			return fmt.Errorf("Atlas authority ladder requires %s", field)
		}
		for _, value := range values {
			if strings.TrimSpace(value) == "" {
				return fmt.Errorf("Atlas authority ladder %s entries must not be empty", field)
			}
		}
	}
	if ladder.DeniedHigherClasses == nil || len(ladder.DeniedHigherClasses) == 0 {
		return errors.New("Atlas authority ladder requires denied_higher_classes reasons")
	}
	for class, reason := range ladder.DeniedHigherClasses {
		if strings.TrimSpace(class) == "" || strings.TrimSpace(reason) == "" {
			return errors.New("Atlas authority ladder denied_higher_classes keys and reasons must not be empty")
		}
	}
	return nil
}

func isSHA256Digest(value string) bool {
	if len(value) != len("sha256:")+64 || !strings.HasPrefix(value, "sha256:") {
		return false
	}
	for _, ch := range strings.TrimPrefix(value, "sha256:") {
		if !((ch >= '0' && ch <= '9') || (ch >= 'a' && ch <= 'f')) {
			return false
		}
	}
	return true
}

func readRSIManifest(path string) (rsiManifestSummary, error) {
	var manifest rsiArchitectureManifest
	bytes, err := os.ReadFile(path)
	if err != nil {
		return rsiManifestSummary{}, fmt.Errorf("read manifest: %w", err)
	}
	if err := json.Unmarshal(bytes, &manifest); err != nil {
		return rsiManifestSummary{}, fmt.Errorf("invalid manifest JSON: %w", err)
	}
	if err := validateRSIManifest(manifest); err != nil {
		return rsiManifestSummary{}, err
	}
	return rsiManifestSummary{
		SchemaVersion:                      "ao.command.rsi-manifest.v0.1",
		CommandSchemaVersion:               commandSchemaVersion,
		Status:                             "passed",
		Manifest:                           path,
		ManifestSchemaVersion:              manifest.SchemaVersion,
		GeneratedDate:                      manifest.GeneratedDate,
		OperatorMode:                       operatorMode,
		MutatesRepositories:                false,
		ClaimLevels:                        manifest.ClaimLevels,
		ActiveRepositories:                 manifest.ActiveRepositories,
		DeprecatedOrOutOfScopeRepositories: manifest.DeprecatedOrOutOfScopeRepositories,
		FullClaimRequiredEvidence:          manifest.FullClaimRequiredEvidence,
	}, nil
}

func validateRSIManifest(manifest rsiArchitectureManifest) error {
	if manifest.SchemaVersion != "ao.architecture.rsi-claim-evidence-manifest.v0.1" {
		return errors.New("invalid RSI manifest schema_version")
	}
	if !hasManifestClaimLevel(manifest.ClaimLevels, "bounded_governed_rsi", "allowed") {
		return errors.New("bounded_governed_rsi allowed claim level is required")
	}
	if !hasManifestClaimLevel(manifest.ClaimLevels, "full_autonomous_self_mutating_rsi", "denied") {
		return errors.New("full_autonomous_self_mutating_rsi denied claim level is required")
	}
	if !hasManifestClaimRequiredEvidence(manifest.ClaimLevels, "bounded_governed_rsi", "ao2_control_plane_rsi_claim_readiness_readback") {
		return errors.New("bounded_governed_rsi required evidence must include ao2-control-plane RSI claim-readiness readback")
	}
	if !hasManifestClaimRequiredEvidence(manifest.ClaimLevels, "bounded_governed_rsi", "ao2_rsi_self_change_dry_run") ||
		!hasManifestClaimRequiredEvidence(manifest.ClaimLevels, "bounded_governed_rsi", "ao2_control_plane_rsi_self_change_dry_run_readback") {
		return errors.New("bounded_governed_rsi required evidence must include AO2 self-change dry-run and control-plane readback")
	}
	if !hasManifestClaimRequiredEvidence(manifest.ClaimLevels, "bounded_governed_rsi", "ao2_rsi_authority_packet_dry_run_candidate") ||
		!hasManifestClaimRequiredEvidence(manifest.ClaimLevels, "bounded_governed_rsi", "ao2_control_plane_rsi_authority_packet_readback") {
		return errors.New("bounded_governed_rsi required evidence must include AO2 authority packet candidate and control-plane readback")
	}
	if !hasManifestClaimRequiredEvidence(manifest.ClaimLevels, "bounded_governed_rsi", "ao_forge_architecture_rsi_pin_readback") {
		return errors.New("AO Forge architecture RSI pin readback evidence is required")
	}
	for _, repo := range []string{"ao-foundry", "ao-forge", "ao-command", "ao-covenant", "ao2", "ao2-control-plane"} {
		if !hasManifestRepository(manifest.ActiveRepositories, repo) {
			return fmt.Errorf("active repository %s is required", repo)
		}
	}
	aoForge, ok := findManifestRepository(manifest.ActiveRepositories, "ao-forge")
	if !ok || !hasAOForgeRetainedCommandManifestEvidence(aoForge) {
		return errors.New("AO Forge retained AO Command RSI manifest evidence is required")
	}
	if !hasAOForgeArchitectureRSIPinReadback(aoForge) {
		return errors.New("AO Forge architecture RSI pin readback evidence is required")
	}
	aoCovenant, ok := findManifestRepository(manifest.ActiveRepositories, "ao-covenant")
	if !ok || !hasAOCovenantRetainedRollbackBoundary(aoCovenant) {
		return errors.New("AO Covenant retained rollback-only denial evidence is required")
	}
	if !hasAOCovenantLiveSelfChangeAuthorityPacket(aoCovenant) {
		return errors.New("AO Covenant live self-change authority packet evidence is required")
	}
	ao2, ok := findManifestRepository(manifest.ActiveRepositories, "ao2")
	if !ok || !hasAO2RSISelfChangeDryRun(ao2) {
		return errors.New("AO2 RSI self-change dry-run evidence is required")
	}
	ao2ControlPlane, ok := findManifestRepository(manifest.ActiveRepositories, "ao2-control-plane")
	if !ok || !hasAO2ControlPlaneRSIReadback(ao2ControlPlane) {
		return errors.New("ao2-control-plane RSI claim-readiness readback is required")
	}
	if !hasAO2ControlPlaneRSISelfChangeDryRunReadback(ao2ControlPlane) {
		return errors.New("ao2-control-plane RSI self-change dry-run readback is required")
	}
	if !hasAO2RSIRollbackRehearsal(ao2) || !hasAO2ControlPlaneRSIRollbackRehearsalReadback(ao2ControlPlane) {
		return errors.New("AO2 RSI rollback rehearsal evidence and control-plane readback are required")
	}
	if !hasAO2RSIAuthorityPacketCandidate(ao2) || !hasAO2ControlPlaneRSIAuthorityPacketReadback(ao2ControlPlane) {
		return errors.New("AO2 RSI authority packet candidate and control-plane readback are required")
	}
	for _, repo := range []string{"ao-operator", "ao-runtime", "ao-control-plane", "ao-conductor", "agy-swarms"} {
		if !hasManifestRepository(manifest.DeprecatedOrOutOfScopeRepositories, repo) {
			return fmt.Errorf("deprecated or out-of-scope repository %s is required", repo)
		}
	}
	for _, term := range []string{"mutation authority", "rollback", "live self-change", "observer readback", "claim.publish"} {
		if !manifestEvidenceContains(manifest.FullClaimRequiredEvidence, term) {
			return fmt.Errorf("full claim required evidence must include %q", term)
		}
	}
	return nil
}

func hasManifestClaimRequiredEvidence(claims []rsiManifestClaimLevel, claimLevel string, evidence string) bool {
	for _, claim := range claims {
		if claim.ClaimLevel == claimLevel {
			return manifestEvidenceContains(claim.RequiredEvidence, evidence)
		}
	}
	return false
}

func hasManifestClaimLevel(claims []rsiManifestClaimLevel, claimLevel string, decision string) bool {
	for _, claim := range claims {
		if claim.ClaimLevel == claimLevel && claim.Decision == decision && strings.TrimSpace(claim.Status) != "" {
			return true
		}
	}
	return false
}

func hasManifestRepository(repositories []rsiManifestRepository, id string) bool {
	for _, repo := range repositories {
		if repo.ID == id {
			return true
		}
	}
	return false
}

func findManifestRepository(repositories []rsiManifestRepository, id string) (rsiManifestRepository, bool) {
	for _, repo := range repositories {
		if repo.ID == id {
			return repo, true
		}
	}
	return rsiManifestRepository{}, false
}

func hasManifestKnownPR(prs []rsiManifestKnownPR, number int, title string) bool {
	for _, pr := range prs {
		if pr.Number == number && strings.Contains(pr.Title, title) && strings.TrimSpace(pr.MergeCommit) != "" {
			return true
		}
	}
	return false
}

func hasAOForgeRetainedCommandManifestEvidence(repo rsiManifestRepository) bool {
	return manifestEvidenceContains(repo.Evidence, "ao-command-rsi-manifest-retention-proof.json") &&
		manifestEvidenceContains(repo.Evidence, "ao-command-rsi-manifest") &&
		manifestEvidenceContains(repo.Evidence, "rollback_rehearsal.status=passed") &&
		hasManifestKnownPR(repo.KnownPRs, 143, "Retain AO Command RSI manifest evidence")
}

func hasAOForgeArchitectureRSIPinReadback(repo rsiManifestRepository) bool {
	return manifestEvidenceContains(repo.Evidence, "docs/contracts/architecture-rsi-pin-readback-v0.1.schema.json") &&
		manifestEvidenceContains(repo.Evidence, "docs/evidence/architecture/ao-architecture-rsi-pin-readback.json") &&
		manifestEvidenceContains(repo.Evidence, "goalrun.architecture_rsi_pin_readback") &&
		hasManifestKnownPR(repo.KnownPRs, 144, "Require architecture RSI pin readback readiness")
}

func hasAOCovenantRetainedRollbackBoundary(repo rsiManifestRepository) bool {
	return manifestEvidenceContains(repo.Evidence, "examples/full-rsi-claim-boundary/rollback-retained.contract.json") &&
		manifestEvidenceContains(repo.Evidence, "examples/full-rsi-claim-boundary/rollback-retained-ticket.json") &&
		manifestEvidenceContains(repo.Evidence, "retained rollback rehearsal alone is insufficient") &&
		hasManifestKnownPR(repo.KnownPRs, 57, "Deny full RSI with retained rollback only")
}

func hasAOCovenantLiveSelfChangeAuthorityPacket(repo rsiManifestRepository) bool {
	return manifestEvidenceContains(repo.Evidence, "examples/full-rsi-claim-boundary/live-self-change-authority.packet.json") &&
		manifestEvidenceContains(repo.Evidence, "schemas/covenant.live-self-change-authority.v1.schema.json") &&
		manifestEvidenceContains(repo.Evidence, "covenant.live-self-change-authority.v1") &&
		hasManifestKnownPR(repo.KnownPRs, 58, "Add live self-change authority packet schema")
}

func hasAO2ControlPlaneRSIReadback(repo rsiManifestRepository) bool {
	return manifestEvidenceContains(repo.Evidence, "scripts/verify_ao2_rsi_claim_readiness.py") &&
		manifestEvidenceContains(repo.Evidence, "ao2.cp-ao2-rsi-claim-readiness-readback.v1") &&
		hasManifestKnownPR(repo.KnownPRs, 70, "Add AO2 RSI claim readiness readback") &&
		manifestEvidenceContains(repo.ClaimOutput, "control_plane_ao2_rsi_claim_readiness_readback=passed") &&
		manifestEvidenceContains(repo.ClaimOutput, "claim_level=bounded_governed_rsi decision=allowed") &&
		manifestEvidenceContains(repo.ClaimOutput, "claim_level=full_autonomous_self_mutating_rsi decision=denied") &&
		strings.Contains(repo.Boundary, "observer_only") &&
		strings.Contains(repo.Boundary, "no_claim_approval") &&
		strings.Contains(repo.Boundary, "no_repository_mutation")
}

func hasAO2RSISelfChangeDryRun(repo rsiManifestRepository) bool {
	return manifestEvidenceContains(repo.Evidence, "scripts/rsi-governed-self-change-dry-run.sh") &&
		manifestEvidenceContains(repo.Evidence, "tests/test_ao2_rsi_governed_self_change_dry_run.py") &&
		manifestEvidenceContains(repo.Evidence, "target/rsi-self-change-dry-run/latest/summary.json") &&
		manifestEvidenceContains(repo.Evidence, "proposed-self-change.patch") &&
		manifestEvidenceContains(repo.Evidence, "rollback-self-change.patch") &&
		manifestEvidenceContains(repo.Evidence, "ao2.rsi-governed-self-change-dry-run.v1") &&
		hasManifestKnownPR(repo.KnownPRs, 199, "Add AO2 RSI self-change dry-run evidence") &&
		manifestEvidenceContains(repo.ClaimOutput, "self_change_dry_run=passed") &&
		manifestEvidenceContains(repo.ClaimOutput, "claim_level=bounded_governed_rsi decision=allowed") &&
		manifestEvidenceContains(repo.ClaimOutput, "claim_level=full_autonomous_self_mutating_rsi decision=denied") &&
		strings.Contains(repo.Boundary, "execution_and_evidence_mechanics_only")
}

func hasAO2RSIRollbackRehearsal(repo rsiManifestRepository) bool {
	return manifestEvidenceContains(repo.Evidence, "rollback_rehearsal.status=passed") &&
		manifestEvidenceContains(repo.Evidence, "rollback-rehearsal/worktree") &&
		hasManifestKnownPR(repo.KnownPRs, 200, "Add RSI rollback rehearsal evidence") &&
		manifestEvidenceContains(repo.ClaimOutput, "rollback_rehearsal=passed")
}

func hasAO2RSIAuthorityPacketCandidate(repo rsiManifestRepository) bool {
	return manifestEvidenceContains(repo.Evidence, "live-self-change-authority.packet.json") &&
		manifestEvidenceContains(repo.Evidence, "covenant.live-self-change-authority.v1") &&
		manifestEvidenceContains(repo.Evidence, "mutation_authority_packet.mode=dry_run_candidate") &&
		manifestEvidenceContains(repo.Evidence, "mutation_authority_packet.schema_valid_for_claim_publish=false") &&
		hasManifestKnownPR(repo.KnownPRs, 201, "Emit RSI authority packet dry-run evidence") &&
		manifestEvidenceContains(repo.ClaimOutput, "mutation_authority_packet=dry_run_candidate") &&
		manifestEvidenceContains(repo.ClaimOutput, "schema_valid_for_claim_publish=false") &&
		manifestEvidenceContains(repo.ClaimOutput, "claim_level=full_autonomous_self_mutating_rsi decision=denied")
}

func hasAO2ControlPlaneRSISelfChangeDryRunReadback(repo rsiManifestRepository) bool {
	return manifestEvidenceContains(repo.Evidence, "scripts/verify_ao2_rsi_self_change_dry_run.py") &&
		manifestEvidenceContains(repo.Evidence, "tests/test_ao2_rsi_self_change_dry_run_readback.py") &&
		manifestEvidenceContains(repo.Evidence, "target/ao2-rsi-self-change-dry-run-readback/summary.json") &&
		manifestEvidenceContains(repo.Evidence, "ao2.cp-ao2-rsi-self-change-dry-run-readback.v1") &&
		hasManifestKnownPR(repo.KnownPRs, 71, "Add AO2 RSI self-change dry-run readback") &&
		manifestEvidenceContains(repo.ClaimOutput, "control_plane_ao2_rsi_self_change_dry_run_readback=passed") &&
		manifestEvidenceContains(repo.ClaimOutput, "claim_level=bounded_governed_rsi decision=allowed") &&
		manifestEvidenceContains(repo.ClaimOutput, "claim_level=full_autonomous_self_mutating_rsi decision=denied") &&
		strings.Contains(repo.Boundary, "observer_only") &&
		strings.Contains(repo.Boundary, "no_claim_approval") &&
		strings.Contains(repo.Boundary, "no_patch_application") &&
		strings.Contains(repo.Boundary, "no_repository_mutation")
}

func hasAO2ControlPlaneRSIRollbackRehearsalReadback(repo rsiManifestRepository) bool {
	return manifestEvidenceContains(repo.Evidence, "rollback_rehearsal.status=passed") &&
		hasManifestKnownPR(repo.KnownPRs, 72, "Require AO2 RSI rollback rehearsal readback")
}

func hasAO2ControlPlaneRSIAuthorityPacketReadback(repo rsiManifestRepository) bool {
	return manifestEvidenceContains(repo.Evidence, "scripts/verify_ao2_rsi_authority_packet.py") &&
		manifestEvidenceContains(repo.Evidence, "tests/test_ao2_rsi_authority_packet_readback.py") &&
		manifestEvidenceContains(repo.Evidence, "target/ao2-rsi-authority-packet-readback/summary.json") &&
		manifestEvidenceContains(repo.Evidence, "ao2.cp-ao2-rsi-authority-packet-readback.v1") &&
		manifestEvidenceContains(repo.Evidence, "covenant.live-self-change-authority.v1") &&
		manifestEvidenceContains(repo.Evidence, "live-self-change-authority.packet.json") &&
		manifestEvidenceContains(repo.Evidence, "schema_valid_for_claim_publish=false") &&
		hasManifestKnownPR(repo.KnownPRs, 73, "Add AO2 RSI authority packet readback") &&
		manifestEvidenceContains(repo.ClaimOutput, "control_plane_ao2_rsi_authority_packet_readback=passed") &&
		manifestEvidenceContains(repo.ClaimOutput, "claim_level=full_autonomous_self_mutating_rsi decision=denied") &&
		strings.Contains(repo.Boundary, "observer_only") &&
		strings.Contains(repo.Boundary, "no_claim_approval") &&
		strings.Contains(repo.Boundary, "no_patch_application") &&
		strings.Contains(repo.Boundary, "no_repository_mutation")
}

func manifestEvidenceContains(values []string, term string) bool {
	term = strings.ToLower(term)
	for _, value := range values {
		if strings.Contains(strings.ToLower(value), term) {
			return true
		}
	}
	return false
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
		SchemaVersion:           "ao.command.rsi-health.v0.1",
		CommandSchemaVersion:    commandSchemaVersion,
		Status:                  status,
		RSIMode:                 "governed_fixture_local",
		RSICapability:           capability,
		OperatorMode:            operatorMode,
		MutatesRepositories:     false,
		ClaimLevels:             rsiClaimLevels(status),
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
		ClaimLevels:             summary.ClaimLevels,
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

func rsiClaimLevels(healthStatus string) []rsiClaimLevel {
	boundedDecision := "denied"
	boundedStatus := "blocked"
	boundedReason := "bounded governed RSI claim requires the Foundry pulse, Forge retention, Command health, 5 percent improvement gate, read-only operator mode, and non-mutating evidence chain to pass"
	if healthStatus == "passed" {
		boundedDecision = "allowed"
		boundedStatus = "passed"
		boundedReason = "bounded governed RSI claim allowed: Foundry pulse, Forge retention, Command health, 5 percent improvement gate, read-only operator mode, and non-mutating evidence chain passed"
	}
	return []rsiClaimLevel{
		{
			Claim:    "bounded_governed_rsi",
			Decision: boundedDecision,
			Status:   boundedStatus,
			Reason:   boundedReason,
		},
		{
			Claim:    "full_autonomous_self_mutating_rsi",
			Decision: "denied",
			Status:   "blocked",
			Reason:   "full autonomous self-mutating RSI remains denied until mutation authority, rollback, and live self-change evidence exist and AO Covenant allows the claim.publish boundary",
		},
	}
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
	gate.normalizeScoreFields()
	passed := gate.SchemaVersion == "ao.foundry.rsi-improvement-gate.v0.1" &&
		gate.Status == "passed" &&
		gate.CandidateScore >= gate.BaselineScore &&
		gate.ActualImprovementPercent >= gate.RequiredImprovementPercent &&
		gate.RequiredImprovementPercent > 0 &&
		gate.AutonomousClaim == "measured_local_improvement" &&
		!gate.MutatesRepositories
	return rsiFamilyStatus{Family: "ao-foundry", Status: gate.Status, Passed: passed, Evidence: path}, gate, nil
}

func (gate *foundryRSIImprovementGate) normalizeScoreFields() {
	if gate.BaselineScore == 0 && gate.BaselineScorePercent != nil {
		gate.BaselineScore = *gate.BaselineScorePercent
	}
	if gate.CandidateScore == 0 && gate.CandidateScorePercent != nil {
		gate.CandidateScore = *gate.CandidateScorePercent
	}
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
		passed = passed && proof.SchemaValid && forgeRetentionBasePassed(proof, goalID, iteration, phase)
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
	bytes, err := os.ReadFile(path)
	if err != nil {
		return forgeRetainedEvidence{}, err
	}
	var retained forgeRetainedEvidence
	if err := json.Unmarshal(bytes, &retained); err != nil {
		return forgeRetainedEvidence{}, fmt.Errorf("invalid JSON: %w", err)
	}
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(bytes, &raw); err != nil {
		return forgeRetainedEvidence{}, err
	}
	if err := validateForgeRetainedEvidenceContract(raw); err != nil {
		retained.SchemaError = err.Error()
	} else {
		retained.SchemaValid = true
	}
	return retained, nil
}

func validateForgeRetainedEvidenceContract(raw map[string]json.RawMessage) error {
	schemaVersion, err := requireJSONString(raw, "schema_version")
	if err != nil {
		return err
	}
	if schemaVersion != "ao.forge.goal-run-retained-evidence.v0.1" {
		return fmt.Errorf("schema_version must be ao.forge.goal-run-retained-evidence.v0.1")
	}
	if _, err := requireJSONString(raw, "goal_id"); err != nil {
		return err
	}
	if _, err := requireJSONString(raw, "iteration"); err != nil {
		return err
	}
	phase, err := requireJSONString(raw, "phase")
	if err != nil {
		return err
	}
	if !stringIn(phase, []string{"planning", "implementation", "verification", "blocked", "backoff", "stopped", "complete"}) {
		return fmt.Errorf("phase has unsupported value %q", phase)
	}
	if summary, err := requireJSONString(raw, "summary"); err != nil {
		return err
	} else if strings.TrimSpace(summary) == "" {
		return fmt.Errorf("summary must not be empty")
	}

	policy, err := requireJSONObject(raw, "retention_policy")
	if err != nil {
		return err
	}
	layout, err := requireJSONString(policy, "layout")
	if err != nil {
		return fmt.Errorf("retention_policy.%w", err)
	}
	if layout != "docs/evidence/goals/<goal_id>/<YYYYMMDDTHHMMSSZ>-<phase>/" {
		return fmt.Errorf("retention_policy.layout must match AO Forge retained evidence layout")
	}
	temporaryPathsAllowed, err := requireJSONBool(policy, "temporary_paths_allowed")
	if err != nil {
		return fmt.Errorf("retention_policy.%w", err)
	}
	if temporaryPathsAllowed {
		return fmt.Errorf("retention_policy.temporary_paths_allowed must be false")
	}
	minRetentionDays, err := requireJSONNumber(policy, "minimum_retention_days_after_terminal_phase")
	if err != nil {
		return fmt.Errorf("retention_policy.%w", err)
	}
	if minRetentionDays < 90 {
		return fmt.Errorf("retention_policy.minimum_retention_days_after_terminal_phase must be at least 90")
	}

	metadata, err := requireJSONObject(raw, "retention_metadata")
	if err != nil {
		return err
	}
	retainedAt, err := requireJSONString(metadata, "retained_at")
	if err != nil {
		return fmt.Errorf("retention_metadata.%w", err)
	}
	if _, err := time.Parse(time.RFC3339, retainedAt); err != nil {
		return fmt.Errorf("retention_metadata.retained_at must be RFC3339: %w", err)
	}
	retentionClass, err := requireJSONString(metadata, "retention_class")
	if err != nil {
		return fmt.Errorf("retention_metadata.%w", err)
	}
	if !stringIn(retentionClass, []string{"loop_evidence", "release_provenance", "promotion_provenance"}) {
		return fmt.Errorf("retention_metadata.retention_class has unsupported value %q", retentionClass)
	}
	retainWhileGoalActive, err := requireJSONBool(metadata, "retain_while_goal_active")
	if err != nil {
		return fmt.Errorf("retention_metadata.%w", err)
	}
	if !retainWhileGoalActive {
		return fmt.Errorf("retention_metadata.retain_while_goal_active must be true")
	}
	deletionRequiresReview, err := requireJSONBool(metadata, "deletion_requires_review")
	if err != nil {
		return fmt.Errorf("retention_metadata.%w", err)
	}
	if !deletionRequiresReview {
		return fmt.Errorf("retention_metadata.deletion_requires_review must be true")
	}
	cleanupFields, err := requireJSONStringArray(metadata, "cleanup_change_must_name")
	if err != nil {
		return fmt.Errorf("retention_metadata.%w", err)
	}
	if len(cleanupFields) < 3 ||
		!stringIn("goal_id", cleanupFields) ||
		!stringIn("iteration", cleanupFields) ||
		!stringIn("reason", cleanupFields) {
		return fmt.Errorf("retention_metadata.cleanup_change_must_name must include goal_id, iteration, and reason")
	}

	outputs, ok, err := optionalJSONArray(raw, "captured_outputs")
	if err != nil {
		return err
	}
	if ok {
		for i, output := range outputs {
			if err := validateForgeRetainedOutputContract(output); err != nil {
				return fmt.Errorf("captured_outputs[%d].%w", i, err)
			}
		}
	}
	return nil
}

func validateForgeRetainedOutputContract(output map[string]json.RawMessage) error {
	label, err := requireJSONString(output, "label")
	if err != nil {
		return err
	}
	if _, err := requireJSONString(output, "command"); err != nil {
		return err
	}
	if _, err := requireJSONString(output, "status"); err != nil {
		return err
	}

	switch label {
	case "ao-command-rsi-health":
		if err := requireStringConst(output, "command", "ao-command rsi health"); err != nil {
			return err
		}
		if err := requireStringConst(output, "status", "passed"); err != nil {
			return err
		}
		if err := requireStringConst(output, "rsi_mode", "governed_fixture_local"); err != nil {
			return err
		}
		if err := requireStringConst(output, "rsi_capability", "demonstrated_local_fixture_loop"); err != nil {
			return err
		}
		if err := requireStringConst(output, "operator_mode", "read_only"); err != nil {
			return err
		}
		if err := requireBoolConst(output, "mutates_repositories", false); err != nil {
			return err
		}
		families, ok, err := optionalJSONArray(output, "families")
		if err != nil {
			return err
		}
		if !ok {
			return fmt.Errorf("missing property %q", "families")
		}
		for _, family := range []string{"ao-arena", "ao-crucible", "ao-sentinel", "ao-promoter", "ao-foundry"} {
			if !retainedFamilyPassed(families, family) {
				return fmt.Errorf("families must include passed %s", family)
			}
		}
	case "ao-foundry-rsi-candidate":
		if err := requireStringConst(output, "command", "foundry pulse run"); err != nil {
			return err
		}
		if err := requireStringConst(output, "schema_version", "ao.foundry.rsi-candidate.v0.1"); err != nil {
			return err
		}
		if err := requireStringConst(output, "status", "ready"); err != nil {
			return err
		}
		if err := requireStringConst(output, "generated_by", "foundry pulse run"); err != nil {
			return err
		}
		if _, err := requireJSONNumber(output, "baseline_score"); err != nil {
			return err
		}
		if _, err := requireJSONNumber(output, "candidate_score"); err != nil {
			return err
		}
		if err := requireBoolConst(output, "mutates_repositories", false); err != nil {
			return err
		}
	case "ao-foundry-rsi-improvement-gate":
		if err := requireStringConst(output, "command", "foundry pulse run"); err != nil {
			return err
		}
		if err := requireStringConst(output, "schema_version", "ao.foundry.rsi-improvement-gate.v0.1"); err != nil {
			return err
		}
		if err := requireStringConst(output, "status", "passed"); err != nil {
			return err
		}
		for _, field := range []string{"baseline_score", "candidate_score"} {
			if _, err := requireJSONNumber(output, field); err != nil {
				return err
			}
		}
		requiredImprovement, err := requireJSONNumber(output, "required_improvement_percent")
		if err != nil {
			return err
		}
		if requiredImprovement < 5 {
			return fmt.Errorf("required_improvement_percent must be at least 5")
		}
		actualImprovement, err := requireJSONNumber(output, "actual_improvement_percent")
		if err != nil {
			return err
		}
		if actualImprovement < 5 {
			return fmt.Errorf("actual_improvement_percent must be at least 5")
		}
		if err := requireStringConst(output, "autonomous_claim", "measured_local_improvement"); err != nil {
			return err
		}
		if err := requireBoolConst(output, "mutates_repositories", false); err != nil {
			return err
		}
	case "ao-foundry-rsi-next-improvement-task":
		if err := requireStringConst(output, "command", "foundry pulse run"); err != nil {
			return err
		}
		if err := requireStringConst(output, "schema_version", "ao.foundry.rsi-next-improvement-task.v0.1"); err != nil {
			return err
		}
		if err := requireStringConst(output, "status", "ready"); err != nil {
			return err
		}
		requiredImprovement, err := requireJSONNumber(output, "required_improvement_percent")
		if err != nil {
			return err
		}
		if requiredImprovement < 5 {
			return fmt.Errorf("required_improvement_percent must be at least 5")
		}
		actualImprovement, err := requireJSONNumber(output, "actual_improvement_percent")
		if err != nil {
			return err
		}
		if actualImprovement < 5 {
			return fmt.Errorf("actual_improvement_percent must be at least 5")
		}
		if err := requireStringConst(output, "autonomous_claim", "derived_local_next_improvement"); err != nil {
			return err
		}
		if err := requireBoolConst(output, "mutates_repositories", false); err != nil {
			return err
		}
	}
	return nil
}

func forgeRetentionBasePassed(proof forgeRetainedEvidence, goalID, iteration, phase string) bool {
	return proof.SchemaVersion == "ao.forge.goal-run-retained-evidence.v0.1" &&
		proof.GoalID == goalID &&
		proof.Iteration == iteration &&
		proof.Phase == phase &&
		proof.Summary != "" &&
		len(proof.CapturedOutputs) == 1 &&
		proof.RetentionPolicy.Layout == "docs/evidence/goals/<goal_id>/<YYYYMMDDTHHMMSSZ>-<phase>/" &&
		!proof.RetentionPolicy.TemporaryPathsAllowed &&
		proof.RetentionPolicy.MinimumRetentionDaysAfterTerminalPhase >= 90 &&
		proof.RetentionMetadata.RetainedAt != "" &&
		proof.RetentionMetadata.RetentionClass == "loop_evidence" &&
		proof.RetentionMetadata.RetainWhileGoalActive &&
		proof.RetentionMetadata.DeletionRequiresReview &&
		stringIn("goal_id", proof.RetentionMetadata.CleanupChangeMustName) &&
		stringIn("iteration", proof.RetentionMetadata.CleanupChangeMustName) &&
		stringIn("reason", proof.RetentionMetadata.CleanupChangeMustName)
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

func retainedFamilyPassed(families []map[string]json.RawMessage, target string) bool {
	for _, family := range families {
		name, err := requireJSONString(family, "family")
		if err != nil || name != target {
			continue
		}
		passed, err := requireJSONBool(family, "passed")
		if err != nil || !passed {
			continue
		}
		if _, err := requireJSONString(family, "status"); err != nil {
			continue
		}
		return true
	}
	return false
}

func requireStringConst(raw map[string]json.RawMessage, field, want string) error {
	got, err := requireJSONString(raw, field)
	if err != nil {
		return err
	}
	if got != want {
		return fmt.Errorf("%s must be %q", field, want)
	}
	return nil
}

func requireBoolConst(raw map[string]json.RawMessage, field string, want bool) error {
	got, err := requireJSONBool(raw, field)
	if err != nil {
		return err
	}
	if got != want {
		return fmt.Errorf("%s must be %t", field, want)
	}
	return nil
}

func requireJSONString(raw map[string]json.RawMessage, field string) (string, error) {
	value, ok := raw[field]
	if !ok {
		return "", fmt.Errorf("missing property %q", field)
	}
	var got string
	if err := json.Unmarshal(value, &got); err != nil {
		return "", fmt.Errorf("%s must be a string", field)
	}
	if strings.TrimSpace(got) == "" {
		return "", fmt.Errorf("%s must not be empty", field)
	}
	return got, nil
}

func requireJSONBool(raw map[string]json.RawMessage, field string) (bool, error) {
	value, ok := raw[field]
	if !ok {
		return false, fmt.Errorf("missing property %q", field)
	}
	var got bool
	if err := json.Unmarshal(value, &got); err != nil {
		return false, fmt.Errorf("%s must be a boolean", field)
	}
	return got, nil
}

func requireJSONNumber(raw map[string]json.RawMessage, field string) (float64, error) {
	value, ok := raw[field]
	if !ok {
		return 0, fmt.Errorf("missing property %q", field)
	}
	var got float64
	if err := json.Unmarshal(value, &got); err != nil {
		return 0, fmt.Errorf("%s must be a number", field)
	}
	return got, nil
}

func requireJSONObject(raw map[string]json.RawMessage, field string) (map[string]json.RawMessage, error) {
	value, ok := raw[field]
	if !ok {
		return nil, fmt.Errorf("missing property %q", field)
	}
	var got map[string]json.RawMessage
	if err := json.Unmarshal(value, &got); err != nil {
		return nil, fmt.Errorf("%s must be an object", field)
	}
	return got, nil
}

func requireJSONStringArray(raw map[string]json.RawMessage, field string) ([]string, error) {
	value, ok := raw[field]
	if !ok {
		return nil, fmt.Errorf("missing property %q", field)
	}
	var got []string
	if err := json.Unmarshal(value, &got); err != nil {
		return nil, fmt.Errorf("%s must be a string array", field)
	}
	return got, nil
}

func optionalJSONArray(raw map[string]json.RawMessage, field string) ([]map[string]json.RawMessage, bool, error) {
	value, ok := raw[field]
	if !ok {
		return nil, false, nil
	}
	var got []map[string]json.RawMessage
	if err := json.Unmarshal(value, &got); err != nil {
		return nil, false, fmt.Errorf("%s must be an array", field)
	}
	return got, true, nil
}

func stringIn(target string, values []string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
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
