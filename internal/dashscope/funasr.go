package dashscope

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/aweffr/easy-asr-cli/internal/redact"
	"github.com/aweffr/easy-asr-cli/internal/srt"
)

type FunASROptions struct {
	APIKey         string
	BaseURL        string
	Model          string
	RequestTimeout time.Duration
	PollInterval   time.Duration
	HTTPClient     HTTPClient
}

type FunASRClient struct {
	apiKey       string
	baseURL      string
	model        string
	pollInterval time.Duration
	httpClient   HTTPClient
}

type FunASRSubmitRequest struct {
	FileURL            string
	ChannelIDs         []int
	Language           string
	VocabularyID       string
	DiarizationEnabled bool
	SpeakerCount       int
}

type FunASRTaskResult struct {
	RequestID        string
	TaskID           string
	TranscriptionURL string
	UsageSeconds     int64
}

func NewFunASRClient(options FunASROptions) *FunASRClient {
	if options.BaseURL == "" {
		options.BaseURL = "https://dashscope.aliyuncs.com/api/v1"
	}
	if options.Model == "" {
		options.Model = "fun-asr"
	}
	if options.RequestTimeout <= 0 {
		options.RequestTimeout = 30 * time.Second
	}
	if options.PollInterval <= 0 {
		options.PollInterval = 5 * time.Second
	}
	httpClient := options.HTTPClient
	if httpClient == nil {
		httpClient = &http.Client{Timeout: options.RequestTimeout}
	}
	return &FunASRClient{
		apiKey:       options.APIKey,
		baseURL:      strings.TrimRight(options.BaseURL, "/"),
		model:        options.Model,
		pollInterval: options.PollInterval,
		httpClient:   httpClient,
	}
}

func (c *FunASRClient) SubmitTask(ctx context.Context, request FunASRSubmitRequest) (string, error) {
	payload := funASRSubmitPayload{
		Model:      c.model,
		Input:      funASRSubmitInput{FileURLs: []string{request.FileURL}},
		Parameters: map[string]any{},
	}
	if len(request.ChannelIDs) > 0 {
		payload.Parameters["channel_id"] = request.ChannelIDs
	}
	if language := strings.TrimSpace(request.Language); language != "" {
		payload.Parameters["language_hints"] = []string{language}
	}
	if vocabularyID := strings.TrimSpace(request.VocabularyID); vocabularyID != "" {
		payload.Parameters["vocabulary_id"] = vocabularyID
	}
	if request.DiarizationEnabled {
		payload.Parameters["diarization_enabled"] = true
	}
	if request.SpeakerCount > 0 {
		payload.Parameters["speaker_count"] = request.SpeakerCount
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}
	httpReq, err := http.NewRequestWithContext(
		ctx,
		http.MethodPost,
		c.baseURL+"/services/audio/asr/transcription",
		bytes.NewReader(body),
	)
	if err != nil {
		return "", err
	}
	httpReq.Header.Set("Authorization", "Bearer "+c.apiKey)
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("X-DashScope-Async", "enable")

	var response funASRSubmitResponse
	if err := c.doJSON(httpReq, &response); err != nil {
		return "", fmt.Errorf("submit Fun-ASR task: %w", err)
	}
	taskID := strings.TrimSpace(response.Output.TaskID)
	if taskID == "" {
		return "", errors.New("submit Fun-ASR task returned empty task_id")
	}
	return taskID, nil
}

