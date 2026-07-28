package cli

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
)

func readActiveStackLedger(path string) (activeStackLedger, error) {
	var ledger activeStackLedger
	bytes, err := os.ReadFile(path)
	if err != nil {
		return ledger, fmt.Errorf("read ledger: %w", err)
	}
	if err := json.Unmarshal(bytes, &ledger); err != nil {
		return ledger, fmt.Errorf("invalid ledger JSON: %w", err)
	}
	if ledger.SchemaVersion != "ao.foundry.active-stack-readiness.v0.1" {
		return ledger, errors.New("invalid active-stack readiness schema_version")
	}
	if ledger.Status == "" || len(ledger.Repositories) == 0 {
		return ledger, errors.New("active-stack ledger requires status and repositories")
	}
	if ledger.ReleaseHandoff.Status == "" || len(ledger.ReleaseHandoff.Gates) == 0 {
		return ledger, errors.New("active-stack ledger requires release_handoff gates")
	}
	return ledger, nil
}

func readAtlasStatus(path string) (atlasStatusSummary, error) {
	var status foundryAtlasStatus
	bytes, err := os.ReadFile(path)
	if err != nil {
		return atlasStatusSummary{}, fmt.Errorf("read status: %w", err)
	}
	if err := json.Unmarshal(bytes, &status); err != nil {
		return atlasStatusSummary{}, fmt.Errorf("invalid status JSON: %w", err)
	}
	if err := validateFoundryAtlasStatus(status); err != nil {
		return atlasStatusSummary{}, err
	}
	return atlasStatusSummary{
		SchemaVersion:        "ao.command.atlas-status.v0.1",
		CommandSchemaVersion: commandSchemaVersion,
		Status:               status.Status,
		FoundryStatus:        path,
		Mode:                 status.Mode,
		RegistryID:           status.RegistryID,
		ImportID:             status.ImportID,
		WorkgraphID:          status.WorkgraphID,
		TargetInstance:       status.TargetInstance,
		ReadbackStatus:       status.ReadbackStatus,
		TaskID:               status.TaskID,
		TaskDigest:           status.TaskDigest,
		RunLinkDigest:        status.RunLinkDigest,
		OperatorMode:         operatorMode,
		OrchestrationOwner:   "ao-foundry",
		AtlasAuthority:       "compile_only",
		SchedulesWork:        status.SchedulesWork,
		ExecutesWork:         status.ExecutesWork,
		ApprovesWork:         status.ApprovesWork,
		MutatesRepositories:  false,
		Evidence:             status.Evidence,
		NextActions:          status.NextActions,
	}, nil
}

func readAtlasAuthorityLadderStatus(path string) (atlasAuthorityLadderSummary, error) {
	var status atlasMissionStatus
	if err := readPublicJSONFile(path, &status); err != nil {
		return atlasAuthorityLadderSummary{}, fmt.Errorf("read mission status: %w", err)
	}
	if err := validateAtlasAuthorityLadderStatus(status); err != nil {
		return atlasAuthorityLadderSummary{}, err
	}
	ladder := status.AuthorityLadder
	return atlasAuthorityLadderSummary{
		SchemaVersion:        "ao.command.atlas-authority-ladder.v0.1",
		CommandSchemaVersion: commandSchemaVersion,
		Status:               status.CompletionStatus,
		MissionStatus:        path,
		WorkgraphID:          status.WorkgraphID,
		TargetInstance:       status.TargetInstance,
		CurrentClass:         ladder.CurrentClass,
		NextClass:            ladder.NextClass,
		ProvenLiveClasses:    ladder.ProvenLiveClasses,
		DryRunReadyClasses:   ladder.DryRunReadyClasses,
		Blockers:             ladder.Blockers,
		RequiredEvidence:     ladder.RequiredEvidence,
		DeniedHigherClasses:  ladder.DeniedHigherClasses,
		DoNotAdvanceGates:    ladder.DoNotAdvanceGates,
		OperatorMode:         operatorMode,
		SchedulesWork:        status.SchedulesWork,
		ExecutesWork:         status.ExecutesWork,
		MutatesRepositories:  false,
	}, nil
}

