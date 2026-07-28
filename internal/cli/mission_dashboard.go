package cli

import (
	"flag"
	"fmt"
	"strings"
)

func (a App) missionDashboard(args []string) int {
	var dashboardPath string
	var jsonOut, compact, terminalCard bool
	fs := flag.NewFlagSet("mission dashboard", flag.ContinueOnError)
	fs.SetOutput(a.Stderr)
	fs.StringVar(&dashboardPath, "dashboard", "", "path to AO Mission dashboard readback JSON")
	fs.BoolVar(&compact, "compact", false, "emit compact long-run mission status")
	fs.BoolVar(&terminalCard, "terminal-card", false, "emit terminal rollup card for operator handoff")
	fs.BoolVar(&jsonOut, "json", false, "emit JSON")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if strings.TrimSpace(dashboardPath) == "" {
		fmt.Fprintln(a.Stderr, "ao-command mission dashboard: --dashboard is required")
		return 2
	}
	summary, err := readMissionDashboardReadback(dashboardPath)
	if err != nil {
		fmt.Fprintf(a.Stderr, "ao-command mission dashboard: %v\n", err)
		return 1
	}
	if terminalCard {
		card := buildMissionTerminalRollupCard(summary)
		if jsonOut {
			return a.writeJSON(card)
		}
		fmt.Fprintf(a.Stdout, "terminal_rollup_card=mission=%s status=%s route=%s nodes=%d/%d ready=%d blocked=%d failed=%d\n",
			card.MissionID,
			card.Status,
			card.LatestRoute,
			card.CompletedNodes,
			card.TotalNodes,
			card.ReadyNodes,
			card.BlockedNodes,
			card.FailedNodes,
		)
		fmt.Fprintf(a.Stdout, "terminal_rollup_evidence=foundry=%s promoter=%s command=%s return_gate=%s final_response_allowed=%t\n",
			card.FoundryRollupStatus,
			card.PromoterStatus,
			card.CommandReadbackStatus,
			card.ReturnGateStatus,
			card.FinalResponseAllowed,
		)
		fmt.Fprintf(a.Stdout, "terminal_rollup_safety=promotion_claimed=%t rsi_remains_denied=%t safe_to_execute=%t executes_work=%t approves_work=%t mutates_repositories=%t\n",
			card.PromotionClaimed,
			card.RSIRemainsDenied,
			card.SafeToExecute,
			card.ExecutesWork,
			card.ApprovesWork,
			card.MutatesRepositories,
		)
		fmt.Fprintf(a.Stdout, "exact_next_action=%s\n", card.ExactNextAction)
		return 0
	}
	if jsonOut {
		return a.writeJSON(summary)
	}
	if compact {
		fmt.Fprintf(a.Stdout, "compact_mission_status=mission=%s status=%s route=%s latest_route=%s events=%d\n", summary.MissionID, summary.MissionStatus, summary.CurrentRoute, summary.LatestRoute, summary.EventCount)
		if summary.hasLongRunStatus() {
			fmt.Fprintf(a.Stdout, "compact_long_run_status=nodes=%d/%d min_nodes=%d ready=%d blocked=%d failed=%d checkpoints=%d elapsed_minutes=%d min_minutes=%d lease=%s return_gate=%s final_response_allowed=%t\n",
				summary.CompletedNodes,
				summary.TotalNodes,
				summary.MinimumNodes,
				summary.ReadyNodes,
				summary.BlockedNodes,
				summary.FailedNodes,
				summary.CheckpointCount,
				summary.ElapsedMinutes,
				summary.MinMinutes,
				summary.LeaseHealthStatus,
				summary.ReturnGateStatus,
				summary.FinalResponseAllowed,
			)
			fmt.Fprintf(a.Stdout, "compact_readback_statuses=workgraph=%s foundry=%s promoter=%s command=%s checkpoint_freshness=%s early_return_risk=%s\n",
				summary.WorkgraphStatus,
				summary.FoundryRollupStatus,
				summary.PromoterStatus,
				summary.CommandReadbackStatus,
				summary.CheckpointFreshnessStatus,
				summary.EarlyReturnRiskStatus,
			)
		}
		fmt.Fprintf(a.Stdout, "safe_to_execute=%t\n", summary.SafeToExecute)
		fmt.Fprintf(a.Stdout, "executes_work=%t\n", summary.ExecutesWork)
		fmt.Fprintf(a.Stdout, "approves_work=%t\n", summary.ApprovesWork)
		fmt.Fprintf(a.Stdout, "mutates_repositories=%t\n", summary.MutatesRepositories)
		fmt.Fprintf(a.Stdout, "exact_next_action=%s\n", summary.ExactNextAction)
		return 0
	}
	fmt.Fprintf(a.Stdout, "ao_command_mission_dashboard=%s\n", summary.Status)
	fmt.Fprintf(a.Stdout, "mission_id=%s\n", summary.MissionID)
	fmt.Fprintf(a.Stdout, "mission_status=%s\n", summary.MissionStatus)
	fmt.Fprintf(a.Stdout, "current_route=%s\n", summary.CurrentRoute)
	fmt.Fprintf(a.Stdout, "latest_route=%s\n", summary.LatestRoute)
	fmt.Fprintf(a.Stdout, "event_count=%d\n", summary.EventCount)
	fmt.Fprintf(a.Stdout, "compact=%t\n", summary.Compact)
	fmt.Fprintf(a.Stdout, "operator_mode=%s\n", summary.OperatorMode)
	fmt.Fprintf(a.Stdout, "safe_to_execute=%t\n", summary.SafeToExecute)
	fmt.Fprintf(a.Stdout, "executes_work=%t\n", summary.ExecutesWork)
	fmt.Fprintf(a.Stdout, "approves_work=%t\n", summary.ApprovesWork)
	fmt.Fprintf(a.Stdout, "mutates_repositories=%t\n", summary.MutatesRepositories)
	fmt.Fprintf(a.Stdout, "exact_next_action=%s\n", summary.ExactNextAction)
	return 0
}

