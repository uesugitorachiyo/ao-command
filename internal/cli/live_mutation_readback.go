package cli

import (
	"errors"
	"fmt"
	"time"
)

func readLiveMutationStatus(authorityPath, requestPath, forgePlanPath, ao2PacketPath, isolationPath, rollbackPath, killSwitchPath, sentinelHoldPath string) (liveMutationStatusSummary, error) {
	specs := []struct {
		name        string
		path        string
		schema      string
		allowPassed bool
	}{
		{name: "covenant_authority", path: authorityPath, schema: "covenant.live-mutation-authority.v1"},
		{name: "foundry_request", path: requestPath, schema: "ao.foundry.live-mutation-request.v0.1"},
		{name: "forge_dry_run_plan", path: forgePlanPath, schema: "ao.forge.live-mutation-dry-run-plan.v0.1"},
		{name: "ao2_dry_run_packet", path: ao2PacketPath, schema: "ao2.live-mutation-dry-run-packet.v1"},
		{name: "worktree_isolation", path: isolationPath, schema: "ao.foundry.worktree-isolation-proof.v0.1"},
		{name: "rollback_rehearsal", path: rollbackPath, schema: "ao.foundry.live-mutation-rollback-rehearsal.v0.1"},
		{name: "operator_kill_switch", path: killSwitchPath, schema: "ao.command.live-mutation-kill-switch.v0.1"},
	}

	status := "ready"
	firstFailingCheck := ""
	killSwitchState := ""
	currentMutationClass := ""
	nextMutationClass := ""
	artifacts := []liveMutationArtifactSummary{}
	rawArtifacts := []map[string]any{}
	repoStates := []liveMutationRepoState{}
	var sentinelHold *liveMutationSentinelHold
	blockingActions := []string{}
	maintenance := []string{
		"Keep this readback observer-only; it does not grant live mutation authority.",
		"Do not request a live mutation class until Sentinel and Promoter evidence also pass.",
	}

	for _, spec := range specs {
		artifact, raw, err := readLiveMutationArtifact(spec.name, spec.path, spec.schema)
		if err != nil {
			return liveMutationStatusSummary{}, err
		}
		rawArtifacts = append(rawArtifacts, raw)
		if spec.name == "operator_kill_switch" {
			killSwitchState = artifact.Status
		}
		artifacts = append(artifacts, artifact)
		if err := validateLiveMutationArtifactBoundaries(spec.name, raw); err != nil {
			return liveMutationStatusSummary{}, err
		}
		if artifactCurrentClass := liveMutationMapString(raw, "current_mutation_class"); artifactCurrentClass != "" {
			if currentMutationClass != "" && currentMutationClass != artifactCurrentClass {
				status = "blocked"
				if firstFailingCheck == "" {
					firstFailingCheck = spec.name
				}
				blockingActions = append(blockingActions, "Repair inconsistent current mutation class evidence before proceeding.")
			} else {
				currentMutationClass = artifactCurrentClass
			}
		}
		if artifactNextClass := liveMutationMapString(raw, "next_mutation_class"); artifactNextClass != "" {
			if nextMutationClass != "" && nextMutationClass != artifactNextClass {
				status = "blocked"
				if firstFailingCheck == "" {
					firstFailingCheck = spec.name
				}
				blockingActions = append(blockingActions, "Repair inconsistent next mutation class evidence before proceeding.")
			} else {
				nextMutationClass = artifactNextClass
			}
		}
		if spec.name == "foundry_request" {
			repoStates = liveMutationRepoStates(raw)
			if blocker := validateLiveMutationRepoStates(raw, repoStates); blocker != "" {
				if status == "ready" {
					status = "blocked"
				}
				if firstFailingCheck == "" {
					firstFailingCheck = "foundry_request"
				}
				blockingActions = append(blockingActions, blocker)
			}
		}
		switch artifact.Status {
		case "ready", "approved", "armed":
		case "passed":
			if !spec.allowPassed {
				status = "blocked"
				if firstFailingCheck == "" {
					firstFailingCheck = spec.name
				}
			}
		case "blocked", "hold":
			if status == "ready" {
				status = "blocked"
			}
			if firstFailingCheck == "" {
				firstFailingCheck = firstNonEmpty(artifact.FirstFailingCheck, spec.name)
			}
		case "failed", "denied":
			status = "failed"
			if firstFailingCheck == "" {
				firstFailingCheck = firstNonEmpty(artifact.FirstFailingCheck, spec.name)
			}
		default:
			if status == "ready" {
				status = "blocked"
			}
			if firstFailingCheck == "" {
				firstFailingCheck = spec.name
			}
		}
		blockingActions = append(blockingActions, liveMutationStringSlice(raw, "blocking_next_actions")...)
		maintenance = append(maintenance, liveMutationStringSlice(raw, "maintenance_suggestions")...)
	}

	if killSwitchState != "armed" {
		if status == "ready" {
			status = "blocked"
		}
		if firstFailingCheck == "" {
			firstFailingCheck = "operator_kill_switch"
		}
		blockingActions = append(blockingActions, "Arm the operator kill switch before requesting live mutation authority.")
	}
	if sentinelHoldPath != "" {
		hold, artifact, err := readLiveMutationSentinelHold(sentinelHoldPath)
		if err != nil {
			return liveMutationStatusSummary{}, err
		}
		sentinelHold = hold
		artifacts = append(artifacts, artifact)
		if hold.MutationClass != "" && nextMutationClass != "" && hold.MutationClass != nextMutationClass {
			if status == "ready" {
				status = "blocked"
			}
			if firstFailingCheck == "" {
				firstFailingCheck = "sentinel_hold"
			}
			blockingActions = append(blockingActions, "Repair Sentinel hold mutation_class mismatch before proceeding.")
		}
		if hold.Status != "clear" || hold.HoldRequired {
			if status == "ready" {
				status = "blocked"
			}
			if firstFailingCheck == "" {
				firstFailingCheck = firstNonEmpty(hold.FirstFailingCheck, "sentinel_hold")
			}
			blockingActions = append(blockingActions, "Resolve AO Sentinel live-mutation hold before requesting the next class.")
		}
	}
	allowedNextAction := "request_first_tiny_live_mutation_class"
	if status == "blocked" {
		allowedNextAction = "repair_live_mutation_evidence"
	} else if status == "failed" {
		allowedNextAction = "stop_and_rebuild_live_mutation_evidence"
	} else if nextMutationClass == "test_only" {
		allowedNextAction = "request_test_only_live_rehearsal"
	} else if nextMutationClass == "low_risk_code" {
		allowedNextAction = "request_low_risk_code_dry_run"
	} else if nextMutationClass == "multi_repo_low_risk" {
		allowedNextAction = "request_multi_repo_low_risk_dry_run"
	}
	if status != "ready" && len(blockingActions) == 0 {
		blockingActions = append(blockingActions, "Repair the first failing live-mutation evidence check before proceeding.")
	}
	requiredEvidence := liveMutationRequiredEvidence(nextMutationClass)
	deniedHigherClasses := liveMutationDeniedHigherClasses(nextMutationClass)
	highestProvenLiveClass, currentClassLiveEvidenceStatus := liveMutationHighestProvenLiveClass(currentMutationClass, rawArtifacts)
	lowRiskCodeLiveEvidenceStatus := ""
	nextDeniedClass := ""
	nextDeniedReason := ""
	var multiRepoDenial *multiRepoLiveRehearsalDenial
	if currentMutationClass == "low_risk_code" || nextMutationClass == "multi_repo_low_risk" {
		lowRiskCodeLiveEvidenceStatus = currentClassLiveEvidenceStatus
		if lowRiskCodeLiveEvidenceStatus != "completed" {
			nextDeniedClass = "multi_repo_low_risk"
			nextDeniedReason = "denied until low_risk_code live rehearsal evidence is recorded"
			if nextMutationClass == "multi_repo_low_risk" {
				multiRepoDenial = newMultiRepoLiveRehearsalDenial(status == "ready", highestProvenLiveClass, lowRiskCodeLiveEvidenceStatus, nextDeniedReason)
			}
		}
	}
	var lowRiskDenialAudit *lowRiskCodeDenialAudit
	if nextMutationClass == "low_risk_code" {
		lowRiskDenialAudit = newLowRiskCodeDenialAudit(status == "ready")
	}

	return liveMutationStatusSummary{
		SchemaVersion:                  "ao.command.live-mutation-status.v0.1",
		CommandSchemaVersion:           commandSchemaVersion,
		Status:                         status,
		AllowedNextAction:              allowedNextAction,
		FirstFailingCheck:              firstFailingCheck,
		KillSwitchState:                killSwitchState,
		Artifacts:                      artifacts,
		BlockingNextActions:            uniqueStrings(blockingActions),
		MaintenanceSuggestions:         uniqueStrings(maintenance),
		CurrentMutationClass:           currentMutationClass,
		NextMutationClass:              nextMutationClass,
		HighestProvenLiveClass:         highestProvenLiveClass,
		CurrentClassLiveEvidenceStatus: currentClassLiveEvidenceStatus,
		LowRiskCodeLiveEvidenceStatus:  lowRiskCodeLiveEvidenceStatus,
		NextDeniedClass:                nextDeniedClass,
		NextDeniedReason:               nextDeniedReason,
		SafeToRequest:                  status == "ready",
		SafeToExecute:                  false,
		RequiredEvidence:               requiredEvidence,
		LowRiskCodeDenialAudit:         lowRiskDenialAudit,
		MultiRepoLiveRehearsalDenial:   multiRepoDenial,
		SentinelHold:                   sentinelHold,
		DeniedHigherClasses:            deniedHigherClasses,
		RepoStates:                     repoStates,
		OperatorMode:                   operatorMode,
		MutatesRepositories:            false,
		SchedulesWork:                  false,
		ExecutesWork:                   false,
		ApprovesWork:                   false,
		CallsProviders:                 false,
		ReleaseOrPublishAllowed:        false,
	}, nil
}

