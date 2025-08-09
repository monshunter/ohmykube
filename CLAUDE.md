# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Build and Development Commands

### Core Commands
- `make build` - Build the binary to bin/ohmykube
- `make install` - Build and install to $GOPATH/bin/ohmykube
- `make test` - Run all tests
- `make lint` - Run golangci-lint
- `make deps` - Update and verify dependencies
- `make clean` - Clean build artifacts

### Testing
- `go test ./...` - Run all tests
- `go test ./pkg/cache/...` - Run tests for specific package
- Check specific test files like `pkg/cache/*_test.go`, `pkg/utils/*_test.go`

### Running the Application
- `ohmykube up` - Create default cluster (1 master + 2 workers)
  - `ohmykube up -f config.yaml` - Create cluster from configuration file
- `ohmykube down` - Delete cluster
  - `ohmykube down -f config.yaml` - Delete cluster specified in configuration file
- `ohmykube config` - Generate cluster configuration template (outputs to `$clusterName.yaml`)
  - `ohmykube config -o custom.yaml` - Specify custom output file
- `ohmykube list` - List all nodes
- `ohmykube status` - Check status of clusters
- `ohmykube switch` - Set default context to specified cluster
- `ohmykube add` - Add new nodes to existing cluster
- `ohmykube delete NODE_NAME` - Delete specific node (supports --force)
- `ohmykube start/stop NODE_NAME` - Start/stop specific nodes
- `ohmykube shell NODE_NAME` - Enter interactive shell on node
- `ohmykube load IMAGE_NAME` - Load local container image to cluster nodes
  - Supports multiple container runtimes: `--runtime docker|podman|nerdctl`
  - Architecture validation: `--skip-arch-check` to bypass compatibility checks
  - Target specific nodes: `--nodes node1,node2`

### Cluster Customization Options
- `--workers N` - Number of worker nodes (default: 2)
- `--k8s-version` - Kubernetes version to install (default: v1.33.0)
- `--cni cilium|flannel` - CNI plugin selection (default: flannel)
- `--csi local-path|rook-ceph` - CSI plugin selection (default: local-path)
- `--lb metallb` - Enable LoadBalancer with MetalLB (auto-sets proxy-mode to ipvs)
- `--master-cpu/memory/disk` - Master node resource allocation
- `--worker-cpu/memory/disk` - Worker node resource allocation
- `--template` - Lima template (default: ubuntu-24.04)
- `--update-system` - Update system packages before installation (global flag)
- `--proxy-mode iptables|ipvs` - Kubernetes proxy mode (default: iptables, auto-set to ipvs with MetalLB)

### Advanced Node Customization
- `--master-labels key=value` - Labels for master nodes (repeatable)
- `--worker-labels key=value` - Labels for worker nodes (repeatable)
- `--master-annotations key=value` - Annotations for master nodes (repeatable)
- `--worker-annotations key=value` - Annotations for worker nodes (repeatable)
- `--master-taints key=value:effect` - Taints for master nodes (repeatable)
- `--worker-taints key=value:effect` - Taints for worker nodes (repeatable)

### Custom Initialization Hooks
- `--master-hook-pre-system-init` - Scripts to run before system update on master
- `--master-hook-post-system-init` - Scripts to run after system update on master
- `--master-hook-pre-k8s-init` - Scripts to run before K8s install on master
- `--master-hook-post-k8s-init` - Scripts to run after K8s install on master
- `--master-upload-file local:remote[:mode[:owner]]` - Upload files to master nodes
- `--master-upload-dir local:remote[:mode[:owner]]` - Upload directories to master nodes
- Worker equivalents: `--worker-hook-*` and `--worker-upload-*`

### Environment Setup
- Kubeconfig: `export KUBECONFIG=~/.kube/ohmykube-config`
- Cluster state stored in: `~/.ohmykube/<cluster-name>/cluster.yaml`
- Current cluster context: `~/.ohmykube/current-cluster`
- SSH keys: `~/.ohmykube/<cluster-name>/ssh/`

## Architecture Overview

OhMyKube is a Kubernetes cluster management tool built on Lima VMs and kubeadm. Key architectural patterns:

