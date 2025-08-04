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
- `ohmykube down` - Delete cluster
- `ohmykube list` - List all nodes
- `ohmykube load IMAGE_NAME` - Load local container image to cluster nodes
  - Supports multiple container runtimes: `--runtime docker|podman|nerdctl`
  - Architecture validation: `--skip-arch-check` to bypass compatibility checks
  - Target specific nodes: `--nodes node1,node2`
- See README.md for full CLI usage

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
- Currently supports Lima only, designed for future cloud provider support
- Provider interface abstracts VM lifecycle operations

#### Cluster State Management
- `pkg/config/cluster.go` - Central cluster configuration with thread-safe operations
- Uses sync.RWMutex for concurrent access to cluster state
- Node groups organize nodes with same specifications
- Condition-based status tracking for nodes and cluster

#### Image Caching System
- `pkg/cache/` - Sophisticated caching for container images and system packages
- Architecture-aware image discovery and downloading
- Compression and decompression for efficient storage
- Implements `interfaces.ClusterImageTracker` for cluster-level image tracking

#### Plugin Architecture
- `pkg/addons/` - Extensible addon system
- CNI plugins: Cilium, Flannel
- CSI plugins: Local-path, Rook
- LB plugins: MetalLB
- Type-safe plugin registration and lifecycle management

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