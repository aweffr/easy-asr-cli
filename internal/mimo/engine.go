package mimo

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/aweffr/easy-asr-cli/internal/assets"
	"github.com/aweffr/easy-asr-cli/internal/audio"
	"github.com/aweffr/easy-asr-cli/internal/config"
	"github.com/aweffr/easy-asr-cli/internal/engine"
	"github.com/aweffr/easy-asr-cli/internal/srt"
	"github.com/aweffr/easy-asr-cli/internal/vad"
)

type SegmentProcessor interface {
	Prepare(ctx context.Context, path string) ([]audio.PreparedSegment, error)
	Cleanup() error
}

type ASRClient interface {
	TranscribeDataURL(ctx context.Context, dataURL string, language string) (TranscriptionResponse, error)
}

type EngineOptions struct {
	Config            *config.MiMoConfig
	Processor         SegmentProcessor
	Client            ASRClient
	MaxConcurrency    int
	RequestStartDelay time.Duration
}

type Engine struct {
	cfg               *config.MiMoConfig
	processor         SegmentProcessor
	client            ASRClient
	maxConcurrency    int
	requestStartDelay time.Duration
}

type rawWrapper struct {
	Engine       string       `json:"engine"`
	AudioPath    string       `json:"audio_path"`
	UsageSeconds int64        `json:"usage_seconds"`
	Segments     []rawSegment `json:"segments"`
}

type rawSegment struct {
	Index        int            `json:"index"`
	Total        int            `json:"total"`
	Start        string         `json:"start"`
	End          string         `json:"end"`
	StartSeconds float64        `json:"start_seconds"`
	EndSeconds   float64        `json:"end_seconds"`
	FinishReason string         `json:"finish_reason,omitempty"`
	UsageSeconds int64          `json:"usage_seconds"`
	Response     map[string]any `json:"response"`
}

func NewEngine(options EngineOptions) *Engine {
	return &Engine{
		cfg:               options.Config,
		processor:         options.Processor,
		client:            options.Client,
		maxConcurrency:    options.MaxConcurrency,
		requestStartDelay: options.RequestStartDelay,
	}
}

