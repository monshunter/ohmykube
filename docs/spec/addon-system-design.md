# Addon 系统设计文档

> **版本**: 1.0
> **状态**: active
> **更新日期**: 2026-03-05

## 1 概述

OhMyKube Addon 系统提供一个**通用的应用安装框架**，支持在 Kubernetes 集群中安装任意应用，包括但不限于 metrics-server、prometheus、grafana 等常用应用。系统支持 Helm Chart 和 Kubernetes Manifest 两种安装方式，并复用现有的镜像缓存机制以加速应用部署。

### 1.1 背景

用户在集群创建后，通常需要安装一些常用应用。虽然可以通过 `kubectl apply -f` 或 `helm` 手动安装，但难以复用，每次安装集群后都需要手动或通过脚本执行后续软件安装过程，且无法缓存资源，新集群总是必须通过网络重新下载镜像，导致集群就绪时间过长。

## 2 设计目标

- **通用性优先**: 支持任意 Helm Chart 和 Kubernetes Manifest 应用，不限定特定软件
- **最小化复杂度**: 命令行参数保持极简，复杂配置留给配置文件
- **复用现有架构**: 直接使用现有的镜像缓存机制，遵循现有 CNI/CSI/LB 插件模式
- **极简数据结构**: 只包含安装必需的最小字段集
- **通过单一 `--addon` 参数支持任意应用安装**
- **与现有集群创建流程无缝集成**

## 3 架构设计

### 3.1 整体架构

Addon 系统由以下核心组件构成：

1. **AddonSpec** — 应用安装的完整配置定义
2. **AddonStatus** — 运行时状态追踪
3. **UniversalInstaller** — 通用安装器，支持 Helm 和 Manifest 两种方式
4. **CLI 集成** — `--addon` 参数和 `ohmykube addon` 管理命令
5. **镜像缓存集成** — 直接复用现有 `ImageManager` 和 `ImageSource` 接口

### 3.2 镜像缓存集成

**设计原则**: 不创建额外封装层，直接使用现有的 `ImageManager` 和 `ImageSource` 接口，完全按照现有 CNI/CSI/LB 插件的模式。

```go
// 在通用安装器中缓存镜像
func (u *UniversalInstaller) cacheImages() error {
    ctx := context.Background()
    imageManager, err := cache.GetImageManager()
    if err != nil {
        return fmt.Errorf("failed to get image manager: %w", err)
    }

    var source interfaces.ImageSource
    switch u.spec.Type {
    case "helm":
        source = interfaces.ImageSource{
            Type:         "helm",
            ChartName:    u.spec.Chart,
            ChartRepo:    u.spec.Repo,
            Version:      u.spec.Version,
            ChartValues:  u.spec.Values,
            ValuesFile:   u.spec.ValuesFiles,
            IsLocalChart: u.isLocalChart(),
        }
    case "manifest":
        source = interfaces.ImageSource{
            Type:          "manifest",
            ManifestFiles: u.spec.Files,
            Version:       u.spec.Version,
        }
    }

    return imageManager.EnsureImages(ctx, source, u.sshRunner, u.controllerNode, u.controllerNode)
}
```

**关键点**:
- 不修改现有 `ImageSource` 接口
- 直接映射 `AddonSpec` 到现有的 `ImageSource` 格式
- 完全复用现有镜像发现和缓存逻辑

## 4 接口定义

### 4.1 公开接口

#### 4.1.1 命令行接口

```bash
# 单个 Helm 应用
ohmykube up my-cluster --addon '{"name":"prometheus","type":"helm","repo":"https://prometheus-community.github.io/helm-charts","chart":"prometheus-community/kube-prometheus-stack","version":"15.18.0"}'

# 单个 Manifest 应用
ohmykube up my-cluster --addon '{"name":"ingress-nginx","type":"manifest","files":["https://raw.githubusercontent.com/kubernetes/ingress-nginx/controller-v1.5.1/deploy/static/provider/cloud/deploy.yaml"],"version":"v1.5.1"}'

# 多个应用（多次使用 --addon）
ohmykube up my-cluster \
  --addon '{"name":"prometheus","type":"helm",...}' \
  --addon '{"name":"metrics-server","type":"helm",...}'

# 通过配置文件
ohmykube up -f cluster-with-addons.yaml

# Addon 管理命令
ohmykube addon list
ohmykube addon status prometheus
```

#### 4.1.2 Addon 管理命令

```go
// cmd/ohmykube/app/addon.go
var addonCmd = &cobra.Command{
    Use:   "addon",
    Short: "Manage cluster addons",
}

var addonListCmd = &cobra.Command{
    Use:   "list",
    Short: "List all addons and their status",
    RunE:  runAddonList,
}

var addonStatusCmd = &cobra.Command{
    Use:   "status [addon-name]",
    Short: "Show detailed status of a specific addon",
    Args:  cobra.ExactArgs(1),
    RunE:  runAddonStatus,
}
```

