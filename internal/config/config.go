package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

const (
	EngineQwen3Filetrans = "qwen3-asr-flash-filetrans"

	defaultBaseURL = "https://dashscope.aliyuncs.com/api/v1"
	defaultModel   = EngineQwen3Filetrans
)

type UsageError struct {
	Message string
}

func (e UsageError) Error() string {
	return e.Message
}

func IsUsageError(err error) bool {
	var target UsageError
	return errors.As(err, &target)
}

type Config struct {
	Engine  string        `yaml:"engine" json:"engine"`
	Engines EnginesConfig `yaml:"engines" json:"engines"`
}

type EnginesConfig struct {
	Qwen3ASRFlashFiletrans Qwen3Config    `yaml:"qwen3_asr_flash_filetrans" json:"qwen3_asr_flash_filetrans"`
	FunASR                 ReservedConfig `yaml:"fun_asr" json:"fun_asr"`
	SeedASR                ReservedConfig `yaml:"seed_asr" json:"seed_asr"`
}

type ReservedConfig struct {
	Enabled bool `yaml:"enabled" json:"enabled"`
}

type Qwen3Config struct {
	DashScope DashScopeConfig `yaml:"dashscope" json:"dashscope"`
	OSS       OSSConfig       `yaml:"oss" json:"oss"`
	ASR       ASRConfig       `yaml:"asr" json:"asr"`
}

type DashScopeConfig struct {
	APIKey  string `yaml:"api_key" json:"api_key"`
	BaseURL string `yaml:"base_url" json:"base_url"`
	Model   string `yaml:"model" json:"model"`
}

type OSSConfig struct {
	Region          string        `yaml:"region" json:"region"`
	Endpoint        string        `yaml:"endpoint" json:"endpoint"`
	Bucket          string        `yaml:"bucket" json:"bucket"`
	AccessKeyID     string        `yaml:"access_key_id" json:"access_key_id"`
	AccessKeySecret string        `yaml:"access_key_secret" json:"access_key_secret"`
	KeyPrefix       string        `yaml:"key_prefix" json:"key_prefix"`
	PresignTTL      time.Duration `yaml:"presign_ttl" json:"presign_ttl"`
}

type ASRConfig struct {
	ChannelIDs     []int         `yaml:"channel_ids" json:"channel_ids"`
	Language       string        `yaml:"language,omitempty" json:"language,omitempty"`
	EnableITN      bool          `yaml:"enable_itn" json:"enable_itn"`
	EnableWords    bool          `yaml:"enable_words" json:"enable_words"`
	PollInterval   time.Duration `yaml:"poll_interval" json:"poll_interval"`
	PollTimeout    time.Duration `yaml:"poll_timeout" json:"poll_timeout"`
	RequestTimeout time.Duration `yaml:"request_timeout" json:"request_timeout"`
}

func (o *OSSConfig) UnmarshalYAML(value *yaml.Node) error {
	type raw struct {
		Region          string `yaml:"region"`
		Endpoint        string `yaml:"endpoint"`
		Bucket          string `yaml:"bucket"`
		AccessKeyID     string `yaml:"access_key_id"`
		AccessKeySecret string `yaml:"access_key_secret"`
		KeyPrefix       string `yaml:"key_prefix"`
		PresignTTL      string `yaml:"presign_ttl"`
	}
	var r raw
	if err := value.Decode(&r); err != nil {
		return err
	}
	if r.Region != "" {
		o.Region = r.Region
	}
	if r.Endpoint != "" {
		o.Endpoint = r.Endpoint
	}
	if r.Bucket != "" {
		o.Bucket = r.Bucket
	}
	if r.AccessKeyID != "" {
		o.AccessKeyID = r.AccessKeyID
	}
	if r.AccessKeySecret != "" {
		o.AccessKeySecret = r.AccessKeySecret
	}
	if r.KeyPrefix != "" {
		o.KeyPrefix = r.KeyPrefix
	}
	if r.PresignTTL != "" {
		duration, err := time.ParseDuration(r.PresignTTL)
		if err != nil {
			return fmt.Errorf("parse oss.presign_ttl: %w", err)
		}
		o.PresignTTL = duration
	}
	return nil
}

