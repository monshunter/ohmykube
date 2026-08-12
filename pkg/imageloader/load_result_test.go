package imageloader

import (
	"errors"
	"strings"
	"testing"
)

func TestValidateLoadResultRequiresEveryTargetNode(t *testing.T) {
	t.Parallel()

	if err := validateLoadResult(3, 3, nil); err != nil {
		t.Fatalf("all nodes should succeed: %v", err)
	}

	err := validateLoadResult(2, 3, errors.New("worker-2 import failed"))
	if err == nil {
		t.Fatal("partial node success must fail")
	}
	for _, want := range []string{"2/3", "worker-2 import failed"} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("partial error %q missing %q", err, want)
		}
	}
}

func TestValidateLoadResultRejectsEmptyTargets(t *testing.T) {
	t.Parallel()

	if err := validateLoadResult(0, 0, nil); err == nil {
		t.Fatal("empty target set must fail")
	}
}
