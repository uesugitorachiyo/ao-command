# AO Command Foundry Legacy Design Note

This document is a legacy product note. The active stack now uses the separate
AO Foundry repository as the persistent engineering operations layer. AO Command
stays the read-only operator surface over AO Forge, AO Foundry, AO Covenant,
AO2, and ao2-control-plane evidence.

Do not use this document to expand `ao-command` into a scheduler, background
loop, release publisher, branch mutator, provider mutator, or replacement for
AO Foundry.

## Positioning

AO Foundry maintains persistent engineering operations state across repositories
and decides the next safe action from verified evidence. AO Command presents
that evidence to a human operator and keeps mutation paths out of scope.

The active AO Foundry layer combines:

- project registry;
- production-readiness state;
- branch and release state;
- CI and docs health;
- dependency and security review queues;
- incident and roadmap task queues;
- AO Covenant signed job outcomes;
- ao2-control-plane evidence readback.

The `ao-command` v0.1 CLI is the read-only operator surface that makes this
evidence usable. AO Forge remains the source of factory truth; AO Foundry owns
the active-stack operations ledger.

For oversized and live-mutation work, `ao-command blueprint-atlas-foundry status`
is the operator readback over the enforced Blueprint -> Atlas -> Foundry path.
It reports Blueprint pack status, Atlas import/preflight status, Foundry gate
status, and ready or blocked reason from existing artifacts only. It does not
compile, schedule, approve, execute, or mutate work.

## Core Differentiator

Persistent operational memory plus verified execution belongs in AO Foundry.
AO Command reports project state, release handoff status, and next actions from
verified evidence, but it does not schedule work or mutate repositories.

Any next-action recommendation shown by AO Command should be proven from AO
Forge, AO Foundry, AO Covenant, AO2, and control-plane evidence.

## AO Foundry Capabilities

- Project registry and active-stack ledger for repos, branches, CI, release
  status, owners, readiness score, and active goals.
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

## Current Build Order

1. Finish AO Forge as the trusted factory brain.
2. Build AO Command v0.1 as the read-only operator surface.
3. Use AO Foundry as the persistent operations layer once cross-repo autonomy
   has reliable gates, memory, evidence, and rollback behavior.
4. Keep AO Command focused on read-only operator visibility unless a future
   Covenant-approved design explicitly changes that boundary.

## V0.1 Boundary

The current `ao-command` scaffold is not AO Foundry. It intentionally avoids
autonomous writes, cross-repo task scheduling, branch mutation, release
publishing, and background loops. Those belong in AO Foundry contracts after AO
Forge and AO Covenant can prove the work is safe.