func (o OSSConfig) MarshalYAML() (any, error) {
	return struct {
		Region          string `yaml:"region"`
		Endpoint        string `yaml:"endpoint"`
		Bucket          string `yaml:"bucket"`
		AccessKeyID     string `yaml:"access_key_id"`
		AccessKeySecret string `yaml:"access_key_secret"`
		KeyPrefix       string `yaml:"key_prefix"`
		PresignTTL      string `yaml:"presign_ttl"`
	}{
		Region:          o.Region,
		Endpoint:        o.Endpoint,
		Bucket:          o.Bucket,
		AccessKeyID:     o.AccessKeyID,
		AccessKeySecret: o.AccessKeySecret,
		KeyPrefix:       o.KeyPrefix,
		PresignTTL:      o.PresignTTL.String(),
	}, nil
}

func (a *ASRConfig) UnmarshalYAML(value *yaml.Node) error {
	type raw struct {
		ChannelIDs     []int          `yaml:"channel_ids"`
		Language       string         `yaml:"language"`
		EnableITN      *bool          `yaml:"enable_itn"`
		EnableWords    *bool          `yaml:"enable_words"`
		PollInterval   string         `yaml:"poll_interval"`
		PollTimeout    string         `yaml:"poll_timeout"`
		RequestTimeout string         `yaml:"request_timeout"`
		Unknown        map[string]any `yaml:",inline"`
	}
	var r raw
	if err := value.Decode(&r); err != nil {
		return err
	}
	if len(r.ChannelIDs) > 0 {
		a.ChannelIDs = r.ChannelIDs
	}
	if r.Language != "" {
		a.Language = r.Language
	}
	if r.EnableITN != nil {
		a.EnableITN = *r.EnableITN
	}
	if r.EnableWords != nil {
		a.EnableWords = *r.EnableWords
	}
	if r.PollInterval != "" {
		duration, err := time.ParseDuration(r.PollInterval)
		if err != nil {
			return fmt.Errorf("parse asr.poll_interval: %w", err)
		}
		a.PollInterval = duration
	}
	if r.PollTimeout != "" {
		duration, err := time.ParseDuration(r.PollTimeout)
		if err != nil {
			return fmt.Errorf("parse asr.poll_timeout: %w", err)
		}
		a.PollTimeout = duration
	}
	if r.RequestTimeout != "" {
		duration, err := time.ParseDuration(r.RequestTimeout)
		if err != nil {
			return fmt.Errorf("parse asr.request_timeout: %w", err)
		}
		a.RequestTimeout = duration
	}
	return nil
}

func (a ASRConfig) MarshalYAML() (any, error) {
	return struct {
		ChannelIDs     []int  `yaml:"channel_ids"`
		Language       string `yaml:"language,omitempty"`
		EnableITN      bool   `yaml:"enable_itn"`
		EnableWords    bool   `yaml:"enable_words"`
		PollInterval   string `yaml:"poll_interval"`
		PollTimeout    string `yaml:"poll_timeout"`
		RequestTimeout string `yaml:"request_timeout"`
	}{
		ChannelIDs:     a.ChannelIDs,
		Language:       a.Language,
		EnableITN:      a.EnableITN,
		EnableWords:    a.EnableWords,
		PollInterval:   a.PollInterval.String(),
		PollTimeout:    a.PollTimeout.String(),
		RequestTimeout: a.RequestTimeout.String(),
	}, nil
}

func DefaultPath() (string, error) {
	if value := strings.TrimSpace(os.Getenv("XDG_CONFIG_HOME")); value != "" {
		return filepath.Join(value, "easy_asr", "config.yaml"), nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".config", "easy_asr", "config.yaml"), nil
}

func Default() *Config {
	return &Config{
		Engine: EngineQwen3Filetrans,
		Engines: EnginesConfig{
			Qwen3ASRFlashFiletrans: Qwen3Config{
				DashScope: DashScopeConfig{
					BaseURL: defaultBaseURL,
					Model:   defaultModel,
				},
				OSS: OSSConfig{
					Region:     "cn-beijing",
					Endpoint:   "https://oss-cn-beijing.aliyuncs.com",
					KeyPrefix:  "easy_asr/tmp",
					PresignTTL: 24 * time.Hour,
				},
				ASR: ASRConfig{
					ChannelIDs:     []int{0},
					EnableITN:      false,
					EnableWords:    true,
					PollInterval:   5 * time.Second,
					PollTimeout:    2 * time.Hour,
					RequestTimeout: 30 * time.Second,
				},
			},
		},
	}
}

func Load(path string) (*Config, error) {
	cfg := Default()
	if strings.TrimSpace(path) == "" {
		var err error
		path, err = DefaultPath()
		if err != nil {
			return nil, err
		}
	}
	body, err := os.ReadFile(path)
	if err != nil {
		if !errors.Is(err, os.ErrNotExist) {
			return nil, err
		}
	} else if len(strings.TrimSpace(string(body))) > 0 {
		if err := yaml.Unmarshal(body, cfg); err != nil {
			return nil, err
		}
	}
	cfg.applyEnv()
	return cfg, nil
}

