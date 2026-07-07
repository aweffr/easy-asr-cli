package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/aweffr/easy-asr-cli/internal/config"
	"github.com/aweffr/easy-asr-cli/internal/engine"
	"github.com/aweffr/easy-asr-cli/internal/funasr"
	"github.com/aweffr/easy-asr-cli/internal/qwen3"
)

type Deps struct {
	Stdout            io.Writer
	Stderr            io.Writer
	DefaultConfigPath func() (string, error)
	LoadConfig        func(path string) (*config.Config, error)
	Registry          func(cfg *config.Config) *engine.Registry
}

func NewRootCommand(deps Deps) *cobra.Command {
	deps = fillDeps(deps)
	state := rootState{deps: deps}
	cmd := &cobra.Command{
		Use:           "easy_asr",
		Short:         "Transcribe local audio files with ASR engines",
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	cmd.SetOut(deps.Stdout)
	cmd.SetErr(deps.Stderr)
	cmd.PersistentFlags().StringVar(&state.configPath, "config", "", "config file path")
	cmd.AddCommand(newTranscribeCommand(&state))
	cmd.AddCommand(newEnginesCommand(&state))
	cmd.AddCommand(newConfigCommand(&state))
	cmd.AddCommand(newSchemaCommand(&state))
	return cmd
}

type rootState struct {
	deps       Deps
	configPath string
}

func fillDeps(deps Deps) Deps {
	if deps.Stdout == nil {
		deps.Stdout = os.Stdout
	}
	if deps.Stderr == nil {
		deps.Stderr = os.Stderr
	}
	if deps.DefaultConfigPath == nil {
		deps.DefaultConfigPath = config.DefaultPath
	}
	if deps.LoadConfig == nil {
		deps.LoadConfig = config.Load
	}
	if deps.Registry == nil {
		deps.Registry = func(cfg *config.Config) *engine.Registry {
			return engine.DefaultRegistry(
				qwen3.NewEngine(qwen3.Options{Config: cfg.Qwen3()}),
				funasr.NewEngine(funasr.Options{Config: cfg.FunASR()}),
			)
		}
	}
	return deps
}

func (s *rootState) resolveConfigPath() (string, error) {
	if strings.TrimSpace(s.configPath) != "" {
		return s.configPath, nil
	}
	return s.deps.DefaultConfigPath()
}

func (s *rootState) loadConfig() (*config.Config, string, error) {
	path, err := s.resolveConfigPath()
	if err != nil {
		return nil, "", err
	}
	cfg, err := s.deps.LoadConfig(path)
	if err != nil {
		return nil, "", err
	}
	return cfg, path, nil
}

func newTranscribeCommand(state *rootState) *cobra.Command {
	opts := transcribeOptions{}
	cmd := &cobra.Command{
		Use:   "transcribe <audio-file>",
		Short: "Transcribe an audio file and write SRT output",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, _, err := state.loadConfig()
			if err != nil {
				return err
			}
			engineName := firstNonEmpty(opts.engine, cfg.Engine)
			if engineName == "" {
				engineName = config.EngineQwen3Filetrans
			}
			cfg.Engine = engineName
			if err := applyTimingFlagOverrides(cmd, cfg, opts); err != nil {
				return err
			}
			if err := validateEngineFlags(cmd, cfg, engineName, opts); err != nil {
				return err
			}
			if err := cfg.Validate(); err != nil {
				return err
			}
			registry := state.deps.Registry(cfg)
			runner, err := registry.Get(engineName)
			if err != nil {
				return err
			}
			audioPath := args[0]
			if _, err := os.Stat(audioPath); err != nil {
				return fmt.Errorf("audio file is not readable: %w", err)
			}
			outputPath := opts.outputPath
			tempOutput := ""
			if outputPath == "" {
				outputPath = defaultSRTPath(audioPath)
			}
			outputPath, err = filepath.Abs(outputPath)
			if err != nil {
				return err
			}
			if opts.stdout {
				file, err := os.CreateTemp("", "easy_asr-*.srt")
				if err != nil {
					return err
				}
				tempOutput = file.Name()
				_ = file.Close()
				outputPath = tempOutput
				defer os.Remove(tempOutput)
			}
			request := engine.Request{
				AudioPath:    audioPath,
				OutputPath:   outputPath,
				RawJSONPath:  opts.rawJSONPath,
				KeepObject:   opts.keepObject,
				Language:     opts.language,
				Hotwords:     opts.hotwords,
				Channels:     opts.channels,
				VocabularyID: opts.vocabularyID,
				SpeakerCount: opts.speakerCount,
			}
			qwenCfg := cfg.Qwen3()
			request.EnableITN = qwenCfg.ASR.EnableITN
			request.EnableWords = qwenCfg.ASR.EnableWords
			if cmd.Flags().Changed("enable-itn") {
				request.EnableITN = opts.enableITN
			}
			if cmd.Flags().Changed("enable-words") {
				request.EnableWords = opts.enableWords
			}
			if opts.noEnableWords {
				request.EnableWords = false
			}
			if opts.hotwordsFile != "" {
				body, err := os.ReadFile(opts.hotwordsFile)
				if err != nil {
					return err
				}
				if request.Hotwords != "" {
					request.Hotwords += "\n"
				}
				request.Hotwords += strings.TrimSpace(string(body))
			}
			if engineName == config.EngineFunASR {
				funCfg := cfg.FunASR()
				request.DiarizationEnabled = funCfg.ASR.DiarizationEnabled
				if opts.noDiarization {
					request.DiarizationEnabled = false
					request.SpeakerCount = 0
				} else if request.SpeakerCount == 0 {
					request.SpeakerCount = funCfg.ASR.SpeakerCount
				}
			}
			result, err := runner.Transcribe(context.Background(), request)
			if err != nil {
				return err
			}
			if opts.stdout {
				body, err := os.ReadFile(tempOutput)
				if err != nil {
					return err
				}
				_, err = state.deps.Stdout.Write(body)
				return err
			}
			if opts.jsonOutput {
				return writeJSON(state.deps.Stdout, result)
			}
			fmt.Fprintln(state.deps.Stdout, result.OutputPath)
			return nil
		},
	}
	cmd.Flags().StringVar(&opts.engine, "engine", "", "ASR engine")
	cmd.Flags().StringVarP(&opts.outputPath, "output", "o", "", "output SRT path")
	cmd.Flags().BoolVar(&opts.stdout, "stdout", false, "write SRT content to stdout")
	cmd.Flags().BoolVar(&opts.jsonOutput, "json", false, "write JSON run result to stdout")
	cmd.Flags().StringVar(&opts.rawJSONPath, "raw-json", "", "save raw transcription JSON to path")
	cmd.Flags().StringVar(&opts.language, "language", "", "audio language hint")
	cmd.Flags().StringVar(&opts.hotwords, "hotwords", "", "hotword context text")
	cmd.Flags().StringVar(&opts.hotwordsFile, "hotwords-file", "", "hotword context file")
	cmd.Flags().StringVar(&opts.vocabularyID, "vocabulary-id", "", "Fun-ASR vocabulary id")
	cmd.Flags().BoolVar(&opts.noDiarization, "no-diarization", false, "disable Fun-ASR speaker diarization")
	cmd.Flags().IntVar(&opts.speakerCount, "speaker-count", 0, "Fun-ASR speaker count hint")
	cmd.Flags().BoolVar(&opts.enableITN, "enable-itn", false, "enable inverse text normalization")
	cmd.Flags().BoolVar(&opts.enableWords, "enable-words", true, "enable word timestamps")
	cmd.Flags().BoolVar(&opts.noEnableWords, "no-enable-words", false, "disable word timestamps")
	cmd.Flags().IntSliceVar(&opts.channels, "channel", nil, "audio channel index; repeatable")
	cmd.Flags().DurationVar(&opts.pollInterval, "poll-interval", 5*time.Second, "ASR task polling interval")
	cmd.Flags().DurationVar(&opts.pollTimeout, "poll-timeout", 2*time.Hour, "ASR task polling timeout")
	cmd.Flags().DurationVar(&opts.requestTimeout, "request-timeout", 30*time.Second, "HTTP request timeout")
	cmd.Flags().BoolVar(&opts.keepObject, "keep-object", false, "keep temporary OSS object")
	return cmd
}

