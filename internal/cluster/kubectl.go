package cluster

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

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
			Name string `json:"name"`
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
			Image:    image,
		})
	}
	sort.Slice(svcs, func(i, j int) bool { return svcs[i].Name < svcs[j].Name })
	return svcs, nil
}
