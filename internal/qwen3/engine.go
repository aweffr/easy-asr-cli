package qwen3

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/aweffr/easy-asr-cli/internal/config"
	"github.com/aweffr/easy-asr-cli/internal/dashscope"
	"github.com/aweffr/easy-asr-cli/internal/engine"
	"github.com/aweffr/easy-asr-cli/internal/redact"
	"github.com/aweffr/easy-asr-cli/internal/srt"
	"github.com/aweffr/easy-asr-cli/internal/storage"
)

type Storage interface {
	Upload(ctx context.Context, path string) (storage.StoredObject, error)
	PresignGet(ctx context.Context, key string) (string, error)
	Delete(ctx context.Context, key string) error
}

type ASRClient interface {
	SubmitTask(ctx context.Context, request dashscope.SubmitRequest) (string, error)
	WaitTask(ctx context.Context, taskID string, timeout time.Duration) (dashscope.TaskResult, error)
	DownloadTranscription(ctx context.Context, url string) (srt.Transcription, error)
}

type Options struct {
	Config  *config.Qwen3Config
	Storage Storage
	ASR     ASRClient
}

type Engine struct {
	cfg     *config.Qwen3Config
	storage Storage
	asr     ASRClient
}

func NewEngine(options Options) *Engine {
	return &Engine{
		cfg:     options.Config,
		storage: options.Storage,
		asr:     options.ASR,
	}
}

func (e *Engine) Transcribe(ctx context.Context, request engine.Request) (result engine.Result, err error) {
	if e.cfg == nil {
		return engine.Result{}, fmt.Errorf("qwen3 config is required")
	}
	if e.storage == nil {
		storageClient, err := storage.NewClientFromConfig(e.cfg.OSS)
		if err != nil {
			return engine.Result{}, err
		}
		e.storage = storageClient
	}
	if e.asr == nil {
		e.asr = dashscope.NewClient(dashscope.Options{
			APIKey:         e.cfg.DashScope.APIKey,
			BaseURL:        e.cfg.DashScope.BaseURL,
			Model:          e.cfg.DashScope.Model,
			RequestTimeout: e.cfg.ASR.RequestTimeout,
			PollInterval:   e.cfg.ASR.PollInterval,
		})
	}
	if request.OutputPath == "" {
		return engine.Result{}, fmt.Errorf("output path is required")
	}

	stored, err := e.storage.Upload(ctx, request.AudioPath)
	if err != nil {
		return engine.Result{}, err
	}
	result = engine.Result{
		Engine:      config.EngineQwen3Filetrans,
		OutputPath:  request.OutputPath,
		RawJSONPath: request.RawJSONPath,
		ObjectKey:   stored.Key,
	}
	cleanup := func() {
		if request.KeepObject {
			return
		}
		if err := e.storage.Delete(ctx, stored.Key); err != nil {
			result.CleanupError = err.Error()
		}
	}
	defer cleanup()

	audioURL, err := e.storage.PresignGet(ctx, stored.Key)
	if err != nil {
		return result, err
	}
	taskID, err := e.asr.SubmitTask(ctx, dashscope.SubmitRequest{
		FileURL:     audioURL,
		ChannelIDs:  firstNonEmptyInts(request.Channels, e.cfg.ASR.ChannelIDs),
		EnableITN:   request.EnableITN,
		EnableWords: request.EnableWords,
		Language:    firstNonEmptyString(request.Language, e.cfg.ASR.Language),
		Hotwords:    request.Hotwords,
	})
	if err != nil {
		return result, err
	}
	result.TaskID = taskID
	taskResult, err := e.asr.WaitTask(ctx, taskID, e.cfg.ASR.PollTimeout)
	if err != nil {
		return result, err
	}
	result.TaskID = firstNonEmptyString(taskResult.TaskID, taskID)
	result.TranscriptionURL = redactSignedURL(taskResult.TranscriptionURL)
	result.UsageSeconds = taskResult.UsageSeconds

	transcription, err := e.asr.DownloadTranscription(ctx, taskResult.TranscriptionURL)
	if err != nil {
		return result, err
	}
	if request.RawJSONPath != "" {
		if err := writeJSON(request.RawJSONPath, transcription); err != nil {
			return result, err
		}
	}
	rendered, err := srt.Render(transcription, srt.Options{})
	if err != nil {
		return result, err
	}
	if err := os.MkdirAll(filepath.Dir(request.OutputPath), 0o755); err != nil {
		return result, err
	}
	if err := os.WriteFile(request.OutputPath, []byte(rendered), 0o644); err != nil {
		return result, err
	}
	return result, nil
}

func writeJSON(path string, value any) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	body, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	body = append(body, '\n')
	return os.WriteFile(path, body, 0o644)
}

func firstNonEmptyString(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}

func firstNonEmptyInts(values ...[]int) []int {
	for _, value := range values {
		if len(value) > 0 {
			out := make([]int, len(value))
			copy(out, value)
			return out
		}
	}
	return nil
}

func redactSignedURL(value string) string {
	return redact.URLQueries(value)
}