func WriteDefault(path string) error {
	cfg := Default()
	body, err := yaml.Marshal(cfg)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	if err := os.Chmod(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		if errors.Is(err, os.ErrExist) {
			return UsageError{Message: "config already exists: " + path}
		}
		return err
	}
	defer file.Close()
	if _, err := file.Write(body); err != nil {
		return err
	}
	return file.Chmod(0o600)
}

func (c *Config) Qwen3() *Qwen3Config {
	return &c.Engines.Qwen3ASRFlashFiletrans
}

func (c *Config) Validate() error {
	if strings.TrimSpace(c.Engine) == "" {
		return UsageError{Message: "engine is required"}
	}
	if c.Engine != EngineQwen3Filetrans {
		return UsageError{Message: fmt.Sprintf("engine %q is not implemented", c.Engine)}
	}
	qwen := c.Qwen3()
	missing := []string{}
	check := func(name, value string) {
		if strings.TrimSpace(value) == "" {
			missing = append(missing, name)
		}
	}
	check("dashscope.api_key", qwen.DashScope.APIKey)
	check("oss.region", qwen.OSS.Region)
	check("oss.endpoint", qwen.OSS.Endpoint)
	check("oss.bucket", qwen.OSS.Bucket)
	check("oss.access_key_id", qwen.OSS.AccessKeyID)
	check("oss.access_key_secret", qwen.OSS.AccessKeySecret)
	if len(missing) > 0 {
		return UsageError{Message: "missing required config: " + strings.Join(missing, ", ")}
	}
	return nil
}

func (c *Config) Redacted() *Config {
	copy := *c
	copy.Engines = c.Engines
	qwen := copy.Qwen3()
	qwen.DashScope.APIKey = redactSecret(qwen.DashScope.APIKey)
	qwen.OSS.AccessKeyID = redactID(qwen.OSS.AccessKeyID)
	qwen.OSS.AccessKeySecret = redactSecret(qwen.OSS.AccessKeySecret)
	return &copy
}

func (c *Config) applyEnv() {
	if value := firstEnv("EASY_ASR_ENGINE"); value != "" {
		c.Engine = value
	}
	qwen := c.Qwen3()
	if value := firstEnv("EASY_ASR_DASHSCOPE_API_KEY", "DASHSCOPE_API_KEY", "ALIBABA_DASHSCOPE_API_KEY"); value != "" {
		qwen.DashScope.APIKey = value
	}
	if value := firstEnv("EASY_ASR_DASHSCOPE_BASE_URL"); value != "" {
		qwen.DashScope.BaseURL = value
	}
	if value := firstEnv("EASY_ASR_DASHSCOPE_MODEL"); value != "" {
		qwen.DashScope.Model = value
	}
	if value := firstEnv("EASY_ASR_OSS_REGION"); value != "" {
		qwen.OSS.Region = value
	}
	if value := firstEnv("EASY_ASR_OSS_ENDPOINT", "AWS_S3_ENDPOINT_URL"); value != "" {
		qwen.OSS.Endpoint = value
	}
	if value := firstEnv("EASY_ASR_OSS_BUCKET", "AWS_STORAGE_BUCKET_NAME"); value != "" {
		qwen.OSS.Bucket = value
	}
	if value := firstEnv("EASY_ASR_OSS_ACCESS_KEY_ID", "AWS_ACCESS_KEY_ID"); value != "" {
		qwen.OSS.AccessKeyID = value
	}
	if value := firstEnv("EASY_ASR_OSS_ACCESS_KEY_SECRET", "AWS_SECRET_ACCESS_KEY"); value != "" {
		qwen.OSS.AccessKeySecret = value
	}
	if value := firstEnv("EASY_ASR_OSS_KEY_PREFIX"); value != "" {
		qwen.OSS.KeyPrefix = value
	}
}

func firstEnv(names ...string) string {
	for _, name := range names {
		if value := strings.TrimSpace(os.Getenv(name)); value != "" {
			return value
		}
	}
	return ""
}

func redactSecret(value string) string {
	if value == "" {
		return ""
	}
	return "<redacted>"
}

func redactID(value string) string {
	if value == "" {
		return ""
	}
	if len(value) <= 6 {
		return value[:1] + "***"
	}
	return value[:3] + "***" + value[len(value)-3:]
}
