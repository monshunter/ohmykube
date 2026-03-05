# OhMyKube 项目需求设计文档

> **版本**: 1.0
> **状态**: active
> **更新日期**: 2026-03-05

## 1 概述

### 1.1 背景

Kubernetes 本地开发环境越来越受开发者重视，社区提供了各种本地开发环境搭建工具（minikube、kind 等）。但这些工具主要基于 Docker 容器模拟 K8s 集群，与真实生产环境存在较大差异（如节点资源管理），导致弹性和调度优化的调试困难。

个人电脑（尤其是 Mac M 芯片系列）近年来规格日益强大，使得在单台计算机上虚拟化多个 Kubernetes 兼容节点成为可能。

### 1.2 使命

OhMyKube 弥合了容器化开发工具（如 kind、k3d）与生产级部署工具（如 kubespray、sealos）之间的差距，提供比容器更真实但比手动配置更简单的 Kubernetes 环境。

### 1.3 核心价值

- **真实虚拟机**: 使用独立 VM 而非容器运行 Kubernetes 节点，更接近生产环境
- **一键部署**: 简洁的命令行界面，快速创建、删除和扩展集群
- **生产级环境**: 真实的网络模型、存储系统和资源隔离
- **开发者友好**: 快速配置、便捷调试和全面的工具集成

### 1.4 目标用户

- **Kubernetes 开发者**: 在类生产环境中测试应用
- **平台工程师**: 学习和试验 Kubernetes 配置
- **DevOps 工程师**: 构建本地 CI/CD 管道和集成测试
- **学生和教育者**: 理解真实 Kubernetes 集群运维

## 2 功能需求

### 2.1 集群生命周期管理

#### 2.1.1 集群创建 (`ohmykube up`)

**状态**: ✅ 已实现

```bash
ohmykube up [flags]
```

**支持的参数**:
- `--workers <count>`: Worker 节点数量（默认: 2）
- `--master-cpu <cores>`: Master 节点 CPU 核数（默认: 2）
- `--master-memory <GB>`: Master 节点内存（默认: 4）
- `--master-disk <GB>`: Master 节点磁盘空间（默认: 20）
- `--worker-cpu <cores>`: Worker 节点 CPU 核数（默认: 1）
- `--worker-memory <GB>`: Worker 节点内存（默认: 2）
- `--worker-disk <GB>`: Worker 节点磁盘空间（默认: 10）
- `--k8s-version <version>`: Kubernetes 版本（默认: v1.33.0）
- `--cni <type>`: CNI 插件（flannel、cilium、none）
- `--csi <type>`: CSI 插件（local-path-provisioner、rook-ceph、none）
- `--enable-swap`: 启用 swap 支持（K8s 1.28+）
- `--kubeadm-config <path>`: 自定义 kubeadm 配置文件

**默认集群配置**:
- 1 Master 节点（2 CPU、4GB RAM、20GB 磁盘）
- 2 Worker 节点（1 CPU、2GB RAM、10GB 磁盘）
- Flannel CNI
- Local-Path-Provisioner CSI
- MetalLB LoadBalancer
- Ubuntu 24.04 基础镜像

#### 2.1.2 集群删除 (`ohmykube down`)

**状态**: ✅ 已实现

```bash
ohmykube down [flags]
```

**功能**:
- 删除前优雅驱逐 Pod
- VM 清理和资源释放
- 配置文件清理
- Kubeconfig 清理

#### 2.1.3 添加节点 (`ohmykube add`)

**状态**: ✅ 已实现

```bash
ohmykube add [flags]
```

**支持的参数**:
- `--cpu <cores>`: 节点 CPU 核数（默认: 1）
- `--memory <GB>`: 节点内存（默认: 2）
- `--disk <GB>`: 节点磁盘空间（默认: 10）
- `--count <number>`: 添加节点数（默认: 1）

#### 2.1.4 删除节点 (`ohmykube delete`)

**状态**: ✅ 已实现

```bash
ohmykube delete <node-name> [node-name...] [flags]
```

**支持的参数**:
- `--force`: 强制删除，不进行 Pod 驱逐

#### 2.1.5 Registry 管理 (`ohmykube registry`)

**状态**: 🚧 计划中

```bash
ohmykube registry [subcommand] [flags]
```

**计划子命令**: `up`、`down`、`status`、`login`、`push`、`pull`

#### 2.1.6 Kubeconfig 管理（自动下载 + 上下文切换）

**状态**: ✅ 已实现

- 集群创建完成后，自动将 kubeconfig 下载到 `~/.ohmykube/<cluster-name>/kubeconfig`
- `ohmykube switch <cluster-name>` 提供 KUBECONFIG 切换指引

