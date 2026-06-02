# wedakube UI Plan

k9s 風、三層 drill-down 的多主機 k3s 運維控制台。畫面是一個 **screen stack**：
`Enter/→` 往下推、`Esc/←` 退回、`q` 離開、`?` 說明。

```
┌─ L1 Host List ─────────┐  Enter  ┌─ L2 Service List ──────┐  Enter  ┌─ L3 Service Detail ────┐
│ hosts you manage       │ ──────► │ services in host's NS  │ ──────► │ summary + action menu  │
│ n/e/d/i  Enter=connect │ ◄────── │ p=pin a=add-pin /=filt │ ◄────── │ inspect / lifecycle    │
└────────────────────────┘   Esc   └────────────────────────┘   Esc   └────────────────────────┘
        modal: Host Form              modal: Add Service-Pin           sub: Logs / Deploy / Confirm
```

全域鍵：`q` quit · `Esc` back · `?` help · `r` refresh · `/` filter（在列表畫面）

---

## L1 — Host List

工具管理的主機清單（state.json 持久化）。一台 host = 一組 SSH 連線設定。

```
┌ wedakube ── hosts ───────────────────────────────────────────────┐
│ NAME                 HOST            USER     NS              ●    │
│ ❯ my-cluster  10.0.0.1   ubuntu   …-weda (auto)   up   │
│   staging-k3s        10.0.3.12       ubuntu   apps            up   │
│   old-lab            10.0.9.4        root      default        down │
└───────────────────────────────────────────────────────────────────┘
 n new · e edit · d delete · i info · Enter connect · r refresh · q quit
```

- `●` = 連通性（背景 ping `ssh ... echo ok`）。
- **n / e** → Host Form（modal）。
- **d** → 只刪「工具裡的 host 設定」，不碰 cluster（安全模型：工具可刪自己的設定，不可刪 cluster 資源）。
- **i** → 唯讀資訊面板：k3s 版本、node Ready、ns 數、ControlMaster 狀態。
- **Enter** → 建立/複用該 host 的 ControlMaster 連線 → 進 L2。

### Host Form（modal）
```
┌ Edit Host ─────────────────────────────────┐
│ Name          [ my-cluster        ]  │
│ HostName/IP   [ 10.0.0.1            ]  │
│ User          [ ubuntu                   ]  │
│ IdentityFile  [ ~/.ssh/private.key       ]  │
│ Namespace     [ (auto: 結尾 -weda)        ]  │  ← 空=自動解析；填=固定
│                                             │
│        [ Test ]   [ Save ]   [ Cancel ]     │
└─────────────────────────────────────────────┘
```
- **Test** 按下去實際跑一次 `ssh -i <key> <user>@<host> 'kubectl version'`，當場回報成功/錯誤（少一次「存了才發現連不上」）。
- 可選便利功能：**Import from ~/.ssh/config**（讀現成 alias 自動填欄位）。

---

## L2 — Service List（進入某 host 後）

```
┌ my-cluster · ns=…-weda ──────────────────── READONLY ─┐
│ ★ NAME                 READY  UP-TO-DATE  IMAGE                RST │
│ ❯★ container-management 1/1    1           container-mgmt:v1…    0 │  ← pinned 浮頂
│  ★ weda-file-transfer   –      –           (not deployed)        – │  ← add-pin 的工作目標
│    authinfo-mgr         1/1    1           authinfo-mgr:v1…      0 │
│    datapoints           1/1    1           datapoints:v1…        0 │
└───────────────────────────────────────────────────────────────────┘
 Enter open · p pin/unpin · a add service-pin · / filter · r refresh · Esc back
```

- 每 3 秒自動刷新（沿用 M1）。`★` = pinned。
- **pin 的兩種來源**：
  1. **p**：把列表上某個「cluster 已存在」的服務標記為工作中（favorite，浮頂）。
  2. **a (add service-pin)**：新增一個**工作目標**，可指向「還沒部署」的服務（如 file-transfer），並綁定本地 repo / Dockerfile —— 這就是 M2 的 deploy `Target`。pin 因此同時是「我關注的服務」+「可部署的目標」。
- **READONLY** 徽章：目前在唯讀模式；按 `w` 切「寫模式」徽章變 **WRITE**（危險動作才解鎖）。

### Add Service-Pin（modal）= 設定一個 deploy Target
```
┌ Add Service-Pin ───────────────────────────┐
│ Service name  [ weda-file-transfer       ]  │  ← k8s deployment 名
│ Repo path     [ ~/advantech/.../file-…   ]  │
│ Dockerfile    [ <repo>/Dockerfile        ]  │  ← 自動偵測填入
│ Image repo    [ harbor…/edge-coa/weda_…  ]  │
│ Version base  [ v0.1.0                   ]  │
│ Container port[ 5001                     ]  │  ← 自動從 Dockerfile EXPOSE 偵測
│        [ Save ]   [ Cancel ]                │
└─────────────────────────────────────────────┘
```

