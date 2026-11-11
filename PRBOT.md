# open-code-review for PRBot

This is `Rukongai/open-code-review`, a fork of
[alibaba/open-code-review](https://github.com/alibaba/open-code-review)
(Apache-2.0). The `prbot` branch is upstream plus a small patch stack that
PRBot needs. Its GitHub releases are consumed by PRBot, which bundles the
`ocr` binary in its desktop app for macOS, Windows and Linux (x64 and arm64)
and downloads each asset pinned by release tag and SHA-256.

`main` tracks upstream unchanged. Do not release from it.

## Patches

On top of upstream `9b0fab8` (`git describe`: `v1.13.3-1-g9b0fab8`, i.e. one
commit after upstream `v1.13.3` and 25 commits before `v1.13.7`):

| Commit | Change |
| --- | --- |
| `3b8b474` | fix(tool): bound long lines in `code_search` and `file_read`, honour excludes |
| `00edb8f` | feat(agent): experimental Decisions API triage for file groups |
| `f36118f` | feat(llm): report LiteLLM gateway cost per run |
| `b835c98` | feat(agent): report excluded files with the rule that matched |

Above them sit release-plumbing commits only (`ci:` and `docs:`): the
`prbot-release.yml` workflow, the fork guards listed below, and this file.
Carry all of them across rebases.

## Releases

Tags are `v<upstream>-prbot.<N>`, e.g. `v1.13.7-prbot.1`, where `<upstream>`
is the upstream release the branch is based on. The base above is not yet
`v1.13.7`; rebase onto it (see below) before cutting a `v1.13.7-prbot.*` tag,
or the binaries will report a version whose upstream changes they lack.

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
OLD_BASE=9b0fab8  # upstream commit prbot currently sits on (Patches table)
NEW_BASE=v1.13.7  # upstream tag to move to
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
