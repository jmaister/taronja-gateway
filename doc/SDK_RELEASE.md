# SDK release automation

The React SDK release should use the main repository release tag as the single source of truth.

## Recommended release model

- Keep `/sdk` as the source package inside this repository.
- Publish the npm package directly from this repository on `release.published`.
- Mirror the SDK source package to `jmaister/taronja-gateway-clients` in `react/` on the same release event.
- Use the same semantic version everywhere by deriving it from the GitHub release tag.

## Release contract

- Create a GitHub release with a tag such as `v0.2.0`.
- `.github/workflows/sdk-release.yml` strips the leading `v`, stamps that version into `sdk/package.json`, runs the SDK tests and build, then publishes `taronja-gateway-react` to npm.
- `.github/workflows/clients.yml` uses the same release tag to build the SDK, generate the Go client, and push both into `jmaister/taronja-gateway-clients`.
- The clients repository receives these tags:
  - `v0.2.0` for the umbrella client release.
  - `go/v0.2.0` for the Go client subtree.
  - `react/v0.2.0` for the React SDK subtree.

## Required secrets

- `NPM_TOKEN`: npm automation token with publish access to `taronja-gateway-react`.
- `CLIENTS_REPO_TOKEN`: GitHub token with push access to `jmaister/taronja-gateway-clients`.

## Why this is the best fit here

- It keeps one source of truth for code and versions.
- It avoids publishing from the mirror repository, which would split ownership and make rollback/debugging harder.
- It reuses the release trigger already used by the Go binary workflow.
- It keeps `taronja-gateway-clients` as a distribution mirror and tag surface, not the canonical build source.
