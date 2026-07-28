package cli

import (
	"flag"
	"fmt"
	"strings"
)

func (a App) missionApprovals(args []string) int {
	var inboxPath, ticketID string
	var jsonOut bool
	fs := flag.NewFlagSet("mission approvals", flag.ContinueOnError)
	fs.SetOutput(a.Stderr)
	fs.StringVar(&inboxPath, "inbox", "", "path to AO Mission approval inbox JSON")
	fs.StringVar(&ticketID, "ticket-id", "", "show one approval ticket by ID")
	fs.BoolVar(&jsonOut, "json", false, "emit JSON")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if strings.TrimSpace(inboxPath) == "" {
		fmt.Fprintln(a.Stderr, "ao-command mission approvals: --inbox is required")
		return 2
	}
	summary, err := readMissionApprovalInbox(inboxPath, ticketID)
	if err != nil {
		fmt.Fprintf(a.Stderr, "ao-command mission approvals: %v\n", err)
		return 1
	}
	if jsonOut {
		return a.writeJSON(summary)
	}
	fmt.Fprintf(a.Stdout, "ao_command_mission_approvals=%s\n", summary.Status)
	fmt.Fprintf(a.Stdout, "mission_id=%s\n", summary.MissionID)
	fmt.Fprintf(a.Stdout, "approval_count=%d\n", summary.ApprovalCount)
	fmt.Fprintf(a.Stdout, "pending_count=%d\n", summary.PendingCount)
	fmt.Fprintf(a.Stdout, "approved_count=%d\n", summary.ApprovedCount)
	fmt.Fprintf(a.Stdout, "denied_count=%d\n", summary.DeniedCount)
	if summary.SelectedTicketID != "" {
		fmt.Fprintf(a.Stdout, "selected_ticket_id=%s\n", summary.SelectedTicketID)
	}
	fmt.Fprintf(a.Stdout, "operator_mode=%s\n", summary.OperatorMode)
	fmt.Fprintf(a.Stdout, "safe_to_execute=%t\n", summary.SafeToExecute)
	fmt.Fprintf(a.Stdout, "executes_work=%t\n", summary.ExecutesWork)
	fmt.Fprintf(a.Stdout, "approves_work=%t\n", summary.ApprovesWork)
	fmt.Fprintf(a.Stdout, "mutates_repositories=%t\n", summary.MutatesRepositories)
	fmt.Fprintf(a.Stdout, "exact_next_action=%s\n", summary.ExactNextAction)
	return 0
}

type missionApprovalTicket struct {
	TicketID       string `json:"ticket_id"`
	RequestID      string `json:"request_id"`
	Status         string `json:"status"`
	Requester      string `json:"requester"`
	Scope          string `json:"scope"`
	Summary        string `json:"summary"`
	CreatedAtUTC   string `json:"created_at_utc"`
	DecisionAtUTC  string `json:"decision_at_utc,omitempty"`
	DecisionReason string `json:"decision_reason,omitempty"`
}

type missionApprovalInboxSummary struct {
	CommandSchemaVersion string                  `json:"command_schema_version"`
	Schema               string                  `json:"schema"`
	MissionID            string                  `json:"mission_id"`
	Status               string                  `json:"status"`
	OperatorMode         string                  `json:"operator_mode"`
	ApprovalCount        int                     `json:"approval_count"`
	PendingCount         int                     `json:"pending_count"`
	ApprovedCount        int                     `json:"approved_count"`
	DeniedCount          int                     `json:"denied_count"`
	SelectedTicketID     string                  `json:"selected_ticket_id,omitempty"`
	Approvals            []missionApprovalTicket `json:"approvals"`
	SafeToExecute        bool                    `json:"safe_to_execute"`
	ExecutesWork         bool                    `json:"executes_work"`
	ApprovesWork         bool                    `json:"approves_work"`
	MutatesRepositories  bool                    `json:"mutates_repositories"`
	ExactNextAction      string                  `json:"exact_next_action"`
}

func readMissionApprovalInbox(path, selectedTicketID string) (missionApprovalInboxSummary, error) {
	var input struct {
		Schema              string                  `json:"schema"`
		MissionID           string                  `json:"mission_id"`
		Status              string                  `json:"status"`
		OperatorMode        string                  `json:"operator_mode"`
		Approvals           []missionApprovalTicket `json:"approvals"`
		SafeToExecute       bool                    `json:"safe_to_execute"`
		ExecutesWork        bool                    `json:"executes_work"`
		ApprovesWork        bool                    `json:"approves_work"`
		MutatesRepositories bool                    `json:"mutates_repositories"`
		ExactNextAction     string                  `json:"exact_next_action"`
	}
	if err := readJSONFile(path, &input); err != nil {
		return missionApprovalInboxSummary{}, err
	}
	if input.Schema != "ao.command.mission-approval-inbox.v0.1" {
		return missionApprovalInboxSummary{}, fmt.Errorf("schema must be ao.command.mission-approval-inbox.v0.1")
	}
	if input.MissionID == "" || input.Status == "" || input.OperatorMode == "" {
		return missionApprovalInboxSummary{}, fmt.Errorf("mission approval inbox requires mission_id, status, and operator_mode")
	}
	if input.OperatorMode != operatorMode {
		return missionApprovalInboxSummary{}, fmt.Errorf("operator_mode must be %s", operatorMode)
	}
	if input.SafeToExecute || input.ExecutesWork || input.ApprovesWork || input.MutatesRepositories {
		return missionApprovalInboxSummary{}, fmt.Errorf("mission approval inbox must not claim execution, approval, or repository mutation authority")
	}
	selected := strings.TrimSpace(selectedTicketID)
	approvals := []missionApprovalTicket{}
	for _, approval := range input.Approvals {
		if approval.TicketID == "" || approval.Status == "" {
			return missionApprovalInboxSummary{}, fmt.Errorf("mission approval inbox tickets require ticket_id and status")
		}
		switch approval.Status {
		case "pending", "approved", "denied":
		default:
			return missionApprovalInboxSummary{}, fmt.Errorf("mission approval inbox ticket %s has unsupported status %s", approval.TicketID, approval.Status)
		}
		if selected == "" || approval.TicketID == selected {
			approvals = append(approvals, approval)
		}
	}
	if selected != "" && len(approvals) == 0 {
		return missionApprovalInboxSummary{}, fmt.Errorf("approval ticket %s not found", selected)
	}
	summary := missionApprovalInboxSummary{
		CommandSchemaVersion: commandSchemaVersion,
		Schema:               input.Schema,
		MissionID:            input.MissionID,
		Status:               input.Status,
		OperatorMode:         input.OperatorMode,
		ApprovalCount:        len(input.Approvals),
		SelectedTicketID:     selected,
		Approvals:            approvals,
		SafeToExecute:        false,
		ExecutesWork:         false,
		ApprovesWork:         false,
		MutatesRepositories:  false,
		ExactNextAction:      input.ExactNextAction,
	}
	for _, approval := range input.Approvals {
		switch approval.Status {
		case "pending":
			summary.PendingCount++
		case "approved":
			summary.ApprovedCount++
		case "denied":
			summary.DeniedCount++
		}
	}
	return summary, nil
}
