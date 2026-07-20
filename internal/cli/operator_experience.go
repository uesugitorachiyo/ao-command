package cli

import (
	"bytes"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/url"
	"os"
	"path"
	"sort"
	"strings"
	"time"
	"unicode"
)

const maxOperatorStatusBytes = 1 << 20

type operatorStatusSource struct {
	Schema           string                     `json:"schema"`
	ReportedStatus   string                     `json:"reported_status"`
	Objective        string                     `json:"objective"`
	CorrelationID    string                     `json:"correlation_id"`
	ActiveRepository string                     `json:"active_repository"`
	WorkgraphID      string                     `json:"workgraph_id"`
	ActiveNodeID     string                     `json:"active_node_id"`
	Nodes            operatorNodeCounts         `json:"nodes"`
	Approval         operatorApprovalStatus     `json:"approval"`
	Verification     operatorVerificationStatus `json:"verification"`
	Worker           operatorWorkerStatus       `json:"worker"`
	StartedAt        string                     `json:"started_at"`
	ProgressPercent  int                        `json:"progress_percent"`
	Release          operatorReleaseStatus      `json:"release"`
	ExactBlocker     string                     `json:"exact_blocker"`
	ExactNextAction  string                     `json:"exact_next_action"`
	Evidence         []operatorEvidenceLink     `json:"evidence"`
	FinalReport      operatorFinalReport        `json:"final_report"`
	Safety           operatorStatusSafety       `json:"safety"`
}

type operatorNodeCounts struct {
	Total     int `json:"total"`
	Completed int `json:"completed"`
	Running   int `json:"running"`
	Ready     int `json:"ready"`
	Blocked   int `json:"blocked"`
	Remaining int `json:"remaining"`
}

type operatorApprovalStatus struct {
	State            string `json:"state"`
	ActionDigest     string `json:"action_digest"`
	ExactInstruction string `json:"exact_instruction"`
}

type operatorVerificationStatus struct {
	Tests operatorVerificationCheck `json:"tests"`
	CI    operatorVerificationCheck `json:"ci"`
}

type operatorVerificationCheck struct {
	Status   string   `json:"status"`
	Evidence []string `json:"evidence"`
}

type operatorWorkerStatus struct {
	State           string `json:"state"`
	HeartbeatAt     string `json:"heartbeat_at"`
	FreshForSeconds int    `json:"fresh_for_seconds"`
	Freshness       string `json:"freshness,omitempty"`
}

type operatorReleaseStatus struct {
	Status               string `json:"status"`
	MissionVersion       string `json:"mission_version"`
	CommandVersion       string `json:"command_version"`
	PubliclyAvailable    bool   `json:"publicly_available"`
	PublicationAttempted bool   `json:"publication_attempted"`
}

type operatorEvidenceLink struct {
	Name     string `json:"name"`
	Location string `json:"location"`
	SHA256   string `json:"sha256"`
}

type operatorFinalReport struct {
	Available bool   `json:"available"`
	Location  string `json:"location"`
	SHA256    string `json:"sha256"`
}

type operatorStatusSafety struct {
	OperatorMode        string `json:"operator_mode"`
	SafeToExecute       bool   `json:"safe_to_execute"`
	ExecutesWork        bool   `json:"executes_work"`
	ApprovesWork        bool   `json:"approves_work"`
	MutatesRepositories bool   `json:"mutates_repositories"`
	CallsProviders      bool   `json:"calls_providers"`
	ReleasesOrDeploys   bool   `json:"releases_or_deploys"`
}

