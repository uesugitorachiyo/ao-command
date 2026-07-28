package cli

import (
	"flag"
	"fmt"
	"strings"
)

func (a App) missionArtifacts(args []string) int {
	var manifestPath string
	var jsonOut bool
	fs := flag.NewFlagSet("mission artifacts", flag.ContinueOnError)
	fs.SetOutput(a.Stderr)
	fs.StringVar(&manifestPath, "manifest", "", "path to AO Mission artifact manifest JSON")
	fs.BoolVar(&jsonOut, "json", false, "emit JSON")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if strings.TrimSpace(manifestPath) == "" {
		fmt.Fprintln(a.Stderr, "ao-command mission artifacts: --manifest is required")
		return 2
	}
	summary, err := readMissionArtifactManifest(manifestPath)
	if err != nil {
		fmt.Fprintf(a.Stderr, "ao-command mission artifacts: %v\n", err)
		return 1
	}
	if jsonOut {
		return a.writeJSON(summary)
	}
	fmt.Fprintf(a.Stdout, "ao_command_mission_artifacts=%s\n", summary.Status)
	fmt.Fprintf(a.Stdout, "mission_id=%s\n", summary.MissionID)
	fmt.Fprintf(a.Stdout, "artifact_count=%d\n", summary.ArtifactCount)
	fmt.Fprintf(a.Stdout, "operator_mode=%s\n", summary.OperatorMode)
	fmt.Fprintf(a.Stdout, "safe_to_execute=%t\n", summary.SafeToExecute)
	fmt.Fprintf(a.Stdout, "executes_work=%t\n", summary.ExecutesWork)
	fmt.Fprintf(a.Stdout, "approves_work=%t\n", summary.ApprovesWork)
	fmt.Fprintf(a.Stdout, "mutates_repositories=%t\n", summary.MutatesRepositories)
	for _, artifact := range summary.Artifacts {
		fmt.Fprintf(a.Stdout, "artifact=%s:%s\n", artifact.Name, artifact.Path)
	}
	fmt.Fprintf(a.Stdout, "exact_next_action=%s\n", summary.ExactNextAction)
	return 0
}

type missionArtifactRef struct {
	Name   string `json:"name"`
	Path   string `json:"path"`
	SHA256 string `json:"sha256"`
}

type missionArtifactsSummary struct {
	CommandSchemaVersion string               `json:"command_schema_version"`
	Schema               string               `json:"schema"`
	MissionID            string               `json:"mission_id"`
	Status               string               `json:"status"`
	OperatorMode         string               `json:"operator_mode"`
	ArtifactCount        int                  `json:"artifact_count"`
	Artifacts            []missionArtifactRef `json:"artifacts"`
	SafeToExecute        bool                 `json:"safe_to_execute"`
	ExecutesWork         bool                 `json:"executes_work"`
	ApprovesWork         bool                 `json:"approves_work"`
	MutatesRepositories  bool                 `json:"mutates_repositories"`
	ExactNextAction      string               `json:"exact_next_action"`
}

func readMissionArtifactManifest(path string) (missionArtifactsSummary, error) {
	var input struct {
		Schema              string               `json:"schema"`
		MissionID           string               `json:"mission_id"`
		Status              string               `json:"status"`
		OperatorMode        string               `json:"operator_mode"`
		ArtifactRefs        []missionArtifactRef `json:"artifact_refs"`
		SafeToExecute       bool                 `json:"safe_to_execute"`
		ExecutesWork        bool                 `json:"executes_work"`
		ApprovesWork        bool                 `json:"approves_work"`
		MutatesRepositories bool                 `json:"mutates_repositories"`
		ExactNextAction     string               `json:"exact_next_action"`
	}
	if err := readJSONFile(path, &input); err != nil {
		return missionArtifactsSummary{}, err
	}
	if input.Schema != "ao.mission.artifact-manifest.v0.1" {
		return missionArtifactsSummary{}, fmt.Errorf("schema must be ao.mission.artifact-manifest.v0.1")
	}
	if input.MissionID == "" || input.Status == "" || input.OperatorMode == "" || len(input.ArtifactRefs) == 0 {
		return missionArtifactsSummary{}, fmt.Errorf("mission artifact manifest requires mission_id, status, operator_mode, and artifact_refs")
	}
	if input.OperatorMode != operatorMode {
		return missionArtifactsSummary{}, fmt.Errorf("operator_mode must be %s", operatorMode)
	}
	if input.SafeToExecute || input.ExecutesWork || input.ApprovesWork || input.MutatesRepositories {
		return missionArtifactsSummary{}, fmt.Errorf("mission artifact manifest must not claim execution, approval, or repository mutation authority")
	}
	for _, artifact := range input.ArtifactRefs {
		if artifact.Name == "" || artifact.Path == "" || artifact.SHA256 == "" {
			return missionArtifactsSummary{}, fmt.Errorf("mission artifact manifest requires artifact name, path, and sha256")
		}
	}
	return missionArtifactsSummary{
		CommandSchemaVersion: commandSchemaVersion,
		Schema:               input.Schema,
		MissionID:            input.MissionID,
		Status:               input.Status,
		OperatorMode:         input.OperatorMode,
		ArtifactCount:        len(input.ArtifactRefs),
		Artifacts:            append([]missionArtifactRef(nil), input.ArtifactRefs...),
		SafeToExecute:        false,
		ExecutesWork:         false,
		ApprovesWork:         false,
		MutatesRepositories:  false,
		ExactNextAction:      input.ExactNextAction,
	}, nil
}
