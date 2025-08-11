# OhMyKube Addon System Implementation Plan

## 背景与目标

基于 `docs/pre_install_requirement.md` 中的需求，设计一个**通用的 Addon 系统**，支持在 Kubernetes 集群中安装任意应用，包括但不限于 metrics-server、prometheus、grafana 等常用应用。

### 核心设计原则
- **通用性优先**: 支持任意 Helm Chart 和 Kubernetes Manifest 应用，不限定特定软件
- **最小化复杂度**: 命令行参数保持极简，复杂配置留给配置文件
- **复用现有架构**: 直接使用现有的镜像缓存机制，遵循现有 CNI/CSI/LB 插件模式
- **极简数据结构**: 只包含安装必需的最小字段集

### 核心目标
- 通过单一 `--addon` 参数支持任意应用安装
- 复用现有镜像缓存系统，无需额外封装  
- 支持 Helm Chart 和 Kubernetes Manifest 两种类型
- 与现有集群创建流程无缝集成

## 架构设计

### 1. 极简数据结构设计

#### 1.1 完整的 Addon 规范结构

```go
// pkg/config/addon.go

// AddonSpec 定义应用安装的完整配置
// 注意：命令行 JSON 只需要最少字段，但配置文件支持完整的自定义选项
type AddonSpec struct {
    // 基础必需字段 (命令行 JSON 最少需要这些)
    Name    string `json:"name" yaml:"name"`       // 应用名称
    Type    string `json:"type" yaml:"type"`       // "helm" 或 "manifest"
    Version string `json:"version" yaml:"version"` // 版本信息
    Enabled *bool   `json:"enabled" yaml:"enabled"` // 是否启用 (默认 true)
    
    // Manifest 类型字段
    URL   string   `json:"url,omitempty" yaml:"url,omitempty"`     // Manifest 文件 URL
    Files []string `json:"files,omitempty" yaml:"files,omitempty"` // 多个 manifest 文件路径
    
    // Helm 类型字段
    Repo       string            `json:"repo,omitempty" yaml:"repo,omitempty"`             // Helm repository URL
    Chart      string            `json:"chart,omitempty" yaml:"chart,omitempty"`           // Helm chart 名称
    Values     map[string]string `json:"values,omitempty" yaml:"values,omitempty"`         // Helm values 覆盖
    ValuesFile string            `json:"valuesFile,omitempty" yaml:"valuesFile,omitempty"` // Values 文件路径
    
    // 高级配置字段 (配置文件中的完整自定义选项)
    Namespace    string            `json:"namespace,omitempty" yaml:"namespace,omitempty"`       // 目标 namespace
    Priority     int               `json:"priority,omitempty" yaml:"priority,omitempty"`         // 安装优先级 (数字越小越优先)
    Dependencies []string          `json:"dependencies,omitempty" yaml:"dependencies,omitempty"` // 依赖的其他 addon
    Timeout      string            `json:"timeout,omitempty" yaml:"timeout,omitempty"`           // 安装超时时间 (默认 300s)
    Labels       map[string]string `json:"labels,omitempty" yaml:"labels,omitempty"`             // 应用标签
    Annotations  map[string]string `json:"annotations,omitempty" yaml:"annotations,omitempty"`   // 应用注解
    
    // 自定义安装配置
    PreInstall  []string `json:"preInstall,omitempty" yaml:"preInstall,omitempty"`   // 安装前执行的命令
    PostInstall []string `json:"postInstall,omitempty" yaml:"postInstall,omitempty"` // 安装后执行的命令
}

// 最小验证（命令行 JSON 需要的最少字段）
func (a *AddonSpec) ValidateMinimal() error {
    if a.Name == "" || a.Type == "" || a.Version == "" {
        return fmt.Errorf("name, type and version are required")
    }
    
    switch a.Type {
    case "helm":
        if a.Repo == "" || a.Chart == "" {
            return fmt.Errorf("repo and chart are required for helm type addon")
        }
    case "manifest":
        if a.URL == "" && len(a.Files) == 0 {
            return fmt.Errorf("url or files are required for manifest type addon")
        }
    default:
        return fmt.Errorf("unsupported addon type: %s (supported: helm, manifest)", a.Type)
    }
    return nil
}

// 完整验证（配置文件使用）
func (a *AddonSpec) Validate() error {
    if err := a.ValidateMinimal(); err != nil {
        return err
    }
    
    // 设置默认值
    if a.Enabled == nil  {
        a.Enabled = new(bool) // 默认启用
        *a.Enabled = true
    }
    if a.Timeout == "" {
        a.Timeout = "300s" // 默认超时
    }
    if a.Priority == 0 {
        a.Priority = 100 // 默认优先级
    }
    
    return nil
}
```

