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

type HTTPClient interface {
	Do(req *http.Request) (*http.Response, error)
}

type Options struct {
	APIKey         string
	BaseURL        string
	Model          string
	RequestTimeout time.Duration
	PollInterval   time.Duration
	HTTPClient     HTTPClient
}

type Client struct {
	apiKey       string
	baseURL      string
	model        string
	pollInterval time.Duration
	httpClient   HTTPClient
}

type SubmitRequest struct {
	FileURL     string
	ChannelIDs  []int
	EnableITN   bool
	EnableWords bool
	Language    string
	Hotwords    string
}

type TaskResult struct {
	RequestID        string
	TaskID           string
	TranscriptionURL string
	UsageSeconds     int64
}

func NewClient(options Options) *Client {
	if options.BaseURL == "" {
		options.BaseURL = "https://dashscope.aliyuncs.com/api/v1"
	}
	if options.Model == "" {
		options.Model = "qwen3-asr-flash-filetrans"
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
	return &Client{
		apiKey:       options.APIKey,
		baseURL:      strings.TrimRight(options.BaseURL, "/"),
		model:        options.Model,
		pollInterval: options.PollInterval,
		httpClient:   httpClient,
	}
}

func (c *Client) SubmitTask(ctx context.Context, request SubmitRequest) (string, error) {
	payload := submitPayload{
		Model: c.model,
		Input: submitInput{FileURL: request.FileURL},
		Parameters: submitParameters{
			ChannelIDs:  request.ChannelIDs,
			EnableITN:   request.EnableITN,
			EnableWords: request.EnableWords,
			Language:    strings.TrimSpace(request.Language),
		},
	}
	if len(payload.Parameters.ChannelIDs) == 0 {
		payload.Parameters.ChannelIDs = []int{0}
	}
	if hotwords := strings.TrimSpace(request.Hotwords); hotwords != "" {
		payload.Parameters.Corpus = &submitCorpus{Text: hotwords}
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
	c.addHeaders(httpReq)

	var response submitResponse
	if err := c.doJSON(httpReq, &response); err != nil {
		return "", fmt.Errorf("submit ASR task: %w", err)
	}
	taskID := strings.TrimSpace(response.Output.TaskID)
	if taskID == "" {
		return "", errors.New("submit ASR task returned empty task_id")
	}
	return taskID, nil
}

func (c *Client) WaitTask(ctx context.Context, taskID string, timeout time.Duration) (TaskResult, error) {
	if timeout <= 0 {
		timeout = 2 * time.Hour
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	for {
		result, status, err := c.fetchTask(ctx, taskID)
		if err != nil {
			return TaskResult{}, err
		}
		switch status {
		case "SUCCEEDED":
			if strings.TrimSpace(result.TranscriptionURL) == "" {
				return TaskResult{}, errors.New("ASR task succeeded without transcription_url")
			}
			return result, nil
		case "PENDING", "RUNNING":
			timer := time.NewTimer(c.pollInterval)
			select {
			case <-ctx.Done():
				timer.Stop()
				return TaskResult{}, fmt.Errorf("ASR task polling timed out: %w", ctx.Err())
			case <-timer.C:
				continue
			}
		case "FAILED", "UNKNOWN":
			return TaskResult{}, fmt.Errorf("ASR task failed: %s", status)
		default:
			return TaskResult{}, fmt.Errorf("ASR task returned unexpected status %q", status)
		}
	}
}

func (c *Client) DownloadTranscription(ctx context.Context, url string) (srt.Transcription, error) {
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return srt.Transcription{}, fmt.Errorf("create transcription download request: %s", redact.URLQueries(err.Error()))
	}
	var payload srt.Transcription
	if err := c.doJSON(httpReq, &payload); err != nil {
		return srt.Transcription{}, fmt.Errorf("download transcription JSON: %w", err)
	}
	return payload, nil
}

func (c *Client) fetchTask(ctx context.Context, taskID string) (TaskResult, string, error) {
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+"/tasks/"+taskID, nil)
	if err != nil {
		return TaskResult{}, "", err
	}
	c.addHeaders(httpReq)
	var response taskResponse
	if err := c.doJSON(httpReq, &response); err != nil {
		return TaskResult{}, "", fmt.Errorf("fetch ASR task: %w", err)
	}
	status := strings.ToUpper(strings.TrimSpace(response.Output.TaskStatus))
	result := TaskResult{
		RequestID:        response.RequestID,
		TaskID:           response.Output.TaskID,
		TranscriptionURL: response.Output.Result.TranscriptionURL,
		UsageSeconds:     response.Usage.Seconds,
	}
	if status == "FAILED" || status == "UNKNOWN" {
		return result, status, fmt.Errorf(
			"ASR task failed: %s %s %s",
			status,
			response.Output.Code,
			redact.URLQueries(response.Output.Message),
		)
	}
	return result, status, nil
}

func (c *Client) addHeaders(req *http.Request) {
	req.Header.Set("Authorization", "Bearer "+c.apiKey)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-DashScope-Async", "enable")
}

func (c *Client) doJSON(req *http.Request, target any) error {
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

type submitPayload struct {
	Model      string           `json:"model"`
	Input      submitInput      `json:"input"`
	Parameters submitParameters `json:"parameters"`
}

type submitInput struct {
	FileURL string `json:"file_url"`
}

type submitParameters struct {
	ChannelIDs  []int         `json:"channel_id"`
	EnableITN   bool          `json:"enable_itn"`
	EnableWords bool          `json:"enable_words"`
	Language    string        `json:"language,omitempty"`
	Corpus      *submitCorpus `json:"corpus,omitempty"`
}

type submitCorpus struct {
	Text string `json:"text"`
}

type submitResponse struct {
	Output struct {
		TaskID string `json:"task_id"`
	} `json:"output"`
}

type taskResponse struct {
	RequestID string `json:"request_id"`
	Output    struct {
		TaskID     string `json:"task_id"`
		TaskStatus string `json:"task_status"`
		Code       string `json:"code"`
		Message    string `json:"message"`
		Result     struct {
			TranscriptionURL string `json:"transcription_url"`
		} `json:"result"`
	} `json:"output"`
	Usage struct {
		Seconds int64 `json:"seconds"`
	} `json:"usage"`
}
