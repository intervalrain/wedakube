package deploy

import (
	"context"
	"fmt"

	"github.com/intervalrain/wedakube/internal/cluster"
	"github.com/intervalrain/wedakube/internal/config"
)

// Render 產生一份最小 Deployment + Service manifest（給全新服務第一次部署用）。
// 標 app 與 app.kubernetes.io/name 兩種 label，方便用 -l app=<svc> 選取自己的 pod。
func Render(t config.Target, tag string) string {
	image := t.ImageRepo + ":" + tag
	return fmt.Sprintf(`apiVersion: apps/v1
kind: Deployment
metadata:
  name: %[1]s
  namespace: %[2]s
  labels:
    app: %[1]s
    app.kubernetes.io/name: %[1]s
spec:
  replicas: 1
  selector:
    matchLabels:
      app: %[1]s
  template:
    metadata:
      labels:
        app: %[1]s
        app.kubernetes.io/name: %[1]s
    spec:
      containers:
        - name: %[1]s
          image: %[3]s
          imagePullPolicy: IfNotPresent
          ports:
            - containerPort: %[4]d
          env:
            - name: ASPNETCORE_ENVIRONMENT
              value: Development
            - name: ECO_ENABLED
              value: "false"
          readinessProbe:
            tcpSocket:
              port: %[4]d
            initialDelaySeconds: 10
            periodSeconds: 5
            failureThreshold: 6
---
apiVersion: v1
kind: Service
metadata:
  name: %[1]s
  namespace: %[2]s
spec:
  selector:
    app: %[1]s
  ports:
    - port: %[4]d
      targetPort: %[4]d
`, t.Service, t.Namespace, image, t.Port)
}

// EnsureNamespace 冪等建立 namespace。
func EnsureNamespace(ctx context.Context, ssh *cluster.SSH, ns string) error {
	cmd := fmt.Sprintf("kubectl create namespace %s --dry-run=client -o yaml | kubectl apply -f -", ns)
	_, err := ssh.Run(ctx, cmd)
	return err
}

// Apply 把 manifest 透過 stdin 餵給遠端 kubectl apply。
func Apply(ctx context.Context, ssh *cluster.SSH, manifest []byte) error {
	if _, err := ssh.RunStdin(ctx, "kubectl apply -f -", manifest); err != nil {
		return fmt.Errorf("apply: %w", err)
	}
	return nil
}

// DeploymentExists 判斷該 deployment 是否已存在（決定走 set image 還是首次 apply）。
func DeploymentExists(ctx context.Context, ssh *cluster.SSH, t config.Target) bool {
	_, err := ssh.Run(ctx, fmt.Sprintf("kubectl -n %s get deploy/%s -o name", t.Namespace, t.Service))
	return err == nil
}
