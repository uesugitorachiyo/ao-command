package cli

import (
	"errors"
	"fmt"
	"strings"
)

func readComplexRefactorStatus(summaryPath string) (complexRefactorStatusSummary, error) {
	var summary foundryComplexRefactorSummary
	if err := readPublicJSONFile(summaryPath, &summary); err != nil {
		return complexRefactorStatusSummary{}, fmt.Errorf("read summary: %w", err)
	}
	if err := validateComplexRefactorSummary(summary); err != nil {
		return complexRefactorStatusSummary{}, err
	}
	nextAction := summary.LoopDecision.NextAction
	if strings.TrimSpace(nextAction) == "" {
		nextAction = deriveComplexRefactorNextAction(summary)
	}
	firstFailingCheck := summary.LoopDecision.FirstFailingCheck
	if summary.Status != "ready" && strings.TrimSpace(firstFailingCheck) == "" {
		firstFailingCheck = "complex_refactor_rehearsal"
	}
	return complexRefactorStatusSummary{
		SchemaVersion:              "ao.command.complex-refactor-status.v0.1",
		CommandSchemaVersion:       commandSchemaVersion,
		Status:                     summary.Status,
		Summary:                    summaryPath,
		Mode:                       summary.Mode,
		NextAction:                 nextAction,
		NextRecommendedFactoryTask: summary.NextRecommendedFactoryTask.TaskID,
		NextRecommendedNode:        summary.NextRecommendedFactoryTask.NodeID,
		TargetFactoryRepo:          summary.NextRecommendedFactoryTask.TargetFactoryRepo,
		TaskCounts:                 summary.TaskCounts,
		RepairPlan:                 summary.RepairPlan,
		ContextRepack:              summary.ContextRepack,
		FirstFailingCheck:          firstFailingCheck,
		BlockingNextActions:        uniqueStrings(summary.BlockingNextActions),
		MaintenanceSuggestions:     uniqueStrings(summary.MaintenanceSuggestions),
		SourceDigests:              summary.SourceDigests,
		Artifacts:                  summary.Artifacts,
		OperatorMode:               operatorMode,
		MutatesRepositories:        false,
		SchedulesWork:              false,
		ExecutesWork:               false,
		ApprovesWork:               false,
		CallsProviders:             false,
	}, nil
}

func deriveComplexRefactorNextAction(summary foundryComplexRefactorSummary) string {
	if summary.Status == "ready" && summary.LoopDecision.MayStartNextReadyTask {
		return "start_next_ready_task"
	}
	if summary.Status == "blocked" {
		return "repair_blocked_nodes"
	}
	return "stop_blocked"
}

