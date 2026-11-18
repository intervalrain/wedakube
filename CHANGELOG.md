# Changelog

All notable changes are kept here. Tags follow [SemVer](https://semver.org)
via `git describe --tags --dirty`.

## v0.0.4 — 2026-11-18

### Added

- `kube ctx` / `kube ctx ls` / `kube ctx select <prefix>` pick the server that
  `kube deploy` ships to (stored as `currentHost`); `kube deploy [path]` defaults to `.`.
- Builds target the node's platform (kubectl nodeInfo, then `uname -sm`, then
  linux/amd64), so arm64 nodes get arm64 images.
- No helmster on the node but an existing helm release: `helm upgrade` on the registry's
  OCI chart with `--reuse-values` and the new `image.tag` (appcfg changes not applied, with
  a warning).
- **Two deploy modes for helm-managed services**, `kube deploy --mode auto|image|full`:
  - `image`: `kubectl set image` on the existing Deployment (fast; skips helm, the
    db-migrator Job and appcfg: env, traefik routes, ECO).
  - `full`: helmster gen + helm upgrade, as CI (db-migrator runs, appcfg applied).
  - `auto` (default): `full` when appcfg, `Migrations/`, a `*DbMigrator` project or
    `charts/` changed since the release's last full deploy (its `app_version`), or when
    that commit is unknown (e.g. the release came from CI); `image` otherwise. The choice
    and its reason are printed.
  - Every build records `image:tag -> commit` in `state.json` (`builds`) for that diff.
  - `set image` targets the Deployment's first container instead of assuming it is named
    after the service.

### Changed

- **Deploys build what CI builds.** `kube deploy` no longer uses the working tree as the
  docker build context. It exports the committed tree of a ref plus every submodule at its
  pinned commit (`internal/deploy/source.go`), so gitignored or uncommitted files never
  reach the image. A local `appsettings.secrets.json` had been baked into a service image and
  made it connect to `localhost`. A dirty working tree is reported, not built.
- `kube deploy --ref <branch|tag|commit>` deploys any ref without switching the checkout.
- The feed PAT is injected into `NuGet.Config` (`PAT_TOKEN_HERE`) in the export, as CI's
  `sed` step does, for repos whose Dockerfile has no `ARG FEED_PAT`. The build args stay.
- Helmster path: `appcfg.yaml`'s top-level `version` is set to the build tag before
  `helmster gen`, like CI's `yq -i .version`, so pre-upgrade Jobs (db-migrator) pull the
  image just pushed instead of the committed tag.

### Fixed

- `helm upgrade` passes the Service's current `.spec.type` as `service.type`, so a Service
  someone patched to NodePort keeps its type and nodePort.
- The feed PAT is passed as both `FEED_PAT` and `FEED_TOKEN` build args.
- A failed `helm upgrade` no longer prints the command with `global.eco.apiKey`; the key is
  masked in helm and ssh output.
- `kube deploy --help` / `-h` prints usage instead of looking up a target named `--help`.

## v0.0.3 — 2026-06-25

### Added

- **ECO config center integration** (`internal/eco`). API client that
  drives eco-server over SSH curl against its clusterIP — no
  port-forward, no local network path needed. Auto login
  (`/auth/token`, `accessToken`), auto re-login on 401.
  - **L2 `E`** lists every registered application (name / status /
    version / id).
  - **L3 `e`** opens the selected service's full ECO config. `E` edits
    the JSON in `$EDITOR` and PUTs it back (confirm-gated, JSON
    validated, no-op if unchanged). `D` deletes the ECO registration
    (confirm-gated; the deployment itself is untouched).
- **Guided mode.** `g` on L2/L3 opens a copy-paste command sheet with
  the *actual* host / namespace / service / ECO key values filled in:
  ssh, discovery, build & push, the full OCI `helm upgrade --install`
  with all `--set` flags, tag-only `--reuse-values` update, rollout
  watching, and the complete ECO curl chain. Learn-by-doing companion
  to every TUI action.
- **`wedakube-ops` Claude skill** (`.claude/skills/wedakube-ops/`).
  Deploy pipeline, rollout-timeout diagnosis (CrashLoopBackOff → ECO
  404 root cause), ECO API recipes, manual helm procedures.

- **Namespace picker.** `N` (L2) and the wizard's namespace step now
  list the cluster's actual namespaces to pick from (● = current,
  🔒 = protected/read-only) instead of free-text input. `i` falls back
  to manual typing for a namespace that doesn't exist yet.

### Improved

- **L3 deploy hints** now catch one more failure ahead of time: repo
  has `appcfg.yaml` but the host's Helm params were never detected →
  explicit warning instead of a mid-deploy error.