func readLiveMutationClassDecision(rollupPath, promoterVerdictPath string) (liveMutationClassDecisionSummary, error) {
	rollupArtifact, rollup, err := readLiveMutationArtifact("foundry_complex_promotion_rollup", rollupPath, "ao.foundry.complex-repo-mutation-promotion-rollup.v0.1")
	if err != nil {
		return liveMutationClassDecisionSummary{}, err
	}
	verdictArtifact, verdict, err := readLiveMutationArtifact("promoter_complex_promotion_verdict", promoterVerdictPath, "ao.promoter.complex-repo-mutation-promotion-verdict.v0.1")
	if err != nil {
		return liveMutationClassDecisionSummary{}, err
	}
	blocking := []string{}
	status := "denied"
	highest := firstNonEmpty(liveMutationMapString(verdict, "highest_proven_live_class"), liveMutationMapString(rollup, "highest_proven_live_class"), "multi_repo_low_risk")
	nextDenied := firstNonEmpty(liveMutationMapString(verdict, "next_denied_class"), liveMutationMapString(rollup, "next_denied_class"), "complex_repo_mutation")
	firstFailingCheck := firstNonEmpty(liveMutationMapString(rollup, "first_failing_check"), liveMutationMapString(verdict, "first_failing_check"))
	promoted := liveMutationMapString(rollup, "status") == "ready" &&
		liveMutationMapString(verdict, "status") == "promoted" &&
		liveMutationMapBool(rollup, "safe_to_promote") &&
		liveMutationMapBool(verdict, "safe_to_promote") &&
		liveMutationMapBool(rollup, "complex_repo_mutation_live_proven") &&
		liveMutationMapBool(verdict, "complex_repo_mutation_live_proven") &&
		liveMutationMapString(rollup, "mutation_class") == "complex_repo_mutation" &&
		liveMutationMapString(verdict, "mutation_class") == "complex_repo_mutation" &&
		liveMutationMapString(rollup, "fully_unsupervised_complex_mutation") == "denied" &&
		liveMutationMapString(verdict, "fully_unsupervised_complex_mutation") == "denied" &&
		liveMutationMapString(rollup, "rsi") == "denied" &&
		liveMutationMapString(verdict, "rsi") == "denied"
	if promoted {
		status = "promoted"
		highest = "complex_repo_mutation"
		nextDenied = "fully_unsupervised_complex_mutation"
	} else {
		blocking = append(blocking, "Do not mark complex_repo_mutation live-proven until Foundry rollup and Promoter verdict both promote the exact class.")
		for _, blocker := range liveMutationStringSlice(rollup, "blockers") {
			blocking = append(blocking, "Foundry rollup blocker: "+blocker)
		}
		for _, raw := range liveMutationAnySlice(verdict["blockers"]) {
			if item, ok := raw.(map[string]any); ok {
				reason := liveMutationMapString(item, "reason")
				if reason != "" {
					blocking = append(blocking, "Promoter verdict blocker: "+reason)
				}
			}
		}
	}
	return liveMutationClassDecisionSummary{
		SchemaVersion:                    "ao.command.complex-repo-mutation-class-decision.v0.1",
		CommandSchemaVersion:             commandSchemaVersion,
		Status:                           status,
		MutationClass:                    "complex_repo_mutation",
		ComplexRepoMutationLiveProven:    promoted,
		HighestProvenLiveClass:           highest,
		NextDeniedClass:                  nextDenied,
		FullyUnsupervisedComplexMutation: "denied",
		RSI:                              "denied",
		SafeToRequest:                    promoted,
		SafeToExecute:                    false,
		FirstFailingCheck:                firstFailingCheck,
		BlockingNextActions:              uniqueStrings(blocking),
		Artifacts:                        []liveMutationArtifactSummary{rollupArtifact, verdictArtifact},
		OperatorMode:                     operatorMode,
		MutatesRepositories:              false,
		SchedulesWork:                    false,
		ExecutesWork:                     false,
		ApprovesWork:                     false,
		CallsProviders:                   false,
		ReleaseOrPublishAllowed:          false,
	}, nil
}

