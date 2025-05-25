package main

import (
	"fmt"
	"log"

	"github.com/monshunter/ohmykube/pkg/cache"
)

// MockSSHRunner implements a simple SSH runner for demonstration
type MockSSHRunner struct{}

func (m *MockSSHRunner) RunSSHCommand(nodeName string, command string) (string, error) {
	fmt.Printf("SSH Command to %s: %s\n", nodeName, command)
	return "success", nil
}

func main() {
	fmt.Println("OhMyKube Package Cache Demo")
	fmt.Println("===========================")

	// Create cache manager
	manager, err := cache.NewPackageManager()
	if err != nil {
		log.Fatalf("Failed to create cache manager: %v", err)
	}
	defer manager.Close()

	// Display cache statistics
	totalPackages, totalCompressed, totalOriginal := manager.GetCacheStats()
	fmt.Printf("Cache Statistics:\n")
	fmt.Printf("  Total packages: %d\n", totalPackages)
	fmt.Printf("  Total compressed size: %d bytes\n", totalCompressed)
	fmt.Printf("  Total original size: %d bytes\n", totalOriginal)

	if totalPackages > 0 {
		compressionRatio := float64(totalCompressed) / float64(totalOriginal) * 100
		fmt.Printf("  Compression ratio: %.1f%%\n", compressionRatio)
	}

	fmt.Println()

	// List cached packages
	packages := manager.ListCachedPackages()
	if len(packages) > 0 {
		fmt.Println("Cached packages:")
		for _, pkg := range packages {
			fmt.Printf("  - %s-%s-%s (cached: %s, size: %d bytes)\n",
				pkg.Name, pkg.Version, pkg.Architecture,
				pkg.CachedAt.Format("2006-01-02 15:04:05"),
				pkg.CompressedSize)
		}
	} else {
		fmt.Println("No packages cached yet.")
	}

	fmt.Println()

	// Demonstrate package definitions
	fmt.Println("Available package definitions:")
	definitions := cache.GetPackageDefinitions()
	for name, def := range definitions {
		fmt.Printf("  - %s (%s)\n", name, def.Type)
		fmt.Printf("    Example URL: %s\n", def.GetURL("latest", "amd64"))
	}

	fmt.Println()

	// Demonstrate cache functionality (without actually downloading)
	fmt.Println("Demo: Ensuring packages are available (mock mode)")

	// Example packages to demonstrate
	demoPackages := []struct {
		name    string
		version string
		arch    string
	}{
		{"containerd", "2.1.0", "amd64"},
		{"kubectl", "v1.33.1", "amd64"},
		{"helm", "v3.18.0", "amd64"},
	}

	for _, pkg := range demoPackages {
		fmt.Printf("Checking package: %s-%s-%s\n", pkg.name, pkg.version, pkg.arch)

		if manager.IsPackageCached(pkg.name, pkg.version, pkg.arch) {
			fmt.Printf("  ✓ Package is already cached\n")

			localPath, err := manager.GetLocalPackagePath(pkg.name, pkg.version, pkg.arch)
			if err == nil {
				fmt.Printf("  Local path: %s\n", localPath)
			}
		} else {
			fmt.Printf("  ⚠ Package not cached (would download in real scenario)\n")

			// In a real scenario, this would download and cache the package:
			// err := manager.EnsurePackage(ctx, pkg.name, pkg.version, pkg.arch, sshRunner, nodeName)
			// if err != nil {
			//     fmt.Printf("  ✗ Failed to ensure package: %v\n", err)
			// } else {
			//     fmt.Printf("  ✓ Package cached and uploaded successfully\n")
			// }
		}
		fmt.Println()
	}

	fmt.Println("Demo completed!")
	fmt.Println()
	fmt.Println("To use the cache system in your code:")
	fmt.Println("1. Create a cache manager: manager, err := cache.NewManager()")
	fmt.Println("2. Ensure packages: manager.EnsurePackage(ctx, name, version, arch, sshRunner, nodeName)")
	fmt.Println("3. Check cache status: manager.IsPackageCached(name, version, arch)")
	fmt.Println("4. Get local paths: manager.GetLocalPackagePath(name, version, arch)")
	fmt.Println()
	fmt.Println("The cache system automatically:")
	fmt.Println("- Downloads packages from official sources")
	fmt.Println("- Compresses them using zstd for efficient storage")
	fmt.Println("- Uploads them to target nodes via SSH")
	fmt.Println("- Maintains an index for fast lookups")
	fmt.Println("- Supports cleanup of old packages")
}
