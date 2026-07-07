package config_test

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/aweffr/easy-asr-cli/internal/config"
)

func TestDefaultPathUsesXDGConfigHome(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", "/tmp/xdg-config")
	got, err := config.DefaultPath()
	if err != nil {
		t.Fatalf("DefaultPath returned error: %v", err)
	}
	want := filepath.Join("/tmp/xdg-config", "easy_asr", "config.yaml")
	if got != want {
		t.Fatalf("DefaultPath() = %q, want %q", got, want)
	}
}

func TestLoadMergesYAMLCompatibleEnvAndDefaults(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	content := []byte(`
engine: qwen3-asr-flash-filetrans
engines:
  qwen3_asr_flash_filetrans:
    dashscope:
      api_key: yaml-dashscope
      base_url: https://dashscope.aliyuncs.com/api/v1
      model: qwen3-asr-flash-filetrans
    oss:
      region: cn-shanghai
      endpoint: https://oss-cn-shanghai.aliyuncs.com
      bucket: yaml-bucket
      access_key_id: yaml-access
      access_key_secret: yaml-secret
      key_prefix: yaml-prefix
      presign_ttl: 12h
    asr:
      channel_ids: [0, 1]
      enable_itn: true
      enable_words: false
      poll_interval: 3s
      poll_timeout: 45m
      request_timeout: 10s
`)
	if err := os.WriteFile(path, content, 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}
	t.Setenv("DASHSCOPE_API_KEY", "env-dashscope")
	t.Setenv("AWS_STORAGE_BUCKET_NAME", "env-bucket")
	t.Setenv("EASY_ASR_OSS_KEY_PREFIX", "env-prefix")

	cfg, err := config.Load(path)
	if err != nil {
		t.Fatalf("Load returned error: %v", err)
	}

	qwen := cfg.Qwen3()
	if cfg.Engine != config.EngineQwen3Filetrans {
		t.Fatalf("Engine = %q", cfg.Engine)
	}
	if qwen.DashScope.APIKey != "env-dashscope" {
		t.Fatalf("DashScope.APIKey = %q", qwen.DashScope.APIKey)
	}
	if qwen.OSS.Bucket != "env-bucket" {
		t.Fatalf("OSS.Bucket = %q", qwen.OSS.Bucket)
	}
	if qwen.OSS.KeyPrefix != "env-prefix" {
		t.Fatalf("OSS.KeyPrefix = %q", qwen.OSS.KeyPrefix)
	}
	if qwen.ASR.PollInterval != 3*time.Second {
		t.Fatalf("PollInterval = %s", qwen.ASR.PollInterval)
	}
	if qwen.ASR.EnableWords {
		t.Fatalf("EnableWords should preserve YAML false when no env/flag overrides it")
	}
}

func TestFunASRConfigInheritsQwen3DefaultsAndSupportsOverrides(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	content := []byte(`
engine: fun-asr
engines:
  qwen3_asr_flash_filetrans:
    dashscope:
      api_key: qwen-key
      base_url: https://dashscope.aliyuncs.com/api/v1
      model: qwen3-asr-flash-filetrans
    oss:
      region: cn-shanghai
      endpoint: https://oss-cn-shanghai.aliyuncs.com
      bucket: qwen-bucket
      access_key_id: qwen-access
      access_key_secret: qwen-secret
      key_prefix: qwen-prefix
      presign_ttl: 12h
    asr:
      channel_ids: [0]
      poll_interval: 3s
      poll_timeout: 45m
      request_timeout: 10s
  fun_asr:
    dashscope:
      model: fun-asr-mtl
    asr:
      vocabulary_id: vocab-123
      speaker_count: 3
`)
	if err := os.WriteFile(path, content, 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}

	cfg, err := config.Load(path)
	if err != nil {
		t.Fatalf("Load returned error: %v", err)
	}
	fun := cfg.FunASR()
	if fun.DashScope.APIKey != "qwen-key" {
		t.Fatalf("FunASR APIKey = %q", fun.DashScope.APIKey)
	}
	if fun.DashScope.Model != "fun-asr-mtl" {
		t.Fatalf("FunASR model = %q", fun.DashScope.Model)
	}
	if fun.OSS.Bucket != "qwen-bucket" || fun.OSS.KeyPrefix != "qwen-prefix" {
		t.Fatalf("FunASR OSS = %#v", fun.OSS)
	}
	if fun.ASR.PollInterval != 3*time.Second {
		t.Fatalf("FunASR PollInterval = %s", fun.ASR.PollInterval)
	}
	if !fun.ASR.DiarizationEnabled {
		t.Fatal("FunASR diarization should default to enabled")
	}
	if fun.ASR.VocabularyID != "vocab-123" || fun.ASR.SpeakerCount != 3 {
		t.Fatalf("FunASR ASR = %#v", fun.ASR)
	}
}

