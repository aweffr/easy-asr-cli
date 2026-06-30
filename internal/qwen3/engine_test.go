package qwen3_test

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/aweffr/easy-asr-cli/internal/config"
	"github.com/aweffr/easy-asr-cli/internal/dashscope"
	"github.com/aweffr/easy-asr-cli/internal/engine"
	"github.com/aweffr/easy-asr-cli/internal/qwen3"
	"github.com/aweffr/easy-asr-cli/internal/srt"
	"github.com/aweffr/easy-asr-cli/internal/storage"
)

func TestEngineTranscribeUploadsCallsASRRendersAndCleansUp(t *testing.T) {
	dir := t.TempDir()
	audio := filepath.Join(dir, "sample.mp3")
	if err := os.WriteFile(audio, []byte("audio"), 0o600); err != nil {
		t.Fatalf("write audio: %v", err)
	}
	outPath := filepath.Join(dir, "sample.srt")
	rawPath := filepath.Join(dir, "raw.json")

	store := &fakeStorage{stored: storage.StoredObject{Bucket: "bucket", Key: "tmp/sample.mp3"}, url: "https://signed.example/sample.mp3"}
	asr := &fakeASR{
		taskID: "task-123",
		result: dashscope.TaskResult{
			TaskID:           "task-123",
			TranscriptionURL: "https://result.example/transcription.json?sig=example",
			UsageSeconds:     9,
		},
		transcription: srt.Transcription{Transcripts: []srt.Transcript{{
			Sentences: []srt.Sentence{{BeginTime: 0, EndTime: 1000, Text: "你好"}},
		}}},
	}
	runner := qwen3.NewEngine(qwen3.Options{
		Config:  config.Default().Qwen3(),
		Storage: store,
		ASR:     asr,
	})

	result, err := runner.Transcribe(context.Background(), engine.Request{
		AudioPath:   audio,
		OutputPath:  outPath,
		RawJSONPath: rawPath,
		Language:    "zh",
		Hotwords:    "星巴克",
		Channels:    []int{0},
		EnableITN:   false,
		EnableWords: true,
	})
	if err != nil {
		t.Fatalf("Transcribe returned error: %v", err)
	}

	if result.Engine != "qwen3-asr-flash-filetrans" || result.TaskID != "task-123" {
		t.Fatalf("result = %#v", result)
	}
	if result.TranscriptionURL != "https://result.example/transcription.json?<redacted>" {
		t.Fatalf("TranscriptionURL should be redacted, got %q", result.TranscriptionURL)
	}
	if result.OutputPath != outPath {
		t.Fatalf("OutputPath = %q", result.OutputPath)
	}
	srtBytes, err := os.ReadFile(outPath)
	if err != nil {
		t.Fatalf("read srt: %v", err)
	}
	if string(srtBytes) != "1\n00:00:00,000 --> 00:00:01,000\n你好\n" {
		t.Fatalf("unexpected srt:\n%s", srtBytes)
	}
	rawBytes, err := os.ReadFile(rawPath)
	if err != nil {
		t.Fatalf("read raw json: %v", err)
	}
	var raw map[string]any
	if err := json.Unmarshal(rawBytes, &raw); err != nil {
		t.Fatalf("raw json is invalid: %v", err)
	}
	if store.deleted != "tmp/sample.mp3" {
		t.Fatalf("deleted key = %q", store.deleted)
	}
	if asr.submit.FileURL != "https://signed.example/sample.mp3" {
		t.Fatalf("submit file url = %q", asr.submit.FileURL)
	}
	if asr.submit.Hotwords != "星巴克" || asr.submit.Language != "zh" || !asr.submit.EnableWords {
		t.Fatalf("submit request = %#v", asr.submit)
	}
}

func TestEngineKeepsObjectWhenRequested(t *testing.T) {
	dir := t.TempDir()
	audio := filepath.Join(dir, "sample.mp3")
	if err := os.WriteFile(audio, []byte("audio"), 0o600); err != nil {
		t.Fatalf("write audio: %v", err)
	}
	store := &fakeStorage{stored: storage.StoredObject{Bucket: "bucket", Key: "tmp/sample.mp3"}, url: "https://signed.example/sample.mp3"}
	asr := &fakeASR{
		taskID: "task-123",
		result: dashscope.TaskResult{TaskID: "task-123", TranscriptionURL: "https://result.example/transcription.json"},
		transcription: srt.Transcription{Transcripts: []srt.Transcript{{
			Sentences: []srt.Sentence{{BeginTime: 0, EndTime: 1000, Text: "你好"}},
		}}},
	}
	runner := qwen3.NewEngine(qwen3.Options{Config: config.Default().Qwen3(), Storage: store, ASR: asr})

	_, err := runner.Transcribe(context.Background(), engine.Request{
		AudioPath:  audio,
		OutputPath: filepath.Join(dir, "sample.srt"),
		KeepObject: true,
	})
	if err != nil {
		t.Fatalf("Transcribe returned error: %v", err)
	}
	if store.deleted != "" {
		t.Fatalf("expected object to be kept, deleted %q", store.deleted)
	}
}

func TestEngineReportsCleanupFailureInResult(t *testing.T) {
	dir := t.TempDir()
	audio := filepath.Join(dir, "sample.mp3")
	if err := os.WriteFile(audio, []byte("audio"), 0o600); err != nil {
		t.Fatalf("write audio: %v", err)
	}
	store := &fakeStorage{
		stored:      storage.StoredObject{Bucket: "bucket", Key: "tmp/sample.mp3"},
		url:         "https://signed.example/sample.mp3",
		deleteError: errors.New("delete denied"),
	}
	asr := &fakeASR{
		taskID: "task-123",
		result: dashscope.TaskResult{TaskID: "task-123", TranscriptionURL: "https://result.example/transcription.json"},
		transcription: srt.Transcription{Transcripts: []srt.Transcript{{
			Sentences: []srt.Sentence{{BeginTime: 0, EndTime: 1000, Text: "你好"}},
		}}},
	}
	runner := qwen3.NewEngine(qwen3.Options{Config: config.Default().Qwen3(), Storage: store, ASR: asr})

	result, err := runner.Transcribe(context.Background(), engine.Request{
		AudioPath:  audio,
		OutputPath: filepath.Join(dir, "sample.srt"),
	})
	if err != nil {
		t.Fatalf("Transcribe returned error: %v", err)
	}
	if !strings.Contains(result.CleanupError, "delete denied") {
		t.Fatalf("CleanupError = %q", result.CleanupError)
	}
}

type fakeStorage struct {
	stored      storage.StoredObject
	url         string
	deleted     string
	deleteError error
}

func (f *fakeStorage) Upload(ctx context.Context, path string) (storage.StoredObject, error) {
	return f.stored, nil
}

func (f *fakeStorage) PresignGet(ctx context.Context, key string) (string, error) {
	return f.url, nil
}

func (f *fakeStorage) Delete(ctx context.Context, key string) error {
	f.deleted = key
	return f.deleteError
}

type fakeASR struct {
	taskID        string
	submit        dashscope.SubmitRequest
	result        dashscope.TaskResult
	transcription srt.Transcription
}

func (f *fakeASR) SubmitTask(ctx context.Context, request dashscope.SubmitRequest) (string, error) {
	f.submit = request
	return f.taskID, nil
}

func (f *fakeASR) WaitTask(ctx context.Context, taskID string, timeout time.Duration) (dashscope.TaskResult, error) {
	return f.result, nil
}

func (f *fakeASR) DownloadTranscription(ctx context.Context, url string) (srt.Transcription, error) {
	return f.transcription, nil
}
