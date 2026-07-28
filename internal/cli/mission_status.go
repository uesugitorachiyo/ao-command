package cli

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"
)

func (a App) missionStatus(args []string) int {
	var statusPath string
	var jsonOut bool
	fs := flag.NewFlagSet("mission status", flag.ContinueOnError)
	fs.SetOutput(a.Stderr)
	fs.StringVar(&statusPath, "status", "", "path to AO Mission command status JSON")
	fs.BoolVar(&jsonOut, "json", false, "emit JSON")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if strings.TrimSpace(statusPath) == "" {
		fmt.Fprintln(a.Stderr, "ao-command mission status: --status is required")
		return 2
	}
	summary, err := readMissionCommandStatus(statusPath)
	if err != nil {
		fmt.Fprintf(a.Stderr, "ao-command mission status: %v\n", err)
		return 1
	}
	if jsonOut {
		return a.writeJSON(summary)
	}
	fmt.Fprintf(a.Stdout, "ao_command_mission_status=%s\n", summary.Status)
	fmt.Fprintf(a.Stdout, "mission_id=%s\n", summary.MissionID)
	fmt.Fprintf(a.Stdout, "current_route=%s\n", summary.CurrentRoute)
	fmt.Fprintf(a.Stdout, "current_phase=%s\n", summary.CurrentPhase)
	if summary.CorrelationID != "" {
		fmt.Fprintf(a.Stdout, "correlation_id=%s\n", summary.CorrelationID)
	}
	fmt.Fprintf(a.Stdout, "operator_mode=%s\n", summary.OperatorMode)
	fmt.Fprintf(a.Stdout, "safe_to_execute=%t\n", summary.SafeToExecute)
	fmt.Fprintf(a.Stdout, "executes_work=%t\n", summary.ExecutesWork)
	fmt.Fprintf(a.Stdout, "approves_work=%t\n", summary.ApprovesWork)
	fmt.Fprintf(a.Stdout, "mutates_repositories=%t\n", summary.MutatesRepositories)
	fmt.Fprintf(a.Stdout, "exact_next_action=%s\n", summary.ExactNextAction)
	return 0
}

type missionCommandStatusSummary struct {
	CommandSchemaVersion string `json:"command_schema_version"`
	Schema               string `json:"schema"`
	MissionID            string `json:"mission_id"`
	Status               string `json:"status"`
	CurrentRoute         string `json:"current_route"`
	CurrentPhase         string `json:"current_phase"`
	CorrelationID        string `json:"correlation_id,omitempty"`
	OperatorMode         string `json:"operator_mode"`
	SafeToExecute        bool   `json:"safe_to_execute"`
	ExecutesWork         bool   `json:"executes_work"`
	ApprovesWork         bool   `json:"approves_work"`
	MutatesRepositories  bool   `json:"mutates_repositories"`
	ExactNextAction      string `json:"exact_next_action"`
}

type missionCommandStatusInput struct {
	Schema               string
	MissionID            string
	Status               string
	CurrentRoute         string
	CurrentPhase         string
	CorrelationID        *string
	OperatorMode         string
	SafeToExecute        bool
	ExecutesWork         bool
	ApprovesWork         bool
	MutatesRepositories  bool
	ExactNextAction      string
	correlationIDPresent bool
}

