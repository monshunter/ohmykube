package app

import (
	"os"

	"github.com/monshunter/ohmykube/pkg/log"
	"github.com/spf13/cobra"
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

// Run adds all child commands to the root command and sets flags, this is the entry point called by main.go
func Run() error {
	return rootCmd.Execute()
}

var (
	password             string
	sshKeyFile           string
	sshPubKeyFile        string
	defaultSSHKeyFile    string
	defaultSSHPubKeyFile string
	clusterName          string
	launcher             string
	parallel             int
	limaTemplate         string
	proxyMode            string
	updateSystem         bool
)

func init() {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		log.Fatalf("Failed to get user home directory: %v", err)
	}
	defaultSSHKeyFile = homeDir + "/.ssh/id_rsa"
	defaultSSHPubKeyFile = homeDir + "/.ssh/id_rsa.pub"
	// Global flags can be added here

	rootCmd.PersistentFlags().StringVar(&password, "password", "ohmykube123", "root password")
	rootCmd.PersistentFlags().StringVar(&sshKeyFile, "ssh-key", defaultSSHKeyFile, "ssh private key file")
	rootCmd.PersistentFlags().StringVar(&sshPubKeyFile, "ssh-pub-key", defaultSSHPubKeyFile, "ssh public key file")
	rootCmd.PersistentFlags().StringVar(&clusterName, "name", "ohmykube", "Cluster name")
	rootCmd.PersistentFlags().StringVar(&launcher, "launcher", "limactl",
		"Launcher to use (currently only limactl is supported)")
	rootCmd.PersistentFlags().IntVar(&parallel, "parallel", 1, "Parallel number for creating nodes")
	rootCmd.PersistentFlags().StringVar(&limaTemplate, "lima-template", "ubuntu-24.04",
		`Lima file or template,please use "limactl create --list-templates" to see what templates are available`)
	rootCmd.PersistentFlags().StringVar(&proxyMode, "proxy-mode", "ipvs", "Proxy mode (iptables or ipvs)")
	rootCmd.PersistentFlags().BoolVar(&updateSystem, "update-system", false,
		"Update system packages before installation")

	// Add subcommands
	rootCmd.AddCommand(upCmd)
	rootCmd.AddCommand(downCmd)
	rootCmd.AddCommand(addCmd)
	rootCmd.AddCommand(deleteCmd)
	rootCmd.AddCommand(listCmd)
	rootCmd.AddCommand(shellCmd)
	rootCmd.AddCommand(startCmd)
	rootCmd.AddCommand(stopCmd)
	rootCmd.AddCommand(versionCmd)
}
