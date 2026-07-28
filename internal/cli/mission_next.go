package cli

import (
	"flag"
	"fmt"
	"strings"
)

func (a App) missionNext(args []string) int {
	var decisionPath string
	var jsonOut bool
	fs := flag.NewFlagSet("mission next", flag.ContinueOnError)
	fs.SetOutput(a.Stderr)
	fs.StringVar(&decisionPath, "decision", "", "path to AO Mission route decision JSON")
	fs.BoolVar(&jsonOut, "json", false, "emit JSON")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if strings.TrimSpace(decisionPath) == "" {
		fmt.Fprintln(a.Stderr, "ao-command mission next: --decision is required")
		return 2
	}
	summary, err := readMissionNextDecision(decisionPath)
	if err != nil {
		fmt.Fprintf(a.Stderr, "ao-command mission next: %v\n", err)
		return 1
	}
	if jsonOut {
		return a.writeJSON(summary)
	}
	fmt.Fprintf(a.Stdout, "ao_command_mission_next=%s\n", summary.Status)
	fmt.Fprintf(a.Stdout, "mission_id=%s\n", summary.MissionID)
	fmt.Fprintf(a.Stdout, "route=%s\n", summary.Route)
	fmt.Fprintf(a.Stdout, "reason=%s\n", summary.Reason)
	fmt.Fprintf(a.Stdout, "safe_to_request=%t\n", summary.SafeToRequest)
	fmt.Fprintf(a.Stdout, "safe_to_execute=%t\n", summary.SafeToExecute)
	fmt.Fprintf(a.Stdout, "executes_work=%t\n", summary.ExecutesWork)
	fmt.Fprintf(a.Stdout, "approves_work=%t\n", summary.ApprovesWork)
	fmt.Fprintf(a.Stdout, "mutates_repositories=%t\n", summary.MutatesRepositories)
	fmt.Fprintf(a.Stdout, "exact_next_action=%s\n", summary.ExactNextAction)
	return 0
}

type missionNextSummary struct {
	CommandSchemaVersion string `json:"command_schema_version"`
	Schema               string `json:"schema"`
	MissionID            string `json:"mission_id"`
	Status               string `json:"status"`
	Route                string `json:"route"`
	Reason               string `json:"reason"`
	OperatorMode         string `json:"operator_mode"`
	SafeToRequest        bool   `json:"safe_to_request"`
	SafeToExecute        bool   `json:"safe_to_execute"`
	ExecutesWork         bool   `json:"executes_work"`
	ApprovesWork         bool   `json:"approves_work"`
	MutatesRepositories  bool   `json:"mutates_repositories"`
	ExactNextAction      string `json:"exact_next_action"`
}

func readMissionNextDecision(path string) (missionNextSummary, error) {
	var input struct {
		Schema              string `json:"schema"`
		MissionID           string `json:"mission_id"`
		Status              string `json:"status"`
		Route               string `json:"route"`
		Reason              string `json:"reason"`
		OperatorMode        string `json:"operator_mode"`
		SafeToRequest       bool   `json:"safe_to_request"`
		SafeToExecute       bool   `json:"safe_to_execute"`
		ExecutesWork        bool   `json:"executes_work"`
		ApprovesWork        bool   `json:"approves_work"`
		MutatesRepositories bool   `json:"mutates_repositories"`
		ExactNextAction     string `json:"exact_next_action"`
	}
	if err := readJSONFile(path, &input); err != nil {
		return missionNextSummary{}, err
	}
	if input.Schema != "ao.mission.route-decision.v0.1" {
		return missionNextSummary{}, fmt.Errorf("schema must be ao.mission.route-decision.v0.1")
	}
	if input.MissionID == "" || input.Status == "" || input.Route == "" || input.OperatorMode == "" || input.ExactNextAction == "" {
		return missionNextSummary{}, fmt.Errorf("mission next requires mission_id, status, route, operator_mode, and exact_next_action")
	}
	if input.OperatorMode != operatorMode {
		return missionNextSummary{}, fmt.Errorf("operator_mode must be %s", operatorMode)
	}
	if input.SafeToExecute || input.ExecutesWork || input.ApprovesWork || input.MutatesRepositories {
		return missionNextSummary{}, fmt.Errorf("mission next must not claim execution, approval, or repository mutation authority")
	}
	return missionNextSummary{
		CommandSchemaVersion: commandSchemaVersion,
		Schema:               input.Schema,
		MissionID:            input.MissionID,
		Status:               input.Status,
		Route:                input.Route,
		Reason:               input.Reason,
		OperatorMode:         input.OperatorMode,
		SafeToRequest:        input.SafeToRequest,
		SafeToExecute:        false,
		ExecutesWork:         false,
		ApprovesWork:         false,
		MutatesRepositories:  false,
		ExactNextAction:      input.ExactNextAction,
	}, nil
}
