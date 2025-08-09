package config

import "fmt"

// GenerateConfigTemplate generates a configuration template with comments
func GenerateConfigTemplate(clusterName, provider, template string, updateSystem bool) string {
	return fmt.Sprintf(`# OhMyKube Cluster Configuration
# This file defines a Kubernetes cluster configuration for OhMyKube
# Use with: ohmykube up -f ohmykube.yaml

# Important notes:
# 1. Cluster name in configuration file must be unique
# 2. If metallb load balancer is enabled, proxy mode will be automatically set to ipvs
# 3. Resource configuration should be adjusted according to actual needs
# 4. Custom script paths must be absolute paths and exist locally
# 5. File upload format: local_path:remote_path[:mode[:owner]]

apiVersion: ohmykube.dev/v1alpha1
kind: Cluster
metadata:
  # Cluster name, must be unique
  name: %s
  
  # Optional: cluster labels
  # labels:
  #   environment: development
  #   team: platform
  
  # Optional: cluster annotations
  # annotations:
  #   description: "Development cluster for testing"

spec:
  # Kubernetes version, e.g.: v1.33.0, v1.32.0
  kubernetesVersion: v1.33.0
  
  # VM provider, currently only lima is supported
  provider: %s
  
  # Update system packages before installation
  updateSystem: %t
  
  # Network configuration
  networking:
    # Proxy mode: iptables or ipvs
    proxyMode: iptables
    
    # CNI plugin: flannel, cilium, none
    cni: flannel
    
    # Pod network CIDR
    podSubnet: 10.244.0.0/16
    
    # Service network CIDR
    serviceSubnet: 10.96.0.0/12
    
    # Load balancer: metallb (optional, leave empty to disable)
    # loadbalancer: metallb
  
  # Storage configuration
  storage:
    # CSI plugin: local-path-provisioner, rook-ceph, none
    csi: local-path-provisioner
  
  # Node configuration
  nodes:
    # Master node group
    master:
      - # Number of replicas (usually 1)
        replica: 1
        
        # Group ID (unique identifier)
        groupid: 1
        
        # Lima template (e.g.: ubuntu-24.04)
        template: %s
        
        # Resource configuration
        resources:
          cpu: "2"        # CPU cores
          memory: 4Gi     # Memory size
          storage: 20Gi   # Disk size
        
        # Optional: node metadata
        # metadata:
        #   labels:
        #     node-role.kubernetes.io/control-plane: ""
        #     custom-label: value
        #   annotations:
        #     custom-annotation: value
        #   taints:
        #     - key: node-role.kubernetes.io/control-plane
        #       effect: NoSchedule
        
        # Optional: custom initialization configuration
        # customInit:
        #   hooks:
        #     preSystemInit:
        #       - /path/to/pre-system-script.sh
        #     postSystemInit:
        #       - /path/to/post-system-script.sh
        #     preK8sInit:
        #       - /path/to/pre-k8s-script.sh
        #     postK8sInit:
        #       - /path/to/post-k8s-script.sh
        #   files:
        #     - localPath: /local/path/config.yml
        #       remotePath: /etc/config.yml
        #       mode: "0644"
        #       owner: "root:root"
    
    # Worker node group
    workers:
      - # Number of replicas
        replica: 2
        
        # Group ID (unique identifier)
        groupid: 2
        
        # Lima template
        template: %s
        
        # Resource configuration
        resources:
          cpu: "1"        # CPU cores
          memory: 2Gi     # Memory size
          storage: 10Gi   # Disk size
        
        # Optional: node metadata
        # metadata:
        #   labels:
        #     node-role.kubernetes.io/worker: ""
        #     custom-label: value
        #   annotations:
        #     custom-annotation: value
        #   taints:
        #     - key: custom-taint
        #       value: custom-value
        #       effect: NoSchedule
        
        # Optional: custom initialization configuration
        # customInit:
        #   hooks:
        #     preSystemInit:
        #       - /path/to/worker-pre-system.sh
        #     postSystemInit:
        #       - /path/to/worker-post-system.sh
        #     preK8sInit:
        #       - /path/to/worker-pre-k8s.sh
        #     postK8sInit:
        #       - /path/to/worker-post-k8s.sh
        #   files:
        #     - localPath: /local/worker-config.yml
        #       remotePath: /etc/worker-config.yml
        #       mode: "0644"
        #       owner: "root:root"
`, clusterName, provider, updateSystem, template, template)
}
