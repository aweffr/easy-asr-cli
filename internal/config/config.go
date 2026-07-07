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
	EngineFunASR         = "fun-asr"
	EngineMimoV25ASR     = "mimo-v2.5-asr"

	defaultBaseURL     = "https://dashscope.aliyuncs.com/api/v1"
	defaultMimoBaseURL = "https://api.xiaomimimo.com/v1"
	defaultModel       = EngineQwen3Filetrans
	defaultFunModel    = EngineFunASR
	defaultMimoModel   = EngineMimoV25ASR
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
	Qwen3ASRFlashFiletrans Qwen3Config  `yaml:"qwen3_asr_flash_filetrans" json:"qwen3_asr_flash_filetrans"`
	FunASR                 FunASRConfig `yaml:"fun_asr" json:"fun_asr"`
	MiMoV25ASR             MiMoConfig   `yaml:"mimo_v2_5_asr" json:"mimo_v2_5_asr"`
}

type ReservedConfig struct {
	Enabled bool `yaml:"enabled" json:"enabled"`
}

type Qwen3Config struct {
	DashScope DashScopeConfig `yaml:"dashscope" json:"dashscope"`
	OSS       OSSConfig       `yaml:"oss" json:"oss"`
	ASR       ASRConfig       `yaml:"asr" json:"asr"`
}

type FunASRConfig struct {
	DashScope DashScopeConfig `yaml:"dashscope" json:"dashscope"`
	OSS       OSSConfig       `yaml:"oss" json:"oss"`
	ASR       FunASRConfigASR `yaml:"asr" json:"asr"`
}

type ResolvedFunASRConfig struct {
	DashScope DashScopeConfig   `json:"dashscope"`
	OSS       OSSConfig         `json:"oss"`
	ASR       ResolvedFunASRASR `json:"asr"`
}

type MiMoConfig struct {
	MiMo         MiMoAPIConfig          `yaml:"mimo" json:"mimo"`
	ASR          MiMoASRConfig          `yaml:"asr" json:"asr"`
	Segmentation MiMoSegmentationConfig `yaml:"segmentation" json:"segmentation"`
}

type MiMoAPIConfig struct {
	APIKey  string `yaml:"api_key" json:"api_key"`
	BaseURL string `yaml:"base_url" json:"base_url"`
	Model   string `yaml:"model" json:"model"`
}

type MiMoASRConfig struct {
	Language       string        `yaml:"language" json:"language"`
	RequestTimeout time.Duration `yaml:"request_timeout" json:"request_timeout"`
}

type MiMoSegmentationConfig struct {
	TargetDuration         time.Duration `yaml:"target_duration" json:"target_duration"`
	MinDuration            time.Duration `yaml:"min_duration" json:"min_duration"`
	MaxDuration            time.Duration `yaml:"max_duration" json:"max_duration"`
	VADThreshold           float32       `yaml:"vad_threshold" json:"vad_threshold"`
	MinSilence             time.Duration `yaml:"min_silence" json:"min_silence"`
	SpeechPad              time.Duration `yaml:"speech_pad" json:"speech_pad"`
	ModelPath              string        `yaml:"model_path" json:"model_path"`
	ONNXRuntimeLibraryPath string        `yaml:"onnx_runtime_library_path" json:"onnx_runtime_library_path"`
	TempDir                string        `yaml:"temp_dir,omitempty" json:"temp_dir,omitempty"`
}

