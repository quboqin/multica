# Release runbook

## Normal release

Release this fork from a reviewed commit on `qqb_main` in `quboqin/multica`,
using a new stable semantic version tag such as `v0.6.2`. The Release workflow
intentionally has no manual trigger: a tag push is the only event that can
publish binaries, Homebrew formulae, and container images. Prerelease and dirty
tags are rejected. Tags are repository-wide: never push upstream tags in bulk.

The verification job requires the tag to resolve to the checked-out commit in
the history of `origin/qqb_main`. The most recent **push** runs of both `CI` and
`Mobile Verify` for that exact commit must have completed successfully; a PR
merge-ref result or a manual run is not sufficient. `ci-gate` verifies all
selected CI jobs and only accepts skips for unselected scopes. Mobile Verify
runs on every `qqb_main` push/PR to make this release check unambiguous.

The release then checks Homebrew publisher credentials, runs Go tests and
`govulncheck`, and publishes CLI/Homebrew, multi-architecture container images,
Helm, and Windows/Linux Desktop. macOS Desktop publication is optional and runs
alongside the required publishers; its Apple credentials are checked only in
`desktop-mac`. The vulnerability scan is fail-closed by default.
GoReleaser is pinned to v2.8.2, which accepts the current `brews` configuration;
run its `check` command and a snapshot build before changing that pin/schema.

Desktop signing uses a pnpm patch for `app-builder-lib@26.8.1` to keep the
temporary keychain password separate from the certificate import passwords.
When upgrading electron-builder, run `apps/desktop/scripts/signing-keychain.test.mjs`
and remove the patch only after confirming the upstream implementation passes.
This regression test uses fake credentials; a signed and notarized macOS build
is still required to validate the real signing setup. Web Docker dependency
stages must copy `patches/`, even when a patched package belongs to an omitted app.

## One-time GitHub configuration

1. Merge the intended product and pipeline changes into `qqb_main`. Make it the
   repository default branch so scheduled workflows also verify it.
2. Enable Actions. Require `ci-gate` and `Mobile Verify`'s `mobile` job for PRs
   (select the actual check names after their first run). Protect `qqb_main`
   against force pushes/deletion; restrict changes to published `v*` tags.
3. Create public `quboqin/homebrew-tap`, initialized with a `main` branch.
   GoReleaser writes `Formula/multica.rb` there; the tap's branch stays `main`.
4. Create a fine-grained token limited to that tap with Contents read/write.
   Save it as `HOMEBREW_TAP_GITHUB_TOKEN` in **quboqin/multica** Actions secrets.
   Allow the publishing identity to write the tap, or configure a PR workflow
   there. The automatically issued `GITHUB_TOKEN` does not grant cross-repo write.
5. Add `MAC_CSC_LINK` (base64 Developer ID Application .p12),
   `MAC_CSC_KEY_PASSWORD`, `APPLE_ID`, `APPLE_APP_SPECIFIC_PASSWORD`, and
   `APPLE_TEAM_ID` as repository secrets. macOS release builds require these,
   sign with `forceCodeSigning`, and notarize both architectures. Windows/Linux
   currently remain unsigned. None of these credentials belongs in source files.
6. After the first publish, verify GHCR packages are public (or provision read
   credentials on private-image deployment hosts) and linked to this repository.
   The workflow's `GITHUB_TOKEN` publishes packages with `packages: write`.

This pipeline publishes a GitHub Release before Desktop uploads finish. Keep
immutable releases disabled with this design: immutable releases cannot accept
later assets or same-tag replacement. To adopt immutability, first change all
publishers to stage a draft, finalize after all uploads, then update Homebrew.
`release-complete` is a deployment gate, not an atomic public-release transaction.

## Optional macOS Desktop publication

`release-complete` does not depend on `desktop-mac`. It verifies all six CLI
targets (including Darwin), Windows/Linux Desktop assets and updater feeds, and
both container architectures after the required publishers finish. Apple
credential, signing, notarization, upload, or macOS asset-validation failures do
not block this gate or an enabled production deployment. `desktop-mac` uses
job-level `continue-on-error`; failures remain visible in its steps, warning,
and job summary, but do not make the workflow fail.

