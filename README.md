# OhMyKube

基于 Multipass 和 kubeadm 快速创建虚拟机集群并构建真实的 Kubernetes 环境。

## 特性

- 通过虚拟机而非容器运行 Kubernetes 节点，提供更真实的环境
- 支持多种CNI选项：Flannel（默认）和 Cilium
- 自动安装和配置 Rook-Ceph (CSI) 和 MetalLB (LoadBalancer)
- 自动安装 Helm 到所有节点
- 支持在本地快速搭建、删除集群
- 方便的节点管理功能
- 支持自定义 kubeadm 配置

## 前提条件

1. 安装 [Multipass](https://multipass.run/)
2. 安装 Go 1.19 或更高版本

## 安装

```bash
# 克隆仓库
git clone https://github.com/monshunter/ohmykube.git
cd ohmykube

# 编译安装
make install
```

## 使用方法

### 创建集群

```bash
# 使用默认配置创建集群（使用Flannel作为CNI）
ohmykube up

# 使用Cilium作为CNI
ohmykube up --cni cilium

# 不安装CNI
ohmykube up --cni none

# 自定义配置
ohmykube up --nodes 3 --master-cpu 4 --master-memory 8192 --worker-cpu 2 --worker-memory 4096

# 使用自定义 kubeadm 配置
ohmykube up --kubeadm-config /path/to/custom-kubeadm-config.yaml
```

### 删除集群

```bash
ohmykube down
```

### 添加节点

```bash
ohmykube add --cpu 2 --memory 4096 --disk 20
```

### 删除节点

```bash
ohmykube delete ohmykube-worker-2

# 强制删除
ohmykube delete ohmykube-worker-2 --force
```

### 创建本地仓库（未实现）

```bash
ohmykube registry --cpu 2 --memory 4096 --disk 40
```

## 自定义 kubeadm 配置

您可以提供自定义的 kubeadm 配置文件来覆盖默认配置。配置文件应符合 kubeadm v1beta4 格式，可以包含以下部分：

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
kubernetesVersion: v1.28.1
networking:
  podSubnet: 192.168.0.0/16
  serviceSubnet: 10.96.0.0/12
```

## 支持的CNI选项

OhMyKube支持以下CNI选项：

- `flannel`: 默认选项，轻量级的CNI插件，适合初学者和测试环境
  - 使用Helm安装，自动与集群podCIDR同步
- `cilium`: 高级CNI插件，提供更多网络策略和安全功能
- `none`: 不安装CNI，允许用户手动安装自定义CNI

## 自动安装的组件

OhMyKube在创建集群时会自动安装以下组件：

- **Helm**: 在所有节点上安装Helm 3，方便后续安装和管理应用
- **CNI**: 根据指定选项安装网络插件（默认为Flannel）
- **CSI**: Rook-Ceph存储系统
- **LoadBalancer**: MetalLB负载均衡器

## 支持平台

- Mac arm64 (优先支持)
- Linux arm64/amd64

## 贡献

欢迎提交 Issue 和 Pull Request！

## 许可证

MIT