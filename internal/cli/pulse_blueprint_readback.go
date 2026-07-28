package cli

import (
	"errors"
	"fmt"
	"strings"
)

func readPulseGateStatus(preflightPath, lifecyclePath, startGatePath string) (pulseGateStatusSummary, error) {
	var preflight foundryPulseIntakePreflight
	if err := readPublicJSONFile(preflightPath, &preflight); err != nil {
		return pulseGateStatusSummary{}, fmt.Errorf("read preflight: %w", err)
	}
	if err := validatePulseIntakePreflight(preflight); err != nil {
		return pulseGateStatusSummary{}, err
	}
	var lifecycle foundryPulsePRLifecycle
	if err := readPublicJSONFile(lifecyclePath, &lifecycle); err != nil {
		return pulseGateStatusSummary{}, fmt.Errorf("read lifecycle: %w", err)
	}
	if err := validatePulsePRLifecycle(lifecycle); err != nil {
		return pulseGateStatusSummary{}, err
	}
	var startGate foundryPulseOvernightStartGate
	if err := readPublicJSONFile(startGatePath, &startGate); err != nil {
		return pulseGateStatusSummary{}, fmt.Errorf("read start gate: %w", err)
	}
	if err := validatePulseOvernightStartGate(startGate); err != nil {
		return pulseGateStatusSummary{}, err
	}
	status := "ready"
	if preflight.Status == "failed" || startGate.Status == "failed" {
		status = "failed"
	} else if preflight.Status == "blocked" || startGate.Status == "blocked" || lifecycle.AllowedNextAction != "start_next_slice" {
		status = "blocked"
	}
	firstFailingCheck := firstNonEmpty(startGate.FirstFailingCheck, preflight.FirstFailingCheck)
	if status == "blocked" && firstFailingCheck == "" {
		firstFailingCheck = "pulse_gate"
	}
	blockingActions := append([]string{}, startGate.BlockingNextActions...)
	blockingActions = append(blockingActions, preflight.BlockingNextActions...)
	if lifecycle.BlockerReason != "" {
		blockingActions = append(blockingActions, lifecycle.BlockerReason)
	}
	maintenance := append([]string{}, startGate.MaintenanceSuggestions...)
	maintenance = append(maintenance, preflight.MaintenanceSuggestions...)
	return pulseGateStatusSummary{
		SchemaVersion:          "ao.command.pulse-gate-status.v0.1",
		CommandSchemaVersion:   commandSchemaVersion,
		Status:                 status,
		Preflight:              preflightPath,
		Lifecycle:              lifecyclePath,
		StartGate:              startGatePath,
		PreflightStatus:        preflight.Status,
		BlueprintStatus:        preflight.BlueprintStatus,
		AtlasStatus:            preflight.AtlasStatus,
		LifecycleStatus:        lifecycle.AllowedNextAction,
		StartGateStatus:        startGate.Status,
		AllowedNextAction:      startGate.AllowedNextAction,
		FirstFailingCheck:      firstFailingCheck,
		BlockingNextActions:    uniqueStrings(blockingActions),
		MaintenanceSuggestions: uniqueStrings(maintenance),
		SourceArtifacts:        preflight.SourceArtifacts,
		SourceHashes:           startGate.SourceHashes,
		OperatorMode:           operatorMode,
		MutatesRepositories:    false,
	}, nil
}

