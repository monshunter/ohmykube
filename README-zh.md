# Oh My Kube

<p align="center">
  <strong>在真实虚拟机上快速搭建完整的 Kubernetes 环境</strong>
</p>

<p align="center">
  中文文档 | <a href="README.md">English</a>
</p>

<p align="center">
  <a href="#核心特性">核心特性</a> •
  <a href="#为什么选择-ohmykube">为什么选择 OhMyKube</a> •
  <a href="#快速开始">快速开始</a> •
  <a href="#使用场景">使用场景</a> •
  <a href="#详细文档">详细文档</a> •
  <a href="#发展路线图">发展路线图</a>
</p>

OhMyKube 是一个基于真实虚拟机构建的 Kubernetes 集群创建工具，使用 Lima 虚拟化技术和 kubeadm，便于开发者快速创建简单的 Kubernetes 环境。

## 核心特性

- 🌟 **真实虚拟机**：使用独立的虚拟机而非容器来运行 Kubernetes 节点，更接近生产环境
- 🔄 **一键部署**：简单的命令行界面，快速创建、删除和扩展集群
- 🧩 **网络插件选择**：支持 Flannel（默认）和 Cilium，满足不同场景需求
- 💾 **存储集成**：自动安装 Local-Path-Provisioner 或 Rook-Ceph 存储系统
- 🔌 **负载均衡**：内置 MetalLB 提供真正的 LoadBalancer 服务体验
- 🛠️ **高度灵活**：支持自定义 kubeadm 配置和可调节的资源分配
- 🚀 **快速节点管理**：轻松添加或删除工作节点

## 为什么选择 OhMyKube

在众多 Kubernetes 工具中，OhMyKube 提供独特价值：

| 特性 | Kind/K3d/Minikube | OhMyKube | Kubespray/Sealos |
|------|----------|----------|------------------|
| 环境真实性 | 🟡 容器模拟 | 🟢 真实虚拟机 | 🟢 生产级别 |
| 资源隔离 | 🟡 容器级别 | 🟢 虚拟机级别 | 🟢 物理机/虚拟机级别 |
| 易用性 | 🟢 非常简单 | 🟢 简单 | 🟡 较复杂 |
| 启动速度 | 🟢 非常快 | 🟡 中等 | 🔴 较慢 |
| 适合本地开发 | 🟢 是 | 🟢 是 | 🟡 是但较重 |
| 接近生产环境 | 🔴 差异较大 | 🟢 相似 | 🟢 相同 |
| 网络模型 | 🟡 简化 | 🟢 真实 | 🟢 真实 |
| 存储支持 | 🟡 有限 | 🟢 全面 | 🟢 全面 |
| 硬件要求 | 🟢 低 | 🟡 中等 | 🔴 高 |

## 快速开始

### 前置要求

1. 安装 [Lima](https://github.com/lima-vm/lima)
2. 安装 Go 1.23.0 或更高版本

### 安装

```bash
# 克隆仓库
git clone https://github.com/monshunter/ohmykube.git
cd ohmykube

# 编译并安装
make install
```

### 基本使用

```bash
# 创建集群（默认：1 个主节点 + 2 个工作节点）
ohmykube up

# 查看集群状态
export KUBECONFIG=~/.kube/ohmykube-config
kubectl get nodes

# 删除集群
ohmykube down
```

## 使用场景

- **开发和测试**：在类似生产环境中测试应用程序
- **学习 Kubernetes**：了解真实 Kubernetes 集群的工作原理
- **本地 CI/CD**：在本地构建完整的集成测试环境
- **网络和存储研究**：测试不同的 CNI 和 CSI 组合
- **集群管理实践**：学习节点管理、维护和故障排除

## 详细文档

### 创建自定义集群

```bash
# 自定义节点数量和资源
ohmykube up --workers 3 --master-cpu 4 --master-memory 8 --master-disk 20 \
            --worker-cpu 2 --worker-memory 4096 --worker-disk 10

# 选择网络插件
ohmykube up --cni cilium

# 选择存储插件
ohmykube up --csi rook-ceph

# 使用自定义 kubeadm 配置
ohmykube up --kubeadm-config /path/to/custom-kubeadm-config.yaml
```

### 集群管理

```bash
# 添加节点
ohmykube add --cpu 2 --memory 4 --disk 20

# 删除节点
ohmykube delete ohmykube-worker-2

# 强制删除（不先驱逐 Pod）
ohmykube delete ohmykube-worker-2 --force

# 下载 kubeconfig
ohmykube download-kubeconfig
```

### 自定义 Kubeadm 配置

您可以提供自定义的 kubeadm 配置文件来覆盖默认设置。支持以下部分：

- InitConfiguration
- ClusterConfiguration
- KubeletConfiguration
- KubeProxyConfiguration

示例：

```yaml
apiVersion: kubeadm.k8s.io/v1beta4
kind: InitConfiguration
nodeRegistration:
  criSocket: unix:///var/run/containerd/containerd.sock
---
apiVersion: kubeadm.k8s.io/v1beta4
kind: ClusterConfiguration
kubernetesVersion: v1.33.0
networking:
  podSubnet: 192.168.0.0/16
  serviceSubnet: 10.96.0.0/12
```

## 发展路线图

我们正在规划以下功能增强：

### 即将推出 🚀

- **镜像管理增强**
  - 本地 Harbor 注册表支持（`ohmykube registry`）
  - 镜像同步工具（`ohmykube load`，类似 kind load）
  - 镜像缓存机制，加速集群创建

- **多集群管理**
  - 项目初始化（`ohmykube init`）
  - 集群切换（`ohmykube switch`）
  - 构建过程检查点，支持中断恢复

### 中期计划 🔄

- **提供商抽象**
  - 支持云 API 虚拟机创建（阿里云、腾讯云等）
  - 支持更多本地虚拟化平台

- **更多平台支持**
  - 改进对不同 CPU 架构的支持
  - 优化 Windows 环境体验

### 长期愿景 🌈

- **插件生态系统**
  - 插件扩展机制
  - 常用插件集成（监控、日志、CI/CD 等）

- **开发者工具**
  - IDE 集成
  - 调试工具链
  - 开发工作流优化

## 支持的平台

- Mac arm64（优先支持）
- Linux arm64/amd64
- 其他平台（实验性支持）

## 贡献

我们欢迎各种形式的贡献，无论是代码、文档还是想法：

- 提交 Issues 报告错误或请求功能
- 提交 Pull Requests 贡献代码或文档
- 参与讨论并分享您的经验
- 帮助测试新功能和版本

## 许可证

MIT
