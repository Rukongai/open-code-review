# open-code-review for PRBot

This is `Rukongai/open-code-review`, a fork of
[alibaba/open-code-review](https://github.com/alibaba/open-code-review)
(Apache-2.0). The `prbot` branch is upstream plus a small patch stack that
PRBot needs. Its GitHub releases are consumed by PRBot, which bundles the
`ocr` binary in its desktop app for macOS, Windows and Linux (x64 and arm64)
and downloads each asset pinned by release tag and SHA-256.

`main` tracks upstream unchanged. Do not release from it.

## Patches

On top of upstream `main` at `c469d7a` (`v1.13.7` plus 7 commits, no newer tag yet):

| Commit | Change |
| --- | --- |
| `7c08e97` | fix(tool): bound long lines in `code_search` and `file_read`, honour excludes |
| `2a56c98` | feat(agent): experimental Decisions API triage for file groups |
| `d2d142d` | feat(llm): report LiteLLM gateway cost per run |
| `4104d0f` | feat(agent): report excluded files with the rule that matched |

Rebase notes for `c469d7a`: one conflict, in `manual_e2e_retry_test.go` (upstream moved tool defs into task templates; keep its block and pass the extra `nil` filter to `buildToolRegistry`). Upstream's runtime refactor
added golden fixtures under `internal/reviewtask/testdata/equivalence/` that
snapshot the tool schemas sent to the model, so the long-lines commit now also
regenerates them (`UPDATE_EQUIVALENCE=1 go test ./internal/reviewtask/`); the
only fixture change is the new `code_search`/`file_read` descriptions and the
`start_col` parameter. Regenerate them the same way whenever a patch changes
`tools.json`.

Above them sit release-plumbing commits only (`ci:` and `docs:`): the
`prbot-release.yml` workflow, the fork guards listed below, and this file.
Carry all of them across rebases.

## Releases

Tags are `v<upstream>-prbot.<N>`, e.g. `v1.13.7-prbot.1`, where `<upstream>`
is the upstream release the branch is based on (see Patches). Rebase onto a
new upstream tag before cutting a tag for it, or the binaries will report a
version whose upstream changes they lack.

Pushing a tag runs
`.github/workflows/prbot-release.yml`, which on a single Linux runner:

1. runs `go test ./...`;
2. cross-compiles with `make build-all BINARY_NAME=ocr VERSION=<tag>`
   (`CGO_ENABLED=0`, `-s -w`, version injected through `-X main.Version`);
3. creates a GitHub release for the tag with these assets:

   - `ocr-darwin-amd64`, `ocr-darwin-arm64`
   - `ocr-linux-amd64`, `ocr-linux-arm64`
   - `ocr-windows-amd64.exe`, `ocr-windows-arm64.exe`
   - `LICENSE`
   - `SHA256SUMS` (`shasum -a 256` format, covers every asset above)

`ocr --version` on those binaries prints `open-code-review v1.13.7-prbot.N (<commit>) <os>/<arch>`.

Every push to the `prbot` branch is a dry run: it tests, builds and uploads
the same files as a workflow artifact named `ocr-v0.0.0-prbot.dryrun`, and
creates no release. A manual run (`gh workflow run prbot-release.yml --ref
prbot -f version=...`) does the same with a chosen embedded version, but
GitHub only dispatches workflows that exist on the default branch, so it
works only if `prbot-release.yml` is on `main` or `prbot` is made the
default branch; `main` is kept as a pristine upstream mirror, hence the
branch-push trigger.

### Cutting the next release

```sh
git switch prbot
git pull --ff-only
N=2   # previous release + 1
git tag -a "v1.13.7-prbot.$N" -m "v1.13.7-prbot.$N"
git push origin "v1.13.7-prbot.$N"
gh run watch "$(gh run list --workflow prbot-release.yml --limit 1 --json databaseId --jq '.[0].databaseId')"
```

Then update PRBot's pinned tag and the per-asset SHA-256 values from the
release's `SHA256SUMS`. Never move or delete a published tag: PRBot pins
the hashes, so replaced assets would fail verification.

## Rebasing onto a new upstream tag

```sh
git remote add upstream https://github.com/alibaba/open-code-review.git  # once
git fetch upstream --tags
OLD_BASE=v1.13.7  # upstream tag prbot currently sits on (Patches table)
NEW_BASE=v1.13.8  # upstream tag to move to
git switch prbot
git rebase --onto "$NEW_BASE" "$OLD_BASE" prbot
make check && make test
git push --force-with-lease origin prbot
git tag -a "$NEW_BASE-prbot.1" -m "$NEW_BASE-prbot.1"
git push origin "$NEW_BASE-prbot.1"
```

Restart `N` at 1 for each new upstream version. Update the base and commit
hashes in the Patches table after rebasing. Earlier release tags keep
pointing at their original commits, so old releases stay reproducible.

If upstream adds a workflow that publishes somewhere or needs upstream-only
infrastructure, guard it as below before pushing.

## Upstream workflows kept off the fork

Upstream jobs mostly run on self-hosted runners and publish with upstream
credentials; in this fork they would queue forever or fail. On `prbot`:

| Workflow | Guard | Why |
| --- | --- | --- |
| `release.yml` | tag filter `!v*-prbot.*`; `verify-trusted-publishing` and `build` require `github.repository != 'Rukongai/open-code-review'` (`release` and `npm-publish` depend on `build`) | would build our tags on a self-hosted runner, create an upstream-style release and publish to npm |
| `ocr-review.yml` | job requires `github.repository != 'Rukongai/open-code-review'` | `pull_request_target` on any branch; needs upstream secrets and runner |
| `translation-sync.yml` | job requires `github.repository != 'Rukongai/open-code-review'` | `pull_request` on any branch; self-hosted runner |

Left unchanged because they never trigger on `prbot` or its tags:
`deploy-pages.yml` (already `alibaba/open-code-review` only), and `ci.yml`,
`codeql.yml`, `acp-ci.yml`, `action-contract.yml`, `plugin-contract.yml`,
`pages-ci.yml`, `frontend-ext.yml`, `idea-ext.yml`, `vscode-ext.yml`,
`pr-commit-policy.yml` (push to and/or PRs into `main` only).