type operatorStatusSummary struct {
	CommandSchemaVersion string                     `json:"command_schema_version"`
	Schema               string                     `json:"schema"`
	Status               string                     `json:"status"`
	Objective            string                     `json:"objective"`
	CorrelationID        string                     `json:"correlation_id"`
	ActiveRepository     string                     `json:"active_repository"`
	WorkgraphID          string                     `json:"workgraph_id"`
	ActiveNodeID         string                     `json:"active_node_id"`
	Nodes                operatorNodeCounts         `json:"nodes"`
	Approval             operatorApprovalStatus     `json:"approval"`
	Verification         operatorVerificationStatus `json:"verification"`
	Worker               operatorWorkerStatus       `json:"worker"`
	ObservedAt           string                     `json:"observed_at"`
	ElapsedSeconds       int64                      `json:"elapsed_seconds"`
	ProgressPercent      int                        `json:"progress_percent"`
	Release              operatorReleaseStatus      `json:"release"`
	ExactBlocker         string                     `json:"exact_blocker"`
	ExactNextAction      string                     `json:"exact_next_action"`
	Evidence             []operatorEvidenceLink     `json:"evidence"`
	FinalReport          operatorFinalReport        `json:"final_report"`
	OperatorMode         string                     `json:"operator_mode"`
	SafeToExecute        bool                       `json:"safe_to_execute"`
	ExecutesWork         bool                       `json:"executes_work"`
	ApprovesWork         bool                       `json:"approves_work"`
	MutatesRepositories  bool                       `json:"mutates_repositories"`
	CallsProviders       bool                       `json:"calls_providers"`
	ReleasesOrDeploys    bool                       `json:"releases_or_deploys"`
}

func (a App) operatorStatus(args []string) int {
	var readbackPath, observedAt string
	var jsonOut bool
	fs := flag.NewFlagSet("operator status", flag.ContinueOnError)
	fs.SetOutput(a.Stderr)
	fs.StringVar(&readbackPath, "readback", "", "path to consolidated operator status source JSON")
	fs.StringVar(&observedAt, "at", "", "RFC3339 observation time; defaults to current UTC")
	fs.BoolVar(&jsonOut, "json", false, "emit JSON")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if strings.TrimSpace(readbackPath) == "" {
		fmt.Fprintln(a.Stderr, "ao-command operator status: --readback is required")
		return 2
	}
	at := time.Now().UTC()
	if observedAt != "" {
		parsed, err := time.Parse(time.RFC3339, observedAt)
		if err != nil {
			fmt.Fprintln(a.Stderr, "ao-command operator status: --at must be RFC3339")
			return 2
		}
		at = parsed.UTC()
	}
	summary, err := readOperatorStatus(readbackPath, at)
	if err != nil {
		fmt.Fprintf(a.Stderr, "ao-command operator status: %v\n", err)
		return 1
	}
	if jsonOut {
		return a.writeJSON(summary)
	}
	fmt.Fprintf(a.Stdout, "ao_command_operator_status=%s\n", summary.Status)
	fmt.Fprintf(a.Stdout, "objective=%s\n", summary.Objective)
	fmt.Fprintf(a.Stdout, "correlation_id=%s\n", summary.CorrelationID)
	fmt.Fprintf(a.Stdout, "active_repository=%s\n", summary.ActiveRepository)
	fmt.Fprintf(a.Stdout, "active_workgraph_node=%s\n", summary.ActiveNodeID)
	fmt.Fprintf(a.Stdout, "nodes=completed:%d running:%d ready:%d blocked:%d remaining:%d total:%d\n",
		summary.Nodes.Completed, summary.Nodes.Running, summary.Nodes.Ready,
		summary.Nodes.Blocked, summary.Nodes.Remaining, summary.Nodes.Total)
	fmt.Fprintf(a.Stdout, "approval_state=%s\n", summary.Approval.State)
	if summary.Approval.State == "waiting" {
		fmt.Fprintf(a.Stdout, "approval_action_digest=%s\n", summary.Approval.ActionDigest)
		fmt.Fprintf(a.Stdout, "approval_exact_instruction=%s\n", summary.Approval.ExactInstruction)
	}
	fmt.Fprintf(a.Stdout, "tests=%s\n", summary.Verification.Tests.Status)
	fmt.Fprintf(a.Stdout, "ci=%s\n", summary.Verification.CI.Status)
	fmt.Fprintf(a.Stdout, "worker_state=%s\n", summary.Worker.State)
	fmt.Fprintf(a.Stdout, "worker_freshness=%s\n", summary.Worker.Freshness)
	fmt.Fprintf(a.Stdout, "elapsed_seconds=%d\n", summary.ElapsedSeconds)
	fmt.Fprintf(a.Stdout, "progress_percent=%d\n", summary.ProgressPercent)
	fmt.Fprintf(a.Stdout, "release_state=%s\n", summary.Release.Status)
	fmt.Fprintf(a.Stdout, "exact_blocker=%s\n", summary.ExactBlocker)
	fmt.Fprintf(a.Stdout, "exact_next_action=%s\n", summary.ExactNextAction)
	fmt.Fprintf(a.Stdout, "evidence_links=%d\n", len(summary.Evidence))
	fmt.Fprintf(a.Stdout, "final_report_available=%t\n", summary.FinalReport.Available)
	fmt.Fprintf(a.Stdout, "operator_mode=%s\n", summary.OperatorMode)
	return 0
}

