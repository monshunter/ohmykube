package config

import (
	"strings"
	"sync"
	"testing"

	"gopkg.in/yaml.v3"
)

func TestNetworkingConfig_LBAddressRange_YAMLRoundTrip(t *testing.T) {
	input := `proxyMode: iptables
cni: flannel
loadbalancer: metallb
lbAddressRange: "192.168.64.200 - 192.168.64.210"
`
	var nc NetworkingConfig
	if err := yaml.Unmarshal([]byte(input), &nc); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if nc.LBAddressRange != "192.168.64.200 - 192.168.64.210" {
		t.Errorf("LBAddressRange = %q, want %q", nc.LBAddressRange, "192.168.64.200 - 192.168.64.210")
	}

	out, err := yaml.Marshal(&nc)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var nc2 NetworkingConfig
	if err := yaml.Unmarshal(out, &nc2); err != nil {
		t.Fatalf("re-unmarshal: %v", err)
	}
	if nc2.LBAddressRange != nc.LBAddressRange {
		t.Errorf("round-trip: got %q, want %q", nc2.LBAddressRange, nc.LBAddressRange)
	}
}

func TestNetworkingConfig_LBAddressRange_OmitEmpty(t *testing.T) {
	nc := NetworkingConfig{
		ProxyMode:    "iptables",
		CNI:          "flannel",
		LoadBalancer: "metallb",
	}
	out, err := yaml.Marshal(&nc)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var nc2 NetworkingConfig
	if err := yaml.Unmarshal(out, &nc2); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if nc2.LBAddressRange != "" {
		t.Errorf("expected empty LBAddressRange when omitted, got %q", nc2.LBAddressRange)
	}
}

func TestCluster_GetSetLBAddressRange(t *testing.T) {
	cfg := &Config{
		Name:     "test",
		Provider: "lima",
		LB:       "metallb",
	}
	cls := NewCluster(cfg)

	// Initially empty
	if got := cls.GetLBAddressRange(); got != "" {
		t.Errorf("initial GetLBAddressRange() = %q, want empty", got)
	}

	// Set and get
	cls.SetLBAddressRange("192.168.64.200 - 192.168.64.210")
	if got := cls.GetLBAddressRange(); got != "192.168.64.200 - 192.168.64.210" {
		t.Errorf("GetLBAddressRange() = %q, want %q", got, "192.168.64.200 - 192.168.64.210")
	}
}

func TestGenerateConfigTemplate_ContainsLBAddressRange(t *testing.T) {
	tmpl := GenerateConfigTemplate("test", "lima", "ubuntu-24.04", false)
	if !strings.Contains(tmpl, "lbAddressRange") {
		t.Error("config template should contain lbAddressRange example")
	}
}

func TestNewCluster_MapsLBAddressRange(t *testing.T) {
	cfg := NewConfig("test", 1, "iptables", Resource{CPU: 2, Memory: 4, Disk: 20}, Resource{CPU: 1, Memory: 2, Disk: 10})
	cfg.LBAddressRange = "192.168.64.200 - 192.168.64.210"
	cls := NewCluster(cfg)
	if got := cls.GetLBAddressRange(); got != "192.168.64.200 - 192.168.64.210" {
		t.Errorf("NewCluster mapping: GetLBAddressRange() = %q, want %q", got, "192.168.64.200 - 192.168.64.210")
	}
}

func TestConfig_SetLBAddressRange(t *testing.T) {
	cfg := NewConfig("test", 1, "iptables", Resource{CPU: 2, Memory: 4, Disk: 20}, Resource{CPU: 1, Memory: 2, Disk: 10})
	if cfg.LBAddressRange != "" {
		t.Errorf("initial LBAddressRange = %q, want empty", cfg.LBAddressRange)
	}
	cfg.SetLBAddressRange("192.168.64.200 - 192.168.64.210")
	if cfg.LBAddressRange != "192.168.64.200 - 192.168.64.210" {
		t.Errorf("LBAddressRange = %q, want %q", cfg.LBAddressRange, "192.168.64.200 - 192.168.64.210")
	}
}

func TestCluster_GetSetLBAddressRange_ThreadSafe(t *testing.T) {
	cfg := &Config{
		Name:     "test",
		Provider: "lima",
		LB:       "metallb",
	}
	cls := NewCluster(cfg)

	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(2)
		go func() {
			defer wg.Done()
			cls.SetLBAddressRange("192.168.64.200 - 192.168.64.210")
		}()
		go func() {
			defer wg.Done()
			_ = cls.GetLBAddressRange()
		}()
	}
	wg.Wait()
}