func newLowRiskCodeDenialAudit(safeToRequest bool) *lowRiskCodeDenialAudit {
	return &lowRiskCodeDenialAudit{
		SchemaVersion:          "ao.command.low-risk-code-denial-audit.v0.1",
		Status:                 "blocked",
		MutationClass:          "low_risk_code",
		CurrentProvenLiveClass: "test_only",
		NextDeniedClass:        "low_risk_code",
		SafeToRequest:          safeToRequest,
		SafeToExecute:          false,
		MissingPolicyEvidence: []string{
			"policy:low_risk_code_live_promotion",
			"command_readback:low_risk_code_live",
		},
		MissingRollbackEvidence: []string{
			"rollback_proof:low_risk_code_live",
		},
		MissingSentinelPromoterEvidence: []string{
			"sentinel_clear:low_risk_code_live",
			"promoter_promotion:low_risk_code_live",
		},
		SentinelState:   "missing_live_no_hold",
		PromoterState:   "missing_live_promotion",
		CIRequirements:  []string{"ci_passed:low_risk_code_live"},
		ExactNextAction: "build_low_risk_code_promotion_prerequisites",
		DenialReason:    "low_risk_code live execution remains denied until policy promotion, rollback proof, Sentinel clear verdict, Promoter promotion, Command readback, and PR CI evidence all exist for the exact class scope.",
	}
}

