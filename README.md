SOP — Accessing & Managing the WEDA Application Deployment on k3s
=================================================================

**Audience:** Developers / DevOps
**Scope:** The `dev` k3s cluster hosting the EdgeSync / WEDA microservices (Container Management, Device Management, etc.)
**Last verified:** 2026-06-01

* * *

1. Purpose
----------

This procedure describes how a developer logs in to the k3s server and uses `kubectl`
to **inspect** and **manage** the deployment status of the WEDA application services
(e.g. `container-management`). It is read-and-operate guidance — destructive actions
are flagged explicitly.

* * *

2. Prerequisites
----------------

| Item | Value |
| --- | --- |
| SSH alias | `my-cluster` |
| Node host | `10.0.0.1` (`dev-k3s`, control-plane + etcd) |
| SSH user | `ubuntu` |
| SSH key | `~/.ssh/private.key` (entry already in `~/.ssh/config`) |
| k3s version | `v1.34.3+k3s1` |
| App namespace | `<TENANT_ID>-weda` (the org/tenant UUID + `-weda`) |

> The namespace UUID is the tenant id. Confirm yours with `kubectl get ns` — the WEDA
> app namespace is the one ending in `-weda`.

`kubectl` is **not installed on your laptop** — it lives on the cluster node. You run it
over SSH (Section 4).

### Confirm SSH access

    ssh my-cluster 'echo CONNECTED && kubectl version'


> On k3s v1.34+ the `--short` flag has been removed — use plain `kubectl version`
> (add `-o json` if you want to parse it).

If you get `Permission denied`, request your public key be added to the node and confirm
your `~/.ssh/config` has:

    host my-cluster
      HostName 10.0.0.1
      user ubuntu
      IdentityFile ~/.ssh/private.key


* * *

3. Login
--------

    ssh my-cluster


Once on the node, set convenience variables for the session (saves typing the long namespace):

    export NS=<TENANT_ID>-weda
    export APP=container-management                 # change to the service you are working on
    export SEL=app.kubernetes.io/name=$APP          # Helm label selector for that service


> **Label selector:** these are Helm charts — the pods are labelled
> `app.kubernetes.io/name=<service>`, **not** `app=<service>`. A bare `-l app=container-management`
> returns _No resources found_. Always select with `$SEL` (i.e. `app.kubernetes.io/name=...`).

> **k3s note:** `kubectl` on the node reads `/etc/rancher/k3s/k3s.yaml`. The `ubuntu` user
> is already configured. If you ever see `permission denied` reading the kubeconfig, prefix
> with `sudo k3s kubectl ...` instead of `kubectl ...`.

* * *

4. Run kubectl without an interactive login (optional)
------------------------------------------------------

For quick one-off checks from your laptop, wrap the command in SSH:

    ssh my-cluster \
      'kubectl -n <TENANT_ID>-weda get pods'


* * *

5. Inspect deployment status (read-only — safe)
-----------------------------------------------

### 5.1 Cluster health

    kubectl get nodes -o wide                 # node Ready? version?
    kubectl get ns                            # list namespaces


### 5.2 All WEDA app services at a glance

    export NS=$(kubectl get ns --no-headers -o custom-columns=NAME:.metadata.name | grep -- '-weda$' | head -1)
    echo "$NS"
    kubectl -n $NS get deploy


Expected (15 services, all `1/1`):

    authinfo-mgr           container-management   custom-devices
    datapoints             device-credentials     device-management
    device-shadows         iam                    iotdb-writer
    notification-srp       reports                telemetry
    transceivers           tunnel-management      weda-mui


### 5.3 Status of one service

    kubectl -n $NS get deploy,rs,pod -l $SEL -o wide
    kubectl -n $NS describe deploy/$APP          # events, image, probes, restart reasons


`READY 1/1` and pod `Running` = healthy. A non-zero **RESTARTS** count or
`CrashLoopBackOff` / `ImagePullBackOff` needs investigation (Section 8).

### 5.4 Which image / version is running

    kubectl -n $NS get deploy/$APP \
      -o jsonpath='{.spec.template.spec.containers[*].image}{"\n"}'


### 5.5 Logs

    kubectl -n $NS logs deploy/$APP --tail=200          # recent (app pod only)
    kubectl -n $NS logs deploy/$APP -f                  # live follow (Ctrl-C to stop)
    kubectl -n $NS logs deploy/$APP --previous          # logs from a crashed/restarted instance


