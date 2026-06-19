# AO Command Foundry

AO Command Foundry is the long-term product direction for AO Command. It is an
autonomous operations factory for many projects, not just a prompt runner or
workflow library.

## Positioning

Most agent frameworks execute workflows. Foundry should maintain persistent
engineering operations state across repositories and decide the next safe action
from verified evidence.

Foundry combines:

- project registry;
- production-readiness state;
- branch and release state;
- CI and docs health;
- dependency and security review queues;
- incident and roadmap task queues;
- AO Covenant signed job outcomes;
- ao2-control-plane evidence readback.

The near-term `ao-command` v0.1 CLI is the read-only operator surface that makes
this evidence usable. Foundry comes after AO Forge is trustworthy enough to be
the source of operational truth.

## Core Differentiator

Persistent operational memory plus verified execution. Foundry should behave
like an engineering operations layer: it knows project state, schedules work,
limits concurrency, records evidence, and reports what remains risky.

It should decide what to do next only when the decision can be proven from AO
Forge, AO Covenant, AO2, and control-plane evidence.

## Foundry Capabilities

- Project registry for repos, branches, CI, release status, owners, readiness
  score, and active goals.
- Task queues with dependency ordering, priority, concurrency limits, and stop
  conditions.
- Safe overnight advancement with one active job per repo/branch, no hidden
  dirty worktrees, no unmerged branch sprawl, and no provider mutation without
  explicit policy.
- Covenant-signed autonomous job result packets.
- Control-plane evidence showing what happened, what changed, what failed, what
  remains risky, and what should happen next.
- Operator review surfaces for promotion, rollback, security review, dependency
  updates, and docs drift.

## Build Order

1. Finish AO Forge as the trusted factory brain.
2. Build AO Command v0.1 as the read-only operator surface.
3. Add AO Arena as an internal quality and replay mode.
4. Evolve AO Command into Foundry once cross-repo autonomy has reliable gates,
   memory, evidence, and rollback behavior.

## V0.1 Boundary

The current `ao-command` scaffold is not Foundry yet. It intentionally avoids
autonomous writes, cross-repo task scheduling, branch mutation, release
publishing, and background loops. Those belong behind later Foundry contracts
after AO Forge and AO Covenant can prove the work is safe.