func readOperatorStatus(path string, at time.Time) (operatorStatusSummary, error) {
	var source operatorStatusSource
	if err := decodeStrictBoundedJSON(path, maxOperatorStatusBytes, &source); err != nil {
		return operatorStatusSummary{}, err
	}
	if err := validateOperatorStatusSource(source); err != nil {
		return operatorStatusSummary{}, err
	}
	startedAt, _ := time.Parse(time.RFC3339, source.StartedAt)
	if at.Before(startedAt) {
		return operatorStatusSummary{}, errors.New("observation time must not precede started_at")
	}
	freshness := "not_applicable"
	status := deriveOperatorStatus(source)
	if status != source.ReportedStatus {
		return operatorStatusSummary{}, fmt.Errorf(
			"reported_status %q contradicts derived status %q", source.ReportedStatus, status,
		)
	}
	blocker := source.ExactBlocker
	nextAction := source.ExactNextAction
	if source.Worker.State == "running" {
		heartbeat, _ := time.Parse(time.RFC3339, source.Worker.HeartbeatAt)
		if at.Before(heartbeat) {
			return operatorStatusSummary{}, errors.New("observation time must not precede worker heartbeat")
		}
		if at.Sub(heartbeat) > time.Duration(source.Worker.FreshForSeconds)*time.Second {
			freshness = "stale"
			if status == "running" {
				status = "stale"
				blocker = "worker heartbeat exceeded its freshness window"
				nextAction = "refresh the worker heartbeat before trusting running state"
			}
		} else {
			freshness = "fresh"
		}
	}
	evidence := append([]operatorEvidenceLink(nil), source.Evidence...)
	sort.Slice(evidence, func(i, j int) bool { return evidence[i].Name < evidence[j].Name })
	worker := source.Worker
	worker.Freshness = freshness
	return operatorStatusSummary{
		CommandSchemaVersion: commandSchemaVersion,
		Schema:               "ao.command.operator-status.v0.1",
		Status:               status,
		Objective:            source.Objective,
		CorrelationID:        source.CorrelationID,
		ActiveRepository:     source.ActiveRepository,
		WorkgraphID:          source.WorkgraphID,
		ActiveNodeID:         source.ActiveNodeID,
		Nodes:                source.Nodes,
		Approval:             source.Approval,
		Verification:         source.Verification,
		Worker:               worker,
		ObservedAt:           at.Format(time.RFC3339),
		ElapsedSeconds:       int64(at.Sub(startedAt) / time.Second),
		ProgressPercent:      source.ProgressPercent,
		Release:              source.Release,
		ExactBlocker:         blocker,
		ExactNextAction:      nextAction,
		Evidence:             evidence,
		FinalReport:          source.FinalReport,
		OperatorMode:         operatorMode,
		SafeToExecute:        false,
		ExecutesWork:         false,
		ApprovesWork:         false,
		MutatesRepositories:  false,
		CallsProviders:       false,
		ReleasesOrDeploys:    false,
	}, nil
}

