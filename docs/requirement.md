# OhMyKube - Comprehensive Requirements Document

## Table of Contents
1. [Project Overview](#project-overview)
2. [Functional Requirements](#functional-requirements)
3. [Non-Functional Requirements](#non-functional-requirements)
4. [Technical Architecture](#technical-architecture)
5. [Implementation Status](#implementation-status)
6. [Development Roadmap](#development-roadmap)
7. [User Experience Requirements](#user-experience-requirements)
8. [Security Requirements](#security-requirements)
9. [Performance Requirements](#performance-requirements)
10. [Testing Requirements](#testing-requirements)

## Project Overview

### Background
Kubernetes local development environments are increasingly valued by developers, and the community provides various tools for setting up local development environments (minikube, kind, etc.). However, these tools are primarily based on Docker containers to simulate K8s clusters, which differ significantly from real production environments (e.g., in terms of node resource management), causing debugging difficulties in elasticity and scheduling optimization.

Personal computers (especially Mac M chip series) have increasingly powerful specifications in recent years, making it possible to virtualize multiple Kubernetes-compatible nodes on a single computer.

### Mission Statement
OhMyKube bridges the gap between containerized development tools (like kind, k3d) and production-grade deployment tools (like kubespray, sealos) by providing a Kubernetes environment that's more realistic than containers but simpler to deploy than manual setups.

### Core Value Proposition
- **Real Virtual Machines**: Uses independent VMs instead of containers to run Kubernetes nodes, closer to production environments
- **One-Click Deployment**: Simple command-line interface for quick creation, deletion, and scaling of clusters
- **Production-Like Environment**: Realistic network models, storage systems, and resource isolation
- **Developer-Friendly**: Fast setup, easy debugging, and comprehensive tooling integration

### Target Users
- **Kubernetes Developers**: Testing applications in production-like environments
- **Platform Engineers**: Learning and experimenting with Kubernetes configurations
- **DevOps Engineers**: Building local CI/CD pipelines and integration tests
- **Students and Educators**: Understanding real Kubernetes cluster operations

## Functional Requirements

### Core Commands and Features

#### 1. Cluster Lifecycle Management

##### 1.1 Cluster Creation (`ohmykube up`)
**Status**: ✅ Implemented
**Description**: Create a complete Kubernetes cluster with configurable components

**Command Syntax**:
```bash
ohmykube up [flags]
```

**Supported Flags**:
- `--workers <count>`: Number of worker nodes (default: 2)
- `--master-cpu <cores>`: Master node CPU cores (default: 2)
- `--master-memory <GB>`: Master node memory (default: 4)
- `--master-disk <GB>`: Master node disk space (default: 20)
- `--worker-cpu <cores>`: Worker node CPU cores (default: 1)
- `--worker-memory <GB>`: Worker node memory (default: 2)
- `--worker-disk <GB>`: Worker node disk space (default: 10)
- `--k8s-version <version>`: Kubernetes version (default: latest stable)
- `--cni <type>`: CNI plugin (flannel, cilium, none)
- `--csi <type>`: CSI plugin (local-path-provisioner, rook-ceph, none)
- `--enable-swap`: Enable swap support (K8s 1.28+)
- `--kubeadm-config <path>`: Custom kubeadm configuration file
- `--download-kubeconfig`: Download kubeconfig to local machine

**Default Cluster Configuration**:
- 1 Master node (2 CPU, 4GB RAM, 20GB disk)
- 2 Worker nodes (1 CPU, 2GB RAM, 10GB disk each)
- Flannel CNI
- Local-Path-Provisioner CSI
- MetalLB LoadBalancer
- Ubuntu 24.04 base image

##### 1.2 Cluster Deletion (`ohmykube down`)
**Status**: ✅ Implemented
**Description**: Delete the entire cluster and clean up all resources

**Command Syntax**:
```bash
ohmykube down [flags]
```

**Features**:
- Graceful pod eviction before node deletion
- VM cleanup and resource deallocation
- Configuration file cleanup
- Kubeconfig cleanup

##### 1.3 Node Management

###### 1.3.1 Add Nodes (`ohmykube add`)
**Status**: ✅ Implemented
**Description**: Add worker nodes to an existing cluster

**Command Syntax**:
```bash
ohmykube add [flags]
```

**Supported Flags**:
- `--cpu <cores>`: Node CPU cores (default: 1)
- `--memory <GB>`: Node memory (default: 2)
- `--disk <GB>`: Node disk space (default: 10)
- `--count <number>`: Number of nodes to add (default: 1)

###### 1.3.2 Delete Nodes (`ohmykube delete`)
**Status**: ✅ Implemented
**Description**: Remove nodes from an existing cluster

**Command Syntax**:
```bash
ohmykube delete <node-name> [node-name...] [flags]
```

**Supported Flags**:
- `--force`: Force deletion without pod eviction

##### 1.4 Registry Management (`ohmykube registry`)
**Status**: 🚧 Planned (Not Implemented)
**Description**: Create and manage local Harbor registry

**Command Syntax**:
```bash
ohmykube registry [subcommand] [flags]
```

**Planned Subcommands**:
- `up`: Create Harbor registry
- `down`: Delete Harbor registry
- `status`: Show registry status
- `login`: Login to registry
- `push <image>`: Push image to registry
- `pull <image>`: Pull image from registry

##### 1.5 Configuration Management (`ohmykube download-kubeconfig`)
**Status**: ✅ Implemented
**Description**: Download cluster kubeconfig for local access

#### 2. Network Plugin Support (CNI)

##### 2.1 Flannel (Default)
**Status**: ✅ Implemented
**Description**: Lightweight overlay network for basic connectivity

**Features**:
- VXLAN backend for cross-node communication
- Configurable pod CIDR (default: 10.244.0.0/16)
- Simple setup and reliable performance

##### 2.2 Cilium
**Status**: ✅ Implemented
**Description**: Advanced CNI with eBPF-based networking and security

**Features**:
- eBPF-based data plane
- Network policies and security
- Service mesh capabilities
- Observability and monitoring

##### 2.3 Custom CNI Support
**Status**: 🚧 Planned
**Description**: Support for custom CNI configurations

#### 3. Storage Plugin Support (CSI)

##### 3.1 Local-Path-Provisioner (Default)
**Status**: ✅ Implemented
**Description**: Local storage provisioner for development workloads

**Features**:
- Dynamic PV provisioning
- Local node storage utilization
- Simple configuration

##### 3.2 Rook-Ceph
**Status**: ✅ Implemented
**Description**: Distributed storage system for production-like environments

**Features**:
- Distributed block, object, and file storage
- High availability and data replication
- Storage classes for different performance tiers

#### 4. Load Balancer Support

##### 4.1 MetalLB
**Status**: ✅ Implemented
**Description**: Bare-metal load balancer for LoadBalancer services

**Features**:
- Layer 2 and BGP modes
- IP address pool management
- Service exposure outside the cluster

### Advanced Features

#### 5. Image Management System
**Status**: ✅ Implemented (Package Cache),✅ Implemented (Image Cache)

##### 5.1 Package Cache System
**Status**: ✅ Fully Implemented
**Description**: Local caching system for Kubernetes and container runtime packages

**Features**:
- ✅ Local package caching with zstd compression
- ✅ Support for multiple package types (containerd, kubectl, kubeadm, kubelet, helm, etc.)
- ✅ Automatic file format normalization to .tar.zst
- ✅ SSH-based package distribution to nodes
- ✅ Package integrity verification with checksums
- ✅ Cache statistics and cleanup operations

**Supported Packages**:
- containerd, runc, cni-plugins
- kubectl, kubeadm, kubelet
- helm, crictl, nerdctl

##### 5.2 Image Cache System
**Status**: ✅ Implemented
**Description**: Local caching system for container images

**Features**:
- ✅ Local image caching to avoid repeated downloads
- ✅  Support for multiple image sources (Docker, Podman, Helm charts)
- ✅ YAML-based index for tracking cached images
- ✅ Automatic image preheating for new nodes
- ✅ Image integrity verification and security scanning

#### 6. Multi-Cluster Management
**Status**: 🚧 Planned

##### 6.1 Project Initialization (`ohmykube init`)
**Status**: 🚧 Planned
**Description**: Initialize project workspace with cluster configurations

##### 6.2 Cluster Switching (`ohmykube switch`)
**Status**: 🚧 Planned
**Description**: Switch between multiple cluster contexts

##### 6.3 Checkpoint and Recovery
**Status**: ✅ Implemented
**Description**: Support for interrupted cluster creation recovery

#### 7. Custom Configuration Support

##### 7.1 Kubeadm Configuration
**Status**: ✅ Implemented
**Description**: Support for custom kubeadm configurations

**Supported Sections**:
- InitConfiguration
- ClusterConfiguration
- KubeletConfiguration
- KubeProxyConfiguration

##### 7.2 Lima Template Customization
**Status**: ✅ Implemented
**Description**: Support for custom Lima VM templates

### Platform Support

#### Supported Platforms
- ✅ Mac arm64 (Primary support)
- 🚧 Linux arm64/amd64
- 🚧 Windows (Experimental)

#### Virtualization Requirements
- Lima (primary)
- Future: Support for other virtualization platforms

## Non-Functional Requirements

### Performance Requirements

#### Cluster Creation Performance
- **Target**: Complete cluster creation in under 5 minutes
- **Current**: Varies based on network and hardware (5-15 minutes)
- **Optimization**: Package and image caching reduces subsequent deployments to ~3 minutes

#### Resource Utilization
- **Minimum System Requirements**:
  - 8GB RAM (for default 3-node cluster)
  - 4 CPU cores
  - 50GB available disk space
- **Recommended System Requirements**:
  - 16GB RAM
  - 8 CPU cores
  - 100GB available disk space

#### Network Performance
- **Inter-node latency**: < 1ms (VM-to-VM communication)
- **Pod-to-pod latency**: < 1ms
- **Service discovery**: < 1ms

### Reliability Requirements

#### Fault Tolerance
- **VM failure recovery**: Automatic detection and reporting
- **Network partition handling**: Graceful degradation
- **Storage failure recovery**: Data protection with Rook-Ceph

#### Data Persistence
- **Configuration persistence**: Cluster state saved locally
- **Volume data persistence**: Survives pod restarts
- **Cache persistence**: Package and image caches survive system restarts

### Scalability Requirements

#### Cluster Scaling
- **Maximum nodes**: 10 worker nodes per cluster (hardware dependent)
- **Node addition time**: < 1 minutes per node
- **Concurrent operations**: Support for parallel node operations

#### Multi-cluster Support
- **Maximum clusters**: Limited by system resources
- **Cluster isolation**: Complete resource and network isolation
- **Context switching**: < 1 second between clusters

## Technical Architecture

### System Architecture

#### Core Components

##### 1. Command Line Interface (CLI)
**Location**: `cmd/ohmykube/`
**Technology**: Cobra CLI framework
**Responsibilities**:
- Command parsing and validation
- User interaction and feedback
- Configuration management
- Error handling and reporting

##### 2. Controller Manager
**Location**: `pkg/controller/`
**Responsibilities**:
- Cluster lifecycle management
- Node orchestration
- Component coordination
- State management

##### 3. Launcher Abstraction
**Location**: `pkg/launcher/`
**Current**: Lima support
**Future**: Multi-platform virtualization support
**Responsibilities**:
- VM creation and management
- Template processing
- Resource allocation
- Network configuration

##### 4. SSH Management
**Location**: `pkg/ssh/`
**Technology**: SSH/SFTP protocols
**Responsibilities**:
- Secure remote command execution
- File transfer operations
- Connection pooling and management
- Authentication handling

##### 5. Initializer System
**Location**: `pkg/initializer/`
**Responsibilities**:
- Node environment setup
- Package installation and configuration
- System optimization
- Service initialization

##### 6. Cache Management
**Location**: `pkg/cache/`
**Responsibilities**:
- Package caching and distribution
- Image caching and management
- Compression and optimization
- Integrity verification

##### 7. Add-on Management
**Location**: `pkg/addons/`
**Responsibilities**:
- CNI plugin installation and configuration
- CSI plugin management
- Load balancer setup
- Custom add-on support

### Data Flow Architecture

#### Cluster Creation Flow
1. **Configuration Parsing**: CLI parses user input and creates cluster configuration
2. **VM Provisioning**: Launcher creates VMs based on specifications
3. **Node Initialization**: Initializer sets up each node with required packages
4. **Kubernetes Bootstrap**: kubeadm initializes the cluster
5. **Add-on Installation**: CNI, CSI, and LB components are deployed
6. **Verification**: Cluster health checks and validation
7. **Kubeconfig Distribution**: Access credentials are provided to user

#### Package Cache Flow
1. **Package Request**: Node requires specific package
2. **Cache Check**: System checks local cache for package
3. **Download**: If not cached, download from official sources
4. **Normalization**: Convert to standard .tar.zst format
5. **Storage**: Store in local cache with metadata
6. **Distribution**: Upload to target node via SSH
7. **Installation**: Extract and install on target node

### Configuration Management

#### Configuration Hierarchy
1. **Default Configuration**: Built-in sensible defaults
2. **User Configuration**: Command-line flags and options
3. **Custom Configuration**: kubeadm and Lima template overrides
4. **Runtime Configuration**: Dynamic adjustments during operation

#### Configuration Storage
- **Cluster State**: `~/.ohmykube/clusters/<cluster-name>/`
- **Cache Data**: `~/.ohmykube/cache/`

## Implementation Status

### ✅ Completed Features

#### Core Functionality
- [x] Basic cluster creation and deletion
- [x] Node addition and removal
- [x] Kubeconfig management
- [x] SSH-based remote operations
- [x] Lima integration for VM management

#### Network Plugins
- [x] Flannel CNI (default)
- [x] Cilium CNI with eBPF support
- [x] CNI plugin abstraction

#### Storage Plugins
- [x] Local-Path-Provisioner (default)
- [x] Rook-Ceph distributed storage
- [x] CSI plugin abstraction

#### Load Balancing
- [x] MetalLB integration
- [x] LoadBalancer service support

#### Package Management
- [x] Complete package cache system
- [x] File format normalization
- [x] SSH-based distribution
- [x] Integrity verification
- [x] Compression optimization
  
#### Image Management
- [x] Container image caching system
- [x] Image preheating for new nodes
- [x] Multi-source image support
- [ ] Image security scanning

#### Configuration
- [x] Custom kubeadm configuration support
- [x] Resource allocation customization
- [x] Lima template customization

### 🚧 In Progress Features



#### Enhanced CLI
- [ ] Interactive cluster creation wizard
- [ ] Progress bars and status indicators
- [ ] Colored output and formatting
- [ ] Shell completion support

### 🚧 Planned Features

#### Registry Management
- [ ] Harbor registry deployment
- [ ] Registry authentication
- [ ] Image push/pull operations
- [ ] Registry web UI access

#### Multi-Cluster Management
- [ ] Project workspace initialization
- [ ] Cluster context switching
- [ ] Cross-cluster networking
- [ ] Cluster templates and profiles

#### Advanced Features
- [ ] Cluster backup and restore
- [ ] Rolling updates and upgrades
- [ ] Monitoring and observability integration
- [ ] CI/CD pipeline integration

#### Platform Expansion
- [ ] Windows support improvement
- [ ] Cloud provider integration
- [ ] Alternative virtualization platforms
- [ ] ARM64 optimization

## Development Roadmap

### Phase 1: Core Stability (Current - Q3 2025)
**Focus**: Stabilize existing features and improve reliability

#### Priority 1 (Critical)
- [ ] Improve error handling and recovery
- [ ] Performance optimization for cluster creation
- [ ] Add comprehensive logging

#### Priority 2 (High)
- [ ] Enhanced CLI user experience
- [ ] Automated testing framework
- [ ] Documentation improvements
- [ ] Bug fixes and stability improvements

### Phase 2: Feature Expansion (Q3 2025)
**Focus**: Add major missing features and improve usability

#### Priority 1 (Critical)
- [ ] Harbor registry implementation
- [ ] Multi-cluster management basics
- [ ] Cluster backup and restore
- [ ] Windows platform support

#### Priority 2 (High)
- [ ] Advanced monitoring integration
- [ ] Custom add-on framework
- [ ] Performance monitoring and optimization
- [ ] Security hardening

### Phase 3: Ecosystem Integration (Q3-Q4 2025)
**Focus**: Integration with broader Kubernetes ecosystem

#### Priority 1 (Critical)
- [ ] CI/CD pipeline integration
- [ ] IDE and development tool integration
- [ ] Cloud provider support
- [ ] Enterprise features

#### Priority 2 (High)
- [ ] Plugin ecosystem development
- [ ] Community contribution framework
- [ ] Advanced networking features
- [ ] Compliance and security certifications

## User Experience Requirements

### Command Line Interface

#### Usability Principles
- **Simplicity**: Common operations should require minimal commands
- **Consistency**: Similar operations should use similar syntax
- **Discoverability**: Help and documentation should be easily accessible
- **Feedback**: Clear progress indication and error messages

#### Required CLI Features
- [ ] Interactive mode for guided cluster creation
- [ ] Colored output with configurable themes
- [ ] Progress bars for long-running operations
- [ ] Verbose and quiet modes
- [ ] JSON/YAML output formats for automation

#### Error Handling
- **Clear Error Messages**: Specific, actionable error descriptions
- **Recovery Suggestions**: Automatic suggestions for common issues
- **Debug Mode**: Detailed logging for troubleshooting
- **Graceful Degradation**: Partial functionality when possible

### Documentation Requirements

#### User Documentation
- [ ] Quick start guide (5-minute setup)
- [ ] Comprehensive user manual
- [ ] Troubleshooting guide
- [ ] Best practices documentation
- [ ] Video tutorials and demos

#### Developer Documentation
- [ ] Architecture documentation
- [ ] API reference
- [ ] Plugin development guide
- [ ] Contributing guidelines
- [ ] Code examples and samples

## Security Requirements

### Authentication and Authorization

#### SSH Security
- **Key-based Authentication**: Default to SSH key authentication
- **Password Fallback**: Support for password authentication when needed
- **Key Management**: Automatic SSH key generation and management
- **Connection Security**: Encrypted connections for all operations

#### Cluster Security
- **RBAC**: Role-based access control for cluster resources
- **Network Policies**: Default network segmentation
- **Pod Security**: Pod security standards enforcement
- **Secret Management**: Secure handling of sensitive data

### Data Protection

#### Data Encryption
- **Data at Rest**: Encrypted storage for sensitive configuration
- **Data in Transit**: TLS encryption for all network communication
- **Key Management**: Secure key storage and rotation

#### Privacy
- **No Data Collection**: No telemetry or usage data collection by default
- **Local Storage**: All data stored locally on user's machine
- **Audit Logging**: Optional audit trail for security compliance

### Vulnerability Management

#### Security Scanning
- [ ] Container image vulnerability scanning
- [ ] Package vulnerability checking
- [ ] Configuration security validation
- [ ] Regular security updates

#### Compliance
- [ ] CIS Kubernetes Benchmark validation
- [ ] Compliance reporting and auditing

## Performance Requirements

### Cluster Creation Performance

#### Target Metrics
- **Initial Cluster Creation**: < 10 minutes (3-node cluster)
- **Subsequent Creations**: < 5 minutes (with cache)
- **Node Addition**: < 1 minutes per node
- **Cluster Deletion**: < 1 minutes

#### Optimization Strategies
- **Parallel Operations**: Concurrent node provisioning and setup
- **Caching**: Package and image caching for faster deployments
- **Incremental Updates**: Only update changed components
- **Resource Optimization**: Efficient resource allocation and utilization

### Runtime Performance

#### Resource Utilization
- **Memory Overhead**: < 100MB for OhMyKube processes
- **CPU Overhead**: < 5% during normal operations
- **Disk I/O**: Optimized for SSD storage
- **Network Bandwidth**: Efficient use of available bandwidth


## Testing Requirements

### Test Categories

#### Unit Testing
- **Coverage Target**: > 80% code coverage
- **Test Framework**: Go testing framework with testify
- **Mock Objects**: Comprehensive mocking for external dependencies
- **Continuous Testing**: Automated testing on every commit

#### Integration Testing
- **End-to-End Tests**: Complete cluster lifecycle testing
- **Component Integration**: Inter-component communication testing
- **Platform Testing**: Multi-platform compatibility testing
- **Performance Testing**: Automated performance regression testing

#### User Acceptance Testing
- **Scenario Testing**: Real-world usage scenario validation
- **Usability Testing**: CLI usability and user experience testing
- **Documentation Testing**: Documentation accuracy and completeness
- **Compatibility Testing**: Version compatibility and upgrade testing

### Test Infrastructure

#### Automated Testing
- [ ] GitHub Actions CI/CD pipeline
- [ ] Automated testing on multiple platforms
- [ ] Performance benchmarking
- [ ] Security vulnerability scanning

#### Manual Testing
- [ ] Release candidate testing
- [ ] User experience testing
- [ ] Edge case validation
- [ ] Stress testing

## Conclusion

This comprehensive requirements document provides a complete specification for the OhMyKube project, covering all aspects from functional requirements to implementation details. The document serves as a roadmap for development and a reference for contributors and users.

### Key Success Metrics
- **User Adoption**: Growing community of users and contributors
- **Reliability**: < 1% failure rate for cluster operations
- **Performance**: Meeting all specified performance targets
- **User Satisfaction**: Positive feedback and low support burden

### Next Steps
1. Review and validate requirements with stakeholders
2. Prioritize features based on user feedback and business value
3. Create detailed implementation plans for each phase
4. Establish testing and quality assurance processes
5. Begin implementation of Phase 1 priorities

This document will be updated regularly to reflect changes in requirements, implementation progress, and user feedback.