type missionDashboardSummary struct {
	CommandSchemaVersion      string `json:"command_schema_version"`
	Schema                    string `json:"schema"`
	MissionID                 string `json:"mission_id"`
	Status                    string `json:"status"`
	MissionStatus             string `json:"mission_status"`
	CurrentRoute              string `json:"current_route"`
	LatestRoute               string `json:"latest_route"`
	WorkgraphStatus           string `json:"workgraph_status,omitempty"`
	FoundryRollupStatus       string `json:"foundry_rollup_status,omitempty"`
	PromoterStatus            string `json:"promoter_status,omitempty"`
	CommandReadbackStatus     string `json:"command_readback_status,omitempty"`
	TotalNodes                int    `json:"total_nodes,omitempty"`
	MinimumNodes              int    `json:"minimum_nodes,omitempty"`
	CompletedNodes            int    `json:"completed_nodes,omitempty"`
	ReadyNodes                int    `json:"ready_nodes,omitempty"`
	BlockedNodes              int    `json:"blocked_nodes,omitempty"`
	FailedNodes               int    `json:"failed_nodes,omitempty"`
	CheckpointCount           int    `json:"checkpoint_count,omitempty"`
	MinMinutes                int    `json:"min_minutes,omitempty"`
	ElapsedMinutes            int    `json:"elapsed_minutes,omitempty"`
	LeaseHealthStatus         string `json:"lease_health_status,omitempty"`
	CheckpointFreshnessStatus string `json:"checkpoint_freshness_status,omitempty"`
	ReturnGateStatus          string `json:"return_gate_status,omitempty"`
	EarlyReturnRiskStatus     string `json:"early_return_risk_status,omitempty"`
	FinalResponseAllowed      bool   `json:"final_response_allowed"`
	PromotionClaimed          bool   `json:"promotion_claimed"`
	RSIRemainsDenied          bool   `json:"rsi_remains_denied"`
	EventCount                int    `json:"event_count"`
	EventIndexDigest          string `json:"event_index_digest"`
	Compact                   bool   `json:"compact"`
	OperatorMode              string `json:"operator_mode"`
	SafeToExecute             bool   `json:"safe_to_execute"`
	ExecutesWork              bool   `json:"executes_work"`
	ApprovesWork              bool   `json:"approves_work"`
	MutatesRepositories       bool   `json:"mutates_repositories"`
	ExactNextAction           string `json:"exact_next_action"`
}

func (s missionDashboardSummary) hasLongRunStatus() bool {
	return s.TotalNodes > 0 ||
		s.MinimumNodes > 0 ||
		s.CompletedNodes > 0 ||
		s.ReadyNodes > 0 ||
		s.BlockedNodes > 0 ||
		s.FailedNodes > 0 ||
		s.CheckpointCount > 0 ||
		s.MinMinutes > 0 ||
		s.ElapsedMinutes > 0 ||
		strings.TrimSpace(s.WorkgraphStatus) != "" ||
		strings.TrimSpace(s.FoundryRollupStatus) != "" ||
		strings.TrimSpace(s.PromoterStatus) != "" ||
		strings.TrimSpace(s.CommandReadbackStatus) != "" ||
		strings.TrimSpace(s.LeaseHealthStatus) != "" ||
		strings.TrimSpace(s.CheckpointFreshnessStatus) != "" ||
		strings.TrimSpace(s.ReturnGateStatus) != "" ||
		strings.TrimSpace(s.EarlyReturnRiskStatus) != ""
}