func newMultiRepoLiveRehearsalDenial(safeToRequest bool, highestProvenLiveClass, lowRiskCodeLiveEvidenceStatus, denialReason string) *multiRepoLiveRehearsalDenial {
	if highestProvenLiveClass == "" {
		highestProvenLiveClass = "test_only"
	}
	if lowRiskCodeLiveEvidenceStatus == "" {
		lowRiskCodeLiveEvidenceStatus = "missing"
	}
	if denialReason == "" {
		denialReason = "denied until low_risk_code live rehearsal evidence is recorded"
	}
	return &multiRepoLiveRehearsalDenial{
		SchemaVersion:                 "ao.command.multi-repo-live-rehearsal-denial.v0.1",
		Status:                        "denied",
		MutationClass:                 "multi_repo_low_risk",
		CurrentClass:                  "low_risk_code",
		HighestProvenLiveClass:        highestProvenLiveClass,
		LowRiskCodeLiveEvidenceStatus: lowRiskCodeLiveEvidenceStatus,
		SafeToRequest:                 safeToRequest,
		SafeToExecute:                 false,
		LiveExecutionAuthority:        false,
		MissingEvidence: []string{
			"low_risk_code_live_success",
			"rollback_proof:low_risk_code_live",
			"sentinel_no_hold:low_risk_code_live",
			"promoter_promotion:low_risk_code_live",
			"command_readback:low_risk_code_live",
			"clean_main_ci:low_risk_code_live",
		},
		ExactNextAction: "complete_low_risk_code_live_rehearsal_before_multi_repo_live",
		DenialReason:    denialReason,
	}
}

