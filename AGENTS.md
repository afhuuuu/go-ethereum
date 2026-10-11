# AGENTS

## Guidelines

- **Keep changes minimal and focused.** Only modify code directly related to the task at hand. Do not refactor unrelated code, rename existing variables or functions for style, or bundle unrelated fixes into the same commit or PR.
- **Do not add, remove, or update dependencies** unless the task explicitly requires it.

## Personal fork workflow (user decision, 2026-10-11)

These rules replace the former requirement to run every full check before every
commit. The user authorized focused validation and a commit/push record for each
round of changes. A development checkpoint is not a release or merge approval.
Preserve historical reports; their old no-push and copy/hash procedures do not
govern newly authorized work. Explicit limits in the current user task still win.

### Scope and work directory

- The feature is optional tracing of the existing sequential `eth_simulateV1`
  execution: `callTracer`, `prestateTracer` with `diffMode`, and the supported mux.
  Normal results and traces must come from the same execution, with a fresh tracer
  per call and request-local state. Missing sparse diff fields are not zero values.
- Public receipt export remains deferred. Do not change `debug_traceCall`, the
  Python business script, EVM/consensus rules, dependencies, or fixtures merely to
  make this feature's tests pass.
- Reuse this checkout and `feat/simulate-v1-tracing`. Create another worktree only
  for actual isolation or a necessary baseline comparison, not for each stage.
  Preserve old evidence directories; do not create new full source/fixture copies,
  numbered packaging rounds, or custom runners by default.
- Keep simulation, unrelated test/build fixes, and personal-fork CI in separate
  logical commits. Fork policy/docs/workflows are not automatically part of a
  future upstream feature PR.

### Validation during development

- Documentation/policy-only changes: review the diff and run `git diff --check`;
  do not launch Go tests, builds, generators, or new tests for prose edits.
- Go changes: format only modified Go files with `gofmt` and `goimports`, then run
  focused tests for changed behavior and directly affected callers. Use race tests
  when tracer lifecycle, cancellation, concurrent requests, or shared state changes.
  Test inherited state, per-call pre/post boundaries, revert/error handling,
  unchanged behavior without tracing, and single execution where relevant.
- Reuse prior results when the relevant code, dependencies, inputs and environment
  still match. Record their source revision/scope and that they are historical.
  A new message, commit, or directory alone is not a reason to repeat them.
- Run generation checks when generators, directives or relevant dependencies
  change; check dependency boundaries when imports/dependencies change. Do not
  install tools or run whole-project generation just to save a checkpoint.
- Use Git status/diff and the baseline commit for scope protection. Record fixture
  versions and real command exit codes. Hash downloaded archives or specific files
  when identity is material; do not repeatedly hash thousands of unchanged files,
  parse old large logs, or require exact whole-directory file counts.
- Preserve the first failure and diagnose its relevant cause. A justified local
  repair may rerun only the affected check within the task budget; no blind retries
  or expanding audit loops. If the user explicitly says one attempt/no retry, obey.
- Keep `PASS`, `FAIL`, `SKIP`, `NOT_RUN`, historical reuse and known gaps distinct.
  A baseline failure or unsupported fixture key is not a feature regression and is
  not a pass. Investigate unrelated failures separately instead of changing roots,
  skip lists or consensus rules to satisfy stale fixtures.

### Checkpoints, push and rollback

- After each meaningful round with changes, create focused commit(s) and push to
  the same feature branch in `afhuuuu/go-ethereum`, unless the user explicitly
  suspends publication. This is standing authorization for that fork and branch;
  do not request it again each round. Read-only reviews need no empty commits.
- A checkpoint may retain clearly documented failing or unrun checks. Mark it as
  WIP in its body/work log and report the actual results; never call it merge-ready.
  Do not block saving progress on unrelated full-suite failures.
