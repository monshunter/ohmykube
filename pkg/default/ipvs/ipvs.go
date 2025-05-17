package ipvs

// K8S_MODULES_CONFIG is the kernel ipvs modules configuration for Kubernetes
const K8S_MODULES_CONFIG = `overlay
br_netfilter
ip_vs
ip_vs_rr
ip_vs_wrr
ip_vs_sh
nf_conntrack
`

// K8S_SYSCTL_CONFIG is the system parameters configuration for Kubernetes
const K8S_SYSCTL_CONFIG = `net.bridge.bridge-nf-call-iptables  = 1
net.bridge.bridge-nf-call-ip6tables = 1
net.ipv4.ip_forward                 = 1
`