#### 1.2 完整的状态追踪系统

```go
// pkg/config/addon.go - 状态追踪结构

// AddonStatus 代表 addon 的运行时状态
type AddonStatus struct {
    Name             string               `yaml:"name,omitempty"`
    Phase            AddonPhase          `yaml:"phase,omitempty"`
    InstalledVersion string              `yaml:"installedVersion,omitempty"`
    DesiredVersion   string              `yaml:"desiredVersion,omitempty"`
    Namespace        string              `yaml:"namespace,omitempty"`
    InstallTime      *time.Time          `yaml:"installTime,omitempty"`
    LastUpdateTime   *time.Time          `yaml:"lastUpdateTime,omitempty"`
    Conditions       []Condition         `yaml:"conditions,omitempty"`
    Resources        []AddonResource     `yaml:"resources,omitempty"`     // 追踪的 K8s 资源
    Images           []string            `yaml:"images,omitempty"`        // 缓存的镜像列表
    Message          string              `yaml:"message,omitempty"`       // 状态消息
    Reason           string              `yaml:"reason,omitempty"`        // 状态原因
}

// AddonPhase 定义 addon 的生命周期状态
type AddonPhase string

const (
    AddonPhasePending     AddonPhase = "Pending"     // 等待安装
    AddonPhaseInstalling  AddonPhase = "Installing"  // 正在安装
    AddonPhaseInstalled   AddonPhase = "Installed"   // 已安装
    AddonPhaseUpgrading   AddonPhase = "Upgrading"   // 正在升级
    AddonPhaseUpgraded    AddonPhase = "Upgraded"    // 已升级
    AddonPhaseFailed      AddonPhase = "Failed"      // 安装/升级失败
    AddonPhaseRemoving    AddonPhase = "Removing"    // 正在删除
    AddonPhaseRemoved     AddonPhase = "Removed"     // 已删除
    AddonPhaseUnknown     AddonPhase = "Unknown"     // 状态未知
)

// AddonResource 追踪由 addon 创建的 K8s 资源
type AddonResource struct {
    APIVersion string `yaml:"apiVersion,omitempty"`
    Kind       string `yaml:"kind,omitempty"`
    Name       string `yaml:"name,omitempty"`
    Namespace  string `yaml:"namespace,omitempty"`
    UID        string `yaml:"uid,omitempty"`        // K8s 资源 UID
    Created    bool   `yaml:"created,omitempty"`    // 是否已创建
}

// 新增 addon 相关条件类型
const (
    // 现有条件...
    ConditionTypeAddonsInstalled  ConditionType = "AddonsInstalled"
    ConditionTypeAddonReady       ConditionType = "AddonReady"
    ConditionTypeAddonInstalled   ConditionType = "AddonInstalled"
    ConditionTypeAddonFailed      ConditionType = "AddonFailed"
)
```

#### 1.3 扩展 Cluster 配置和状态管理