func validateOperatorStatusSource(source operatorStatusSource) error {
	if source.Schema != "ao.operator.status-source.v0.1" {
		return errors.New("schema must be ao.operator.status-source.v0.1")
	}
	if !stringIn(source.ReportedStatus, []string{"running", "waiting_approval", "ready", "blocked", "completed"}) {
		return fmt.Errorf("unsupported reported_status %q", source.ReportedStatus)
	}
	if strings.TrimSpace(source.Objective) == "" || len(source.Objective) > 1024 {
		return errors.New("objective must be present and at most 1024 bytes")
	}
	if containsControlCharacter(source.Objective) {
		return errors.New("objective must not contain control characters")
	}
	if !correlationIDPattern.MatchString(source.CorrelationID) {
		return errors.New("correlation_id must match [A-Za-z0-9][A-Za-z0-9._:-]{0,127}")
	}
	for _, field := range []struct {
		label string
		value string
		limit int
	}{
		{"active_repository", source.ActiveRepository, 128},
		{"workgraph_id", source.WorkgraphID, 256},
		{"active_node_id", source.ActiveNodeID, 256},
		{"exact_next_action", source.ExactNextAction, 2048},
	} {
		if err := validateBoundedLine(field.label, field.value, field.limit, false); err != nil {
			return err
		}
	}
	counts := source.Nodes
	if counts.Total < 1 || counts.Completed < 0 || counts.Running < 0 ||
		counts.Ready < 0 || counts.Blocked < 0 || counts.Remaining < 0 {
		return errors.New("node counts must be non-negative and total must be positive")
	}
	if counts.Total != counts.Completed+counts.Running+counts.Ready+counts.Blocked ||
		counts.Remaining != counts.Total-counts.Completed {
		return errors.New("node counts are inconsistent")
	}
	if counts.Running > 1 {
		return errors.New("operator status supports at most one running node")
	}
	if counts.Running == 0 && source.Worker.State == "running" {
		return errors.New("running worker requires one running node")
	}
	if counts.Running == 1 && strings.TrimSpace(source.ActiveNodeID) == "" {
		return errors.New("running node requires active_node_id")
	}
	if err := validateOperatorApproval(source.Approval); err != nil {
		return err
	}
	if err := validateOperatorVerification("tests", source.Verification.Tests); err != nil {
		return err
	}
	if err := validateOperatorVerification("ci", source.Verification.CI); err != nil {
		return err
	}
	if err := validateOperatorWorker(source.Worker); err != nil {
		return err
	}
	if _, err := time.Parse(time.RFC3339, source.StartedAt); err != nil {
		return errors.New("started_at must be RFC3339")
	}
	if source.ProgressPercent < 0 || source.ProgressPercent > 100 {
		return errors.New("progress_percent must be between 0 and 100")
	}
	if err := validateOperatorRelease(source.Release); err != nil {
		return err
	}
	if len(source.ExactBlocker) > 2048 {
		return errors.New("exact_blocker exceeds 2048 bytes")
	}
	if containsControlCharacter(source.ExactBlocker) {
		return errors.New("exact_blocker must not contain control characters")
	}
	if deriveOperatorStatus(source) == "blocked" && strings.TrimSpace(source.ExactBlocker) == "" {
		return errors.New("blocked status requires exact_blocker")
	}
	if len(source.Evidence) == 0 || len(source.Evidence) > 64 {
		return errors.New("evidence must contain between 1 and 64 links")
	}
	seen := make(map[string]struct{}, len(source.Evidence))
	for _, evidence := range source.Evidence {
		if strings.TrimSpace(evidence.Name) == "" || len(evidence.Name) > 128 {
			return errors.New("evidence name must be present and bounded")
		}
		if containsControlCharacter(evidence.Name) {
			return errors.New("evidence name must not contain control characters")
		}
		if _, ok := seen[evidence.Name]; ok {
			return fmt.Errorf("duplicate evidence name %q", evidence.Name)
		}
		seen[evidence.Name] = struct{}{}
		if !isSafeEvidenceLocation(evidence.Location) {
			return fmt.Errorf("evidence %q location must be relative or HTTPS", evidence.Name)
		}
		if !isSHA256Digest(evidence.SHA256) {
			return fmt.Errorf("evidence %q sha256 must be a SHA-256 digest", evidence.Name)
		}
	}
	if err := validateOperatorFinalReport(source.FinalReport); err != nil {
		return err
	}
	if source.Safety.OperatorMode != operatorMode ||
		source.Safety.SafeToExecute || source.Safety.ExecutesWork ||
		source.Safety.ApprovesWork || source.Safety.MutatesRepositories ||
		source.Safety.CallsProviders || source.Safety.ReleasesOrDeploys {
		return errors.New("operator status source must remain read-only and must not claim execution, approval, repository mutation, provider, release, or deployment authority")
	}
	if source.ReportedStatus == "completed" {
		if counts.Completed != counts.Total || counts.Running != 0 || counts.Ready != 0 || counts.Blocked != 0 ||
			source.Verification.Tests.Status != "passed" || source.Verification.CI.Status != "passed" ||
			source.Worker.State == "running" || !source.FinalReport.Available {
			return errors.New("completed status requires all nodes completed, passed tests and CI, a stopped worker, and a digest-bound final report")
		}
	}
	return nil
}