func readMissionCommandStatus(path string) (missionCommandStatusSummary, error) {
	input, err := readStrictMissionCommandStatus(path)
	if err != nil {
		return missionCommandStatusSummary{}, err
	}
	if input.Schema != "ao.command.mission-status.v0.1" {
		return missionCommandStatusSummary{}, fmt.Errorf("schema must be ao.command.mission-status.v0.1")
	}
	if input.MissionID == "" || input.Status == "" || input.CurrentRoute == "" || input.OperatorMode == "" {
		return missionCommandStatusSummary{}, fmt.Errorf("mission status requires mission_id, status, current_route, and operator_mode")
	}
	if input.OperatorMode != operatorMode {
		return missionCommandStatusSummary{}, fmt.Errorf("operator_mode must be %s", operatorMode)
	}
	if input.SafeToExecute || input.ExecutesWork || input.ApprovesWork || input.MutatesRepositories {
		return missionCommandStatusSummary{}, fmt.Errorf("mission status must not claim execution, approval, or repository mutation authority")
	}
	if input.correlationIDPresent && input.CorrelationID == nil {
		return missionCommandStatusSummary{}, fmt.Errorf("correlation_id must be a string")
	}
	if input.CorrelationID != nil && !correlationIDPattern.MatchString(*input.CorrelationID) {
		return missionCommandStatusSummary{}, fmt.Errorf("correlation_id must match [A-Za-z0-9][A-Za-z0-9._:-]{0,127}")
	}
	correlationID := ""
	if input.CorrelationID != nil {
		correlationID = *input.CorrelationID
	}
	return missionCommandStatusSummary{
		CommandSchemaVersion: commandSchemaVersion,
		Schema:               input.Schema,
		MissionID:            input.MissionID,
		Status:               input.Status,
		CurrentRoute:         input.CurrentRoute,
		CurrentPhase:         input.CurrentPhase,
		CorrelationID:        correlationID,
		OperatorMode:         input.OperatorMode,
		SafeToExecute:        false,
		ExecutesWork:         false,
		ApprovesWork:         false,
		MutatesRepositories:  false,
		ExactNextAction:      input.ExactNextAction,
	}, nil
}

func readStrictMissionCommandStatus(path string) (missionCommandStatusInput, error) {
	file, err := os.Open(path)
	if err != nil {
		return missionCommandStatusInput{}, err
	}
	defer file.Close()

	decoder := json.NewDecoder(file)
	token, err := decoder.Token()
	if err != nil {
		return missionCommandStatusInput{}, fmt.Errorf("invalid JSON: %w", err)
	}
	if delimiter, ok := token.(json.Delim); !ok || delimiter != '{' {
		return missionCommandStatusInput{}, fmt.Errorf("invalid JSON: mission status must be an object")
	}

	var input missionCommandStatusInput
	seen := make(map[string]struct{})
	for decoder.More() {
		token, err := decoder.Token()
		if err != nil {
			return missionCommandStatusInput{}, fmt.Errorf("invalid JSON: %w", err)
		}
		field, ok := token.(string)
		if !ok {
			return missionCommandStatusInput{}, fmt.Errorf("invalid JSON: mission status field name must be a string")
		}
		if _, ok := seen[field]; ok {
			return missionCommandStatusInput{}, fmt.Errorf("invalid JSON: duplicate field %q", field)
		}
		seen[field] = struct{}{}

		var target any
		switch field {
		case "schema":
			target = &input.Schema
		case "mission_id":
			target = &input.MissionID
		case "status":
			target = &input.Status
		case "current_route":
			target = &input.CurrentRoute
		case "current_phase":
			target = &input.CurrentPhase
		case "correlation_id":
			input.correlationIDPresent = true
			target = &input.CorrelationID
		case "operator_mode":
			target = &input.OperatorMode
		case "safe_to_execute":
			target = &input.SafeToExecute
		case "executes_work":
			target = &input.ExecutesWork
		case "approves_work":
			target = &input.ApprovesWork
		case "mutates_repositories":
			target = &input.MutatesRepositories
		case "exact_next_action":
			target = &input.ExactNextAction
		default:
			return missionCommandStatusInput{}, fmt.Errorf("invalid JSON: unknown field %q", field)
		}
		if err := decoder.Decode(target); err != nil {
			return missionCommandStatusInput{}, fmt.Errorf("invalid JSON: %w", err)
		}
	}

	token, err = decoder.Token()
	if err != nil {
		return missionCommandStatusInput{}, fmt.Errorf("invalid JSON: %w", err)
	}
	if delimiter, ok := token.(json.Delim); !ok || delimiter != '}' {
		return missionCommandStatusInput{}, fmt.Errorf("invalid JSON: mission status must end with an object")
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		if err == nil {
			return missionCommandStatusInput{}, fmt.Errorf("invalid JSON: multiple JSON values")
		}
		return missionCommandStatusInput{}, fmt.Errorf("invalid JSON: %w", err)
	}
	return input, nil
}
