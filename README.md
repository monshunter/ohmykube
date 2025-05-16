# OhMyKube

<p align="center">
  <strong>在真实虚拟机上快速搭建完整的Kubernetes环境</strong>
</p>

<p align="center">
  <a href="#核心特性">核心特性</a> •
  <a href="#为什么选择OhMyKube">为什么选择OhMyKube</a> •
  <a href="#快速开始">快速开始</a> •
  <a href="#使用场景">使用场景</a> •
  <a href="#详细文档">详细文档</a> •
  <a href="#路线图">路线图</a>
</p>

OhMyKube 是一个基于真实虚拟机构建的Kubernetes集群创建工具，填补了容器化工具（如kind、k3d）与生产级别工具（如kubespray、sealos）之间的空白。它通过Multipass虚拟化技术和kubeadm，提供比容器更真实但比手动部署更简单的Kubernetes环境。

## 核心特性

- 🌟 **真实虚拟机**：使用独立VM而非容器来运行Kubernetes节点，更接近生产环境
- 🔄 **一键部署**：简单的命令行接口，快速创建、删除、扩展集群
- 🧩 **网络插件选择**：支持Flannel（默认）和Cilium，满足不同场景需求
- 💾 **存储集成**：自动安装Local-Path-Provisioner或Rook-Ceph存储系统
- 🔌 **负载均衡**：内置MetalLB，提供真实的LoadBalancer服务体验
- 🛠️ **高度灵活**：支持自定义kubeadm配置，可调节资源分配
- 🚀 **快速节点管理**：轻松添加或删除工作节点

## 为什么选择OhMyKube

在众多Kubernetes工具中，OhMyKube有其独特价值：

| 特性 | Kind/K3d | OhMyKube | Kubespray/Sealos |
|------|----------|----------|------------------|
| 环境真实度 | 🟡 容器模拟 | 🟢 真实VM | 🟢 生产级 |
| 资源隔离 | 🟡 容器级 | 🟢 VM级 | 🟢 物理/VM级 |
| 易用性 | 🟢 非常简单 | 🟢 简单 | 🟡 较复杂 |
| 启动速度 | 🟢 极快 | 🟡 中等 | 🔴 较慢 |
| 适合本地开发 | 🟢 是 | 🟢 是 | 🟡 可以但重 |
| 接近生产环境 | 🔴 差异大 | 🟢 相似 | 🟢 完全一致 |
| 网络模型 | 🟡 简化 | 🟢 真实 | 🟢 真实 |
| 存储支持 | 🟡 有限 | 🟢 全面 | 🟢 全面 |
| 硬件要求 | 🟢 低 | 🟡 中等 | 🔴 高 |

## 快速开始

### 前提条件

1. 安装 [Multipass](https://multipass.run/)
2. 安装 Go 1.23.0 或更高版本

### 安装

```bash
# 克隆仓库
git clone https://github.com/monshunter/ohmykube.git
cd ohmykube

# 编译安装
make install
```

### 基本用法

```bash
# 创建集群（默认1个master + 2个worker）
ohmykube up

# 查看集群状态
export KUBECONFIG=~/.kube/config-ohmykube
kubectl get nodes

# 删除集群
ohmykube down
```

## 使用场景

- **开发测试**：在与生产环境相似的设置中测试应用程序
- **学习Kubernetes**：了解真实Kubernetes集群的工作原理
- **本地CI/CD**：在本地构建完整的集成测试环境
- **网络和存储研究**：测试不同的CNI和CSI组合
- **集群管理实践**：学习节点管理、维护和故障排除

## 详细文档

### 创建定制集群

```bash
# 自定义节点数量和资源
ohmykube up --workers 3 --master-cpu 4 --master-memory 8192 --master-disk 20 \
            --worker-cpu 2 --worker-memory 4096 --worker-disk 10

# 选择网络插件
ohmykube up --cni cilium

# 选择存储插件
ohmykube up --csi rook-ceph

# 使用自定义kubeadm配置
ohmykube up --kubeadm-config /path/to/custom-kubeadm-config.yaml
```

### 集群管理

```bash
# 添加节点
ohmykube add --cpu 2 --memory 4096 --disk 20

# 删除节点
ohmykube delete ohmykube-worker-2

# 强制删除（不先驱逐Pod）
ohmykube delete ohmykube-worker-2 --force

# 下载kubeconfig
ohmykube download-kubeconfig
```

### 自定义Kubeadm配置

您可以提供自定义的kubeadm配置文件来覆盖默认配置，支持以下部分：

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

## 路线图

我们正在规划以下功能增强：

### 即将推出 🚀

- **镜像管理增强**
  - 本地Harbor仓库支持 (`ohmykube registry`)
  - 镜像同步工具 (`ohmykube load`，类似kind load)
  - 镜像缓存机制，加速集群创建

- **多集群管理**
  - 项目初始化 (`ohmykube init`)
  - 集群切换 (`ohmykube switch`)
  - 构建过程checkpoint，支持中断恢复

### 中期规划 🔄

- **提供商抽象**
  - 支持云API创建虚拟机（阿里云、腾讯云等）
  - 支持更多本地虚拟化平台

- **更多平台支持**
  - 完善不同CPU架构的支持
  - 优化Windows环境体验

### 长期愿景 🌈

- **插件生态系统**
  - 插件扩展机制
  - 常用插件集成（监控、日志、CI/CD等）

- **开发者工具**
  - IDE集成
  - 调试工具链
  - 开发流程优化

## 支持平台

- Mac arm64 (优先支持)
- Linux arm64/amd64
- 其他平台（实验性支持）

## 贡献

我们欢迎各种形式的贡献，无论是代码、文档还是想法：

- 提交 Issue 报告bug或提出功能需求
- 提交 Pull Request 贡献代码或文档
- 参与讨论，分享您的使用经验
- 帮助测试新功能和发布版本

## 许可证

MIT