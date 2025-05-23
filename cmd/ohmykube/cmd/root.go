package cmd

import (
	"os"

	"github.com/monshunter/ohmykube/pkg/log"
	"github.com/spf13/cobra"
)

var (
	password      string
	sshKeyFile    string
	sshPubKeyFile string
)

var rootCmd = &cobra.Command{
	Use:   "ohmykube",
	Short: "OhMyKube - Tool for quickly setting up real Kubernetes clusters locally",
	Long: `OhMyKube is a tool based on Lima and kubeadm, designed to quickly set up
real Kubernetes clusters on developer computers using virtual machines, 
including Cilium(CNI), Rook(CSI), and MetalLB(LB).`,
	// If preparation work is needed before execution, PersistentPreRun can be added
	PersistentPreRun: func(cmd *cobra.Command, args []string) {
		// Initialization work
	},
}

// Execute adds all child commands to the root command and sets flags, this is the entry point called by main.go
func Execute() error {
	return rootCmd.Execute()
}

const (
	defaultPassword = "ohmykube123"
)

var (
	defaultSSHKeyFile    string
	defaultSSHPubKeyFile string
	clusterName          string
	launcher             string
	parallel             int
	limaTemplate         string
	proxyMode            string
)

func init() {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		log.Fatalf("Failed to get user home directory: %v", err)
	}
	defaultSSHKeyFile = homeDir + "/.ssh/id_rsa"
	defaultSSHPubKeyFile = homeDir + "/.ssh/id_rsa.pub"
	// Global flags can be added here
	rootCmd.PersistentFlags().StringVar(&password, "password", defaultPassword, "Password")
	rootCmd.PersistentFlags().StringVar(&sshKeyFile, "ssh-key", defaultSSHKeyFile, "SSH private key file")
	rootCmd.PersistentFlags().StringVar(&sshPubKeyFile, "ssh-pub-key", defaultSSHPubKeyFile, "SSH public key file")
	rootCmd.PersistentFlags().StringVar(&clusterName, "name", "ohmykube", "Cluster name")
	rootCmd.PersistentFlags().StringVar(&launcher, "launcher", "limactl", "Launcher to use (currently only limactl is supported)")
	rootCmd.PersistentFlags().IntVar(&parallel, "parallel", 1, "Parallel number for creating nodes")
	rootCmd.PersistentFlags().StringVar(&limaTemplate, "lima-template", "ubuntu-24.04", "Lima file or template")
	rootCmd.PersistentFlags().StringVar(&proxyMode, "proxy-mode", "ipvs", "Proxy mode (iptables or ipvs)")
	// Add subcommands
	rootCmd.AddCommand(upCmd)
	rootCmd.AddCommand(downCmd)
	rootCmd.AddCommand(registryCmd)
	rootCmd.AddCommand(addCmd)
	rootCmd.AddCommand(deleteCmd)
}
