package cli

import (
	"flag"
	"fmt"
	"strings"
)

func (a App) missionTimeline(args []string) int {
	var readbackPath string
	var jsonOut bool
	fs := flag.NewFlagSet("mission timeline", flag.ContinueOnError)
	fs.SetOutput(a.Stderr)
	fs.StringVar(&readbackPath, "readback", "", "path to AO Mission status/timeline compatibility vector JSON")
	fs.BoolVar(&jsonOut, "json", false, "emit JSON")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if strings.TrimSpace(readbackPath) == "" {
		fmt.Fprintln(a.Stderr, "ao-command mission timeline: --readback is required")
		return 2
	}
	summary, err := readMissionTimelineVector(readbackPath)
	if err != nil {
		fmt.Fprintf(a.Stderr, "ao-command mission timeline: %v\n", err)
		return 1
	}
	if jsonOut {
		return a.writeJSON(summary)
	}
	fmt.Fprintf(a.Stdout, "ao_command_mission_timeline=%s\n", summary.Status)
	fmt.Fprintf(a.Stdout, "mission_id=%s\n", summary.MissionID)
	fmt.Fprintf(a.Stdout, "mission_status=%s\n", summary.MissionStatus)
	fmt.Fprintf(a.Stdout, "current_route=%s\n", summary.CurrentRoute)
	fmt.Fprintf(a.Stdout, "current_phase=%s\n", summary.CurrentPhase)
	fmt.Fprintf(a.Stdout, "timeline_event_count=%d\n", summary.TimelineEventCount)
	fmt.Fprintf(a.Stdout, "latest_event=%s\n", summary.LatestEvent)
	fmt.Fprintf(a.Stdout, "tested_edge_count=%d\n", summary.TestedEdgeCount)
	fmt.Fprintf(a.Stdout, "full_stack_compatibility_complete=%t\n", summary.FullStackCompatibilityComplete)
	fmt.Fprintf(a.Stdout, "operator_mode=%s\n", summary.OperatorMode)
	fmt.Fprintf(a.Stdout, "safe_to_execute=%t\n", summary.SafeToExecute)
	fmt.Fprintf(a.Stdout, "executes_work=%t\n", summary.ExecutesWork)
	fmt.Fprintf(a.Stdout, "approves_work=%t\n", summary.ApprovesWork)
	fmt.Fprintf(a.Stdout, "mutates_repositories=%t\n", summary.MutatesRepositories)
	fmt.Fprintf(a.Stdout, "exact_next_action=%s\n", summary.ExactNextAction)
	return 0
}

type missionTimelineEvent struct {
	Schema         string `json:"schema"`
	MissionID      string `json:"mission_id"`
	Kind           string `json:"kind"`
	Sequence       int    `json:"sequence"`
	Status         string `json:"status"`
	Route          string `json:"route"`
	Phase          string `json:"phase"`
	Summary        string `json:"summary"`
	GeneratedAtUTC string `json:"generated_at_utc"`
}

type missionTimelineRunStatus struct {
	Schema              string `json:"schema"`
	MissionID           string `json:"mission_id"`
	Status              string `json:"status"`
	MissionStatus       string `json:"mission_status"`
	CurrentRoute        string `json:"current_route"`
	CurrentPhase        string `json:"current_phase"`
	SafeToExecute       bool   `json:"safe_to_execute"`
	ExecutesWork        bool   `json:"executes_work"`
	ApprovesWork        bool   `json:"approves_work"`
	MutatesRepositories bool   `json:"mutates_repositories"`
	ExactNextAction     string `json:"exact_next_action"`
}

type missionTimelineReadback struct {
	Schema              string                 `json:"schema"`
	Status              string                 `json:"status"`
	MissionID           string                 `json:"mission_id"`
	Compact             bool                   `json:"compact"`
	Events              []missionTimelineEvent `json:"events"`
	SafeToExecute       bool                   `json:"safe_to_execute"`
	ExecutesWork        bool                   `json:"executes_work"`
	ApprovesWork        bool                   `json:"approves_work"`
	MutatesRepositories bool                   `json:"mutates_repositories"`
}

type missionTimelineCompatibility struct {
	MatrixStatus                   string `json:"matrix_status"`
	TestedEdgeCount                int    `json:"tested_edge_count"`
	FullStackCompatibilityComplete bool   `json:"full_stack_compatibility_complete"`
}

type missionTimelineExpectedCommand struct {
	Schema                         string `json:"schema"`
	Status                         string `json:"status"`
	SourceRunStatusSchema          string `json:"source_run_status_schema"`
	SourceTimelineSchema           string `json:"source_timeline_schema"`
	TimelineEventCount             int    `json:"timeline_event_count"`
	LatestEvent                    string `json:"latest_event"`
	FullStackCompatibilityComplete bool   `json:"full_stack_compatibility_complete"`
}

type missionTimelineVector struct {
	SchemaVersion           string                         `json:"schema_version"`
	VectorID                string                         `json:"vector_id"`
	Edge                    string                         `json:"edge"`
	RunStatus               missionTimelineRunStatus       `json:"run_status"`
	Timeline                missionTimelineReadback        `json:"timeline"`
	Compatibility           missionTimelineCompatibility   `json:"compatibility"`
	ExpectedCommandTimeline missionTimelineExpectedCommand `json:"expected_command_operator_timeline"`
}