```go
// pkg/config/cluster.go - 扩展现有结构

type ClusterSpec struct {
    KubernetesVersion string           `yaml:"kubernetesVersion,omitempty"`
    Provider          string           `yaml:"provider,omitempty"`
    UpdateSystem      bool             `yaml:"updateSystem,omitempty"`
    Networking        NetworkingConfig `yaml:"networking,omitempty"`
    Storage           StorageConfig    `yaml:"storage,omitempty"`
    Nodes             NodesConfig      `yaml:"nodes,omitempty"`
    Addons            []AddonSpec      `yaml:"addons,omitempty"`        // 可选：addon 配置
}

type ClusterStatus struct {
    Phase      ClusterPhase      `yaml:"phase,omitempty"`
    Auth       Auth              `yaml:"auth,omitempty"`
    Images     Images            `yaml:"images,omitempty"`
    Nodes      []NodeGroupStatus `yaml:"nodes,omitempty"`
    Conditions []Condition       `yaml:"conditions,omitempty"`
    Addons     []AddonStatus     `yaml:"addons,omitempty"`     // Addon 状态追踪
}

// Addon 状态管理方法
func (c *Cluster) SetAddonStatus(addonName string, status AddonStatus) { /* ... */ }
func (c *Cluster) GetAddonStatus(addonName string) (*AddonStatus, bool) { /* ... */ }
func (c *Cluster) SetAddonPhase(addonName string, phase AddonPhase, message, reason string) { /* ... */ }
func (c *Cluster) ListAddonStatuses() []AddonStatus { /* ... */ }
func (c *Cluster) AreAllAddonsReady() bool { /* ... */ }
```

### 2. 镜像缓存集成

#### 2.1 直接复用现有镜像缓存系统

**设计原则**: 不创建额外的封装层，直接使用现有的 `ImageManager` 和 `ImageSource` 接口，完全按照现有 CNI/CSI/LB 插件的模式。

```go
// 参考现有 pkg/addons/plugins/cni/flannel.go 的实现模式

// 在通用安装器中缓存镜像 
func (u *UniversalInstaller) cacheImages() error {
    ctx := context.Background()
    
    // 1. 获取现有的 ImageManager (完全复用)
    imageManager, err := cache.GetImageManager()
    if err != nil {
        return fmt.Errorf("failed to get image manager: %w", err)
    }
    
    // 2. 根据 addon 类型构造对应的 ImageSource (复用现有结构)
    var source interfaces.ImageSource
    switch u.spec.Type {
    case "helm":
        source = interfaces.ImageSource{
            Type:      "helm", 
            ChartName: u.spec.Chart,
            ChartRepo: u.spec.Repo,
            Version:   u.spec.Version,
        }
    case "manifest":
        source = interfaces.ImageSource{
            Type:        "manifest",
            ManifestURL: u.spec.URL,
            Version:     u.spec.Version,
        }
    }
    
    // 3. 直接调用现有方法进行镜像缓存
    return imageManager.EnsureImages(ctx, source, u.sshRunner, u.controllerNode, u.controllerNode)
}
```

**关键点**: 
- 不修改现有 `ImageSource` 接口
- 不创建新的 `AddonImageDiscovery` 类
- 直接映射 `AddonSpec` 到现有的 `ImageSource` 格式
- 完全复用现有镜像发现和缓存逻辑

### 3. 增强的安装器设计 - 支持状态追踪和高级功能

#### 3.1 完整的通用安装器实现

