package cli

import "strings"

type missionTerminalRollupCard struct {
	CommandSchemaVersion  string `json:"command_schema_version"`
	Schema                string `json:"schema"`
	Status                string `json:"status"`
	MissionID             string `json:"mission_id"`
	MissionStatus         string `json:"mission_status"`
	LatestRoute           string `json:"latest_route"`
	CompletedNodes        int    `json:"completed_nodes"`
	TotalNodes            int    `json:"total_nodes"`
	ReadyNodes            int    `json:"ready_nodes"`
	BlockedNodes          int    `json:"blocked_nodes"`
	FailedNodes           int    `json:"failed_nodes"`
	FoundryRollupStatus   string `json:"foundry_rollup_status"`
	PromoterStatus        string `json:"promoter_status"`
	CommandReadbackStatus string `json:"command_readback_status"`
	ReturnGateStatus      string `json:"return_gate_status"`
	FinalResponseAllowed  bool   `json:"final_response_allowed"`
	PromotionClaimed      bool   `json:"promotion_claimed"`
	RSIRemainsDenied      bool   `json:"rsi_remains_denied"`
	OperatorMode          string `json:"operator_mode"`
	SafeToExecute         bool   `json:"safe_to_execute"`
	ExecutesWork          bool   `json:"executes_work"`
	ApprovesWork          bool   `json:"approves_work"`
	MutatesRepositories   bool   `json:"mutates_repositories"`
	ExactNextAction       string `json:"exact_next_action"`
}

func buildMissionTerminalRollupCard(summary missionDashboardSummary) missionTerminalRollupCard {
	status := "terminal_handoff_review"
	if summary.MissionStatus == "done" &&
		summary.FinalResponseAllowed &&
		summary.CompletedNodes == summary.TotalNodes &&
		summary.ReadyNodes == 0 &&
		summary.BlockedNodes == 0 &&
		summary.FailedNodes == 0 &&
		summary.ReturnGateStatus == "final_response_allowed" {
		status = "terminal_handoff_ready"
	}
	exactNextAction := strings.TrimSpace(summary.ExactNextAction)
	if exactNextAction == "" {
		exactNextAction = "review terminal rollup card; continue only through AO Mission governed recommendations"
	}
	return missionTerminalRollupCard{
		CommandSchemaVersion:  commandSchemaVersion,
		Schema:                "ao.command.mission-terminal-rollup-card.v0.1",
		Status:                status,
		MissionID:             summary.MissionID,
		MissionStatus:         summary.MissionStatus,
		LatestRoute:           summary.LatestRoute,
		CompletedNodes:        summary.CompletedNodes,
		TotalNodes:            summary.TotalNodes,
		ReadyNodes:            summary.ReadyNodes,
		BlockedNodes:          summary.BlockedNodes,
		FailedNodes:           summary.FailedNodes,
		FoundryRollupStatus:   summary.FoundryRollupStatus,
		PromoterStatus:        summary.PromoterStatus,
		CommandReadbackStatus: summary.CommandReadbackStatus,
		ReturnGateStatus:      summary.ReturnGateStatus,
		FinalResponseAllowed:  summary.FinalResponseAllowed,
		PromotionClaimed:      summary.PromotionClaimed,
		RSIRemainsDenied:      summary.RSIRemainsDenied,
		OperatorMode:          operatorMode,
		SafeToExecute:         false,
		ExecutesWork:          false,
		ApprovesWork:          false,
		MutatesRepositories:   false,
		ExactNextAction:       exactNextAction,
	}
}
