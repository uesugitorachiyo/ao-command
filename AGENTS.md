# AO Command Agent Instructions

## Status And Role

AO Command is the active read-only operator interface for AO. It renders status, timelines, next actions, validation summaries, and JSON readbacks from records owned by Mission, Atlas, Foundry, Forge, Covenant, AO2, and AO2 Control Plane.

Command does not become a source of domain truth. It has no authority to approve, schedule, execute, mutate AO state or repositories, merge changes, call providers, or publish releases.

## Sources Of Truth

- [docs/design/AO-COMMAND-V0.1.md](docs/design/AO-COMMAND-V0.1.md) and [docs/design/AO-COMMAND-FOUNDRY.md](docs/design/AO-COMMAND-FOUNDRY.md) define the read-only interface and producer boundaries.
- `docs/contracts/`, `internal/cli/`, and their tests own Command's output schemas and implemented validation. [REFERENCE.md](REFERENCE.md) is the current command reference.
- [docs/operations/PRODUCTION-READINESS.md](docs/operations/PRODUCTION-READINESS.md) and [docs/operations/RETAINED-EVIDENCE.md](docs/operations/RETAINED-EVIDENCE.md) define readiness and provenance handling.
- `scripts/production-readiness-audit.sh`, `scripts/ao-command-smoke.sh`, and [`.github/workflows/ci.yml`](.github/workflows/ci.yml) define the broad gates.

## Ownership And Boundaries

- Validate every input against its producer-owned schema and preserve source repository, source head, artifact path, SHA-256 digest, freshness, and authority flags in readback.
- Report producer state without synthesizing approval or authority. Reject packets that claim execution, scheduling, mutation, release, provider, credential, or policy authority beyond their contract.
- Forge owns GoalRun and release state; Foundry owns portfolio coordination; Covenant owns policy decisions; AO2 owns execution; Mission and Atlas own their lifecycle and workgraph records.
- Treat `docs/operations/public-provenance-manifest.json`, `docs/release/`, publication records, and retained evidence as historical. Do not rewrite them to make a current status or readiness claim pass.
- Keep generated views, audits, smoke output, binaries, and release previews in ignored `tmp/`, `bin/`, or `dist/`. Do not hand-edit generated results.
- Do not record secrets, credentials, private repository content, account identifiers, or machine-local paths. Release, deployment, publication, live mutation, credentialed operation, and direct-main changes require separate authority and are not Command operations.

## Working Method

- Change the smallest presentation or validation surface while preserving deterministic output, fail-closed input checks, producer attribution, redaction, path containment, and read-only behavior.
- Add negative tests for stale, digest-mismatched, malformed, private, or over-authority inputs. Do not weaken producer fixtures to make a view render.
- Update this file in the same pull request when durable commands, architecture, ownership, or authority boundaries change.

## Verification

- Command and readback changes: `go test ./internal/cli -count=1`.
- Format relevant Go source with `gofmt -d cmd internal`; run `go test ./... -count=1`, `go vet ./...`, and `go build -o bin/ao-command ./cmd/ao-command`.
- Run `scripts/ao-command-smoke.sh --forge ../ao-forge --foundry ../ao-foundry --out tmp/ao-command-smoke` when cross-repository views change.
- Run the documented production-readiness audit in non-admin mode for release-facing or readiness changes. Release rehearsal and publication remain conditional on separate authority.
- For instruction changes run `python3 ../ao-architecture/scripts/verify_agent_instruction_layout.py --workspace-root .. --repository ao-command`. Always run `git diff --check`.

## Evidence And Completion

- Record the Command source head, producer source heads, commands and exits, and relevant input/output digests. Report skipped, unavailable, stale, or failed checks explicitly.
- A rendered ready state is readback evidence only. Completion requires focused and broad gates, green pull-request CI, clean synchronized `main`, and task-branch cleanup.
