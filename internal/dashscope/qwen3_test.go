package dashscope_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/aweffr/easy-asr-cli/internal/dashscope"
)

func TestSubmitTaskBuildsQwen3FiletransPayload(t *testing.T) {
	var gotPath string
	var gotHeader http.Header
	var gotPayload map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotHeader = r.Header.Clone()
		if err := json.NewDecoder(r.Body).Decode(&gotPayload); err != nil {
			t.Fatalf("decode payload: %v", err)
		}
		_, _ = w.Write([]byte(`{"output":{"task_id":"task-123"}}`))
	}))
	defer server.Close()

	client := dashscope.NewClient(dashscope.Options{
		APIKey:         "dashscope-key",
		BaseURL:        server.URL + "/api/v1",
		Model:          "qwen3-asr-flash-filetrans",
		RequestTimeout: time.Second,
	})

	taskID, err := client.SubmitTask(context.Background(), dashscope.SubmitRequest{
		FileURL:     "https://signed.example/audio.m4a",
		ChannelIDs:  []int{0, 1},
		EnableITN:   false,
		EnableWords: true,
		Language:    "zh",
		Hotwords:    "星巴克 永丰国际广场店",
	})
	if err != nil {
		t.Fatalf("SubmitTask returned error: %v", err)
	}
	if taskID != "task-123" {
		t.Fatalf("taskID = %q", taskID)
	}
	if gotPath != "/api/v1/services/audio/asr/transcription" {
		t.Fatalf("path = %q", gotPath)
	}
	if gotHeader.Get("Authorization") != "Bearer dashscope-key" {
		t.Fatalf("Authorization header = %q", gotHeader.Get("Authorization"))
	}
	if gotHeader.Get("X-DashScope-Async") != "enable" {
		t.Fatalf("X-DashScope-Async header = %q", gotHeader.Get("X-DashScope-Async"))
	}
	if gotPayload["model"] != "qwen3-asr-flash-filetrans" {
		t.Fatalf("model = %#v", gotPayload["model"])
	}
	input := gotPayload["input"].(map[string]any)
	if input["file_url"] != "https://signed.example/audio.m4a" {
		t.Fatalf("input.file_url = %#v", input["file_url"])
	}
	parameters := gotPayload["parameters"].(map[string]any)
	if parameters["language"] != "zh" {
		t.Fatalf("parameters.language = %#v", parameters["language"])
	}
	if parameters["enable_itn"] != false || parameters["enable_words"] != true {
		t.Fatalf("enable flags = %#v", parameters)
	}
	corpus := parameters["corpus"].(map[string]any)
	if corpus["text"] != "星巴克 永丰国际广场店" {
		t.Fatalf("corpus.text = %#v", corpus["text"])
	}
}

func TestWaitTaskPollsUntilSucceededAndExtractsResultURL(t *testing.T) {
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Fatalf("method = %s", r.Method)
		}
		calls++
		if calls == 1 {
			_, _ = w.Write([]byte(`{"output":{"task_status":"RUNNING"}}`))
			return
		}
		_, _ = w.Write([]byte(`{
			"request_id": "req-1",
			"output": {
				"task_id": "task-123",
				"task_status": "SUCCEEDED",
				"result": {"transcription_url": "https://result.example/transcription.json"}
			},
			"usage": {"seconds": 12}
		}`))
	}))
	defer server.Close()

	client := dashscope.NewClient(dashscope.Options{
		APIKey:         "dashscope-key",
		BaseURL:        server.URL + "/api/v1",
		Model:          "qwen3-asr-flash-filetrans",
		RequestTimeout: time.Second,
		PollInterval:   time.Nanosecond,
	})

	result, err := client.WaitTask(context.Background(), "task-123", 3*time.Second)
	if err != nil {
		t.Fatalf("WaitTask returned error: %v", err)
	}
	if calls != 2 {
		t.Fatalf("calls = %d", calls)
	}
	if result.TranscriptionURL != "https://result.example/transcription.json" {
		t.Fatalf("TranscriptionURL = %q", result.TranscriptionURL)
	}
	if result.UsageSeconds != 12 {
		t.Fatalf("UsageSeconds = %d", result.UsageSeconds)
	}
}