func (f FunASRConfig) MarshalYAML() (any, error) {
	type dashScope struct {
		APIKey  string `yaml:"api_key,omitempty"`
		BaseURL string `yaml:"base_url,omitempty"`
		Model   string `yaml:"model,omitempty"`
	}
	type raw struct {
		DashScope dashScope       `yaml:"dashscope,omitempty"`
		OSS       *OSSConfig      `yaml:"oss,omitempty"`
		ASR       FunASRConfigASR `yaml:"asr,omitempty"`
	}
	var oss *OSSConfig
	if f.OSS.Region != "" ||
		f.OSS.Endpoint != "" ||
		f.OSS.Bucket != "" ||
		f.OSS.AccessKeyID != "" ||
		f.OSS.AccessKeySecret != "" ||
		f.OSS.KeyPrefix != "" ||
		f.OSS.PresignTTL > 0 {
		copy := f.OSS
		oss = &copy
	}
	return raw{
		DashScope: dashScope{
			APIKey:  f.DashScope.APIKey,
			BaseURL: f.DashScope.BaseURL,
			Model:   f.DashScope.Model,
		},
		OSS: oss,
		ASR: f.ASR,
	}, nil
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

type FunASRConfigASR struct {
	ChannelIDs         []int         `yaml:"channel_ids" json:"channel_ids"`
	Language           string        `yaml:"language,omitempty" json:"language,omitempty"`
	VocabularyID       string        `yaml:"vocabulary_id,omitempty" json:"vocabulary_id,omitempty"`
	DiarizationEnabled *bool         `yaml:"diarization_enabled,omitempty" json:"diarization_enabled,omitempty"`
	SpeakerCount       int           `yaml:"speaker_count,omitempty" json:"speaker_count,omitempty"`
	PollInterval       time.Duration `yaml:"poll_interval" json:"poll_interval"`
	PollTimeout        time.Duration `yaml:"poll_timeout" json:"poll_timeout"`
	RequestTimeout     time.Duration `yaml:"request_timeout" json:"request_timeout"`
}

type ResolvedFunASRASR struct {
	ChannelIDs         []int         `json:"channel_ids"`
	Language           string        `json:"language,omitempty"`
	VocabularyID       string        `json:"vocabulary_id,omitempty"`
	DiarizationEnabled bool          `json:"diarization_enabled"`
	SpeakerCount       int           `json:"speaker_count,omitempty"`
	PollInterval       time.Duration `json:"poll_interval"`
	PollTimeout        time.Duration `json:"poll_timeout"`
	RequestTimeout     time.Duration `json:"request_timeout"`
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

func (a *FunASRConfigASR) UnmarshalYAML(value *yaml.Node) error {
	type raw struct {
		ChannelIDs         []int  `yaml:"channel_ids"`
		Language           string `yaml:"language"`
		VocabularyID       string `yaml:"vocabulary_id"`
		DiarizationEnabled *bool  `yaml:"diarization_enabled"`
		SpeakerCount       int    `yaml:"speaker_count"`
		PollInterval       string `yaml:"poll_interval"`
		PollTimeout        string `yaml:"poll_timeout"`
		RequestTimeout     string `yaml:"request_timeout"`
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
	if r.VocabularyID != "" {
		a.VocabularyID = r.VocabularyID
	}
	if r.DiarizationEnabled != nil {
		a.DiarizationEnabled = r.DiarizationEnabled
	}
	if r.SpeakerCount != 0 {
		a.SpeakerCount = r.SpeakerCount
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

func (a FunASRConfigASR) MarshalYAML() (any, error) {
	type raw struct {
		ChannelIDs         []int  `yaml:"channel_ids,omitempty"`
		Language           string `yaml:"language,omitempty"`
		VocabularyID       string `yaml:"vocabulary_id,omitempty"`
		DiarizationEnabled *bool  `yaml:"diarization_enabled,omitempty"`
		SpeakerCount       int    `yaml:"speaker_count,omitempty"`
		PollInterval       string `yaml:"poll_interval,omitempty"`
		PollTimeout        string `yaml:"poll_timeout,omitempty"`
		RequestTimeout     string `yaml:"request_timeout,omitempty"`
	}
	out := raw{
		ChannelIDs:         a.ChannelIDs,
		Language:           a.Language,
		VocabularyID:       a.VocabularyID,
		DiarizationEnabled: a.DiarizationEnabled,
		SpeakerCount:       a.SpeakerCount,
	}
	if a.PollInterval > 0 {
		out.PollInterval = a.PollInterval.String()
	}
	if a.PollTimeout > 0 {
		out.PollTimeout = a.PollTimeout.String()
	}
	if a.RequestTimeout > 0 {
		out.RequestTimeout = a.RequestTimeout.String()
	}
	return out, nil
}

func (a *MiMoASRConfig) UnmarshalYAML(value *yaml.Node) error {
	type raw struct {
		Language       string `yaml:"language"`
		RequestTimeout string `yaml:"request_timeout"`
	}
	var r raw
	if err := value.Decode(&r); err != nil {
		return err
	}
	if r.Language != "" {
		a.Language = r.Language
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

func (a MiMoASRConfig) MarshalYAML() (any, error) {
	return struct {
		Language       string `yaml:"language"`
		RequestTimeout string `yaml:"request_timeout"`
	}{
		Language:       a.Language,
		RequestTimeout: a.RequestTimeout.String(),
	}, nil
}

func (s *MiMoSegmentationConfig) UnmarshalYAML(value *yaml.Node) error {
	type raw struct {
		TargetDuration         string  `yaml:"target_duration"`
		MinDuration            string  `yaml:"min_duration"`
		MaxDuration            string  `yaml:"max_duration"`
		VADThreshold           float32 `yaml:"vad_threshold"`
		MinSilence             string  `yaml:"min_silence"`
		SpeechPad              string  `yaml:"speech_pad"`
		ModelPath              string  `yaml:"model_path"`
		ONNXRuntimeLibraryPath string  `yaml:"onnx_runtime_library_path"`
		TempDir                string  `yaml:"temp_dir"`
	}
	var r raw
	if err := value.Decode(&r); err != nil {
		return err
	}
	parse := func(name, value string) (time.Duration, error) {
		if value == "" {
			return 0, nil
		}
		duration, err := time.ParseDuration(value)
		if err != nil {
			return 0, fmt.Errorf("parse segmentation.%s: %w", name, err)
		}
		return duration, nil
	}
	if duration, err := parse("target_duration", r.TargetDuration); err != nil {
		return err
	} else if duration > 0 {
		s.TargetDuration = duration
	}
	if duration, err := parse("min_duration", r.MinDuration); err != nil {
		return err
	} else if duration > 0 {
		s.MinDuration = duration
	}
	if duration, err := parse("max_duration", r.MaxDuration); err != nil {
		return err
	} else if duration > 0 {
		s.MaxDuration = duration
	}
	if r.VADThreshold != 0 {
		s.VADThreshold = r.VADThreshold
	}
	if duration, err := parse("min_silence", r.MinSilence); err != nil {
		return err
	} else if duration > 0 {
		s.MinSilence = duration
	}
	if duration, err := parse("speech_pad", r.SpeechPad); err != nil {
		return err
	} else if duration > 0 {
		s.SpeechPad = duration
	}
	if r.ModelPath != "" {
		s.ModelPath = r.ModelPath
	}
	if r.ONNXRuntimeLibraryPath != "" {
		s.ONNXRuntimeLibraryPath = r.ONNXRuntimeLibraryPath
	}
	if r.TempDir != "" {
		s.TempDir = r.TempDir
	}
	return nil
}

func (s MiMoSegmentationConfig) MarshalYAML() (any, error) {
	return struct {
		TargetDuration         string  `yaml:"target_duration"`
		MinDuration            string  `yaml:"min_duration"`
		MaxDuration            string  `yaml:"max_duration"`
		VADThreshold           float32 `yaml:"vad_threshold"`
		MinSilence             string  `yaml:"min_silence"`
		SpeechPad              string  `yaml:"speech_pad"`
		ModelPath              string  `yaml:"model_path,omitempty"`
		ONNXRuntimeLibraryPath string  `yaml:"onnx_runtime_library_path,omitempty"`
		TempDir                string  `yaml:"temp_dir,omitempty"`
	}{
		TargetDuration:         s.TargetDuration.String(),
		MinDuration:            s.MinDuration.String(),
		MaxDuration:            s.MaxDuration.String(),
		VADThreshold:           s.VADThreshold,
		MinSilence:             s.MinSilence.String(),
		SpeechPad:              s.SpeechPad.String(),
		ModelPath:              s.ModelPath,
		ONNXRuntimeLibraryPath: s.ONNXRuntimeLibraryPath,
		TempDir:                s.TempDir,
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
	diarizationEnabled := true
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
			FunASR: FunASRConfig{
				DashScope: DashScopeConfig{
					Model: defaultFunModel,
				},
				ASR: FunASRConfigASR{
					DiarizationEnabled: &diarizationEnabled,
				},
			},
			MiMoV25ASR: MiMoConfig{
				MiMo: MiMoAPIConfig{
					BaseURL: defaultMimoBaseURL,
					Model:   defaultMimoModel,
				},
				ASR: MiMoASRConfig{
					Language:       "auto",
					RequestTimeout: 5 * time.Minute,
				},
				Segmentation: MiMoSegmentationConfig{
					TargetDuration: 3 * time.Minute,
					MinDuration:    150 * time.Second,
					MaxDuration:    210 * time.Second,
					VADThreshold:   0.5,
					MinSilence:     100 * time.Millisecond,
					SpeechPad:      30 * time.Millisecond,
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

func (c *Config) FunASR() *ResolvedFunASRConfig {
	qwen := c.Qwen3()
	fun := c.Engines.FunASR
	resolved := &ResolvedFunASRConfig{
		DashScope: qwen.DashScope,
		OSS:       qwen.OSS,
		ASR: ResolvedFunASRASR{
			ChannelIDs:         cloneInts(qwen.ASR.ChannelIDs),
			Language:           qwen.ASR.Language,
			DiarizationEnabled: true,
			PollInterval:       qwen.ASR.PollInterval,
			PollTimeout:        qwen.ASR.PollTimeout,
			RequestTimeout:     qwen.ASR.RequestTimeout,
		},
	}
	resolved.DashScope.Model = defaultFunModel
	if strings.TrimSpace(fun.DashScope.APIKey) != "" {
		resolved.DashScope.APIKey = fun.DashScope.APIKey
	}
	if strings.TrimSpace(fun.DashScope.BaseURL) != "" {
		resolved.DashScope.BaseURL = fun.DashScope.BaseURL
	}
	if strings.TrimSpace(fun.DashScope.Model) != "" {
		resolved.DashScope.Model = fun.DashScope.Model
	}
	if strings.TrimSpace(fun.OSS.Region) != "" {
		resolved.OSS.Region = fun.OSS.Region
	}
	if strings.TrimSpace(fun.OSS.Endpoint) != "" {
		resolved.OSS.Endpoint = fun.OSS.Endpoint
	}
	if strings.TrimSpace(fun.OSS.Bucket) != "" {
		resolved.OSS.Bucket = fun.OSS.Bucket
	}
	if strings.TrimSpace(fun.OSS.AccessKeyID) != "" {
		resolved.OSS.AccessKeyID = fun.OSS.AccessKeyID
	}
	if strings.TrimSpace(fun.OSS.AccessKeySecret) != "" {
		resolved.OSS.AccessKeySecret = fun.OSS.AccessKeySecret
	}
	if strings.TrimSpace(fun.OSS.KeyPrefix) != "" {
		resolved.OSS.KeyPrefix = fun.OSS.KeyPrefix
	}
	if fun.OSS.PresignTTL > 0 {
		resolved.OSS.PresignTTL = fun.OSS.PresignTTL
	}
	if len(fun.ASR.ChannelIDs) > 0 {
		resolved.ASR.ChannelIDs = cloneInts(fun.ASR.ChannelIDs)
	}
	if strings.TrimSpace(fun.ASR.Language) != "" {
		resolved.ASR.Language = fun.ASR.Language
	}
	if strings.TrimSpace(fun.ASR.VocabularyID) != "" {
		resolved.ASR.VocabularyID = fun.ASR.VocabularyID
	}
	if fun.ASR.DiarizationEnabled != nil {
		resolved.ASR.DiarizationEnabled = *fun.ASR.DiarizationEnabled
	}
	if fun.ASR.SpeakerCount != 0 {
		resolved.ASR.SpeakerCount = fun.ASR.SpeakerCount
	}
	if fun.ASR.PollInterval > 0 {
		resolved.ASR.PollInterval = fun.ASR.PollInterval
	}
	if fun.ASR.PollTimeout > 0 {
		resolved.ASR.PollTimeout = fun.ASR.PollTimeout
	}
	if fun.ASR.RequestTimeout > 0 {
		resolved.ASR.RequestTimeout = fun.ASR.RequestTimeout
	}
	return resolved
}

func (c *Config) MiMoV25ASR() *MiMoConfig {
	return &c.Engines.MiMoV25ASR
}

func (c *Config) Validate() error {
	if strings.TrimSpace(c.Engine) == "" {
		return UsageError{Message: "engine is required"}
	}
	if c.Engine != EngineQwen3Filetrans && c.Engine != EngineFunASR && c.Engine != EngineMimoV25ASR {
		return UsageError{Message: fmt.Sprintf("engine %q is not implemented", c.Engine)}
	}
	if c.Engine == EngineMimoV25ASR {
		return validateMiMo(c.MiMoV25ASR())
	}
	if c.Engine == EngineFunASR {
		fun := c.FunASR()
		if err := validateDashScopeStorage(fun.DashScope, fun.OSS); err != nil {
			return err
		}
		return validateFunASR(fun)
	}
	qwen := c.Qwen3()
	return validateDashScopeStorage(qwen.DashScope, qwen.OSS)
}

func validateMiMo(mimo *MiMoConfig) error {
	missing := []string{}
	check := func(name, value string) {
		if strings.TrimSpace(value) == "" {
			missing = append(missing, name)
		}
	}
	check("mimo.api_key", mimo.MiMo.APIKey)
	check("mimo.base_url", mimo.MiMo.BaseURL)
	check("mimo.model", mimo.MiMo.Model)
	if len(missing) > 0 {
		return UsageError{Message: "missing required config: " + strings.Join(missing, ", ")}
	}
	switch mimo.ASR.Language {
	case "", "auto", "zh", "en":
	default:
		return UsageError{Message: "mimo_v2_5_asr.asr.language must be one of auto, zh, en"}
	}
	if mimo.ASR.RequestTimeout <= 0 {
		return UsageError{Message: "mimo_v2_5_asr.asr.request_timeout must be positive"}
	}
	if mimo.Segmentation.TargetDuration <= 0 || mimo.Segmentation.MinDuration <= 0 || mimo.Segmentation.MaxDuration <= 0 {
		return UsageError{Message: "mimo_v2_5_asr.segmentation durations must be positive"}
	}
	if mimo.Segmentation.MinDuration > mimo.Segmentation.TargetDuration || mimo.Segmentation.TargetDuration > mimo.Segmentation.MaxDuration {
		return UsageError{Message: "mimo_v2_5_asr.segmentation must satisfy min_duration <= target_duration <= max_duration"}
	}
	if mimo.Segmentation.VADThreshold <= 0 || mimo.Segmentation.VADThreshold >= 1 {
		return UsageError{Message: "mimo_v2_5_asr.segmentation.vad_threshold must be between 0 and 1"}
	}
	return nil
}

func validateFunASR(fun *ResolvedFunASRConfig) error {
	if fun.ASR.SpeakerCount != 0 && (fun.ASR.SpeakerCount < 2 || fun.ASR.SpeakerCount > 100) {
		return UsageError{Message: "fun_asr.asr.speaker_count must be between 2 and 100"}
	}
	if fun.ASR.SpeakerCount != 0 && !fun.ASR.DiarizationEnabled {
		return UsageError{Message: "fun_asr.asr.speaker_count requires diarization_enabled"}
	}
	return nil
}

func validateDashScopeStorage(dashScope DashScopeConfig, oss OSSConfig) error {
	missing := []string{}
	check := func(name, value string) {
		if strings.TrimSpace(value) == "" {
			missing = append(missing, name)
		}
	}
	check("dashscope.api_key", dashScope.APIKey)
	check("dashscope.base_url", dashScope.BaseURL)
	check("dashscope.model", dashScope.Model)
	check("oss.region", oss.Region)
	check("oss.endpoint", oss.Endpoint)
	check("oss.bucket", oss.Bucket)
	check("oss.access_key_id", oss.AccessKeyID)
	check("oss.access_key_secret", oss.AccessKeySecret)
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
	copy.Engines.FunASR.DashScope.APIKey = redactSecret(copy.Engines.FunASR.DashScope.APIKey)
	copy.Engines.FunASR.OSS.AccessKeyID = redactID(copy.Engines.FunASR.OSS.AccessKeyID)
	copy.Engines.FunASR.OSS.AccessKeySecret = redactSecret(copy.Engines.FunASR.OSS.AccessKeySecret)
	copy.Engines.MiMoV25ASR.MiMo.APIKey = redactSecret(copy.Engines.MiMoV25ASR.MiMo.APIKey)
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
	mimo := c.MiMoV25ASR()
	if value := firstEnv("EASY_ASR_MIMO_API_KEY", "MIMO_API_KEY"); value != "" {
		mimo.MiMo.APIKey = value
	}
	if value := firstEnv("EASY_ASR_MIMO_BASE_URL"); value != "" {
		mimo.MiMo.BaseURL = value
	}
	if value := firstEnv("EASY_ASR_MIMO_MODEL"); value != "" {
		mimo.MiMo.Model = value
	}
	if value := firstEnv("EASY_ASR_SILERO_VAD_MODEL"); value != "" {
		mimo.Segmentation.ModelPath = value
	}
	if value := firstEnv("EASY_ASR_ONNXRUNTIME_LIBRARY"); value != "" {
		mimo.Segmentation.ONNXRuntimeLibraryPath = value
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

func cloneInts(values []int) []int {
	if len(values) == 0 {
		return nil
	}
	out := make([]int, len(values))
	copy(out, values)
	return out
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
