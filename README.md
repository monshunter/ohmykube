# OhMyKube

基于 Multipass 和 kubeadm 快速创建虚拟机集群并构建真实的 Kubernetes 环境。

## 特性

- 通过虚拟机而非容器运行 Kubernetes 节点，提供更真实的环境
- 自动安装和配置 Cilium (CNI)、Rook-Ceph (CSI) 和 MetalLB (LoadBalancer)
- 支持在本地快速搭建、删除集群
- 方便的节点管理功能

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
# 使用默认配置创建集群
ohmykube up

# 自定义配置
ohmykube up --nodes 3 --master-cpu 4 --master-memory 8192 --worker-cpu 2 --worker-memory 4096
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

## 支持平台

- Mac arm64 (优先支持)
- Linux arm64/amd64

## 贡献

欢迎提交 Issue 和 Pull Request！

## 许可证

MIT