package mimo_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/aweffr/easy-asr-cli/internal/mimo"
)

func TestClientSendsDataURLPayloadAndMapsUsage(t *testing.T) {
	var gotAPIKey string
	var gotPayload map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/chat/completions" {
			t.Fatalf("path = %q", r.URL.Path)
		}
		gotAPIKey = r.Header.Get("api-key")
		if err := json.NewDecoder(r.Body).Decode(&gotPayload); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"id":    "chatcmpl-1",
			"model": "mimo-v2.5-asr",
			"choices": []map[string]any{{
				"finish_reason": "stop",
				"message":       map[string]any{"content": "你好世界"},
			}},
			"usage": map[string]any{"seconds": 12},
		})
	}))
	defer server.Close()

	client := mimo.NewClient(mimo.ClientOptions{
		APIKey:         "mimo-key",
		BaseURL:        server.URL,
		Model:          "mimo-v2.5-asr",
		RequestTimeout: time.Second,
	})
	response, err := client.TranscribeDataURL(context.Background(), "data:audio/wav;base64,AAAA", "zh")
	if err != nil {
		t.Fatalf("TranscribeDataURL returned error: %v", err)
	}

	if gotAPIKey != "mimo-key" {
		t.Fatalf("api-key header = %q", gotAPIKey)
	}
	if gotPayload["model"] != "mimo-v2.5-asr" {
		t.Fatalf("model = %#v", gotPayload["model"])
	}
	if gotPayload["temperature"] != float64(0) || gotPayload["top_p"] != 0.01 || gotPayload["max_tokens"] != float64(4096) {
		t.Fatalf("generation params = %#v", gotPayload)
	}
	options := gotPayload["asr_options"].(map[string]any)
	if options["language"] != "zh" {
		t.Fatalf("language = %#v", options["language"])
	}
	messages := gotPayload["messages"].([]any)
	content := messages[0].(map[string]any)["content"].([]any)
	inputAudio := content[0].(map[string]any)["input_audio"].(map[string]any)
	if inputAudio["data"] != "data:audio/wav;base64,AAAA" {
		t.Fatalf("input_audio.data = %#v", inputAudio["data"])
	}
	if response.Content != "你好世界" || response.UsageSeconds != 12 {
		t.Fatalf("response = %#v", response)
	}
	if response.FinishReason != "stop" {
		t.Fatalf("FinishReason = %q", response.FinishReason)
	}
	if response.Raw["id"] != "chatcmpl-1" {
		t.Fatalf("raw response missing id: %#v", response.Raw)
	}
}

func TestClientReturnsHTTPErrorBody(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, `{"error":"bad audio"}`, http.StatusBadRequest)
	}))
	defer server.Close()

	client := mimo.NewClient(mimo.ClientOptions{
		APIKey:         "mimo-key",
		BaseURL:        server.URL,
		Model:          "mimo-v2.5-asr",
		RequestTimeout: time.Second,
	})
	if _, err := client.TranscribeDataURL(context.Background(), "data:audio/wav;base64,AAAA", "auto"); err == nil {
		t.Fatal("TranscribeDataURL returned nil error")
	}
}
