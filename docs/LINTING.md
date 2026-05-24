# Linting

The motes repo runs `golangci-lint` in its pre-commit hook
(`.githooks/pre-commit`). Pinned version: **`v2.10.1`**.

## --new-from-rev=HEAD model

The hook invokes:

```
CGO_ENABLED=0 go run github.com/golangci/golangci-lint/v2/cmd/golangci-lint@v2.10.1 \
    run --new-from-rev=HEAD --fix
```

This means:

- **Blocked**: any commit that introduces a NEW lint violation relative to `HEAD`.
- **Allowed**: commits that only touch files containing pre-existing
  (baseline) warnings, as long as the diff does not add new ones.

The goal is "don't make it worse" rather than "fix the world before
you can commit". It keeps the hook fast and the contributor focused on
their own change.

## Inspecting the full baseline

To see every lint warning in the repo (including pre-existing ones):

```bash
CGO_ENABLED=0 go run github.com/golangci/golangci-lint/v2/cmd/golangci-lint@v2.10.1 \
    run ./...
```

This is informational — it does not block any workflow today. If the
team chooses to start eliminating the baseline, work it down as a
deliberate cleanup, not as a commit-time gate.

## Version pin policy

The pinned version lives in **`.githooks/pre-commit`**, which is the
single source of truth. If/when a `pre-commit`-framework config is
added (STORY-BR-21), it must reference the same version. Drift
between the two would let CI and local commits disagree about what
counts as a lint violation.

## Why CGO_ENABLED=0

Some transitive deps activate CGO conditionally. `golangci-lint`'s
typecheck phase has been known to panic on such packages. Motes has
no CGO of its own today, so this is belt-and-suspenders — cheap to
include and future-proof.