### 4.2 内部接口

#### 4.2.1 UniversalInstaller

```go
// pkg/addons/installer.go
type UniversalInstaller struct {
    sshRunner      interfaces.SSHRunner
    controllerNode string
    spec           config.AddonSpec
    cluster        *config.Cluster
}

func NewUniversalInstaller(sshRunner interfaces.SSHRunner, controllerNode string, spec config.AddonSpec, cluster *config.Cluster) *UniversalInstaller
func (u *UniversalInstaller) Install() error
```

**Install 流程**:
1. 设置初始状态 → `AddonPhaseInstalling`
2. 执行 pre-install 钩子
3. 缓存镜像（失败仅警告）
4. 执行安装（Helm 或 Manifest）
5. 执行 post-install 钩子
6. 验证安装结果
7. 设置最终状态 → `AddonPhaseInstalled`

#### 4.2.2 Cluster 状态管理方法

```go
func (c *Cluster) SetAddonStatus(addonName string, status AddonStatus)
func (c *Cluster) GetAddonStatus(addonName string) (*AddonStatus, bool)
func (c *Cluster) SetAddonPhase(addonName string, phase AddonPhase, message, reason string)
func (c *Cluster) ListAddonStatuses() []AddonStatus
func (c *Cluster) AreAllAddonsReady() bool
```

## 5 数据结构

### 5.1 AddonSpec

```go
// pkg/config/addon.go
type AddonSpec struct {
    // 基础必需字段（命令行 JSON 最少需要这些）
    Name    string `json:"name" yaml:"name"`
    Type    string `json:"type" yaml:"type"`       // "helm" 或 "manifest"
    Version string `json:"version" yaml:"version"`
    Enabled *bool  `json:"enabled" yaml:"enabled"` // 默认 true

    // Manifest 类型字段（支持本地文件、URL 或远程路径）
    Files []string `json:"files,omitempty" yaml:"files,omitempty"`

    // Helm 类型字段
    Repo       string            `json:"repo,omitempty" yaml:"repo,omitempty"`
    Chart      string            `json:"chart,omitempty" yaml:"chart,omitempty"`
    Values      map[string]string `json:"values,omitempty" yaml:"values,omitempty"`
    ValuesFiles []string          `json:"valuesFiles,omitempty" yaml:"valuesFiles,omitempty"`

    // 高级配置字段
    Namespace    string            `json:"namespace,omitempty" yaml:"namespace,omitempty"`
    Priority     int               `json:"priority,omitempty" yaml:"priority,omitempty"`
    Dependencies []string          `json:"dependencies,omitempty" yaml:"dependencies,omitempty"`
    Timeout      string            `json:"timeout,omitempty" yaml:"timeout,omitempty"`
    Labels       map[string]string `json:"labels,omitempty" yaml:"labels,omitempty"`
    Annotations  map[string]string `json:"annotations,omitempty" yaml:"annotations,omitempty"`

    // 自定义安装钩子
    PreInstall  []string `json:"preInstall,omitempty" yaml:"preInstall,omitempty"`
    PostInstall []string `json:"postInstall,omitempty" yaml:"postInstall,omitempty"`
}
```

**验证规则**:

| 类型 | 必需字段 |
|------|----------|
| 所有类型 | `name`, `type`, `version` |
| helm | `repo`, `chart` |
| manifest | `files`（至少一个，支持本地路径或 URL） |

**默认值**:
- `Enabled`: `true`
- `Timeout`: `"300s"`
- `Priority`: `100`

### 5.2 AddonStatus

```go
type AddonStatus struct {
    Name             string          `yaml:"name,omitempty"`
    Phase            AddonPhase      `yaml:"phase,omitempty"`
    InstalledVersion string          `yaml:"installedVersion,omitempty"`
    DesiredVersion   string          `yaml:"desiredVersion,omitempty"`
    Namespace        string          `yaml:"namespace,omitempty"`
    InstallTime      *time.Time      `yaml:"installTime,omitempty"`
    LastUpdateTime   *time.Time      `yaml:"lastUpdateTime,omitempty"`
    Conditions       []Condition     `yaml:"conditions,omitempty"`
    Resources        []AddonResource `yaml:"resources,omitempty"`
    Images           []string        `yaml:"images,omitempty"`
    Message          string          `yaml:"message,omitempty"`
    Reason           string          `yaml:"reason,omitempty"`
}
```

### 5.3 AddonPhase

