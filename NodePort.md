# 在 Kubernetes 環境透過 NodePort 開啟 Swagger UI

本文件說明如何將已部署應用的 Kubernetes Service 暫時改為 NodePort，以便從瀏覽器直接訪問 Swagger UI。

---

## 適用情境

- 應用已透過 Helm 部署至 K8s 叢集
- Service 類型為 `ClusterIP`（預設），外部無法直接存取
- 需要臨時開放 Swagger UI 進行 API 測試或驗證

---

## 操作步驟

### 1. 確認目前的 Service 名稱與狀態

```bash
kubectl get svc -n <namespace>
```

範例輸出：

```
NAME       TYPE        CLUSTER-IP      EXTERNAL-IP   PORT(S)    AGE
myapp      ClusterIP   10.96.123.45    <none>        8080/TCP   10m
```

---

### 2. 將 Service 類型改為 NodePort

使用 `kubectl patch` 一次完成修改（推薦）：

```bash
kubectl patch svc <service-name> -n <namespace> \
  -p '{"spec": {"type": "NodePort"}}'
```

或使用互動式編輯器手動修改：

```bash
kubectl edit svc <service-name> -n <namespace>
```

找到以下欄位，將 `ClusterIP` 改為 `NodePort`：

```yaml
spec:
  type: NodePort   # 原本為 ClusterIP
```

> **提示：** 若要指定固定的 NodePort 號碼（範圍 30000–32767），可在 `ports` 區塊加入 `nodePort` 欄位：
> ```yaml
> ports:
>   - port: 8080
>     targetPort: 8080
>     nodePort: 30080   # 自訂 port，不指定則自動分配
> ```

---

### 3. 查詢分配到的 NodePort

```bash
kubectl get svc <service-name> -n <namespace>
```

範例輸出：

```
NAME    TYPE       CLUSTER-IP     EXTERNAL-IP   PORT(S)          AGE
myapp   NodePort   10.96.123.45   <none>        8080:31234/TCP   1m
```

冒號後方的數字（此例為 `31234`）即為 NodePort。

---

### 4. 取得 Node IP

```bash
kubectl get nodes -o wide
```

範例輸出：

```
NAME     STATUS   ROLES           AGE   VERSION   INTERNAL-IP     ...
node-1   Ready    control-plane   5d    v1.28.0   10.0.0.1   ...
```

取 `INTERNAL-IP` 欄位的值作為訪問 IP。

---

### 5. 用瀏覽器開啟 Swagger UI

在瀏覽器輸入：

```
http://<node-ip>:<node-port>/swagger
```

範例：

```
http://10.0.0.1:31234/swagger
```

> **注意：** Swagger 路徑依應用實作而異，常見路徑包括：
> - `/swagger`
> - `/swagger/index.html`
> - `/swagger/v1/swagger.json`（JSON 定義檔）

---

## 操作完畢後還原設定

Swagger 驗證完成後，建議將 Service 還原為 `ClusterIP`，避免不必要的端口暴露：

```bash
kubectl patch svc <service-name> -n <namespace> \
  -p '{"spec": {"type": "ClusterIP"}}'
```

確認已還原：

```bash
kubectl get svc <service-name> -n <namespace>
```

---

## 快速參考

| 步驟 | 指令 |
|------|------|
| 查看 Service 列表 | `kubectl get svc -n <namespace>` |
| 改為 NodePort | `kubectl patch svc <name> -n <namespace> -p '{"spec":{"type":"NodePort"}}'` |
| 查看分配的 Port | `kubectl get svc <name> -n <namespace>` |
| 查看 Node IP | `kubectl get nodes -o wide` |
| 還原為 ClusterIP | `kubectl patch svc <name> -n <namespace> -p '{"spec":{"type":"ClusterIP"}}'` |

