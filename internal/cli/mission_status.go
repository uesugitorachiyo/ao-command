package cli

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"
	"time"
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
	Schema                     string
	MissionID                  string
	Status                     string
	CurrentRoute               string
	CurrentPhase               string
	CorrelationID              *string
	OperatorMode               string
	ReadOnly                   bool
	SafeToExecute              bool
	ExecutesWork               bool
	ApprovesWork               bool
	MutatesRepositories        bool
	ExactNextAction            string
	CheckpointFreshnessStatus  string
	CheckpointCount            int
	ReturnGateStatus           string
	GoalLease                  *missionCommandGoalLease
	AtlasRecommendation        *missionCommandAtlasRecommendation
	Blockers                   []string
	GeneratedAtUTC             string
	correlationIDPresent       bool
	readOnlyPresent            bool
	goalLeasePresent           bool
	atlasRecommendationPresent bool
}

type missionCommandGoalLease struct {
	Schema           string `json:"schema"`
	MinNodes         int    `json:"min_nodes"`
	MinMinutes       int    `json:"min_minutes"`
	MaxMinutes       int    `json:"max_minutes"`
	MaxIterations    int    `json:"max_iterations"`
	ReturnOnlyWhen   string `json:"return_only_when"`
	CheckpointPolicy string `json:"checkpoint_policy"`
	CreatedAtUTC     string `json:"created_at_utc"`
	UpdatedAtUTC     string `json:"updated_at_utc"`
}

type missionCommandAtlasRecommendation struct {
	Status               string `json:"status"`
	TotalNodes           int    `json:"total_nodes"`
	CompletedNodes       int    `json:"completed_nodes"`
	ReadyNodes           int    `json:"ready_nodes"`
	CheckpointCount      int    `json:"checkpoint_count"`
	ElapsedMinutes       int    `json:"elapsed_minutes"`
	MinMinutesMet        bool   `json:"min_minutes_met"`
	LeaseTimeStatus      string `json:"lease_time_status"`
	ReturnGateStatus     string `json:"return_gate_status"`
	FinalResponseAllowed bool   `json:"final_response_allowed"`
	Blocker              string `json:"blocker,omitempty"`
	RSIRemainsDenied     bool   `json:"rsi_remains_denied,omitempty"`
	ExactNextAction      string `json:"exact_next_action"`
}

func (lease *missionCommandGoalLease) UnmarshalJSON(data []byte) error {
	type goalLease missionCommandGoalLease
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode((*goalLease)(lease)); err != nil {
		return err
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		if err == nil {
			return fmt.Errorf("multiple JSON values")
		}
		return err
	}
	return nil
}

func (recommendation *missionCommandAtlasRecommendation) UnmarshalJSON(data []byte) error {
	type atlasRecommendation missionCommandAtlasRecommendation
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode((*atlasRecommendation)(recommendation)); err != nil {
		return err
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		if err == nil {
			return fmt.Errorf("multiple JSON values")
		}
		return err
	}
	return nil
}

func readMissionCommandStatus(path string) (missionCommandStatusSummary, error) {
	input, err := readStrictMissionCommandStatus(path)
	if err != nil {
		return missionCommandStatusSummary{}, err
	}
	if input.Schema != "ao.command.mission-status.v0.1" {
		return missionCommandStatusSummary{}, fmt.Errorf("schema must be ao.command.mission-status.v0.1")
	}
	resolvedOperatorMode := input.OperatorMode
	if resolvedOperatorMode == "" && input.readOnlyPresent && input.ReadOnly {
		resolvedOperatorMode = operatorMode
	}
	if input.readOnlyPresent && !input.ReadOnly {
		return missionCommandStatusSummary{}, fmt.Errorf("read_only must be true")
	}
	if input.MissionID == "" || input.Status == "" || input.CurrentRoute == "" || resolvedOperatorMode == "" {
		return missionCommandStatusSummary{}, fmt.Errorf("mission status requires mission_id, status, current_route, and operator_mode")
	}
	if resolvedOperatorMode != operatorMode {
		return missionCommandStatusSummary{}, fmt.Errorf("operator_mode must be %s", operatorMode)
	}
	if input.SafeToExecute || input.ExecutesWork || input.ApprovesWork || input.MutatesRepositories {
		return missionCommandStatusSummary{}, fmt.Errorf("mission status must not claim execution, approval, or repository mutation authority")
	}
	if input.CheckpointCount < 0 {
		return missionCommandStatusSummary{}, fmt.Errorf("checkpoint_count must not be negative")
	}
	switch input.CheckpointFreshnessStatus {
	case "", "missing", "fresh", "stale_or_missing", "not_required":
	default:
		return missionCommandStatusSummary{}, fmt.Errorf("checkpoint_freshness_status is invalid")
	}
	switch input.ReturnGateStatus {
	case "", "early_return_denied", "final_response_allowed":
	default:
		return missionCommandStatusSummary{}, fmt.Errorf("return_gate_status is invalid")
	}
	if input.GoalLease != nil {
		if err := validateMissionCommandGoalLease(*input.GoalLease); err != nil {
			return missionCommandStatusSummary{}, err
		}
	} else if input.goalLeasePresent {
		return missionCommandStatusSummary{}, fmt.Errorf("goal_lease must be an object")
	}
	if input.AtlasRecommendation != nil {
		if err := validateMissionCommandAtlasRecommendation(*input.AtlasRecommendation); err != nil {
			return missionCommandStatusSummary{}, err
		}
	} else if input.atlasRecommendationPresent {
		return missionCommandStatusSummary{}, fmt.Errorf("atlas_recommendation must be an object")
	}
	if input.GeneratedAtUTC != "" {
		if _, err := time.Parse(time.RFC3339, input.GeneratedAtUTC); err != nil {
			return missionCommandStatusSummary{}, fmt.Errorf("generated_at_utc must be RFC3339")
		}
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
		OperatorMode:         resolvedOperatorMode,
		SafeToExecute:        false,
		ExecutesWork:         false,
		ApprovesWork:         false,
		MutatesRepositories:  false,
		ExactNextAction:      input.ExactNextAction,
	}, nil
}

