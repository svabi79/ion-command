package plugins

import (
	"context"
	"testing"
)

type testSource struct {
	id string
}

func (s testSource) ID() string                                    { return s.id }
func (s testSource) Type() string                                  { return "mock.radio" }
func (s testSource) Start(context.Context, chan<- RawRecord) error { return nil }

func TestSecondSourceInstanceRegistersWithoutCoreChanges(t *testing.T) {
	registry := NewRegistry()
	if err := registry.RegisterSource(testSource{id: "mock-primary"}); err != nil {
		t.Fatal(err)
	}
	if err := registry.RegisterSource(testSource{id: "mock-secondary"}); err != nil {
		t.Fatalf("second source instance should be independently registrable: %v", err)
	}
	if got := len(registry.Sources()); got != 2 {
		t.Fatalf("expected two registered sources, got %d", got)
	}
}

func TestDuplicateSourceIdentityIsRejected(t *testing.T) {
	registry := NewRegistry()
	if err := registry.RegisterSource(testSource{id: "same"}); err != nil {
		t.Fatal(err)
	}
	if err := registry.RegisterSource(testSource{id: "same"}); err == nil {
		t.Fatal("duplicate source identity accepted")
	}
}
