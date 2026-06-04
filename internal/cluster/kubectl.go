package cluster

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/intervalrain/wedakube/internal/config"
	"github.com/intervalrain/wedakube/internal/model"
)

// Kubectl 透過 SSH 在 node 上跑 kubectl，把結果轉成 domain model。
type Kubectl struct {
	ssh *SSH
	ns  string
}

func NewKubectl(ssh *SSH, ns string) *Kubectl {
	return &Kubectl{ssh: ssh, ns: ns}
}

func (k *Kubectl) Namespace() string { return k.ns }

// SetNamespace 讓 L2 在 user 編輯 host.Namespace 後可以原地切換 ns 而不用重建 kubectl。
func (k *Kubectl) SetNamespace(ns string) { k.ns = ns }

// SSH 讓上層（部署流程）取用底層連線。
func (k *Kubectl) SSH() *SSH { return k.ssh }

// ResolveWedaNamespace 找出結尾為 -weda 的 tenant namespace（README 的作法）。
func ResolveWedaNamespace(ctx context.Context, ssh *SSH) (string, error) {
	out, err := ssh.Run(ctx, `kubectl get ns -o jsonpath='{range .items[*]}{.metadata.name}{"\n"}{end}'`)
	if err != nil {
		return "", err
	}
	for _, line := range strings.Split(string(out), "\n") {
		name := strings.TrimSpace(line)
		if strings.HasSuffix(name, "-weda") {
			return name, nil
		}
	}
	return "", fmt.Errorf("no namespace ending in -weda found")
}

// deployList 只映射我們需要的 deploy JSON 子集。
type deployList struct {
	Items []struct {
		Metadata struct {
			Name              string `json:"name"`
			CreationTimestamp string `json:"creationTimestamp"`
		} `json:"metadata"`
		Spec struct {
			Template struct {
				Spec struct {
					Containers []struct {
						Image string `json:"image"`
					} `json:"containers"`
				} `json:"spec"`
			} `json:"template"`
		} `json:"spec"`
		Status struct {
			Replicas        int `json:"replicas"`
			ReadyReplicas   int `json:"readyReplicas"`
			UpdatedReplicas int `json:"updatedReplicas"`
		} `json:"status"`
	} `json:"items"`
}

// Deployments 回傳 namespace 內所有 deployment 的精簡視圖。
func (k *Kubectl) Deployments(ctx context.Context) ([]model.Service, error) {
	out, err := k.ssh.Run(ctx, fmt.Sprintf("kubectl -n %s get deploy -o json", k.ns))
	if err != nil {
		return nil, err
	}

	var list deployList
	if err := json.Unmarshal(out, &list); err != nil {
		return nil, fmt.Errorf("parse deploy json: %w", err)
	}

	svcs := make([]model.Service, 0, len(list.Items))
	for _, it := range list.Items {
		image := ""
		if len(it.Spec.Template.Spec.Containers) > 0 {
			image = it.Spec.Template.Spec.Containers[0].Image
		}
		svcs = append(svcs, model.Service{
			Name:     it.Metadata.Name,
			Ready:    fmt.Sprintf("%d/%d", it.Status.ReadyReplicas, it.Status.Replicas),
			UpToDate: it.Status.UpdatedReplicas,
			Age:      humanAge(it.Metadata.CreationTimestamp),
			Image:    image,
		})
	}
	sort.Slice(svcs, func(i, j int) bool { return svcs[i].Name < svcs[j].Name })
	return svcs, nil
}

