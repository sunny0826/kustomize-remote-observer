package controllers

const (
	DeployTemplate = `apiVersion: apps/v1
kind: Deployment
metadata:
  name: {{ .AppName }}
spec:
  template:
    spec:
      imagePullSecrets:
      - name: {{ .PullSecrets }}
      containers:
        - name: {{ .AppName }}
          image: {{ .Image }}
          imagePullPolicy: Always
`
	SvcTemplate = `apiVersion: v1
kind: Service
metadata:
  name: {{ .AppName }}
spec:
  ports:
  - name: web
    port: {{ .Port }}
    targetPort: {{ .TargetPort }}
  type: ClusterIP
`
	BaseKustTemplate = `commonLabels:
  app: {{ .AppName }}

resources:
- service.yaml
- deployment.yaml
`
	HealthCheckTemplate = `apiVersion: apps/v1
kind: Deployment
metadata:
  name: {{ .AppName }}
spec:
  replicas: 1
  template:
    spec:
      containers:
        - name: {{ .AppName }}
          livenessProbe:    #存活检查
            #监控检查模式:
            httpGet:
              path: {{ .Path }}
              port: {{ .TargetPort }}
            initialDelaySeconds: 60 #在Pod启动130秒后进行检测。
            periodSeconds: 60  #进行健康监测的频率为60秒1次。
            timeoutSeconds: 3 #健康检查超时时间
          readinessProbe: #就绪检查
            #监控检查模式:
            httpGet:
              path: {{ .Path }}
              port: {{ .TargetPort }}
            initialDelaySeconds: 60 #在Pod启动130秒后进行检测。
            periodSeconds: 60  #进行健康监测的频率为60秒1次。
            timeoutSeconds: 3 #健康检查超时时间
`
	ResourceTemplate = `apiVersion: apps/v1
kind: Deployment
metadata:
  name: {{ .AppName }}
spec:
  template:
    spec:
      containers:
        - name: {{ .AppName }}
          resources:
            limits:
              cpu: {{ .CpuLimits }}
              memory: {{ .MemoryLimits }}
            requests:
              cpu: {{ .CpuRequests	 }}
              memory: {{ .MemoryRequests }}
          env:
          - name: runShell
            value: RunShell
`
	StrategyTemplate = `apiVersion: apps/v1
kind: Deployment
metadata:
  name: {{ .AppName }}
spec:
  strategy:  # k8s更新策略
      type: RollingUpdate #滚动更新
      rollingUpdate:
        maxSurge: 1  # 更新时允许最大激增的容器数，默认 replicas 的 1/4 向上取整
        maxUnavailable: 0  # 更新时允许最大 unavailable 容器数，默认 replicas 的 1/4 向下取整

`
	OverlaysKustTemplate = `bases:
- ../../base
patchesStrategicMerge:
- strategy_patch.yaml
- healthcheck_patch.yaml
- memorylimit_patch.yaml
namespace: {{ .Namespace }}
`
)
