package cli

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

type missionV02TestRef struct {
	Schema     string `json:"schema"`
	Ref        string `json:"ref"`
	ContentRef string `json:"content_ref"`
	Digest     string `json:"digest"`
	Kind       string `json:"kind,omitempty"`
}

type missionV02TestManifest struct {
	Schema         string              `json:"schema"`
	MissionID      string              `json:"mission_id"`
	ArtifactRefs   []missionV02TestRef `json:"artifact_refs"`
	ManifestDigest string              `json:"manifest_digest"`
	Signature      string              `json:"signature"`
	SafeToExecute  bool                `json:"safe_to_execute"`
	ExecutesWork   bool                `json:"executes_work"`
	ApprovesWork   bool                `json:"approves_work"`
	GeneratedAtUTC string              `json:"generated_at_utc,omitempty"`
}

func TestMissionArtifactsReadsMissionV02RetainedArtifact(t *testing.T) {
	manifestPath, contentRoot, ref := writeMissionV02Fixture(t, []byte("line one\r\nline two\r\n"))
	t.Setenv("PYTHONUTF8", "0")

	code, stdout, stderr := runWithFake([]string{"mission", "artifacts", "--manifest", manifestPath, "--content-root", contentRoot}, &fakeRunner{})
	if code != 0 {
		t.Fatalf("mission artifacts v0.2 exit=%d stderr=%s", code, stderr)
	}
	for _, want := range []string{
		"ao_command_mission_artifacts=ready",
		"mission_id=mission-v02",
		"artifact_count=1",
		"operator_mode=read_only",
		"artifact_ref=" + ref.Ref,
		"content_ref=" + ref.ContentRef,
		"digest=" + ref.Digest,
	} {
		if !strings.Contains(stdout, want) {
			t.Fatalf("mission artifacts v0.2 stdout missing %q:\n%s", want, stdout)
		}
	}

	code, stdout, stderr = runWithFake([]string{"mission", "artifacts", "--manifest", manifestPath, "--content-root", contentRoot, "--json"}, &fakeRunner{})
	if code != 0 {
		t.Fatalf("mission artifacts v0.2 json exit=%d stderr=%s", code, stderr)
	}
	var got struct {
		Schema        string `json:"schema"`
		OperatorMode  string `json:"operator_mode"`
		ArtifactCount int    `json:"artifact_count"`
		Artifacts     []struct {
			Ref        string `json:"ref"`
			ContentRef string `json:"content_ref"`
			Digest     string `json:"digest"`
		} `json:"artifacts"`
	}
	if err := json.Unmarshal([]byte(stdout), &got); err != nil {
		t.Fatalf("invalid mission artifacts v0.2 JSON: %v\n%s", err, stdout)
	}
	if got.Schema != "ao.mission.artifact-manifest.v0.2" || got.OperatorMode != operatorMode || got.ArtifactCount != 1 || len(got.Artifacts) != 1 || got.Artifacts[0].Ref != ref.Ref || got.Artifacts[0].ContentRef != ref.ContentRef || got.Artifacts[0].Digest != ref.Digest {
		t.Fatalf("unexpected mission artifacts v0.2 summary: %#v", got)
	}
}

func TestMissionArtifactsRejectsMissionV02IdentityMismatches(t *testing.T) {
	tests := map[string]func(*missionV02TestManifest){
		"content locator digest": func(manifest *missionV02TestManifest) {
			manifest.ArtifactRefs[0].ContentRef = "artifacts/sha256/" + strings.Repeat("b", 64)
			finalizeMissionV02TestManifest(manifest)
		},
		"manifest digest": func(manifest *missionV02TestManifest) {
			manifest.ManifestDigest = "sha256:" + strings.Repeat("c", 64)
			manifest.Signature = "ao-mission-local-digest:" + manifest.ManifestDigest
		},
		"signature": func(manifest *missionV02TestManifest) {
			manifest.Signature = "ao-mission-local-digest:sha256:" + strings.Repeat("d", 64)
		},
	}
	for name, mutate := range tests {
		t.Run(name, func(t *testing.T) {
			manifestPath, contentRoot, _ := writeMissionV02Fixture(t, []byte("retained evidence"))
			manifest := readMissionV02TestManifest(t, manifestPath)
			mutate(&manifest)
			writeMissionV02TestManifest(t, manifestPath, manifest)
			assertMissionV02Rejected(t, manifestPath, contentRoot)
		})
	}
}

