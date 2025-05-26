# OhMyKube Documentation

## Overview
This directory contains comprehensive documentation for the OhMyKube project, including requirements, technical specifications, implementation details, and user guides.

## Documentation Structure

### 📋 Requirements and Specifications
- **[requirement.md](requirement.md)** - Comprehensive requirements document covering all aspects of the project
- **[technical-specifications.md](technical-specifications.md)** - Detailed technical specifications and implementation guidelines
- **[requirement-fqa.md](requirement-fqa.md)** - Frequently asked questions about requirements and features

### 🗺️ Implementation and Planning
- **[implementation-roadmap.md](implementation-roadmap.md)** - Detailed development roadmap with phases and timelines
- **[packages-cache.md](packages-cache.md)** - Complete documentation of the package cache system (✅ Implemented)
- **[image-cache-integration-requriement.md](image-cache-integration-requriement.md)** - Image cache system requirements and integration details

### 🚀 Quick Navigation

#### For Users
- **Getting Started**: See [../README.md](../README.md) for installation and basic usage
- **Configuration Options**: Check [requirement-fqa.md](requirement-fqa.md) section 3
- **Platform Support**: Review [requirement-fqa.md](requirement-fqa.md) section 4
- **Troubleshooting**: Refer to [requirement-fqa.md](requirement-fqa.md) section 10

#### For Developers
- **Architecture Overview**: See [technical-specifications.md](technical-specifications.md)
- **Implementation Status**: Check [requirement.md](requirement.md) section 5
- **Development Roadmap**: Review [implementation-roadmap.md](implementation-roadmap.md)
- **API Specifications**: See [technical-specifications.md](technical-specifications.md) section 3

#### For Contributors
- **Current Priorities**: Check [implementation-roadmap.md](implementation-roadmap.md) Phase 1
- **Code Quality Standards**: See [implementation-roadmap.md](implementation-roadmap.md) implementation guidelines
- **Testing Requirements**: Review [requirement.md](requirement.md) section 10

## Document Status

### ✅ Complete and Current
- **requirement.md** - Comprehensive requirements specification
- **technical-specifications.md** - Technical implementation details
- **requirement-fqa.md** - FAQ covering common questions
- **implementation-roadmap.md** - Development roadmap and planning
- **packages-cache.md** - Package cache system documentation

### 🚧 In Progress
- **image-cache-integration-requriement.md** - Needs completion for image cache system
- **images-cache-requriement.md** - Empty file, needs content

### 📝 Planned Documentation
- **user-guide.md** - Comprehensive user guide with examples
- **developer-guide.md** - Developer setup and contribution guide
- **api-reference.md** - Complete API reference documentation
- **troubleshooting-guide.md** - Detailed troubleshooting procedures
- **best-practices.md** - Best practices for using OhMyKube

## Key Features Overview

### ✅ Implemented Features
- **Core Cluster Management**: Create, delete, and manage Kubernetes clusters
- **Node Management**: Add and remove worker nodes dynamically
- **Network Plugins**: Flannel (default) and Cilium CNI support
- **Storage Plugins**: Local-Path-Provisioner and Rook-Ceph CSI support
- **Load Balancing**: MetalLB integration for LoadBalancer services
- **Package Cache**: Complete package caching system with compression and distribution
- **SSH Management**: Secure remote operations with key-based authentication
- **Configuration**: Custom kubeadm configurations and Lima template support

### 🚧 In Progress Features
- **Image Cache System**: Container image caching and distribution
- **Enhanced CLI**: Progress bars, colored output, and interactive modes
- **Comprehensive Testing**: Automated testing framework and CI/CD pipeline

### 🚧 Planned Features
- **Harbor Registry**: Local container registry deployment and management
- **Multi-Cluster Management**: Project workspaces and cluster switching
- **Windows Support**: Full Windows platform compatibility
- **Monitoring Integration**: Built-in monitoring and observability tools
- **CI/CD Integration**: Integration with popular CI/CD platforms

## System Requirements

### Minimum Requirements
- **CPU**: 4 cores (Intel/AMD x64 or Apple Silicon)
- **Memory**: 8GB RAM
- **Storage**: 50GB available disk space
- **Network**: Broadband internet connection
- **OS**: macOS 10.15+, Linux (Ubuntu 20.04+, CentOS 8+), Windows 10+ (experimental)

### Recommended Requirements
- **CPU**: 8+ cores
- **Memory**: 16GB+ RAM
- **Storage**: 100GB+ SSD storage
- **Network**: High-speed internet connection

### Dependencies
- **Lima**: 0.18.0+ (virtualization platform)
- **Go**: 1.23.0+ (for building from source)
- **SSH**: SSH client and key pair for authentication

## Quick Start

### Installation
```bash
# Clone the repository
git clone https://github.com/monshunter/ohmykube.git
cd ohmykube

# Install dependencies (macOS)
brew install lima

# Build and install
make install
```

### Basic Usage
```bash
# Create a cluster (default: 1 master + 2 workers)
ohmykube up

# View cluster status
export KUBECONFIG=~/.kube/ohmykube-config
kubectl get nodes

# Add a worker node
ohmykube add --cpu 2 --memory 4 --disk 20

# Delete the cluster
ohmykube down
```

### Advanced Configuration
```bash
# Custom cluster with Cilium CNI and Rook-Ceph storage
ohmykube up --workers 3 --cni cilium --csi rook-ceph \
            --master-cpu 4 --master-memory 8 \
            --worker-cpu 2 --worker-memory 4

# Use custom kubeadm configuration
ohmykube up --kubeadm-config /path/to/custom-config.yaml
```

## Performance Characteristics

### Cluster Creation Times(3 worker nodes)
- **Initial Creation**: 5-15 minutes (network dependent)
- **Cached Creation**: 2-5 minutes (with package & image cache)
- **Node Addition**:  < 1 minutes per node

### Resource Usage
- **OhMyKube Overhead**: < 100MB memory, < 5% CPU
- **Default Cluster**: ~6GB RAM, ~3 CPU cores
- **Storage**: ~30GB for default 3-node cluster

### Scalability Limits
- **Maximum Nodes**: 10 worker nodes per cluster
- **Maximum Clusters**: Limited by system resources
- **Recommended**: 1-5 worker nodes for development

## Support and Community

### Getting Help
- **Documentation**: Start with this documentation set
- **GitHub Issues**: Report bugs and request features
- **Discussions**: Community Q&A and discussions
- **FAQ**: Check [requirement-fqa.md](requirement-fqa.md) for common questions

### Contributing
- **Code Contributions**: Follow the development roadmap priorities
- **Documentation**: Help improve and expand documentation
- **Testing**: Test on different platforms and report issues
- **Feedback**: Provide user experience feedback and suggestions

### Roadmap and Development
- **Current Phase**: Core Stability and Reliability (Q2 2025)
- **Next Phase**: Feature Expansion and Platform Support (Q3 2025)
- **Long-term**: Ecosystem Integration and Enterprise Features (Q3-Q4 2025)

## Document Maintenance

### Update Schedule
- **Requirements**: Updated as features are implemented or requirements change
- **Technical Specs**: Updated with architectural changes and new APIs
- **Roadmap**: Reviewed and updated monthly
- **FAQ**: Updated as new questions arise

### Version Control
All documentation is version controlled alongside the codebase to ensure consistency between documentation and implementation.

### Feedback
Documentation feedback is welcome through:
- GitHub Issues for corrections and improvements
- Pull Requests for direct contributions
- Discussions for suggestions and questions

---

**Last Updated**: May 2025
**Documentation Version**: 0.0.1
**Project Version**: See [../README.md](../README.md) for current project version
