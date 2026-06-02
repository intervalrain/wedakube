package config

// Target 描述一個 repo 要部署成什麼（一個 pin 綁定的部署目標）。
type Target struct {
	RepoPath    string `json:"repoPath"`    // 本地 repo 路徑（state 的 key）
	Host        string `json:"host"`        // 部署到哪台 host（Host.Name）
	Service     string `json:"service"`     // k8s deployment / container 名
	Namespace   string `json:"namespace"`   // 部署到哪個 ns
	ImageRepo   string `json:"imageRepo"`   // harbor.../edge-coa/weda_file_transfer
	VersionBase string `json:"versionBase"` // v0.1.0
	Dockerfile  string `json:"dockerfile"`  // build 用的 Dockerfile 路徑
	Context     string `json:"context"`     // docker build context
	Port        int    `json:"port"`        // 容器埠

	// SSHAlias 為舊版相容欄位：Host 為空時用它建一個 alias-only 連線。
	SSHAlias string `json:"sshAlias,omitempty"`
}
