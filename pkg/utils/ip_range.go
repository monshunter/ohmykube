package utils

import (
	"fmt"
	"net"
	"strings"
)

// NormalizeAndValidateIPv4Range validates and normalizes an IPv4 range.
// Accepts "startIP-endIP" or "startIP - endIP", and returns "startIP - endIP".
func NormalizeAndValidateIPv4Range(input string) (string, error) {
	input = strings.TrimSpace(input)
	parts := strings.SplitN(input, "-", 2)
	if len(parts) != 2 {
		return "", fmt.Errorf("invalid address range format %q: expected \"startIP-endIP\"", input)
	}

	startStr := strings.TrimSpace(parts[0])
	endStr := strings.TrimSpace(parts[1])

	startIP := net.ParseIP(startStr)
	if startIP == nil || startIP.To4() == nil {
		return "", fmt.Errorf("invalid start IP %q: must be a valid IPv4 address", startStr)
	}

	endIP := net.ParseIP(endStr)
	if endIP == nil || endIP.To4() == nil {
		return "", fmt.Errorf("invalid end IP %q: must be a valid IPv4 address", endStr)
	}

	s4 := startIP.To4()
	e4 := endIP.To4()
	if s4[0] != e4[0] || s4[1] != e4[1] || s4[2] != e4[2] {
		return "", fmt.Errorf("start IP %s and end IP %s must be in the same /24 subnet", startStr, endStr)
	}
	if s4[3] >= e4[3] {
		return "", fmt.Errorf("start IP %s must be strictly less than end IP %s", startStr, endStr)
	}

	return fmt.Sprintf("%s - %s", s4.String(), e4.String()), nil
}
