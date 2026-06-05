# Changelog

All notable changes are kept here. Tags follow [SemVer](https://semver.org)
via `git describe --tags --dirty`.

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