### 2.2 网络插件支持（CNI）

#### 2.2.1 Flannel（默认）

**状态**: ✅ 已实现

- VXLAN 后端实现跨节点通信
- 可配置 Pod CIDR（默认: 10.244.0.0/16）
- 简单配置，性能可靠

#### 2.2.2 Cilium

**状态**: ✅ 已实现

- 基于 eBPF 的数据平面
- 网络策略和安全
- Service Mesh 能力
- 可观测性和监控

#### 2.2.3 自定义 CNI 支持

**状态**: 🚧 计划中

### 2.3 存储插件支持（CSI）

#### 2.3.1 Local-Path-Provisioner（默认）

**状态**: ✅ 已实现

- 动态 PV 供应
- 本地节点存储利用
- 简单配置

#### 2.3.2 Rook-Ceph

**状态**: ✅ 已实现

- 分布式块、对象和文件存储
- 高可用和数据复制
- 不同性能层级的存储类

### 2.4 负载均衡支持

#### 2.4.1 MetalLB

**状态**: ✅ 已实现

- Layer 2 和 BGP 模式
- IP 地址池管理
- 集群外服务暴露

### 2.5 高级功能

#### 2.5.1 包缓存系统

**状态**: ✅ 已实现

- 本地包缓存（zstd 压缩）
- 支持多种包类型（containerd、kubectl、kubeadm、kubelet、helm 等）
- 自动文件格式标准化为 .tar.zst
- SSH 分发包到节点
- 校验和验证包完整性
- 缓存统计和清理操作

#### 2.5.2 镜像缓存系统

**状态**: ✅ 已实现

- 本地镜像缓存避免重复下载
- 支持多种镜像源（Docker、Podman、Helm Charts）
- YAML 索引追踪缓存镜像
- 新节点自动镜像预热
- 镜像完整性验证

#### 2.5.3 多集群管理

**状态**: 🚧 部分实现（基础能力）

- 项目工作区初始化 (`ohmykube init`)：计划中
- 集群上下文切换 (`ohmykube switch`)：✅ 已实现（基础切换）

#### 2.5.4 检查点与恢复

**状态**: ✅ 已实现

支持中断的集群创建恢复。

#### 2.5.5 自定义配置支持

**状态**: ✅ 已实现

- 自定义 kubeadm 配置（InitConfiguration、ClusterConfiguration、KubeletConfiguration、KubeProxyConfiguration）
- 自定义 Lima VM 模板

### 2.6 平台支持

| 平台 | 状态 |
|------|------|
| Mac arm64 | ✅ 主要支持 |
| Linux arm64/amd64 | 🚧 计划中 |
| Windows | 🚧 实验性 |

虚拟化要求: Lima（主要），未来支持其他虚拟化平台。

## 3 非功能需求

### 3.1 性能需求

#### 3.1.1 集群创建性能

| 指标 | 目标 |
|------|------|
| 首次集群创建 | < 10 分钟（3 节点集群） |
| 后续创建（带缓存） | < 5 分钟 |
| 节点添加 | < 1 分钟/节点 |
| 集群删除 | < 1 分钟 |

优化策略: 并行操作、包/镜像缓存、增量更新、资源优化。

#### 3.1.2 资源利用

| 需求级别 | RAM | CPU | 磁盘 |
|----------|-----|-----|------|
| 最低 | 8GB | 4 核 | 50GB |
| 推荐 | 16GB | 8 核 | 100GB |

#### 3.1.3 网络性能

- 节点间延迟: < 1ms
- Pod 间延迟: < 1ms
- 服务发现: < 1ms

#### 3.1.4 运行时性能

- 内存开销: < 100MB（OhMyKube 进程）
- CPU 开销: < 5%（正常运行）
- 磁盘 I/O: 针对 SSD 优化

### 3.2 可靠性需求

- **VM 故障恢复**: 自动检测和报告
- **网络分区处理**: 优雅降级
- **存储故障恢复**: Rook-Ceph 数据保护
- **配置持久化**: 集群状态本地保存
- **卷数据持久化**: Pod 重启后保留
- **缓存持久化**: 系统重启后保留

### 3.3 可扩展性需求

- **最大节点数**: 每集群 10 个 Worker 节点（取决于硬件）
- **节点添加时间**: < 1 分钟/节点
- **并发操作**: 支持并行节点操作
- **多集群**: 受系统资源限制
- **集群隔离**: 完全资源和网络隔离
- **上下文切换**: < 1 秒

