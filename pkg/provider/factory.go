package provider

import (
	"fmt"

	"github.com/monshunter/ohmykube/pkg/provider/lima"
	"github.com/monshunter/ohmykube/pkg/provider/options"
)

// ProviderType represents the type of VM provider to use
type ProviderType string

const (
	// LimaProvider is the lima provider
	LimaProvider ProviderType = "lima"
	// Future cloud providers can be added here:
	// AliCloudProvider ProviderType = "alicloud"
	// GKEProvider ProviderType = "gke"
	// AWSProvider ProviderType = "aws"
	// TKEProvider ProviderType = "tke"
)

func (l ProviderType) String() string {
	return string(l)
}

func (l ProviderType) IsValid() bool {
	return l == LimaProvider
	// TODO: Add validation for future cloud providers
}

// NewProvider creates a new provider of the specified type
func NewProvider(providerType ProviderType, options *options.Options) (Provider, error) {
	switch providerType {
	case LimaProvider:
		return lima.NewLimaProvider(options)
	default:
		return nil, fmt.Errorf("unsupported provider type: %s, currently only 'lima' is supported", providerType)
	}
}
