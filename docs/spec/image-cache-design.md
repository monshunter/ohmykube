# 镜像缓存系统设计文档

> **版本**: 1.0
> **状态**: active
> **更新日期**: 2026-03-05

## 1 概述

镜像缓存系统旨在提高 OhMyKube 中容器镜像管理的效率和可靠性，通过本地缓存和优化分发机制，避免重复下载镜像，加速集群创建和节点扩容。

## 2 设计目标

- 本地缓存容器镜像，避免重复下载
- 支持多种镜像源（Helm Chart、Kubernetes Manifest、kubeadm、容器镜像）
- YAML 索引文件追踪缓存镜像元数据
- 通过 SSH 上传缓存镜像到目标节点
- 上传前验证镜像是否已存在，避免不必要传输
- 支持多架构（amd64、arm64）
- 与现有 CNI/CSI/LB 插件无缝集成

## 3 架构设计

### 3.1 核心组件

| 组件 | 文件 | 职责 |
|------|------|------|
| ImageManager | `pkg/cache/image_manager.go` | 镜像缓存管理（单例模式） |
| ImageDiscovery | `pkg/cache/image_discovery.go` | 从多种源发现所需镜像 |
| ToolDetector | `pkg/cache/tool_detector.go` | 检测节点可用工具，确定最优策略 |
| Types | `pkg/cache/types.go` | 核心数据结构定义 |

### 3.2 策略选择机制

系统根据节点上可用工具自动选择最优的镜像管理策略：

| 策略 | 条件 | 说明 |
|------|------|------|
| `ImageManagementAuto` | 默认 | 自动检测最佳策略 |
| `ImageManagementController` | 需要 nerdctl | 在 Controller 节点执行镜像操作 |
| `ImageManagementTarget` | 始终可用 | 直接在目标节点操作（兜底方案） |

**自动策略选择逻辑**:
1. 检测 Controller 是否有 nerdctl → 使用 Controller 策略
2. 回退到 Target 策略（始终可用）

**镜像源与工具依赖**:

| 镜像源类型 | 所需工具 |
|-----------|---------|
| helm | helm |
| manifest | curl（远程 URL） |
| kubeadm | kubeadm |
| container | nerdctl |

### 3.3 镜像缓存与分发流程

1. **架构检测**: `uname -m` → 标准化（x86_64→amd64, aarch64→arm64）
2. **镜像发现**: 根据源类型提取镜像引用列表
3. **拉取与缓存**: 在 Controller 节点 `nerdctl pull` + `nerdctl save`，下载到本地缓存
4. **上传分发**: 本地 tar 文件通过 SSH 上传到目标节点
5. **加载验证**: `ctr load` 加载镜像，验证是否成功

### 3.4 镜像发现机制

#### 3.4.1 Helm Chart 发现

1. 添加 Helm repo 并更新
2. 在 Controller 节点执行 `helm template` 渲染 Chart
3. 从渲染输出中提取镜像引用
4. 自动去重

#### 3.4.2 Manifest 发现

1. URL 文件通过 `curl` 下载到 Controller
2. 本地文件直接读取
3. 目录递归收集 `.yaml`/`.yml` 文件
4. 拼接所有 manifest 并提取镜像引用

#### 3.4.3 kubeadm 发现

1. 执行 `kubeadm config images list --kubernetes-version VERSION`
2. 解析输出获取镜像列表

## 4 接口定义

### 4.1 公开接口

#### 4.1.1 ImageManager

```go
// 初始化
func NewImageManager() (*ImageManager, error)
func NewImageManagerWithConfig(config ImageManagementConfig) (*ImageManager, error)
func GetImageManager() (*ImageManager, error) // 单例

// 核心操作
func (m *ImageManager) EnsureImages(ctx context.Context, source ImageSource, sshRunner SSHRunner, nodeName string, controllerNode string) error
func (m *ImageManager) EnsureImage(ctx context.Context, image string, sshRunner SSHRunner, nodeName string, controllerNode string) error
func (m *ImageManager) UploadImageToNode(ctx context.Context, ref ImageReference, sshRunner SSHRunner, nodeName string) error

// 集群预热
func (m *ImageManager) ReCacheClusterImages(sshRunner SSHRunner) error
func (m *ImageManager) CacheClusterImagesForNodes(nodeNames []string, sshRunner SSHRunner) error

// 查询
func (m *ImageManager) IsImageCached(cacheKey string) bool

// 配置
func (m *ImageManager) SetImageRecorder(imageRecorder interfaces.ImageRecorder)
func SetGlobalImageRecorder(imageRecorder interfaces.ImageRecorder)
```

#### 4.1.2 ImageDiscovery

```go
func GetRequiredImages(ctx context.Context, source ImageSource, sshRunner SSHRunner, controllerNode string) ([]string, error)
```

#### 4.1.3 ToolDetector