```go
// pkg/addons/installer.go - 增强版安装器，支持状态追踪和完整功能

// UniversalInstaller 通用应用安装器
type UniversalInstaller struct {
    sshRunner      interfaces.SSHRunner
    controllerNode string
    spec           config.AddonSpec
    cluster        *config.Cluster
}

func NewUniversalInstaller(sshRunner interfaces.SSHRunner, controllerNode string, spec config.AddonSpec, cluster *config.Cluster) *UniversalInstaller {
    return &UniversalInstaller{
        sshRunner:      sshRunner,
        controllerNode: controllerNode,
        spec:           spec,
        cluster:        cluster,
    }
}

func (u *UniversalInstaller) Install() error {
    log.Infof("Installing addon: %s (type: %s, version: %s)", u.spec.Name, u.spec.Type, u.spec.Version)
    
    // 1. 设置初始状态
    u.setAddonPhase(config.AddonPhaseInstalling, "Starting installation", "Installing")
    
    // 2. 执行 pre-install 钩子
    if err := u.executeHooks(u.spec.PreInstall, "pre-install"); err != nil {
        u.setAddonPhase(config.AddonPhaseFailed, fmt.Sprintf("Pre-install hook failed: %v", err), "PreInstallFailed")
        return fmt.Errorf("pre-install hook failed: %w", err)
    }
    
    // 3. 缓存镜像
    u.setAddonPhase(config.AddonPhaseInstalling, "Caching images", "CachingImages")
    if err := u.cacheImages(); err != nil {
        log.Warningf("Failed to cache images for %s: %v", u.spec.Name, err)
    }
    
    // 4. 执行安装
    u.setAddonPhase(config.AddonPhaseInstalling, "Installing application", "Installing")
    
    var err error
    switch u.spec.Type {
    case "helm":
        err = u.installHelm()
    case "manifest":
        err = u.installManifest()
    default:
        err = fmt.Errorf("unsupported addon type: %s", u.spec.Type)
    }
    
    if err != nil {
        u.setAddonPhase(config.AddonPhaseFailed, fmt.Sprintf("Installation failed: %v", err), "InstallationFailed")
        return err
    }
    
    // 5. 执行 post-install 钩子
    if err := u.executeHooks(u.spec.PostInstall, "post-install"); err != nil {
        u.setAddonPhase(config.AddonPhaseFailed, fmt.Sprintf("Post-install hook failed: %v", err), "PostInstallFailed")
        return fmt.Errorf("post-install hook failed: %w", err)
    }
    
    // 6. 验证安装结果
    u.setAddonPhase(config.AddonPhaseInstalling, "Verifying installation", "Verifying")
    if err := u.verifyInstallation(); err != nil {
        u.setAddonPhase(config.AddonPhaseFailed, fmt.Sprintf("Verification failed: %v", err), "VerificationFailed")
        return fmt.Errorf("verification failed: %w", err)
    }
    
    // 7. 设置最终状态
    u.setFinalStatus()
    
    log.Infof("✅ Addon %s installed successfully", u.spec.Name)
    return nil
}

func (u *UniversalInstaller) installHelm() error {
    // 1. 添加 Helm repository
    repoName := u.getRepoName()
    addRepoCmd := fmt.Sprintf(`helm repo add %s %s || true && helm repo update`, repoName, u.spec.Repo)
    
    _, err := u.sshRunner.RunCommand(u.controllerNode, addRepoCmd)
    if err != nil {
        return fmt.Errorf("failed to add helm repository: %w", err)
    }
    
    // 2. 构建 helm install 命令
    installCmd := u.buildHelmInstallCommand()
    
    _, err = u.sshRunner.RunCommand(u.controllerNode, installCmd)
    if err != nil {
        return fmt.Errorf("failed to install helm chart: %w", err)
    }
    
    return nil
}

func (u *UniversalInstaller) buildHelmInstallCommand() string {
    cmd := fmt.Sprintf("helm install %s %s --version %s", u.spec.Name, u.spec.Chart, u.spec.Version)
    
    // 添加 namespace
    if u.spec.Namespace != "" {
        cmd += fmt.Sprintf(" --namespace %s --create-namespace", u.spec.Namespace)
    }
    
    // 添加 values
    for key, value := range u.spec.Values {
        cmd += fmt.Sprintf(" --set %s=%s", key, value)
    }
    
    // 添加 values 文件
    if u.spec.ValuesFile != "" {
        cmd += fmt.Sprintf(" --values %s", u.spec.ValuesFile)
    }
    
    // 添加超时
    timeout := u.spec.Timeout
    if timeout == "" {
        timeout = "300s"
    }
    cmd += fmt.Sprintf(" --wait --timeout %s", timeout)
    
    return cmd
}

func (u *UniversalInstaller) installManifest() error {
    var installCmd string
    
    if u.spec.URL != "" {
        installCmd = fmt.Sprintf("kubectl apply -f %s", u.spec.URL)
    } else if len(u.spec.Files) > 0 {
        installCmd = fmt.Sprintf("kubectl apply -f %s", strings.Join(u.spec.Files, " -f "))
    } else {
        return fmt.Errorf("no URL or files specified for manifest addon")
    }
    
    // 添加 namespace
    if u.spec.Namespace != "" {
        installCmd += fmt.Sprintf(" --namespace %s", u.spec.Namespace)
    }
    
    _, err := u.sshRunner.RunCommand(u.controllerNode, installCmd)
    if err != nil {
        return fmt.Errorf("failed to apply manifest: %w", err)
    }
    return nil
}

func (u *UniversalInstaller) setAddonPhase(phase config.AddonPhase, message, reason string) {
    u.cluster.SetAddonPhase(u.spec.Name, phase, message, reason)
    
    // 保存集群状态
    if err := u.cluster.Save(); err != nil {
        log.Warningf("Failed to save cluster state: %v", err)
    }
}

func (u *UniversalInstaller) executeHooks(hooks []string, hookType string) error {
    for i, hook := range hooks {
        log.Debugf("Executing %s hook %d/%d for %s: %s", hookType, i+1, len(hooks), u.spec.Name, hook)
        
        _, err := u.sshRunner.RunCommand(u.controllerNode, hook)
        if err != nil {
            return fmt.Errorf("hook %d failed: %w", i+1, err)
        }
    }
    return nil
}

func (u *UniversalInstaller) verifyInstallation() error {
    // 根据类型验证安装
    switch u.spec.Type {
    case "helm":
        return u.verifyHelmInstallation()
    case "manifest":
        return u.verifyManifestInstallation()
    default:
        return fmt.Errorf("unsupported addon type: %s", u.spec.Type)
    }
}

func (u *UniversalInstaller) setFinalStatus() {
    now := time.Now()
    status := config.AddonStatus{
        Name:             u.spec.Name,
        Phase:            config.AddonPhaseInstalled,
        InstalledVersion: u.spec.Version,
        DesiredVersion:   u.spec.Version,
        Namespace:        u.spec.Namespace,
        InstallTime:      &now,
        LastUpdateTime:   &now,
        Message:          "Installation completed successfully",
        Reason:           "Installed",
    }
    
    u.cluster.SetAddonStatus(u.spec.Name, status)
}

// cacheImages 完全按照现有插件的模式实现镜像缓存
func (u *UniversalInstaller) cacheImages() error {
    ctx := context.Background()
    
    // 获取现有的 ImageManager
    imageManager, err := cache.GetImageManager()
    if err != nil {
        return fmt.Errorf("failed to get image manager: %w", err)
    }
    
    // 根据类型构造 ImageSource
    var source interfaces.ImageSource
    switch u.spec.Type {
    case "helm":
        source = interfaces.ImageSource{
            Type:        "helm",
            ChartName:   u.spec.Chart,
            ChartRepo:   u.spec.Repo,
            Version:     u.spec.Version,
            ChartValues: u.spec.Values,
        }
    case "manifest":
        source = interfaces.ImageSource{
            Type:        "manifest",
            ManifestURL: u.spec.URL,
            Version:     u.spec.Version,
        }
    }
    
    // 调用现有的镜像缓存方法
    return imageManager.EnsureImages(ctx, source, u.sshRunner, u.controllerNode, u.controllerNode)
}
```

