package lb

import (
	"testing"
)

func TestNewMetalLBInstaller_WithAddressRange(t *testing.T) {
	installer := NewMetalLBInstaller(nil, "node1", "192.168.64.100", "10.0.0.100 - 10.0.0.110")
	if installer.addressRange != "10.0.0.100 - 10.0.0.110" {
		t.Errorf("addressRange = %q, want %q", installer.addressRange, "10.0.0.100 - 10.0.0.110")
	}
}

func TestNewMetalLBInstaller_EmptyAddressRange(t *testing.T) {
	installer := NewMetalLBInstaller(nil, "node1", "192.168.64.100", "")
	if installer.addressRange != "" {
		t.Errorf("addressRange = %q, want empty", installer.addressRange)
	}
}

func TestGetMetalLBAddressRange_UserSpecified(t *testing.T) {
	installer := &MetalLBInstaller{
		controllerIP: "192.168.64.100",
		addressRange: "10.0.0.100 - 10.0.0.110",
	}
	got, err := installer.getMetalLBAddressRange()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "10.0.0.100 - 10.0.0.110" {
		t.Errorf("got %q, want %q", got, "10.0.0.100 - 10.0.0.110")
	}
}

func TestGetMetalLBAddressRange_AutoDerive(t *testing.T) {
	installer := &MetalLBInstaller{
		controllerIP: "192.168.64.100",
		addressRange: "",
	}
	got, err := installer.getMetalLBAddressRange()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "192.168.64.200 - 192.168.64.250" {
		t.Errorf("got %q, want %q", got, "192.168.64.200 - 192.168.64.250")
	}
}

func TestGetMetalLBAddressRange_InvalidControllerIP(t *testing.T) {
	installer := &MetalLBInstaller{
		controllerIP: "not-an-ip",
		addressRange: "",
	}
	_, err := installer.getMetalLBAddressRange()
	if err == nil {
		t.Error("expected error for invalid controllerIP")
	}
}

func TestGetMetalLBAddressRange_InvalidUserRange(t *testing.T) {
	installer := &MetalLBInstaller{
		controllerIP: "192.168.64.100",
		addressRange: "invalid-range",
	}
	_, err := installer.getMetalLBAddressRange()
	if err == nil {
		t.Error("expected error for invalid user-specified range")
	}
}

func TestGetAllocatedRange(t *testing.T) {
	installer := &MetalLBInstaller{}
	if got := installer.GetAllocatedRange(); got != "" {
		t.Errorf("initial GetAllocatedRange() = %q, want empty", got)
	}
	installer.allocatedRange = "192.168.64.200 - 192.168.64.250"
	if got := installer.GetAllocatedRange(); got != "192.168.64.200 - 192.168.64.250" {
		t.Errorf("GetAllocatedRange() = %q, want %q", got, "192.168.64.200 - 192.168.64.250")
	}
}