func readBlueprintAtlasFoundryStatus(atlasImportPath, preflightPath, foundryGatePath string) (blueprintAtlasFoundryStatusSummary, error) {
	var atlasImport atlasBlueprintImport
	if err := readPublicJSONFile(atlasImportPath, &atlasImport); err != nil {
		return blueprintAtlasFoundryStatusSummary{}, fmt.Errorf("read Atlas Blueprint import: %w", err)
	}
	if err := validateAtlasBlueprintImportReadback(atlasImport); err != nil {
		return blueprintAtlasFoundryStatusSummary{}, err
	}
	var preflight foundryPulseIntakePreflight
	if err := readPublicJSONFile(preflightPath, &preflight); err != nil {
		return blueprintAtlasFoundryStatusSummary{}, fmt.Errorf("read preflight: %w", err)
	}
	if err := validatePulseIntakePreflight(preflight); err != nil {
		return blueprintAtlasFoundryStatusSummary{}, err
	}
	var foundryGate foundryPulseOvernightStartGate
	if err := readPublicJSONFile(foundryGatePath, &foundryGate); err != nil {
		return blueprintAtlasFoundryStatusSummary{}, fmt.Errorf("read Foundry gate: %w", err)
	}
	if err := validatePulseOvernightStartGate(foundryGate); err != nil {
		return blueprintAtlasFoundryStatusSummary{}, err
	}
	if err := validateBlueprintAtlasFoundryBindings(atlasImportPath, preflightPath, atlasImport, preflight, foundryGate); err != nil {
		return blueprintAtlasFoundryStatusSummary{}, err
	}
	status := "ready"
	if atlasImport.Status == "blocked" || preflight.Status == "blocked" || foundryGate.Status == "blocked" {
		status = "blocked"
	}
	if preflight.Status == "failed" || foundryGate.Status == "failed" {
		status = "failed"
	}
	readyReason := ""
	blockedReason := ""
	if status == "ready" {
		readyReason = "Blueprint pack compiled by Atlas and Foundry gate is ready."
	} else {
		blockedReason = firstNonEmpty(foundryGate.FirstFailingCheck, preflight.FirstFailingCheck, atlasImport.Reason, "blueprint_atlas_foundry_path")
	}
	blocking := append([]string{}, foundryGate.BlockingNextActions...)
	blocking = append(blocking, preflight.BlockingNextActions...)
	blocking = append(blocking, atlasImport.BlockingNextActions...)
	atlasBlueprintImportStatus := firstNonEmpty(foundryGate.AtlasBlueprintStatus.Status, atlasImport.Status)
	policyEvidenceStatus := "missing"
	if foundryGate.PolicyEvidenceStatus.Present {
		policyEvidenceStatus = firstNonEmpty(foundryGate.PolicyEvidenceStatus.Status, "present")
	}
	return blueprintAtlasFoundryStatusSummary{
		SchemaVersion:              "ao.command.blueprint-atlas-foundry-status.v0.1",
		CommandSchemaVersion:       commandSchemaVersion,
		Status:                     status,
		AtlasBlueprintImport:       atlasImportPath,
		Preflight:                  preflightPath,
		FoundryGate:                foundryGatePath,
		BlueprintPackStatus:        atlasImport.Status,
		BlueprintPackRef:           atlasImport.BlueprintPack.Ref,
		BlueprintPackDigest:        atlasImport.BlueprintPack.Digest,
		BuildAuthorizationRef:      atlasImport.BuildAuthorization.Ref,
		AtlasImportStatus:          atlasImport.Status,
		AtlasBlueprintImportStatus: atlasBlueprintImportStatus,
		PolicyEvidenceStatus:       policyEvidenceStatus,
		AtlasPreflightStatus:       firstNonEmpty(preflight.AtlasBlueprintStatus, preflight.AtlasStatus),
		PreflightStatus:            preflight.Status,
		BlueprintStatus:            preflight.BlueprintStatus,
		FoundryGateStatus:          foundryGate.Status,
		AllowedNextAction:          foundryGate.AllowedNextAction,
		ReadyReason:                readyReason,
		BlockedReason:              blockedReason,
		BlockingNextActions:        uniqueStrings(blocking),
		OperatorMode:               operatorMode,
		SafeToExecute:              false,
		LiveExecutionProven:        false,
		MutatesRepositories:        false,
		SchedulesWork:              false,
		ExecutesWork:               false,
		ApprovesWork:               false,
		CallsProviders:             false,
	}, nil
}

func validatePulseIntakePreflight(preflight foundryPulseIntakePreflight) error {
	if preflight.SchemaVersion != "ao.foundry.pulse-intake-preflight.v0.1" {
		return errors.New("invalid Pulse intake preflight schema_version")
	}
	if !isPulseStatus(preflight.Status) {
		return fmt.Errorf("invalid Pulse intake preflight status %q", preflight.Status)
	}
	if strings.TrimSpace(preflight.BlueprintStatus) == "" || strings.TrimSpace(preflight.AtlasStatus) == "" {
		return errors.New("Pulse intake preflight requires blueprint_status and atlas_status")
	}
	for _, source := range preflight.SourceArtifacts {
		if err := validatePulseSource(source, true); err != nil {
			return fmt.Errorf("Pulse intake preflight source_artifacts: %w", err)
		}
	}
	if preflight.Status == "ready" && len(preflight.SourceArtifacts) == 0 {
		return errors.New("ready Pulse intake preflight requires source_artifacts")
	}
	return nil
}

