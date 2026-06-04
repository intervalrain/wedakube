# Helm Chart Generator

## 簡介
Helm Chart Generator，簡稱 `helmster` ，在 app 有整合 ECO 的情況下，可透過指令 `helmster gen` 從應用程式設定檔案（appcfg.yaml）產生 Helm Chart。此指令會讀取應用程式設定，進行驗證，並根據設定產生完整的 Helm Chart 檔案結構，包含 Chart.yaml、values.yaml 以及所有必要的模板檔案。

樣板的 Helm Chart 使用 Go 模板語法，並採用 `[[ ]]` 作為分隔符號（而非 Helm 原生的 `{{ }}`），以避免與 Helm 模板語法產生衝突。

## 下載點
下載請 [點我](https://dev.azure.com/Advantech-EBO/Infrastructure/_build/results?buildId=17712&view=artifacts&pathAsName=false&type=publishedArtifacts)

## 使用方式

```bash
helmster gen -f <設定檔案路徑> -o <輸出目錄>
```

## 必要參數

### `-f, --file` (必要)
指定 appcfg.yaml 設定檔案的路徑。

- **類型**: 字串
- **範例**: `-f appcfg.yaml` 或 `-f examples/appcfg.yaml`

## 選填參數

### `-o, --output` (選填)
指定 Helm Chart 的輸出目錄。

- **類型**: 字串
- **預設值**: `.` (當前目錄)
- **範例**: `-o ./output` 或 `-o /home/user/charts`

## 全域設定參數

除了 gen 指令專用參數外，還可以使用以下全域變數來自訂 Helm Chart 的產生行為：

### `--tenant-id`
租戶 ID 識別碼。

- **類型**: 字串
- **預設值**: `00000000-0000-0000-0000-000000000001`
- **用途**: 用於多租戶環境的識別

### `--tenant-path`
租戶路徑名稱。

- **類型**: 字串
- **預設值**: `platform`
- **用途**: 用於路由路徑和命名空間管理

### `--tenant-alias`
租戶別名。

- **類型**: 字串
- **預設值**: `rsv`
- **用途**: 租戶的簡短識別名稱

### `--srp-name`
SRP (Solution Ready Package) 名稱。

- **類型**: 字串
- **預設值**: `solutions`
- **用途**: 用於路由路徑生成和服務分組

### `--domain-name`
部署環境的網域名稱。

- **類型**: 字串
- **預設值**: `k3s.weda.dev`
- **用途**: 用於 Ingress 和 IngressRoute 的主機名稱設定

### `--registry`
容器鏡像的 Registry 主機名稱。

- **類型**: 字串
- **預設值**: `registry.example.com`
- **用途**: Docker/Container 鏡像的來源 Registry

### `--project`
容器鏡像所在的專案名稱。

- **類型**: 字串
- **預設值**: `edge-coa`
- **用途**: Registry 中的專案或命名空間

### `--eco-enabled`
是否啟用 ECO (Enhanced Configuration Orchestrator) 整合。

- **類型**: 布林值
- **預設值**: `true`
- **用途**: 控制 ECO 是否啟用

### `--eco-endpoint`
ECO 服務的端點 URL。

- **類型**: 字串
- **預設值**: `http://<ECO_HOST>`
- **用途**: ECO 服務的連線位址

### `--eco-api-key`
ECO API 金鑰。

- **類型**: 字串
- **預設值**: `replace-your-api-key`
- **用途**: ECO 服務的認證金鑰

### `--jwt-middleware`
JWT 認證中介軟體的名稱。

- **類型**: 字串
- **預設值**: `secure-api@file`
- **用途**: Traefik 路由中使用的 JWT 中介軟體名稱

### `--db-provider`
資料庫提供者類型。

- **類型**: 字串
- **預設值**: `postgresql`
- **用途**: 指定資料庫類型（如 postgresql、mysql 等）

### `--config`
Helmster 設定檔案路徑。

- **類型**: 字串
- **預設值**: `$HOME/.helmster.yaml`
- **用途**: 可以將上述全域變數儲存在設定檔案中，避免每次執行都要輸入

## 設定檔案設定（appcfg.yaml）

appcfg.yaml 是應用程式的核心設定檔案，包含以下主要設定：

### 基本設定

| 欄位 | 類型 | 必要 | 說明 |
|------|------|------|------|
| `name` | 字串 | ✓ | 應用程式名稱（會作為 Chart 名稱和鏡像名稱） |
| `version` | 字串 | ✓ | 應用程式版本號 |
| `description` | 字串 | ✗ | 應用程式描述 |
| `port` | 整數 | ✓ | 應用程式服務埠號（預設: 8080） |
| `swaggerPath` | 字串 | ✗ | Swagger API 文件路徑 |

### 健康檢查設定 (healthCheck)

設定 Kubernetes 探針來監控應用程式的健康狀態。此部分為**選填**，僅在需要時才設定。

支援三種探針類型：
- `startup`: 啟動探針（用於慢速啟動的應用程式）
- `liveness`: 存活探針（檢查容器是否需要重啟）
- `readiness`: 就緒探針（檢查容器是否準備好接收流量）

每種探針支援四種檢查方式：
- `httpGet`: HTTP GET 請求
- `tcp`: TCP 連線檢查
- `exec`: 執行命令
- `grpc`: gRPC 健康檢查

### 資源限制設定 (resources)

設定容器的 CPU 和記憶體資源限制。

### Traefik 路由設定 (traefik)

設定 Traefik IngressRoute 和 Middleware，用於服務的路由和流量管理。

支援簡化設定模式，可以快速設定多個路由規則，包括：
- 路徑前綴比對 (pathPrefix)
- 正規表示式比對 (pathRegexp)
- 路由優先級 (priority)
- 自動產生 StripPrefix 中介軟體
- JWT 認證中介軟體整合

### Jobs 設定 (jobs)

設定 Kubernetes Job，用於執行一次性任務（如資料庫遷移、資料初始化等）。

Jobs 使用 Helm hooks 機制，在 pre-install 和 pre-upgrade 階段執行。

## 使用範例

### 範例 1: 基本使用

```bash
# 從當前目錄的 appcfg.yaml 產生 Chart 到 ./output 目錄
helmster gen -f appcfg.yaml -o ./output

# 安裝產生的 Chart
cd output
helm install myapp ./myapp
```

### 範例 2: 自訂租戶和 SRP 設定

```bash
# 指定租戶和 SRP 相關參數
helmster gen \
  -f appcfg.yaml \
  -o ./output \
  --tenant-path mycompany \
  --srp-name services \
  --domain-name prod.example.com
```

### 範例 3: 使用設定檔案

建立 `$HOME/.helmster.yaml`:

```yaml
tenant-id: "12345678-1234-1234-1234-123456789012"
tenant-path: "production"
tenant-alias: "prod"
srp-name: "backend-services"
domain-name: "k8s.example.com"
registry: "registry.example.com"
project: "microservices"
eco-enabled: true
eco-endpoint: "http://eco.example.com"
eco-api-key: "your-api-key-here"
jwt-middleware: "auth-jwt@file"
db-provider: "postgresql"
```

然後執行：

```bash
# 使用設定檔案中的設定
helmster gen -f appcfg.yaml -o ./output

# 或明確指定設定檔案位置
helmster gen -f appcfg.yaml -o ./output --config /path/to/config.yaml
```

### 範例 4: 產生多個應用程式的 Charts

```bash
# 產生多個應用程式
helmster gen -f apps/service-a/appcfg.yaml -o ./charts
helmster gen -f apps/service-b/appcfg.yaml -o ./charts
helmster gen -f apps/service-c/appcfg.yaml -o ./charts

# 批次安裝
cd charts
for chart in */; do
  helm install "${chart%/}" "./$chart"
done
```

### 範例 5: 驗證產生的 Chart

```bash
# 產生 Chart
helmster gen -f appcfg.yaml -o ./output

# 使用 Helm 驗證
cd output/myapp
helm lint .

# 檢視渲染後的模板
helm template . --debug

# 模擬安裝（不實際部署）
helm install --dry-run=client --debug myapp .
```

## 產生檔案說明

執行 `helmster gen` 後，會在輸出目錄中產生以下檔案結構：

```
output/
└── <應用程式名稱>/
    ├── Chart.yaml              # Helm Chart 元資料
    ├── values.yaml             # 預設設定值
    └── templates/              # Kubernetes 資源模板
        ├── _helpers.tpl        # 輔助模板函式
        ├── deployment.yaml     # Deployment 資源
        ├── service.yaml        # Service 資源
        ├── ingress.yaml        # Ingress 資源（標準 K8s）
        ├── ingressroute.yaml   # IngressRoute 資源（Traefik）
        ├── middleware.yaml     # Middleware 資源（Traefik）
        ├── job.yaml            # Job 資源（如有設定）
        └── NOTES.txt           # 安裝後的說明資訊
```

### 檔案說明

- **Chart.yaml**: 包含 Chart 的名稱、版本、描述等元資料
- **values.yaml**: 包含所有可設定的參數及其預設值，使用者可以在安裝時覆寫這些值
- **templates/deployment.yaml**: Kubernetes Deployment 資源定義，包含容器設定、健康檢查、資源限制等
- **templates/service.yaml**: Kubernetes Service 資源定義，用於服務發現和負載均衡
- **templates/ingress.yaml**: 標準 Kubernetes Ingress 資源（如需要）
- **templates/ingressroute.yaml**: Traefik IngressRoute 資源（如啟用 Traefik）
- **templates/middleware.yaml**: Traefik Middleware 資源（如啟用 Traefik）
- **templates/job.yaml**: Kubernetes Job 資源（如設定了 jobs）
- **templates/_helpers.tpl**: Go 模板輔助函式，用於產生標籤、名稱等
- **templates/NOTES.txt**: 安裝完成後顯示的使用說明

## 預設值與自動處理

### 自動設定的預設值

如果以下欄位未在 appcfg.yaml 中指定，gen 指令會自動設定預設值：

| 欄位 | 預設值 | 說明 |
|------|--------|------|
| `port` | `8080` | 應用程式服務埠號 |
| `resources.limits.cpu` | `500m` | CPU 限制 |
| `resources.limits.memory` | `512Mi` | 記憶體限制 |

### 自動處理邏輯

1. **鏡像名稱**: `image` 欄位會自動設定為應用程式的 `name`
2. **Traefik 設定轉換**: 如果設定了 `traefik` 簡化設定，會自動轉換為完整的 Traefik 設定，包括：
   - 根據全域設定（tenant-path、srp-name）自動產生路由比對規則
   - 自動產生 StripPrefix Middleware
   - 自動整合 JWT Middleware（如啟用）
3. **Jobs 繼承**: Job 設定會繼承應用程式的預設設定（鏡像、版本、環境變數等）

## 設定驗證

gen 指令會執行以下驗證：

1. **必要欄位檢查**: 確保 `name` 欄位已設定
2. **Jobs 設定驗證**: 
   - 如指定 `image`，則必須同時指定 `imageTag`
   - 如未指定 `image`，則不應指定 `imageTag`
3. **健康檢查設定驗證**: 驗證探針設定的完整性和正確性
4. **Traefik 設定驗證**: 驗證路由規則的有效性

如果驗證失敗，會顯示詳細的錯誤訊息並終止執行。

## 執行流程

`helmster gen` 的執行流程如下：

1. **載入設定**: 
   - 讀取全域設定（CLI flags 或設定檔案）
   - 載入 appcfg.yaml 檔案
2. **驗證設定**: 
   - 檢查必要欄位
   - 驗證設定的有效性
3. **設定預設值**: 
   - 為未指定的欄位設定預設值
4. **轉換設定**: 
   - 將 Traefik 簡化設定轉換為完整設定
5. **產生 Chart**: 
   - 使用 Chart Service 產生 Helm Chart
6. **寫入檔案**: 
   - 將產生的 Chart 寫入輸出目錄
7. **顯示結果**: 
   - 顯示產生的檔案路徑
   - 顯示安裝指令範例

## 錯誤處理

常見錯誤及解決方法：

| 錯誤訊息 | 原因 | 解決方法 |
|----------|------|----------|
| `config.name is required` | 未設定應用程式名稱 | 在 appcfg.yaml 中加入 `name` 欄位 |
| `failed to load config file` | 設定檔案讀取失敗 | 檢查檔案路徑是否正確、檔案權限是否足夠 |
| `config validation failed` | 設定驗證失敗 | 根據詳細錯誤訊息修正設定 |
| `job 'xxx': imageTag is required when image is specified` | Job 只指定 image 未指定 imageTag | 同時指定 `image` 和 `imageTag`，或都不指定使用預設 |
| `failed to generate chart` | Chart 產生失敗 | 檢查設定是否符合 schema 規範 |
| `failed to write chart files` | 檔案寫入失敗 | 檢查輸出目錄權限、磁碟空間是否足夠 |

## 最佳實踐

1. **版本管理**: 將 appcfg.yaml 納入版本控制系統（如 Git）
2. **環境區分**: 為不同環境（開發、測試、生產）建立不同的設定檔案
3. **參數化**: 使用全域變數或 Helm values 來管理環境相關的差異
4. **驗證習慣**: 產生 Chart 後使用 `helm lint` 和 `helm template` 驗證
5. **漸進部署**: 先使用 `--dry-run` 模擬安裝，確認無誤後再實際部署
6. **文件維護**: 在 appcfg.yaml 中適當加入註解說明設定用途
7. **模組化**: 使用 Jobs 功能將初始化任務與主要應用程式分離
8. **監控整合**: 正確設定健康檢查探針，確保 Kubernetes 能正確監控應用程式狀態

## 疑難排解

### 產生的 Chart 無法安裝

1. 檢查 Kubernetes 叢集連線狀態
2. 驗證當前 context 和 namespace
3. 使用 `helm lint` 檢查 Chart 語法
4. 使用 `helm install --dry-run=client --debug` 查看詳細錯誤

### Traefik 路由無法正常工作

1. 確認 Traefik 已安裝在叢集中
2. 檢查 IngressRoute 資源是否正確建立
3. 驗證 Middleware 資源是否正確綁定
4. 檢查路由優先級設定是否合理

### 健康檢查探針失敗

1. 確認應用程式的健康檢查端點正常運作
2. 檢查探針的 `initialDelaySeconds` 是否足夠
3. 調整 `timeoutSeconds` 和 `failureThreshold` 參數
4. 查看 Pod 日誌了解應用程式狀態

### Jobs 執行失敗

1. 檢查 Job Pod 的日誌
2. 確認鏡像和標籤正確
3. 驗證 command 設定正確
4. 檢查環境變數和資源設定

## 進階使用

### 與 CI/CD 整合

```bash
# GitLab CI 範例
build-chart:
  stage: build
  script:
    - helmster gen -f appcfg.yaml -o ./charts
    - helm package ./charts/*
  artifacts:
    paths:
      - "*.tgz"
```

### 批次產生與管理

```bash
#!/bin/bash
# 批次產生多個應用程式的 Charts

APPS_DIR="./apps"
OUTPUT_DIR="./charts"

for app in "$APPS_DIR"/*; do
  if [ -f "$app/appcfg.yaml" ]; then
    echo "Generating chart for $(basename "$app")..."
    helmster gen -f "$app/appcfg.yaml" -o "$OUTPUT_DIR"
  fi
done

echo "All charts generated successfully!"
```

### 動態參數覆寫

```bash
# 從環境變數讀取參數
export TENANT_PATH="${ENV_NAME:-platform}"
export DOMAIN_NAME="${CLUSTER_DOMAIN:-k3s.weda.dev}"

helmster gen \
  -f appcfg.yaml \
  -o ./output \
  --tenant-path "$TENANT_PATH" \
  --domain-name "$DOMAIN_NAME"
```

## 相關文件
* [Defining AppCfg Extension Specifications for Kubernetes Environments](/WEDA-Infrastructure-Introduction/開發整合/Configuration-Center/Defining-AppCfg-Extension-Specifications-for-Kubernetes-Environments)