func TestMissionArtifactsRejectsMissionV02RetainedByteDigestMismatch(t *testing.T) {
	manifestPath, contentRoot, ref := writeMissionV02Fixture(t, []byte("retained evidence"))
	contentPath := filepath.Join(contentRoot, filepath.FromSlash(ref.ContentRef))
	if err := os.WriteFile(contentPath, []byte("tampered evidence"), 0o600); err != nil {
		t.Fatal(err)
	}
	assertMissionV02Rejected(t, manifestPath, contentRoot)
}

func TestMissionArtifactsRejectsMalformedOrAmbiguousMissionV02(t *testing.T) {
	tests := map[string]func(*missionV02TestManifest){
		"missing ref":         func(manifest *missionV02TestManifest) { manifest.ArtifactRefs[0].Ref = "" },
		"missing content ref": func(manifest *missionV02TestManifest) { manifest.ArtifactRefs[0].ContentRef = "" },
		"missing digest":      func(manifest *missionV02TestManifest) { manifest.ArtifactRefs[0].Digest = "" },
		"wrong ref schema":    func(manifest *missionV02TestManifest) { manifest.ArtifactRefs[0].Schema = "ao.mission.artifact-ref.v9" },
		"noncanonical digest": func(manifest *missionV02TestManifest) {
			manifest.ArtifactRefs[0].Digest = "sha256:" + strings.Repeat("A", 64)
		},
		"empty refs": func(manifest *missionV02TestManifest) { manifest.ArtifactRefs = nil },
		"duplicate ref": func(manifest *missionV02TestManifest) {
			manifest.ArtifactRefs = append(manifest.ArtifactRefs, manifest.ArtifactRefs[0])
		},
		"duplicate content identity": func(manifest *missionV02TestManifest) {
			duplicate := manifest.ArtifactRefs[0]
			duplicate.Ref = "other-source.json"
			manifest.ArtifactRefs = append(manifest.ArtifactRefs, duplicate)
		},
	}
	for name, mutate := range tests {
		t.Run(name, func(t *testing.T) {
			manifestPath, contentRoot, _ := writeMissionV02Fixture(t, []byte("retained evidence"))
			manifest := readMissionV02TestManifest(t, manifestPath)
			mutate(&manifest)
			finalizeMissionV02TestManifest(&manifest)
			writeMissionV02TestManifest(t, manifestPath, manifest)
			assertMissionV02Rejected(t, manifestPath, contentRoot)
		})
	}
}

func TestMissionArtifactsRejectsNonStrictMissionV02JSON(t *testing.T) {
	tests := map[string]func(string) string{
		"unknown field": func(body string) string {
			return strings.Replace(body, "{", `{"unexpected":true,`, 1)
		},
		"duplicate key": func(body string) string {
			return strings.Replace(body, `"mission_id": "mission-v02",`, `"mission_id": "shadow", "mission_id": "mission-v02",`, 1)
		},
		"wrong authority type": func(body string) string {
			return strings.Replace(body, `"safe_to_execute": false`, `"safe_to_execute": "false"`, 1)
		},
	}
	for name, mutate := range tests {
		t.Run(name, func(t *testing.T) {
			manifestPath, contentRoot, _ := writeMissionV02Fixture(t, []byte("retained evidence"))
			body, err := os.ReadFile(manifestPath)
			if err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(manifestPath, []byte(mutate(string(body))), 0o600); err != nil {
				t.Fatal(err)
			}
			assertMissionV02Rejected(t, manifestPath, contentRoot)
		})
	}
}