func validateOperatorApproval(approval operatorApprovalStatus) error {
	switch approval.State {
	case "none":
		if approval.ActionDigest != "" || approval.ExactInstruction != "" {
			return errors.New("approval state none must not include a digest or instruction")
		}
	case "waiting":
		if !isSHA256Digest(approval.ActionDigest) || strings.TrimSpace(approval.ExactInstruction) == "" ||
			len(approval.ExactInstruction) > 2048 {
			return errors.New("waiting approval requires an exact SHA-256 digest and bounded instruction")
		}
		if containsControlCharacter(approval.ExactInstruction) {
			return errors.New("approval instruction must not contain control characters")
		}
	default:
		return fmt.Errorf("unsupported approval state %q", approval.State)
	}
	return nil
}

func validateOperatorVerification(label string, check operatorVerificationCheck) error {
	if !stringIn(check.Status, []string{"passed", "running", "failed", "blocked", "not_run"}) {
		return fmt.Errorf("%s has unsupported status %q", label, check.Status)
	}
	if len(check.Evidence) > 32 {
		return fmt.Errorf("%s evidence exceeds 32 links", label)
	}
	if check.Status == "passed" && len(check.Evidence) == 0 {
		return fmt.Errorf("%s passed status requires evidence", label)
	}
	for _, location := range check.Evidence {
		if !isSafeEvidenceLocation(location) {
			return fmt.Errorf("%s evidence location must be relative or HTTPS", label)
		}
	}
	return nil
}

func validateOperatorWorker(worker operatorWorkerStatus) error {
	if !stringIn(worker.State, []string{"running", "idle", "stopped"}) {
		return fmt.Errorf("unsupported worker state %q", worker.State)
	}
	if worker.State == "running" {
		if _, err := time.Parse(time.RFC3339, worker.HeartbeatAt); err != nil {
			return errors.New("running worker heartbeat_at must be RFC3339")
		}
		if worker.FreshForSeconds < 1 || worker.FreshForSeconds > 86400 {
			return errors.New("running worker fresh_for_seconds must be between 1 and 86400")
		}
	} else if worker.FreshForSeconds < 0 || worker.FreshForSeconds > 86400 {
		return errors.New("worker fresh_for_seconds must be between 0 and 86400")
	}
	return nil
}

