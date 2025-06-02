# Oh My Kube: Make Kubernetes Simple and Fast!

<p align="center">
  <strong>Quickly start a complete local kubernetes cluster on multi-node or heterogeneous virtual machines</strong>
</p>

<p align="center">
  <a href="README-zh.md">中文文档</a> | English
</p>

<p align="center">
  <a href="#quick-start">Quick Start</a> •
  <a href="#use-cases">Use Cases</a> •
  <a href="#kubernetes-version-support">Kubernetes Version Support</a> •
  <a href="#roadmap">Roadmap</a>
</p>

## Quick Start

### Prerequisites

1. Install [Lima](https://github.com/lima-vm/lima)
2. Install Go 1.23.0 or higher

### Install

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

### Creating Custom Clusters

```bash
# Customize node count and resources
ohmykube up --workers 3 --master-cpu 4 --master-memory 8 --master-disk 20 \
            --worker-cpu 2 --worker-memory 4096 --worker-disk 10

# Select network plugin
ohmykube up --cni cilium

# Select storage plugin
ohmykube up --csi rook-ceph

# Disable LoadBalancer
ohmykube up --lb ""

# Use custom kubeadm configuration
ohmykube up --kubeadm-config /path/to/custom-kubeadm-config.yaml
```

### Cluster Management

```bash
# List all nodes
ohmykube list

# Add a node
ohmykube add --cpu 2 --memory 4 --disk 20

# Delete a node
ohmykube delete ohmykube-worker-2

# Force delete (without evicting Pods first)
ohmykube delete ohmykube-worker-2 --force

# Start a node
ohmykube start ohmykube-worker-2

# Stop a node
ohmykube stop ohmykube-worker-2

# Enter node Shell
ohmykube shell ohmykube-worker-2

```

### Custom Kubeadm Configuration (not supported yet, but coming soon)

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

## Use Cases

- **Development and Testing**: Test applications in an environment similar to production
- **Learning Kubernetes**: Understand how real Kubernetes clusters work
- **Local CI/CD**: Build complete integration testing environments locally
- **Network and Storage Research**: Test different CNI and CSI combinations
- **Cluster Management Practice**: Learn node management, maintenance, and troubleshooting

## Roadmap

We are planning the following feature enhancements:

### Coming Soon 🚀
- **Multi-Cluster Management**
  - Project initialization (`ohmykube init`)
  - Cluster switching (`ohmykube switch`)

### Mid-term Plans 🔄

- **Provider Abstraction**
  - Support for cloud API virtual machine creation (Alibaba Cloud, Tencent Cloud, AWS, GKE, etc.)
  - Support for more local virtualization platforms

### Long-term Vision 🌈

- **Plugin Ecosystem**
  - Plugin extension mechanism
  - Common plugin integration (monitoring, logging, CI/CD, etc.)

- **Developer Tools**
  - IDE integration
  - Debugging toolchain
  - Development workflow optimization

## Supported Platforms

- Mac arm64 (supported)
- Linux arm64/amd64 (coming soon)
- Other platforms (coming soon)

## Kubernetes Version Support

✅ **Supports Kubernetes v1.24.x and above**

## Contributing

We welcome contributions of all kinds, whether code, documentation, or ideas:

- Submit Issues to report bugs or request features
- Submit Pull Requests to contribute code or documentation
- Participate in discussions and share your experiences
- Help test new features and releases

## License

MIT