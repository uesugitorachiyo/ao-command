# AO Command v0.1

AO Command v0.1 is a read-only command center over AO Forge evidence. It should
make daily operation boring: status, goals, packets, decisions, evidence, safe
rehearsal, and next recommended action.

This is the first operator-surface slice for the active AO stack. AO Foundry is
the persistent operations factory for many projects; AO Command deliberately
does not schedule autonomous cross-repo work. v0.1 proves that AO Command can
read AO Forge and AO Foundry evidence and guide a human operator first.

## Boundaries

- AO Forge owns GoalRun state, production-readiness scoring, release gates, and
  retained evidence policy.
- AO Covenant owns allow, deny, and block decisions.
- AO2 executes governed work.
- ao2-control-plane stores and exposes evidence.
- AO Command reads and presents the evidence. It does not reimplement policies.

## V0.1 Surface

- `status`: show AO Forge production-readiness percentage, gate count,
  required next action count, derived production-ready decision, operator mode,
  and release governance state.
- `next`: explain the next operator action from AO Forge readiness evidence.
- `goals`: inspect an AO Forge GoalRun and show phase, next task, and guard.
- `evidence`: validate a contract document through AO Forge.
- `rehearse`: run AO Forge release-preview dry-run evidence and inspect it.

## Acceptance Criteria

- Commands are read-only by default.
- Every recommendation cites AO Forge output.
- Rehearsal proves dry-run evidence before any release mutation path exists.
- JSON output is available for automation.
- No secret, token, or provider payload is displayed by default.

## AO Foundry Hand-Off

AO Foundry owns the project registry, readiness loop, release handoff, task
queues, overnight advancement, Covenant-signed job results, and control-plane
evidence for many repos. AO Command consumes that evidence through read-only
commands such as `ao-command stack --ledger`.