```go
type AddonPhase string

const (
    AddonPhasePending    AddonPhase = "Pending"
    AddonPhaseInstalling AddonPhase = "Installing"
    AddonPhaseInstalled  AddonPhase = "Installed"
    AddonPhaseUpgrading  AddonPhase = "Upgrading"
    AddonPhaseUpgraded   AddonPhase = "Upgraded"
    AddonPhaseFailed     AddonPhase = "Failed"
    AddonPhaseRemoving   AddonPhase = "Removing"
    AddonPhaseRemoved    AddonPhase = "Removed"
    AddonPhaseReady      AddonPhase = "Ready"
    AddonPhaseUnknown    AddonPhase = "Unknown"
)
```

### 5.4 AddonResource

```go
type AddonResource struct {
    APIVersion string `yaml:"apiVersion,omitempty"`
    Kind       string `yaml:"kind,omitempty"`
    Name       string `yaml:"name,omitempty"`
    Namespace  string `yaml:"namespace,omitempty"`
    UID        string `yaml:"uid,omitempty"`
    Created    bool   `yaml:"created,omitempty"`
}
```

### 5.5 Cluster 扩展

```go
type ClusterSpec struct {
    // ... 现有字段 ...
    Addons []AddonSpec `yaml:"addons,omitempty"` // 可选：addon 配置
}

type ClusterStatus struct {
    // ... 现有字段 ...
    Addons []AddonStatus `yaml:"addons,omitempty"` // Addon 状态追踪
}
```

### 5.6 新增条件类型

```go
const (
    ConditionTypeAddonsInstalled ConditionType = "AddonsInstalled"
    ConditionTypeAddonReady      ConditionType = "AddonReady"
    ConditionTypeAddonInstalled  ConditionType = "AddonInstalled"
    ConditionTypeAddonFailed     ConditionType = "AddonFailed"
)
```

## 6 配置文件示例

### 6.1 完整配置文件

```yaml
apiVersion: ohmykube.dev/v1alpha1
kind: Cluster
metadata:
  name: my-cluster
spec:
  kubernetesVersion: v1.33.0
  provider: lima
  addons:
    # Prometheus 监控栈（Helm）
    - name: "prometheus"
      type: "helm"
      version: "15.18.0"
      enabled: true
      repo: "https://prometheus-community.github.io/helm-charts"
      chart: "prometheus-community/kube-prometheus-stack"
      namespace: "monitoring"
      priority: 100
      timeout: "600s"
      values:
        "grafana.adminPassword": "admin123"
        "grafana.persistence.enabled": "true"
      dependencies: ["metrics-server"]
      preInstall:
        - "kubectl create namespace monitoring --dry-run=client -o yaml | kubectl apply -f -"
      postInstall:
        - "kubectl -n monitoring wait --for=condition=available deployment/prometheus-kube-prometheus-prometheus-operator --timeout=300s"

    # Metrics Server（Helm）
    - name: "metrics-server"
      type: "helm"
      version: "3.8.2"
      repo: "https://kubernetes-sigs.github.io/metrics-server/"
      chart: "metrics-server/metrics-server"
      namespace: "kube-system"
      priority: 50
      values:
        "args[0]": "--kubelet-insecure-tls"
        "args[1]": "--kubelet-preferred-address-types=InternalIP"

    # Ingress Nginx（Manifest）
    - name: "ingress-nginx"
      type: "manifest"
      version: "v1.5.1"
      enabled: false
      files:
        - "https://raw.githubusercontent.com/kubernetes/ingress-nginx/controller-v1.5.1/deploy/static/provider/cloud/deploy.yaml"
      namespace: "ingress-nginx"

    # 多文件 Manifest
    - name: "custom-app"
      type: "manifest"
      version: "v1.0.0"
      files:
        - "/path/to/deployment.yaml"
        - "/path/to/service.yaml"
        - "/path/to/configmap.yaml"
      namespace: "default"
      dependencies: ["prometheus"]
```

### 6.2 JSON 格式说明

**Helm 类型（最小字段）**:

```json
{
  "name": "应用名称",
  "type": "helm",
  "version": "chart版本",
  "repo": "helm仓库URL",
  "chart": "chart名称"
}
```

**Manifest 类型（最小字段）**:

```json
{
  "name": "应用名称",
  "type": "manifest",
  "version": "应用版本",
  "files": ["manifest文件路径或URL"]
}
```

## 7 错误处理

### 7.1 安装失败策略

- Addon 安装失败**不阻断**集群创建流程，仅记录错误
- 每个 Addon 独立安装，单个失败不影响其他 Addon
- 镜像缓存失败仅警告，不阻断安装

### 7.2 状态追踪

- 每个安装步骤更新 `AddonPhase`，状态实时持久化到集群配置
- 失败时记录详细的 `Message` 和 `Reason`

### 7.3 钩子执行

- Pre/Post-install 钩子按顺序执行
- 任一钩子失败则整个安装标记为 `Failed`

## 8 关联文档

- [项目需求设计](./project-requirements-design.md)
- [镜像缓存设计](./image-cache-design.md)
