package app

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/monshunter/ohmykube/pkg/config"
	"github.com/monshunter/ohmykube/pkg/log"
	"github.com/spf13/cobra"
)

var (
	outputPath string
)

var configCmd = &cobra.Command{
	Use:   "config",
	Short: "Generate default cluster configuration file",
	Long: `Generate a default cluster configuration file that can be used with 'ohmykube up -f'.
The generated file contains all cluster settings in YAML format with detailed comments.`,
	Args: cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		// Set default template if empty
		if template == "" {
			template = "ubuntu-24.04"
		}

		// Generate configuration file content with comments
		content := config.GenerateConfigTemplate(clusterName, provider, template, updateSystem)

		// Determine output path
		if outputPath == "" {
			outputPath = fmt.Sprintf("%s.yaml", clusterName)
		}

		// Ensure directory exists
		dir := filepath.Dir(outputPath)
		if dir != "." {
			if err := os.MkdirAll(dir, 0755); err != nil {
				return fmt.Errorf("failed to create directory %s: %w", dir, err)
			}
		}

		// Write to file
		if err := os.WriteFile(outputPath, []byte(content), 0644); err != nil {
			return fmt.Errorf("failed to write configuration file: %w", err)
		}

		log.Infof("✅ Configuration file generated: %s", outputPath)
		return nil
	},
}

func init() {
	// Add output flag
	configCmd.Flags().StringVarP(&outputPath, "output", "o", "", "Output file path (default: $clusterName.yaml)")
}