// SampleHelmParams 從現役服務的 env / image 推導 cluster 級 helm 參數。
// 第一次連 host 時呼叫一次就好，省下手填 9 個 --set。
func (k *Kubectl) SampleHelmParams(ctx context.Context) (config.HelmParams, error) {
	var hp config.HelmParams
	out, err := k.ssh.Run(ctx, fmt.Sprintf("kubectl -n %s get deploy -o json", k.ns))
	if err != nil {
		return hp, err
	}
	var list struct {
		Items []struct {
			Spec struct {
				Template struct {
					Spec struct {
						Containers []struct {
							Image string `json:"image"`
							Env   []struct {
								Name  string `json:"name"`
								Value string `json:"value"`
							} `json:"env"`
						} `json:"containers"`
					} `json:"spec"`
				} `json:"template"`
			} `json:"spec"`
		} `json:"items"`
	}
	if err := json.Unmarshal(out, &list); err != nil {
		return hp, fmt.Errorf("parse deploy json: %w", err)
	}

	for _, it := range list.Items {
		if len(it.Spec.Template.Spec.Containers) == 0 {
			continue
		}
		c := it.Spec.Template.Spec.Containers[0]

		got := map[string]string{}
		for _, e := range c.Env {
			got[e.Name] = e.Value
		}
		// 認得出來才採用（避免拿到 db-migrator 之類沒 ECO 的 deploy）
		if got["ECO_API_KEY"] == "" {
			continue
		}

		hp.TenantID = got["TENANT_ID"]
		hp.TenantPath = got["TENANT_NAME"]
		hp.TenantAlias = got["TENANT_ALIAS"]
		hp.SrpName = got["SRP_NAME"]
		hp.Domain = got["DOMAIN"]
		hp.EcoEndpoint = got["ECO_API_ENDPOINT"]
		hp.EcoApiKey = got["ECO_API_KEY"]

		// image 形如 harbor.../edge-coa/<svc>:<tag> -> 拆 registry / project
		if i1 := strings.Index(c.Image, "/"); i1 > 0 {
			hp.Registry = c.Image[:i1]
			if rest := c.Image[i1+1:]; len(rest) > 0 {
				if i2 := strings.Index(rest, "/"); i2 > 0 {
					hp.Project = rest[:i2]
				}
			}
		}
		return hp, nil
	}
	return hp, fmt.Errorf("no sample deployment with ECO_API_KEY in ns %s", k.ns)
}

// PatchServiceType 把 svc 切到 NodePort（暫時暴露）或還原 ClusterIP。
func (k *Kubectl) PatchServiceType(ctx context.Context, svc, svcType string) (string, error) {
	body := fmt.Sprintf(`{"spec":{"type":"%s"}}`, svcType)
	out, err := k.ssh.Run(ctx, fmt.Sprintf("kubectl -n %s patch svc/%s -p '%s' 2>&1", k.ns, svc, body))
	if err != nil {
		return string(out), fmt.Errorf("patch svc/%s -> %s: %w", svc, svcType, err)
	}
	return string(out), nil
}

// NodePort 讀 svc 第一個 port 分配到的 nodePort（NodePort 模式下才有值）。
func (k *Kubectl) NodePort(ctx context.Context, svc string) (int, error) {
	out, err := k.ssh.Run(ctx, fmt.Sprintf(
		"kubectl -n %s get svc/%s -o jsonpath='{.spec.ports[0].nodePort}'", k.ns, svc))
	if err != nil {
		return 0, err
	}
	s := strings.TrimSpace(string(out))
	if s == "" {
		return 0, fmt.Errorf("no nodePort allocated for svc/%s", svc)
	}
	return strconv.Atoi(s)
}

// NodeIP 從 kubectl get nodes 拿第一個 node 的 InternalIP（給組瀏覽器 URL 用）。
func (k *Kubectl) NodeIP(ctx context.Context) (string, error) {
	out, err := k.ssh.Run(ctx, `kubectl get nodes -o jsonpath='{.items[0].status.addresses[?(@.type=="InternalIP")].address}'`)
	if err != nil {
		return "", err
	}
	s := strings.TrimSpace(string(out))
	if s == "" {
		return "", fmt.Errorf("no InternalIP found")
	}
	return s, nil
}