func (e *Engine) Transcribe(ctx context.Context, request engine.Request) (result engine.Result, err error) {
	if e.cfg == nil {
		return engine.Result{}, fmt.Errorf("mimo config is required")
	}
	if request.OutputPath == "" {
		return engine.Result{}, fmt.Errorf("output path is required")
	}
	if e.processor == nil {
		modelPath, err := assets.ResolveSileroVADModelPath(e.cfg.Segmentation.ModelPath)
		if err != nil {
			return engine.Result{}, err
		}
		detector, err := vad.NewSileroDetector(vad.SileroOptions{
			ModelPath:              modelPath,
			ONNXRuntimeLibraryPath: assets.ResolveONNXRuntimeLibraryPath(e.cfg.Segmentation.ONNXRuntimeLibraryPath),
			Threshold:              e.cfg.Segmentation.VADThreshold,
			MinSilence:             e.cfg.Segmentation.MinSilence,
			SpeechPad:              e.cfg.Segmentation.SpeechPad,
		})
		if err != nil {
			return engine.Result{}, err
		}
		e.processor = &audio.Processor{
			Detector: detector,
			Options: audio.SegmentOptions{
				TargetDuration: e.cfg.Segmentation.TargetDuration,
				MinDuration:    e.cfg.Segmentation.MinDuration,
				MaxDuration:    e.cfg.Segmentation.MaxDuration,
			},
			TempDir: e.cfg.Segmentation.TempDir,
		}
	}
	if e.client == nil {
		e.client = NewClient(ClientOptions{
			APIKey:         e.cfg.MiMo.APIKey,
			BaseURL:        e.cfg.MiMo.BaseURL,
			Model:          e.cfg.MiMo.Model,
			RequestTimeout: e.cfg.ASR.RequestTimeout,
		})
	}

	result = engine.Result{
		Engine:      config.EngineMimoV25ASR,
		OutputPath:  request.OutputPath,
		RawJSONPath: request.RawJSONPath,
	}
	segments, err := e.processor.Prepare(ctx, request.AudioPath)
	if err != nil {
		return result, err
	}
	defer func() {
		if err := e.processor.Cleanup(); err != nil {
			result.CleanupError = err.Error()
		}
	}()

	language := firstNonEmpty(request.Language, e.cfg.ASR.Language, "auto")
	wrapper := rawWrapper{Engine: config.EngineMimoV25ASR, AudioPath: request.AudioPath}
	transcription := srt.Transcription{Transcripts: []srt.Transcript{{ChannelID: 0}}}
	responses, err := e.transcribeSegments(ctx, segments, language)
	if err != nil {
		return result, err
	}
	for _, segmentResponse := range responses {
		segment := segmentResponse.segment
		response := segmentResponse.response
		if segmentResponse.err != nil {
			return result, segmentResponse.err
		}
		result.UsageSeconds += response.UsageSeconds
		wrapper.UsageSeconds += response.UsageSeconds
		if response.FinishReason == "length" {
			result.Warnings = append(result.Warnings, fmt.Sprintf("mimo segment %d/%d reached finish_reason=length; transcript may be truncated", segment.Index, segment.Total))
		}
		wrapper.Segments = append(wrapper.Segments, rawSegment{
			Index:        segment.Index,
			Total:        segment.Total,
			Start:        formatClock(segment.Start),
			End:          formatClock(segment.End),
			StartSeconds: segment.Start.Seconds(),
			EndSeconds:   segment.End.Seconds(),
			FinishReason: response.FinishReason,
			UsageSeconds: response.UsageSeconds,
			Response:     response.Raw,
		})
		text := strings.TrimSpace(response.Content)
		if text == "" {
			continue
		}
		label := fmt.Sprintf("[PART %d/%d %s-%s]", segment.Index, segment.Total, formatClock(segment.Start), formatClock(segment.End))
		if response.FinishReason == "length" {
			label += "[TRUNCATED]"
		}
		transcription.Transcripts[0].Sentences = append(transcription.Transcripts[0].Sentences, srt.Sentence{
			SentenceID: segment.Index,
			BeginTime:  int64(segment.Start / time.Millisecond),
			EndTime:    int64(segment.End / time.Millisecond),
			Text:       label + " " + text,
		})
	}
	if request.RawJSONPath != "" {
		if err := writeJSON(request.RawJSONPath, wrapper); err != nil {
			return result, err
		}
	}
	rendered, err := srt.Render(transcription, srt.Options{MaxCueDuration: int64(4 * time.Hour / time.Millisecond)})
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

type segmentResponse struct {
	segment  audio.PreparedSegment
	response TranscriptionResponse
	err      error
}

func (e *Engine) transcribeSegments(ctx context.Context, segments []audio.PreparedSegment, language string) ([]segmentResponse, error) {
	maxConcurrency := e.maxConcurrency
	if maxConcurrency <= 0 {
		maxConcurrency = 5
	}
	startDelay := e.requestStartDelay
	if startDelay < 0 {
		startDelay = 0
	}
	if e.requestStartDelay == 0 {
		startDelay = time.Second
	}
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	responses := make([]segmentResponse, len(segments))
	sem := make(chan struct{}, maxConcurrency)
	limiter := newRequestStartLimiter(startDelay)
	var wg sync.WaitGroup
	errCh := make(chan error, 1)
	for i, segment := range segments {
		wg.Add(1)
		go func(i int, segment audio.PreparedSegment) {
			defer wg.Done()
			select {
			case sem <- struct{}{}:
			case <-ctx.Done():
				responses[i] = segmentResponse{segment: segment, err: ctx.Err()}
				return
			}
			defer func() { <-sem }()
			dataURL, err := wavDataURL(segment.Path)
			if err != nil {
				responses[i] = segmentResponse{segment: segment, err: err}
				sendFirstError(errCh, cancel, err)
				return
			}
			if err := limiter.Wait(ctx); err != nil {
				responses[i] = segmentResponse{segment: segment, err: err}
				return
			}
			response, err := e.client.TranscribeDataURL(ctx, dataURL, language)
			if err != nil {
				responses[i] = segmentResponse{segment: segment, err: err}
				sendFirstError(errCh, cancel, err)
				return
			}
			responses[i] = segmentResponse{segment: segment, response: response}
		}(i, segment)
	}
	wg.Wait()
	select {
	case err := <-errCh:
		return responses, err
	default:
		return responses, nil
	}
}

func sendFirstError(errCh chan<- error, cancel context.CancelFunc, err error) {
	select {
	case errCh <- err:
		cancel()
	default:
	}
}

type requestStartLimiter struct {
	mu    sync.Mutex
	delay time.Duration
	next  time.Time
}

func newRequestStartLimiter(delay time.Duration) *requestStartLimiter {
	return &requestStartLimiter{delay: delay}
}

func (l *requestStartLimiter) Wait(ctx context.Context) error {
	if l.delay <= 0 {
		return nil
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	now := time.Now()
	if !l.next.IsZero() && now.Before(l.next) {
		timer := time.NewTimer(time.Until(l.next))
		select {
		case <-timer.C:
		case <-ctx.Done():
			if !timer.Stop() {
				select {
				case <-timer.C:
				default:
				}
			}
			return ctx.Err()
		}
	}
	l.next = time.Now().Add(l.delay)
	return nil
}

func wavDataURL(path string) (string, error) {
	body, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	return "data:audio/wav;base64," + base64.StdEncoding.EncodeToString(body), nil
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

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

func formatClock(value time.Duration) string {
	if value < 0 {
		value = 0
	}
	total := int64(value / time.Second)
	hours := total / 3600
	minutes := (total % 3600) / 60
	seconds := total % 60
	return fmt.Sprintf("%02d:%02d:%02d", hours, minutes, seconds)
}
