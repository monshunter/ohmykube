package utils // FormatFileSize formats a file size in bytes to a human-readable string

import (
	"fmt"
	"os/exec"
	"runtime"
	"strings"

	"github.com/monshunter/ohmykube/pkg/log"
	"gopkg.in/yaml.v3"
)

// FormatSize formats a size in bytes to a human-readable string
func FormatSize(bytes int64) string {
	const unit = 1024
	if bytes < unit {
		return fmt.Sprintf("%d B", bytes)
	}
	div, exp := int64(unit), 0
	for n := bytes / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(bytes)/float64(div), "KMGTPE"[exp])
}

// ExtractImagesWithParser uses a real YAML parser to extract images (exported for testing)
func ExtractImagesWithParser(yamlContent string) ([]string, error) {
	// Check if content is empty
	trimmedContent := strings.TrimSpace(yamlContent)
	if trimmedContent == "" {
		return nil, fmt.Errorf("received empty YAML content")
	}

	// Split content by potential YAML document separators
	parts := strings.Split(yamlContent, "---")
	var allImages []string
	validDocCount := 0

	// Process each potential YAML document directly
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue // Skip empty parts
		}

		// Try to parse this part as YAML
		var docData map[string]any
		if err := yaml.Unmarshal([]byte(part), &docData); err != nil {
			// Not valid YAML (probably log messages), skip silently
			continue
		}

		// This is valid YAML, extract images from it
		validDocCount++
		findImagesRecursive(docData, &allImages)
	}

	if validDocCount == 0 {
		return nil, fmt.Errorf("no valid YAML documents found in content")
	}

	// Remove duplicates
	imageMap := make(map[string]bool)
	for _, img := range allImages {
		imageMap[img] = true
	}
	result := []string{}
	for img := range imageMap {
		result = append(result, img)
	}

	log.Infof("Successfully extracted %d unique images from %d valid YAML documents", len(result), validDocCount)
	return result, nil
}

// findImagesRecursive recursively searches for the "image" key in any parsed map/slice structure
func findImagesRecursive(data any, images *[]string) {
	// Use type switch to determine the specific type of the current data
	switch v := data.(type) {
	// Case 1: The data is a map (YAML object)
	case map[string]any:
		for key, value := range v {
			// If the key name is "image"
			if key == "image" {
				// And its value is a string
				if imageStr, ok := value.(string); ok {
					*images = append(*images, imageStr)
				}
			} else {
				// Otherwise, recursively search the value of this key
				findImagesRecursive(value, images)
			}
		}
	// Case 2: The data is a slice (YAML sequence/list)
	case []any:
		for _, item := range v {
			// Recursively search each element in the list
			findImagesRecursive(item, images)
		}
	}
}

// IsProcessRunning checks if a process with the given name is running
// This function works cross-platform (macOS, Linux, Windows)
func IsProcessRunning(processName string) (bool, error) {
	var cmd *exec.Cmd

	switch runtime.GOOS {
	case "windows":
		// Use tasklist on Windows
		cmd = exec.Command("tasklist", "/FI", fmt.Sprintf("IMAGENAME eq %s*", processName))
	case "darwin", "linux":
		// Use pgrep on macOS and Linux for more reliable detection
		cmd = exec.Command("pgrep", "-f", processName)
	default:
		return false, fmt.Errorf("unsupported operating system: %s", runtime.GOOS)
	}

	output, err := cmd.Output()
	if err != nil {
		// pgrep returns exit code 1 when no processes are found, which is normal
		if exitError, ok := err.(*exec.ExitError); ok {
			if exitError.ExitCode() == 1 {
				return false, nil // Process not found, but no error
			}
		}
		return false, fmt.Errorf("failed to check process %s: %w", processName, err)
	}

	outputStr := strings.TrimSpace(string(output))

	switch runtime.GOOS {
	case "windows":
		// On Windows, tasklist returns the process info if found
		return strings.Contains(outputStr, processName), nil
	case "darwin", "linux":
		// On Unix systems, pgrep returns process IDs if found
		return outputStr != "", nil
	default:
		return false, fmt.Errorf("unsupported operating system: %s", runtime.GOOS)
	}
}