func validateComplexRefactorSummary(summary foundryComplexRefactorSummary) error {
	if summary.SchemaVersion != "ao.foundry.complex-refactor-workgraph-rehearsal.v0.1" {
		return errors.New("invalid complex-refactor rehearsal schema_version")
	}
	if !isPulseStatus(summary.Status) {
		return fmt.Errorf("invalid complex-refactor rehearsal status %q", summary.Status)
	}
	if summary.Mode != "fixture_only_rehearsal" {
		return errors.New("complex-refactor rehearsal mode must be fixture_only_rehearsal")
	}
	if summary.MutatesRepositories || summary.SchedulesWork || summary.ExecutesWork || summary.ApprovesWork || summary.CallsProviders {
		return errors.New("complex-refactor rehearsal must remain read-only and cannot schedule, execute, approve, call providers, or mutate repositories")
	}
	if !summary.NoDuplicatedStackFolders {
		return errors.New("complex-refactor rehearsal must prove no duplicated stack folders")
	}
	if err := validateComplexRefactorTaskCounts(summary.TaskCounts); err != nil {
		return err
	}
	if summary.TaskCounts.Ready > 0 {
		if strings.TrimSpace(summary.NextRecommendedFactoryTask.NodeID) == "" ||
			strings.TrimSpace(summary.NextRecommendedFactoryTask.TaskID) == "" ||
			strings.TrimSpace(summary.NextRecommendedFactoryTask.TargetFactoryRepo) == "" {
			return errors.New("complex-refactor rehearsal requires next_recommended_factory_task when ready tasks exist")
		}
	}
	if !summary.LoopDecision.MustNotStartBlockedTasks {
		return errors.New("complex-refactor rehearsal must block unsafe/blocked tasks")
	}
	if summary.Status == "ready" && summary.TaskCounts.Ready > 0 && !summary.LoopDecision.MayStartNextReadyTask {
		return errors.New("ready complex-refactor rehearsal must allow the next ready task")
	}
	if err := validateComplexRefactorRepairPlan(summary.RepairPlan); err != nil {
		return err
	}
	if err := validateComplexRefactorContextRepack(summary.ContextRepack); err != nil {
		return err
	}
	if len(summary.SourceDigests) == 0 {
		return errors.New("complex-refactor rehearsal requires source_digests")
	}
	for _, source := range summary.SourceDigests {
		if strings.TrimSpace(source.Name) == "" || strings.TrimSpace(source.Path) == "" {
			return errors.New("complex-refactor source_digests require name and path")
		}
		if !isHexSHA256(source.SHA256) {
			return errors.New("complex-refactor source_digests require 64-character sha256")
		}
		if err := validatePublicSafeText(source.Path); err != nil {
			return err
		}
	}
	if len(summary.Artifacts) == 0 {
		return errors.New("complex-refactor rehearsal requires artifacts")
	}
	for key, value := range summary.Artifacts {
		if strings.TrimSpace(key) == "" || strings.TrimSpace(value) == "" {
			return errors.New("complex-refactor artifacts require non-empty keys and paths")
		}
		if err := validatePublicSafeText(value); err != nil {
			return err
		}
	}
	return nil
}

func validateComplexRefactorRepairPlan(repair complexRefactorRepairPlan) error {
	if strings.TrimSpace(repair.Status) == "" && strings.TrimSpace(repair.Path) == "" && strings.TrimSpace(repair.RepairTaskID) == "" {
		return nil
	}
	if repair.Status != "repair_required" {
		return errors.New("complex-refactor repair_plan status must be repair_required")
	}
	if strings.TrimSpace(repair.Path) == "" || strings.TrimSpace(repair.RepairTaskID) == "" {
		return errors.New("complex-refactor repair_plan requires path and repair_task_id")
	}
	if repair.SchedulesWork || repair.ExecutesWork || repair.ApprovesWork {
		return errors.New("complex-refactor repair_plan must remain read-only")
	}
	if err := validatePublicSafeText(repair.Path); err != nil {
		return err
	}
	return nil
}

func validateComplexRefactorContextRepack(repack complexRefactorContextRepack) error {
	if strings.TrimSpace(repack.Status) == "" && strings.TrimSpace(repack.Path) == "" && strings.TrimSpace(repack.MissingContextReason) == "" {
		return nil
	}
	if repack.Status != "ready" {
		return errors.New("complex-refactor context_repack status must be ready")
	}
	if strings.TrimSpace(repack.Path) == "" || strings.TrimSpace(repack.MissingContextReason) == "" {
		return errors.New("complex-refactor context_repack requires path and missing_context_reason")
	}
	if repack.SchedulesWork || repack.ExecutesWork || repack.ApprovesWork {
		return errors.New("complex-refactor context_repack must remain read-only")
	}
	if err := validatePublicSafeText(repack.Path); err != nil {
		return err
	}
	if err := validatePublicSafeText(repack.MissingContextReason); err != nil {
		return err
	}
	return nil
}

func validateComplexRefactorTaskCounts(counts complexRefactorTaskCounts) error {
	if counts.Total < 0 || counts.Ready < 0 || counts.Blocked < 0 || counts.Completed < 0 || counts.Failed < 0 {
		return errors.New("complex-refactor task counts must not be negative")
	}
	if counts.Total == 0 {
		return errors.New("complex-refactor task counts require at least one task")
	}
	if counts.Ready+counts.Blocked+counts.Completed+counts.Failed != counts.Total {
		return errors.New("complex-refactor task counts must sum to total")
	}
	return nil
}