> Use `deploy/$APP` (not `-l $SEL`) for logs: the `app.kubernetes.io/name` label also matches
> the one-shot `*-db-migrator` job pod, so a label selector can return migration logs instead
> of the running app.

### 5.6 Resource usage

    kubectl -n $NS top pod -l $SEL                      # needs metrics-server


### 5.7 Services / networking

    kubectl -n $NS get svc                              # ClusterIPs & ports (app port = 44327)


* * *

6. Manage the deployment (changes — use with care)
--------------------------------------------------

> **Before any change:** confirm the correct `$NS` and `$APP`, and announce in the team
> channel if this is a shared dev environment.

### 6.1 Restart (rolling — recreates the pod, no spec change)

    kubectl -n $NS rollout restart deploy/$APP
    kubectl -n $NS rollout status  deploy/$APP          # waits until the new pod is Ready


### 6.2 Stop / start (scale to 0, then back to 1)

    kubectl -n $NS scale deploy/$APP --replicas=0        # stop
    kubectl -n $NS scale deploy/$APP --replicas=1        # start


### 6.3 Update image (OTA / version bump)

    # Find the container name first
    kubectl -n $NS get deploy/$APP \
      -o jsonpath='{.spec.template.spec.containers[*].name}{"\n"}'

    kubectl -n $NS set image deploy/$APP <container-name>=<registry>/container-management:<tag>
    kubectl -n $NS rollout status deploy/$APP


### 6.4 Rollback to a previous revision

    kubectl -n $NS rollout history deploy/$APP           # list revisions
    kubectl -n $NS rollout undo    deploy/$APP           # back to previous
    kubectl -n $NS rollout undo    deploy/$APP --to-revision=<N>


### 6.5 Edit configuration / env live

    kubectl -n $NS edit deploy/$APP                      # opens the manifest in $EDITOR


> Live edits drift from the source-of-truth (Helm/GitOps/Azure Pipeline). Treat them as
> temporary; fold any keeper into the deployment manifest and redeploy through CI.

* * *

7. Exec & port-forward (debugging)
----------------------------------

    # Shell inside the running container
    kubectl -n $NS exec -it deploy/$APP -- sh

    # Expose the service port locally to hit its API/swagger
    kubectl -n $NS port-forward deploy/$APP 8080:44327
    # then, in another terminal on the node: curl http://localhost:8080/swagger/...


> Because kubectl runs on the node, `port-forward` binds on the node. To reach it from your
> laptop, add SSH tunneling: `ssh -L 8080:localhost:8080 my-cluster` and run the
> port-forward on the node.

* * *

8. Troubleshooting quick reference
----------------------------------

| Symptom | Command to diagnose |
| --- | --- |
| Pod not `Running` | `kubectl -n $NS describe pod -l $SEL` (read **Events**) |
| Repeated restarts | `kubectl -n $NS logs deploy/$APP --previous` |
| `ImagePullBackOff` | `kubectl -n $NS describe pod -l $SEL` → check image/registry creds |
| Pending / unschedulable | `kubectl -n $NS describe pod ...` + `kubectl get nodes`, `kubectl describe node dev-k3s` |
| DB migration job | `kubectl -n $NS get pods |
| Recent cluster events | `kubectl -n $NS get events --sort-by=.lastTimestamp |

* * *

9. Safety rules (Do / Don't)
----------------------------

**Do**
*   Default to read-only commands (Section 5).
*   Use `rollout status` after every change to confirm the new pod is healthy.
*   Keep the source-of-truth (CI pipeline / GitOps) as the real way to deploy.
**Don't**
*   `kubectl delete` deployments/namespaces — recovery requires a redeploy.
*   Edit `kube-system`, `wise-system`, `wise-backing-service` (NATS), or `wise-observability`
    namespaces unless you own them.
*   Make live `edit`/`set image` changes on shared environments without telling the team.
*   Hard-code or share the kubeconfig / SSH private key.

* * *

10. Appendix — Cluster facts (as of 2026-06-01)
-----------------------------------------------

*   **Node:** `dev-k3s` — `10.0.0.1`, Ubuntu 24.04.2, k3s `v1.34.3+k3s1`, runtime `containerd 2.1.5-k3s1`
*   **App namespace:** `<TENANT_ID>-weda`
*   **App service port:** `44327/TCP` (ClusterIP) for all `*-management` / data services
*   **Message bus:** `nats-bus` (LoadBalancer `10.0.0.3:4222`) in `wise-backing-service`
*   **UI:** `weda-mui` service on port `5009`
*   **Other namespaces:** `wise-system`, `wise-backing-service`, `wise-observability`, `kube-system`