---

## L3 — Service Detail（選一個服務後）

```
┌ container-management · …-weda · adlk ───────────── READONLY ─┐
│ image  harbor…/edge-coa/container-management:v1.0.0_20260424.1 │
│ ready  1/1   up-to-date 1   available 1   restarts 0          │
│ ─────────────────────────────────────────────────────────────│
│ INSPECT (唯讀，即時)                                            │
│   s status      i info(image/ver)   l logs                    │
│   u resource    k services/networking                         │
│ LIFECYCLE (寫，需 WRITE 模式 + 確認)                            │
│   D deploy      R restart   ↑ start   ↓ stop   z rollback     │
│ DEBUG                                                          │
│   x exec(shell)   f port-forward   w swagger                  │
└───────────────────────────────────────────────────────────────┘
 Esc back · ? help
```

對應到底層操作（README / M2 engine）：

| 鍵 | 動作 | 底層 | 寫? |
|---|---|---|---|
| s | status | `get deploy,rs,pod -l app… -o wide` | 讀 |
| i | info | `get deploy -o jsonpath`(image) + rollout history | 讀 |
| l | logs | `logs deploy/<svc> -f`（全螢幕 viewer，f 切 follow，/ 搜尋） | 讀 |
| u | resource | `top pod -l app…` | 讀 |
| k | networking | `get svc` | 讀 |
| D | **deploy** | M2 engine：buildx→push→(apply\|set image)→rollout 進度條 | 寫 |
| R | restart | `rollout restart` + WaitRollout | 寫 |
| ↑ | start | `scale --replicas=1` | 寫 |
| ↓ | stop | `scale --replicas=0` | 寫 |
| z | rollback | `rollout undo`（可選 revision） | 寫 |
| x | exec | `exec -it deploy/<svc> -- sh`（暫離 TUI 接管終端） | 偵錯 |
| f | port-forward | `ssh -L` tunnel + node port-forward | 偵錯 |
| w | swagger | port-forward + tunnel + 開本地瀏覽器 /swagger | 偵錯 |

### 子畫面
- **Logs viewer**：全螢幕、follow 開關、`/` 搜尋、`Esc` 退回。
- **Deploy progress**：phase 列（tag→build→push→apply/setimage→rollout）+ `bubbles/progress` 進度條 + build log 串流窗；失敗顯示 Diagnose 並回報 rollback 結果。
- **Confirm modal**（所有寫動作）：印出**將執行的確切 kubectl 指令**，`y` 確認 `n` 取消。

---

## 安全模型（沿用既定）
- 預設 **READONLY**；寫動作需先 `w` 切 WRITE 模式，且每次仍跳 Confirm。
- **Protected namespaces**（`kube-system` / `wise-system` / `wise-backing-service` / `wise-observability`）：在 L2/L3 一律唯讀、Lifecycle 鍵 disabled。
- **不提供 cluster delete**；工具只能刪自己的 host/pin 設定。

---

## 持久化（~/.k3sdeploy/state.json 擴充）
```jsonc
{
  "hosts": [
    { "name":"my-cluster", "hostName":"10.0.0.1",
      "user":"ubuntu", "identityFile":"~/.ssh/private.key", "namespace":"" }
  ],
  "pins": {                         // key = hostName
    "my-cluster": ["container-management", "weda-file-transfer"]
  },
  "targets": { "...repoPath...": { /* 既有 deploy Target，加 host 參照 */ } },
  "counters": { "weda-file-transfer|20260602": 1 }
}
```

---

## 需要的 engine 改動（實作時）
1. **SSH 改成顯式身分**：`NewSSH` 接 host/user/identityFile，組 `ssh -i <key> -l <user> <host>`（不再只靠 alias）。ControlMaster path 以 host 為 key。
2. **kubectl 的 ns**：每 host 可固定或 auto-resolve（沿用 `ResolveWedaNamespace`）。
3. **新增 ops**：`top pod`、`get svc`、`scale`、`rollout restart`、`exec`(接管 PTY)、`port-forward`(背景 `ssh -L`)。
4. **TUI screen stack**：L1/L2/L3 各一個 bubbletea sub-model，root model 管堆疊與全域鍵。

---

## 開放決策（規劃確認）
- **Pin 語意**：建議「pin = 工作目標」，既存服務 pin 只是 favorite 浮頂；`a` 新增的 pin 可綁 repo 成為可部署 target（涵蓋 file-transfer 這種還沒部署的）。
- **Host 身分來源**：建議工具自帶 HostName/User/IdentityFile（符合你草圖），另提供「從 ~/.ssh/config 匯入」便利鍵。
```