func validateFoundryAtlasStatus(status foundryAtlasStatus) error {
	if status.SchemaVersion != "ao.foundry.atlas-status.v0.1" {
		return errors.New("invalid Foundry Atlas status schema_version")
	}
	if status.Status != "ready" || status.ReadbackStatus != "ready" {
		return errors.New("Foundry Atlas status and readback_status must be ready")
	}
	if status.Mode != "fixture_only_readback" {
		return errors.New("Foundry Atlas status mode must be fixture_only_readback")
	}
	for field, value := range map[string]string{
		"registry_id":     status.RegistryID,
		"import_id":       status.ImportID,
		"workgraph_id":    status.WorkgraphID,
		"target_instance": status.TargetInstance,
		"task_id":         status.TaskID,
		"task_digest":     status.TaskDigest,
		"run_link_digest": status.RunLinkDigest,
	} {
		if strings.TrimSpace(value) == "" {
			return fmt.Errorf("Foundry Atlas status missing %s", field)
		}
	}
	if !isSHA256Digest(status.TaskDigest) || !isSHA256Digest(status.RunLinkDigest) {
		return errors.New("Foundry Atlas status requires sha256 task and run-link digests")
	}
	if status.SchedulesWork || status.ExecutesWork || status.ApprovesWork {
		return errors.New("Foundry Atlas status must remain observer-only and cannot schedule, execute, or approve work")
	}
	if len(status.Evidence) == 0 {
		return errors.New("Foundry Atlas status requires evidence")
	}
	for key, value := range status.Evidence {
		if strings.TrimSpace(key) == "" || strings.TrimSpace(value) == "" {
			return errors.New("Foundry Atlas status evidence keys and paths must not be empty")
		}
	}
	return nil
}

func validateAtlasAuthorityLadderStatus(status atlasMissionStatus) error {
	if status.ContractVersion != "ao.atlas.mission-status.v0.1" {
		return errors.New("invalid Atlas mission status contract_version")
	}
	for field, value := range map[string]string{
		"intake_id":         status.IntakeID,
		"workgraph_id":      status.WorkgraphID,
		"target_instance":   status.TargetInstance,
		"completion_status": status.CompletionStatus,
	} {
		if strings.TrimSpace(value) == "" {
			return fmt.Errorf("Atlas authority ladder missing %s", field)
		}
	}
	if status.SchedulesWork || status.ExecutesWork {
		return errors.New("Atlas authority ladder mission status must remain read-only and cannot schedule or execute work")
	}
	if status.AuthorityLadder == nil {
		return errors.New("Atlas authority ladder mission status requires authority_ladder readback")
	}
	ladder := *status.AuthorityLadder
	for field, value := range map[string]string{
		"authority_ladder.current_class": ladder.CurrentClass,
		"authority_ladder.next_class":    ladder.NextClass,
	} {
		if strings.TrimSpace(value) == "" {
			return fmt.Errorf("Atlas authority ladder missing %s", field)
		}
	}
	for field, values := range map[string][]string{
		"authority_ladder.proven_live_classes":  ladder.ProvenLiveClasses,
		"authority_ladder.blockers":             ladder.Blockers,
		"authority_ladder.required_evidence":    ladder.RequiredEvidence,
		"authority_ladder.do_not_advance_gates": ladder.DoNotAdvanceGates,
	} {
		if len(values) == 0 {
			return fmt.Errorf("Atlas authority ladder requires %s", field)
		}
		for _, value := range values {
			if strings.TrimSpace(value) == "" {
				return fmt.Errorf("Atlas authority ladder %s entries must not be empty", field)
			}
		}
	}
	if ladder.DeniedHigherClasses == nil || len(ladder.DeniedHigherClasses) == 0 {
		return errors.New("Atlas authority ladder requires denied_higher_classes reasons")
	}
	for class, reason := range ladder.DeniedHigherClasses {
		if strings.TrimSpace(class) == "" || strings.TrimSpace(reason) == "" {
			return errors.New("Atlas authority ladder denied_higher_classes keys and reasons must not be empty")
		}
	}
	return nil
}