// ContainerPort 回傳該 deployment 第一個容器的 containerPort（給 swagger / port-forward 預設用）。
func (k *Kubectl) ContainerPort(ctx context.Context, service string) (int, error) {
	out, err := k.ssh.Run(ctx, fmt.Sprintf(
		"kubectl -n %s get deploy/%s -o jsonpath='{.spec.template.spec.containers[0].ports[0].containerPort}'",
		k.ns, service))
	if err != nil {
		return 0, err
	}
	s := strings.TrimSpace(string(out))
	if s == "" {
		return 0, fmt.Errorf("no containerPort declared on deploy/%s", service)
	}
	return strconv.Atoi(s)
}

// HelmReleaseFor 從 deployment annotation 偵測它是哪個 helm release 裝的。
// 空字串 = 不是 helm 管的（不該 uninstall）。
func (k *Kubectl) HelmReleaseFor(ctx context.Context, service string) (release, namespace string, err error) {
	out, err := k.ssh.Run(ctx, fmt.Sprintf("kubectl -n %s get deploy/%s -o json", k.ns, service))
	if err != nil {
		return "", "", err
	}
	var d struct {
		Metadata struct {
			Annotations map[string]string `json:"annotations"`
		} `json:"metadata"`
	}
	if err := json.Unmarshal(out, &d); err != nil {
		return "", "", err
	}
	return d.Metadata.Annotations["meta.helm.sh/release-name"],
		d.Metadata.Annotations["meta.helm.sh/release-namespace"], nil
}

// HelmUninstall 在 node 上跑 helm uninstall。Application CRD 因為 resource-policy:keep 會留下。
func (k *Kubectl) HelmUninstall(ctx context.Context, release, namespace string) (string, error) {
	out, err := k.ssh.Run(ctx, fmt.Sprintf("helm uninstall %s -n %s 2>&1", release, namespace))
	return string(out), err
}

// HelmRollback 退一個 release 到上一個 revision。
func (k *Kubectl) HelmRollback(ctx context.Context, release, namespace string) (string, error) {
	out, err := k.ssh.Run(ctx, fmt.Sprintf("helm rollback %s -n %s 2>&1", release, namespace))
	return string(out), err
}

// RolloutRestart 觸發 rolling restart（README §6.1）。
func (k *Kubectl) RolloutRestart(ctx context.Context, service string) (string, error) {
	out, err := k.ssh.Run(ctx, fmt.Sprintf("kubectl -n %s rollout restart deploy/%s 2>&1", k.ns, service))
	return string(out), err
}

// Scale 設定 deployment 的副本數（0 = stop, 1 = start）。
func (k *Kubectl) Scale(ctx context.Context, service string, replicas int) (string, error) {
	out, err := k.ssh.Run(ctx, fmt.Sprintf("kubectl -n %s scale deploy/%s --replicas=%d 2>&1", k.ns, service, replicas))
	return string(out), err
}

// RolloutUndo 退回 deployment 上一個 revision（非 helm-managed 用）。
func (k *Kubectl) RolloutUndo(ctx context.Context, service string) (string, error) {
	out, err := k.ssh.Run(ctx, fmt.Sprintf("kubectl -n %s rollout undo deploy/%s 2>&1", k.ns, service))
	return string(out), err
}

// Raw 跑一條 kubectl 子指令（已帶 -n <ns>），回傳合併 stdout/stderr 文字。供 L3 唯讀檢視用。
func (k *Kubectl) Raw(ctx context.Context, args string) (string, error) {
	out, err := k.ssh.Run(ctx, fmt.Sprintf("kubectl -n %s %s 2>&1", k.ns, args))
	return string(out), err
}

// humanAge 把 RFC3339 建立時間轉成 kubectl 風格年齡（34d / 2h / 15m）。
func humanAge(ts string) string {
	t, err := time.Parse(time.RFC3339, ts)
	if err != nil {
		return "?"
	}
	d := time.Since(t)
	switch {
	case d < time.Minute:
		return fmt.Sprintf("%ds", int(d.Seconds()))
	case d < time.Hour:
		return fmt.Sprintf("%dm", int(d.Minutes()))
	case d < 24*time.Hour:
		return fmt.Sprintf("%dh", int(d.Hours()))
	default:
		return fmt.Sprintf("%dd", int(d.Hours())/24)
	}
}
