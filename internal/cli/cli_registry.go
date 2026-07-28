package cli

import (
	"context"
	"fmt"
	"io"
)

type rootCommandEntry struct {
	names   []string
	handler func(App, context.Context, []string) int
}

type missionCommandEntry struct {
	names   []string
	handler func(App, []string) int
}

var rootCommandRegistry = []rootCommandEntry{
	{names: []string{"help", "--help", "-h"}, handler: runRootHelp},
	{names: []string{"version"}, handler: runRootVersion},
	{names: []string{"status"}, handler: runRootStatus},
	{names: []string{"stack"}, handler: runRootStack},
	{names: []string{"atlas"}, handler: runRootAtlas},
	{names: []string{"pulse"}, handler: runRootPulse},
	{names: []string{"blueprint-atlas-foundry"}, handler: runRootBlueprintAtlasFoundry},
	{names: []string{"complex-refactor"}, handler: runRootComplexRefactor},
	{names: []string{"live-mutation"}, handler: runRootLiveMutation},
	{names: []string{"mission"}, handler: runRootMission},
	{names: []string{"control-plane"}, handler: runRootControlPlane},
	{names: []string{"controlled-loop"}, handler: runRootControlledLoop},
	{names: []string{"operator"}, handler: runRootOperator},
	{names: []string{"covenant"}, handler: runRootCovenant},
	{names: []string{"forge"}, handler: runRootForge},
	{names: []string{"promoter"}, handler: runRootPromoter},
	{names: []string{"rsi"}, handler: runRootRSI},
	{names: []string{"next"}, handler: runRootNext},
	{names: []string{"goals"}, handler: runRootGoals},
	{names: []string{"evidence"}, handler: runRootEvidence},
	{names: []string{"rehearse"}, handler: runRootRehearse},
}

var missionCommandRegistry = []missionCommandEntry{
	{names: []string{"aggregate"}, handler: runMissionAggregate},
	{names: []string{"approvals"}, handler: runMissionApprovals},
	{names: []string{"artifacts"}, handler: runMissionArtifacts},
	{names: []string{"dashboard"}, handler: runMissionDashboard},
	{names: []string{"evidence"}, handler: runMissionEvidence},
	{names: []string{"gateway"}, handler: runMissionGateway},
	{names: []string{"history"}, handler: runMissionHistory},
	{names: []string{"next"}, handler: runMissionNext},
	{names: []string{"readiness"}, handler: runMissionReadiness},
	{names: []string{"status"}, handler: runMissionStatus},
	{names: []string{"timeline"}, handler: runMissionTimeline},
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

	for _, command := range rootCommandRegistry {
		for _, name := range command.names {
			if args[0] == name {
				return command.handler(a, ctx, args[1:])
			}
		}
	}
	fmt.Fprintf(a.Stderr, "ao-command: unknown command %q\n", args[0])
	a.printHelp()
	return 2
}

