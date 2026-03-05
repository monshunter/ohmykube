package app

import (
	"testing"

	"github.com/monshunter/ohmykube/pkg/config"
)

func TestNormalizeAndValidateLBAddressRange(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    string
		wantErr bool
	}{
		{"compact format", "192.168.64.200-192.168.64.210", "192.168.64.200 - 192.168.64.210", false},
		{"spaced format", "192.168.64.200 - 192.168.64.210", "192.168.64.200 - 192.168.64.210", false},
		{"extra spaces", "  192.168.64.200  -  192.168.64.210  ", "192.168.64.200 - 192.168.64.210", false},
		{"invalid start IP", "999.999.999.999-192.168.64.210", "", true},
		{"invalid end IP", "192.168.64.200-notanip", "", true},
		{"start >= end", "192.168.64.210-192.168.64.200", "", true},
		{"same IP", "192.168.64.200-192.168.64.200", "", true},
		{"different /24 subnet", "192.168.64.200-192.168.65.210", "", true},
		{"no separator", "192.168.64.200", "", true},
		{"IPv6", "::1-::2", "", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := normalizeAndValidateLBAddressRange(tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if got != tt.want {
				t.Errorf("got %q, want %q", got, tt.want)
			}
		})
	}
}

func TestUpCmd_LBRangeWithNonMetalLB_ReturnsError(t *testing.T) {
	// Save and restore globals
	origLB, origLBRange := lb, lbAddressRange
	defer func() { lb, lbAddressRange = origLB, origLBRange }()

	lb = "none"
	lbAddressRange = "192.168.64.200-192.168.64.210"

	err := validateLBRangeConstraint()
	if err == nil {
		t.Error("expected error when --lb-range used with non-metallb lb, got nil")
	}
}

func TestUpCmd_LBRangeWithMetalLB_NoError(t *testing.T) {
	origLB, origLBRange := lb, lbAddressRange
	defer func() { lb, lbAddressRange = origLB, origLBRange }()

	lb = "metallb"
	lbAddressRange = "192.168.64.200-192.168.64.210"

	err := validateLBRangeConstraint()
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestUpCmd_LBRangeEmpty_NoError(t *testing.T) {
	origLB, origLBRange := lb, lbAddressRange
	defer func() { lb, lbAddressRange = origLB, origLBRange }()

	lb = "none"
	lbAddressRange = ""

	err := validateLBRangeConstraint()
	if err != nil {
		t.Errorf("unexpected error when lb-range is empty: %v", err)
	}
}

func TestPrepareLBAddressRange_NewCluster(t *testing.T) {
	normalized, err := prepareLBAddressRange("192.168.64.200-192.168.64.210", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if normalized != "192.168.64.200 - 192.168.64.210" {
		t.Errorf("got %q, want %q", normalized, "192.168.64.200 - 192.168.64.210")
	}
}

func TestPrepareLBAddressRange_NewCluster_Invalid(t *testing.T) {
	_, err := prepareLBAddressRange("invalid", nil)
	if err == nil {
		t.Error("expected error for invalid range")
	}
}

func TestPrepareLBAddressRange_NewCluster_Empty(t *testing.T) {
	normalized, err := prepareLBAddressRange("", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if normalized != "" {
		t.Errorf("got %q, want empty", normalized)
	}
}

func TestPrepareLBAddressRange_Resume_PersistedTakesPriority(t *testing.T) {
	cfg := &config.Config{Name: "test", Provider: "lima", LB: "metallb"}
	cls := config.NewCluster(cfg)
	cls.SetLBAddressRange("10.0.0.100 - 10.0.0.110")

	// CLI provides different range — persisted value wins
	got, err := prepareLBAddressRange("10.0.0.200-10.0.0.210", cls)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "10.0.0.100 - 10.0.0.110" {
		t.Errorf("got %q, want persisted %q", got, "10.0.0.100 - 10.0.0.110")
	}
}

func TestPrepareLBAddressRange_Resume_NoPersistedUseCLI(t *testing.T) {
	cfg := &config.Config{Name: "test", Provider: "lima", LB: "metallb"}
	cls := config.NewCluster(cfg)
	// No persisted value

	got, err := prepareLBAddressRange("10.0.0.200-10.0.0.210", cls)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "10.0.0.200 - 10.0.0.210" {
		t.Errorf("got %q, want %q", got, "10.0.0.200 - 10.0.0.210")
	}
}

func TestUpCmd_LBRangeFlag_Registered(t *testing.T) {
	f := upCmd.Flags().Lookup("lb-range")
	if f == nil {
		t.Fatal("--lb-range flag is not registered on upCmd")
	}
	if f.DefValue != "" {
		t.Errorf("default value = %q, want empty", f.DefValue)
	}
}

func TestEffectiveLBForRangeValidation_UsesClusterLBWhenFlagEmpty(t *testing.T) {
	cfg := &config.Config{Name: "test", Provider: "lima", LB: "metallb"}
	cls := config.NewCluster(cfg)

	got := effectiveLBForRangeValidation("", cls)
	if got != "metallb" {
		t.Errorf("got %q, want %q", got, "metallb")
	}
}

func TestValidateLBRangeConstraint_EffectiveLBFromCluster_NoError(t *testing.T) {
	cfg := &config.Config{Name: "test", Provider: "lima", LB: "metallb"}
	cls := config.NewCluster(cfg)

	effectiveLB := effectiveLBForRangeValidation("", cls)
	err := validateLBRangeConstraintWith(effectiveLB, "192.168.64.200-192.168.64.210", false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestValidateLBRangeConstraint_EffectiveLBNone_ReturnsError(t *testing.T) {
	cfg := &config.Config{Name: "test", Provider: "lima", LB: "none"}
	cls := config.NewCluster(cfg)

	effectiveLB := effectiveLBForRangeValidation("", cls)
	err := validateLBRangeConstraintWith(effectiveLB, "192.168.64.200-192.168.64.210", false)
	if err == nil {
		t.Fatal("expected error when effective lb is not metallb")
	}
}

func TestValidateLBRangeConstraint_ExplicitMetalLBWithoutRange_ReturnsError(t *testing.T) {
	err := validateLBRangeConstraintWith("metallb", "", true)
	if err == nil {
		t.Fatal("expected error when --lb metallb is specified without --lb-range")
	}
}

func TestValidateLBRangeConstraint_ImplicitMetalLBWithoutRange_NoError(t *testing.T) {
	err := validateLBRangeConstraintWith("metallb", "", false)
	if err != nil {
		t.Fatalf("unexpected error for implicit metallb without --lb-range: %v", err)
	}
}
