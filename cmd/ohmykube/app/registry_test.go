package app

import (
	"errors"
	"strings"
	"testing"
)

func TestNormalizeRegistryEndpoint(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name      string
		input     string
		want      string
		wantError bool
	}{
		{name: "IPv4", input: "192.168.10.86:5052", want: "192.168.10.86:5052"},
		{name: "DNS", input: "Registry.Local:5000", want: "registry.local:5000"},
		{name: "scheme", input: "http://registry.local:5000", wantError: true},
		{name: "path", input: "registry.local:5000/project", wantError: true},
		{name: "userinfo", input: "user@registry.local:5000", wantError: true},
		{name: "missing port", input: "registry.local", wantError: true},
		{name: "invalid port", input: "registry.local:70000", wantError: true},
		{name: "shell metacharacter", input: "registry;touch-pwned:5000", wantError: true},
	} {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got, err := normalizeRegistryEndpoint(tc.input)
			if tc.wantError {
				if err == nil {
					t.Fatalf("normalizeRegistryEndpoint(%q) succeeded, want error", tc.input)
				}
				return
			}
			if err != nil {
				t.Fatalf("normalizeRegistryEndpoint(%q): %v", tc.input, err)
			}
			if got != tc.want {
				t.Fatalf("normalizeRegistryEndpoint(%q) = %q, want %q", tc.input, got, tc.want)
			}
		})
	}
}

func TestRegistryConfigureCommandUsesEncodedDeterministicConfig(t *testing.T) {
	t.Parallel()

	command, err := registryConfigureCommand("192.168.10.86:5052", true)
	if err != nil {
		t.Fatalf("registryConfigureCommand: %v", err)
	}
	for _, want := range []string{
		"/etc/containerd/certs.d/192.168.10.86:5052/hosts.toml",
		"base64 --decode",
		"sudo systemctl restart containerd",
	} {
		if !strings.Contains(command, want) {
			t.Fatalf("configure command missing %q: %s", want, command)
		}
	}
	if strings.Contains(command, `[host."http://192.168.10.86:5052"]`) {
		t.Fatal("raw TOML must be encoded instead of interpolated into the shell command")
	}
}

func TestRegistryRemoveCommandTargetsExactEndpoint(t *testing.T) {
	t.Parallel()

	command, err := registryRemoveCommand("registry.local:5000")
	if err != nil {
		t.Fatalf("registryRemoveCommand: %v", err)
	}
	for _, want := range []string{
		"sudo rm -rf /etc/containerd/certs.d/registry.local:5000",
		"sudo systemctl restart containerd",
	} {
		if !strings.Contains(command, want) {
			t.Fatalf("remove command missing %q: %s", want, command)
		}
	}
}

func TestValidateRegistryNodeResultsFailsClosed(t *testing.T) {
	t.Parallel()

	if err := validateRegistryNodeResults(3, 3, nil); err != nil {
		t.Fatalf("all nodes should succeed: %v", err)
	}
	err := validateRegistryNodeResults(2, 3, errors.New("worker-2 failed"))
	if err == nil {
		t.Fatal("partial node success must fail")
	}
	for _, want := range []string{"2/3", "worker-2 failed"} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("partial error %q missing %q", err, want)
		}
	}
	if err := validateRegistryNodeResults(0, 0, nil); err == nil {
		t.Fatal("empty running node set must fail")
	}
}