```go
func DetectToolAvailability(ctx context.Context, sshRunner SSHRunner, controllerNode string) (*ToolAvailability, error)
func DetermineOptimalStrategy(availability *ToolAvailability, config ImageManagementConfig) ImageManagementStrategy
func DetermineOptimalStrategyForSource(source ImageSource, availability *ToolAvailability, config ImageManagementConfig) ImageManagementStrategy
func CanStrategyHandleSource(strategy ImageManagementStrategy, source ImageSource, availability *ToolAvailability) bool
```

### 4.2 内部接口

#### 4.2.1 插件集成模式

参考 `pkg/addons/plugins/cni/flannel.go` 的集成方式：

```go
// 1. 构造 ImageSource
source := cache.ImageSource{
    Type:      "helm",
    ChartName: "flannel/flannel",
    Version:   "v0.26.7",
    ChartValues: map[string]string{
        "podCidr": "10.244.0.0/16",
    },
}

// 2. 获取单例 ImageManager
imageManager, _ := cache.GetImageManager()

// 3. 缓存镜像
imageManager.EnsureImages(ctx, source, sshRunner, nodeName, controllerNode)

// 4. 集群预热
imageManager.ReCacheClusterImages(sshRunner)
```

## 5 数据结构

### 5.1 ImageSource

```go
type ImageSource struct {
    Type         string            // "helm", "manifest", "kubeadm", "container"
    ChartName    string            // Helm chart 名称
    ChartRepo    string            // Helm 仓库 URL
    Version      string            // 版本
    ChartValues  map[string]string // Helm values
    IsLocalChart bool              // 是否为本地 chart
    ValuesFile   []string          // Values 文件路径
    ManifestFiles []string         // Manifest 文件路径或 URL
}
```

### 5.2 ImageReference

```go
type ImageReference struct {
    Registry string // 例: "docker.io"
    Project  string // 例: "library"
    Image    string // 例: "nginx"
    Tag      string // 例: "1.21"（默认 "latest"）
    Digest   string // SHA256 摘要
    Original string // 原始完整引用
    Arch     string // 目标架构
}
```

**关键方法**:
- `String()` — 完整镜像引用（例: `docker.io/library/nginx:1.21`）
- `NormalizedName()` — 安全文件名（替换 `/`、`:`、`@` 为 `_`）
- `CacheKey()` — 唯一标识（含架构信息）

### 5.3 ImageInfo

```go
type ImageInfo struct {
    Name             string         // 缓存键
    Reference        ImageReference // 镜像引用
    LocalPath        string         // 本地文件路径
    Size             int64          // 压缩大小（字节）
    LastAccessed     time.Time      // 最后访问时间
    LastUpdated      time.Time      // 最后更新时间
    OriginalSize     int64          // 未压缩大小
    CompressionRatio float64        // 压缩率
    Architectures    []string       // 支持的架构列表
}
```

### 5.4 ImageIndex

```go
type ImageIndex struct {
    Version   string      // 索引格式版本（"1.0"）
    Images    []ImageInfo // 缓存镜像列表
    UpdatedAt time.Time   // 最后修改时间
}
```

存储位置: `~/.ohmykube/cache/images/index.yaml`

### 5.5 ToolAvailability

```go
type ToolAvailability struct {
    Nerdctl  bool // nerdctl 可用
    Helm     bool // helm 可用
    Kubeadm  bool // kubeadm 可用
    Curl     bool // curl 可用
}
```

## 6 缓存存储结构

```
~/.ohmykube/cache/images/
├── index.yaml                                    # 元数据索引
├── docker.io_library_nginx_1.21_amd64.tar       # 缓存的镜像文件
├── quay.io_cilium_cilium_1.14_arm64.tar
└── registry.k8s.io_pause_3.9_amd64.tar
```

## 7 设计模式

| 模式 | 实现 |
|------|------|
| 单例 | ImageManager 通过 `sync.Once` 实现 |
| 信号量 | 按镜像粒度的拉取锁，防止重复并发拉取 |
| 惰性发现 | 在 `EnsureImages` 时按需发现 |
| 线程安全索引 | `RWMutex` 保护 ImageIndex 操作 |
| 架构映射 | 按架构变体分别缓存 |
| 回退策略 | Auto → Controller → Target 偏好顺序 |
| 非阻塞 | 插件安装在缓存失败时继续执行 |

## 8 错误处理

- 镜像缓存失败不阻断安装流程，仅记录警告
- 上传失败可重试，支持断点恢复
- 镜像文件大小验证（> 100KB 视为有效）
- 单个镜像失败不影响其他镜像的缓存

## 9 已集成的插件

| 插件 | 文件 |
|------|------|
| Flannel CNI | `pkg/addons/plugins/cni/flannel.go` |
| Cilium CNI | `pkg/addons/plugins/cni/cilium.go` |
| Rook-Ceph CSI | `pkg/addons/plugins/csi/rook.go` |
| Local-Path CSI | `pkg/addons/plugins/csi/local_path.go` |
| MetalLB LB | `pkg/addons/plugins/lb/metallb.go` |
| Kube 工具集成 | `pkg/kube/kube.go` |

## 10 关联文档

- [项目需求设计](./project-requirements-design.md)
- [Addon 系统设计](./addon-system-design.md)
