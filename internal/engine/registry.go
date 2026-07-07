package engine

import (
	"context"
	"errors"
	"fmt"

	"github.com/aweffr/easy-asr-cli/internal/observe"
)

var ErrNotImplemented = errors.New("engine not implemented")

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

type Request struct {
	AudioPath          string
	OutputPath         string
	RawJSONPath        string
	KeepObject         bool
	Language           string
	Hotwords           string
	Channels           []int
	EnableITN          bool
	EnableWords        bool
	VocabularyID       string
	DiarizationEnabled bool
	SpeakerCount       int
	Observer           observe.Observer
}

type Result struct {
	Engine           string   `json:"engine"`
	TaskID           string   `json:"task_id,omitempty"`
	OutputPath       string   `json:"output_path,omitempty"`
	RawJSONPath      string   `json:"raw_json_path,omitempty"`
	ObjectKey        string   `json:"object_key,omitempty"`
	TranscriptionURL string   `json:"transcription_url,omitempty"`
	UsageSeconds     int64    `json:"usage_seconds,omitempty"`
	CleanupError     string   `json:"cleanup_error,omitempty"`
	Warnings         []string `json:"warnings,omitempty"`
}

type Runner interface {
	Transcribe(ctx context.Context, request Request) (Result, error)
}

type RunnerFunc func(ctx context.Context, request Request) (Result, error)

func (f RunnerFunc) Transcribe(ctx context.Context, request Request) (Result, error) {
	return f(ctx, request)
}

type Info struct {
	Name                     string  `json:"name"`
	Implemented              bool    `json:"implemented"`
	Default                  bool    `json:"default"`
	Description              string  `json:"description"`
	ReferencePriceCNYPerHour float64 `json:"reference_price_cny_per_hour"`
}

type Registry struct {
	defaultName string
	infos       []Info
	runners     map[string]Runner
}

func DefaultRegistry(qwen Runner, runners ...Runner) *Registry {
	if qwen == nil {
		qwen = RunnerFunc(func(context.Context, Request) (Result, error) {
			return Result{}, nil
		})
	}
	funRunner := notImplementedRunner("fun-asr")
	if len(runners) > 0 && runners[0] != nil {
		funRunner = runners[0]
	}
	mimoRunner := notImplementedRunner("mimo-v2.5-asr")
	if len(runners) > 1 && runners[1] != nil {
		mimoRunner = runners[1]
	}
	infos := []Info{
		{
			Name:                     "qwen3-asr-flash-filetrans",
			Implemented:              true,
			Default:                  true,
			Description:              "Aliyun DashScope Qwen3 async file transcription (~¥0.79/hour)",
			ReferencePriceCNYPerHour: 0.79,
		},
		{
			Name:                     "fun-asr",
			Implemented:              true,
			Description:              "Aliyun DashScope Fun-ASR async file transcription (~¥0.79/hour)",
			ReferencePriceCNYPerHour: 0.79,
		},
		{
			Name:                     "mimo-v2.5-asr",
			Implemented:              true,
			Description:              "Xiaomi MiMo V2.5 ASR with local VAD segmentation (~¥0.50/hour)",
			ReferencePriceCNYPerHour: 0.50,
		},
	}
	return &Registry{
		defaultName: infos[0].Name,
		infos:       infos,
		runners: map[string]Runner{
			infos[0].Name: qwen,
			infos[1].Name: funRunner,
			infos[2].Name: mimoRunner,
		},
	}
}

func (r *Registry) DefaultName() string {
	return r.defaultName
}

func (r *Registry) List() []Info {
	out := make([]Info, len(r.infos))
	copy(out, r.infos)
	return out
}

func (r *Registry) Get(name string) (Runner, error) {
	runner, ok := r.runners[name]
	if !ok {
		return nil, UsageError{Message: fmt.Sprintf("unknown engine %q", name)}
	}
	return runner, nil
}

func notImplementedRunner(name string) Runner {
	return RunnerFunc(func(context.Context, Request) (Result, error) {
		return Result{}, fmt.Errorf("%s: %w", name, ErrNotImplemented)
	})
}
