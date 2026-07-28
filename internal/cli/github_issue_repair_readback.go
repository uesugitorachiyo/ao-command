package cli

import (
	"bytes"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"regexp"
	"strings"
	"time"
)

const (
	githubIssueRepairReadbackSchema = "ao.command.github-issue-repair-readback.v1"
	githubIssueRepairSourceSchema   = "ao.architecture.autonomous-issue-repair.discovery-result.v1"
	githubIssueRepairSourceCommit   = "b8c64860003238ab45fe7c76d7e8950f80a4043b"
	githubIssueRepairSourceDigest   = "f53c8ab36753cc645c48f391d8538ddb0b26cd9fe72edfd149e653e9975b3547"
	githubIssueRepairInputLimit     = int64(1 << 20)
	githubIssueRepairNextAction     = "Continue only through downstream governance; AO Command grants no mutation authority."
)

var (
	githubIssueRepairRunIDPattern  = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{7,127}$`)
	githubIssueRepairRepoPattern   = regexp.MustCompile(`^[A-Za-z0-9_.-]+/[A-Za-z0-9_.-]+$`)
	githubIssueRepairSHA1Pattern   = regexp.MustCompile(`^[0-9a-f]{40}$`)
	githubIssueRepairSHA256Pattern = regexp.MustCompile(`^[0-9a-f]{64}$`)
)

type githubIssueRepairDiscovery struct {
	Schema              string                           `json:"schema"`
	RunID               string                           `json:"run_id"`
	Repository          string                           `json:"repository"`
	DefaultBranch       string                           `json:"default_branch"`
	HeadSHA             string                           `json:"head_sha"`
	SourceURL           string                           `json:"source_url"`
	SnapshotLimit       int                              `json:"snapshot_limit"`
	CandidateLimit      int                              `json:"candidate_limit"`
	SelectedLimit       int                              `json:"selected_limit"`
	PageCount           int                              `json:"page_count"`
	ResponseDigests     []string                         `json:"response_digests"`
	Issues              []githubIssueRepairSnapshotIssue `json:"issues"`
	Candidates          []githubIssueRepairCandidate     `json:"candidates"`
	SelectedIssueNumber *int                             `json:"selected_issue_number"`
	ExclusionLedger     []githubIssueRepairExclusion     `json:"exclusion_ledger"`
	MutationPerformed   bool                             `json:"mutation_performed"`
	CompletedAt         string                           `json:"completed_at"`
}

type githubIssueRepairSnapshotIssue struct {
	Number        int    `json:"number"`
	State         string `json:"state"`
	UpdatedAt     string `json:"updated_at"`
	ContentDigest string `json:"content_digest"`
}

type githubIssueRepairCandidate struct {
	IssueNumber    int    `json:"issue_number"`
	Rank           int    `json:"rank"`
	DecisionDigest string `json:"decision_digest"`
}

type githubIssueRepairExclusion struct {
	IssueNumber     int      `json:"issue_number"`
	ReasonCodes     []string `json:"reason_codes"`
	EvidenceDigests []string `json:"evidence_digests"`
}

type githubIssueRepairReadbackSummary struct {
	CommandSchemaVersion string `json:"command_schema_version"`
	Schema               string `json:"schema"`
	SourceSchema         string `json:"source_schema"`
	SourceContractCommit string `json:"source_contract_commit"`
	SourceSchemaSHA256   string `json:"source_schema_sha256"`
	RunID                string `json:"run_id"`
	Repository           string `json:"repository"`
	HeadSHA              string `json:"head_sha"`
	CompletedAt          string `json:"completed_at"`
	SnapshotCount        int    `json:"snapshot_count"`
	CandidateCount       int    `json:"candidate_count"`
	ExclusionCount       int    `json:"exclusion_count"`
	SelectedIssue        *int   `json:"selected_issue"`
	Status               string `json:"status"`
	OperatorMode         string `json:"operator_mode"`
	SafeToExecute        bool   `json:"safe_to_execute"`
	ApprovesWork         bool   `json:"approves_work"`
	MutatesGitHub        bool   `json:"mutates_github"`
	ExactNextAction      string `json:"exact_next_action"`
}

func (a App) githubIssue(args []string) int {
	if len(args) == 0 || args[0] != "repair-readback" {
		fmt.Fprintln(a.Stderr, githubIssueUsage())
		return 2
	}
	return a.githubIssueRepairReadback(args[1:])
}

func githubIssueUsage() string {
	return "ao-command github-issue: usage: ao-command github-issue repair-readback --discovery PATH [--json]"
}

func (a App) githubIssueRepairReadback(args []string) int {
	var discoveryPath string
	var jsonOut bool
	fs := flag.NewFlagSet("github-issue repair-readback", flag.ContinueOnError)
	fs.SetOutput(a.Stderr)
	fs.StringVar(&discoveryPath, "discovery", "", "path to bounded discovery result JSON")
	fs.BoolVar(&jsonOut, "json", false, "emit JSON")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if strings.TrimSpace(discoveryPath) == "" {
		fmt.Fprintln(a.Stderr, "ao-command github-issue repair-readback: --discovery is required")
		return 2
	}
	if fs.NArg() != 0 {
		fmt.Fprintln(a.Stderr, githubIssueUsage())
		return 2
	}
	summary, err := readGitHubIssueRepairDiscovery(discoveryPath)
	if err != nil {
		fmt.Fprintf(a.Stderr, "ao-command github-issue repair-readback: %v\n", err)
		return 1
	}
	if jsonOut {
		return a.writeJSON(summary)
	}
	fmt.Fprintf(a.Stdout, "ao_command_github_issue_repair_readback=%s\n", summary.Status)
	fmt.Fprintf(a.Stdout, "command_schema_version=%s\n", summary.CommandSchemaVersion)
	fmt.Fprintf(a.Stdout, "schema=%s\n", summary.Schema)
	fmt.Fprintf(a.Stdout, "source_schema=%s\n", summary.SourceSchema)
	fmt.Fprintf(a.Stdout, "source_contract_commit=%s\n", summary.SourceContractCommit)
	fmt.Fprintf(a.Stdout, "source_schema_sha256=%s\n", summary.SourceSchemaSHA256)
	fmt.Fprintf(a.Stdout, "run_id=%s\n", summary.RunID)
	fmt.Fprintf(a.Stdout, "repository=%s\n", summary.Repository)
	fmt.Fprintf(a.Stdout, "head_sha=%s\n", summary.HeadSHA)
	fmt.Fprintf(a.Stdout, "completed_at=%s\n", summary.CompletedAt)
	fmt.Fprintf(a.Stdout, "snapshot_count=%d\n", summary.SnapshotCount)
	fmt.Fprintf(a.Stdout, "candidate_count=%d\n", summary.CandidateCount)
	fmt.Fprintf(a.Stdout, "exclusion_count=%d\n", summary.ExclusionCount)
	if summary.SelectedIssue == nil {
		fmt.Fprintln(a.Stdout, "selected_issue=null")
	} else {
		fmt.Fprintf(a.Stdout, "selected_issue=%d\n", *summary.SelectedIssue)
	}
	fmt.Fprintf(a.Stdout, "status=%s\n", summary.Status)
	fmt.Fprintf(a.Stdout, "operator_mode=%s\n", summary.OperatorMode)
	fmt.Fprintf(a.Stdout, "safe_to_execute=%t\n", summary.SafeToExecute)
	fmt.Fprintf(a.Stdout, "approves_work=%t\n", summary.ApprovesWork)
	fmt.Fprintf(a.Stdout, "mutates_github=%t\n", summary.MutatesGitHub)
	fmt.Fprintf(a.Stdout, "exact_next_action=%s\n", summary.ExactNextAction)
	return 0
}

func readGitHubIssueRepairDiscovery(path string) (githubIssueRepairReadbackSummary, error) {
	body, err := readGitHubIssueRepairInput(path)
	if err != nil {
		return githubIssueRepairReadbackSummary{}, err
	}
	if err := validateGitHubIssueRepairPresence(body); err != nil {
		return githubIssueRepairReadbackSummary{}, err
	}
	var discovery githubIssueRepairDiscovery
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&discovery); err != nil {
		return githubIssueRepairReadbackSummary{}, fmt.Errorf("invalid JSON: %w", err)
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		if err == nil {
			return githubIssueRepairReadbackSummary{}, errors.New("invalid JSON: multiple values are not allowed")
		}
		return githubIssueRepairReadbackSummary{}, fmt.Errorf("invalid JSON: %w", err)
	}
	if err := validateGitHubIssueRepairDiscovery(discovery); err != nil {
		return githubIssueRepairReadbackSummary{}, err
	}
	status := "candidate_selected"
	if discovery.SelectedIssueNumber == nil {
		status = "no_eligible_issue"
	}
	return githubIssueRepairReadbackSummary{
		CommandSchemaVersion: commandSchemaVersion,
		Schema:               githubIssueRepairReadbackSchema,
		SourceSchema:         githubIssueRepairSourceSchema,
		SourceContractCommit: githubIssueRepairSourceCommit,
		SourceSchemaSHA256:   githubIssueRepairSourceDigest,
		RunID:                discovery.RunID,
		Repository:           discovery.Repository,
		HeadSHA:              discovery.HeadSHA,
		CompletedAt:          discovery.CompletedAt,
		SnapshotCount:        len(discovery.Issues),
		CandidateCount:       len(discovery.Candidates),
		ExclusionCount:       len(discovery.ExclusionLedger),
		SelectedIssue:        discovery.SelectedIssueNumber,
		Status:               status,
		OperatorMode:         operatorMode,
		SafeToExecute:        false,
		ApprovesWork:         false,
		MutatesGitHub:        false,
		ExactNextAction:      githubIssueRepairNextAction,
	}, nil
}

func readGitHubIssueRepairInput(path string) ([]byte, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	body, err := io.ReadAll(io.LimitReader(file, githubIssueRepairInputLimit+1))
	if err != nil {
		return nil, err
	}
	if int64(len(body)) > githubIssueRepairInputLimit {
		return nil, fmt.Errorf("input exceeds %d bytes", githubIssueRepairInputLimit)
	}
	if err := rejectDuplicateJSONFields(body); err != nil {
		return nil, err
	}
	return body, nil
}

func validateGitHubIssueRepairPresence(body []byte) error {
	var top map[string]json.RawMessage
	if err := json.Unmarshal(body, &top); err != nil {
		return fmt.Errorf("invalid JSON: %w", err)
	}
	if err := requireGitHubIssueRepairFields("discovery result", top, false,
		"schema", "run_id", "repository", "default_branch", "head_sha", "source_url",
		"snapshot_limit", "candidate_limit", "selected_limit", "page_count",
		"response_digests", "issues", "candidates", "selected_issue_number",
		"exclusion_ledger", "mutation_performed", "completed_at"); err != nil {
		return err
	}
	for index, raw := range rawArray(top["issues"]) {
		if err := requireGitHubIssueRepairObjectFields(fmt.Sprintf("issues[%d]", index), raw,
			"number", "state", "updated_at", "content_digest"); err != nil {
			return err
		}
	}
	for index, raw := range rawArray(top["candidates"]) {
		if err := requireGitHubIssueRepairObjectFields(fmt.Sprintf("candidates[%d]", index), raw,
			"issue_number", "rank", "decision_digest"); err != nil {
			return err
		}
	}
	for index, raw := range rawArray(top["exclusion_ledger"]) {
		if err := requireGitHubIssueRepairObjectFields(fmt.Sprintf("exclusion_ledger[%d]", index), raw,
			"issue_number", "reason_codes", "evidence_digests"); err != nil {
			return err
		}
	}
	return nil
}

func requireGitHubIssueRepairObjectFields(label string, body json.RawMessage, fields ...string) error {
	var object map[string]json.RawMessage
	if err := json.Unmarshal(body, &object); err != nil {
		return fmt.Errorf("%s must be an object", label)
	}
	return requireGitHubIssueRepairFields(label, object, false, fields...)
}

func requireGitHubIssueRepairFields(label string, object map[string]json.RawMessage, allowNull bool, fields ...string) error {
	allowed := make(map[string]struct{}, len(fields))
	for _, field := range fields {
		allowed[field] = struct{}{}
		raw, exists := object[field]
		if !exists {
			return fmt.Errorf("%s missing required field %q", label, field)
		}
		if !allowNull && string(raw) == "null" && field != "selected_issue_number" {
			return fmt.Errorf("%s field %q must not be null", label, field)
		}
	}
	for field := range object {
		if _, exists := allowed[field]; !exists {
			return fmt.Errorf("%s contains unknown field %q", label, field)
		}
	}
	return nil
}

func rawArray(body json.RawMessage) []json.RawMessage {
	var values []json.RawMessage
	_ = json.Unmarshal(body, &values)
	return values
}

func validateGitHubIssueRepairDiscovery(discovery githubIssueRepairDiscovery) error {
	if discovery.Schema != githubIssueRepairSourceSchema {
		return errors.New("invalid discovery schema")
	}
	if !githubIssueRepairRunIDPattern.MatchString(discovery.RunID) {
		return errors.New("invalid run_id")
	}
	if !githubIssueRepairRepoPattern.MatchString(discovery.Repository) {
		return errors.New("invalid repository")
	}
	if len(discovery.DefaultBranch) < 1 || len(discovery.DefaultBranch) > 255 {
		return errors.New("invalid default_branch")
	}
	if !githubIssueRepairSHA1Pattern.MatchString(discovery.HeadSHA) {
		return errors.New("invalid head_sha")
	}
	expectedURL := "https://github.com/" + discovery.Repository + "/issues"
	if discovery.SourceURL != expectedURL {
		return errors.New("source_url must exactly match repository")
	}
	if discovery.SnapshotLimit < 1 || discovery.SnapshotLimit > 50 ||
		discovery.CandidateLimit < 1 || discovery.CandidateLimit > 10 ||
		discovery.SelectedLimit != 1 || discovery.PageCount < 1 {
		return errors.New("invalid discovery bounds")
	}
	if len(discovery.ResponseDigests) < 1 ||
		len(discovery.Issues) > 50 || len(discovery.Issues) > discovery.SnapshotLimit ||
		len(discovery.Candidates) > 10 || len(discovery.Candidates) > discovery.CandidateLimit ||
		len(discovery.ExclusionLedger) > 50 {
		return errors.New("discovery result exceeds declared bounds")
	}
	if err := validateUniqueDigests("response_digests", discovery.ResponseDigests); err != nil {
		return err
	}
	if discovery.MutationPerformed {
		return errors.New("discovery result must have mutation_performed=false")
	}
	if err := validateGitHubIssueRepairTimestamp("completed_at", discovery.CompletedAt, true); err != nil {
		return err
	}
	return validateGitHubIssueRepairCollections(discovery)
}

func validateGitHubIssueRepairCollections(discovery githubIssueRepairDiscovery) error {
	issues := make(map[int]struct{}, len(discovery.Issues))
	contentDigests := make(map[string]struct{}, len(discovery.Issues))
	for _, issue := range discovery.Issues {
		if issue.Number < 1 || issue.State != "open" || !githubIssueRepairSHA256Pattern.MatchString(issue.ContentDigest) {
			return errors.New("invalid snapshotted issue")
		}
		if err := validateGitHubIssueRepairTimestamp("issue updated_at", issue.UpdatedAt, false); err != nil {
			return err
		}
		if _, exists := issues[issue.Number]; exists {
			return errors.New("duplicate snapshotted issue number")
		}
		if _, exists := contentDigests[issue.ContentDigest]; exists {
			return errors.New("duplicate snapshotted issue digest")
		}
		issues[issue.Number] = struct{}{}
		contentDigests[issue.ContentDigest] = struct{}{}
	}

	candidates := make(map[int]struct{}, len(discovery.Candidates))
	decisionDigests := make(map[string]struct{}, len(discovery.Candidates))
	for index, candidate := range discovery.Candidates {
		if candidate.IssueNumber < 1 || candidate.Rank != index+1 || candidate.Rank > 10 ||
			!githubIssueRepairSHA256Pattern.MatchString(candidate.DecisionDigest) {
			return errors.New("invalid or noncontiguous candidate rank")
		}
		if _, exists := issues[candidate.IssueNumber]; !exists {
			return errors.New("candidate does not refer to a snapshotted issue")
		}
		if _, exists := candidates[candidate.IssueNumber]; exists {
			return errors.New("duplicate candidate issue")
		}
		if _, exists := decisionDigests[candidate.DecisionDigest]; exists {
			return errors.New("duplicate candidate decision digest")
		}
		candidates[candidate.IssueNumber] = struct{}{}
		decisionDigests[candidate.DecisionDigest] = struct{}{}
	}

	exclusions := make(map[int]struct{}, len(discovery.ExclusionLedger))
	evidenceDigests := make(map[string]struct{})
	for _, exclusion := range discovery.ExclusionLedger {
		if _, exists := issues[exclusion.IssueNumber]; !exists {
			return errors.New("exclusion does not refer to a snapshotted issue")
		}
		if _, exists := candidates[exclusion.IssueNumber]; exists {
			return errors.New("candidate and exclusion sets must be disjoint")
		}
		if _, exists := exclusions[exclusion.IssueNumber]; exists {
			return errors.New("snapshotted issue excluded more than once")
		}
		if err := validateGitHubIssueRepairReasons(exclusion.ReasonCodes); err != nil {
			return err
		}
		if err := validateUniqueDigestsAcross("exclusion evidence_digests", exclusion.EvidenceDigests, evidenceDigests); err != nil {
			return err
		}
		exclusions[exclusion.IssueNumber] = struct{}{}
	}
	for issueNumber := range issues {
		_, candidate := candidates[issueNumber]
		_, excluded := exclusions[issueNumber]
		if !candidate && !excluded {
			return errors.New("every non-candidate issue must be excluded exactly once")
		}
	}
	if discovery.SelectedIssueNumber == nil {
		if len(discovery.Candidates) != 0 {
			return errors.New("selected issue is required when candidates exist")
		}
		return nil
	}
	if len(discovery.Candidates) == 0 || *discovery.SelectedIssueNumber != discovery.Candidates[0].IssueNumber {
		return errors.New("selected issue must be the rank-1 candidate")
	}
	return nil
}

func validateGitHubIssueRepairReasons(reasons []string) error {
	if len(reasons) == 0 {
		return errors.New("exclusion reason_codes must not be empty")
	}
	seen := make(map[string]struct{}, len(reasons))
	for _, reason := range reasons {
		if len(reason) < 1 || len(reason) > 128 {
			return errors.New("invalid exclusion reason code")
		}
		if _, exists := seen[reason]; exists {
			return errors.New("duplicate exclusion reason code")
		}
		seen[reason] = struct{}{}
	}
	return nil
}

func validateUniqueDigests(label string, digests []string) error {
	return validateUniqueDigestsAcross(label, digests, make(map[string]struct{}))
}

func validateUniqueDigestsAcross(label string, digests []string, seen map[string]struct{}) error {
	if len(digests) == 0 {
		return fmt.Errorf("%s must not be empty", label)
	}
	for _, digest := range digests {
		if !githubIssueRepairSHA256Pattern.MatchString(digest) {
			return fmt.Errorf("invalid %s digest", label)
		}
		if _, exists := seen[digest]; exists {
			return fmt.Errorf("duplicate %s digest", label)
		}
		seen[digest] = struct{}{}
	}
	return nil
}

func validateGitHubIssueRepairTimestamp(label, value string, requireUTC bool) error {
	parsed, err := time.Parse(time.RFC3339, value)
	if err != nil {
		return fmt.Errorf("%s must be RFC3339: %w", label, err)
	}
	if requireUTC && (!strings.HasSuffix(value, "Z") || parsed.Location() != time.UTC) {
		return fmt.Errorf("%s must be RFC3339 UTC", label)
	}
	return nil
}