### 4. 极简命令行接口

#### 4.1 单一通用参数设计

```go
// cmd/ohmykube/app/up.go - 极简参数设计

var (
    addonsFlag []string // --addon (可重复使用)
)

func init() {
    // 现有 flag 初始化...
    upCmd.Flags().StringSliceVar(&addonsFlag, "addon", nil, "Add addon from JSON spec (repeatable)")
}

// 处理 addon 参数
func processAddonFlags() ([]config.AddonSpec, error) {
    var specs []config.AddonSpec
    
    for _, addonJSON := range addonsFlag {
        var addon config.AddonSpec
        if err := json.Unmarshal([]byte(addonJSON), &addon); err != nil {
            return nil, fmt.Errorf("invalid addon JSON: %s, error: %w", addonJSON, err)
        }
        
        // 验证 addon 规范 (使用最小验证，适用于命令行)
        if err := addon.ValidateMinimal(); err != nil {
            return nil, fmt.Errorf("invalid addon spec: %w", err)
        }
        
        specs = append(specs, addon)
    }
    
    return specs, nil
}
```

#### 4.2 集成到集群创建流程

```go
// cmd/ohmykube/app/up.go - 集成到现有流程

func runUp(cmd *cobra.Command, args []string) {
    // ... 现有集群创建逻辑 ...
    
    // 处理 addon 参数
    addonSpecs, err := processAddonFlags()
    if err != nil {
        log.Fatalf("❌ Invalid addon configuration: %v", err)
    }
    
    // ... 集群创建完成 ...
    
    // 安装 addons (如果有指定)
    if len(addonSpecs) > 0 {
        if err := installAddons(sshRunner, cls.GetMasterName(), addonSpecs); err != nil {
            log.Errorf("❌ Failed to install addons: %v", err)
            // 不阻断集群创建，addon 失败只是警告
        }
    }
}

// 安装 addons
func installAddons(sshRunner interfaces.SSHRunner, masterNode string, specs []config.AddonSpec, cluster *config.Cluster) error {
    for _, spec := range specs {
        log.Infof("🔌 Installing addon: %s", spec.Name)
        
        installer := addons.NewUniversalInstaller(sshRunner, masterNode, spec, cluster)
        if err := installer.Install(); err != nil {
            return fmt.Errorf("failed to install %s: %w", spec.Name, err)
        }
        
        log.Infof("✅ Addon %s installed successfully", spec.Name)
    }
    return nil
}

### 5. 新增 addon 管理命令

#### 5.1 完整的 addon 管理 CLI

```go
// cmd/ohmykube/app/addon.go - 完整的 addon 管理命令

