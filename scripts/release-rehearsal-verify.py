#!/usr/bin/env python3

import hashlib
import json
import os
import re
import stat
import struct
import sys
import tarfile
import zipfile
from pathlib import Path, PurePosixPath


MANIFEST_SCHEMA = "ao.command.approved-release-manifest.v0.1"
CANDIDATE_SCHEMA = "ao.command.release-rehearsal-candidate.v0.3"
PROVENANCE_SCHEMA = "ao.command.release-rehearsal-provenance.v0.1"
PLAN_SCHEMA = "ao.command.release-rehearsal-plan.v0.3"
EXPECTED_TARGETS = {
    "linux-x86_64": {
        "archive_extension": "tar.gz",
        "executable": "ao-command",
        "executable_format": "elf",
        "goarch": "amd64",
        "goos": "linux",
        "runner_arch": "X64",
    },
    "macos-aarch64": {
        "archive_extension": "tar.gz",
        "executable": "ao-command",
        "executable_format": "macho",
        "goarch": "arm64",
        "goos": "darwin",
        "runner_arch": "ARM64",
    },
    "windows-x86_64": {
        "archive_extension": "zip",
        "executable": "ao-command.exe",
        "executable_format": "pe",
        "goarch": "amd64",
        "goos": "windows",
        "runner_arch": "X64",
    },
}
EVIDENCE_FILES = {
    "LICENSE",
    "functional-smoke.json",
    "help-smoke.txt",
    "provenance.json",
    "sbom.json",
    "version-readback.json",
}
MAX_MANIFEST_BYTES = 64 * 1024
MAX_ARCHIVE_MEMBER_BYTES = 256 * 1024 * 1024
MAX_ARCHIVE_TOTAL_BYTES = 512 * 1024 * 1024


def fail(message):
    raise SystemExit(message)


def sha256_bytes(data):
    return hashlib.sha256(data).hexdigest()


def require_digest(value, label):
    if not isinstance(value, str) or re.fullmatch(r"[0-9a-f]{64}", value) is None:
        fail(f"{label} malformed")
    return value


def load_json_bytes(path, label):
    try:
        data = path.read_bytes()
        return data, json.loads(data)
    except (OSError, UnicodeError, json.JSONDecodeError) as error:
        fail(f"{label} malformed: {error}")


def expected_bindings():
    return {
        "version": os.environ["VERSION"],
        "tag": os.environ["TAG"],
        "source_commit": os.environ["SOURCE_COMMIT"],
        "release_notes_digest": os.environ["RELEASE_NOTES_DIGEST"],
    }


def validate_manifest():
    path = Path("approved-manifest/approved-release-manifest.json")
    try:
        manifest_bytes = path.read_bytes()
    except OSError as error:
        fail(f"approved manifest malformed: {error}")
    if len(manifest_bytes) > MAX_MANIFEST_BYTES:
        fail("approved manifest exceeds bounded size")
    try:
        manifest = json.loads(manifest_bytes)
    except (UnicodeError, json.JSONDecodeError) as error:
        fail(f"approved manifest malformed: {error}")
    manifest_digest = sha256_bytes(manifest_bytes)
    if manifest_digest != os.environ["APPROVED_MANIFEST_DIGEST"]:
        fail("approved manifest digest mismatch")
    expected_keys = {
        "candidates",
        "immutable",
        "release_notes_digest",
        "repository",
        "schema_version",
        "source_commit",
        "tag",
        "version",
    }
    if not isinstance(manifest, dict) or set(manifest) != expected_keys:
        fail("approved manifest schema mismatch")
    if manifest["schema_version"] != MANIFEST_SCHEMA:
        fail("approved manifest schema mismatch")
    if manifest["immutable"] is not True or manifest["repository"] != "ao-command":
        fail("approved manifest identity mismatch")
    for key, expected in expected_bindings().items():
        if manifest[key] != expected:
            fail(f"approved manifest {key} mismatch")
    candidates = manifest["candidates"]
    if not isinstance(candidates, list) or len(candidates) != len(EXPECTED_TARGETS):
        fail("approved manifest candidate inventory mismatch")
    by_target = {}
    for candidate in candidates:
        if not isinstance(candidate, dict) or set(candidate) != {"archive", "archive_sha256", "target"}:
            fail("approved manifest candidate schema mismatch")
        target = candidate["target"]
        if target not in EXPECTED_TARGETS or target in by_target:
            fail("approved manifest candidate inventory mismatch")
        identity = EXPECTED_TARGETS[target]
        expected_archive = (
            f"ao-command-{os.environ['VERSION']}-{target}."
            f"{identity['archive_extension']}"
        )
        if candidate["archive"] != expected_archive:
            fail("approved manifest candidate inventory mismatch")
        require_digest(candidate["archive_sha256"], "approved manifest candidate digest")
        by_target[target] = candidate
    if set(by_target) != set(EXPECTED_TARGETS):
        fail("approved manifest candidate inventory mismatch")
    return manifest_digest, by_target


