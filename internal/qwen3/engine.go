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
	"github.com/aweffr/easy-asr-cli/internal/observe"
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
	observer := request.Observer
	runStart := time.Now()
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

	result = engine.Result{
		Engine:      config.EngineQwen3Filetrans,
		OutputPath:  request.OutputPath,
		RawJSONPath: request.RawJSONPath,
	}
	emit(observer, observe.Event{Event: "asr.run.started", Engine: config.EngineQwen3Filetrans, Step: "run", Message: "transcription started"})
	defer func() {
		event := observe.Event{Event: "asr.run.completed", Engine: config.EngineQwen3Filetrans, Step: "run", ElapsedMS: elapsedMS(runStart), UsageSeconds: result.UsageSeconds, Message: "transcription completed"}
		if err != nil {
			event.Event = "asr.run.failed"
			event.Level = "error"
			event.Error = err.Error()
			event.ErrorType = fmt.Sprintf("%T", err)
			event.Message = "transcription failed"
		}
		emit(observer, event)
	}()
	uploadStart := time.Now()
	emit(observer, observe.Event{Event: "storage.upload.started", Engine: config.EngineQwen3Filetrans, Step: "upload", Message: "uploading audio object"})
	stored, err := e.storage.Upload(ctx, request.AudioPath)
	if err != nil {
		emitFailed(observer, config.EngineQwen3Filetrans, "storage.upload.failed", "upload", uploadStart, err)
		return result, err
	}
	result.ObjectKey = stored.Key
	emit(observer, observe.Event{Event: "storage.upload.completed", Engine: config.EngineQwen3Filetrans, Step: "upload", ElapsedMS: elapsedMS(uploadStart), Message: "audio object uploaded"})
	cleanup := func() {
		if request.KeepObject {
			emit(observer, observe.Event{Event: "cleanup.skipped", Engine: config.EngineQwen3Filetrans, Step: "cleanup", Message: "temporary object kept"})
			return
		}
		cleanupStart := time.Now()
		emit(observer, observe.Event{Event: "cleanup.started", Engine: config.EngineQwen3Filetrans, Step: "cleanup", Message: "deleting temporary object"})
		if err := e.storage.Delete(ctx, stored.Key); err != nil {
			result.CleanupError = err.Error()
			emitFailed(observer, config.EngineQwen3Filetrans, "cleanup.failed", "cleanup", cleanupStart, err)
			return
		}
		emit(observer, observe.Event{Event: "cleanup.completed", Engine: config.EngineQwen3Filetrans, Step: "cleanup", ElapsedMS: elapsedMS(cleanupStart), Message: "temporary object deleted"})
	}
	defer cleanup()

	presignStart := time.Now()
	emit(observer, observe.Event{Event: "storage.presign.started", Engine: config.EngineQwen3Filetrans, Step: "presign", Message: "signing audio object URL"})
	audioURL, err := e.storage.PresignGet(ctx, stored.Key)
	if err != nil {
		emitFailed(observer, config.EngineQwen3Filetrans, "storage.presign.failed", "presign", presignStart, err)
		return result, err
	}
	emit(observer, observe.Event{Event: "storage.presign.completed", Engine: config.EngineQwen3Filetrans, Step: "presign", ElapsedMS: elapsedMS(presignStart), Message: "audio object URL signed"})
	submitStart := time.Now()
	emit(observer, observe.Event{Event: "dashscope.submit.started", Engine: config.EngineQwen3Filetrans, Step: "submit", Message: "submitting DashScope task"})
	taskID, err := e.asr.SubmitTask(ctx, dashscope.SubmitRequest{
		FileURL:     audioURL,
		ChannelIDs:  firstNonEmptyInts(request.Channels, e.cfg.ASR.ChannelIDs),
		EnableITN:   request.EnableITN,
		EnableWords: request.EnableWords,
		Language:    firstNonEmptyString(request.Language, e.cfg.ASR.Language),
		Hotwords:    request.Hotwords,
	})
	if err != nil {
		emitFailed(observer, config.EngineQwen3Filetrans, "dashscope.submit.failed", "submit", submitStart, err)
		return result, err
	}
	result.TaskID = taskID
	emit(observer, observe.Event{Event: "dashscope.submit.completed", Engine: config.EngineQwen3Filetrans, Step: "submit", ElapsedMS: elapsedMS(submitStart), Message: "DashScope task submitted"})
	pollStart := time.Now()
	emit(observer, observe.Event{Event: "dashscope.poll.started", Engine: config.EngineQwen3Filetrans, Step: "poll", Message: "waiting for DashScope task"})
	taskResult, err := e.asr.WaitTask(ctx, taskID, e.cfg.ASR.PollTimeout)
	if err != nil {
		emitFailed(observer, config.EngineQwen3Filetrans, "dashscope.poll.failed", "poll", pollStart, err)
		return result, err
	}
	result.TaskID = firstNonEmptyString(taskResult.TaskID, taskID)
	result.TranscriptionURL = redactSignedURL(taskResult.TranscriptionURL)
	result.UsageSeconds = taskResult.UsageSeconds
	emit(observer, observe.Event{Event: "dashscope.poll.completed", Engine: config.EngineQwen3Filetrans, Step: "poll", ElapsedMS: elapsedMS(pollStart), UsageSeconds: result.UsageSeconds, Message: "DashScope task succeeded"})

	downloadStart := time.Now()
	emit(observer, observe.Event{Event: "transcription.download.started", Engine: config.EngineQwen3Filetrans, Step: "download", Message: "downloading transcription JSON"})
	transcription, err := e.asr.DownloadTranscription(ctx, taskResult.TranscriptionURL)
	if err != nil {
		emitFailed(observer, config.EngineQwen3Filetrans, "transcription.download.failed", "download", downloadStart, err)
		return result, err
	}
	emit(observer, observe.Event{Event: "transcription.download.completed", Engine: config.EngineQwen3Filetrans, Step: "download", ElapsedMS: elapsedMS(downloadStart), Message: "transcription JSON downloaded"})
	if request.RawJSONPath != "" {
		if err := writeJSON(request.RawJSONPath, transcription); err != nil {
			return result, err
		}
	}
	renderStart := time.Now()
	emit(observer, observe.Event{Event: "srt.render.started", Engine: config.EngineQwen3Filetrans, Step: "render", Message: "rendering SRT"})
	rendered, err := srt.Render(transcription, srt.Options{})
	if err != nil {
		emitFailed(observer, config.EngineQwen3Filetrans, "srt.render.failed", "render", renderStart, err)
		return result, err
	}
	if err := os.MkdirAll(filepath.Dir(request.OutputPath), 0o755); err != nil {
		return result, err
	}
	if err := os.WriteFile(request.OutputPath, []byte(rendered), 0o644); err != nil {
		return result, err
	}
	emit(observer, observe.Event{Event: "srt.render.completed", Engine: config.EngineQwen3Filetrans, Step: "render", ElapsedMS: elapsedMS(renderStart), Message: "SRT output written"})
	return result, nil
}

func emit(observer observe.Observer, event observe.Event) {
	if observer != nil {
		observer.Emit(event)
	}
}

func emitFailed(observer observe.Observer, engineName string, name string, step string, start time.Time, err error) {
	emit(observer, observe.Event{
		Event:     name,
		Level:     "error",
		Engine:    engineName,
		Step:      step,
		ElapsedMS: elapsedMS(start),
		Error:     err.Error(),
		ErrorType: fmt.Sprintf("%T", err),
	})
}

func elapsedMS(start time.Time) int64 {
	return time.Since(start).Milliseconds()
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
