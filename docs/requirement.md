# Oh My Kube
## Background
Kubernetes local development environments are increasingly valued by developers, and the community provides various tools for setting up local development environments (minikube, kind, etc.). However, these tools are primarily based on Docker containers to simulate K8s clusters, which differ significantly from real production environments (e.g., in terms of node resource management), causing debugging difficulties in elasticity and scheduling optimization.
Personal computers (especially Mac M chip series) have increasingly powerful specifications in recent years, making it possible to virtualize multiple Kubernetes-compatible nodes on a single computer.
## Objective
Quickly set up a real Kubernetes cluster based on virtual machines on developers' computers, with simple operations.
MVP (Minimum Viable Product) will include the following core components:
- **CNI (Container Network Interface)**: Implemented with Cilium.
- **CSI (Container Storage Interface)**: Implemented with Rook (Ceph).
- **LoadBalancer**: Implemented with MetalLB.

## Solution
Quickly create virtual machines and build an MVP Kubernetes cluster based on Lima and kubeadm

### Command Line
ohmykube
### Subcommands
ohmykube up : Create a K8s cluster (including Cilium, Rook, MetalLB)
ohmykube down : Delete a K8s cluster
ohmykube registry : (Optional) Create a local Harbor registry
ohmykube add : Add a node
ohmykube delete : Delete a node
### Supported Platforms
- Mac arm64 (priority)
- Linux arm64/amd64