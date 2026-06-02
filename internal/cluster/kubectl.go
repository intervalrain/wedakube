package cluster

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

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
