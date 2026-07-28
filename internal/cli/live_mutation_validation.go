package cli

import (
	"encoding/json"
	"fmt"
	"strings"
)

func readLiveMutationArtifact(name, path, expectedSchema string) (liveMutationArtifactSummary, map[string]any, error) {
	var raw map[string]any
	if err := readPublicJSONFile(path, &raw); err != nil {
		return liveMutationArtifactSummary{}, nil, fmt.Errorf("read %s: %w", name, err)
	}
	schema := liveMutationMapString(raw, "schema_version")
	if schema != expectedSchema {
		return liveMutationArtifactSummary{}, nil, fmt.Errorf("%s has invalid schema_version %q", name, schema)
	}
	status := liveMutationMapString(raw, "status")
	if name == "operator_kill_switch" {
		status = liveMutationMapString(raw, "state")
	}
	if strings.TrimSpace(status) == "" {
		return liveMutationArtifactSummary{}, nil, fmt.Errorf("%s requires status", name)
	}
	sha, err := sha256File(path)
	if err != nil {
		return liveMutationArtifactSummary{}, nil, fmt.Errorf("hash %s: %w", name, err)
	}
	return liveMutationArtifactSummary{
		Name:              name,
		Path:              path,
		SchemaVersion:     schema,
		Status:            status,
		SHA256:            sha,
		FirstFailingCheck: liveMutationMapString(raw, "first_failing_check"),
	}, raw, nil
}

func validateLiveMutationArtifactBoundaries(name string, raw map[string]any) error {
	if err := validateLiveMutationMode(name, raw); err != nil {
		return err
	}
	for _, field := range []string{
		"mutates_live_state",
		"mutates_repositories",
		"schedules_work",
		"executes_work",
		"approves_work",
		"calls_providers",
		"provider_calls_allowed",
		"release_or_publish_allowed",
		"uploads_artifacts",
		"live_mutation_allowed",
	} {
		if liveMutationMapBool(raw, field) {
			return fmt.Errorf("%s expands forbidden authority via %s", name, field)
		}
	}
	if boundaries, ok := raw["authority_boundaries"].(map[string]any); ok {
		if !liveMutationMapBool(boundaries, "dry_run_only") {
			return fmt.Errorf("%s authority_boundaries must remain dry_run_only", name)
		}
		for _, field := range []string{
			"live_mutation_allowed",
			"mutates_repositories",
			"schedules_work",
			"executes_work",
			"approves_work",
			"calls_providers",
			"provider_calls_allowed",
			"release_or_publish_allowed",
			"sibling_repo_mutation_allowed",
		} {
			if liveMutationMapBool(boundaries, field) {
				return fmt.Errorf("%s expands forbidden authority via authority_boundaries.%s", name, field)
			}
		}
	}
	return nil
}

func validateLiveMutationMode(name string, raw map[string]any) error {
	mode := liveMutationMapString(raw, "mode")
	if mode == "" {
		return nil
	}
	switch mode {
	case "dry_run_only", "dry_run_packet", "fixture_only", "fixture_only_rehearsal":
		return nil
	default:
		return fmt.Errorf("%s has unsafe mode %q", name, mode)
	}
}

func liveMutationMapString(raw map[string]any, key string) string {
	if value, ok := raw[key].(string); ok {
		return value
	}
	return ""
}

func liveMutationMapBool(raw map[string]any, key string) bool {
	if value, ok := raw[key].(bool); ok {
		return value
	}
	return false
}

func jsonEquivalent(left any, right any) bool {
	leftBytes, leftErr := json.Marshal(left)
	rightBytes, rightErr := json.Marshal(right)
	return leftErr == nil && rightErr == nil && string(leftBytes) == string(rightBytes)
}

func liveMutationStringSlice(raw map[string]any, key string) []string {
	values, ok := raw[key].([]any)
	if !ok {
		return nil
	}
	result := []string{}
	for _, value := range values {
		if text, ok := value.(string); ok && strings.TrimSpace(text) != "" {
			result = append(result, text)
		}
	}
	return result
}

