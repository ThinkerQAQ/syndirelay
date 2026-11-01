package publishing

import (
	"reflect"
	"sync"
	"testing"
)

func TestNormalizeDoesNotMutateCallerMap(t *testing.T) {
	original := DefaultConfig()
	original.Platforms["devto"] = PlatformConfig{Language: "en"}
	snapshot := make(map[string]PlatformConfig, len(original.Platforms))
	for name, platform := range original.Platforms {
		snapshot[name] = platform
	}
	normalized, err := Normalize(original)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(original.Platforms, snapshot) {
		t.Fatal("Normalize mutated its source Platform map")
	}
	if normalized.Platforms["devto"].Footer.Template == "" {
		t.Fatal("expected missing footer template to be resolved")
	}
	normalized.Platforms["devto"] = PlatformConfig{}
	if !reflect.DeepEqual(original.Platforms, snapshot) {
		t.Fatal("normalizing result aliases the caller's map")
	}
}

func TestParallelNormalizeSharedPublishingConfig(t *testing.T) {
	original := DefaultConfig()
	// Include partially configured profiles to force normalization writes.
	original.Platforms["medium"] = PlatformConfig{Language: "en"}
	original.Platforms["zhihu"] = PlatformConfig{Language: "zh-CN"}
	var workers sync.WaitGroup
	errors := make(chan error, 128)
	for i := 0; i < 128; i++ {
		workers.Add(1)
		go func() {
			defer workers.Done()
			for j := 0; j < 15; j++ {
				normalized, err := Normalize(original)
				if err != nil {
					errors <- err
					return
				}
				if normalized.Platforms["medium"].Footer.Template == "" {
					errors <- errMissingFooter{}
					return
				}
			}
		}()
	}
	workers.Wait()
	close(errors)
	for err := range errors {
		t.Errorf("concurrent Normalize: %v", err)
	}
}

type errMissingFooter struct{}

func (errMissingFooter) Error() string { return "missing normalized footer" }
