package cli

import (
	"encoding/json"
	"flag"
	"fmt"
	"strings"
)

func (a App) missionAggregate(args []string) int {
	var statusPath, atlasMetadataPath, foundrySmokePath string
	var jsonOut, jsonlOut, watch, compact bool
	var iterations int
	fs := flag.NewFlagSet("mission aggregate", flag.ContinueOnError)
	fs.SetOutput(a.Stderr)
	fs.StringVar(&statusPath, "status", "", "path to AO Mission command status JSON")
	fs.StringVar(&atlasMetadataPath, "atlas-metadata", "", "path to AO Atlas Mission workgraph metadata JSON")
	fs.StringVar(&foundrySmokePath, "foundry-smoke", "", "path to AO Foundry Mission e2e smoke JSON")
	fs.BoolVar(&jsonOut, "json", false, "emit JSON")
	fs.BoolVar(&jsonlOut, "jsonl", false, "emit one JSON object per watch iteration")
	fs.BoolVar(&watch, "watch", false, "poll read-only aggregate status")
	fs.BoolVar(&compact, "compact", false, "emit compact watch summary")
	fs.IntVar(&iterations, "iterations", 1, "bounded watch iterations")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if strings.TrimSpace(statusPath) == "" || strings.TrimSpace(atlasMetadataPath) == "" || strings.TrimSpace(foundrySmokePath) == "" {
		fmt.Fprintln(a.Stderr, "ao-command mission aggregate: --status, --atlas-metadata, and --foundry-smoke are required")
		return 2
	}
	summary, err := readMissionAggregate(statusPath, atlasMetadataPath, foundrySmokePath)
	if err != nil {
		fmt.Fprintf(a.Stderr, "ao-command mission aggregate: %v\n", err)
		return 1
	}
	if watch {
		if iterations < 1 {
			fmt.Fprintln(a.Stderr, "ao-command mission aggregate: --iterations must be positive")
			return 2
		}
		watchSummary := buildMissionAggregateWatchSummary(summary, iterations)
		if jsonlOut {
			for i := 1; i <= iterations; i++ {
				line := missionAggregateWatchLine{
					CommandSchemaVersion:     watchSummary.CommandSchemaVersion,
					Schema:                   watchSummary.Schema,
					MissionID:                watchSummary.MissionID,
					Status:                   watchSummary.Status,
					Iteration:                i,
					Iterations:               iterations,
					Latest:                   summary,
					PrimaryMissionProvenance: watchSummary.PrimaryMissionProvenance,
					ProvenanceDiagnostics:    watchSummary.ProvenanceDiagnostics,
					TimelineCompactionBound:  watchSummary.TimelineCompactionBound,
					OperatorMode:             watchSummary.OperatorMode,
					SafeToExecute:            false,
					ExecutesWork:             false,
					ApprovesWork:             false,
					MutatesRepositories:      false,
					ExactNextAction:          watchSummary.ExactNextAction,
				}
				body, err := json.Marshal(line)
				if err != nil {
					fmt.Fprintf(a.Stderr, "ao-command mission aggregate: %v\n", err)
					return 1
				}
				fmt.Fprintln(a.Stdout, string(body))
			}
			return 0
		}
		if jsonOut {
			return a.writeJSON(watchSummary)
		}
		if compact {
			fmt.Fprintf(a.Stdout, "compact_summary=mission=%s status=%s route=%s provenance=%s timeline_compaction_bound=%t iterations=%d\n", watchSummary.MissionID, watchSummary.Status, summary.CurrentRoute, watchSummary.PrimaryMissionProvenance, watchSummary.TimelineCompactionBound, watchSummary.Iterations)
			fmt.Fprintf(a.Stdout, "safe_to_execute=%t\n", watchSummary.SafeToExecute)
			fmt.Fprintf(a.Stdout, "executes_work=%t\n", watchSummary.ExecutesWork)
			fmt.Fprintf(a.Stdout, "approves_work=%t\n", watchSummary.ApprovesWork)
			fmt.Fprintf(a.Stdout, "exact_next_action=%s\n", watchSummary.ExactNextAction)
			return 0
		}
		fmt.Fprintf(a.Stdout, "ao_command_mission_aggregate_watch=%s\n", watchSummary.Status)
		fmt.Fprintf(a.Stdout, "mission_id=%s\n", watchSummary.MissionID)
		fmt.Fprintf(a.Stdout, "iterations=%d\n", watchSummary.Iterations)
		fmt.Fprintf(a.Stdout, "safe_to_execute=%t\n", watchSummary.SafeToExecute)
		fmt.Fprintf(a.Stdout, "executes_work=%t\n", watchSummary.ExecutesWork)
		fmt.Fprintf(a.Stdout, "approves_work=%t\n", watchSummary.ApprovesWork)
		fmt.Fprintf(a.Stdout, "exact_next_action=%s\n", watchSummary.ExactNextAction)
		return 0
	}
	if jsonOut {
		return a.writeJSON(summary)
	}
	fmt.Fprintf(a.Stdout, "ao_command_mission_aggregate=%s\n", summary.Status)
	fmt.Fprintf(a.Stdout, "mission_id=%s\n", summary.MissionID)
	fmt.Fprintf(a.Stdout, "current_route=%s\n", summary.CurrentRoute)
	fmt.Fprintf(a.Stdout, "atlas_workgraph_id=%s\n", summary.AtlasWorkgraphID)
	fmt.Fprintf(a.Stdout, "primary_mission_provenance=%s\n", summary.PrimaryMissionProvenance)
	fmt.Fprintf(a.Stdout, "timeline_compaction_bound=%t\n", summary.TimelineCompactionBound)
	fmt.Fprintf(a.Stdout, "foundry_status=%s\n", summary.FoundryStatus)
	fmt.Fprintf(a.Stdout, "operator_mode=%s\n", summary.OperatorMode)
	fmt.Fprintf(a.Stdout, "safe_to_execute=%t\n", summary.SafeToExecute)
	fmt.Fprintf(a.Stdout, "executes_work=%t\n", summary.ExecutesWork)
	fmt.Fprintf(a.Stdout, "approves_work=%t\n", summary.ApprovesWork)
	fmt.Fprintf(a.Stdout, "mutates_repositories=%t\n", summary.MutatesRepositories)
	fmt.Fprintf(a.Stdout, "exact_next_action=%s\n", summary.ExactNextAction)
	return 0
}