func TestWaitTaskReportsFailedStatus(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{
			"output": {
				"task_status": "FAILED",
				"code": "FILE_403_FORBIDDEN",
				"message": "download failed: https://signed.example/audio.mp3?sig=example"
			}
		}`))
	}))
	defer server.Close()
	client := dashscope.NewClient(dashscope.Options{
		APIKey:         "dashscope-key",
		BaseURL:        server.URL + "/api/v1",
		RequestTimeout: time.Second,
	})

	_, err := client.WaitTask(context.Background(), "task-123", time.Second)
	if err == nil {
		t.Fatal("WaitTask returned nil error")
	}
	if !strings.Contains(err.Error(), "FILE_403_FORBIDDEN") {
		t.Fatalf("error = %v", err)
	}
	if strings.Contains(err.Error(), "sig=example") {
		t.Fatalf("error leaked signed URL query: %v", err)
	}
	if !strings.Contains(err.Error(), "?<redacted>") {
		t.Fatalf("error should contain redacted URL query: %v", err)
	}
}

func TestHTTPErrorRedactsSignedURLQueryFromBody(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`bad file_url https://signed.example/audio.mp3?ttl=1&sig=example`))
	}))
	defer server.Close()
	client := dashscope.NewClient(dashscope.Options{
		APIKey:         "dashscope-key",
		BaseURL:        server.URL + "/api/v1",
		RequestTimeout: time.Second,
	})

	_, err := client.SubmitTask(context.Background(), dashscope.SubmitRequest{
		FileURL: "https://signed.example/audio.mp3?ttl=1&sig=example",
	})
	if err == nil {
		t.Fatal("SubmitTask returned nil error")
	}
	if strings.Contains(err.Error(), "sig=example") || strings.Contains(err.Error(), "ttl=1") {
		t.Fatalf("error leaked signed URL query: %v", err)
	}
	if !strings.Contains(err.Error(), "?<redacted>") {
		t.Fatalf("error should contain redacted URL query: %v", err)
	}
}

func TestHTTPErrorRedactsEscapedSignedURLQueryFromBody(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`bad file_url https:\/\/signed.example\/audio.mp3?ttl=1&sig=example`))
	}))
	defer server.Close()
	client := dashscope.NewClient(dashscope.Options{
		APIKey:         "dashscope-key",
		BaseURL:        server.URL + "/api/v1",
		RequestTimeout: time.Second,
	})

	_, err := client.SubmitTask(context.Background(), dashscope.SubmitRequest{
		FileURL: "https://signed.example/audio.mp3?ttl=1&sig=example",
	})
	if err == nil {
		t.Fatal("SubmitTask returned nil error")
	}
	if strings.Contains(err.Error(), "sig=example") || strings.Contains(err.Error(), "ttl=1") {
		t.Fatalf("error leaked escaped signed URL query: %v", err)
	}
	if !strings.Contains(err.Error(), "?<redacted>") {
		t.Fatalf("error should contain redacted URL query: %v", err)
	}
}

func TestDownloadTranscriptionRedactsSignedURLQueryFromTransportError(t *testing.T) {
	client := dashscope.NewClient(dashscope.Options{
		APIKey:     "dashscope-key",
		HTTPClient: failingHTTPClient{},
	})

	_, err := client.DownloadTranscription(
		context.Background(),
		"https://result.example/transcription.json?ttl=1&sig=example",
	)
	if err == nil {
		t.Fatal("DownloadTranscription returned nil error")
	}
	if strings.Contains(err.Error(), "sig=example") || strings.Contains(err.Error(), "ttl=1") {
		t.Fatalf("error leaked signed URL query: %v", err)
	}
	if !strings.Contains(err.Error(), "?<redacted>") {
		t.Fatalf("error should contain redacted URL query: %v", err)
	}
}

func TestDownloadTranscriptionRedactsSignedURLQueryFromMalformedURL(t *testing.T) {
	client := dashscope.NewClient(dashscope.Options{APIKey: "dashscope-key"})

	_, err := client.DownloadTranscription(
		context.Background(),
		"https://result.example/%zz?ttl=1&sig=example",
	)
	if err == nil {
		t.Fatal("DownloadTranscription returned nil error")
	}
	if strings.Contains(err.Error(), "sig=example") || strings.Contains(err.Error(), "ttl=1") {
		t.Fatalf("error leaked signed URL query: %v", err)
	}
	if !strings.Contains(err.Error(), "?<redacted>") {
		t.Fatalf("error should contain redacted URL query: %v", err)
	}
}

func TestDownloadTranscriptionDecodesJSON(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{
			"transcripts": [{
				"sentences": [{"begin_time":0,"end_time":1000,"text":"hello"}]
			}]
		}`))
	}))
	defer server.Close()
	client := dashscope.NewClient(dashscope.Options{
		APIKey:         "dashscope-key",
		BaseURL:        server.URL + "/api/v1",
		RequestTimeout: time.Second,
	})

	payload, err := client.DownloadTranscription(context.Background(), server.URL+"/result.json")
	if err != nil {
		t.Fatalf("DownloadTranscription returned error: %v", err)
	}
	if payload.Transcripts[0].Sentences[0].Text != "hello" {
		t.Fatalf("decoded payload = %#v", payload)
	}
}

type failingHTTPClient struct{}

func (failingHTTPClient) Do(req *http.Request) (*http.Response, error) {
	return nil, fmt.Errorf("Get %q: dial tcp failed", req.URL.String())
}