func validateOperatorRelease(release operatorReleaseStatus) error {
	if !stringIn(release.Status, []string{"not_attempted", "candidate_only", "no_release"}) {
		return fmt.Errorf("unsupported release status %q", release.Status)
	}
	if release.PubliclyAvailable || release.PublicationAttempted {
		return errors.New("operator status source must not claim public availability or publication")
	}
	if len(release.MissionVersion) > 128 || len(release.CommandVersion) > 128 ||
		containsControlCharacter(release.MissionVersion) || containsControlCharacter(release.CommandVersion) {
		return errors.New("release versions must be bounded single-line values")
	}
	if release.Status == "candidate_only" &&
		(strings.TrimSpace(release.MissionVersion) == "" || strings.TrimSpace(release.CommandVersion) == "") {
		return errors.New("candidate_only release status requires Mission and Command versions")
	}
	return nil
}

func validateOperatorFinalReport(report operatorFinalReport) error {
	if report.Available {
		if !isSafeEvidenceLocation(report.Location) || !isSHA256Digest(report.SHA256) {
			return errors.New("available final report requires a safe location and SHA-256 digest")
		}
	} else if report.Location != "" || report.SHA256 != "" {
		return errors.New("unavailable final report must not include location or digest")
	}
	return nil
}

func deriveOperatorStatus(source operatorStatusSource) string {
	if source.Approval.State == "waiting" {
		return "waiting_approval"
	}
	if source.Nodes.Blocked > 0 ||
		stringIn(source.Verification.Tests.Status, []string{"failed", "blocked"}) ||
		stringIn(source.Verification.CI.Status, []string{"failed", "blocked"}) ||
		strings.TrimSpace(source.ExactBlocker) != "" {
		return "blocked"
	}
	if source.Nodes.Running > 0 {
		return "running"
	}
	if source.Nodes.Ready > 0 {
		return "ready"
	}
	return "completed"
}

