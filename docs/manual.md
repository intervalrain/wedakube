# kube 使用手冊（v0.0.3）

`kube` 是 WEDA dev k3s 的 ops console：從本機 build image、推 registry、
helm 部署到 cluster、看 log、修 config，全部不用離開終端機。

---

## 1. 安裝與啟動

```bash
cd ~/advantech/tools/kube
make install        # 或 go build -o kube . 後放進 PATH

kube                # 開 TUI
kube version        # 看版本
kube deploy <repo>  # headless 部署（見 §7）
```

第一次啟動前的前置：

1. `docker login <registry>`（buildx push 需要）
2. 私有 NuGet 的 `FEED_PAT`：進 TUI 後在 L1 按 `S` 設定（存到
   `~/.k3sdeploy/secrets.json`，權限 0600），或用環境變數 `FEED_PAT`

---

## 2. 畫面結構

三層堆疊，`enter` 往下、`esc` 往上、`ctrl+c` 隨時離開：

```
L1 Hosts ──enter──▶ L2 Services ──enter──▶ L3 Service Detail
（主機列表）         （服務列表）             （單一服務操作）
```

所有會寫 cluster 的動作都有 confirm 畫面——先秀出完整指令，按 `y` 才執行。
保護的 namespace（`kube-system`、`wise-system`、`wise-backing-service`、
`wise-observability`）一律鎖寫入。

---

## 3. L1 — Hosts（主機列表）

| Key | 動作 |
|-----|------|
| `n` | 新增 host（Name / HostName / User / IdentityFile / Namespace 表單） |
| `e` | 編輯選定的 host |
| `d` | 刪除 host |
| `enter` | 連線，進 L2。**第一次連線會自動偵測 Helm params**（tenantId、srp、registry、ECO key…），部署與 ECO 功能都靠它 |
| `R` | 重新偵測 Helm params（tenant 換了 / ECO key 輪替時用） |
| `S` | 設定 `FEED_PAT` |
| `r` | 重新整理 |

> 💡 如果 L3 出現「host helm params not detected」提示，回 L1 對該 host
> 按一次 `enter`（或 `R`）就會自動補上。

---

## 4. L2 — Services（服務列表）

k9s 風格表格，每 3 秒自動刷新。`◆` 開頭 = 已綁定本機 repo（可部署）。

| Key | 動作 |
|-----|------|
| `enter` | 進 L3 |
| `a` | **wizard：綁定本機 repo 成部署目標**。目錄瀏覽器會用 `◆` 標出含 `appcfg.yaml` / `Dockerfile` 的 repo；選定後自動從 appcfg 帶出 service 名 / image / port / version |
| `E` | **ECO：列出所有 registered app**（name / status / version / id），enter 進單一 app 的 config |
| `g` | **引導模式**：這台 host 的指令小抄（見 §8） |
| `d` | 解除綁定（只動本機設定，不碰 cluster） |
| `N` | 編輯 namespace |
| `r` | 手動刷新 |

---

## 5. L3 — Service Detail（單一服務）

### INSPECT（唯讀）

| Key | 動作 |
|-----|------|
| `s` | status（deploy / rs / pod 總覽） |
| `i` | describe deploy |
| `l` | live logs（`f` 切換 follow、可捲動） |
| `u` | top pod（資源用量） |
| `k` | get svc（網路） |

### LIFECYCLE（confirm-gated）

| Key | 動作 |
|-----|------|
| `D` | **部署**：build → push → helm/manifest → rollout（見 §6） |
| `R` | rolling restart |
| `+` / `-` | start（replicas=1）/ stop（replicas=0） |
| `z` | rollback（helm-managed 走 `helm rollback`，否則 `rollout undo`） |
| `X` | helm uninstall（只有 helm-managed 服務會亮） |

### DEBUG

| Key | 動作 |
|-----|------|
| `x` | shell 進 pod（exit 回 TUI） |
| `w` | swagger：暫時 patch 成 NodePort 開瀏覽器，**esc 自動還原 ClusterIP** |
| `e` | **ECO config**（見 §9） |
| `g` | 這個服務的指令小抄 |

---

## 6. 部署流程（按 `D` 之後發生什麼）

