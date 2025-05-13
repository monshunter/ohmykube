# Oh my Kube
## 背景
kubernetes 本地开发环境越来越受到开发者的重视，社区也提供了多种多样的本地开发环境的搭建工具(minikube、kube 等等)。然而，这些工具基本上都是基于 docker 容器进行 k8s 集群的模拟，和真实的生产环境存在不小差异（比如： node 的真实资源管理），这在弹性和调度优化方面造成一定的调试困难。
个人电脑（尤其是 Mac M 芯片系列）这几年的规格配置越来越高，使得在一台电脑上虚拟多个满足kubernetes 的 node 节点成为可能。
## 目标
在开发者电脑上快速搭建基于虚拟机实现的真实 kubernetes 集群，要求操作简单。
MVP (最小可行产品) 将包括以下核心组件：
- **CNI (Container Network Interface)**: 基于 Cilium 实现。
- **CSI (Container Storage Interface)**: 基于 Rook (Ceph) 实现。
- **LoadBalancer**: 基于 MetalLB 实现。

## 方案
基于Multipass 和 kubeadm 快速创建虚拟机并构建 MVP kubernetes 集群

### 命令行
ohmykube
### 子命令
ohmykube up : 创建一个 k8s 集群 (包含Cilium, Rook, MetalLB)
ohmykube down : 删除一个 k8s 集群 
ohmykube registry : (可选) 创建一个本地harbor
ohmykube add : 添加一个节点
ohmykube delete : 删除一个节点 
### 支持平台
- Mac arm64 (优先)
- Linux arm64/amd64