func liveMutationAnySlice(value any) []any {
	values, _ := value.([]any)
	return values
}

func liveMutationRepoStates(raw map[string]any) []liveMutationRepoState {
	values, ok := raw["repo_states"].([]any)
	if !ok {
		return nil
	}
	states := []liveMutationRepoState{}
	for _, value := range values {
		object, ok := value.(map[string]any)
		if !ok {
			continue
		}
		states = append(states, liveMutationRepoState{
			Repo:            liveMutationMapString(object, "repo"),
			Order:           liveMutationMapInt(object, "order"),
			PlannedPR:       liveMutationMapString(object, "planned_pr"),
			Status:          liveMutationMapString(object, "status"),
			ExecutionStatus: liveMutationMapString(object, "execution_status"),
			RollbackScope:   liveMutationStringSlice(object, "rollback_scope"),
			RollbackStatus:  liveMutationMapString(object, "rollback_status"),
			DependsOn:       liveMutationStringSlice(object, "depends_on"),
			MergeAfter:      liveMutationStringSlice(object, "merge_after"),
		})
	}
	return states
}

func validateLiveMutationRepoStates(raw map[string]any, states []liveMutationRepoState) string {
	if len(states) == 0 {
		return ""
	}
	policy, _ := raw["concurrency_policy"].(map[string]any)
	if liveMutationMapBool(policy, "concurrent_execution_allowed") ||
		liveMutationMapInt(policy, "max_active_repos") != 1 ||
		!liveMutationMapBool(policy, "required_serialized_dependency_order") {
		return "Repair multi_repo_low_risk dry-run evidence: unsafe concurrent execution is not allowed."
	}
	seen := map[string]bool{}
	readyToExecute := 0
	for i, state := range states {
		switch {
		case state.Repo == "":
			return "Repair multi_repo_low_risk dry-run evidence: repo_state missing repo."
		case seen[state.Repo]:
			return "Repair multi_repo_low_risk dry-run evidence: duplicate repo_state."
		case state.Order != i+1:
			return "Repair multi_repo_low_risk dry-run evidence: repo_state dependency order is invalid."
		case state.PlannedPR == "":
			return "Repair multi_repo_low_risk dry-run evidence: planned PR dependency is missing."
		case state.Status != "ready":
			return "Repair multi_repo_low_risk dry-run evidence: repo_state is not ready."
		case state.ExecutionStatus == "executing" || state.ExecutionStatus == "active":
			return "Repair multi_repo_low_risk dry-run evidence: unsafe concurrent execution is not allowed."
		case state.ExecutionStatus == "ready_to_execute":
			readyToExecute++
		case state.ExecutionStatus != "sequenced_dry_run_only":
			return "Repair multi_repo_low_risk dry-run evidence: repo_state is not sequenced dry-run only."
		case len(state.RollbackScope) == 0 || state.RollbackStatus != "ready":
			return "Repair multi_repo_low_risk dry-run evidence: per-repo rollback is not ready."
		}
		if !equalStringSlices(state.DependsOn, state.MergeAfter) {
			return "Repair multi_repo_low_risk dry-run evidence: merge_after must match depends_on."
		}
		for _, dependency := range state.DependsOn {
			if !seen[dependency] {
				return "Repair multi_repo_low_risk dry-run evidence: repo dependency must appear earlier in dependency order."
			}
		}
		seen[state.Repo] = true
	}
	if len(states) < 2 {
		return "Repair multi_repo_low_risk dry-run evidence: at least two repos are required."
	}
	if readyToExecute > 1 {
		return "Repair multi_repo_low_risk dry-run evidence: unsafe concurrent execution is not allowed."
	}
	return ""
}

func liveMutationMapInt(raw map[string]any, key string) int {
	switch value := raw[key].(type) {
	case float64:
		return int(value)
	case int:
		return value
	case json.Number:
		parsed, err := value.Int64()
		if err != nil {
			return 0
		}
		return int(parsed)
	default:
		return 0
	}
}
