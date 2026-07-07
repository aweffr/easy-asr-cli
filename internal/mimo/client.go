package mimo

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

type ClientOptions struct {
	APIKey         string
	BaseURL        string
	Model          string
	RequestTimeout time.Duration
	HTTPClient     *http.Client
}

type Client struct {
	apiKey     string
	baseURL    string
	model      string
	httpClient *http.Client
}

type TranscriptionResponse struct {
	ID           string
	Model        string
	Content      string
	FinishReason string
	UsageSeconds int64
	Raw          map[string]any
}

func NewClient(options ClientOptions) *Client {
	timeout := options.RequestTimeout
	if timeout <= 0 {
		timeout = 5 * time.Minute
	}
	httpClient := options.HTTPClient
	if httpClient == nil {
		httpClient = &http.Client{Timeout: timeout}
	}
	return &Client{
		apiKey:     options.APIKey,
		baseURL:    strings.TrimRight(options.BaseURL, "/"),
		model:      options.Model,
		httpClient: httpClient,
	}
}

func (c *Client) TranscribeDataURL(ctx context.Context, dataURL string, language string) (TranscriptionResponse, error) {
	if strings.TrimSpace(c.apiKey) == "" {
		return TranscriptionResponse{}, fmt.Errorf("mimo api key is required")
	}
	if strings.TrimSpace(c.baseURL) == "" {
		return TranscriptionResponse{}, fmt.Errorf("mimo base url is required")
	}
	if strings.TrimSpace(c.model) == "" {
		return TranscriptionResponse{}, fmt.Errorf("mimo model is required")
	}
	if strings.TrimSpace(language) == "" {
		language = "auto"
	}
	payload := map[string]any{
		"model":       c.model,
		"temperature": 0,
		"top_p":       0.01,
		"max_tokens":  4096,
		"messages": []map[string]any{{
			"role": "user",
			"content": []map[string]any{{
				"type":        "input_audio",
				"input_audio": map[string]string{"data": dataURL},
			}},
		}},
		"asr_options": map[string]string{"language": language},
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return TranscriptionResponse{}, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/chat/completions", bytes.NewReader(body))
	if err != nil {
		return TranscriptionResponse{}, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("api-key", c.apiKey)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return TranscriptionResponse{}, err
	}
	defer resp.Body.Close()
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return TranscriptionResponse{}, err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return TranscriptionResponse{}, fmt.Errorf("mimo request failed with HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(respBody)))
	}
	var raw map[string]any
	if err := json.Unmarshal(respBody, &raw); err != nil {
		return TranscriptionResponse{}, fmt.Errorf("decode mimo response: %w", err)
	}
	return parseResponse(raw)
}

func parseResponse(raw map[string]any) (TranscriptionResponse, error) {
	response := TranscriptionResponse{Raw: raw}
	if id, ok := raw["id"].(string); ok {
		response.ID = id
	}
	if model, ok := raw["model"].(string); ok {
		response.Model = model
	}
	if usage, ok := raw["usage"].(map[string]any); ok {
		if seconds, ok := usage["seconds"].(float64); ok {
			response.UsageSeconds = int64(seconds)
		}
	}
	choices, ok := raw["choices"].([]any)
	if !ok || len(choices) == 0 {
		return response, fmt.Errorf("mimo response has no choices")
	}
	first, ok := choices[0].(map[string]any)
	if !ok {
		return response, fmt.Errorf("mimo response choice has unexpected shape")
	}
	if reason, ok := first["finish_reason"].(string); ok {
		response.FinishReason = reason
	}
	message, ok := first["message"].(map[string]any)
	if !ok {
		return response, fmt.Errorf("mimo response choice has no message")
	}
	content, ok := message["content"].(string)
	if !ok {
		return response, fmt.Errorf("mimo response message has no text content")
	}
	response.Content = strings.TrimSpace(content)
	return response, nil
}