## 4 技术架构

### 4.1 核心组件

| 组件 | 位置 | 职责 |
|------|------|------|
| CLI | `cmd/ohmykube/` | 命令解析、用户交互、配置管理、错误处理 |
| Controller Manager | `pkg/controller/` | 集群生命周期、节点编排、组件协调、状态管理 |
| Provider | `pkg/provider/` | VM 创建管理、模板处理、资源分配、网络配置 |
| SSH Manager | `pkg/ssh/` | 远程命令执行、文件传输、连接池、认证处理 |
| Initializer | `pkg/initializer/` | 节点环境配置、包安装、系统优化、服务初始化 |
| Cache Manager | `pkg/cache/` | 包缓存分发、镜像缓存管理、压缩优化、完整性验证 |
| Addon Manager | `pkg/addons/` | CNI/CSI/LB 插件安装配置、自定义 Addon 支持 |

### 4.2 集群创建流程

1. **配置解析**: CLI 解析用户输入，创建集群配置
2. **VM 供应**: Provider 按规格创建 VM
3. **节点初始化**: Initializer 在每个节点上安装所需包
4. **Kubernetes 引导**: kubeadm 初始化集群
5. **Addon 安装**: 部署 CNI、CSI 和 LB 组件
6. **验证**: 集群健康检查和验证
7. **Kubeconfig 分发**: 向用户提供访问凭证

### 4.3 包缓存流程

1. **包请求**: 节点需要特定包
2. **缓存检查**: 系统检查本地缓存
3. **下载**: 如未缓存，从官方源下载
4. **标准化**: 转换为标准 .tar.zst 格式
5. **存储**: 存储到本地缓存（含元数据）
6. **分发**: 通过 SSH 上传到目标节点
7. **安装**: 在目标节点解压安装

### 4.4 配置管理

**配置层次**:
1. 默认配置: 内置的合理默认值
2. 用户配置: 命令行参数和选项
3. 自定义配置: kubeadm 和 Lima 模板覆盖
4. 运行时配置: 运行期间的动态调整

**配置存储**:
- 集群状态: `~/.ohmykube/<cluster-name>/`
- 缓存数据: `~/.ohmykube/cache/`

## 5 实现状态

### 5.1 已完成功能

- 基础集群创建和删除
- 节点添加和删除
- Kubeconfig 管理
- SSH 远程操作
- Lima 集成
- Flannel / Cilium CNI
- Local-Path-Provisioner / Rook-Ceph CSI
- MetalLB 负载均衡
- 完整包缓存系统
- 容器镜像缓存系统
- 自定义 kubeadm 配置
- 检查点与恢复

### 5.2 计划中功能

- Harbor Registry 管理
- 多集群管理
- 集群备份与恢复
- 高级监控集成
- CI/CD 管道集成
- 增强 CLI 体验（交互式向导、进度条、着色输出、Shell 补全）
- Windows/Linux 平台扩展

## 6 安全需求

### 6.1 认证与授权

- SSH 密钥认证（默认）
- 密码认证回退
- 自动 SSH 密钥生成和管理
- 所有操作加密连接
- RBAC 基于角色的访问控制
- 默认网络分段
- Pod 安全标准执行

### 6.2 数据保护

- 静态数据加密
- 传输 TLS 加密
- 安全密钥存储和轮换
- 无遥测数据收集（默认）
- 所有数据本地存储
- 可选审计日志

## 7 测试需求

### 7.1 单元测试

- 覆盖率目标: > 80%
- 测试框架: Go testing + testify
- 全面的外部依赖 Mock
- 每次提交自动测试

### 7.2 集成测试

- 端到端集群生命周期测试
- 组件间通信测试
- 多平台兼容性测试
- 性能回归测试

### 7.3 用户验收测试

- 真实场景验证
- CLI 可用性测试
- 文档准确性测试
- 版本兼容性测试

## 8 开发路线图

### 8.1 Phase 1: 核心稳定性（当前）

- 改进错误处理和恢复
- 集群创建性能优化
- 全面日志记录
- 增强 CLI 用户体验
- 自动化测试框架

### 8.2 Phase 2: 功能扩展

- Harbor Registry 实现
- 多集群管理基础
- 集群备份与恢复
- Windows 平台支持
- 高级监控集成
- 自定义 Addon 框架

### 8.3 Phase 3: 生态集成

- CI/CD 管道集成
- IDE 和开发工具集成
- 云提供商支持
- 插件生态系统开发

## 9 关联文档

- [Addon 系统设计](./addon-system-design.md)
- [镜像缓存设计](./image-cache-design.md)
