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

var beforeMissionArtifactContentOpen = func(string) error { return nil }

const missionArtifactManifestLimit = int64(1 << 20)

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
	body, err := readBoundedMissionArtifactManifest(path)
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
		Schema         string                   `json:"schema"`
		MissionID      string                   `json:"mission_id"`
		ArtifactRefs   *[]missionArtifactRefV02 `json:"artifact_refs"`
		ManifestDigest string                   `json:"manifest_digest"`
		Signature      string                   `json:"signature"`
		SafeToExecute  *bool                    `json:"safe_to_execute"`
		ExecutesWork   *bool                    `json:"executes_work"`
		ApprovesWork   *bool                    `json:"approves_work"`
		GeneratedAtUTC string                   `json:"generated_at_utc,omitempty"`
	}
	if err := validateMissionArtifactManifestV02Fields(body); err != nil {
		return missionArtifactsSummary{}, fmt.Errorf("invalid v0.2 manifest: %w", err)
	}
	if err := decodeStrictJSON(body, &input); err != nil {
		return missionArtifactsSummary{}, fmt.Errorf("invalid v0.2 manifest: %w", err)
	}
	if strings.TrimSpace(input.MissionID) == "" || input.ArtifactRefs == nil || input.SafeToExecute == nil || input.ExecutesWork == nil || input.ApprovesWork == nil {
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
	}{input.Schema, input.MissionID, *input.ArtifactRefs})
	if digestBytesSHA256(manifestBody) != input.ManifestDigest {
		return missionArtifactsSummary{}, fmt.Errorf("artifact manifest digest mismatch")
	}
	if input.Signature != "ao-mission-local-digest:"+input.ManifestDigest {
		return missionArtifactsSummary{}, fmt.Errorf("artifact manifest signature does not bind manifest digest")
	}
	if strings.TrimSpace(contentRoot) == "" {
		return missionArtifactsSummary{}, fmt.Errorf("--content-root is required for ao.mission.artifact-manifest.v0.2")
	}

	refs := make(map[string]struct{}, len(*input.ArtifactRefs))
	contents := make(map[string]struct{}, len(*input.ArtifactRefs))
	artifacts := make([]missionArtifactRef, 0, len(*input.ArtifactRefs))
	for _, ref := range *input.ArtifactRefs {
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
	rootHandle, err := os.OpenRoot(root)
	if err != nil {
		return fmt.Errorf("open content root: %w", err)
	}
	defer rootHandle.Close()
	before, err := rootHandle.Lstat(ref.ContentRef)
	if err != nil {
		return fmt.Errorf("inspect retained artifact %s: %w", ref.Ref, err)
	}
	if !before.Mode().IsRegular() || before.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("retained artifact must be a regular non-symlink file")
	}
	target := filepath.Join(root, filepath.FromSlash(ref.ContentRef))
	if err := beforeMissionArtifactContentOpen(target); err != nil {
		return fmt.Errorf("before retained artifact open: %w", err)
	}
	file, err := rootHandle.Open(ref.ContentRef)
	if err != nil {
		return fmt.Errorf("open retained artifact %s: %w", ref.Ref, err)
	}
	opened, err := file.Stat()
	if err != nil {
		_ = file.Close()
		return fmt.Errorf("stat retained artifact %s: %w", ref.Ref, err)
	}
	if !opened.Mode().IsRegular() || !os.SameFile(before, opened) {
		_ = file.Close()
		return fmt.Errorf("retained artifact changed while opening")
	}
	digest, readErr := digestMissionArtifact(file)
	afterHandle, statErr := file.Stat()
	closeErr := file.Close()
	if readErr != nil {
		return fmt.Errorf("hash retained artifact %s: %w", ref.Ref, readErr)
	}
	if statErr != nil {
		return fmt.Errorf("reinspect opened retained artifact %s: %w", ref.Ref, statErr)
	}
	if closeErr != nil {
		return fmt.Errorf("close retained artifact %s: %w", ref.Ref, closeErr)
	}
	after, err := rootHandle.Lstat(ref.ContentRef)
	if err != nil {
		return fmt.Errorf("reinspect retained artifact %s: %w", ref.Ref, err)
	}
	if !after.Mode().IsRegular() || after.Mode()&os.ModeSymlink != 0 || !os.SameFile(opened, afterHandle) || !os.SameFile(opened, after) {
		return fmt.Errorf("retained artifact changed while reading")
	}
	if digest != ref.Digest {
		return fmt.Errorf("artifact digest mismatch for %s", ref.Ref)
	}
	return nil
}