def safe_member_name(name):
    path = PurePosixPath(name)
    return bool(name) and not path.is_absolute() and ".." not in path.parts and str(path) == name


def read_archive(path, target):
    identity = EXPECTED_TARGETS[target]
    expected_members = EVIDENCE_FILES | {identity["executable"]}
    contents = {}
    total_size = 0
    try:
        if identity["archive_extension"] == "zip":
            with zipfile.ZipFile(path) as package:
                members = package.infolist()
                names = [member.filename for member in members]
                if len(names) != len(set(names)):
                    fail("archive contains duplicate members")
                for member in members:
                    mode = member.external_attr >> 16
                    if (
                        member.is_dir()
                        or stat.S_ISLNK(mode)
                        or not safe_member_name(member.filename)
                        or member.file_size > MAX_ARCHIVE_MEMBER_BYTES
                    ):
                        fail("archive contains unsafe member")
                    total_size += member.file_size
                    if total_size > MAX_ARCHIVE_TOTAL_BYTES:
                        fail("archive exceeds bounded size")
                    contents[member.filename] = package.read(member)
        else:
            with tarfile.open(path, mode="r:gz") as package:
                members = package.getmembers()
                names = [member.name for member in members]
                if len(names) != len(set(names)):
                    fail("archive contains duplicate members")
                for member in members:
                    if (
                        not member.isfile()
                        or not safe_member_name(member.name)
                        or member.size > MAX_ARCHIVE_MEMBER_BYTES
                    ):
                        fail("archive contains unsafe member")
                    total_size += member.size
                    if total_size > MAX_ARCHIVE_TOTAL_BYTES:
                        fail("archive exceeds bounded size")
                    extracted = package.extractfile(member)
                    if extracted is None:
                        fail("archive member cannot be read")
                    contents[member.name] = extracted.read()
    except (OSError, tarfile.TarError, zipfile.BadZipFile, RuntimeError) as error:
        fail(f"archive malformed: {error}")
    if set(contents) != expected_members:
        fail("archive exact inventory mismatch")
    return contents


def executable_identity(data):
    if (
        len(data) >= 20
        and data[:4] == b"\x7fELF"
        and data[4] == 2
        and data[5] == 1
        and struct.unpack_from("<H", data, 18)[0] == 62
    ):
        return "elf", "linux", "amd64"
    if (
        len(data) >= 8
        and data[:4] == b"\xcf\xfa\xed\xfe"
        and struct.unpack_from("<I", data, 4)[0] == 0x0100000C
    ):
        return "macho", "darwin", "arm64"
    if len(data) >= 0x46 and data[:2] == b"MZ":
        pe_offset = struct.unpack_from("<I", data, 0x3C)[0]
        if (
            pe_offset + 6 <= len(data)
            and data[pe_offset : pe_offset + 4] == b"PE\x00\x00"
            and struct.unpack_from("<H", data, pe_offset + 4)[0] == 0x8664
        ):
            return "pe", "windows", "amd64"
    return "unknown", "unknown", "unknown"


