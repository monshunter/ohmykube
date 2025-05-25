package main

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/monshunter/ohmykube/pkg/cache"
)

func main() {
	fmt.Println("=== OhMyKube Image Cache System Demo ===")
	fmt.Println()

	// Create image cache manager with different strategies
	fmt.Println("1. Creating image cache managers with different strategies...")

	// Default auto strategy
	fmt.Println("   1.1. Auto strategy (detects best approach):")
	autoImageManager, err := cache.NewImageManager()
	if err != nil {
		log.Fatalf("Failed to create auto image manager: %v", err)
	}
	fmt.Println("   ✓ Auto strategy image manager created")

	// Local preferred strategy
	fmt.Println("   1.2. Local preferred strategy:")
	localConfig := cache.LocalPreferredImageManagementConfig()
	_, err = cache.NewImageManagerWithConfig(localConfig)
	if err != nil {
		log.Fatalf("Failed to create local image manager: %v", err)
	}
	fmt.Println("   ✓ Local preferred image manager created")

	// Controller only strategy
	fmt.Println("   1.3. Controller only strategy:")
	controllerConfig := cache.ControllerOnlyImageManagementConfig()
	_, err = cache.NewImageManagerWithConfig(controllerConfig)
	if err != nil {
		log.Fatalf("Failed to create controller image manager: %v", err)
	}
	fmt.Println("   ✓ Controller only image manager created")

	// Use the auto manager for the rest of the demo
	imageCacheManager := autoImageManager

	// Demonstrate tool detection
	fmt.Println("   1.4. Tool detection simulation:")
	toolDetector := cache.NewToolDetector()
	fmt.Printf("   Local docker available: %t\n", toolDetector.IsLocalToolAvailable("docker"))
	fmt.Printf("   Local podman available: %t\n", toolDetector.IsLocalToolAvailable("podman"))
	fmt.Printf("   Local nerdctl available: %t\n", toolDetector.IsLocalToolAvailable("nerdctl"))
	fmt.Printf("   Local helm available: %t\n", toolDetector.IsLocalToolAvailable("helm"))
	fmt.Printf("   Local kubeadm available: %t\n", toolDetector.IsLocalToolAvailable("kubeadm"))

	// Display cache statistics
	fmt.Println("2. Current cache statistics:")
	count, totalSize := imageCacheManager.GetCacheStats()
	fmt.Printf("   - Cached images: %d\n", count)
	fmt.Printf("   - Total size: %d bytes (%.2f MB)\n", totalSize, float64(totalSize)/(1024*1024))
	fmt.Println()

	// Demo image reference parsing
	fmt.Println("3. Image reference parsing examples:")
	testImages := []string{
		"nginx:latest",
		"quay.io/cilium/cilium:v1.14.0",
		"registry.k8s.io/kube-apiserver:v1.28.0",
		"docker.io/library/redis:7-alpine",
		"quay.io/cilium/cilium:v1.14.0@sha256:abcdef123456", // Digest-based reference (handled uniformly)
	}

	for _, img := range testImages {
		ref := cache.ParseImageReference(img, "amd64")
		fmt.Printf("   Original: %s\n", img)
		fmt.Printf("   Parsed:   Registry=%s, Project=%s, Image=%s, Tag=%s, Digest=%s\n",
			ref.Registry, ref.Project, ref.Image, ref.Tag, ref.Digest)
		fmt.Printf("   Cache Key: %s\n", ref.CacheKey())
		fmt.Printf("   Normalized: %s\n", ref.NormalizedName())
		fmt.Println()
	}

	// Demo image discovery (local tools only for demo)
	fmt.Println("4. Image discovery examples (using local tools):")

	// Example: Local discovery
	fmt.Println("   4.1. Local image discovery capabilities:")
	localDiscovery := cache.NewLocalImageDiscovery()

	// Check if local tools are available for discovery
	ctx := context.Background()
	fmt.Printf("   Local kubeadm available: %t\n", toolDetector.IsLocalToolAvailable("kubeadm"))
	fmt.Printf("   Local helm available: %t\n", toolDetector.IsLocalToolAvailable("helm"))
	fmt.Printf("   Local curl available: %t\n", toolDetector.IsLocalToolAvailable("curl"))

	// Example kubeadm source (would work if kubeadm is available locally)
	kubeadmSource := cache.ImageSource{
		Type:    "kubeadm",
		Version: "v1.28.0",
	}

	if toolDetector.IsLocalToolAvailable("kubeadm") {
		fmt.Println("   4.2. Kubeadm images for Kubernetes v1.28.0:")
		images, err := localDiscovery.GetRequiredImagesForArch(ctx, kubeadmSource, "amd64")
		if err != nil {
			fmt.Printf("   Error discovering kubeadm images: %v\n", err)
		} else {
			for i, img := range images {
				if i >= 3 { // Limit output for demo
					fmt.Printf("   ... and %d more images\n", len(images)-3)
					break
				}
				fmt.Printf("   - %s\n", img)
			}
		}
	} else {
		fmt.Println("   4.2. Kubeadm not available locally - would use controller node")
		fmt.Println("   Example images that would be discovered:")
		fmt.Println("   - registry.k8s.io/kube-apiserver:v1.28.0")
		fmt.Println("   - registry.k8s.io/kube-controller-manager:v1.28.0")
		fmt.Println("   - registry.k8s.io/kube-scheduler:v1.28.0")
	}
	fmt.Println()

	// Demo image management strategies
	fmt.Println("5. Image management strategy configuration:")

	// Default auto strategy
	defaultConfig := cache.DefaultImageManagementConfig()
	fmt.Printf("   Default strategy: %s\n", defaultConfig.Strategy.String())
	fmt.Printf("   Fallback order: ")
	for i, strategy := range defaultConfig.FallbackOrder {
		if i > 0 {
			fmt.Printf(", ")
		}
		fmt.Printf("%s", strategy.String())
	}
	fmt.Println()

	// Local preferred strategy
	localPrefConfig := cache.LocalPreferredImageManagementConfig()
	fmt.Printf("   Local preferred strategy: %s\n", localPrefConfig.Strategy.String())

	// Controller only strategy
	controllerOnlyConfig := cache.ControllerOnlyImageManagementConfig()
	fmt.Printf("   Controller only strategy: %s (forced: %t)\n", controllerOnlyConfig.Strategy.String(), controllerOnlyConfig.ForceStrategy)
	fmt.Println()

	// Demo cache cleanup
	fmt.Println("6. Cache cleanup simulation:")
	fmt.Println("   Cleaning up images older than 30 days...")
	err = imageCacheManager.CleanupOldImages(30 * 24 * time.Hour)
	if err != nil {
		fmt.Printf("   Error during cleanup: %v\n", err)
	} else {
		fmt.Println("   Cleanup completed successfully")
	}

	// Final cache statistics
	fmt.Println("7. Final cache statistics:")
	count, totalSize = imageCacheManager.GetCacheStats()
	fmt.Printf("   - Cached images: %d\n", count)
	fmt.Printf("   - Total size: %d bytes (%.2f MB)\n", totalSize, float64(totalSize)/(1024*1024))
	fmt.Println()

	fmt.Println("=== Demo completed ===")
	fmt.Println()
	fmt.Println("To use the image cache system in your cluster:")
	fmt.Println("1. Create image manager with desired strategy:")
	fmt.Println("   // Auto strategy (detects best approach)")
	fmt.Println("   imageManager, _ := cache.NewImageManager()")
	fmt.Println()
	fmt.Println("   // Local preferred strategy (uses local tools when available)")
	fmt.Println("   config := cache.LocalPreferredImageManagementConfig()")
	fmt.Println("   imageManager, _ := cache.NewImageManagerWithConfig(config)")
	fmt.Println()
	fmt.Println("   // Controller only strategy (always uses controller node)")
	fmt.Println("   config := cache.ControllerOnlyImageManagementConfig()")
	fmt.Println("   imageManager, _ := cache.NewImageManagerWithConfig(config)")
	fmt.Println()
	fmt.Println("2. The system will automatically:")
	fmt.Println("   - Detect available tools (docker, podman, nerdctl, helm, kubeadm)")
	fmt.Println("   - Choose optimal strategy based on tool availability")
	fmt.Println("   - Discover required images from various sources")
	fmt.Println("   - Cache images efficiently without additional compression")
	fmt.Println("   - Upload images to target nodes as needed")
	fmt.Println()
	fmt.Println("Benefits:")
	fmt.Println("- Flexible strategy selection (local vs controller vs auto)")
	fmt.Println("- Efficient tool usage (prefers local when available)")
	fmt.Println("- No additional compression (images already compressed)")
	fmt.Println("- Architecture-aware (handles amd64/arm64 automatically)")
	fmt.Println("- Fallback mechanisms (graceful degradation)")
	fmt.Println("- Better user experience (no mandatory local tool installation)")
}