type transcribeOptions struct {
	engine         string
	outputPath     string
	stdout         bool
	jsonOutput     bool
	rawJSONPath    string
	language       string
	hotwords       string
	hotwordsFile   string
	vocabularyID   string
	noDiarization  bool
	speakerCount   int
	enableITN      bool
	enableWords    bool
	noEnableWords  bool
	channels       []int
	pollInterval   time.Duration
	pollTimeout    time.Duration
	requestTimeout time.Duration
	keepObject     bool
}

func newEnginesCommand(state *rootState) *cobra.Command {
	var jsonOutput bool
	cmd := &cobra.Command{
		Use:   "engines",
		Short: "List ASR engines",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, _, err := state.loadConfig()
			if err != nil {
				return err
			}
			infos := state.deps.Registry(cfg).List()
			if jsonOutput {
				return writeJSON(state.deps.Stdout, infos)
			}
			for _, info := range infos {
				status := "reserved"
				if info.Implemented {
					status = "implemented"
				}
				if info.Default {
					status += ", default"
				}
				fmt.Fprintf(state.deps.Stdout, "%s\t%s\t%s\n", info.Name, status, info.Description)
			}
			return nil
		},
	}
	cmd.Flags().BoolVar(&jsonOutput, "json", false, "write JSON to stdout")
	return cmd
}

