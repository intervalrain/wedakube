package config

import (
	"os"
	"path/filepath"
	"strings"
)

// Host 是一台受管 k3s 主機的連線設定。
// 兩種模式：填了 HostName 走顯式 ssh -i/-l；只填 Alias 則沿用 ~/.ssh/config。
type Host struct {
	Name         string `json:"name"`                   // 顯示名 / ControlMaster key（唯一）
	Alias        string `json:"alias,omitempty"`        // 選填：~/.ssh/config 的 alias
	HostName     string `json:"hostName,omitempty"`     // IP / 主機名
	User         string `json:"user,omitempty"`         // ssh 使用者
	IdentityFile string `json:"identityFile,omitempty"` // 私鑰路徑（可含 ~）
	Namespace    string `json:"namespace,omitempty"`    // 空 = 自動解析結尾 -weda
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