def validate_evidence(root, candidate, archive_contents):
    bindings = expected_bindings()
    target = candidate["target"]
    identity = EXPECTED_TARGETS[target]

    _, version = load_json_bytes(root / "version-readback.json", "version readback")
    if version != {
        "provider_calls": False,
        "schema_version": "ao.command.version.v0.1",
        "source_commit": bindings["source_commit"],
        "version": bindings["version"],
    }:
        fail("version readback mismatch")

    _, functional = load_json_bytes(root / "functional-smoke.json", "functional smoke")
    if (
        functional.get("schema_version")
        != "ao.command.release-rehearsal-functional-smoke.v0.1"
        or functional.get("status") != "passed"
        or functional.get("provider_calls") is not False
    ):
        fail("functional smoke provider boundary mismatch")
    readback = functional.get("readback")
    if not isinstance(readback, dict) or (
        readback.get("command_schema_version") != "ao.command.v0.1"
        or readback.get("status") != "ready"
        or readback.get("operator_mode") != "read_only"
        or readback.get("safe_to_execute") is not False
    ):
        fail("functional smoke readback mismatch")

    _, provenance = load_json_bytes(root / "provenance.json", "candidate provenance")
    expected_provenance_keys = {
        "executable_format",
        "goarch",
        "goos",
        "provider_calls",
        "release_notes_digest",
        "repository",
        "runner_arch",
        "schema_version",
        "source_commit",
        "target",
        "version",
        "workflow_identity",
    }
    if not isinstance(provenance, dict) or set(provenance) != expected_provenance_keys:
        fail("candidate provenance schema mismatch")
    if (
        provenance["schema_version"] != PROVENANCE_SCHEMA
        or provenance["repository"] != "ao-command"
        or provenance["provider_calls"] is not False
        or provenance["workflow_identity"]
        != f".github/workflows/release-rehearsal.yml@{bindings['source_commit']}"
    ):
        fail("candidate provenance identity mismatch")
    for key, expected in bindings.items():
        if key == "tag":
            continue
        if provenance[key] != expected:
            fail(f"candidate provenance {key} mismatch")
    for key in ("executable_format", "goarch", "goos", "runner_arch"):
        if provenance[key] != identity[key]:
            fail("candidate provenance target identity mismatch")
    if provenance["target"] != target:
        fail("candidate provenance target identity mismatch")

    for name in EVIDENCE_FILES:
        if archive_contents[name] != (root / name).read_bytes():
            fail(f"archive evidence mismatch for {name}")
    actual_executable_identity = executable_identity(
        archive_contents[identity["executable"]]
    )
    expected_executable_identity = (
        identity["executable_format"],
        identity["goos"],
        identity["goarch"],
    )
    if actual_executable_identity != expected_executable_identity:
        fail("archive executable format or architecture mismatch")


