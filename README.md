# Oh My Kube

<p align="center">
  <strong>Quickly Set Up Complete Kubernetes Environments on Real Virtual Machines</strong>
</p>

<p align="center">
  <a href="README-zh.md">中文文档</a> | English
</p>

<p align="center">
  <a href="#core-features">Core Features</a> •
  <a href="#why-choose-ohmykube">Why Choose OhMyKube</a> •
  <a href="#quick-start">Quick Start</a> •
  <a href="#use-cases">Use Cases</a> •
  <a href="#detailed-documentation">Detailed Documentation</a> •
  <a href="#roadmap">Roadmap</a>
</p>

OhMyKube is a Kubernetes cluster creation tool built on real virtual machines. It uses Lima virtualization technology and kubeadm, making it easy for developers to quickly create a simple Kubernetes environment.

## Core Features

- 🌟 **Real Virtual Machines**: Uses independent VMs instead of containers to run Kubernetes nodes, closer to production environments
- 🔄 **One-Click Deployment**: Simple command-line interface for quick creation, deletion, and scaling of clusters
- 🧩 **Network Plugin Options**: Supports Flannel (default) and Cilium to meet different scenario requirements
- 💾 **Storage Integration**: Automatically installs Local-Path-Provisioner or Rook-Ceph storage systems
- 🔌 **Load Balancing**: Built-in MetalLB provides a genuine LoadBalancer service experience
- 🛠️ **Highly Flexible**: Supports custom kubeadm configurations and adjustable resource allocation
- 🚀 **Quick Node Management**: Easily add or remove worker nodes

## Why Choose OhMyKube

Among many Kubernetes tools, OhMyKube offers unique value:

| Feature | Kind/K3d/Minikube | OhMyKube | Kubespray/Sealos |
|---------|----------|----------|------------------|
| Environment Realism | 🟡 Container Simulation | 🟢 Real VM | 🟢 Production Grade |
| Resource Isolation | 🟡 Container Level | 🟢 VM Level | 🟢 Physical/VM Level |
| Ease of Use | 🟢 Very Simple | 🟢 Simple | 🟡 More Complex |
| Startup Speed | 🟢 Very Fast | 🟡 Medium | 🔴 Slower |
| Suitable for Local Development | 🟢 Yes | 🟢 Yes | 🟡 Yes but Heavy |
| Proximity to Production | 🔴 Major Differences | 🟢 Similar | 🟢 Identical |
| Network Model | 🟡 Simplified | 🟢 Realistic | 🟢 Realistic |
| Storage Support | 🟡 Limited | 🟢 Comprehensive | 🟢 Comprehensive |
| Hardware Requirements | 🟢 Low | 🟡 Medium | 🔴 High |

## Quick Start

### Prerequisites

1. Install [Lima](https://github.com/lima-vm/lima)
2. Install Go 1.23.0 or higher

### Installation

```bash
# Clone the repository
git clone https://github.com/monshunter/ohmykube.git
cd ohmykube

# Compile and install
make install
```

### Basic Usage

```bash
# Create a cluster (default: 1 master + 2 workers)
ohmykube up

# View cluster status
export KUBECONFIG=~/.kube/ohmykube-config
kubectl get nodes

# Delete the cluster
ohmykube down
```

## Use Cases

- **Development and Testing**: Test applications in an environment similar to production
- **Learning Kubernetes**: Understand how real Kubernetes clusters work
- **Local CI/CD**: Build complete integration testing environments locally
- **Network and Storage Research**: Test different CNI and CSI combinations
- **Cluster Management Practice**: Learn node management, maintenance, and troubleshooting

## Detailed Documentation

### Creating Custom Clusters

```bash
# Customize node count and resources
ohmykube up --workers 3 --master-cpu 4 --master-memory 8 --master-disk 20 \
            --worker-cpu 2 --worker-memory 4096 --worker-disk 10

# Select network plugin
ohmykube up --cni cilium

# Select storage plugin
ohmykube up --csi rook-ceph

# Use custom kubeadm configuration
ohmykube up --kubeadm-config /path/to/custom-kubeadm-config.yaml
```

### Cluster Management

```bash
# Add a node
ohmykube add --cpu 2 --memory 4 --disk 20

# Delete a node
ohmykube delete ohmykube-worker-2

# Force delete (without evicting Pods first)
ohmykube delete ohmykube-worker-2 --force

# Download kubeconfig
ohmykube download-kubeconfig
```

### Custom Kubeadm Configuration

You can provide a custom kubeadm configuration file to override the default settings. The following sections are supported:

- InitConfiguration
- ClusterConfiguration
- KubeletConfiguration
- KubeProxyConfiguration

Example:

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

## Roadmap

We are planning the following feature enhancements:

### Coming Soon 🚀

- **Image Management Enhancements**
  - Local Harbor registry support (`ohmykube registry`)
  - Image synchronization tools (`ohmykube load`, similar to kind load)
  - Image caching mechanism to accelerate cluster creation

- **Multi-Cluster Management**
  - Project initialization (`ohmykube init`)
  - Cluster switching (`ohmykube switch`)
  - Build process checkpoints, supporting interrupted recovery

### Mid-term Plans 🔄

- **Provider Abstraction**
  - Support for cloud API virtual machine creation (Alibaba Cloud, Tencent Cloud, etc.)
  - Support for more local virtualization platforms

- **More Platform Support**
  - Improved support for different CPU architectures
  - Optimized Windows environment experience

### Long-term Vision 🌈

- **Plugin Ecosystem**
  - Plugin extension mechanism
  - Common plugin integration (monitoring, logging, CI/CD, etc.)

- **Developer Tools**
  - IDE integration
  - Debugging toolchain
  - Development workflow optimization

## Supported Platforms

- Mac arm64 (priority support)
- Linux arm64/amd64
- Other platforms (experimental support)

## Contributing

We welcome contributions of all kinds, whether code, documentation, or ideas:

- Submit Issues to report bugs or request features
- Submit Pull Requests to contribute code or documentation
- Participate in discussions and share your experiences
- Help test new features and releases

## License

MIT