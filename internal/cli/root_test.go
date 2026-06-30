package cli_test

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/aweffr/easy-asr-cli/internal/cli"
	"github.com/aweffr/easy-asr-cli/internal/config"
	"github.com/aweffr/easy-asr-cli/internal/engine"
)

func TestTranscribeDefaultsToSiblingSRTAndPrintsOnlyPath(t *testing.T) {
	dir := t.TempDir()
	audio := filepath.Join(dir, "voice.mp3")
	if err := os.WriteFile(audio, []byte("audio"), 0o600); err != nil {
		t.Fatalf("write audio: %v", err)
	}
	var stdout, stderr bytes.Buffer
	runner := &fakeRunner{}
	cmd := cli.NewRootCommand(cli.Deps{
		Stdout: &stdout,
		Stderr: &stderr,
		DefaultConfigPath: func() (string, error) {
			return filepath.Join(dir, "config.yaml"), nil
		},
		LoadConfig: func(path string) (*config.Config, error) {
			return validConfig(), nil
		},
		Registry: func(cfg *config.Config) *engine.Registry {
			return engine.DefaultRegistry(runner)
		},
	})
	cmd.SetArgs([]string{"transcribe", audio})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}

	wantOut := filepath.Join(dir, "voice.srt")
	if strings.TrimSpace(stdout.String()) != wantOut {
		t.Fatalf("stdout = %q, want %q", stdout.String(), wantOut)
	}
	if stderr.String() != "" {
		t.Fatalf("stderr = %q, want empty", stderr.String())
	}
	if runner.request.AudioPath != audio {
		t.Fatalf("AudioPath = %q", runner.request.AudioPath)
	}
	if runner.request.OutputPath != wantOut {
		t.Fatalf("OutputPath = %q", runner.request.OutputPath)
	}
	if !runner.request.EnableWords {
		t.Fatal("EnableWords should default to config true")
	}
}

func TestTranscribeJSONPrintsRunResult(t *testing.T) {
	dir := t.TempDir()
	audio := filepath.Join(dir, "voice.mp3")
	if err := os.WriteFile(audio, []byte("audio"), 0o600); err != nil {
		t.Fatalf("write audio: %v", err)
	}
	var stdout bytes.Buffer
	runner := &fakeRunner{result: engine.Result{Engine: config.EngineQwen3Filetrans, TaskID: "task-1", OutputPath: filepath.Join(dir, "voice.srt")}}
	cmd := cli.NewRootCommand(cli.Deps{
		Stdout:            &stdout,
		Stderr:            &bytes.Buffer{},
		DefaultConfigPath: func() (string, error) { return filepath.Join(dir, "config.yaml"), nil },
		LoadConfig:        func(path string) (*config.Config, error) { return validConfig(), nil },
		Registry:          func(cfg *config.Config) *engine.Registry { return engine.DefaultRegistry(runner) },
	})
	cmd.SetArgs([]string{"transcribe", "--json", audio})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	var payload engine.Result
	if err := json.Unmarshal(stdout.Bytes(), &payload); err != nil {
		t.Fatalf("stdout is not JSON: %v\n%s", err, stdout.String())
	}
	if payload.TaskID != "task-1" || payload.OutputPath == "" {
		t.Fatalf("payload = %#v", payload)
	}
}

func TestEnginesJSONListsReservedEngines(t *testing.T) {
	var stdout bytes.Buffer
	cmd := cli.NewRootCommand(cli.Deps{
		Stdout:            &stdout,
		Stderr:            &bytes.Buffer{},
		DefaultConfigPath: func() (string, error) { return "/tmp/config.yaml", nil },
		LoadConfig:        func(path string) (*config.Config, error) { return validConfig(), nil },
		Registry:          func(cfg *config.Config) *engine.Registry { return engine.DefaultRegistry(&fakeRunner{}) },
	})
	cmd.SetArgs([]string{"engines", "--json"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	var infos []engine.Info
	if err := json.Unmarshal(stdout.Bytes(), &infos); err != nil {
		t.Fatalf("stdout is not JSON: %v\n%s", err, stdout.String())
	}
	if len(infos) != 3 || infos[0].Name != config.EngineQwen3Filetrans || infos[1].Implemented {
		t.Fatalf("infos = %#v", infos)
	}
}

func TestConfigPathUsesDefaultPath(t *testing.T) {
	var stdout bytes.Buffer
	cmd := cli.NewRootCommand(cli.Deps{
		Stdout:            &stdout,
		Stderr:            &bytes.Buffer{},
		DefaultConfigPath: func() (string, error) { return "/tmp/easy_asr/config.yaml", nil },
		LoadConfig:        func(path string) (*config.Config, error) { return validConfig(), nil },
		Registry:          func(cfg *config.Config) *engine.Registry { return engine.DefaultRegistry(&fakeRunner{}) },
	})
	cmd.SetArgs([]string{"config", "path"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	if strings.TrimSpace(stdout.String()) != "/tmp/easy_asr/config.yaml" {
		t.Fatalf("stdout = %q", stdout.String())
	}
}

func TestExecuteMapsUsageErrorsToExitCodeTwo(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := cli.Execute([]string{"missing-command"}, &stdout, &stderr)
	if code != cli.ExitUsage {
		t.Fatalf("exit code = %d, want %d", code, cli.ExitUsage)
	}
	if stdout.String() != "" {
		t.Fatalf("stdout = %q, want empty", stdout.String())
	}
	if !strings.Contains(stderr.String(), "error:") {
		t.Fatalf("stderr = %q, want error", stderr.String())
	}
}

type fakeRunner struct {
	request engine.Request
	result  engine.Result
}

func (f *fakeRunner) Transcribe(ctx context.Context, request engine.Request) (engine.Result, error) {
	f.request = request
	if f.result.Engine == "" {
		f.result = engine.Result{Engine: config.EngineQwen3Filetrans, OutputPath: request.OutputPath}
	}
	return f.result, nil
}

func validConfig() *config.Config {
	cfg := config.Default()
	qwen := cfg.Qwen3()
	qwen.DashScope.APIKey = "dashscope-key"
	qwen.OSS.Bucket = "bucket"
	qwen.OSS.AccessKeyID = "access"
	qwen.OSS.AccessKeySecret = "secret"
	return cfg
}
