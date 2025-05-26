# OhMyKube Requirements FAQ

## Frequently Asked Questions About Requirements

### 1. **What Makes OhMyKube a "Real" Kubernetes Cluster?**

**Q**: How does OhMyKube differ from container-based solutions like kind or k3d?

**A**: OhMyKube creates "real" Kubernetes clusters by using actual virtual machines instead of containers to simulate nodes. This provides several key advantages:

- **True Resource Isolation**: Each node runs in its own VM with dedicated CPU, memory, and storage
- **Realistic Network Stack**: Full network stack including iptables, routing, and network interfaces
- **Production-Like Environment**: Same kernel, systemd, and system services as production
- **Storage Realism**: Real block devices and filesystem behavior
- **Security Boundaries**: VM-level isolation similar to production environments

**Current Implementation**: ✅ Fully implemented with Lima-based VMs

### 2. **MVP Scope and Harbor Registry**

**Q**: Is Harbor registry part of the core MVP or an optional feature?

**A**: Harbor registry is **not** part of the core MVP. The current scope is:

**Core MVP (✅ Implemented)**:
- Basic cluster creation and management
- CNI plugins (Flannel, Cilium)
- CSI plugins (Local-Path-Provisioner, Rook-Ceph)
- MetalLB load balancer

**Optional Features (🚧 Planned)**:
- Harbor registry (`ohmykube registry` command)
- Multi-cluster management
- Advanced monitoring and observability

Harbor will be implemented as a separate command that creates an additional VM with Harbor installed, not automatically included in `ohmykube up`.

### 3. **Configuration Options and Customization**

**Q**: What configuration options are available for cluster customization?

**A**: OhMyKube supports extensive configuration through command-line flags and custom configuration files:

**Resource Configuration** (✅ Implemented):
```bash
ohmykube up --workers 3 --master-cpu 4 --master-memory 8 --master-disk 20 \
            --worker-cpu 2 --worker-memory 4 --worker-disk 10
```

**Component Selection** (✅ Implemented):
```bash
ohmykube up --cni cilium --csi rook-ceph --k8s-version v1.33.0
```

**Custom kubeadm Configuration** (✅ Implemented):
```bash
ohmykube up --kubeadm-config /path/to/custom-config.yaml
```

**Default Configuration**:
- 1 Master node (2 CPU, 4GB RAM, 20GB disk)
- 2 Worker nodes (1 CPU, 2GB RAM, 10GB disk each)
- Latest stable Kubernetes version
- Flannel CNI, Local-Path-Provisioner CSI, MetalLB LB

### 4. **Platform Compatibility and Support**

**Q**: Which operating systems and architectures are supported?

**A**: Current platform support status:

**Primary Support** (✅ Production Ready):
- macOS arm64 (Apple Silicon)

**Secondary Support** (✅ Functional):
- macOS amd64 (Intel)
- Linux arm64
- Linux amd64

**Experimental Support** (🚧 Limited):
- Windows (requires WSL2 and Lima)

**Requirements**:
- Lima virtualization platform
- Go 1.23.0 or higher for building from source
- 8GB+ RAM, 4+ CPU cores, 50GB+ disk space

### 5. **User Experience and Interface Design**

**Q**: How user-friendly is the command-line interface?

**A**: OhMyKube prioritizes user experience with several features:

**Current UX Features** (✅ Implemented):
- Simple, intuitive command syntax
- Comprehensive help and documentation
- Automatic kubeconfig download and setup
- Clear error messages with context

**Planned UX Improvements** (🚧 In Development):
- Progress bars for long-running operations
- Colored output with themes
- Interactive cluster creation wizard
- Shell completion for all commands
- Verbose and quiet modes

**Error Handling**:
- Descriptive error messages with suggested solutions
- Automatic retry for transient failures
- Graceful degradation for partial failures
- Debug mode for troubleshooting

### 6. **Node Management and High Availability**

**Q**: What types of nodes can be added or removed?

**A**: Current node management capabilities:

**Worker Node Management** (✅ Implemented):
- `ohmykube add`: Add worker nodes to existing cluster
- `ohmykube delete <node-name>`: Remove worker nodes
- Support for custom resource allocation per node
- Graceful pod eviction before node removal

**Control Plane Management** (🚧 Planned):
- High Availability (HA) control plane support
- Multiple master node deployment
- Control plane node replacement
- Etcd backup and restore

**Current Limitations**:
- Only single master node supported
- Cannot add/remove control plane nodes
- HA features planned for Phase 2 development

### 7. **Version Dependencies and Compatibility**

**Q**: What are the version requirements for dependencies?

**A**: Specific version requirements and compatibility:

**Lima Requirements**:
- Minimum version: 1.0
- Recommended: Latest stable release
- Template support: Ubuntu 24.04 (default), custom templates supported

**Kubernetes Compatibility**:
- Supported versions: 1.24.x - 1.33.x
- Default: Latest stable release
- Custom versions supported via `--k8s-version` flag

**Go Build Requirements**:
- Minimum: Go 1.23.0
- Recommended: Latest stable Go release

**Container Runtime**:
- containerd (default and recommended)
- Docker (experimental support)

### 8. **Performance and Scalability**

**Q**: What are the performance characteristics and limits?

**A**: Performance specifications and limitations:

**Cluster Creation Performance**:
- Initial creation: 5-15 minutes (network dependent)
- Cached creation: 3-5 minutes
- Node addition: <1 minutes per node

**Scalability Limits**:
- Maximum worker nodes: 10 per cluster (hardware dependent)
- Maximum clusters: Limited by system resources
- Recommended: 1-5 worker nodes for development

**Resource Requirements**:
- Minimum: 8GB RAM, 4 CPU cores, 50GB disk
- Recommended: 16GB RAM, 8 CPU cores, 100GB SSD

### 9. **Security and Networking**

**Q**: What security features and network configurations are available?

**A**: Security and networking capabilities:

**Security Features** (✅ Implemented):
- SSH key-based authentication (default)
- Encrypted communication between components
- RBAC enabled by default
- Pod Security Standards enforcement

**Network Configuration** (✅ Implemented):
- Configurable pod and service CIDRs
- CNI plugin selection (Flannel, Cilium)
- MetalLB for LoadBalancer services
- Network policies support (with Cilium)

**Planned Security Enhancements** (🚧 Future):
- Image vulnerability scanning
- Policy enforcement frameworks
- Compliance reporting
- Advanced network security

### 10. **Troubleshooting and Support**

**Q**: What troubleshooting and support options are available?

**A**: Troubleshooting and support resources:

**Built-in Diagnostics** (✅ Available):
- Comprehensive logging with configurable levels
- Health checks for all components
- Cluster validation and verification
- Debug mode for detailed troubleshooting

**Documentation** (🚧 Future):
- Comprehensive user documentation
- Troubleshooting guides
- Best practices documentation
- API reference documentation

**Community Support**:
- GitHub Issues for bug reports and feature requests
- Community discussions and Q&A
- Contributing guidelines for community involvement

**Planned Support Enhancements** (🚧 Future):
- Interactive troubleshooting tools
- Automated problem detection and resolution
- Performance monitoring and alerting
- Enterprise support options

## Additional Questions?

If you have additional questions not covered in this FAQ, please:

1. Check the main requirements document for detailed specifications
2. Review the technical specifications for implementation details
3. Consult the implementation roadmap for planned features
4. Submit an issue on GitHub for specific questions or clarifications

This FAQ will be updated regularly as new questions arise and features are implemented.
