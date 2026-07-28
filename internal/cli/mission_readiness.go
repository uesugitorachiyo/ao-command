package cli

import (
	"flag"
	"fmt"
	"strings"
)

func (a App) missionReadiness(args []string) int {
	var bundlePath string
	var jsonOut bool
	fs := flag.NewFlagSet("mission readiness", flag.ContinueOnError)
	fs.SetOutput(a.Stderr)
	fs.StringVar(&bundlePath, "bundle", "", "path to AO Mission readiness bundle JSON")
	fs.BoolVar(&jsonOut, "json", false, "emit JSON")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if strings.TrimSpace(bundlePath) == "" {
		fmt.Fprintln(a.Stderr, "ao-command mission readiness: --bundle is required")
		return 2
	}
	summary, err := readMissionReadinessBundle(bundlePath)
	if err != nil {
		fmt.Fprintf(a.Stderr, "ao-command mission readiness: %v\n", err)
		return 1
	}
	if jsonOut {
		return a.writeJSON(summary)
	}
	fmt.Fprintf(a.Stdout, "ao_command_mission_readiness=%s\n", summary.Status)
	fmt.Fprintf(a.Stdout, "repo_count=%d\n", summary.RepoCount)
	fmt.Fprintf(a.Stdout, "ready_repos=%d\n", summary.ReadyRepos)
	fmt.Fprintf(a.Stdout, "blocked_repos=%d\n", summary.BlockedRepos)
	fmt.Fprintf(a.Stdout, "operator_mode=%s\n", summary.OperatorMode)
	fmt.Fprintf(a.Stdout, "safe_to_execute=%t\n", summary.SafeToExecute)
	fmt.Fprintf(a.Stdout, "executes_work=%t\n", summary.ExecutesWork)
	fmt.Fprintf(a.Stdout, "approves_work=%t\n", summary.ApprovesWork)
	fmt.Fprintf(a.Stdout, "mutates_repositories=%t\n", summary.MutatesRepositories)
	fmt.Fprintf(a.Stdout, "exact_next_action=%s\n", summary.ExactNextAction)
	return 0
}

type missionReadinessSummary struct {
	CommandSchemaVersion string `json:"command_schema_version"`
	Schema               string `json:"schema"`
	Status               string `json:"status"`
	RepoCount            int    `json:"repo_count"`
	ReadyRepos           int    `json:"ready_repos"`
	BlockedRepos         int    `json:"blocked_repos"`
	OperatorMode         string `json:"operator_mode"`
	SafeToExecute        bool   `json:"safe_to_execute"`
	ExecutesWork         bool   `json:"executes_work"`
	ApprovesWork         bool   `json:"approves_work"`
	MutatesRepositories  bool   `json:"mutates_repositories"`
	ExactNextAction      string `json:"exact_next_action"`
}

func readMissionReadinessBundle(path string) (missionReadinessSummary, error) {
	var input struct {
		Schema              string `json:"schema"`
		Status              string `json:"status"`
		RepoCount           int    `json:"repo_count"`
		ReadyRepos          int    `json:"ready_repos"`
		BlockedRepos        int    `json:"blocked_repos"`
		SafeToExecute       bool   `json:"safe_to_execute"`
		ExecutesWork        bool   `json:"executes_work"`
		ApprovesWork        bool   `json:"approves_work"`
		MutatesRepositories bool   `json:"mutates_repositories"`
		ExactNextAction     string `json:"exact_next_action"`
	}
	if err := readJSONFile(path, &input); err != nil {
		return missionReadinessSummary{}, err
	}
	if input.Schema != "ao.mission.readiness-bundle-readback.v0.1" {
		return missionReadinessSummary{}, fmt.Errorf("unsupported mission readiness schema %q", input.Schema)
	}
	if input.SafeToExecute || input.ExecutesWork || input.ApprovesWork || input.MutatesRepositories {
		return missionReadinessSummary{}, fmt.Errorf("mission readiness bundle must not claim execution, approval, or repository mutation authority")
	}
	status := input.Status
	if strings.TrimSpace(status) == "" {
		status = "ready"
	}
	return missionReadinessSummary{
		CommandSchemaVersion: commandSchemaVersion,
		Schema:               "ao.command.mission-readiness.v0.1",
		Status:               status,
		RepoCount:            input.RepoCount,
		ReadyRepos:           input.ReadyRepos,
		BlockedRepos:         input.BlockedRepos,
		OperatorMode:         operatorMode,
		SafeToExecute:        false,
		ExecutesWork:         false,
		ApprovesWork:         false,
		MutatesRepositories:  false,
		ExactNextAction:      input.ExactNextAction,
	}, nil
}
