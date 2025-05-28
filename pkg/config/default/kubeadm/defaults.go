package kubeadm

import (
	"fmt"

	"gopkg.in/yaml.v3"
)

// loadInitConfig loads the default InitConfiguration configuration
func loadInitConfig(advertiseAddress string) YAMLDocument {
	yamlTemplate := `
apiVersion: kubeadm.k8s.io/v1beta4
# Bootstrap tokens are used for node joining
# This is a fixed token with a very long TTL for local testing purposes
# DO NOT use this approach in production environments!
bootstrapTokens:
  - groups:
      - system:bootstrappers:kubeadm:default-node-token
    # Fixed token for easier debugging and testing
    # First part: 6 chars (0-9a-z)
    # Second part: 16 chars (0-9a-z)
    token: localk.8s0123456789test
    # Set a very long TTL (10 years) for local testing
    ttl: 24h0m0s
    usages:
      - signing
      - authentication
kind: InitConfiguration
nodeRegistration:
  criSocket: unix:///var/run/containerd/containerd.sock
localAPIEndpoint:
  advertiseAddress: "%s"
  bindPort: 6443
`
	yamlStr := fmt.Sprintf(yamlTemplate, advertiseAddress)
	var doc YAMLDocument
	yaml.Unmarshal([]byte(yamlStr), &doc)
	return doc
}

// loadClusterConfig loads the default ClusterConfiguration configuration
func loadClusterConfig(version string) YAMLDocument {
	const template = `
# ClusterConfiguration
apiServer: {}
apiVersion: kubeadm.k8s.io/v1beta4
caCertificateValidityPeriod: 87600h0m0s
certificateValidityPeriod: 8760h0m0s
certificatesDir: /etc/kubernetes/pki
clusterName: kubernetes
controllerManager: {}
dns: {}
encryptionAlgorithm: RSA-2048
etcd:
  local:
    dataDir: /var/lib/etcd
imageRepository: registry.k8s.io
kind: ClusterConfiguration
kubernetesVersion: %s
networking:
  dnsDomain: cluster.local
  podSubnet: 10.244.0.0/16
  serviceSubnet: 10.96.0.0/12
proxy: {}
scheduler: {}
`
	yamlStr := fmt.Sprintf(template, version)
	var doc YAMLDocument
	yaml.Unmarshal([]byte(yamlStr), &doc)
	return doc
}

// loadKubeletConfig loads the default KubeletConfiguration configuration
func loadKubeletConfig() YAMLDocument {
	yamlStr := `
# KubeletConfiguration
apiVersion: kubelet.config.k8s.io/v1beta1
authentication:
  anonymous:
    enabled: false
  webhook:
    cacheTTL: 0s
    enabled: true
  x509:
    clientCAFile: /etc/kubernetes/pki/ca.crt
authorization:
  mode: Webhook
  webhook:
    cacheAuthorizedTTL: 0s
    cacheUnauthorizedTTL: 0s
cgroupDriver: systemd
clusterDNS:
  - 10.96.0.10
clusterDomain: cluster.local
containerRuntimeEndpoint: ""
cpuManagerReconcilePeriod: 0s
crashLoopBackOff: {}
evictionPressureTransitionPeriod: 0s
fileCheckFrequency: 0s
healthzBindAddress: 127.0.0.1
healthzPort: 10248
httpCheckFrequency: 0s
imageMaximumGCAge: 0s
imageMinimumGCAge: 0s
kind: KubeletConfiguration
logging:
  flushFrequency: 0
  options:
    json:
      infoBufferSize: "0"
    text:
      infoBufferSize: "0"
  verbosity: 0
memorySwap: {}
nodeStatusReportFrequency: 0s
nodeStatusUpdateFrequency: 0s
# resolvConf: /run/systemd/resolve/resolv.conf
resolvConf: /etc/ohmykube/resolve/resolv.conf
rotateCertificates: true
runtimeRequestTimeout: 0s
shutdownGracePeriod: 0s
shutdownGracePeriodCriticalPods: 0s
staticPodPath: /etc/kubernetes/manifests
streamingConnectionIdleTimeout: 0s
syncFrequency: 0s
volumeStatsAggPeriod: 0s
`
	var doc YAMLDocument
	yaml.Unmarshal([]byte(yamlStr), &doc)
	return doc
}

// loadKubeProxyConfig loads the default KubeProxyConfiguration configuration
func loadKubeProxyConfig(proxyMode string) YAMLDocument {
	const template = `
# KubeProxyConfiguration
apiVersion: kubeproxy.config.k8s.io/v1alpha1
bindAddress: 0.0.0.0
bindAddressHardFail: false
clientConnection:
  acceptContentTypes: ""
  burst: 0
  contentType: ""
  kubeconfig: /var/lib/kube-proxy/kubeconfig.conf
  qps: 0
clusterCIDR: 10.244.0.0/16
configSyncPeriod: 0s
conntrack:
  maxPerCore: null
  min: null
  tcpBeLiberal: false
  tcpCloseWaitTimeout: null
  tcpEstablishedTimeout: null
  udpStreamTimeout: 0s
  udpTimeout: 0s
detectLocal:
  bridgeInterface: ""
  interfaceNamePrefix: ""
detectLocalMode: ""
enableProfiling: false
healthzBindAddress: ""
hostnameOverride: ""
iptables:
  localhostNodePorts: null
  masqueradeAll: false
  masqueradeBit: null
  minSyncPeriod: 0s
  syncPeriod: 0s
ipvs:
  excludeCIDRs: null
  minSyncPeriod: 0s
  scheduler: ""
  strictARP: %v
  syncPeriod: 0s
  tcpFinTimeout: 0s
  tcpTimeout: 0s
  udpTimeout: 0s
kind: KubeProxyConfiguration
logging:
  flushFrequency: 0
  options:
    json:
      infoBufferSize: "0"
    text:
      infoBufferSize: "0"
  verbosity: 0
metricsBindAddress: ""
mode: %s
nftables:
  masqueradeAll: false
  masqueradeBit: null
  minSyncPeriod: 0s
  syncPeriod: 0s
nodePortAddresses: null
oomScoreAdj: null
portRange: ""
showHiddenMetricsForVersion: ""
winkernel:
  enableDSR: false
  forwardHealthCheckVip: false
  networkName: ""
  rootHnsEndpointName: ""
  sourceVip: ""
`
	if proxyMode == "" {
		proxyMode = "iptables"
	}
	yamlStr := fmt.Sprintf(template, proxyMode == "ipvs", proxyMode)
	var doc YAMLDocument
	yaml.Unmarshal([]byte(yamlStr), &doc)
	return doc
}