def collect_candidates(manifest_by_target):
    root = Path("downloaded-candidates")
    summaries = sorted(root.rglob("candidate-summary.json"))
    if len(summaries) != len(EXPECTED_TARGETS):
        fail(
            "unexpected artifact inventory: expected "
            f"{len(EXPECTED_TARGETS)} summaries, found {len(summaries)}"
        )
    bindings = expected_bindings()
    candidates = []
    seen_targets = set()
    for summary_path in summaries:
        _, candidate = load_json_bytes(summary_path, "candidate summary")
        expected_candidate_keys = {
            "approved_manifest_digest",
            "archive",
            "archive_sha256",
            "executable",
            "executable_format",
            "goarch",
            "goos",
            "inventory",
            "provider_calls",
            "release_notes_digest",
            "repository",
            "runner_arch",
            "schema_version",
            "smoke",
            "source_commit",
            "tag",
            "target",
            "version",
        }
        if not isinstance(candidate, dict) or set(candidate) != expected_candidate_keys:
            fail("candidate schema mismatch")
        if candidate["schema_version"] != CANDIDATE_SCHEMA:
            fail("candidate schema mismatch")
        target = candidate["target"]
        if target not in EXPECTED_TARGETS or target in seen_targets:
            fail("unexpected artifact inventory: missing, duplicate, or substituted target")
        seen_targets.add(target)
        identity = EXPECTED_TARGETS[target]
        for key, expected in bindings.items():
            if candidate[key] != expected:
                fail(f"{key} drift in {summary_path}")
        if (
            candidate["repository"] != "ao-command"
            or candidate["approved_manifest_digest"]
            != os.environ["APPROVED_MANIFEST_DIGEST"]
            or candidate["provider_calls"] is not False
            or candidate["smoke"]
            != {
                "functional": "passed",
                "help": "passed",
                "provider_calls": False,
                "version": "passed",
            }
        ):
            fail("candidate bindings or smoke mismatch")
        for key in (
            "executable",
            "executable_format",
            "goarch",
            "goos",
            "runner_arch",
        ):
            if candidate[key] != identity[key]:
                fail("candidate target identity mismatch")

        archive = candidate["archive"]
        manifest_candidate = manifest_by_target[target]
        if {
            "archive": archive,
            "archive_sha256": candidate["archive_sha256"],
            "target": target,
        } != manifest_candidate:
            fail("approved manifest candidate mismatch")
        require_digest(candidate["archive_sha256"], "candidate archive digest")
        inventory = candidate["inventory"]
        if not isinstance(inventory, list):
            fail("candidate inventory malformed")
        inventory_by_name = {}
        for item in inventory:
            if (
                not isinstance(item, dict)
                or set(item) != {"name", "sha256"}
                or item["name"] in inventory_by_name
            ):
                fail("candidate inventory malformed")
            inventory_by_name[item["name"]] = require_digest(
                item["sha256"], "candidate inventory digest"
            )
        expected_files = EVIDENCE_FILES | {"SHA256SUMS", archive}
        if set(inventory_by_name) != expected_files:
            fail("candidate inventory mismatch")
        actual_files = {
            path.name for path in summary_path.parent.iterdir() if path.is_file()
        } - {"candidate-summary.json"}
        if actual_files != expected_files:
            fail("unexpected artifact inventory")
        for name, expected_digest in inventory_by_name.items():
            actual_digest = sha256_bytes((summary_path.parent / name).read_bytes())
            if actual_digest != expected_digest:
                if name == archive:
                    fail("candidate archive checksum mismatch")
                fail("candidate inventory mismatch")
        checksum = (summary_path.parent / "SHA256SUMS").read_text(encoding="utf-8")
        expected_checksum = f"{candidate['archive_sha256']}  {archive}\n"
        if checksum != expected_checksum:
            fail("SHA256SUMS exact filename or digest mismatch")
        if inventory_by_name[archive] != candidate["archive_sha256"]:
            fail("candidate archive checksum mismatch")
        archive_contents = read_archive(summary_path.parent / archive, target)
        validate_evidence(summary_path.parent, candidate, archive_contents)
        candidates.append(candidate)
    if seen_targets != set(EXPECTED_TARGETS):
        fail("unexpected artifact inventory: missing target")
    return sorted(candidates, key=lambda candidate: candidate["target"])


def plan_document(manifest_digest, candidates):
    bindings = expected_bindings()
    return {
        "approved_manifest_digest": manifest_digest,
        "candidates": candidates,
        "immutable": True,
        "release_notes_digest": bindings["release_notes_digest"],
        "repository": "ao-command",
        "schema_version": PLAN_SCHEMA,
        "source_commit": bindings["source_commit"],
        "tag": bindings["tag"],
        "version": bindings["version"],
    }


def assemble():
    manifest_digest, manifest_by_target = validate_manifest()
    candidates = collect_candidates(manifest_by_target)
    output = Path("target/release-rehearsal-plan")
    output.mkdir(parents=True, exist_ok=True)
    plan = plan_document(manifest_digest, candidates)
    plan_path = output / "release-rehearsal-plan.json"
    plan_path.write_text(
        json.dumps(plan, indent=2, sort_keys=True) + "\n", encoding="utf-8"
    )
    plan_digest = sha256_bytes(plan_path.read_bytes())
    expected_plan_digest = os.environ.get("EXPECTED_PLAN_DIGEST", "")
    if expected_plan_digest and plan_digest != expected_plan_digest:
        fail("assembled plan digest does not match expected plan digest")
    (output / "release-rehearsal-plan.sha256").write_text(
        f"{plan_digest}  release-rehearsal-plan.json\n", encoding="utf-8"
    )
    boundary = {
        "dry_run": os.environ["DRY_RUN"].lower() == "true",
        "publication_performed": False,
        "public_upload_attempted": False,
        "release_creation_attempted": False,
        "schema_version": "ao.command.release-rehearsal-dry-run-boundary.v0.1",
        "source_commit": os.environ["SOURCE_COMMIT"],
        "tag_creation_attempted": False,
    }
    (output / "dry-run-boundary.json").write_text(
        json.dumps(boundary, indent=2, sort_keys=True) + "\n", encoding="utf-8"
    )


