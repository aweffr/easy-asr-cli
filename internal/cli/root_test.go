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

func TestEnginesJSONListsImplementedEngines(t *testing.T) {
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
	if len(infos) != 3 || infos[0].Name != config.EngineQwen3Filetrans || infos[1].Name != config.EngineFunASR || infos[2].Name != config.EngineMimoV25ASR || !infos[2].Implemented {
		t.Fatalf("infos = %#v", infos)
	}
	if infos[2].ReferencePriceCNYPerHour != 0.5 {
		t.Fatalf("mimo reference price = %v", infos[2].ReferencePriceCNYPerHour)
	}
}

func TestTranscribeFunASRMapsFlags(t *testing.T) {
	dir := t.TempDir()
	audio := filepath.Join(dir, "voice.wav")
	if err := os.WriteFile(audio, []byte("audio"), 0o600); err != nil {
		t.Fatalf("write audio: %v", err)
	}
	var stdout bytes.Buffer
	runner := &fakeRunner{}
	cmd := cli.NewRootCommand(cli.Deps{
		Stdout:            &stdout,
		Stderr:            &bytes.Buffer{},
		DefaultConfigPath: func() (string, error) { return filepath.Join(dir, "config.yaml"), nil },
		LoadConfig: func(path string) (*config.Config, error) {
			cfg := validConfig()
			cfg.Engine = config.EngineFunASR
			return cfg, nil
		},
		Registry: func(cfg *config.Config) *engine.Registry {
			return engine.DefaultRegistry(&fakeRunner{}, runner)
		},
	})
	cmd.SetArgs([]string{
		"transcribe",
		"--engine", "fun-asr",
		"--language", "zh",
		"--channel", "0",
		"--vocabulary-id", "vocab-123",
		"--speaker-count", "2",
		audio,
	})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	if runner.request.VocabularyID != "vocab-123" {
		t.Fatalf("VocabularyID = %q", runner.request.VocabularyID)
	}
	if !runner.request.DiarizationEnabled {
		t.Fatal("DiarizationEnabled should default to true for fun-asr")
	}
	if runner.request.SpeakerCount != 2 {
		t.Fatalf("SpeakerCount = %d", runner.request.SpeakerCount)
	}
}

func TestTranscribeFunASRRejectsQwenOnlyFlags(t *testing.T) {
	dir := t.TempDir()
	audio := filepath.Join(dir, "voice.wav")
	if err := os.WriteFile(audio, []byte("audio"), 0o600); err != nil {
		t.Fatalf("write audio: %v", err)
	}
	cmd := cli.NewRootCommand(cli.Deps{
		Stdout:            &bytes.Buffer{},
		Stderr:            &bytes.Buffer{},
		DefaultConfigPath: func() (string, error) { return filepath.Join(dir, "config.yaml"), nil },
		LoadConfig: func(path string) (*config.Config, error) {
			cfg := validConfig()
			cfg.Engine = config.EngineFunASR
			return cfg, nil
		},
		Registry: func(cfg *config.Config) *engine.Registry {
			return engine.DefaultRegistry(&fakeRunner{}, &fakeRunner{})
		},
	})
	cmd.SetArgs([]string{"transcribe", "--engine", "fun-asr", "--hotwords", "星巴克", audio})

	err := cmd.Execute()
	if err == nil {
		t.Fatal("Execute returned nil error")
	}
	if !engine.IsUsageError(err) {
		t.Fatalf("error should be usage error, got %T: %v", err, err)
	}
}

func TestTranscribeFunASRRejectsDiarizationWithMultipleChannels(t *testing.T) {
	dir := t.TempDir()
	audio := filepath.Join(dir, "voice.wav")
	if err := os.WriteFile(audio, []byte("audio"), 0o600); err != nil {
		t.Fatalf("write audio: %v", err)
	}
	cmd := cli.NewRootCommand(cli.Deps{
		Stdout:            &bytes.Buffer{},
		Stderr:            &bytes.Buffer{},
		DefaultConfigPath: func() (string, error) { return filepath.Join(dir, "config.yaml"), nil },
		LoadConfig: func(path string) (*config.Config, error) {
			cfg := validConfig()
			cfg.Engine = config.EngineFunASR
			return cfg, nil
		},
		Registry: func(cfg *config.Config) *engine.Registry {
			return engine.DefaultRegistry(&fakeRunner{}, &fakeRunner{})
		},
	})
	cmd.SetArgs([]string{"transcribe", "--engine", "fun-asr", "--channel", "0", "--channel", "1", audio})

	err := cmd.Execute()
	if err == nil {
		t.Fatal("Execute returned nil error")
	}
	if !engine.IsUsageError(err) {
		t.Fatalf("error should be usage error, got %T: %v", err, err)
	}
}

