package kubeadm

const INIT_CONFIG_V1_BETA_4 = `
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
  advertiseAddress: %s
  bindPort: 6443
`

const INIT_CONFIG_V1_BETA_3 = `
apiVersion: kubeadm.k8s.io/v1beta3
bootstrapTokens:
- groups:
  - system:bootstrappers:kubeadm:default-node-token
  token: abcdef.0123456789abcdef
  ttl: 24h0m0s
  usages:
  - signing
  - authentication
kind: InitConfiguration
localAPIEndpoint:
  advertiseAddress: %s
  bindPort: 6443
nodeRegistration:
  criSocket: unix:///var/run/containerd/containerd.sock
  imagePullPolicy: IfNotPresent
`
