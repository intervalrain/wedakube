package config

import (
	"os"
	"path/filepath"
	"strings"
)

// HelmParams 是這台 host 上 helmster + helm install 要帶的 cluster 級設定。
// 第一次連線時自動從現役服務的 env 偵測，存進 Host。
type HelmParams struct {
	TenantID    string `json:"tenantId,omitempty"`
	TenantPath  string `json:"tenantPath,omitempty"`  // = TENANT_NAME
	TenantAlias string `json:"tenantAlias,omitempty"`
	SrpName     string `json:"srpName,omitempty"`
	Domain      string `json:"domain,omitempty"`
	Registry    string `json:"registry,omitempty"`     // 從 image 推
	Project     string `json:"project,omitempty"`      // 從 image 推
	EcoEndpoint string `json:"ecoEndpoint,omitempty"`
	EcoApiKey   string `json:"ecoApiKey,omitempty"`
}

// Host 是一台受管 k3s 主機的連線設定。
// 兩種模式：填了 HostName 走顯式 ssh -i/-l；只填 Alias 則沿用 ~/.ssh/config。
type Host struct {
	Name         string     `json:"name"`                   // 顯示名 / ControlMaster key（唯一）
	Alias        string     `json:"alias,omitempty"`        // 選填：~/.ssh/config 的 alias
	HostName     string     `json:"hostName,omitempty"`     // IP / 主機名
	User         string     `json:"user,omitempty"`         // ssh 使用者
	IdentityFile string     `json:"identityFile,omitempty"` // 私鑰路徑（可含 ~）
	Namespace    string     `json:"namespace,omitempty"`    // 空 = 自動推導 / 解析
	Helm         HelmParams `json:"helm,omitempty"`         // 部署/註冊用的 cluster 參數
}

// DerivedNamespace 優先用 Helm.TenantID + Helm.SrpName 推導 SRP namespace（WEDA 慣例）。
// 若沒有就回傳 Host.Namespace（呼叫端再 fallback 到自動解析）。
func (h Host) DerivedNamespace() string {
	if h.Helm.TenantID != "" && h.Helm.SrpName != "" {
		return h.Helm.TenantID + "-" + h.Helm.SrpName
	}
	return h.Namespace
}

// Dest 是 ssh 的目的地（HostName 優先，否則 Alias，否則 Name）。
func (h Host) Dest() string {
	switch {
	case h.HostName != "":
		return h.HostName
	case h.Alias != "":
		return h.Alias
	default:
		return h.Name
	}
}

// ExpandIdentity 把 IdentityFile 開頭的 ~ 展開成家目錄。
func (h Host) ExpandIdentity() string {
	p := h.IdentityFile
	if strings.HasPrefix(p, "~") {
		if home, err := os.UserHomeDir(); err == nil {
			p = filepath.Join(home, strings.TrimPrefix(p, "~"))
		}
	}
	return p
}