func validatePulsePRLifecycle(lifecycle foundryPulsePRLifecycle) error {
	if lifecycle.SchemaVersion != "ao.foundry.pulse-pr-lifecycle.v0.1" {
		return errors.New("invalid Pulse PR lifecycle schema_version")
	}
	for field, value := range map[string]string{
		"current_slice":       lifecycle.CurrentSlice,
		"target_repo":         lifecycle.TargetRepo,
		"branch":              lifecycle.Branch,
		"pr_state":            lifecycle.PRState,
		"check_state":         lifecycle.CheckState,
		"merge_state":         lifecycle.MergeState,
		"cleanup_state":       lifecycle.CleanupState,
		"allowed_next_action": lifecycle.AllowedNextAction,
	} {
		if strings.TrimSpace(value) == "" {
			return fmt.Errorf("Pulse PR lifecycle missing %s", field)
		}
	}
	return nil
}

func validatePulseOvernightStartGate(startGate foundryPulseOvernightStartGate) error {
	if startGate.SchemaVersion != "ao.foundry.pulse-overnight-start-gate.v0.1" {
		return errors.New("invalid Pulse overnight start gate schema_version")
	}
	if !isPulseStatus(startGate.Status) {
		return fmt.Errorf("invalid Pulse overnight start gate status %q", startGate.Status)
	}
	if strings.TrimSpace(startGate.AllowedNextAction) == "" {
		return errors.New("Pulse overnight start gate requires allowed_next_action")
	}
	if len(startGate.SourceHashes) == 0 {
		return errors.New("Pulse overnight start gate requires source_hashes")
	}
	for _, source := range startGate.SourceHashes {
		if err := validatePulseSource(source, false); err != nil {
			return fmt.Errorf("Pulse overnight start gate source_hashes: %w", err)
		}
	}
	return nil
}

func validateAtlasBlueprintImportReadback(importRecord atlasBlueprintImport) error {
	if importRecord.ContractVersion != "ao.atlas.blueprint-import.v0.1" {
		return errors.New("invalid Atlas Blueprint import contract_version")
	}
	if strings.TrimSpace(importRecord.ID) == "" || strings.TrimSpace(importRecord.ProjectID) == "" || strings.TrimSpace(importRecord.Reason) == "" {
		return errors.New("Atlas Blueprint import requires id, project_id, and reason")
	}
	if importRecord.Status != "ready" && importRecord.Status != "blocked" {
		return fmt.Errorf("invalid Atlas Blueprint import status %q", importRecord.Status)
	}
	if err := validateAtlasSourceRef("blueprint_pack", importRecord.BlueprintPack, true); err != nil {
		return err
	}
	if len(importRecord.Digests) == 0 {
		return errors.New("Atlas Blueprint import requires digest bindings")
	}
	for key, digest := range importRecord.Digests {
		if strings.TrimSpace(key) == "" || !isSHA256Digest(digest) {
			return fmt.Errorf("Atlas Blueprint import digest %q must be sha256:<64 hex>", key)
		}
	}
	if importRecord.Status == "ready" {
		if !importRecord.ReadyForFoundry {
			return errors.New("ready Atlas Blueprint import requires ready_for_foundry=true")
		}
		for field, value := range map[string]string{
			"target_instance": importRecord.TargetInstance,
			"workgraph_id":    importRecord.WorkgraphID,
			"mutation_class":  importRecord.MutationClass,
		} {
			if strings.TrimSpace(value) == "" {
				return fmt.Errorf("ready Atlas Blueprint import missing %s", field)
			}
		}
		if err := validateAtlasSourceRef("build_authorization", importRecord.BuildAuthorization, true); err != nil {
			return err
		}
		if err := validateAtlasSourceRef("downstream_foundry_import", importRecord.DownstreamFoundryImport, true); err != nil {
			return err
		}
		if importRecord.DownstreamFoundryImport.Digest != importRecord.Digests["downstream_foundry_import"] {
			return errors.New("Atlas Blueprint import downstream Foundry import digest is not bound")
		}
	} else {
		if importRecord.ReadyForFoundry {
			return errors.New("blocked Atlas Blueprint import requires ready_for_foundry=false")
		}
		if len(importRecord.BlockingNextActions) == 0 {
			return errors.New("blocked Atlas Blueprint import requires blocking_next_actions")
		}
	}
	if importRecord.SafeToExecute || importRecord.LiveExecutionProven ||
		importRecord.SchedulesWork || importRecord.ExecutesWork || importRecord.ApprovesWork ||
		importRecord.MutatesRepositories || importRecord.CallsProviders || importRecord.ReleaseOrPublishAllowed {
		return errors.New("Atlas Blueprint import must remain read-only and cannot claim execution, scheduling, approval, provider, release, or repository mutation authority")
	}
	for _, value := range append(append([]string{}, importRecord.SafetyLimits...), importRecord.BlockingNextActions...) {
		if strings.TrimSpace(value) == "" {
			return errors.New("Atlas Blueprint import safety and blocker lists must not contain empty values")
		}
		if err := validatePublicSafeText(value); err != nil {
			return err
		}
	}
	return nil
}

