package workflowpolicy

import (
	"fmt"
	"regexp"
	"sort"
	"strings"
)

const protectedPublisherConditionValue = "${{ inputs.dry_run == false && needs.validate-inputs.result == 'success' && needs.live-preflight.result == 'success' && needs.assemble-plan.result == 'success' && inputs.exact_confirmation == format('publish-ao-command-{0}-{1}-{2}-{3}-{4}-{5}', inputs.version, inputs.tag, inputs.source_commit, inputs.approved_manifest_digest, needs.validate-inputs.outputs.release_notes_digest, inputs.expected_plan_digest) }}"

var (
	publicReleaseCommand  = regexp.MustCompile(`\bgh\s+release\s+(?:upload|create)\b`)
	lineContinuation      = regexp.MustCompile(`\\[[:space:]]*\n`)
	whitespace            = regexp.MustCompile(`\s+`)
	protectedReleaseNeeds = map[string][]string{
		"validate-inputs": {},
		"build-candidate": {"validate-inputs"},
		"live-preflight":  {"validate-inputs"},
		"assemble-plan":   {"build-candidate", "validate-inputs"},
		"publish":         {"assemble-plan", "live-preflight", "validate-inputs"},
	}
)

func validateWorkflow(document map[string]any) error {
	jobs, ok := stringMap(document["jobs"])
	if !ok {
		return nil
	}
	regulated := false
	for _, name := range sortedKeys(jobs) {
		job, ok := stringMap(jobs[name])
		if !ok {
			continue
		}
		permissions := document["permissions"]
		if value, exists := job["permissions"]; exists {
			permissions = value
		}
		if artifactUploadJob(job) || writePermissions(permissions) || containsPublicReleaseCommand(job) {
			regulated = true
			break
		}
	}
	if !regulated {
		return nil
	}
	if !manualDispatchOnly(document["on"]) {
		return malformed("artifact uploads and protected publication require workflow_dispatch-only triggers")
	}
	workflowPermissions := document["permissions"]
	if !readOnlyPermissions(workflowPermissions, true) {
		return malformed("workflow permissions must be explicit read-only")
	}

	protectedPublishers := 0
	for _, name := range sortedKeys(jobs) {
		job, ok := stringMap(jobs[name])
		if !ok {
			continue
		}
		permissions := workflowPermissions
		if value, exists := job["permissions"]; exists {
			permissions = value
		}
		uploads := artifactUploadJob(job)
		publishes := containsPublicReleaseCommand(job)
		writes := writePermissions(permissions)
		protected := protectedPublisher(name, job, jobs)
		if uploads && !readOnlyPermissions(permissions, true) {
			return malformed("job %s artifact uploads require contents: read and no writes", name)
		}
		if writes {
			if !protected {
				return malformed("job %s has forbidden write permissions", name)
			}
			protectedPublishers++
		} else if publishes {
			return malformed("job %s public release command requires protected publisher", name)
		}
		if publishes && !protected {
			return malformed("job %s public release command requires protected publisher", name)
		}
	}
	if protectedPublishers > 1 {
		return malformed("multiple protected publisher jobs are forbidden")
	}
	return nil
}

func readOnlyPermissions(value any, requireContents bool) bool {
	permissions, ok := stringMap(value)
	if !ok {
		return false
	}
	if requireContents && fmt.Sprint(permissions["contents"]) != "read" {
		return false
	}
	for _, value := range permissions {
		text := fmt.Sprint(value)
		if text != "read" && text != "none" {
			return false
		}
	}
	return true
}

func containsPublicReleaseCommand(value any) bool {
	switch value := value.(type) {
	case string:
		return publicReleaseCommand.MatchString(lineContinuation.ReplaceAllString(value, " "))
	case []any:
		for _, child := range value {
			if containsPublicReleaseCommand(child) {
				return true
			}
		}
	case map[string]any:
		for key, child := range value {
			if containsPublicReleaseCommand(key) || containsPublicReleaseCommand(child) {
				return true
			}
		}
	case map[any]any:
		for key, child := range value {
			if containsPublicReleaseCommand(key) || containsPublicReleaseCommand(child) {
				return true
			}
		}
	}
	return false
}

func artifactUploadJob(job map[string]any) bool {
	steps, ok := job["steps"].([]any)
	if !ok {
		return false
	}
	for _, value := range steps {
		step, ok := stringMap(value)
		uses, isString := step["uses"].(string)
		if ok && isString && strings.HasPrefix(uses, "actions/upload-artifact@") {
			return true
		}
	}
	return false
}

func manualDispatchOnly(value any) bool {
	if trigger, ok := value.(string); ok {
		return trigger == "workflow_dispatch"
	}
	triggers, ok := stringMap(value)
	if !ok || len(triggers) != 1 {
		return false
	}
	_, ok = triggers["workflow_dispatch"]
	return ok
}

func writePermissions(value any) bool {
	if text, ok := value.(string); ok && text == "write-all" {
		return true
	}
	permissions, ok := stringMap(value)
	if !ok {
		return false
	}
	for _, value := range permissions {
		if fmt.Sprint(value) == "write" {
			return true
		}
	}
	return false
}

func jobNeeds(job map[string]any) []string {
	switch needs := job["needs"].(type) {
	case string:
		return []string{needs}
	case []any:
		result := make([]string, len(needs))
		for i, need := range needs {
			result[i] = fmt.Sprint(need)
		}
		return result
	default:
		return []string{}
	}
}

func protectedReleaseDependencyGraph(jobs map[string]any) bool {
	for name, expected := range protectedReleaseNeeds {
		job, ok := stringMap(jobs[name])
		if !ok {
			return false
		}
		actual := jobNeeds(job)
		sort.Strings(actual)
		if strings.Join(actual, "\x00") != strings.Join(expected, "\x00") {
			return false
		}
	}
	return true
}

func protectedPublisher(name string, job, jobs map[string]any) bool {
	permissions, ok := stringMap(job["permissions"])
	if name != "publish" || !ok || len(permissions) != 1 || fmt.Sprint(permissions["contents"]) != "write" ||
		job["environment"] != "protected-release" || artifactUploadJob(job) || !protectedReleaseDependencyGraph(jobs) {
		return false
	}
	condition, ok := job["if"].(string)
	return ok && strings.TrimSpace(whitespace.ReplaceAllString(condition, " ")) == protectedPublisherConditionValue
}

func stringMap(value any) (map[string]any, bool) {
	if result, ok := value.(map[string]any); ok {
		return result, true
	}
	values, ok := value.(map[any]any)
	if !ok {
		return nil, false
	}
	result := make(map[string]any, len(values))
	for key, value := range values {
		text, ok := key.(string)
		if !ok {
			text = fmt.Sprintf("\x00%T:%#v", key, key)
		}
		result[text] = value
	}
	return result, true
}

func sortedKeys(values map[string]any) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}
