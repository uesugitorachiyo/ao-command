package cli

import (
	"flag"
	"fmt"
	"strings"
)

func (a App) missionEvidence(args []string) int {
	var readbackPath string
	var jsonOut bool
	fs := flag.NewFlagSet("mission evidence", flag.ContinueOnError)
	fs.SetOutput(a.Stderr)
	fs.StringVar(&readbackPath, "readback", "", "path to AO Mission scheduler recovery or ledger compaction readback JSON")
	fs.BoolVar(&jsonOut, "json", false, "emit JSON")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if strings.TrimSpace(readbackPath) == "" {
		fmt.Fprintln(a.Stderr, "ao-command mission evidence: --readback is required")
		return 2
	}
	summary, err := readMissionEvidenceReadback(readbackPath)
	if err != nil {
		fmt.Fprintf(a.Stderr, "ao-command mission evidence: %v\n", err)
		return 1
	}
	if jsonOut {
		return a.writeJSON(summary)
	}
	fmt.Fprintf(a.Stdout, "ao_command_mission_evidence=%s\n", summary.Status)
	fmt.Fprintf(a.Stdout, "mission_id=%s\n", summary.MissionID)
	fmt.Fprintf(a.Stdout, "evidence_kind=%s\n", summary.EvidenceKind)
	fmt.Fprintf(a.Stdout, "operator_mode=%s\n", summary.OperatorMode)
	fmt.Fprintf(a.Stdout, "safe_to_execute=%t\n", summary.SafeToExecute)
	fmt.Fprintf(a.Stdout, "schedules_work=%t\n", summary.SchedulesWork)
	fmt.Fprintf(a.Stdout, "executes_work=%t\n", summary.ExecutesWork)
	fmt.Fprintf(a.Stdout, "approves_work=%t\n", summary.ApprovesWork)
	fmt.Fprintf(a.Stdout, "mutates_repositories=%t\n", summary.MutatesRepositories)
	fmt.Fprintf(a.Stdout, "exact_next_action=%s\n", summary.ExactNextAction)
	return 0
}

type missionEvidenceSummary struct {
	CommandSchemaVersion string `json:"command_schema_version"`
	Schema               string `json:"schema"`
	MissionID            string `json:"mission_id"`
	Status               string `json:"status"`
	EvidenceKind         string `json:"evidence_kind"`
	OperatorMode         string `json:"operator_mode"`
	SafeToExecute        bool   `json:"safe_to_execute"`
	SchedulesWork        bool   `json:"schedules_work"`
	ExecutesWork         bool   `json:"executes_work"`
	ApprovesWork         bool   `json:"approves_work"`
	MutatesRepositories  bool   `json:"mutates_repositories"`
	ExactNextAction      string `json:"exact_next_action"`
}

func readMissionEvidenceReadback(path string) (missionEvidenceSummary, error) {
	var input struct {
		Schema              string `json:"schema"`
		MissionID           string `json:"mission_id"`
		Status              string `json:"status"`
		ExactNextAction     string `json:"exact_next_action"`
		SafeToExecute       bool   `json:"safe_to_execute"`
		SchedulesWork       bool   `json:"schedules_work"`
		ExecutesWork        bool   `json:"executes_work"`
		ApprovesWork        bool   `json:"approves_work"`
		MutatesRepositories bool   `json:"mutates_repositories"`
		ProviderCalls       bool   `json:"provider_calls"`
		ReleaseOrPublish    bool   `json:"release_or_publish"`
		CredentialUse       bool   `json:"credential_use"`
		DirectMainMutation  bool   `json:"direct_main_mutation"`
		ConcurrentMutation  bool   `json:"concurrent_mutation"`
	}
	if err := readJSONFile(path, &input); err != nil {
		return missionEvidenceSummary{}, err
	}
	evidenceKind := ""
	switch input.Schema {
	case "ao.mission.scheduler-recovery-readback.v0.1":
		evidenceKind = "scheduler_recovery"
	case "ao.mission.ledger-compaction-readback.v0.1":
		evidenceKind = "ledger_compaction"
	case "ao.mission.timeline-compaction-readback.v0.1":
		evidenceKind = "timeline_compaction"
	default:
		return missionEvidenceSummary{}, fmt.Errorf("unsupported mission evidence schema %q", input.Schema)
	}
	if strings.TrimSpace(input.MissionID) == "" {
		return missionEvidenceSummary{}, fmt.Errorf("mission evidence readback requires mission_id")
	}
	if input.SafeToExecute || input.SchedulesWork || input.ExecutesWork || input.ApprovesWork || input.MutatesRepositories || input.ProviderCalls || input.ReleaseOrPublish || input.CredentialUse || input.DirectMainMutation || input.ConcurrentMutation {
		return missionEvidenceSummary{}, fmt.Errorf("mission evidence readback must not claim scheduling, execution, approval, repository mutation, provider, release, credential, direct-main, or concurrent authority")
	}
	status := input.Status
	if strings.TrimSpace(status) == "" {
		status = "ready"
	}
	return missionEvidenceSummary{
		CommandSchemaVersion: commandSchemaVersion,
		Schema:               "ao.command.mission-evidence.v0.1",
		MissionID:            input.MissionID,
		Status:               status,
		EvidenceKind:         evidenceKind,
		OperatorMode:         operatorMode,
		SafeToExecute:        false,
		SchedulesWork:        false,
		ExecutesWork:         false,
		ApprovesWork:         false,
		MutatesRepositories:  false,
		ExactNextAction:      input.ExactNextAction,
	}, nil
}