def verify():
    plan_path = Path("target/release-rehearsal-plan/release-rehearsal-plan.json")
    try:
        plan_bytes = plan_path.read_bytes()
    except OSError as error:
        fail(f"plan malformed: {error}")
    plan_digest = sha256_bytes(plan_bytes)
    if plan_digest != os.environ["EXPECTED_PLAN_DIGEST"]:
        fail("plan digest mismatch")
    try:
        checksum = Path(
            "target/release-rehearsal-plan/release-rehearsal-plan.sha256"
        ).read_text(encoding="utf-8")
    except (OSError, UnicodeError) as error:
        fail(f"plan checksum file malformed: {error}")
    if checksum != f"{plan_digest}  release-rehearsal-plan.json\n":
        fail("plan checksum file mismatch")
    expected_confirmation = (
        "publish-ao-command-{version}-{tag}-{source}-{manifest}-{notes}-{plan}".format(
            version=os.environ["VERSION"],
            tag=os.environ["TAG"],
            source=os.environ["SOURCE_COMMIT"],
            manifest=os.environ["APPROVED_MANIFEST_DIGEST"],
            notes=os.environ["RELEASE_NOTES_DIGEST"],
            plan=plan_digest,
        )
    )
    if os.environ.get("EXACT_CONFIRMATION") != expected_confirmation:
        fail("exact confirmation mismatch")
    try:
        plan = json.loads(plan_bytes)
    except (UnicodeError, json.JSONDecodeError) as error:
        fail(f"plan malformed: {error}")
    expected_plan_keys = {
        "approved_manifest_digest",
        "candidates",
        "immutable",
        "release_notes_digest",
        "repository",
        "schema_version",
        "source_commit",
        "tag",
        "version",
    }
    if not isinstance(plan, dict) or set(plan) != expected_plan_keys:
        fail("plan schema mismatch")
    if plan["schema_version"] != PLAN_SCHEMA:
        fail("plan schema mismatch")
    if plan["immutable"] is not True:
        fail("plan must be immutable")
    if plan["repository"] != "ao-command":
        fail("plan repository mismatch")
    for key, expected in expected_bindings().items():
        if plan[key] != expected:
            fail(f"plan {key} mismatch")
    if plan["approved_manifest_digest"] != os.environ["APPROVED_MANIFEST_DIGEST"]:
        fail("plan approved_manifest_digest mismatch")
    candidates = plan["candidates"]
    if not isinstance(candidates, list) or len(candidates) != len(EXPECTED_TARGETS):
        fail("plan candidate inventory mismatch")
    targets = [
        candidate.get("target") for candidate in candidates if isinstance(candidate, dict)
    ]
    if set(targets) != set(EXPECTED_TARGETS) or len(targets) != len(set(targets)):
        fail("plan candidate inventory mismatch")
    manifest_digest, manifest_by_target = validate_manifest()
    verified_candidates = collect_candidates(manifest_by_target)
    if plan != plan_document(manifest_digest, verified_candidates):
        fail("publisher candidate summary mismatch")


def main():
    if len(sys.argv) != 2 or sys.argv[1] not in {"manifest", "assemble", "verify"}:
        fail("usage: release-rehearsal-verify.py manifest|assemble|verify")
    if sys.argv[1] == "manifest":
        validate_manifest()
    elif sys.argv[1] == "assemble":
        assemble()
    else:
        verify()


if __name__ == "__main__":
    main()
