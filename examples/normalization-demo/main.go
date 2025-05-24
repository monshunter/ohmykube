package main

import (
	"fmt"
	"log"
	"os"
	"path/filepath"

	"github.com/monshunter/ohmykube/pkg/initializer/cache"
)

func main() {
	fmt.Println("OhMyKube File Format Normalization Demo")
	fmt.Println("======================================")

	// Create a temporary directory for demonstration
	tempDir, err := os.MkdirTemp("", "normalization-demo")
	if err != nil {
		log.Fatalf("Failed to create temp directory: %v", err)
	}
	defer os.RemoveAll(tempDir)

	fmt.Printf("Working directory: %s\n\n", tempDir)

	// Create compressor
	compressor, err := cache.NewCompressor()
	if err != nil {
		log.Fatalf("Failed to create compressor: %v", err)
	}
	defer compressor.Close()

	// Demo 1: Binary file normalization
	fmt.Println("Demo 1: Binary File Normalization")
	fmt.Println("---------------------------------")
	
	// Create a sample binary file
	binaryPath := filepath.Join(tempDir, "kubectl")
	binaryContent := []byte("#!/bin/bash\n# This is a mock kubectl binary\necho 'kubectl version'\n")
	if err := os.WriteFile(binaryPath, binaryContent, 0755); err != nil {
		log.Fatalf("Failed to create binary file: %v", err)
	}
	
	fmt.Printf("Created binary file: %s (%d bytes)\n", binaryPath, len(binaryContent))
	
	// Normalize to tar.zst
	normalizedPath := filepath.Join(tempDir, "kubectl.tar.zst")
	if err := compressor.NormalizeToTarZst(binaryPath, normalizedPath, cache.FileFormatBinary); err != nil {
		log.Fatalf("Failed to normalize binary: %v", err)
	}
	
	// Check the result
	normalizedInfo, _ := os.Stat(normalizedPath)
	fmt.Printf("Normalized to: %s (%d bytes)\n", normalizedPath, normalizedInfo.Size())
	
	// Extract and verify
	extractDir := filepath.Join(tempDir, "extract1")
	if err := compressor.DecompressFile(normalizedPath, extractDir); err != nil {
		log.Fatalf("Failed to extract: %v", err)
	}
	
	extractedFile := filepath.Join(extractDir, "kubectl")
	extractedContent, _ := os.ReadFile(extractedFile)
	fmt.Printf("Extracted file: %s (%d bytes)\n", extractedFile, len(extractedContent))
	fmt.Printf("Content matches: %t\n\n", string(extractedContent) == string(binaryContent))

	// Demo 2: Show supported formats
	fmt.Println("Demo 2: Supported File Formats")
	fmt.Println("------------------------------")
	
	formats := []struct {
		format      cache.FileFormat
		description string
		process     string
	}{
		{cache.FileFormatBinary, "Binary files", "Package into .tar → Compress with zstd"},
		{cache.FileFormatTar, ".tar files", "Direct zstd compression"},
		{cache.FileFormatTarGz, ".tar.gz files", "Decompress to .tar → Compress with zstd"},
		{cache.FileFormatTgz, ".tgz files", "Decompress to .tar → Compress with zstd"},
		{cache.FileFormatTarBz2, ".tar.bz2 files", "Decompress to .tar → Compress with zstd"},
		{cache.FileFormatTarXz, ".tar.xz files", "Decompress to .tar → Compress with zstd"},
	}
	
	for _, f := range formats {
		fmt.Printf("%-15s: %s\n", f.description, f.process)
	}
	
	fmt.Println("\nAll formats are normalized to .tar.zst for:")
	fmt.Println("✓ Consistent storage format")
	fmt.Println("✓ Optimal compression ratio")
	fmt.Println("✓ Fast decompression speed")
	fmt.Println("✓ Simplified handling")

	// Demo 3: Package definitions with formats
	fmt.Println("\nDemo 3: Package Definitions with File Formats")
	fmt.Println("---------------------------------------------")
	
	definitions := cache.GetPackageDefinitions()
	
	examples := []struct {
		name   string
		format cache.FileFormat
	}{
		{"containerd", cache.FileFormatTarGz},
		{"kubectl", cache.FileFormatBinary},
		{"helm", cache.FileFormatTarGz},
		{"cni-plugins", cache.FileFormatTgz},
		{"runc", cache.FileFormatBinary},
	}
	
	for _, example := range examples {
		if def, exists := definitions[example.name]; exists {
			fmt.Printf("%-12s: %s → %s.tar.zst\n", 
				example.name, 
				def.GetFilename("latest", "amd64"),
				example.name+"-latest-amd64")
		}
	}

	fmt.Println("\nDemo completed!")
	fmt.Println("\nThe normalization process ensures that:")
	fmt.Println("• All packages are stored in a consistent .tar.zst format")
	fmt.Println("• Optimal compression and decompression performance")
	fmt.Println("• Simplified cache management and distribution")
	fmt.Println("• Better space efficiency compared to mixed formats")
}
