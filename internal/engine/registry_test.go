package engine_test

import (
	"context"
	"errors"
	"testing"

	"github.com/aweffr/easy-asr-cli/internal/engine"
)

func TestRegistryExposesDefaultAndReservedEngines(t *testing.T) {
	registry := engine.DefaultRegistry(nil)

	infos := registry.List()
	if len(infos) != 3 {
		t.Fatalf("List returned %d engines, want 3", len(infos))
	}
	if registry.DefaultName() != "qwen3-asr-flash-filetrans" {
		t.Fatalf("DefaultName = %q", registry.DefaultName())
	}
	if !infos[0].Implemented {
		t.Fatalf("default engine should be implemented")
	}
	if infos[1].Implemented || infos[2].Implemented {
		t.Fatalf("reserved engines should not be implemented: %#v", infos)
	}
}

func TestReservedEngineReturnsNotImplemented(t *testing.T) {
	registry := engine.DefaultRegistry(nil)
	runner, err := registry.Get("fun-asr")
	if err != nil {
		t.Fatalf("Get fun-asr returned error: %v", err)
	}

	_, err = runner.Transcribe(context.Background(), engine.Request{})
	if !errors.Is(err, engine.ErrNotImplemented) {
		t.Fatalf("reserved engine error = %v, want ErrNotImplemented", err)
	}
}

func TestUnknownEngineIsUsageError(t *testing.T) {
	registry := engine.DefaultRegistry(nil)
	_, err := registry.Get("missing")
	if err == nil {
		t.Fatal("Get missing returned nil error")
	}
	if !engine.IsUsageError(err) {
		t.Fatalf("unknown engine should be usage error, got %T: %v", err, err)
	}
}