func readBoundedMissionArtifactManifest(path string) ([]byte, error) {
	before, err := os.Lstat(path)
	if err != nil {
		return nil, err
	}
	if !before.Mode().IsRegular() || before.Mode()&os.ModeSymlink != 0 {
		return nil, fmt.Errorf("artifact manifest must be a regular non-symlink file")
	}
	if before.Size() > missionArtifactManifestLimit {
		return nil, fmt.Errorf("artifact manifest exceeds %d bytes", missionArtifactManifestLimit)
	}
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	opened, err := file.Stat()
	if err != nil {
		_ = file.Close()
		return nil, err
	}
	if !opened.Mode().IsRegular() || opened.Size() > missionArtifactManifestLimit || !os.SameFile(before, opened) {
		_ = file.Close()
		return nil, fmt.Errorf("artifact manifest changed while opening or exceeds %d bytes", missionArtifactManifestLimit)
	}
	body, readErr := io.ReadAll(io.LimitReader(file, missionArtifactManifestLimit+1))
	afterHandle, statErr := file.Stat()
	closeErr := file.Close()
	if readErr != nil {
		return nil, readErr
	}
	if statErr != nil {
		return nil, statErr
	}
	if closeErr != nil {
		return nil, closeErr
	}
	if int64(len(body)) > missionArtifactManifestLimit {
		return nil, fmt.Errorf("artifact manifest exceeds %d bytes", missionArtifactManifestLimit)
	}
	after, err := os.Lstat(path)
	if err != nil {
		return nil, err
	}
	if !after.Mode().IsRegular() || after.Mode()&os.ModeSymlink != 0 || !os.SameFile(opened, afterHandle) || !os.SameFile(opened, after) {
		return nil, fmt.Errorf("artifact manifest changed while reading")
	}
	return body, nil
}

func validateMissionArtifactManifestV02Fields(body []byte) error {
	top, err := validateExactMissionArtifactObject("artifact manifest v0.2", body,
		[]string{"schema", "mission_id", "artifact_refs", "manifest_digest", "signature", "safe_to_execute", "executes_work", "approves_work", "generated_at_utc"},
		[]string{"schema", "mission_id", "artifact_refs", "manifest_digest", "signature", "safe_to_execute", "executes_work", "approves_work"})
	if err != nil {
		return err
	}
	rawRefs := bytes.TrimSpace(top["artifact_refs"])
	if bytes.Equal(rawRefs, []byte("null")) {
		return fmt.Errorf("artifact manifest v0.2 field %q must be an array", "artifact_refs")
	}
	var refs []json.RawMessage
	if err := json.Unmarshal(rawRefs, &refs); err != nil {
		return fmt.Errorf("artifact manifest v0.2 field %q must be an array: %w", "artifact_refs", err)
	}
	for index, rawRef := range refs {
		if _, err := validateExactMissionArtifactObject(fmt.Sprintf("artifact manifest v0.2 artifact ref %d", index), rawRef,
			[]string{"schema", "ref", "content_ref", "digest", "kind"},
			[]string{"schema", "ref", "content_ref", "digest"}); err != nil {
			return err
		}
	}
	return nil
}

func validateExactMissionArtifactObject(label string, body []byte, allowed, required []string) (map[string]json.RawMessage, error) {
	var object map[string]json.RawMessage
	if err := json.Unmarshal(body, &object); err != nil {
		return nil, fmt.Errorf("%s must be a JSON object: %w", label, err)
	}
	if object == nil || bytes.Equal(bytes.TrimSpace(body), []byte("null")) {
		return nil, fmt.Errorf("%s must be a JSON object", label)
	}
	allowedFields := make(map[string]struct{}, len(allowed))
	for _, field := range allowed {
		allowedFields[field] = struct{}{}
	}
	for field := range object {
		if _, ok := allowedFields[field]; !ok {
			return nil, fmt.Errorf("%s has unknown field %q", label, field)
		}
	}
	for _, field := range required {
		if _, ok := object[field]; !ok {
			return nil, fmt.Errorf("%s requires field %q", label, field)
		}
	}
	return object, nil
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

func digestMissionArtifact(reader io.Reader) (string, error) {
	hash := sha256.New()
	if _, err := io.Copy(hash, reader); err != nil {
		return "", err
	}
	return "sha256:" + hex.EncodeToString(hash.Sum(nil)), nil
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