func liveMutationHighestProvenLiveClass(currentClass string, artifacts []map[string]any) (string, string) {
	if currentClass == "" {
		return "", ""
	}
	for _, artifact := range artifacts {
		rehearsal, _ := artifact["completed_live_rehearsal"].(map[string]any)
		if liveMutationMapString(rehearsal, "status") == "completed" &&
			liveMutationMapString(rehearsal, "mutation_class") == currentClass {
			return currentClass, "completed"
		}
	}
	if currentClass == "low_risk_code" {
		return previousLiveMutationClass(currentClass), "missing"
	}
	return currentClass, "completed"
}

func previousLiveMutationClass(class string) string {
	switch class {
	case "docs_only_multi_file":
		return "docs_only_single_file"
	case "test_only":
		return "docs_only_multi_file"
	case "low_risk_code":
		return "test_only"
	case "multi_repo_low_risk":
		return "low_risk_code"
	case "complex_repo_mutation":
		return "multi_repo_low_risk"
	default:
		return ""
	}
}

func readLiveMutationSentinelHold(path string) (*liveMutationSentinelHold, liveMutationArtifactSummary, error) {
	artifact, raw, err := readLiveMutationArtifact("sentinel_hold", path, "ao.sentinel.live-mutation-hold.v0.1")
	if err != nil {
		return nil, liveMutationArtifactSummary{}, err
	}
	if err := validateLiveMutationArtifactBoundaries("sentinel_hold", raw); err != nil {
		return nil, liveMutationArtifactSummary{}, err
	}
	status := liveMutationMapString(raw, "status")
	if status != "clear" && status != "hold" {
		return nil, liveMutationArtifactSummary{}, fmt.Errorf("sentinel_hold status must be clear or hold, got %q", status)
	}
	classVerdict, _ := raw["class_hold_verdict"].(map[string]any)
	hold := &liveMutationSentinelHold{
		Path:                    path,
		SchemaVersion:           artifact.SchemaVersion,
		Status:                  status,
		MutationClass:           liveMutationMapString(raw, "mutation_class"),
		HoldRequired:            liveMutationMapBool(raw, "hold_required"),
		FirstFailingCheck:       liveMutationMapString(raw, "first_failing_check"),
		ClassVerdictStatus:      liveMutationMapString(classVerdict, "status"),
		TestCoverageStatus:      liveMutationMapString(classVerdict, "test_coverage_status"),
		RollbackStatus:          liveMutationMapString(classVerdict, "rollback_status"),
		DiffSizeStatus:          liveMutationMapString(classVerdict, "diff_size_status"),
		FileClassStatus:         liveMutationMapString(classVerdict, "file_class_status"),
		EvidenceFreshnessStatus: liveMutationMapString(classVerdict, "evidence_freshness_status"),
		CIStatus:                liveMutationMapString(classVerdict, "ci_status"),
	}
	if hold.MutationClass == "" || hold.ClassVerdictStatus == "" {
		return nil, liveMutationArtifactSummary{}, errors.New("sentinel_hold requires mutation_class and class_hold_verdict.status")
	}
	return hold, artifact, nil
}