func readMissionDashboardReadback(path string) (missionDashboardSummary, error) {
	var input struct {
		Schema                    string `json:"schema"`
		MissionID                 string `json:"mission_id"`
		Status                    string `json:"status"`
		MissionStatus             string `json:"mission_status"`
		CurrentRoute              string `json:"current_route"`
		LatestRoute               string `json:"latest_route"`
		WorkgraphStatus           string `json:"workgraph_status"`
		FoundryRollupStatus       string `json:"foundry_rollup_status"`
		PromoterStatus            string `json:"promoter_status"`
		CommandReadbackStatus     string `json:"command_readback_status"`
		TotalNodes                int    `json:"total_nodes"`
		MinimumNodes              int    `json:"minimum_nodes"`
		CompletedNodes            int    `json:"completed_nodes"`
		ReadyNodes                int    `json:"ready_nodes"`
		BlockedNodes              int    `json:"blocked_nodes"`
		FailedNodes               int    `json:"failed_nodes"`
		CheckpointCount           int    `json:"checkpoint_count"`
		MinMinutes                int    `json:"min_minutes"`
		ElapsedMinutes            int    `json:"elapsed_minutes"`
		LeaseHealthStatus         string `json:"lease_health_status"`
		CheckpointFreshnessStatus string `json:"checkpoint_freshness_status"`
		ReturnGateStatus          string `json:"return_gate_status"`
		EarlyReturnRiskStatus     string `json:"early_return_risk_status"`
		FinalResponseAllowed      bool   `json:"final_response_allowed"`
		PromotionClaimed          bool   `json:"promotion_claimed"`
		RSIRemainsDenied          bool   `json:"rsi_remains_denied"`
		EventCount                int    `json:"event_count"`
		EventIndexDigest          string `json:"event_index_digest"`
		Compact                   bool   `json:"compact"`
		SafeToExecute             bool   `json:"safe_to_execute"`
		ExecutesWork              bool   `json:"executes_work"`
		ApprovesWork              bool   `json:"approves_work"`
		MutatesRepositories       bool   `json:"mutates_repositories"`
		ExactNextAction           string `json:"exact_next_action"`
	}
	if err := readJSONFile(path, &input); err != nil {
		return missionDashboardSummary{}, err
	}
	if input.Schema != "ao.mission.dashboard-readback.v0.1" {
		return missionDashboardSummary{}, fmt.Errorf("unsupported mission dashboard schema %q", input.Schema)
	}
	if strings.TrimSpace(input.MissionID) == "" {
		return missionDashboardSummary{}, fmt.Errorf("mission dashboard readback requires mission_id")
	}
	if input.SafeToExecute || input.ExecutesWork || input.ApprovesWork || input.MutatesRepositories {
		return missionDashboardSummary{}, fmt.Errorf("mission dashboard readback must not claim execution, approval, or repository mutation authority")
	}
	status := input.Status
	if strings.TrimSpace(status) == "" {
		status = "ready"
	}
	return missionDashboardSummary{
		CommandSchemaVersion:      commandSchemaVersion,
		Schema:                    "ao.command.mission-dashboard.v0.1",
		MissionID:                 input.MissionID,
		Status:                    status,
		MissionStatus:             input.MissionStatus,
		CurrentRoute:              input.CurrentRoute,
		LatestRoute:               input.LatestRoute,
		WorkgraphStatus:           input.WorkgraphStatus,
		FoundryRollupStatus:       input.FoundryRollupStatus,
		PromoterStatus:            input.PromoterStatus,
		CommandReadbackStatus:     input.CommandReadbackStatus,
		TotalNodes:                input.TotalNodes,
		MinimumNodes:              input.MinimumNodes,
		CompletedNodes:            input.CompletedNodes,
		ReadyNodes:                input.ReadyNodes,
		BlockedNodes:              input.BlockedNodes,
		FailedNodes:               input.FailedNodes,
		CheckpointCount:           input.CheckpointCount,
		MinMinutes:                input.MinMinutes,
		ElapsedMinutes:            input.ElapsedMinutes,
		LeaseHealthStatus:         input.LeaseHealthStatus,
		CheckpointFreshnessStatus: input.CheckpointFreshnessStatus,
		ReturnGateStatus:          input.ReturnGateStatus,
		EarlyReturnRiskStatus:     input.EarlyReturnRiskStatus,
		FinalResponseAllowed:      input.FinalResponseAllowed,
		PromotionClaimed:          input.PromotionClaimed,
		RSIRemainsDenied:          input.RSIRemainsDenied,
		EventCount:                input.EventCount,
		EventIndexDigest:          input.EventIndexDigest,
		Compact:                   input.Compact,
		OperatorMode:              operatorMode,
		SafeToExecute:             false,
		ExecutesWork:              false,
		ApprovesWork:              false,
		MutatesRepositories:       false,
		ExactNextAction:           input.ExactNextAction,
	}, nil
}