func TestTranscribeFunASRNoDiarizationClearsConfiguredSpeakerCount(t *testing.T) {
	dir := t.TempDir()
	audio := filepath.Join(dir, "voice.wav")
	if err := os.WriteFile(audio, []byte("audio"), 0o600); err != nil {
		t.Fatalf("write audio: %v", err)
	}
	runner := &fakeRunner{}
	cmd := cli.NewRootCommand(cli.Deps{
		Stdout:            &bytes.Buffer{},
		Stderr:            &bytes.Buffer{},
		DefaultConfigPath: func() (string, error) { return filepath.Join(dir, "config.yaml"), nil },
		LoadConfig: func(path string) (*config.Config, error) {
			cfg := validConfig()
			cfg.Engine = config.EngineFunASR
			cfg.Engines.FunASR.ASR.SpeakerCount = 2
			return cfg, nil
		},
		Registry: func(cfg *config.Config) *engine.Registry {
			return engine.DefaultRegistry(&fakeRunner{}, runner)
		},
	})
	cmd.SetArgs([]string{"transcribe", "--engine", "fun-asr", "--no-diarization", audio})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	if runner.request.DiarizationEnabled {
		t.Fatal("DiarizationEnabled should be false")
	}
	if runner.request.SpeakerCount != 0 {
		t.Fatalf("SpeakerCount = %d, want 0", runner.request.SpeakerCount)
	}
}

func TestTranscribeMimoMapsLanguageAndRejectsUnsupportedFlags(t *testing.T) {
	dir := t.TempDir()
	audio := filepath.Join(dir, "voice.wav")
	if err := os.WriteFile(audio, []byte("audio"), 0o600); err != nil {
		t.Fatalf("write audio: %v", err)
	}
	runner := &fakeRunner{}
	cmd := cli.NewRootCommand(cli.Deps{
		Stdout:            &bytes.Buffer{},
		Stderr:            &bytes.Buffer{},
		DefaultConfigPath: func() (string, error) { return filepath.Join(dir, "config.yaml"), nil },
		LoadConfig: func(path string) (*config.Config, error) {
			cfg := validConfig()
			cfg.Engine = config.EngineMimoV25ASR
			cfg.MiMoV25ASR().MiMo.APIKey = "mimo-key"
			return cfg, nil
		},
		Registry: func(cfg *config.Config) *engine.Registry {
			return engine.DefaultRegistry(&fakeRunner{}, &fakeRunner{}, runner)
		},
	})
	cmd.SetArgs([]string{"transcribe", "--engine", "mimo-v2.5-asr", "--language", "en", audio})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	if runner.request.Language != "en" {
		t.Fatalf("Language = %q", runner.request.Language)
	}

	cmd = cli.NewRootCommand(cli.Deps{
		Stdout:            &bytes.Buffer{},
		Stderr:            &bytes.Buffer{},
		DefaultConfigPath: func() (string, error) { return filepath.Join(dir, "config.yaml"), nil },
		LoadConfig: func(path string) (*config.Config, error) {
			cfg := validConfig()
			cfg.Engine = config.EngineMimoV25ASR
			cfg.MiMoV25ASR().MiMo.APIKey = "mimo-key"
			return cfg, nil
		},
		Registry: func(cfg *config.Config) *engine.Registry {
			return engine.DefaultRegistry(&fakeRunner{}, &fakeRunner{}, runner)
		},
	})
	cmd.SetArgs([]string{"transcribe", "--engine", "mimo-v2.5-asr", "--hotwords", "test", audio})
	err := cmd.Execute()
	if err == nil {
		t.Fatal("Execute returned nil error")
	}
	if !engine.IsUsageError(err) {
		t.Fatalf("error should be usage error, got %T: %v", err, err)
	}
}

func TestDoctorReportsMissingLocalDependencies(t *testing.T) {
	var stdout bytes.Buffer
	cmd := cli.NewRootCommand(cli.Deps{
		Stdout:            &stdout,
		Stderr:            &bytes.Buffer{},
		DefaultConfigPath: func() (string, error) { return "/tmp/config.yaml", nil },
		LoadConfig: func(path string) (*config.Config, error) {
			cfg := validConfig()
			cfg.Engine = config.EngineMimoV25ASR
			cfg.MiMoV25ASR().MiMo.APIKey = "mimo-key"
			cfg.MiMoV25ASR().Segmentation.ModelPath = filepath.Join(t.TempDir(), "missing.onnx")
			cfg.MiMoV25ASR().Segmentation.ONNXRuntimeLibraryPath = filepath.Join(t.TempDir(), "missing.dylib")
			return cfg, nil
		},
		Registry: func(cfg *config.Config) *engine.Registry { return engine.DefaultRegistry(&fakeRunner{}) },
	})
	cmd.SetArgs([]string{"doctor"})

	err := cmd.Execute()
	if err == nil {
		t.Fatal("Execute returned nil error")
	}
	if !strings.Contains(stdout.String(), "silero vad model") || !strings.Contains(stdout.String(), "onnx runtime") {
		t.Fatalf("doctor output = %q", stdout.String())
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
