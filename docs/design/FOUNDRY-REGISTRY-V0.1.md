# Foundry Registry v0.1

The Foundry registry is the first durable model AO Command will need before it
can become a multi-repo operations factory. It should remain read-only until AO
Forge, AO Covenant, and ao2-control-plane evidence can prove that autonomous
work is safe.

## Registry Record

Each project record should describe:

- repository owner and name;
- default branch and protected branches;
- current CI status;
- latest production-readiness percentage;
- latest release preview and release promotion status;
- active GoalRuns;
- docs drift status;
- dependency update status;
- security review status;
- maximum concurrent jobs;
- allowed automation scope;
- stop conditions.

## Scheduling Rules

- Only one active job per repo and branch.
- No hidden dirty worktrees.
- No unmerged branch sprawl.
- No release, provider, or control-plane mutation without explicit Covenant and
  operator approval.
- Every queued job must name its evidence source, expected output, rollback or
  backoff condition, and owner.

## Privacy Boundary

The registry may contain private repo names, branch names, incidents, security
review state, and release timing. Treat it as private operational memory. Do
not publish registry snapshots, CI artifacts, or control-plane exports until the
repository is explicitly approved for public release.