func isSafeEvidenceLocation(location string) bool {
	if strings.TrimSpace(location) == "" || len(location) > 2048 {
		return false
	}
	if containsControlCharacter(location) {
		return false
	}
	if strings.Contains(location, `\`) {
		location = strings.ReplaceAll(location, `\`, "/")
	}
	if parsed, err := url.Parse(location); err == nil && parsed.Scheme != "" {
		return parsed.Scheme == "https" && parsed.Host != "" && parsed.User == nil
	}
	if strings.HasPrefix(location, "/") || strings.HasPrefix(location, "//") ||
		(len(location) >= 3 && location[1] == ':' && location[2] == '/') {
		return false
	}
	clean := path.Clean(location)
	return clean != "." && clean != ".." && !strings.HasPrefix(clean, "../")
}

func validateBoundedLine(label, value string, limit int, allowEmpty bool) error {
	if (!allowEmpty && strings.TrimSpace(value) == "") || len(value) > limit {
		return fmt.Errorf("%s must be present and at most %d bytes", label, limit)
	}
	if containsControlCharacter(value) {
		return fmt.Errorf("%s must not contain control characters", label)
	}
	return nil
}

func containsControlCharacter(value string) bool {
	for _, character := range value {
		if unicode.IsControl(character) {
			return true
		}
	}
	return false
}

func decodeStrictBoundedJSON(path string, limit int64, target any) error {
	file, err := os.Open(path)
	if err != nil {
		return err
	}
	defer file.Close()
	body, err := io.ReadAll(io.LimitReader(file, limit+1))
	if err != nil {
		return err
	}
	if int64(len(body)) > limit {
		return fmt.Errorf("input exceeds %d bytes", limit)
	}
	if err := rejectDuplicateJSONFields(body); err != nil {
		return err
	}
	if _, ok := target.(*operatorStatusSource); ok {
		if err := validateOperatorStatusFieldPresence(body); err != nil {
			return err
		}
	}
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return fmt.Errorf("invalid JSON: %w", err)
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		if err == nil {
			return errors.New("invalid JSON: multiple values are not allowed")
		}
		return fmt.Errorf("invalid JSON: %w", err)
	}
	return nil
}

func rejectDuplicateJSONFields(body []byte) error {
	decoder := json.NewDecoder(bytes.NewReader(body))
	if err := scanStrictJSONValue(decoder); err != nil {
		return err
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		if err == nil {
			return errors.New("invalid JSON: multiple values are not allowed")
		}
		return fmt.Errorf("invalid JSON: %w", err)
	}
	return nil
}

func scanStrictJSONValue(decoder *json.Decoder) error {
	token, err := decoder.Token()
	if err != nil {
		return fmt.Errorf("invalid JSON: %w", err)
	}
	delimiter, isDelimiter := token.(json.Delim)
	if !isDelimiter {
		return nil
	}
	switch delimiter {
	case '{':
		seen := make(map[string]struct{})
		for decoder.More() {
			token, err := decoder.Token()
			if err != nil {
				return fmt.Errorf("invalid JSON: %w", err)
			}
			field, ok := token.(string)
			if !ok {
				return errors.New("invalid JSON: field name must be a string")
			}
			if _, exists := seen[field]; exists {
				return fmt.Errorf("invalid JSON: duplicate field %q", field)
			}
			seen[field] = struct{}{}
			if err := scanStrictJSONValue(decoder); err != nil {
				return err
			}
		}
	case '[':
		for decoder.More() {
			if err := scanStrictJSONValue(decoder); err != nil {
				return err
			}
		}
	default:
		return errors.New("invalid JSON: unexpected delimiter")
	}
	if _, err := decoder.Token(); err != nil {
		return fmt.Errorf("invalid JSON: %w", err)
	}
	return nil
}

func validateOperatorStatusFieldPresence(body []byte) error {
	var top map[string]json.RawMessage
	if err := json.Unmarshal(body, &top); err != nil {
		return fmt.Errorf("invalid JSON: %w", err)
	}
	if err := requireRawFields("operator status source", top,
		"schema", "reported_status", "objective", "correlation_id",
		"active_repository", "workgraph_id", "active_node_id", "nodes",
		"approval", "verification", "worker", "started_at", "progress_percent",
		"release", "exact_blocker", "exact_next_action", "evidence",
		"final_report", "safety"); err != nil {
		return err
	}
	requiredObjects := map[string][]string{
		"nodes":        {"total", "completed", "running", "ready", "blocked", "remaining"},
		"approval":     {"state", "action_digest", "exact_instruction"},
		"verification": {"tests", "ci"},
		"worker":       {"state", "heartbeat_at", "fresh_for_seconds"},
		"release":      {"status", "mission_version", "command_version", "publicly_available", "publication_attempted"},
		"final_report": {"available", "location", "sha256"},
		"safety":       {"operator_mode", "safe_to_execute", "executes_work", "approves_work", "mutates_repositories", "calls_providers", "releases_or_deploys"},
	}
	for field, required := range requiredObjects {
		object, err := rawObject(top[field], field)
		if err != nil {
			return err
		}
		if err := requireRawFields(field, object, required...); err != nil {
			return err
		}
	}
	verification, _ := rawObject(top["verification"], "verification")
	for _, field := range []string{"tests", "ci"} {
		check, err := rawObject(verification[field], "verification."+field)
		if err != nil {
			return err
		}
		if err := requireRawFields("verification."+field, check, "status", "evidence"); err != nil {
			return err
		}
	}
	var evidence []map[string]json.RawMessage
	if err := json.Unmarshal(top["evidence"], &evidence); err != nil {
		return errors.New("evidence must be an array of objects")
	}
	for index, item := range evidence {
		if err := requireRawFields(fmt.Sprintf("evidence[%d]", index), item, "name", "location", "sha256"); err != nil {
			return err
		}
	}
	return nil
}

func rawObject(raw json.RawMessage, label string) (map[string]json.RawMessage, error) {
	var object map[string]json.RawMessage
	if err := json.Unmarshal(raw, &object); err != nil || object == nil {
		return nil, fmt.Errorf("%s must be an object", label)
	}
	return object, nil
}

func requireRawFields(label string, object map[string]json.RawMessage, fields ...string) error {
	for _, field := range fields {
		if _, ok := object[field]; !ok {
			return fmt.Errorf("%s missing required field %q", label, field)
		}
	}
	return nil
}