func TestMissionArtifactsRejectsMissionV02PathEscapeAndMissingContentRoot(t *testing.T) {
	manifestPath, contentRoot, _ := writeMissionV02Fixture(t, []byte("retained evidence"))
	manifest := readMissionV02TestManifest(t, manifestPath)
	manifest.ArtifactRefs[0].ContentRef = "../outside"
	finalizeMissionV02TestManifest(&manifest)
	writeMissionV02TestManifest(t, manifestPath, manifest)
	assertMissionV02Rejected(t, manifestPath, contentRoot)

	manifestPath, _, _ = writeMissionV02Fixture(t, []byte("retained evidence"))
	assertMissionV02Rejected(t, manifestPath, "")
}

func writeMissionV02Fixture(t *testing.T, content []byte) (string, string, missionV02TestRef) {
	t.Helper()
	root := filepath.Join(t.TempDir(), "Mission Home With Spaces")
	digest := missionV02TestDigest(content)
	contentRef := filepath.ToSlash(filepath.Join("artifacts", "sha256", strings.TrimPrefix(digest, "sha256:")))
	contentPath := filepath.Join(root, filepath.FromSlash(contentRef))
	if err := os.MkdirAll(filepath.Dir(contentPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(contentPath, content, 0o600); err != nil {
		t.Fatal(err)
	}
	ref := missionV02TestRef{Schema: "ao.mission.artifact-ref.v0.1", Ref: "source evidence.json", ContentRef: contentRef, Digest: digest, Kind: "ao2_evidence"}
	manifest := missionV02TestManifest{Schema: "ao.mission.artifact-manifest.v0.2", MissionID: "mission-v02", ArtifactRefs: []missionV02TestRef{ref}, GeneratedAtUTC: "2026-08-20T00:00:00Z"}
	finalizeMissionV02TestManifest(&manifest)
	manifestPath := filepath.Join(root, "outputs", "artifact manifest.json")
	if err := os.MkdirAll(filepath.Dir(manifestPath), 0o755); err != nil {
		t.Fatal(err)
	}
	writeMissionV02TestManifest(t, manifestPath, manifest)
	return manifestPath, root, ref
}

func finalizeMissionV02TestManifest(manifest *missionV02TestManifest) {
	body, _ := json.Marshal(struct {
		Schema       string              `json:"schema"`
		MissionID    string              `json:"mission_id"`
		ArtifactRefs []missionV02TestRef `json:"artifact_refs"`
	}{manifest.Schema, manifest.MissionID, manifest.ArtifactRefs})
	manifest.ManifestDigest = missionV02TestDigest(body)
	manifest.Signature = "ao-mission-local-digest:" + manifest.ManifestDigest
}

func missionV02TestDigest(body []byte) string {
	sum := sha256.Sum256(body)
	return "sha256:" + hex.EncodeToString(sum[:])
}

func readMissionV02TestManifest(t *testing.T, path string) missionV02TestManifest {
	t.Helper()
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var manifest missionV02TestManifest
	if err := json.Unmarshal(body, &manifest); err != nil {
		t.Fatal(err)
	}
	return manifest
}

func writeMissionV02TestManifest(t *testing.T, path string, manifest missionV02TestManifest) {
	t.Helper()
	body, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, body, 0o600); err != nil {
		t.Fatal(err)
	}
}

func assertMissionV02Rejected(t *testing.T, manifestPath, contentRoot string) {
	t.Helper()
	args := []string{"mission", "artifacts", "--manifest", manifestPath}
	if contentRoot != "" {
		args = append(args, "--content-root", contentRoot)
	}
	code, _, stderr := runWithFake(args, &fakeRunner{})
	if code == 0 {
		t.Fatalf("invalid mission v0.2 manifest accepted; stderr=%s", stderr)
	}
}