- Before staging, inspect existing staged and unstaged changes. Stage explicit
  paths/hunks for the current logical change; do not absorb unrelated user work.
  Review the staged diff and all commits pending push, including deleted content,
  for secrets, RPC credentials, personal data, binaries and bulky logs/fixtures.
  Report suspected secrets by location/type only, never by value.
- The only permitted push repository is `git@github.com:afhuuuu/go-ethereum.git`.
  Verify all effective push URLs and the destination branch before each push.
  Keep upstream push disabled. Never push to `ethereum/go-ethereum`, `master`, or
  another repository, and never use `--all`, `--mirror` or force push.
- Use `git push origin HEAD:refs/heads/feat/simulate-v1-tracing` and verify the
  remote branch SHA equals local HEAD. Report commit SHA, branch, push result and
  validation status. If push fails, retain the local commit and report the failure;
  do not silently change remotes, rewrite history or claim remote backup exists.
- Preserve published history. Prefer an explicit `git revert` for a requested
  rollback; do not reset, amend/rebase published commits, or delete old workspaces
  merely to make the checkout look clean.
- Keep a short tracked work log under `docs/simulate-v1/`. Save test summaries and
  commands there; large/raw artifacts stay local. Do not make self-referential
  "record this commit's own hash" commits; report its SHA after committing.

### Final validation and CI

- Ordinary fork pushes should run only relevant simulation/tracer tests, applicable
  race checks and a geth build. Documentation-only pushes should not run Go jobs.
- Full lint/generation/dependency checks, full-suite multi-version runs, keeper
  cross-builds and 386 coverage belong to an explicit final-validation run, such
  as manual `workflow_dispatch`, not every checkpoint push. Do not dispatch that
  expensive run without an explicit task budget. A push-triggered focused CI run
  is covered by the checkpoint authorization.
- Before claiming merge/release/upstream-PR readiness, review the final diff and
  complete the applicable full checks once against the final candidate:

  ```sh
  make all
  go run ./build/ci.go test
  go run ./build/ci.go lint
  go run ./build/ci.go check_generate
  go run ./build/ci.go check_baddeps
  ```

  The full test command must not use `-short`. Confirm actual keeper module and
  platform coverage from the build scripts/CI; `make all` alone is not evidence of
  all keeper cross-builds or tests. Check formatting of the changed Go files too.
- Retain unresolved Legacy fixture compatibility and unvisited-key findings until
  an evidence-based resolution. Full-check failures prevent a readiness claim,
  not a clearly labelled development checkpoint. Do not merge or open an upstream
  PR as a side effect of a progress push.

### Windows execution

- Use explicit UTF-8 for scripts and text files (`Python -X utf8`, explicit file
  encodings); copy patches/source as bytes when byte identity matters.
- Prefer direct commands and existing tools over another runner framework. Capture
  the actual process exit code. Scope environment/proxy changes to the subprocess;
  do not install an OS/toolchain or alter machine-wide settings without a task need.

## What to include in commits

Do not commit binaries, whether they are produced by the main build or byproducts of investigations.

## Commit Message Format

Commit messages must be prefixed with the package(s) they modify, followed by a short lowercase description:

```
<package(s)>: description
```

Examples:
- `core/vm: fix stack overflow in PUSH instruction`
- `eth, rpc: make trace configs optional`
- `cmd/geth: add new flag for sync mode`

Use comma-separated package names when multiple areas are affected. Keep the description concise.

## Pull Request Title Format

PR titles follow the same convention as commit messages:

```
<list of modified paths>: description
```

Examples:
- `core/vm: fix stack overflow in PUSH instruction`
- `core, eth: add arena allocator support`
- `cmd/geth, internal/ethapi: refactor transaction args`
- `trie/archiver: streaming subtree archival to fix OOM`

Use the top-level package paths, comma-separated if multiple areas are affected. Only mention the directories with functional changes, interface changes that trickle all over the codebase should not generate an exhaustive list. The description should be a short, lowercase summary of the change.
