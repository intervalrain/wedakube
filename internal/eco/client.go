// Package eco 是 ECO config center（eco-server）的 API client。
// 所有請求都透過 SSH 在 node 上用 curl 打 eco-server 的 clusterIP —
// node 可以直接路由 service CIDR，所以不需要 port-forward，也不需要本機網路能通。
package eco

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"github.com/intervalrain/wedakube/internal/cluster"
)

// App 是 /v1/applications 回傳的一筆應用程式。
type App struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	TenantID  string `json:"tenantId"`
	SrpName   string `json:"srpName"`
	Version   int    `json:"version"`
	Status    string `json:"status"`
	UpdatedAt string `json:"updatedAt"`
}

// Client 帶著一組 apiKey/srpName，lazy 解析 endpoint、lazy 換 token。
type Client struct {
	ssh     *cluster.SSH
	APIKey  string
	SrpName string

	fallbackNS string // 非空 = login 失敗時，從這個 ns 現役服務撈候選 key 重試
	endpoint   string // http://<clusterIP>:<port>，第一次用到時解析
	token      string
}

func New(ssh *cluster.SSH, apiKey, srpName string) *Client {
	if srpName == "" {
		srpName = "weda" // dev cluster 的預設 SRP
	}
	return &Client{ssh: ssh, APIKey: apiKey, SrpName: srpName}
}

// Endpoint 解析 eco-server svc 的 clusterIP:port。
// node 上 *.svc.cluster.local 的 DNS 不通，所以要用 IP 直打。
func (c *Client) Endpoint(ctx context.Context) (string, error) {
	if c.endpoint != "" {
		return c.endpoint, nil
	}
	out, err := c.ssh.Run(ctx,
		"kubectl -n wise-system get svc eco-server -o jsonpath='{.spec.clusterIP}:{.spec.ports[0].port}'")
	if err != nil {
		return "", fmt.Errorf("resolve eco-server svc: %w", err)
	}
	hostport := strings.TrimSpace(string(out))
	if hostport == "" || strings.HasPrefix(hostport, ":") {
		return "", fmt.Errorf("eco-server svc not found in wise-system")
	}
	c.endpoint = "http://" + hostport
	return c.endpoint, nil
}

// SetFallbackNamespace 讓 Login 在主 key 失敗（例如 record not found）時，
// 自動從該 ns「跑得起來的」服務撈其他 ECO_API_KEY 逐一重試。
func (c *Client) SetFallbackNamespace(ns string) { c.fallbackNS = ns }

// Login 用 apiKey+srpName 換 accessToken（POST /auth/token）。
// cached key 可能來自一個 key 本身就是錯的服務（CrashLoop 中的那種），
// 所以失敗時會嘗試 fallback ns 裡現役服務的 key，成功就換用它。
func (c *Client) Login(ctx context.Context) error {
	if c.APIKey == "" && c.fallbackNS == "" {
		return fmt.Errorf("no ECO API key on this host — reconnect the host once so helm params get auto-detected")
	}
	primaryErr := c.loginWith(ctx, c.APIKey)
	if primaryErr == nil {
		return nil
	}
	if c.fallbackNS == "" {
		return primaryErr
	}
	keys, err := c.discoverKeys(ctx)
	if err != nil {
		return fmt.Errorf("%w (fallback key discovery also failed: %v)", primaryErr, err)
	}
	for _, k := range keys {
		if k == c.APIKey {
			continue
		}
		if err := c.loginWith(ctx, k); err == nil {
			c.APIKey = k // 這把有效，之後都用它
			return nil
		}
	}
	return fmt.Errorf("%w (tried %d fallback keys from running services in %s — all rejected)",
		primaryErr, len(keys), c.fallbackNS)
}

func (c *Client) loginWith(ctx context.Context, apiKey string) error {
	if apiKey == "" {
		return fmt.Errorf("empty api key")
	}
	body, _ := json.Marshal(map[string]string{"apiKey": apiKey, "srpName": c.SrpName})
	code, out, err := c.call(ctx, "POST", "/auth/token", body, false)
	if err != nil {
		return err
	}
	if code != 200 {
		return fmt.Errorf("eco login HTTP %d: %s", code, firstN(strings.TrimSpace(string(out)), 200))
	}
	var r struct {
		AccessToken string `json:"accessToken"`
	}
	if err := json.Unmarshal(out, &r); err != nil || r.AccessToken == "" {
		return fmt.Errorf("eco login: unexpected response: %s", firstN(string(out), 200))
	}
	c.token = r.AccessToken
	return nil
}

// discoverKeys 從 fallback ns 撈現役（ready>0）服務的 ECO_API_KEY 清單（去重）。
func (c *Client) discoverKeys(ctx context.Context) ([]string, error) {
	out, err := c.ssh.Run(ctx, fmt.Sprintf("kubectl -n %s get deploy -o json", c.fallbackNS))
	if err != nil {
		return nil, err
	}
	var list struct {
		Items []struct {
			Spec struct {
				Template struct {
					Spec struct {
						Containers []struct {
							Env []struct {
								Name  string `json:"name"`
								Value string `json:"value"`
							} `json:"env"`
						} `json:"containers"`
					} `json:"spec"`
				} `json:"template"`
			} `json:"spec"`
			Status struct {
				ReadyReplicas int `json:"readyReplicas"`
			} `json:"status"`
		} `json:"items"`
	}
	if err := json.Unmarshal(out, &list); err != nil {
		return nil, err
	}
	seen := map[string]bool{}
	var keys []string
	for _, it := range list.Items {
		if it.Status.ReadyReplicas == 0 || len(it.Spec.Template.Spec.Containers) == 0 {
			continue
		}
		for _, e := range it.Spec.Template.Spec.Containers[0].Env {
			if e.Name == "ECO_API_KEY" && e.Value != "" && !seen[e.Value] {
				seen[e.Value] = true
				keys = append(keys, e.Value)
			}
		}
	}
	return keys, nil
}