macOS still requires Developer ID signing and Apple notarization. After both
architectures upload, that job separately verifies the macOS installers,
blockmaps, and updater references. A successful `release-complete` or overall
workflow does **not** imply that macOS downloads or updates are available. Check
`desktop-mac`'s packaging and asset-verification steps before announcing them.

The asset verifier supports `--scope=required` for the main gate and
`--scope=mac` for the macOS job. Omitting the option (or using `--scope=all`)
still requires all platforms for an explicit full-release audit.

Because macOS remains in the same workflow, the overall run stays in progress
while Apple is processing, even after the required release/deployment completes.
The workflow-wide `release-stable` concurrency lock also remains held, so a new
tag's workflow waits for the older run to finish. This preserves ordering of
shared updater feeds and Homebrew publication across versions. Making later
releases independent of that wait requires separating macOS publication and
designing its cross-version publication ordering.

Workflow changes apply to new tags pointing at the reviewed change. Re-running
an older tag uses that tag's workflow; do not move `v0.7.1` or another existing
tag to apply this policy retroactively.

## Channels and connection defaults

| Purpose | Destination |
| --- | --- |
| Release assets | `https://github.com/quboqin/multica/releases` |
| Install scripts | `https://raw.githubusercontent.com/quboqin/multica/qqb_main/scripts/` |
| Homebrew CLI | `quboqin/tap/multica` |
| Backend / web images | `ghcr.io/quboqin/multica-backend`, `ghcr.io/quboqin/multica-web` |
| Helm | `oci://ghcr.io/quboqin/charts/multica` |
| Desktop web origin | `https://multica.magicefire.xyz:18443` |
| Desktop API origin | `https://multica.magicefire.xyz:18444` |
| Desktop WebSocket | `wss://multica.magicefire.xyz:18444/ws` |

The packaged Desktop reads `~/.multica/desktop.json` when present; otherwise it
uses these fork defaults. Dev `VITE_*` overrides remain development-only. App
identity/protocol and local data paths have not been renamed; this is an upgrade
distribution, not a side-by-side branded app. Users of the official distribution
must explicitly install the fork once. Mobile app publication is separate.

Connect a standalone CLI with:

```sh
multica setup self-host \
  --server-url https://multica.magicefire.xyz:18444 \
  --app-url https://multica.magicefire.xyz:18443
```

CLI `multica update`, Desktop updates/bootstrap, the download page and in-product
install commands all use this fork. Explicit `multica setup cloud` still means
the upstream cloud. Self-hosted daemon background updates remain default-off;
operators may opt in with `MULTICA_DAEMON_AUTO_UPDATE=true` after testing upgrades.
Desktop-managed daemons update with the bundled CLI, not independently.

## Optional automatic production upgrades

The repository variable `ENABLE_PRODUCTION_DEPLOY` defaults to disabled. Set it
to the exact string `true` only after provisioning an existing Compose stack and
testing the deployment procedure. No infrastructure is created by this workflow.

Create the `production` GitHub Environment. Allow `v*` tags as deployment sources
(the calling workflow is a tag run, not a branch run). Configure:

| Kind | Name | Value |
| --- | --- | --- |
| Variable | `DEPLOY_HOST` | SSH host name or IPv4 address, without a port |
| Variable | `DEPLOY_PORT` | SSH port, 1–65535; defaults to 22. This deployment uses 23022. |
| Variable | `DEPLOY_USER` | Account with Docker and deployment-directory access |
| Variable | `DEPLOY_PATH` | Absolute path to an existing deployment; no spaces |
| Secret | `DEPLOY_SSH_KEY` | Dedicated SSH private key |
| Secret | `DEPLOY_KNOWN_HOSTS` | Independently verified SSH host public-key entry |

For a non-default SSH port, the known-hosts entry must identify
`[host]:port`, for example `[multica.magicefire.xyz]:23022`. Compare the host-key
fingerprint with the server console or an existing trusted record before saving
it; `ssh-keyscan` alone does not establish trust. Both SSH commands and SCP file
transfers use `DEPLOY_PORT` with strict host-key verification.

