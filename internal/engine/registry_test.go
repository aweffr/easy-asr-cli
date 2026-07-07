package engine_test

import (
	"context"
	"errors"
	"testing"

	"github.com/aweffr/easy-asr-cli/internal/engine"
)

func TestRegistryExposesImplementedEngines(t *testing.T) {
	registry := engine.DefaultRegistry(nil)

	infos := registry.List()
	if len(infos) != 3 {
		t.Fatalf("List returned %d engines, want 3", len(infos))
	}
	if registry.DefaultName() != "qwen3-asr-flash-filetrans" {
		t.Fatalf("DefaultName = %q", registry.DefaultName())
	}
	for _, info := range infos {
		if !info.Implemented {
			t.Fatalf("engine should be implemented: %#v", info)
		}
		if info.ReferencePriceCNYPerHour <= 0 {
			t.Fatalf("engine should expose reference price: %#v", info)
		}
	}
}

func TestFunASREngineCanBeRegistered(t *testing.T) {
	fun := engine.RunnerFunc(func(context.Context, engine.Request) (engine.Result, error) {
		return engine.Result{Engine: "fun-asr"}, nil
	})
	registry := engine.DefaultRegistry(nil, fun)
	runner, err := registry.Get("fun-asr")
	if err != nil {
		t.Fatalf("Get fun-asr returned error: %v", err)
	}

	result, err := runner.Transcribe(context.Background(), engine.Request{})
	if err != nil {
		t.Fatalf("Transcribe returned error: %v", err)
	}
	if result.Engine != "fun-asr" {
		t.Fatalf("result = %#v", result)
	}
}

func TestMissingFunASRRunnerReturnsNotImplemented(t *testing.T) {
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