type missionAggregateSummary struct {
	CommandSchemaVersion     string `json:"command_schema_version"`
	Schema                   string `json:"schema"`
	MissionID                string `json:"mission_id"`
	Status                   string `json:"status"`
	CurrentRoute             string `json:"current_route"`
	AtlasWorkgraphID         string `json:"atlas_workgraph_id"`
	PrimaryMissionProvenance string `json:"primary_mission_provenance"`
	ProvenanceDiagnostics    string `json:"provenance_diagnostics"`
	FoundryStatus            string `json:"foundry_status"`
	TimelineCompactionBound  bool   `json:"timeline_compaction_bound"`
	OperatorMode             string `json:"operator_mode"`
	SafeToExecute            bool   `json:"safe_to_execute"`
	ExecutesWork             bool   `json:"executes_work"`
	ApprovesWork             bool   `json:"approves_work"`
	MutatesRepositories      bool   `json:"mutates_repositories"`
	ExactNextAction          string `json:"exact_next_action"`
}

func readMissionAggregate(statusPath, atlasMetadataPath, foundrySmokePath string) (missionAggregateSummary, error) {
	status, err := readMissionCommandStatus(statusPath)
	if err != nil {
		return missionAggregateSummary{}, err
	}
	var atlas struct {
		ContractVersion          string `json:"contract_version"`
		MissionID                string `json:"mission_id"`
		WorkgraphID              string `json:"workgraph_id"`
		PrimaryMissionProvenance string `json:"primary_mission_provenance"`
		ProvenanceDiagnostics    string `json:"provenance_diagnostics"`
		SafeToExecute            bool   `json:"safe_to_execute"`
		SchedulesWork            bool   `json:"schedules_work"`
		ExecutesWork             bool   `json:"executes_work"`
		ApprovesWork             bool   `json:"approves_work"`
	}
	if err := readJSONFile(atlasMetadataPath, &atlas); err != nil {
		return missionAggregateSummary{}, err
	}
	if atlas.ContractVersion != "ao.atlas.ao-mission-workgraph-metadata.v0.1" {
		return missionAggregateSummary{}, fmt.Errorf("atlas metadata contract_version must be ao.atlas.ao-mission-workgraph-metadata.v0.1")
	}
	if atlas.MissionID != status.MissionID || strings.TrimSpace(atlas.WorkgraphID) == "" || strings.TrimSpace(atlas.PrimaryMissionProvenance) == "" {
		return missionAggregateSummary{}, fmt.Errorf("atlas metadata must bind mission_id, workgraph_id, and primary provenance")
	}
	if atlas.SafeToExecute || atlas.SchedulesWork || atlas.ExecutesWork || atlas.ApprovesWork {
		return missionAggregateSummary{}, fmt.Errorf("atlas metadata must not claim scheduling, execution, or approval authority")
	}
	var foundry struct {
		Schema                   string `json:"schema"`
		Status                   string `json:"status"`
		MissionID                string `json:"mission_id"`
		AtlasWorkgraphID         string `json:"atlas_workgraph_id"`
		PrimaryMissionProvenance string `json:"primary_mission_provenance"`
		ProvenanceDiagnostics    string `json:"provenance_diagnostics"`
		SafeToExecute            bool   `json:"safe_to_execute"`
		ExecutesWork             bool   `json:"executes_work"`
		ApprovesWork             bool   `json:"approves_work"`
		MutatesRepositories      bool   `json:"mutates_repositories"`
	}
	if err := readJSONFile(foundrySmokePath, &foundry); err != nil {
		return missionAggregateSummary{}, err
	}
	if foundry.Schema != "ao.foundry.ao-mission-e2e-smoke.v0.1" {
		return missionAggregateSummary{}, fmt.Errorf("foundry smoke schema must be ao.foundry.ao-mission-e2e-smoke.v0.1")
	}
	if foundry.MissionID != status.MissionID || foundry.AtlasWorkgraphID != atlas.WorkgraphID {
		return missionAggregateSummary{}, fmt.Errorf("foundry smoke must bind mission_id and atlas_workgraph_id")
	}
	if foundry.PrimaryMissionProvenance != atlas.PrimaryMissionProvenance {
		return missionAggregateSummary{}, fmt.Errorf("foundry smoke primary provenance must match Atlas metadata")
	}
	if foundry.SafeToExecute || foundry.ExecutesWork || foundry.ApprovesWork || foundry.MutatesRepositories {
		return missionAggregateSummary{}, fmt.Errorf("foundry smoke must not claim execution, approval, or repository mutation authority")
	}
	diagnostics := atlas.ProvenanceDiagnostics
	if strings.TrimSpace(diagnostics) == "" {
		diagnostics = foundry.ProvenanceDiagnostics
	}
	return missionAggregateSummary{
		CommandSchemaVersion:     commandSchemaVersion,
		Schema:                   "ao.command.mission-aggregate.v0.1",
		MissionID:                status.MissionID,
		Status:                   "ready",
		CurrentRoute:             status.CurrentRoute,
		AtlasWorkgraphID:         atlas.WorkgraphID,
		PrimaryMissionProvenance: atlas.PrimaryMissionProvenance,
		ProvenanceDiagnostics:    diagnostics,
		FoundryStatus:            foundry.Status,
		TimelineCompactionBound:  strings.Contains(diagnostics, "timeline_compaction="),
		OperatorMode:             operatorMode,
		SafeToExecute:            false,
		ExecutesWork:             false,
		ApprovesWork:             false,
		MutatesRepositories:      false,
		ExactNextAction:          "read Mission aggregate status; continue through AO Mission routing and governed downstream gates",
	}, nil
}
