package cli

type missionAggregateWatchSummary struct {
	CommandSchemaVersion     string                  `json:"command_schema_version"`
	Schema                   string                  `json:"schema"`
	MissionID                string                  `json:"mission_id"`
	Status                   string                  `json:"status"`
	Iterations               int                     `json:"iterations"`
	Latest                   missionAggregateSummary `json:"latest"`
	PrimaryMissionProvenance string                  `json:"primary_mission_provenance"`
	ProvenanceDiagnostics    string                  `json:"provenance_diagnostics"`
	TimelineCompactionBound  bool                    `json:"timeline_compaction_bound"`
	OperatorMode             string                  `json:"operator_mode"`
	SafeToExecute            bool                    `json:"safe_to_execute"`
	ExecutesWork             bool                    `json:"executes_work"`
	ApprovesWork             bool                    `json:"approves_work"`
	MutatesRepositories      bool                    `json:"mutates_repositories"`
	ExactNextAction          string                  `json:"exact_next_action"`
}

type missionAggregateWatchLine struct {
	CommandSchemaVersion     string                  `json:"command_schema_version"`
	Schema                   string                  `json:"schema"`
	MissionID                string                  `json:"mission_id"`
	Status                   string                  `json:"status"`
	Iteration                int                     `json:"iteration"`
	Iterations               int                     `json:"iterations"`
	Latest                   missionAggregateSummary `json:"latest"`
	PrimaryMissionProvenance string                  `json:"primary_mission_provenance"`
	ProvenanceDiagnostics    string                  `json:"provenance_diagnostics"`
	TimelineCompactionBound  bool                    `json:"timeline_compaction_bound"`
	OperatorMode             string                  `json:"operator_mode"`
	SafeToExecute            bool                    `json:"safe_to_execute"`
	ExecutesWork             bool                    `json:"executes_work"`
	ApprovesWork             bool                    `json:"approves_work"`
	MutatesRepositories      bool                    `json:"mutates_repositories"`
	ExactNextAction          string                  `json:"exact_next_action"`
}

func buildMissionAggregateWatchSummary(latest missionAggregateSummary, iterations int) missionAggregateWatchSummary {
	return missionAggregateWatchSummary{
		CommandSchemaVersion:     commandSchemaVersion,
		Schema:                   "ao.command.mission-aggregate-watch.v0.1",
		MissionID:                latest.MissionID,
		Status:                   latest.Status,
		Iterations:               iterations,
		Latest:                   latest,
		PrimaryMissionProvenance: latest.PrimaryMissionProvenance,
		ProvenanceDiagnostics:    latest.ProvenanceDiagnostics,
		TimelineCompactionBound:  latest.TimelineCompactionBound,
		OperatorMode:             operatorMode,
		SafeToExecute:            false,
		ExecutesWork:             false,
		ApprovesWork:             false,
		MutatesRepositories:      false,
		ExactNextAction:          "watch Mission aggregate readback only; do not schedule or execute work from Command",
	}
}