func liveMutationRequiredEvidence(nextClass string) []string {
	switch nextClass {
	case "test_only":
		return []string{
			"covenant_class_ticket:test_only",
			"foundry_class_gate:test_only",
			"ao2_bounded_patch_packet:test_only",
			"sentinel_no_hold:test_only",
			"promoter_ready:test_only",
			"rollback_proof:test_only",
			"ci_passed:test_only",
		}
	case "low_risk_code":
		return []string{
			"test_only_success",
			"covenant_class_ticket:low_risk_code",
			"foundry_class_gate:low_risk_code",
			"ao2_dry_run_packet:low_risk_code",
			"sentinel_no_hold:low_risk_code",
			"promoter_ready:low_risk_code",
			"rollback_proof:low_risk_code",
			"ci_passed:low_risk_code",
		}
	case "multi_repo_low_risk":
		return []string{
			"low_risk_code_live_success",
			"covenant_class_ticket:multi_repo_low_risk",
			"foundry_class_gate:multi_repo_low_risk",
			"multi_repo_sequencing_plan",
			"per_repo_rollback:ao-atlas",
			"per_repo_rollback:ao-foundry",
			"per_repo_rollback:ao-command",
			"prevent_concurrent_unsafe_execution",
			"sentinel_no_hold:multi_repo_low_risk",
			"promoter_ready:multi_repo_low_risk",
			"ci_passed:multi_repo_low_risk",
		}
	default:
		return nil
	}
}

func liveMutationDeniedHigherClasses(nextClass string) map[string]string {
	switch nextClass {
	case "test_only":
		reason := "denied until test_only live rehearsal, rollback proof, CI, Sentinel, Promoter, and Command evidence complete"
		return map[string]string{
			"low_risk_code":          reason,
			"multi_repo_low_risk":    reason,
			"complex_repo_mutation":  reason,
			"fully_unsupervised_rsi": "denied until every governed lower mutation class has completed live evidence and no active holds",
		}
	case "low_risk_code":
		reason := "denied until low_risk_code dry-run is promoted, live rehearsal evidence exists, rollback proof and CI pass, and no holds remain"
		return map[string]string{
			"multi_repo_low_risk":    reason,
			"complex_repo_mutation":  reason,
			"fully_unsupervised_rsi": "denied until every governed lower mutation class has completed live evidence and no active holds",
		}
	case "multi_repo_low_risk":
		reason := "denied until multi_repo_low_risk live rehearsal evidence exists, per-repo rollback proof and CI pass, and no holds remain"
		return map[string]string{
			"complex_repo_mutation":  reason,
			"fully_unsupervised_rsi": "denied until every governed lower mutation class has completed live evidence and no active holds",
		}
	default:
		return nil
	}
}