- **ECO login resilience.** Helm-param auto-detect now samples only
  *ready* deployments (a CrashLooping service is often the one whose
  key is wrong), and ECO login falls back to trying keys from other
  running services in the namespace when the cached key is rejected
  (`record not found`), adopting the first key that works.

## v0.0.2 — 2026-06-05

### Fixed

- **L1 `n` new host** finally does something. The key was advertised
  in the footer but had no handler — pressing it was a no-op. Replaced
  with a proper multi-field form (`Name / HostName / User /
  IdentityFile / Namespace`) that validates required fields, rejects
  name collisions, and persists to `~/.k3sdeploy/state.json`.

### Added

- **L1 `e` edit host** opens the same form pre-filled, supporting
  rename while preserving the host's cached `HelmParams` so you don't
  need to `R refresh-helm` after editing.
- `hostsReloadMsg` fires on `HostsScreen.Init`, so popping back from
  the form refreshes the host list immediately.

## v0.0.1 — 2026-06-05

First taggable release. Multi-host k3s ops console + build-push-deploy
pipeline, verified end-to-end on a fresh .NET service.

### Features

- **L1 host list.** Multi-host, one connection per host via SSH
  ControlMaster. First connect auto-detects the cluster's Helm params
  (`R` re-probes when the tenant flips or the ECO API key rotates).
  `S` opens a setup screen for the private-NuGet `FEED_PAT`,
  stored at `~/.k3sdeploy/secrets.json` (`0600`).
- **L2 service list.** k9s-style table at full terminal height; auto
  refresh + `r` manual. `a` opens a smart directory-tree wizard that
  marks repos with `appcfg.yaml` or `Dockerfile` and auto-fills service
  name / image / port / version from the repo. `d` unpins a deploy
  target (state-only, never touches the cluster). `N` edits the host's
  namespace inline.
- **L3 service detail.** AGE column on the list, summary on the detail,
  grouped action menu:
  - **Inspect** (read-only) `s` status · `i` describe · `l` live tail
    logs (follow toggle + scrollback) · `u` resource · `k` networking.
  - **Lifecycle** (confirm-modal-gated) `D` deploy · `R` restart ·
    `+` start (scale 1) · `-` stop (scale 0) · `z` rollback (helm or
    rollout depending on management).
  - **Debug** `x` exec into pod · `w` swagger via NodePort patch with
    auto-revert on `esc`.
- **Deploy auto-dispatch.** When the repo has `appcfg.yaml` or the
  cluster already has a Helm release for the service, the deploy
  pipeline switches to the official path: scp `appcfg.yaml` → `helmster
  gen` → `helm upgrade --install` with all 9 host-cached Helm `--set`
  flags. Otherwise the M2 hand-rolled manifest path runs.
- **Cross-arch.** `docker buildx build --platform linux/amd64 --push`
  is enforced; an arm64 laptop cannot push an arm64 image by accident.
- **Safety.** All write actions print the exact kubectl/helm command
  before `y` runs them. Protected namespaces (`kube-system`,
  `wise-system`, `wise-backing-service`, `wise-observability`) lock
  every write key. No cluster-resource delete; only the tool's own
  host/pin config can be removed.
- **D prereq gate.** `D deploy` stays grey unless the bound repo has
  both `Dockerfile` and `appcfg.yaml`.

### Tooling

- `make build / install / uninstall / cross / clean` (single-binary,
  `CGO_ENABLED=0`, signed with `codesign -` on macOS to avoid Gatekeeper).
- `kube version` derives from `git describe --tags --dirty`.
- `make audit` scans working tree (gitignore-aware) + commit messages +
  patches for likely-sensitive strings (UUIDs, API-key shapes, plus
  optional regex patterns from a gitignored `.audit-patterns`).
- `make install-hooks` symlinks `.git/hooks/pre-push` to
  `scripts/pre-push`, which runs the audit and blocks the push on a
  leak — so internal cluster details can't accidentally land in a
  public remote.

### Verified end-to-end

Deployed a fresh .NET 10 service (`mini`) from `~/advantech/playground/mini`
all the way to `Ready 1/1` on the dev cluster:

```
wizard → buildx linux/amd64 → push <REGISTRY> →
scp appcfg.yaml → ssh helmster gen → ssh helm upgrade --install →
Application CRD → ECO registers → Deployment rolls out → /health 200
```

Then exercised `l` tail logs, `w` swagger (NodePort, esc reverts),
`x` shell into pod — all working through the same ControlMaster SSH
session.
