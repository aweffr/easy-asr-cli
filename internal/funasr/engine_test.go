package funasr_test

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/aweffr/easy-asr-cli/internal/config"
	"github.com/aweffr/easy-asr-cli/internal/dashscope"
	"github.com/aweffr/easy-asr-cli/internal/engine"
	"github.com/aweffr/easy-asr-cli/internal/funasr"
	"github.com/aweffr/easy-asr-cli/internal/srt"
	"github.com/aweffr/easy-asr-cli/internal/storage"
)

func TestEngineTranscribeUploadsCallsASRRendersSpeakerLabelsAndCleansUp(t *testing.T) {
	dir := t.TempDir()
	audio := filepath.Join(dir, "sample.wav")
	if err := os.WriteFile(audio, []byte("audio"), 0o600); err != nil {
		t.Fatalf("write audio: %v", err)
	}
	outPath := filepath.Join(dir, "sample.srt")
	rawPath := filepath.Join(dir, "raw.json")
	speakerID := 1

	store := &fakeStorage{stored: storage.StoredObject{Bucket: "bucket", Key: "tmp/sample.wav"}, url: "https://signed.example/sample.wav"}
	asr := &fakeASR{
		taskID: "task-fun",
		result: dashscope.FunASRTaskResult{
			TaskID:           "task-fun",
			TranscriptionURL: "https://result.example/fun.json?sig=example",
			UsageSeconds:     8,
		},
		transcription: srt.Transcription{Transcripts: []srt.Transcript{{
			Sentences: []srt.Sentence{{
				BeginTime: 0,
				EndTime:   1000,
				Text:      "你好",
				SpeakerID: &speakerID,
			}},
		}}},
	}
	runner := funasr.NewEngine(funasr.Options{
		Config:  config.Default().FunASR(),
		Storage: store,
		ASR:     asr,
	})

	result, err := runner.Transcribe(context.Background(), engine.Request{
		AudioPath:          audio,
		OutputPath:         outPath,
		RawJSONPath:        rawPath,
		Language:           "zh",
		Channels:           []int{0},
		VocabularyID:       "vocab-123",
		DiarizationEnabled: true,
		SpeakerCount:       2,
	})
	if err != nil {
		t.Fatalf("Transcribe returned error: %v", err)
	}
	if result.Engine != config.EngineFunASR || result.TaskID != "task-fun" {
		t.Fatalf("result = %#v", result)
	}
	if result.TranscriptionURL != "https://result.example/fun.json?<redacted>" {
		t.Fatalf("TranscriptionURL should be redacted, got %q", result.TranscriptionURL)
	}
	if result.UsageSeconds != 8 {
		t.Fatalf("UsageSeconds = %d", result.UsageSeconds)
	}
	srtBytes, err := os.ReadFile(outPath)
	if err != nil {
		t.Fatalf("read srt: %v", err)
	}
	if !strings.Contains(string(srtBytes), "[SPEAKER_1] 你好") {
		t.Fatalf("speaker label missing from SRT:\n%s", srtBytes)
	}
	rawBytes, err := os.ReadFile(rawPath)
	if err != nil {
		t.Fatalf("read raw json: %v", err)
	}
	var raw map[string]any
	if err := json.Unmarshal(rawBytes, &raw); err != nil {
		t.Fatalf("raw json is invalid: %v", err)
	}
	if store.deleted != "tmp/sample.wav" {
		t.Fatalf("deleted key = %q", store.deleted)
	}
	if asr.submit.FileURL != "https://signed.example/sample.wav" {
		t.Fatalf("submit file url = %q", asr.submit.FileURL)
	}
	if asr.submit.VocabularyID != "vocab-123" || !asr.submit.DiarizationEnabled || asr.submit.SpeakerCount != 2 {
		t.Fatalf("submit request = %#v", asr.submit)
	}
}

type fakeStorage struct {
	stored  storage.StoredObject
	url     string
	deleted string
}

func (f *fakeStorage) Upload(ctx context.Context, path string) (storage.StoredObject, error) {
	return f.stored, nil
}

func (f *fakeStorage) PresignGet(ctx context.Context, key string) (string, error) {
	return f.url, nil
}

func (f *fakeStorage) Delete(ctx context.Context, key string) error {
	f.deleted = key
	return nil
}

type fakeASR struct {
	taskID        string
	submit        dashscope.FunASRSubmitRequest
	result        dashscope.FunASRTaskResult
	transcription srt.Transcription
}

func (f *fakeASR) SubmitTask(ctx context.Context, request dashscope.FunASRSubmitRequest) (string, error) {
	f.submit = request
	return f.taskID, nil
}

func (f *fakeASR) WaitTask(ctx context.Context, taskID string, timeout time.Duration) (dashscope.FunASRTaskResult, error) {
	return f.result, nil
}

func (f *fakeASR) DownloadTranscription(ctx context.Context, url string) (srt.Transcription, error) {
	return f.transcription, nil
}