func newConfigCommand(state *rootState) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "config",
		Short: "Manage easy_asr config",
	}
	cmd.AddCommand(&cobra.Command{
		Use:   "path",
		Short: "Print config path",
		RunE: func(cmd *cobra.Command, args []string) error {
			path, err := state.resolveConfigPath()
			if err != nil {
				return err
			}
			fmt.Fprintln(state.deps.Stdout, path)
			return nil
		},
	})
	cmd.AddCommand(&cobra.Command{
		Use:   "validate",
		Short: "Validate config",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, _, err := state.loadConfig()
			if err != nil {
				return err
			}
			if err := cfg.Validate(); err != nil {
				return err
			}
			fmt.Fprintln(state.deps.Stdout, "ok")
			return nil
		},
	})
	cmd.AddCommand(&cobra.Command{
		Use:   "init",
		Short: "Create a default config file",
		RunE: func(cmd *cobra.Command, args []string) error {
			path, err := state.resolveConfigPath()
			if err != nil {
				return err
			}
			if err := config.WriteDefault(path); err != nil {
				return err
			}
			fmt.Fprintln(state.deps.Stdout, path)
			return nil
		},
	})
	return cmd
}

func newSchemaCommand(state *rootState) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "schema",
		Short: "Print machine-readable schemas",
	}
	cmd.AddCommand(&cobra.Command{
		Use:   "run-result",
		Short: "Print run result JSON Schema",
		RunE: func(cmd *cobra.Command, args []string) error {
			return writeJSON(state.deps.Stdout, map[string]any{
				"$schema": "https://json-schema.org/draft/2020-12/schema",
				"type":    "object",
				"properties": map[string]any{
					"engine":            map[string]string{"type": "string"},
					"task_id":           map[string]string{"type": "string"},
					"output_path":       map[string]string{"type": "string"},
					"raw_json_path":     map[string]string{"type": "string"},
					"object_key":        map[string]string{"type": "string"},
					"transcription_url": map[string]string{"type": "string"},
					"usage_seconds":     map[string]string{"type": "integer"},
					"cleanup_error":     map[string]string{"type": "string"},
				},
			})
		},
	})
	return cmd
}

func defaultSRTPath(audioPath string) string {
	ext := filepath.Ext(audioPath)
	if ext == "" {
		return audioPath + ".srt"
	}
	return strings.TrimSuffix(audioPath, ext) + ".srt"
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

func applyTimingFlagOverrides(cmd *cobra.Command, cfg *config.Config, opts transcribeOptions) error {
	qwenCfg := cfg.Qwen3()
	if cmd.Flags().Changed("poll-interval") {
		qwenCfg.ASR.PollInterval = opts.pollInterval
		cfg.Engines.FunASR.ASR.PollInterval = opts.pollInterval
	}
	if cmd.Flags().Changed("poll-timeout") {
		qwenCfg.ASR.PollTimeout = opts.pollTimeout
		cfg.Engines.FunASR.ASR.PollTimeout = opts.pollTimeout
	}
	if cmd.Flags().Changed("request-timeout") {
		qwenCfg.ASR.RequestTimeout = opts.requestTimeout
		cfg.Engines.FunASR.ASR.RequestTimeout = opts.requestTimeout
	}
	return nil
}

func validateEngineFlags(cmd *cobra.Command, cfg *config.Config, engineName string, opts transcribeOptions) error {
	if engineName != config.EngineFunASR {
		return nil
	}
	if strings.TrimSpace(opts.hotwords) != "" {
		return engine.UsageError{Message: "fun-asr does not support --hotwords; use --vocabulary-id"}
	}
	if strings.TrimSpace(opts.hotwordsFile) != "" {
		return engine.UsageError{Message: "fun-asr does not support --hotwords-file; use --vocabulary-id"}
	}
	for _, name := range []string{"enable-itn", "enable-words", "no-enable-words"} {
		if cmd.Flags().Changed(name) {
			return engine.UsageError{Message: fmt.Sprintf("fun-asr does not support --%s", name)}
		}
	}
	if opts.speakerCount != 0 && (opts.speakerCount < 2 || opts.speakerCount > 100) {
		return engine.UsageError{Message: "--speaker-count must be between 2 and 100"}
	}
	funCfg := cfg.FunASR()
	diarizationEnabled := funCfg.ASR.DiarizationEnabled && !opts.noDiarization
	if opts.speakerCount != 0 && !diarizationEnabled {
		return engine.UsageError{Message: "--speaker-count requires Fun-ASR diarization"}
	}
	channels := opts.channels
	if len(channels) == 0 {
		channels = funCfg.ASR.ChannelIDs
	}
	if diarizationEnabled && len(channels) > 1 {
		return engine.UsageError{Message: "fun-asr diarization supports only one channel; use one --channel or --no-diarization"}
	}
	return nil
}

func writeJSON(writer io.Writer, value any) error {
	encoder := json.NewEncoder(writer)
	encoder.SetIndent("", "  ")
	return encoder.Encode(value)
}
