package addons

type AddonManager interface {
	// InstallCNI installs CNI
	InstallCNI() error
	// InstallCSI installs CSI
	InstallCSI() error
	// InstallLB installs LoadBalancer
	InstallLB() error
}