// ListApps 列出這個 tenant/srp 下所有 registered app。
func (c *Client) ListApps(ctx context.Context) ([]App, error) {
	out, err := c.authed(ctx, "GET", "/v1/applications", nil)
	if err != nil {
		return nil, err
	}
	return parseApps(out)
}

// AppByName 用名字查一個 app（拿 ID 用）。
func (c *Client) AppByName(ctx context.Context, name string) (App, error) {
	out, err := c.authed(ctx, "GET", "/v1/applications?name="+name, nil)
	if err != nil {
		return App{}, err
	}
	apps, err := parseApps(out)
	if err != nil {
		return App{}, err
	}
	if len(apps) == 0 {
		return App{}, fmt.Errorf("app %q not registered in ECO (srp=%s)", name, c.SrpName)
	}
	return apps[0], nil
}

// AppDetail 抓單一 app 的完整 config，回傳縮排後的 JSON 文字。
func (c *Client) AppDetail(ctx context.Context, id string) (string, error) {
	out, err := c.authed(ctx, "GET", "/v1/applications/"+id, nil)
	if err != nil {
		return "", err
	}
	return prettyJSON(out), nil
}

// UpdateApp 把編輯後的 JSON PUT 回去。
func (c *Client) UpdateApp(ctx context.Context, id string, body []byte) (string, error) {
	if !json.Valid(body) {
		return "", fmt.Errorf("edited content is not valid JSON — aborted, nothing sent")
	}
	out, err := c.authed(ctx, "PUT", "/v1/applications/"+id, body)
	return prettyJSON(out), err
}

// DeleteApp 刪掉一個 registered app。⚠ 服務本身（deployment）不會被刪。
func (c *Client) DeleteApp(ctx context.Context, id string) (string, error) {
	out, err := c.authed(ctx, "DELETE", "/v1/applications/"+id, nil)
	return strings.TrimSpace(string(out)), err
}

// authed 確保有 token 後呼叫；401 時自動重新 login 一次（token 過期）。
func (c *Client) authed(ctx context.Context, method, path string, body []byte) ([]byte, error) {
	if c.token == "" {
		if err := c.Login(ctx); err != nil {
			return nil, err
		}
	}
	code, out, err := c.call(ctx, method, path, body, true)
	if err != nil {
		return out, err
	}
	if code == 401 { // token 過期 → 換一次再試
		if err := c.Login(ctx); err != nil {
			return nil, err
		}
		code, out, err = c.call(ctx, method, path, body, true)
		if err != nil {
			return out, err
		}
	}
	if code < 200 || code >= 300 {
		return out, fmt.Errorf("eco %s %s HTTP %d: %s", method, path, code, firstN(strings.TrimSpace(string(out)), 300))
	}
	return out, nil
}

// call 在 node 上跑一次 curl。body 走 stdin（-d @-）避免 shell quoting 問題。
// -w 在最後多印一行 http status，splitStatus 再拆開。
func (c *Client) call(ctx context.Context, method, path string, body []byte, auth bool) (int, []byte, error) {
	ep, err := c.Endpoint(ctx)
	if err != nil {
		return 0, nil, err
	}
	cmd := "curl -sS -X " + method + " -H 'Content-Type: application/json'"
	if auth {
		cmd += " -H 'Authorization: Bearer " + c.token + "'"
	}
	if body != nil {
		cmd += " -d @-"
	}
	cmd += ` -w '\n%{http_code}' '` + ep + path + `'`

	var out []byte
	if body != nil {
		out, err = c.ssh.RunStdin(ctx, cmd, body)
	} else {
		out, err = c.ssh.Run(ctx, cmd)
	}
	if err != nil {
		return 0, out, err
	}
	return splitStatus(out)
}

// splitStatus 把 curl 輸出的最後一行（http status）跟 body 拆開。
func splitStatus(out []byte) (int, []byte, error) {
	s := strings.TrimRight(string(out), "\n")
	i := strings.LastIndexByte(s, '\n')
	statusStr := strings.TrimSpace(s[i+1:]) // i == -1 時取整串
	code, err := strconv.Atoi(statusStr)
	if err != nil {
		return 0, []byte(s), fmt.Errorf("parse http status from curl output: %q", firstN(s, 120))
	}
	if i < 0 {
		return code, nil, nil
	}
	return code, []byte(s[:i]), nil
}

// parseApps 同時支援 {"data":[...]} 與裸陣列兩種格式。
func parseApps(out []byte) ([]App, error) {
	var wrapped struct {
		Data []App `json:"data"`
	}
	if err := json.Unmarshal(out, &wrapped); err == nil && wrapped.Data != nil {
		return wrapped.Data, nil
	}
	var plain []App
	if err := json.Unmarshal(out, &plain); err == nil {
		return plain, nil
	}
	return nil, fmt.Errorf("parse apps: unexpected response: %s", firstN(string(out), 200))
}

func prettyJSON(b []byte) string {
	var buf strings.Builder
	dec := json.NewDecoder(strings.NewReader(string(b)))
	dec.UseNumber()
	var v any
	if err := dec.Decode(&v); err != nil {
		return string(b) // 不是 JSON 就原樣顯示
	}
	enc := json.NewEncoder(&buf)
	enc.SetIndent("", "  ")
	enc.SetEscapeHTML(false)
	if err := enc.Encode(v); err != nil {
		return string(b)
	}
	return strings.TrimRight(buf.String(), "\n")
}

func firstN(s string, n int) string {
	if len(s) > n {
		return s[:n] + "…"
	}
	return s
}