func validateAtlasSourceRef(name string, source atlasSourceRef, required bool) error {
	if strings.TrimSpace(source.Ref) == "" && strings.TrimSpace(source.Digest) == "" && !required {
		return nil
	}
	if strings.TrimSpace(source.Ref) == "" || strings.TrimSpace(source.Digest) == "" {
		return fmt.Errorf("Atlas Blueprint import %s requires ref and digest", name)
	}
	if err := validatePublicSafeText(source.Ref); err != nil {
		return err
	}
	if !isSHA256Digest(source.Digest) {
		return fmt.Errorf("Atlas Blueprint import %s digest must be sha256:<64 hex>", name)
	}
	return nil
}

func validateBlueprintAtlasFoundryBindings(atlasImportPath, preflightPath string, atlasImport atlasBlueprintImport, preflight foundryPulseIntakePreflight, foundryGate foundryPulseOvernightStartGate) error {
	if preflight.AtlasBlueprintStatus != "" && preflight.AtlasBlueprintStatus != atlasImport.Status {
		return errors.New("Foundry preflight atlas_blueprint_status does not match Atlas Blueprint import status")
	}
	if atlasSource, ok := pulseSourceByName(preflight.SourceArtifacts, "atlas_blueprint_import"); ok {
		atlasSHA, err := sha256File(atlasImportPath)
		if err != nil {
			return fmt.Errorf("hash Atlas Blueprint import: %w", err)
		}
		if atlasSource.SHA256 != atlasSHA {
			return errors.New("Foundry preflight atlas_blueprint_import source digest does not match provided Atlas Blueprint import")
		}
		if atlasSource.Status != "" && atlasSource.Status != atlasImport.Status {
			return errors.New("Foundry preflight atlas_blueprint_import source status does not match Atlas Blueprint import")
		}
	} else if preflight.Status == "ready" {
		return errors.New("ready Foundry preflight must include atlas_blueprint_import source artifact")
	}
	preflightSource, ok := pulseSourceByName(foundryGate.SourceHashes, "intake_preflight")
	if !ok {
		return errors.New("Foundry gate must include intake_preflight source hash")
	}
	preflightSHA, err := sha256File(preflightPath)
	if err != nil {
		return fmt.Errorf("hash Foundry preflight: %w", err)
	}
	if preflightSource.SHA256 != preflightSHA {
		return errors.New("Foundry gate intake_preflight source digest does not match provided preflight")
	}
	return nil
}

func pulseSourceByName(sources []pulseSource, name string) (pulseSource, bool) {
	for _, source := range sources {
		if source.Name == name {
			return source, true
		}
	}
	return pulseSource{}, false
}

func validatePulseSource(source pulseSource, requireStatus bool) error {
	if strings.TrimSpace(source.Name) == "" || strings.TrimSpace(source.Path) == "" || strings.TrimSpace(source.SchemaVersion) == "" {
		return errors.New("source requires name, path, and schema_version")
	}
	if requireStatus && strings.TrimSpace(source.Status) == "" {
		return errors.New("source requires status")
	}
	if !isHexSHA256(source.SHA256) {
		return errors.New("source requires 64-character sha256")
	}
	if err := validatePublicSafeText(source.Path); err != nil {
		return err
	}
	return nil
}

func isPulseStatus(status string) bool {
	return status == "ready" || status == "blocked" || status == "failed"
}

func isHexSHA256(value string) bool {
	if len(value) != 64 {
		return false
	}
	for _, ch := range value {
		if !((ch >= '0' && ch <= '9') || (ch >= 'a' && ch <= 'f') || (ch >= 'A' && ch <= 'F')) {
			return false
		}
	}
	return true
}
