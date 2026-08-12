package kube

import (
	"os"
	"path/filepath"
	"testing"
)

func TestWriteKubeconfigUsesOwnerOnlyPermissions(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "kubeconfig")
	if err := writeKubeconfig(path, []byte("apiVersion: v1\n")); err != nil {
		t.Fatalf("write kubeconfig: %v", err)
	}

	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat kubeconfig: %v", err)
	}
	if got, want := info.Mode().Perm(), os.FileMode(0600); got != want {
		t.Fatalf("kubeconfig permissions = %04o, want %04o", got, want)
	}
}

func TestWriteKubeconfigTightensExistingPermissions(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "kubeconfig")
	if err := os.WriteFile(path, []byte("old"), 0644); err != nil {
		t.Fatalf("seed kubeconfig: %v", err)
	}
	if err := writeKubeconfig(path, []byte("new")); err != nil {
		t.Fatalf("rewrite kubeconfig: %v", err)
	}

	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat kubeconfig: %v", err)
	}
	if got, want := info.Mode().Perm(), os.FileMode(0600); got != want {
		t.Fatalf("kubeconfig permissions = %04o, want %04o", got, want)
	}
}