var addonCmd = &cobra.Command{
    Use:   "addon",
    Short: "Manage cluster addons",
    Long:  "Install, upgrade, remove and manage cluster addons",
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

func init() {
    rootCmd.AddCommand(addonCmd)
    addonCmd.AddCommand(addonListCmd)
    addonCmd.AddCommand(addonStatusCmd)
}

func runAddonList(cmd *cobra.Command, args []string) error {
    cluster := getCurrentCluster()
    if cluster == nil {
        return fmt.Errorf("no active cluster found")
    }
    
    statuses := cluster.ListAddonStatuses()
    if len(statuses) == 0 {
        fmt.Println("No addons installed")
        return nil
    }
    
    // 格式化输出 addon 列表
    printAddonTable(statuses)
    return nil
}

func printAddonTable(statuses []config.AddonStatus) {
    fmt.Printf("%-20s %-12s %-15s %-15s %-20s\n", "NAME", "PHASE", "INSTALLED", "DESIRED", "LAST UPDATE")
    fmt.Println(strings.Repeat("-", 90))
    
    for _, status := range statuses {
        lastUpdate := "Never"
        if status.LastUpdateTime != nil {
            lastUpdate = status.LastUpdateTime.Format("2006-01-02 15:04:05")
        }
        
        fmt.Printf("%-20s %-12s %-15s %-15s %-20s\n",
            status.Name,
            status.Phase,
            status.InstalledVersion,
            status.DesiredVersion,
            lastUpdate,
        )
    }
}
```

### 6. 配置文件模板扩展

#### 6.1 扩展现有配置生成 - 支持完整的自定义选项

```go
// pkg/config/template.go - 在现有模板中添加 addon 示例

func generateConfigTemplate() string {
    return `apiVersion: ohmykube.dev/v1alpha1
kind: Cluster
metadata:
  name: my-cluster
spec:
  kubernetesVersion: v1.33.0
  provider: lima
  updateSystem: true
  networking:
    cni: flannel
    proxyMode: iptables
    loadbalancer: ""
  storage:
    csi: local-path
  nodes:
    master:
      - replica: 1
        groupid: 1
        template: ubuntu-24.04
        resources:
          cpu: "2"
          memory: 4Gi
          storage: 50Gi
    workers:
      - replica: 2
        groupid: 2
        template: ubuntu-24.04
        resources:
          cpu: "2"
          memory: 4Gi
          storage: 50Gi
  # 可选：Addon 配置示例 (支持完整的自定义选项)
  addons:
    # Prometheus 监控栈 (Helm) - 完整配置示例
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
        "prometheus.prometheusSpec.storageSpec.volumeClaimTemplate.spec.resources.requests.storage": "10Gi"
        "grafana.persistence.enabled": "true"
        "grafana.persistence.size": "1Gi"
      labels:
        category: "monitoring"
        team: "platform"
      dependencies: ["metrics-server"]
      preInstall:
        - "kubectl create namespace monitoring --dry-run=client -o yaml | kubectl apply -f -"
      postInstall:
        - "kubectl -n monitoring wait --for=condition=available deployment/prometheus-kube-prometheus-prometheus-operator --timeout=300s"
    
    # Metrics Server (Helm) - 基础配置
    - name: "metrics-server"
      type: "helm"
      version: "3.8.2"
      enabled: true
      repo: "https://kubernetes-sigs.github.io/metrics-server/"
      chart: "metrics-server/metrics-server"
      namespace: "kube-system"
      priority: 50
      values:
        "args[0]": "--kubelet-insecure-tls"
        "args[1]": "--kubelet-preferred-address-types=InternalIP"
    
    # Ingress Nginx (Manifest) - 基础配置
    - name: "ingress-nginx"
      type: "manifest"
      version: "v1.5.1"
      enabled: false
      url: "https://raw.githubusercontent.com/kubernetes/ingress-nginx/controller-v1.5.1/deploy/static/provider/cloud/deploy.yaml"
      namespace: "ingress-nginx"
      priority: 200
      timeout: "300s"
    
    # 多文件 Manifest 示例
    - name: "custom-app"
      type: "manifest"
      version: "v1.0.0"
      enabled: false
      files:
        - "/path/to/deployment.yaml"
        - "/path/to/service.yaml"
        - "/path/to/configmap.yaml"
      namespace: "default"
      priority: 300
      dependencies: ["prometheus"]
      preInstall:
        - "kubectl create configmap app-config --from-literal=env=production --dry-run=client -o yaml | kubectl apply -f -"
      postInstall:
        - "kubectl rollout status deployment/custom-app --timeout=300s"
`
}
```

## 使用示例

### 1. 命令行使用方式

#### 基础用法（最少字段）
```bash
# 单个 Helm 应用（最小 JSON）
ohmykube up my-cluster --addon '{"name":"prometheus","type":"helm","repo":"https://prometheus-community.github.io/helm-charts","chart":"prometheus-community/kube-prometheus-stack","version":"15.18.0"}'

# 单个 Manifest 应用（最小 JSON）
ohmykube up my-cluster --addon '{"name":"ingress-nginx","type":"manifest","url":"https://raw.githubusercontent.com/kubernetes/ingress-nginx/controller-v1.5.1/deploy/static/provider/cloud/deploy.yaml","version":"v1.5.1"}'

# 多个应用（多次使用 --addon）
ohmykube up my-cluster \
  --addon '{"name":"prometheus","type":"helm","repo":"https://prometheus-community.github.io/helm-charts","chart":"prometheus-community/kube-prometheus-stack","version":"15.18.0"}' \
  --addon '{"name":"metrics-server","type":"helm","repo":"https://kubernetes-sigs.github.io/metrics-server/","chart":"metrics-server/metrics-server","version":"3.8.2"}'
```

#### 高级用法（包含自定义选项）
```bash
# Helm 应用 - 带 namespace 和 values
ohmykube up my-cluster --addon '{"name":"prometheus","type":"helm","repo":"https://prometheus-community.github.io/helm-charts","chart":"prometheus-community/kube-prometheus-stack","version":"15.18.0","namespace":"monitoring","values":{"grafana.adminPassword":"admin123"}}'

# Manifest 应用 - 带自定义 namespace
ohmykube up my-cluster --addon '{"name":"ingress-nginx","type":"manifest","url":"https://raw.githubusercontent.com/kubernetes/ingress-nginx/controller-v1.5.1/deploy/static/provider/cloud/deploy.yaml","version":"v1.5.1","namespace":"ingress-nginx"}'
```

#### 推荐用法（配置文件 + addon 管理）
```bash
# 通过配置文件（复杂配置推荐方式）
ohmykube up -f cluster-with-addons.yaml

# 后续 addon 管理
ohmykube addon list
ohmykube addon status prometheus
```

### 2. JSON 格式说明

#### Helm 类型 addon:
```json
{
  "name": "应用名称",
  "type": "helm", 
  "version": "chart版本",
  "repo": "helm仓库URL",
  "chart": "chart名称"
}
```

#### Manifest 类型 addon:
```json
{
  "name": "应用名称",
  "type": "manifest",
  "version": "应用版本", 
  "url": "manifest文件URL"
}
```

## 实施路线图

### Phase 1: 核心基础 (第1周)
1. **数据结构实现**
   - 实现 `pkg/config/addon.go` 中的 `AddonSpec` 结构
   - 添加验证函数和基本方法
   - 扩展 `ClusterSpec` 支持 addons 字段

2. **命令行参数集成**
   - 在 `up` 命令中添加 `--addon` 参数
   - 实现 JSON 解析和验证逻辑

### Phase 2: 通用安装器 (第2周)  
1. **UniversalInstaller 实现**
   - 实现通用安装器基本框架
   - 添加 Helm 和 Manifest 两种安装方式
   - 集成现有镜像缓存机制

2. **集群创建流程集成**
   - 将 addon 安装集成到 `up` 命令流程中
   - 添加错误处理和日志输出

### Phase 3: 配置文件支持和优化 (第3周)
1. **配置文件模板扩展**
   - 扩展 `ohmykube config` 命令生成包含 addon 示例的配置文件
   - 添加配置文件中 addon 的加载和处理逻辑

2. **错误处理和稳定性**
   - 完善错误处理机制
   - 添加 addon 安装失败的回退逻辑
   - 优化日志输出和用户反馈

### Phase 4: 测试和文档 (第4周)
1. **测试覆盖**
   - 添加单元测试覆盖核心功能
   - 实现集成测试验证端到端流程
   - 测试各种错误场景

2. **文档和示例**
   - 完善用户文档和使用示例
   - 添加常用应用的 JSON 配置示例

## 设计优势

1. **极简设计**: 只有一个 `--addon` 参数，支持所有类型的应用
2. **完全通用**: 不限定特定软件，支持任意 Helm Chart 和 Manifest 应用  
3. **零封装**: 直接复用现有的镜像缓存系统，不创建额外抽象层
4. **一致模式**: 完全遵循现有 CNI/CSI/LB 插件的实现模式
5. **最小侵入**: 对现有代码结构的修改最小化
6. **向后兼容**: 不影响现有功能，addon 为完全可选特性

这种设计既满足了通用性要求，又保持了系统的简洁性和一致性。