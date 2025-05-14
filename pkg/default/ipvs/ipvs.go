package ipvs

// K8S_MODULES_CONFIG 是需要加载的内核模块配置内容
const K8S_MODULES_CONFIG = `overlay
br_netfilter
ip_vs
ip_vs_rr
ip_vs_wrr
ip_vs_sh
nf_conntrack
`

// K8S_SYSCTL_CONFIG 是需要设置的系统参数配置内容
const K8S_SYSCTL_CONFIG = `net.bridge.bridge-nf-call-iptables  = 1
net.bridge.bridge-nf-call-ip6tables = 1
net.ipv4.ip_forward                 = 1
`