### Project Structure
- **cmd/ohmykube/**: CLI application entry point with Cobra commands
- **pkg/**: Core packages organized by functionality
- **pkg/provider/**: VM provider abstraction (currently Lima only)
- **pkg/config/**: Cluster configuration and state management
- **pkg/cache/**: Container image and package caching system
- **pkg/addons/**: Plugin system for CNI, CSI, and LoadBalancer components

### Key Design Patterns

#### Provider Pattern
- `pkg/provider/factory.go` - Abstract factory for VM providers
- `pkg/provider/provider.go` - Provider interface defining VM lifecycle operations
- Currently supports Lima only (`pkg/provider/lima/`), designed for future cloud provider support
- Provider abstraction allows swapping VM backends (planned: AliCloud, AWS, GKE, TKE)

#### Cluster State Management
- `pkg/config/cluster.go` - Central cluster configuration with thread-safe operations
- `pkg/config/cluster_manager.go` - High-level cluster management operations
- Uses sync.RWMutex for concurrent access to cluster state
- Node groups organize nodes with same specifications via `NodeGroupSpec`
- Condition-based status tracking for nodes and cluster health
- YAML serialization for persistent state storage

#### Image Caching System  
- `pkg/cache/` - Sophisticated caching for container images and system packages
- `pkg/interfaces/cacher.go` - Interface definitions for cache managers
- Architecture-aware image discovery and downloading with compression
- Separate `PackageCacheManager` and `ImageCacheManager` interfaces
- `ImageSource` abstraction supports helm, manifest, kubeadm, and custom sources
- Cluster-level image tracking for efficient distribution

#### Plugin Architecture
- `pkg/addons/` - Extensible addon system with type-safe registration
- CNI plugins: Cilium, Flannel with network policy support  
- CSI plugins: Local-path-provisioner, Rook-Ceph for storage
- LoadBalancer: MetalLB integration with IPVS proxy mode
- Default configurations in `pkg/config/default/` for each component type

### Important Implementation Details

#### Concurrency and Thread Safety
- Cluster operations use mutex locks extensively
- `pkg/utils/keyed_semaphore.go` - Keyed semaphore for resource-specific locking
- Node operations are coordinated through cluster state

#### Authentication and SSH
- `pkg/ssh/` - SSH client management for VM communication
- Key-based authentication with configurable users and ports
- Connection pooling and reuse

#### Configuration Defaults
- Default Kubernetes version and networking (see cmd/ohmykube/app/)
- Pod subnet: 10.244.0.0/16, Service subnet: 10.96.0.0/12
- Default VM template: ubuntu-24.04

### Dependencies and Constraints

#### Go Version
- Requires Go 1.23.0+ (see go.mod)
- Uses recent Go features like slices package

#### External Dependencies
- Lima for VM management
- kubeadm for Kubernetes cluster initialization
- Container runtime: containerd
- SSH for VM communication

#### Platform Support
- Primary support: macOS arm64
- Limited support for other platforms

### Development Guidelines

#### Module Organization
- Follow existing package structure
- Keep provider-specific code in pkg/provider/
- Use interfaces for extensibility (see pkg/interfaces/)
- Maintain thread safety in shared state

#### Testing Strategy
- Unit tests for utility functions and algorithms
- Integration tests in tests/ directory
- Test files follow *_test.go convention

#### Configuration Management
- Cluster specs use YAML serialization
- State persisted to ~/.ohmykube/<cluster-name>/cluster.yaml
- Default values handled in config package
- Configuration consistency: stored cluster settings (UpdateSystem, networking) take precedence over CLI flags in `add` operations
- Default configurations for system components in `pkg/config/default/`:
  - Kubeadm cluster/init configurations with network settings
  - Containerd runtime configurations (versions 1.7.24, 2.1.0)
  - IPVS proxy mode settings
  - MetalLB LoadBalancer configurations
- Template generation: `pkg/config/template.go` provides comprehensive YAML templates with comments

### Critical Implementation Patterns

#### Configuration Consistency
- **Problem**: CLI flags vs stored cluster configuration conflicts
- **Solution**: Stored cluster configuration takes precedence in operations like `add`
- **Key locations**: `up.go:193`, `add.go:71-92` use `cls.GetUpdateSystem()` instead of flag values
- **Context management**: `up` command calls `config.SetCurrentCluster()` after successful creation

#### Thread-Safe Cluster Operations
- All cluster state modifications use `sync.RWMutex` for concurrent access
- Node group operations are atomic and coordinated through cluster state
- See `pkg/config/cluster.go` for mutex patterns

#### Provider Abstraction Patterns
- Interface-based design allows swapping VM backends
- Current implementation: Lima only, but designed for cloud providers
- Factory pattern in `pkg/provider/factory.go` for extensibility

#### Error Handling and Resilience
- Graceful shutdown handlers in all long-running operations
- Resume capability: `up` command can continue from partial cluster state
- Comprehensive validation before destructive operations

### Command Structure and Flow

#### CLI Architecture
- `cmd/ohmykube/main.go` - Entry point with version info injection via LDFLAGS
- `cmd/ohmykube/app/root.go` - Cobra root command with global flags and subcommand registration
- `cmd/ohmykube/app/*.go` - Individual subcommand implementations
- Global state management for current cluster context via `config.GetCurrentCluster()`
- Configuration file support via `-f --file` flags in `up`, `down` commands
- Auto-context switching: `up` command sets current cluster context after successful creation

#### Key Command Implementations
- `up.go` - Cluster creation with graceful shutdown handling and validation
  - Supports configuration file loading via `-f` flag
  - Auto-sets current cluster context after successful creation
  - Handles resume from partial cluster state
- `down.go` - Cluster deletion with proper cleanup
  - Supports configuration file loading via `-f` flag
  - Auto-switches to next available cluster after deletion
- `add.go` - Dynamic node addition to existing clusters
  - Uses stored cluster configuration for consistency (UpdateSystem, networking, etc.)
- `load.go` - Container image loading with runtime detection
- `status.go` / `switch.go` - Multi-cluster management commands
- `config.go` - Configuration template generation with dynamic file naming