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

## Install

Download the artifact for the target operating system from the exact hosted
run. Verify `SHA256SUMS` before extracting it. Install into a
user-controlled directory rather than replacing an existing binary in place.
Run:

```text
ao-mission version --json
ao-command version --json
```

The reported source commit must match the candidate inventory. A mismatch
stops the rehearsal.

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

Keep the currently installed binary and its verified checksum. Place the new
candidate beside it, verify its checksum and version output, then atomically
switch the user-controlled launcher or path entry. Re-run the operator status
fixture before removing the previous binary.

## Rollback

Restore the previous verified binary and checksum record. Re-run its version
command and the last compatible readback fixture. Rollback does not create a
tag, release, upload, deployment, or approval.

## Uninstall

Remove only the explicitly installed candidate binary, its user-controlled
launcher entry, and the candidate-specific checksum record. Do not remove
Mission records, AO2 evidence packs, Control Plane records, or unrelated
configuration.

These steps are candidate rehearsals. Public installation instructions remain
out of scope until a later release decision authorizes publication.
