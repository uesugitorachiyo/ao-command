package cli

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

func (a App) missionArtifacts(args []string) int {
	var manifestPath string
	var contentRoot string
	var jsonOut bool
	fs := flag.NewFlagSet("mission artifacts", flag.ContinueOnError)
	fs.SetOutput(a.Stderr)
	fs.StringVar(&manifestPath, "manifest", "", "path to AO Mission artifact manifest JSON")
	fs.StringVar(&contentRoot, "content-root", "", "trusted AO Mission home containing retained artifacts")
	fs.BoolVar(&jsonOut, "json", false, "emit JSON")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if strings.TrimSpace(manifestPath) == "" {
		fmt.Fprintln(a.Stderr, "ao-command mission artifacts: --manifest is required")
		return 2
	}
	summary, err := readMissionArtifactManifest(manifestPath, contentRoot)
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
		if summary.Schema == "ao.mission.artifact-manifest.v0.2" {
			fmt.Fprintf(a.Stdout, "artifact_ref=%s content_ref=%s digest=%s\n", artifact.Ref, artifact.ContentRef, artifact.Digest)
		} else {
			fmt.Fprintf(a.Stdout, "artifact=%s:%s\n", artifact.Name, artifact.Path)
		}
	}
	fmt.Fprintf(a.Stdout, "exact_next_action=%s\n", summary.ExactNextAction)
	return 0
}

type missionArtifactRef struct {
	Name       string `json:"name,omitempty"`
	Path       string `json:"path,omitempty"`
	SHA256     string `json:"sha256,omitempty"`
	Schema     string `json:"schema,omitempty"`
	Ref        string `json:"ref,omitempty"`
	ContentRef string `json:"content_ref,omitempty"`
	Digest     string `json:"digest,omitempty"`
	Kind       string `json:"kind,omitempty"`
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

func readMissionArtifactManifest(path, contentRoot string) (missionArtifactsSummary, error) {
	body, err := os.ReadFile(path)
	if err != nil {
		return missionArtifactsSummary{}, err
	}
	if err := rejectDuplicateJSONKeys(body); err != nil {
		return missionArtifactsSummary{}, fmt.Errorf("invalid JSON: %w", err)
	}
	var envelope struct {
		Schema string `json:"schema"`
	}
	if err := json.Unmarshal(body, &envelope); err != nil {
		return missionArtifactsSummary{}, fmt.Errorf("invalid JSON: %w", err)
	}
	if envelope.Schema == "ao.mission.artifact-manifest.v0.2" {
		return readMissionArtifactManifestV02(body, contentRoot)
	}
	if envelope.Schema != "ao.mission.artifact-manifest.v0.1" {
		return missionArtifactsSummary{}, fmt.Errorf("schema must be ao.mission.artifact-manifest.v0.1 or ao.mission.artifact-manifest.v0.2")
	}
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
	if err := json.Unmarshal(body, &input); err != nil {
		return missionArtifactsSummary{}, err
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

type missionArtifactRefV02 struct {
	Schema     string `json:"schema"`
	Ref        string `json:"ref"`
	ContentRef string `json:"content_ref"`
	Digest     string `json:"digest"`
	Kind       string `json:"kind,omitempty"`
}

func readMissionArtifactManifestV02(body []byte, contentRoot string) (missionArtifactsSummary, error) {
	var input struct {
		Schema         string                  `json:"schema"`
		MissionID      string                  `json:"mission_id"`
		ArtifactRefs   []missionArtifactRefV02 `json:"artifact_refs"`
		ManifestDigest string                  `json:"manifest_digest"`
		Signature      string                  `json:"signature"`
		SafeToExecute  *bool                   `json:"safe_to_execute"`
		ExecutesWork   *bool                   `json:"executes_work"`
		ApprovesWork   *bool                   `json:"approves_work"`
		GeneratedAtUTC string                  `json:"generated_at_utc,omitempty"`
	}
	if err := decodeStrictJSON(body, &input); err != nil {
		return missionArtifactsSummary{}, fmt.Errorf("invalid v0.2 manifest: %w", err)
	}
	if strings.TrimSpace(input.MissionID) == "" || len(input.ArtifactRefs) == 0 || input.SafeToExecute == nil || input.ExecutesWork == nil || input.ApprovesWork == nil {
		return missionArtifactsSummary{}, fmt.Errorf("mission artifact manifest v0.2 requires mission_id, artifact_refs, and authority flags")
	}
	if *input.SafeToExecute || *input.ExecutesWork || *input.ApprovesWork {
		return missionArtifactsSummary{}, fmt.Errorf("mission artifact manifest must not claim execution or approval authority")
	}
	if !canonicalSHA256Digest(input.ManifestDigest) {
		return missionArtifactsSummary{}, fmt.Errorf("manifest_digest must be a canonical sha256 digest")
	}
	manifestBody, _ := json.Marshal(struct {
		Schema       string                  `json:"schema"`
		MissionID    string                  `json:"mission_id"`
		ArtifactRefs []missionArtifactRefV02 `json:"artifact_refs"`
	}{input.Schema, input.MissionID, input.ArtifactRefs})
	if digestBytesSHA256(manifestBody) != input.ManifestDigest {
		return missionArtifactsSummary{}, fmt.Errorf("artifact manifest digest mismatch")
	}
	if input.Signature != "ao-mission-local-digest:"+input.ManifestDigest {
		return missionArtifactsSummary{}, fmt.Errorf("artifact manifest signature does not bind manifest digest")
	}
	if strings.TrimSpace(contentRoot) == "" {
		return missionArtifactsSummary{}, fmt.Errorf("--content-root is required for ao.mission.artifact-manifest.v0.2")
	}

	refs := make(map[string]struct{}, len(input.ArtifactRefs))
	contents := make(map[string]struct{}, len(input.ArtifactRefs))
	artifacts := make([]missionArtifactRef, 0, len(input.ArtifactRefs))
	for _, ref := range input.ArtifactRefs {
		if ref.Schema != "ao.mission.artifact-ref.v0.1" || strings.TrimSpace(ref.Ref) == "" || !canonicalSHA256Digest(ref.Digest) {
			return missionArtifactsSummary{}, fmt.Errorf("artifact refs require schema, ref, content_ref, and canonical digest")
		}
		expectedContentRef := "artifacts/sha256/" + strings.TrimPrefix(ref.Digest, "sha256:")
		if ref.ContentRef != expectedContentRef {
			return missionArtifactsSummary{}, fmt.Errorf("artifact ref %s content_ref does not match digest", ref.Ref)
		}
		if _, exists := refs[ref.Ref]; exists {
			return missionArtifactsSummary{}, fmt.Errorf("duplicate artifact ref %s", ref.Ref)
		}
		if _, exists := contents[ref.ContentRef]; exists {
			return missionArtifactsSummary{}, fmt.Errorf("duplicate artifact content identity %s", ref.ContentRef)
		}
		refs[ref.Ref] = struct{}{}
		contents[ref.ContentRef] = struct{}{}
		if err := verifyMissionArtifactContent(contentRoot, ref); err != nil {
			return missionArtifactsSummary{}, err
		}
		artifacts = append(artifacts, missionArtifactRef{Schema: ref.Schema, Ref: ref.Ref, ContentRef: ref.ContentRef, Digest: ref.Digest, Kind: ref.Kind})
	}
	return missionArtifactsSummary{
		CommandSchemaVersion: commandSchemaVersion,
		Schema:               input.Schema,
		MissionID:            input.MissionID,
		Status:               "ready",
		OperatorMode:         operatorMode,
		ArtifactCount:        len(artifacts),
		Artifacts:            artifacts,
		SafeToExecute:        false,
		ExecutesWork:         false,
		ApprovesWork:         false,
		MutatesRepositories:  false,
	}, nil
}

func verifyMissionArtifactContent(contentRoot string, ref missionArtifactRefV02) error {
	root, err := filepath.Abs(contentRoot)
	if err != nil {
		return err
	}
	target := filepath.Join(root, filepath.FromSlash(ref.ContentRef))
	relative, err := filepath.Rel(root, target)
	if err != nil || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) || filepath.IsAbs(relative) {
		return fmt.Errorf("artifact content_ref escapes content root")
	}
	resolvedRoot, err := filepath.EvalSymlinks(root)
	if err != nil {
		return fmt.Errorf("resolve content root: %w", err)
	}
	resolvedTarget, err := filepath.EvalSymlinks(target)
	if err != nil {
		return fmt.Errorf("resolve retained artifact %s: %w", ref.Ref, err)
	}
	originalInfo, err := os.Lstat(target)
	if err != nil {
		return err
	}
	if !originalInfo.Mode().IsRegular() || originalInfo.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("retained artifact must be a regular non-symlink file")
	}
	resolvedRelative, err := filepath.Rel(resolvedRoot, resolvedTarget)
	if err != nil || resolvedRelative == ".." || strings.HasPrefix(resolvedRelative, ".."+string(filepath.Separator)) || filepath.IsAbs(resolvedRelative) {
		return fmt.Errorf("artifact content_ref resolves outside content root")
	}
	info, err := os.Lstat(resolvedTarget)
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("retained artifact must be a regular non-symlink file")
	}
	content, err := os.ReadFile(resolvedTarget)
	if err != nil {
		return err
	}
	if digestBytesSHA256(content) != ref.Digest {
		return fmt.Errorf("artifact digest mismatch for %s", ref.Ref)
	}
	return nil
}