func (c *FunASRClient) WaitTask(ctx context.Context, taskID string, timeout time.Duration) (FunASRTaskResult, error) {
	if timeout <= 0 {
		timeout = 2 * time.Hour
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	for {
		result, status, err := c.fetchTask(ctx, taskID)
		if err != nil {
			return FunASRTaskResult{}, err
		}
		switch status {
		case "SUCCEEDED":
			return result, nil
		case "PENDING", "RUNNING":
			timer := time.NewTimer(c.pollInterval)
			select {
			case <-ctx.Done():
				timer.Stop()
				return FunASRTaskResult{}, fmt.Errorf("Fun-ASR task polling timed out: %w", ctx.Err())
			case <-timer.C:
				continue
			}
		case "FAILED", "UNKNOWN":
			return FunASRTaskResult{}, fmt.Errorf("Fun-ASR task failed: %s", status)
		default:
			return FunASRTaskResult{}, fmt.Errorf("Fun-ASR task returned unexpected status %q", status)
		}
	}
}

func (c *FunASRClient) DownloadTranscription(ctx context.Context, url string) (srt.Transcription, error) {
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return srt.Transcription{}, fmt.Errorf("create Fun-ASR transcription download request: %s", redact.URLQueries(err.Error()))
	}
	var payload srt.Transcription
	if err := c.doJSON(httpReq, &payload); err != nil {
		return srt.Transcription{}, fmt.Errorf("download Fun-ASR transcription JSON: %w", err)
	}
	return payload, nil
}

func (c *FunASRClient) fetchTask(ctx context.Context, taskID string) (FunASRTaskResult, string, error) {
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+"/tasks/"+taskID, nil)
	if err != nil {
		return FunASRTaskResult{}, "", err
	}
	httpReq.Header.Set("Authorization", "Bearer "+c.apiKey)
	var response funASRTaskResponse
	if err := c.doJSON(httpReq, &response); err != nil {
		return FunASRTaskResult{}, "", fmt.Errorf("fetch Fun-ASR task: %w", err)
	}
	status := strings.ToUpper(strings.TrimSpace(response.Output.TaskStatus))
	result := FunASRTaskResult{
		RequestID:    response.RequestID,
		TaskID:       response.Output.TaskID,
		UsageSeconds: response.Usage.Duration,
	}
	if status == "FAILED" || status == "UNKNOWN" {
		return result, status, fmt.Errorf(
			"Fun-ASR task failed: %s %s %s",
			status,
			response.Output.Code,
			redact.URLQueries(response.Output.Message),
		)
	}
	if status != "SUCCEEDED" {
		return result, status, nil
	}
	for _, item := range response.Output.Results {
		subtaskStatus := strings.ToUpper(strings.TrimSpace(item.SubtaskStatus))
		if subtaskStatus == "SUCCEEDED" && strings.TrimSpace(item.TranscriptionURL) != "" {
			result.TranscriptionURL = item.TranscriptionURL
			return result, status, nil
		}
		if subtaskStatus == "FAILED" {
			return result, status, fmt.Errorf(
				"Fun-ASR subtask failed: %s %s",
				item.Code,
				redact.URLQueries(item.Message),
			)
		}
	}
	return result, status, errors.New("Fun-ASR task succeeded without transcription_url")
}

func (c *FunASRClient) doJSON(req *http.Request, target any) error {
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return errors.New(redact.URLQueries(err.Error()))
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		preview, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return fmt.Errorf("HTTP %d: %s", resp.StatusCode, redact.URLQueries(strings.TrimSpace(string(preview))))
	}
	if err := json.NewDecoder(resp.Body).Decode(target); err != nil {
		return err
	}
	return nil
}

type funASRSubmitPayload struct {
	Model      string            `json:"model"`
	Input      funASRSubmitInput `json:"input"`
	Parameters map[string]any    `json:"parameters"`
}

type funASRSubmitInput struct {
	FileURLs []string `json:"file_urls"`
}

type funASRSubmitResponse struct {
	Output struct {
		TaskID string `json:"task_id"`
	} `json:"output"`
}

type funASRTaskResponse struct {
	RequestID string `json:"request_id"`
	Output    struct {
		TaskID     string `json:"task_id"`
		TaskStatus string `json:"task_status"`
		Code       string `json:"code"`
		Message    string `json:"message"`
		Results    []struct {
			FileURL          string `json:"file_url"`
			TranscriptionURL string `json:"transcription_url"`
			SubtaskStatus    string `json:"subtask_status"`
			Code             string `json:"code"`
			Message          string `json:"message"`
		} `json:"results"`
	} `json:"output"`
	Usage struct {
		Duration int64 `json:"duration"`
	} `json:"usage"`
}