Install Docker Compose on that host. Its deployment directory must contain a
production `.env`, with `postgres` and `backend` already running under the Compose
project configured by the supplied file (`multica` unless explicitly configured).
For private GHCR packages, authenticate Docker on the host before enabling
deployment; the workflow does not send registry tokens to the host. Keep database,
JWT, VCS encryption, mail and storage secrets in the server `.env`.

For this deployment set `FRONTEND_ORIGIN` and `MULTICA_APP_URL` to
`https://multica.magicefire.xyz:18443`. Configure the reverse proxy separately:
18443 serves web, 18444 serves API and `/ws`. Use Compose's loopback bindings;
the public HTTPS ports are proxy ports, not a reason to expose raw containers.

After `release-complete`, `deploy.yml` checks the tag again, transfers that tag's
Compose file/script over host-verified SSH and executes `deploy-selfhost.sh`.
It pulls both images, stops frontend/backend writers, backs up PostgreSQL and
the local uploads volume, starts the new containers (including normal startup
migrations), and checks internal web/API health plus the expected backend commit.
Successful image selection is persisted to `.env`, the Compose file and
`.current-release`. Backups are private files under `DEPLOY_PATH/backups/`.

This process causes downtime. Object-store files require a separate storage
backup/versioning policy; the built-in upload backup covers only the local volume.
Health checks do not prove login, permissions, WebSocket or Documents/Collections
behavior. Verify those with a test account after deployment. Backup retention and
off-host copies are operator responsibilities.

On backup/startup/health failure, the script fails and does **not** attempt a
database rollback. Inspect the backup and container state before restarting or
restoring. A backup failure after stopping writers can leave the old service
stopped. A hard interruption can leave `.deploy-lock`; remove it only after
checking there is no deployment still running. GitHub and host-side locks prevent
overlapping upgrades; the production job is never cancelled by a newer release.

## Per-release procedure

1. Merge the release PR into `qqb_main`; wait for both push workflows at that SHA.
2. Verify a clean checkout at that commit, choose an unused stable tag and push
   **only that tag**. No package.json version bump is required for Desktop;
   its packaging wrapper derives the version from Git.

   ```sh
   git switch qqb_main
   git pull --ff-only origin qqb_main
   git status --short
   git tag -a v0.6.2 -m 'Release v0.6.2'
   git push origin v0.6.2
   ```

3. Wait for `release-complete`: it requires CLI/Homebrew, Windows/Linux Desktop,
   images and Helm publishers; validates required asset names, nonempty uploads,
   checksum entries and per-architecture updater references; and verifies both
   image architectures. Check macOS publication separately in `desktop-mac`.
   It does not download every installer or execute it. Installation and a real
   A-to-B update test remain necessary acceptance checks.
4. If automatic deployment is enabled, inspect its result and backups. Otherwise
   deploy the exact image tag manually using the self-hosting guide.
5. Check Documents/Collections access, persistent data, live updates and daemon
   connectivity. On two successive versions test `brew upgrade`, `multica update`
   and Desktop's actual download/restart flow on each supported platform.

Use `Desktop Smoke Build` with `qqb_main` for unsigned Windows/Linux packaging
checks without publication. Release retries are serialized; do not move an
existing tag to different code. Fix product failures in a new patch release.

## Emergency vulnerability-scan bypass

Use the bypass only when `govulncheck` itself or its live vulnerability database
is unavailable, or when maintainers have documented a confirmed false positive
that blocks an urgent release. Never use it to publish a release with an
unresolved reachable vulnerability.

1. Record the reason and maintainer approval in the release issue or pull
   request, and confirm no other release is in progress.
2. In **Settings → Secrets and variables → Actions → Variables**, set the
   repository variable `ALLOW_VULN_BYPASS_FOR_TAG` to the exact release tag,
   for example `v0.18.4`.
3. Re-run the failed Release workflow for that tag. A different tag, an empty
   value, or any typo keeps the scan enabled.
4. Confirm the verification log contains the explicit bypass warning and retain
   the workflow URL in the incident record.
5. Delete `ALLOW_VULN_BYPASS_FOR_TAG` immediately after the release run
   completes. The tag-scoped value prevents a concurrent release with another
   tag from inheriting the bypass.

Every Go binary retains its compiler version in the standard Go build metadata;
use `go version -m <binary>` when auditing a downloaded release artifact.