```
tag：{versionBase}_{YYYYMMDD}.{N}     ← N 每天自動遞增
  ↓
docker buildx build --platform linux/amd64 --push
  ↓                                   ← 本機 arm64 → node amd64，不會推錯架構
repo 有 appcfg.yaml 或已是 helm release？
  ├─ 是 → scp appcfg → helmster gen → helm upgrade --install（官方路徑，ECO 接管）
  └─ 否 → kubectl set image（既有）/ 首次 apply 最小 manifest
  ↓
等 rollout（3 分鐘 timeout）
  ├─ 成功 → ✓ deployed
  └─ 失敗 → 自動抓 describe + logs 顯示在畫面 → 自動 rollback 回前一版
```

### D 是灰色的？依提示行處理：

| 提示 | 解法 |
|------|------|
| `no pin/repo` | 回 L2 按 `a` 綁 repo |
| `repo missing: Dockerfile / appcfg.yaml` | 補檔案 |
| `host helm params not detected` | 回 L1 對 host 按 `enter` 或 `R` |
| `protected namespace` | 換 namespace（`N`） |

### rollout timeout 卡在 `ready 1/2`？

= 舊 pod 還活著、**新 pod 起不來**。失敗畫面已附 describe+logs；手動追：

```bash
kubectl -n <ns> get pod | grep <svc>              # CrashLoopBackOff?
kubectl -n <ns> logs <pod> --previous --tail=50   # crash 原因
```

最常見：dotnet 服務啟動時 ECO login 404（`ECO_API_KEY` 跟 ECO 註冊的不一致
→ `record not found`）。用 `e` 進 ECO config 比對，或重新 helm 部署帶正確的
`global.eco.apiKey`。

---

## 7. CLI 模式（CI / 腳本用）

```bash
kube ctx ls               # 列出 server，* 是目前選擇
kube ctx select <server>  # 選擇要部署的 server（唯一前綴即可）
kube deploy .             # 部署當下目錄的 repo（路徑可省略，預設 "."）
```

用的是 TUI wizard 存的同一份 target 設定，事件逐行印出。前置：
`FEED_PAT` 已設、`docker login` 過、該 repo 已在 wizard 設定過 target。
有用 `ctx select` 選 server 時，部署會覆寫 target 原本的 host，
namespace 也改用該 server 的推導值。

---

## 8. 引導模式（`g`）

不想被 TUI 包著？每一層按 `g` 就把該動作背後的**真實指令**攤開，
而且已填入你這台 host 的實際值（ssh -i、namespace、ECO key、registry…），
直接複製到終端機自己跑：

- **L2 `g`**：ssh 連線、找 namespace、列 deploy、ECO API 全套（換 token →
  列 app → 查 config → 更新 config）、troubleshoot 速查
- **L3 `g`**：該服務的 inspect / lifecycle 指令、本機 buildx 指令、完整
  OCI `helm upgrade --install`（含所有 `--set`）、`--reuse-values
  --set image.tag=` 快速換版、rollout 觀察、ECO curl 鏈

---

## 9. ECO Config Center

原理：所有 API 都透過 SSH 在 node 上 curl eco-server 的 clusterIP——
不用 port-forward、本機網路不用通 cluster。token 自動換發、過期自動重換。

| 位置 | Key | 動作 |
|------|-----|------|
| L2 | `E` | 列出所有 registered app |
| L3 | `e` | 看這個服務的完整 config（JSON、可捲動） |
| ECO 畫面內 | `E` | 用 `$EDITOR` 開 JSON 編輯，存檔關閉後 → confirm → PUT 回去（JSON 格式錯誤會擋下、沒改不送） |
| ECO 畫面內 | `D` | 刪除 ECO registration（confirm；deployment 本身不會被刪） |
| ECO 畫面內 | `r` | 重新抓 |

**Create** 沒有獨立按鍵——正規路徑是 helm 部署帶 `global.eco.crd=true`，
Application CRD 會自動註冊。

ECO 錯誤判讀：

| 錯誤 | 意思 |
|------|------|
| `/auth/token` 404 `record not found` | 這組 apiKey 沒註冊在 ECO |
| `400 srpName is required` | 請求少帶 srpName |
| `401 auth header is empty/invalid` | token 缺失或過期（工具會自動重試） |

---

## 10. 檔案位置

| 路徑 | 內容 |
|------|------|
| `~/.k3sdeploy/state.json` | hosts、部署 targets、build 計數器 |
| `~/.k3sdeploy/secrets.json` | `FEED_PAT`（0600） |

## 11. 安全守則

- 保護 namespace 永遠鎖寫入；所有寫操作先看指令再按 `y`
- push 前 `make audit` 會掃 commit 訊息與歷史 patch 裡的內部資訊
  （IP / UUID / API key），pre-push hook 自動擋
