package app

import (
	"os"

	"github.com/monshunter/ohmykube/pkg/log"
	"github.com/spf13/cobra"
)

var (
	verbose bool
	quiet   bool
)

var rootCmd = &cobra.Command{
	Use:   "ohmykube",
	Short: "OhMyKube - Tool for quickly setting up real Kubernetes clusters locally",
	Long: `OhMyKube is a tool based on Lima and kubeadm, designed to quickly set up
real Kubernetes clusters on developer computers using virtual machines,
including Cilium(CNI), Rook(CSI), and MetalLB(LB).`,
	// If preparation work is needed before execution, PersistentPreRun can be added
	PersistentPreRun: func(cmd *cobra.Command, args []string) {
		// Set log modes based on flags
		if verbose {
			log.SetVerbose(true)
		}
		if quiet {
			log.SetQuiet(true)
		}
	},
}

func init() {
	// Add global flags
	rootCmd.PersistentFlags().BoolVarP(&verbose, "verbose", "v", false, "Enable verbose output")
	rootCmd.PersistentFlags().BoolVarP(&quiet, "quiet", "q", false, "Enable quiet mode (minimal output)")
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
	provider             string
	template             string
	proxyMode            string
	updateSystem         bool
	parallel             int
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
	rootCmd.PersistentFlags().IntVar(&parallel, "parallel", 1, "Parallel number for creating nodes")
	rootCmd.PersistentFlags().StringVar(&provider, "provider", "lima", "Provider to use (currently only lima is supported)")
	rootCmd.PersistentFlags().StringVar(&template, "template", "",
		`template or file, for example: "ubuntu-24.04" or "/path/to/file", default "ubuntu-24.04" in lima`)
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
