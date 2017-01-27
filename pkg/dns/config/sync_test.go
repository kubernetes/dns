package config

import "testing"
import "reflect"

func TestEmptyInitialSync(t *testing.T) {
	// New mock source that returns empty results, but not errors
	mockSource := newMockSource(syncResult{}, nil)
	s := newSync(mockSource)

	// Make sure we get a default config from Once()
	config, err := s.Once()
	if err != nil {
		t.Fatal(err)
	}
	if config == nil {
		t.Fatal("unexpected nil config")
	}
	if !reflect.DeepEqual(config, NewDefaultConfig()) {
		t.Fatalf("expected default config, got %#v", config)
	}
}