type missionTimelineSummary struct {
	CommandSchemaVersion           string `json:"command_schema_version"`
	Schema                         string `json:"schema"`
	MissionID                      string `json:"mission_id"`
	Status                         string `json:"status"`
	MissionStatus                  string `json:"mission_status"`
	CurrentRoute                   string `json:"current_route"`
	CurrentPhase                   string `json:"current_phase"`
	TimelineEventCount             int    `json:"timeline_event_count"`
	LatestEvent                    string `json:"latest_event"`
	TestedEdgeCount                int    `json:"tested_edge_count"`
	FullStackCompatibilityComplete bool   `json:"full_stack_compatibility_complete"`
	OperatorMode                   string `json:"operator_mode"`
	SafeToExecute                  bool   `json:"safe_to_execute"`
	ExecutesWork                   bool   `json:"executes_work"`
	ApprovesWork                   bool   `json:"approves_work"`
	MutatesRepositories            bool   `json:"mutates_repositories"`
	ExactNextAction                string `json:"exact_next_action"`
}

func readMissionTimelineVector(path string) (missionTimelineSummary, error) {
	var vector missionTimelineVector
	if err := readJSONFile(path, &vector); err != nil {
		return missionTimelineSummary{}, err
	}
	if vector.SchemaVersion != "ao.compatibility.mission-status-timeline-vector.v1" {
		return missionTimelineSummary{}, fmt.Errorf("mission timeline vector schema must be ao.compatibility.mission-status-timeline-vector.v1")
	}
	if vector.Edge != "ao-mission.run_status_timeline -> ao-command.operator_timeline" {
		return missionTimelineSummary{}, fmt.Errorf("mission timeline vector edge must bind Mission run status/timeline to Command operator timeline")
	}
	runStatus := vector.RunStatus
	timeline := vector.Timeline
	expected := vector.ExpectedCommandTimeline
	if runStatus.Schema != "ao.mission.run-status-timeline.v0.1" {
		return missionTimelineSummary{}, fmt.Errorf("mission run status schema must be ao.mission.run-status-timeline.v0.1")
	}
	if timeline.Schema != "ao.mission.compact-timeline.v0.1" {
		return missionTimelineSummary{}, fmt.Errorf("mission timeline schema must be ao.mission.compact-timeline.v0.1")
	}
	if expected.Schema != "ao-command.operator-timeline.v1" {
		return missionTimelineSummary{}, fmt.Errorf("expected Command operator timeline schema must be ao-command.operator-timeline.v1")
	}
	if strings.TrimSpace(runStatus.MissionID) == "" || strings.TrimSpace(timeline.MissionID) == "" {
		return missionTimelineSummary{}, fmt.Errorf("mission timeline vector requires mission IDs")
	}
	if runStatus.MissionID != timeline.MissionID {
		return missionTimelineSummary{}, fmt.Errorf("mission timeline run status and timeline mission IDs must match")
	}
	if runStatus.SafeToExecute || runStatus.ExecutesWork || runStatus.ApprovesWork || runStatus.MutatesRepositories ||
		timeline.SafeToExecute || timeline.ExecutesWork || timeline.ApprovesWork || timeline.MutatesRepositories {
		return missionTimelineSummary{}, fmt.Errorf("mission timeline vector must not claim execution, approval, or repository mutation authority")
	}
	if vector.Compatibility.TestedEdgeCount < 0 {
		return missionTimelineSummary{}, fmt.Errorf("mission timeline tested edge count must be non-negative")
	}
	if vector.Compatibility.FullStackCompatibilityComplete || expected.FullStackCompatibilityComplete {
		return missionTimelineSummary{}, fmt.Errorf("mission timeline vector must not claim full stack compatibility complete")
	}
	if expected.SourceRunStatusSchema != runStatus.Schema || expected.SourceTimelineSchema != timeline.Schema {
		return missionTimelineSummary{}, fmt.Errorf("expected Command timeline must identify source Mission schemas")
	}
	if expected.TimelineEventCount != len(timeline.Events) {
		return missionTimelineSummary{}, fmt.Errorf("expected Command timeline event count must match Mission timeline events")
	}
	if len(timeline.Events) == 0 {
		return missionTimelineSummary{}, fmt.Errorf("mission timeline requires at least one event")
	}
	latestEvent := timeline.Events[len(timeline.Events)-1].Kind
	if expected.LatestEvent != latestEvent {
		return missionTimelineSummary{}, fmt.Errorf("expected Command latest event must match Mission timeline")
	}
	for _, event := range timeline.Events {
		if event.Schema != "ao.mission.event.v0.1" {
			return missionTimelineSummary{}, fmt.Errorf("mission timeline events must use ao.mission.event.v0.1")
		}
		if event.MissionID != runStatus.MissionID {
			return missionTimelineSummary{}, fmt.Errorf("mission timeline event mission ID must match run status")
		}
	}
	status := expected.Status
	if strings.TrimSpace(status) == "" {
		status = "ready"
	}
	return missionTimelineSummary{
		CommandSchemaVersion:           commandSchemaVersion,
		Schema:                         "ao.command.mission-operator-timeline.v0.1",
		MissionID:                      runStatus.MissionID,
		Status:                         status,
		MissionStatus:                  runStatus.MissionStatus,
		CurrentRoute:                   runStatus.CurrentRoute,
		CurrentPhase:                   runStatus.CurrentPhase,
		TimelineEventCount:             len(timeline.Events),
		LatestEvent:                    latestEvent,
		TestedEdgeCount:                vector.Compatibility.TestedEdgeCount,
		FullStackCompatibilityComplete: false,
		OperatorMode:                   operatorMode,
		SafeToExecute:                  false,
		ExecutesWork:                   false,
		ApprovesWork:                   false,
		MutatesRepositories:            false,
		ExactNextAction:                runStatus.ExactNextAction,
	}, nil
}