func (a App) printHelp() {
	fmt.Fprintln(a.Stdout, `ao-command is the read-only operator command surface for the AO2-first AO stack.

Usage:
  ao-command version [--json]
  ao-command status [--forge PATH] [--forge-bin PATH] [--json]
  ao-command stack --ledger PATH [--json]
  ao-command atlas status --status PATH [--json]
  ao-command atlas authority-ladder --mission-status PATH [--json]
  ao-command mission aggregate --status PATH --atlas-metadata PATH --foundry-smoke PATH [--json]
  ao-command mission approvals --inbox PATH [--ticket-id ID] [--json]
  ao-command mission status --status PATH [--json]
  ao-command mission next --decision PATH [--json]
  ao-command mission history --history PATH [--route ROUTE] [--status-filter STATUS] [--query TEXT] [--pilot-readiness] [--compact] [--json]
  ao-command mission timeline --readback PATH [--json]
  ao-command mission artifacts --manifest PATH [--json]
  ao-command mission dashboard --dashboard PATH [--compact] [--terminal-card] [--json]
  ao-command mission readiness --bundle PATH [--json]
  ao-command mission gateway --readback PATH [--json]
  ao-command mission evidence --readback PATH [--json]
  ao-command control-plane boundary --packet PATH [--json]
  ao-command control-plane qualification-progress --readback PATH [--json]
  ao-command control-plane status --readback PATH [--json]
  ao-command operator status --readback PATH [--at RFC3339] [--json]
  ao-command operator workflow --readback PATH [--json]
  ao-command covenant policy --readback PATH [--json]
  ao-command controlled-loop status --readback PATH [--json]
  ao-command forge timeline --readback PATH [--json]
  ao-command promoter status --readback PATH [--json]
  ao-command pulse status --preflight PATH --lifecycle PATH --start-gate PATH [--json]
  ao-command blueprint-atlas-foundry status --atlas-blueprint-import PATH --preflight PATH --foundry-gate PATH [--json]
  ao-command complex-refactor status --summary PATH [--json]
  ao-command live-mutation status --authority PATH --request PATH --forge-plan PATH --ao2-packet PATH --isolation PATH --rollback PATH --kill-switch PATH [--json]
  ao-command live-mutation approval --request PATH --ticket PATH [--json]
  ao-command live-mutation pr-rehearsal --gate PATH [--json]
  ao-command live-mutation class-decision --rollup PATH --promoter-verdict PATH [--json]
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

func (a App) mission(args []string) int {
	if len(args) == 0 {
		fmt.Fprintln(a.Stderr, missionUsage())
		return 2
	}
	for _, command := range missionCommandRegistry {
		for _, name := range command.names {
			if args[0] == name {
				return command.handler(a, args[1:])
			}
		}
	}
	fmt.Fprintln(a.Stderr, missionUsage())
	return 2
}

func missionUsage() string {
	return "ao-command mission: usage: ao-command mission aggregate --status PATH --atlas-metadata PATH --foundry-smoke PATH [--json] | ao-command mission approvals --inbox PATH [--ticket-id ID] [--json] | ao-command mission status --status PATH [--json] | ao-command mission next --decision PATH [--json] | ao-command mission history --history PATH [--route ROUTE] [--status-filter STATUS] [--query TEXT] [--pilot-readiness] [--compact] [--json] | ao-command mission timeline --readback PATH [--json] | ao-command mission artifacts --manifest PATH [--json] | ao-command mission dashboard --dashboard PATH [--compact] [--terminal-card] [--json] | ao-command mission readiness --bundle PATH [--json] | ao-command mission gateway --readback PATH [--json] | ao-command mission evidence --readback PATH [--json]"
}

func runRootHelp(a App, _ context.Context, _ []string) int {
	a.printHelp()
	return 0
}

func runRootVersion(a App, _ context.Context, args []string) int {
	return a.version(args)
}

func runRootStatus(a App, ctx context.Context, args []string) int {
	return a.status(ctx, args)
}

func runRootStack(a App, _ context.Context, args []string) int {
	return a.stack(args)
}

func runRootAtlas(a App, _ context.Context, args []string) int {
	return a.atlas(args)
}

func runRootPulse(a App, _ context.Context, args []string) int {
	return a.pulse(args)
}

func runRootBlueprintAtlasFoundry(a App, _ context.Context, args []string) int {
	return a.blueprintAtlasFoundry(args)
}

func runRootComplexRefactor(a App, _ context.Context, args []string) int {
	return a.complexRefactor(args)
}

func runRootLiveMutation(a App, _ context.Context, args []string) int {
	return a.liveMutation(args)
}

func runRootMission(a App, _ context.Context, args []string) int {
	return a.mission(args)
}

func runRootControlPlane(a App, _ context.Context, args []string) int {
	return a.controlPlane(args)
}

func runRootControlledLoop(a App, _ context.Context, args []string) int {
	return a.controlledLoop(args)
}

func runRootOperator(a App, _ context.Context, args []string) int {
	return a.operator(args)
}

func runRootCovenant(a App, _ context.Context, args []string) int {
	return a.covenant(args)
}

func runRootForge(a App, _ context.Context, args []string) int {
	return a.forge(args)
}

func runRootPromoter(a App, _ context.Context, args []string) int {
	return a.promoter(args)
}

func runRootRSI(a App, _ context.Context, args []string) int {
	return a.rsi(args)
}

func runRootNext(a App, ctx context.Context, args []string) int {
	return a.next(ctx, args)
}

func runRootGoals(a App, ctx context.Context, args []string) int {
	return a.goals(ctx, args)
}

func runRootEvidence(a App, ctx context.Context, args []string) int {
	return a.evidence(ctx, args)
}

func runRootRehearse(a App, ctx context.Context, args []string) int {
	return a.rehearse(ctx, args)
}

func runMissionAggregate(a App, args []string) int {
	return a.missionAggregate(args)
}

func runMissionApprovals(a App, args []string) int {
	return a.missionApprovals(args)
}

func runMissionArtifacts(a App, args []string) int {
	return a.missionArtifacts(args)
}

func runMissionDashboard(a App, args []string) int {
	return a.missionDashboard(args)
}

func runMissionEvidence(a App, args []string) int {
	return a.missionEvidence(args)
}

func runMissionGateway(a App, args []string) int {
	return a.missionGateway(args)
}

func runMissionHistory(a App, args []string) int {
	return a.missionHistory(args)
}

func runMissionNext(a App, args []string) int {
	return a.missionNext(args)
}

func runMissionReadiness(a App, args []string) int {
	return a.missionReadiness(args)
}

func runMissionStatus(a App, args []string) int {
	return a.missionStatus(args)
}

func runMissionTimeline(a App, args []string) int {
	return a.missionTimeline(args)
}
