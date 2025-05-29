package cache

import (
	"testing"
)

func TestImageManagerSingleton(t *testing.T) {
	// Test that GetImageManager returns the same instance
	t.Run("GetImageManager returns singleton", func(t *testing.T) {
		manager1, err1 := GetImageManager()
		if err1 != nil {
			t.Fatalf("Failed to get first ImageManager: %v", err1)
		}

		manager2, err2 := GetImageManager()
		if err2 != nil {
			t.Fatalf("Failed to get second ImageManager: %v", err2)
		}

		if manager1 != manager2 {
			t.Error("GetImageManager should return the same instance (singleton)")
		}
	})

	// Test that NewImageManager creates new instances (not singleton)
	t.Run("NewImageManager creates new instances", func(t *testing.T) {
		manager1, err1 := NewImageManager()
		if err1 != nil {
			t.Fatalf("Failed to create first ImageManager: %v", err1)
		}

		manager2, err2 := NewImageManager()
		if err2 != nil {
			t.Fatalf("Failed to create second ImageManager: %v", err2)
		}

		if manager1 == manager2 {
			t.Error("NewImageManager should create new instances, not return singletons")
		}
	})

	// Test that singleton and new instances are different
	t.Run("Singleton vs new instances", func(t *testing.T) {
		singleton, err1 := GetImageManager()
		if err1 != nil {
			t.Fatalf("Failed to get singleton ImageManager: %v", err1)
		}

		newInstance, err2 := NewImageManager()
		if err2 != nil {
			t.Fatalf("Failed to create new ImageManager: %v", err2)
		}

		if singleton == newInstance {
			t.Error("Singleton and new instance should be different")
		}
	})
}
