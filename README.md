# kube

Terminal-native ops console for k3s clusters. Browse hosts and services, ship
.NET (or any) services from your laptop, debug live — without putting `kubectl`
or `kubeconfig` on your machine.

## What it does

- **Multi-host k9s-style TUI.** Manage several k3s clusters from one screen.
  All cluster calls go over `ssh` with a multiplexed ControlMaster connection.
- **Build & deploy pipeline.** `docker buildx` (cross-arch arm64 → amd64) →
  push to registry → first-time `kubectl apply` or follow-up `kubectl set image` →
  live rollout bar driven by real replica counts → auto-rollback with captured
  logs on failure.
- **Helmster integration.** When a repo has `appcfg.yaml`, the deploy switches
  to the team's `helmster gen` + `helm upgrade --install` path so Application
  CRD + config-center registration happens correctly.
- **Inspect group.** `s` status, `i` describe, `l` follow logs (tail -f over
  ssh), `u` resource (`top`), `k` networking (`get svc`).
- **Lifecycle group with safety gates.** Confirm-before-run modal on every
  write; protected namespaces (`kube-system`, etc.) lock all writes.
- **Swagger via NodePort.** `w` flips the service to NodePort, opens your
  browser at `http://<node>:<nodePort>/swagger`, and reverts on `esc`.

## Install

```bash
make install         # builds and installs to /usr/local/bin/kube
```

Or just build it:

```bash
make build           # produces ./kube
```

## Quick start

```bash
# Set your default host (one-time; or add via state.json)
export KUBE_DEFAULT_HOST=my-cluster
export KUBE_DEFAULT_HOSTNAME=10.0.0.1
export KUBE_DEFAULT_USER=ubuntu
export KUBE_DEFAULT_KEY=~/.ssh/id_ed25519

kube
```

Inside the TUI:

```
L1 Host list  →  enter  →  L2 Services  →  enter  →  L3 Detail
                            a  add deploy target          D  deploy
                            d  unpin                      X  helm uninstall
                            N  edit namespace             w  swagger
```

Press `?` on most screens for the full key map (footer is always visible).

## Deploy prerequisites

For `D deploy` to be available on a service, the bound repo must contain
both:

- `Dockerfile` (multi-stage; targets `linux/amd64`)
- `appcfg.yaml` (helmster's app schema — name / version / port / health /
  optional backing services)

Without these, `D` is greyed out and the LIFECYCLE hint shows what's missing.

## Headless deploy

```bash
kube deploy /path/to/repo
```

Reads the same target/host config (`~/.k3sdeploy/state.json`) and streams
deploy events to stdout. Useful from CI or scripts.

## State

- `~/.k3sdeploy/state.json` — hosts, deploy targets, build counters.
- `~/.k3sdeploy/secrets.json` (`0600`) — credentials (e.g. private-NuGet PAT).

Both files live in your home; they're never committed.

## Versioning

`kube version` prints the SemVer derived from `git describe --tags --dirty`:

- on a tag, clean tree → `v0.0.1`
- on a tag with local edits → `v0.0.1-dirty`
- N commits past tag → `v0.0.1-N-gSHA`

## License

MIT.
