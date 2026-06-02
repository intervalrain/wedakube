package deploy

// Target 描述一個 repo 要部署成什麼。state 存在 ~/.k3sdeploy/state.json。
type Target struct {
	RepoPath    string `json:"repoPath"`    // 本地 repo 路徑（state 的 key）
	Service     string `json:"service"`     // k8s deployment / container 名
	Namespace   string `json:"namespace"`   // 部署到哪個 ns
	ImageRepo   string `json:"imageRepo"`   // harbor.../edge-coa/weda_file_transfer
	VersionBase string `json:"versionBase"` // v0.1.0
	Dockerfile  string `json:"dockerfile"`  // build 用的 Dockerfile 路徑
	Context     string `json:"context"`     // docker build context
	Port        int    `json:"port"`        // 容器埠
	SSHAlias    string `json:"sshAlias"`    // ~/.ssh/config 的 alias
}