func canonicalSHA256Digest(digest string) bool {
	hexDigest := strings.TrimPrefix(digest, "sha256:")
	if hexDigest == digest || len(hexDigest) != sha256.Size*2 || hexDigest != strings.ToLower(hexDigest) {
		return false
	}
	_, err := hex.DecodeString(hexDigest)
	return err == nil
}

func digestBytesSHA256(body []byte) string {
	sum := sha256.Sum256(body)
	return "sha256:" + hex.EncodeToString(sum[:])
}

func decodeStrictJSON(body []byte, target any) error {
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return err
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		if err == nil {
			return fmt.Errorf("multiple JSON values")
		}
		return err
	}
	return nil
}

func rejectDuplicateJSONKeys(body []byte) error {
	decoder := json.NewDecoder(bytes.NewReader(body))
	var walk func() error
	walk = func() error {
		token, err := decoder.Token()
		if err != nil {
			return err
		}
		delimiter, ok := token.(json.Delim)
		if !ok {
			return nil
		}
		switch delimiter {
		case '{':
			seen := map[string]struct{}{}
			for decoder.More() {
				keyToken, err := decoder.Token()
				if err != nil {
					return err
				}
				key := keyToken.(string)
				if _, exists := seen[key]; exists {
					return fmt.Errorf("duplicate object key %q", key)
				}
				seen[key] = struct{}{}
				if err := walk(); err != nil {
					return err
				}
			}
			_, err = decoder.Token()
			return err
		case '[':
			for decoder.More() {
				if err := walk(); err != nil {
					return err
				}
			}
			_, err = decoder.Token()
			return err
		default:
			return fmt.Errorf("unexpected JSON delimiter %q", delimiter)
		}
	}
	if err := walk(); err != nil {
		return err
	}
	if _, err := decoder.Token(); err != io.EOF {
		if err == nil {
			return fmt.Errorf("multiple JSON values")
		}
		return err
	}
	return nil
}
