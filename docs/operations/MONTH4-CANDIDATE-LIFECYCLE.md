# Mission and Command Candidate Lifecycle

The Month 2 Mission and AO Command artifacts are unreleased hosted candidates.
They are not public downloads and must not be represented as installed public
versions.

## Candidate Pair

| Component | Version | Source commit | Hosted run |
| --- | --- | --- | --- |
| AO Mission | `0.1.0` | `d10bc1986fe1ea5d9ac58454db4fffc08ab76bdd` | `29727037067` |
| AO Command | `0.1.0` | `822345d718b1c660530ac91343b494a6c463a81f` | `29728298780` |

Both runs produced Linux x86-64, macOS arm64, and Windows x86-64 candidate
artifacts. The immutable plans and independently downloaded artifacts passed
verification. No tag, release, public upload, or publication was attempted.

The AO Command Windows candidate is an x86-64 PE executable packaged in a zip.
`scripts/qualify-windows-candidate.ps1` qualifies that exact zip under both
Windows PowerShell 5.1 and PowerShell 7. Its reports are external evidence
bound to the unchanged archive digest.

Qualification rejects archives larger than 32 MiB, members larger than 16 MiB,
more than 32 MiB total expanded content, or a member compression ratio above
200:1. These exact limits are recorded in each qualification report. Extraction
uses bounded entry streams and creates each destination file without overwrite.

## Install

Download the artifact for the target operating system from the exact hosted
run. Verify `SHA256SUMS` before extracting it. Install into a
user-controlled directory rather than replacing an existing binary in place.
On Windows, installation means extracting the zip into that user-controlled
directory; it is not a system installer and does not modify `PATH`.
Run:

```text
ao-mission version --json
ao-command version --json
```

The reported source commit must match the candidate inventory. A mismatch
stops the rehearsal.

AO Command has no `doctor` interface. The replacement diagnostic is the exact
`version --json` identity plus provider-free `mission status` against the ready
Mission fixture. Qualification records doctor as `not_applicable` with
`replacement_diagnostic=version_and_mission_status`.

## Compatibility

Use Mission and Command from the same candidate pair. The supported Month 4
readback is:

```text
ao-command operator status --readback operator-status.json --json
```

The source contract is
`docs/contracts/operator-status-source-v0.1.schema.json`; the emitted contract
is `docs/contracts/operator-status-v0.1.schema.json`. AO Command rejects
unknown fields, unsupported status or release claims, unsafe authority flags,
and passed verification claims without evidence.

## Upgrade

Upgrade belongs to the separate stack Gate 4. It is not proven by the AO
Command component qualification report.

## Rollback

Rollback also belongs to the separate stack Gate 4 and is not proven by this
component report. Rollback does not create a tag, release, upload, deployment,
or approval.

## Uninstall

Remove only the user-controlled extracted candidate directory and its
candidate-specific checksum record. This removal is not a system uninstaller.
Do not remove
Mission records, AO2 evidence packs, Control Plane records, or unrelated
configuration.

These steps are candidate rehearsals. Public installation instructions remain
out of scope until a later release decision authorizes publication.