func readLiveMutationPRRehearsal(gatePath string) (liveMutationPRRehearsalSummary, error) {
	var gate struct {
		SchemaVersion          string         `json:"schema_version"`
		Status                 string         `json:"status"`
		FirstLiveClass         string         `json:"first_live_class"`
		SafeToRequest          bool           `json:"safe_to_request"`
		SafeToExecute          bool           `json:"safe_to_execute"`
		ExactNextStep          string         `json:"exact_next_step"`
		AllowedNextAction      string         `json:"allowed_next_action"`
		FirstFailingCheck      string         `json:"first_failing_check"`
		BlockingNextActions    []string       `json:"blocking_next_actions"`
		MaintenanceSuggestions []string       `json:"maintenance_suggestions"`
		SourceHashes           []pulseSource  `json:"source_hashes"`
		AuthorityBoundaries    map[string]any `json:"authority_boundaries"`
	}
	if err := readPublicJSONFile(gatePath, &gate); err != nil {
		return liveMutationPRRehearsalSummary{}, fmt.Errorf("read gate: %w", err)
	}
	if gate.SchemaVersion != "ao.foundry.live-docs-pr-rehearsal-gate.v0.1" {
		return liveMutationPRRehearsalSummary{}, errors.New("gate schema_version must be ao.foundry.live-docs-pr-rehearsal-gate.v0.1")
	}
	if gate.Status != "ready" && gate.Status != "blocked" {
		return liveMutationPRRehearsalSummary{}, fmt.Errorf("gate status must be ready or blocked, got %q", gate.Status)
	}
	if gate.FirstLiveClass != "docs_only" {
		return liveMutationPRRehearsalSummary{}, errors.New("gate first_live_class must be docs_only")
	}
	if !gate.SafeToRequest {
		return liveMutationPRRehearsalSummary{}, errors.New("gate safe_to_request must be true")
	}
	if len(gate.SourceHashes) == 0 {
		return liveMutationPRRehearsalSummary{}, errors.New("gate requires source_hashes")
	}
	for _, source := range gate.SourceHashes {
		if source.Name == "" || source.Path == "" || source.SchemaVersion == "" || len(source.SHA256) != 64 {
			return liveMutationPRRehearsalSummary{}, errors.New("gate source_hashes must include name, path, schema_version, and sha256")
		}
	}
	if gate.Status == "ready" {
		if !gate.SafeToExecute || gate.ExactNextStep != "start_first_docs_only_live_pr_rehearsal" || gate.FirstFailingCheck != "" {
			return liveMutationPRRehearsalSummary{}, errors.New("ready gate must allow only start_first_docs_only_live_pr_rehearsal with no failing check")
		}
	} else {
		if gate.SafeToExecute || gate.FirstFailingCheck == "" {
			return liveMutationPRRehearsalSummary{}, errors.New("blocked gate must keep safe_to_execute=false and report first_failing_check")
		}
	}
	if gate.AuthorityBoundaries == nil {
		return liveMutationPRRehearsalSummary{}, errors.New("gate authority_boundaries are required")
	}
	if !liveMutationMapBool(gate.AuthorityBoundaries, "emits_decision_only") {
		return liveMutationPRRehearsalSummary{}, errors.New("gate must emit decisions only")
	}
	if liveMutationMapString(gate.AuthorityBoundaries, "first_live_class") != "docs_only" {
		return liveMutationPRRehearsalSummary{}, errors.New("gate authority_boundaries.first_live_class must be docs_only")
	}
	for _, field := range []string{
		"broad_live_mutation_allowed",
		"fully_unsupervised_complex_mutation_claimed",
		"mutates_repositories",
		"creates_branch",
		"creates_worktree",
		"opens_pr",
		"merges_pr",
		"schedules_work",
		"executes_work",
		"approves_work",
		"provider_calls_allowed",
		"release_or_publish_allowed",
	} {
		if liveMutationMapBool(gate.AuthorityBoundaries, field) {
			return liveMutationPRRehearsalSummary{}, fmt.Errorf("gate expands forbidden authority via authority_boundaries.%s", field)
		}
	}
	return liveMutationPRRehearsalSummary{
		SchemaVersion:           "ao.command.live-docs-pr-rehearsal-status.v0.1",
		CommandSchemaVersion:    commandSchemaVersion,
		Status:                  gate.Status,
		Gate:                    gatePath,
		GateSchemaVersion:       gate.SchemaVersion,
		FirstLiveClass:          gate.FirstLiveClass,
		SafeToRequest:           gate.SafeToRequest,
		SafeToExecute:           gate.SafeToExecute,
		ExactNextStep:           gate.ExactNextStep,
		AllowedNextAction:       gate.AllowedNextAction,
		FirstFailingCheck:       gate.FirstFailingCheck,
		BlockingNextActions:     uniqueStrings(gate.BlockingNextActions),
		MaintenanceSuggestions:  uniqueStrings(gate.MaintenanceSuggestions),
		SourceHashes:            gate.SourceHashes,
		OperatorMode:            operatorMode,
		MutatesRepositories:     false,
		CreatesBranch:           false,
		CreatesWorktree:         false,
		OpensPR:                 false,
		MergesPR:                false,
		SchedulesWork:           false,
		ExecutesWork:            false,
		ApprovesWork:            false,
		CallsProviders:          false,
		ReleaseOrPublishAllowed: false,
	}, nil
}