func validateMissionCommandGoalLease(lease missionCommandGoalLease) error {
	if lease.Schema != "ao.mission.goal-lease.v0.3" {
		return fmt.Errorf("goal_lease schema must be ao.mission.goal-lease.v0.3")
	}
	if lease.MinNodes <= 0 || lease.MinMinutes < 0 || lease.MaxMinutes <= 0 ||
		lease.MaxMinutes < lease.MinMinutes || lease.MaxIterations <= 0 {
		return fmt.Errorf("goal_lease bounds are invalid")
	}
	if strings.TrimSpace(lease.ReturnOnlyWhen) == "" || strings.TrimSpace(lease.CheckpointPolicy) == "" {
		return fmt.Errorf("goal_lease policy fields are required")
	}
	if _, err := time.Parse(time.RFC3339, lease.CreatedAtUTC); err != nil {
		return fmt.Errorf("goal_lease created_at_utc must be RFC3339")
	}
	if _, err := time.Parse(time.RFC3339, lease.UpdatedAtUTC); err != nil {
		return fmt.Errorf("goal_lease updated_at_utc must be RFC3339")
	}
	return nil
}

func validateMissionCommandAtlasRecommendation(recommendation missionCommandAtlasRecommendation) error {
	if recommendation.Status == "" || recommendation.TotalNodes < 0 ||
		recommendation.CompletedNodes < 0 || recommendation.ReadyNodes < 0 ||
		recommendation.CheckpointCount < 0 || recommendation.ElapsedMinutes < 0 ||
		recommendation.CompletedNodes+recommendation.ReadyNodes > recommendation.TotalNodes {
		return fmt.Errorf("atlas_recommendation counts are invalid")
	}
	switch recommendation.ReturnGateStatus {
	case "early_return_denied", "final_response_allowed":
	default:
		return fmt.Errorf("atlas_recommendation return_gate_status is invalid")
	}
	switch recommendation.LeaseTimeStatus {
	case "within_window", "maximum_exceeded", "minimum_not_met":
	default:
		return fmt.Errorf("atlas_recommendation lease_time_status is invalid")
	}
	if recommendation.FinalResponseAllowed &&
		(recommendation.ReturnGateStatus != "final_response_allowed" ||
			recommendation.ReadyNodes != 0 || recommendation.LeaseTimeStatus != "within_window") {
		return fmt.Errorf("atlas_recommendation final response is contradictory")
	}
	if strings.TrimSpace(recommendation.ExactNextAction) == "" {
		return fmt.Errorf("atlas_recommendation exact_next_action is required")
	}
	return nil
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
		case "read_only":
			input.readOnlyPresent = true
			target = &input.ReadOnly
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
		case "checkpoint_freshness_status":
			target = &input.CheckpointFreshnessStatus
		case "checkpoint_count":
			target = &input.CheckpointCount
		case "return_gate_status":
			target = &input.ReturnGateStatus
		case "goal_lease":
			input.goalLeasePresent = true
			target = &input.GoalLease
		case "atlas_recommendation":
			input.atlasRecommendationPresent = true
			target = &input.AtlasRecommendation
		case "blockers":
			target = &input.Blockers
		case "generated_at_utc":
			target = &input.GeneratedAtUTC
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