func TestValidateAcceptsFunASRWithInheritedCredentials(t *testing.T) {
	cfg := config.Default()
	cfg.Engine = config.EngineFunASR
	qwen := cfg.Qwen3()
	qwen.DashScope.APIKey = "dashscope-key"
	qwen.OSS.Bucket = "bucket"
	qwen.OSS.AccessKeyID = "access"
	qwen.OSS.AccessKeySecret = "secret"

	if err := cfg.Validate(); err != nil {
		t.Fatalf("Validate returned error: %v", err)
	}
}

func TestValidateRejectsInvalidFunASRSpeakerCount(t *testing.T) {
	cfg := config.Default()
	cfg.Engine = config.EngineFunASR
	qwen := cfg.Qwen3()
	qwen.DashScope.APIKey = "dashscope-key"
	qwen.OSS.Bucket = "bucket"
	qwen.OSS.AccessKeyID = "access"
	qwen.OSS.AccessKeySecret = "secret"
	cfg.Engines.FunASR.ASR.SpeakerCount = 101

	err := cfg.Validate()
	if err == nil {
		t.Fatal("Validate returned nil error")
	}
	if !config.IsUsageError(err) {
		t.Fatalf("error should be usage error, got %T: %v", err, err)
	}
}

func TestValidateRequiresQwen3Credentials(t *testing.T) {
	cfg := config.Default()
	err := cfg.Validate()
	if err == nil {
		t.Fatal("Validate returned nil for empty credentials")
	}
	if !config.IsUsageError(err) {
		t.Fatalf("Validate error should be usage error, got %T: %v", err, err)
	}
}

func TestRedactedConfigHidesSecrets(t *testing.T) {
	cfg := config.Default()
	qwen := cfg.Qwen3()
	qwen.DashScope.APIKey = "dashscope-secret"
	qwen.OSS.AccessKeyID = "oss-access"
	qwen.OSS.AccessKeySecret = "oss-secret"

	redacted := cfg.Redacted()
	qwenRedacted := redacted.Qwen3()
	if qwenRedacted.DashScope.APIKey == "dashscope-secret" {
		t.Fatal("DashScope API key was not redacted")
	}
	if qwenRedacted.OSS.AccessKeySecret == "oss-secret" {
		t.Fatal("OSS secret was not redacted")
	}
	if qwenRedacted.OSS.AccessKeyID == "" {
		t.Fatal("access key id should be partially visible after redaction")
	}
}

func TestWriteDefaultRefusesToOverwriteExistingConfigAndFixesDirectoryMode(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "easy_asr")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	path := filepath.Join(dir, "config.yaml")
	if err := os.WriteFile(path, []byte("existing"), 0o644); err != nil {
		t.Fatalf("write existing: %v", err)
	}

	err := config.WriteDefault(path)
	if err == nil {
		t.Fatal("WriteDefault returned nil for existing config")
	}
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read existing: %v", err)
	}
	if string(body) != "existing" {
		t.Fatalf("existing config was overwritten: %q", body)
	}
	info, err := os.Stat(dir)
	if err != nil {
		t.Fatalf("stat dir: %v", err)
	}
	if info.Mode().Perm() != 0o700 {
		t.Fatalf("dir mode = %o, want 700", info.Mode().Perm())
	}
}

func TestWriteDefaultCreatesSecretSafeFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "easy_asr", "config.yaml")
	if err := config.WriteDefault(path); err != nil {
		t.Fatalf("WriteDefault returned error: %v", err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat config: %v", err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("file mode = %o, want 600", info.Mode().Perm())
	}
}
