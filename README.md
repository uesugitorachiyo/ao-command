# AO Command

AO Command is the read-only operator interface for AO. It turns Mission,
Atlas, Foundry, Forge, Covenant, AO2, and Control Plane records into terminal
or JSON views without becoming another source of domain truth.

## Role In AO

- **Inputs:** Mission lifecycle state, portfolio status, GoalRun evidence,
  policy decisions, AO2 evidence, and Control Plane readbacks.
- **Outputs:** Status, timelines, next actions, validation summaries, and JSON
  readbacks.
- **Upstream:** AO Mission, AO Atlas, AO Foundry, AO Forge, AO Covenant, AO2,
  and AO2 Control Plane.
- **Downstream:** Operators and read-only automation.

See the [AO Architecture guide](https://github.com/uesugitorachiyo/ao-architecture)
and the
[AO Command component page](https://github.com/uesugitorachiyo/ao-architecture/blob/main/components/ao-command.md)
for the cross-repository flow.

## Common Commands

```sh
go run ./cmd/ao-command status --forge ../ao-forge
go run ./cmd/ao-command mission status \
  --status examples/mission/command-status.ready.json
go run ./cmd/ao-command mission next \
  --decision examples/mission/route-decision.ready.json
go run ./cmd/ao-command stack \
  --ledger ../ao-foundry/examples/readiness/active-stack-readiness.ledger.json
go run ./cmd/ao-command atlas status \
  --status ../ao-foundry/examples/contract-fixtures/valid/foundry-atlas-status-v0.1.json
go run ./cmd/ao-command pulse status \
  --preflight ../ao-foundry/examples/pulse-overnight-start-gate/ready.intake-preflight.json \
  --lifecycle ../ao-foundry/examples/pulse-lifecycle/ready-to-start-next-slice.json \
  --start-gate ../ao-foundry/examples/pulse-overnight-start-gate/ready.json
```

Use `--json` when a command supports machine-readable output. The
[full command reference](REFERENCE.md) documents every status, evidence,
rehearsal, release, and live-mutation readback.

### Windows Mission JSON handoff

`ao-command mission status` accepts strict UTF-8 JSON without a byte-order mark
(BOM). Windows PowerShell 5.1 `Set-Content -Encoding UTF8` adds a BOM, so write
Mission’s JSON with a BOM-free encoder:

```powershell
$status = & ao-mission command status --mission <mission-id> --json
[System.IO.File]::WriteAllText('mission-command-status.json', $status, [System.Text.UTF8Encoding]::new($false))
.\ao-command.exe mission status --status mission-command-status.json
```

## Install v0.1.3

The current published release is [v0.1.3](https://github.com/uesugitorachiyo/ao-command/releases/tag/v0.1.3).
After extracting an archive, run `./ao-command --help` on macOS or Linux, or
`.\ao-command.exe --help` in PowerShell.

- macOS Apple Silicon: [`ao-command-0.1.3-macos-aarch64.tar.gz`](https://github.com/uesugitorachiyo/ao-command/releases/download/v0.1.3/ao-command-0.1.3-macos-aarch64.tar.gz)
- Linux x86_64: [`ao-command-0.1.3-linux-x86_64.tar.gz`](https://github.com/uesugitorachiyo/ao-command/releases/download/v0.1.3/ao-command-0.1.3-linux-x86_64.tar.gz)
- Windows x86_64: [`ao-command-0.1.3-windows-x86_64.zip`](https://github.com/uesugitorachiyo/ao-command/releases/download/v0.1.3/ao-command-0.1.3-windows-x86_64.zip)

The release has no separate checksum asset. Verify a downloaded archive against
the GitHub asset digest before extracting it:

```sh
archive=ao-command-0.1.3-macos-aarch64.tar.gz
expected="$(gh release view v0.1.3 --repo uesugitorachiyo/ao-command --json assets --jq ".assets[] | select(.name == \"$archive\") | .digest")"
if command -v sha256sum >/dev/null 2>&1; then
  actual="sha256:$(sha256sum "$archive" | awk '{print $1}')"
else
  actual="sha256:$(shasum -a 256 "$archive" | awk '{print $1}')"
fi
test "$actual" = "$expected"
```

On Linux x86_64, set
`archive=ao-command-0.1.3-linux-x86_64.tar.gz` before running the same block.

On PowerShell:

```powershell
$archive = 'ao-command-0.1.3-windows-x86_64.zip'
$expected = gh release view v0.1.3 --repo uesugitorachiyo/ao-command --json assets --jq '.assets[] | select(.name == "ao-command-0.1.3-windows-x86_64.zip") | .digest'
$actual = "sha256:$((Get-FileHash $archive -Algorithm SHA256).Hash.ToLower())"
if ($actual -ne $expected) { throw 'release digest mismatch' }
```

For development, Go 1.26 or later is required; source builds remain the normal
path: `go run ./cmd/ao-command ...`.

## Safety Boundary

AO Command reads records owned by other components. It does not start loops,
approve work, execute patches, merge pull requests, call providers, publish
releases, or mutate repositories. Commands reject input packets that claim
authority outside their documented contract.

AO Forge remains the source of truth for GoalRun and release state. AO
Covenant owns policy decisions. AO2 owns governed execution. AO Foundry owns
portfolio coordination.

## Documentation

- [AO Command Foundry Design](docs/design/AO-COMMAND-FOUNDRY.md)
- [Production Readiness](docs/operations/PRODUCTION-READINESS.md)
- [Branch Protection](docs/operations/BRANCH-PROTECTION.md)
- [Retained Evidence](docs/operations/RETAINED-EVIDENCE.md)
- [Publication Checklist](docs/operations/PUBLICATION-CHECKLIST.md)
- [Historical v0.1.2 Operator Closeout](docs/release/V0.1.2-OPERATOR-CLOSEOUT.md)
- [Full Reference](REFERENCE.md)

## Verification

```sh
go test ./...
go vet ./...
go build -o bin/ao-command ./cmd/ao-command
scripts/ao-command-smoke.sh \
  --forge ../ao-forge \
  --foundry ../ao-foundry \
  --out tmp/ao-command-smoke
```

## License

AO Command is licensed under Apache 2.0. See [LICENSE](LICENSE).