func readLiveMutationApproval(requestPath, ticketPath string) (liveMutationApprovalSummary, error) {
	var request map[string]any
	var ticket map[string]any
	if err := readJSONFile(requestPath, &request); err != nil {
		return liveMutationApprovalSummary{}, fmt.Errorf("read request: %w", err)
	}
	if err := readJSONFile(ticketPath, &ticket); err != nil {
		return liveMutationApprovalSummary{}, fmt.Errorf("read ticket: %w", err)
	}
	if err := validatePublicSafeText(requestPath); err != nil {
		return liveMutationApprovalSummary{}, err
	}
	if err := validatePublicSafeText(ticketPath); err != nil {
		return liveMutationApprovalSummary{}, err
	}
	if liveMutationMapString(request, "schema_version") != "ao.foundry.live-mutation-approval-request.v0.1" {
		return liveMutationApprovalSummary{}, errors.New("request schema_version must be ao.foundry.live-mutation-approval-request.v0.1")
	}
	if liveMutationMapString(ticket, "schema_version") != "covenant.live-docs-approval-ticket.v1" {
		return liveMutationApprovalSummary{}, errors.New("ticket schema_version must be covenant.live-docs-approval-ticket.v1")
	}
	requestSHA, err := sha256File(requestPath)
	if err != nil {
		return liveMutationApprovalSummary{}, fmt.Errorf("hash request: %w", err)
	}
	ticketSHA, err := sha256File(ticketPath)
	if err != nil {
		return liveMutationApprovalSummary{}, fmt.Errorf("hash ticket: %w", err)
	}
	summary := liveMutationApprovalSummary{
		SchemaVersion:        "ao.command.live-mutation-approval-status.v0.1",
		CommandSchemaVersion: commandSchemaVersion,
		Status:               "blocked",
		SafeToRequest:        liveMutationMapBool(request, "safe_to_request"),
		SafeToExecute:        false,
		ApprovalState:        liveMutationMapString(ticket, "approval_state"),
		RequestID:            liveMutationMapString(ticket, "request_id"),
		TicketID:             liveMutationMapString(ticket, "ticket_id"),
		RequestSHA256:        requestSHA,
		TicketSHA256:         ticketSHA,
		FirstFailingCheck:    "",
		OperatorMode:         operatorMode,
		MutatesRepositories:  false,
		ApprovesWork:         false,
		ExecutesWork:         false,
		CallsProviders:       false,
	}
	if summary.RequestID != liveMutationMapString(request, "request_id") {
		summary.FirstFailingCheck = "request_id_mismatch"
		return summary, nil
	}
	if summary.ApprovalState != "approved" {
		summary.FirstFailingCheck = "approval_state"
		return summary, nil
	}
	if liveMutationMapBool(ticket, "consumed") {
		summary.FirstFailingCheck = "ticket_consumed"
		return summary, nil
	}
	expiresAt, err := time.Parse(time.RFC3339, liveMutationMapString(ticket, "expires_at"))
	if err != nil {
		return liveMutationApprovalSummary{}, fmt.Errorf("ticket expires_at must be RFC3339: %w", err)
	}
	if !expiresAt.After(time.Now().UTC()) {
		summary.FirstFailingCheck = "ticket_expired"
		return summary, nil
	}
	scope, ok := ticket["approved_scope"].(map[string]any)
	if !ok {
		return liveMutationApprovalSummary{}, errors.New("ticket approved_scope is required")
	}
	for _, field := range []string{"repo", "branch_policy", "docs_only_path_allowlist", "forbidden_paths", "max_changed_files"} {
		if !jsonEquivalent(scope[field], request[field]) {
			summary.FirstFailingCheck = "scope_mismatch"
			return summary, nil
		}
	}
	summary.Status = "approved"
	summary.SafeToExecute = true
	return summary, nil
}
