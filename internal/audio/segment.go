package audio

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/aweffr/easy-asr-cli/internal/observe"
)

type SpeechSegment struct {
	Start time.Duration
	End   time.Duration
}

type SegmentOptions struct {
	TargetDuration time.Duration
	MinDuration    time.Duration
	MaxDuration    time.Duration
}

type Segment struct {
	Index int
	Total int
	Start time.Duration
	End   time.Duration
}

type PreparedSegment struct {
	Index int
	Total int
	Start time.Duration
	End   time.Duration
	Path  string
}

type Detector interface {
	Detect(ctx context.Context, wavPath string) ([]SpeechSegment, error)
}

type Processor struct {
	Detector  Detector
	Options   SegmentOptions
	TempDir   string
	Observer  observe.Observer
	Probe     func(ctx context.Context, path string) (time.Duration, error)
	Normalize func(ctx context.Context, inputPath string, outputPath string) error
	Cut       func(ctx context.Context, inputPath string, outputPath string, start time.Duration, duration time.Duration) error

	tempDir string
}

func DefaultSegmentOptions() SegmentOptions {
	return SegmentOptions{
		TargetDuration: 3 * time.Minute,
		MinDuration:    150 * time.Second,
		MaxDuration:    210 * time.Second,
	}
}

func PlanSegments(duration time.Duration, speech []SpeechSegment, options SegmentOptions) []Segment {
	options = fillSegmentOptions(options)
	if duration <= 0 {
		return nil
	}
	var out []Segment
	start := time.Duration(0)
	for start < duration {
		remaining := duration - start
		if remaining <= options.MaxDuration || start+options.MaxDuration >= duration {
			out = append(out, Segment{Start: start, End: duration})
			break
		}
		windowStart := start + options.MinDuration
		windowEnd := start + options.MaxDuration
		target := start + options.TargetDuration
		cut, ok := bestBoundary(speech, windowStart, windowEnd, target)
		if !ok {
			cut = windowEnd
		}
		if cut <= start {
			cut = minDuration(start+options.MaxDuration, duration)
		}
		out = append(out, Segment{Start: start, End: cut})
		start = cut
	}
	total := len(out)
	for i := range out {
		out[i].Index = i + 1
		out[i].Total = total
	}
	return out
}

func (p *Processor) Prepare(ctx context.Context, inputPath string) ([]PreparedSegment, error) {
	if p.Detector == nil {
		return nil, fmt.Errorf("audio detector is required")
	}
	probe := p.Probe
	if probe == nil {
		probe = ProbeDuration
	}
	normalize := p.Normalize
	if normalize == nil {
		normalize = NormalizeWAV
	}
	cut := p.Cut
	if cut == nil {
		cut = CutWAV
	}
	probeStart := time.Now()
	emit(p.Observer, observe.Event{Event: "audio.probe.started", Step: "probe", Message: "probing audio duration"})
	duration, err := probe(ctx, inputPath)
	if err != nil {
		emitFailed(p.Observer, "audio.probe.failed", "probe", probeStart, err)
		return nil, err
	}
	emit(p.Observer, observe.Event{Event: "audio.probe.completed", Step: "probe", ElapsedMS: elapsedMS(probeStart), EndSeconds: duration.Seconds(), Message: "audio duration probed"})
	tempDir, err := os.MkdirTemp(p.TempDir, "easy_asr-mimo-*")
	if err != nil {
		return nil, err
	}
	p.tempDir = tempDir
	normalized := filepath.Join(tempDir, "normalized.wav")
	normalizeStart := time.Now()
	emit(p.Observer, observe.Event{Event: "audio.normalize.started", Step: "normalize", Message: "normalizing audio to 16k mono WAV"})
	if err := normalize(ctx, inputPath, normalized); err != nil {
		emitFailed(p.Observer, "audio.normalize.failed", "normalize", normalizeStart, err)
		_ = p.Cleanup()
		return nil, err
	}
	emit(p.Observer, observe.Event{Event: "audio.normalize.completed", Step: "normalize", ElapsedMS: elapsedMS(normalizeStart), Message: "audio normalized to 16k mono WAV"})
	vadStart := time.Now()
	emit(p.Observer, observe.Event{Event: "vad.detect.started", Step: "vad", Message: "detecting speech regions"})
	speech, err := p.Detector.Detect(ctx, normalized)
	if err != nil {
		emitFailed(p.Observer, "vad.detect.failed", "vad", vadStart, err)
		_ = p.Cleanup()
		return nil, err
	}
	emit(p.Observer, observe.Event{Event: "vad.detect.completed", Step: "vad", ElapsedMS: elapsedMS(vadStart), SegmentTotal: len(speech), Message: "speech regions detected"})
	plan := PlanSegments(duration, speech, p.Options)
	emit(p.Observer, observe.Event{Event: "audio.segment_plan.completed", Step: "segment_plan", SegmentTotal: len(plan), EndSeconds: duration.Seconds(), Message: "audio segment plan created"})
	prepared := make([]PreparedSegment, 0, len(plan))
	for _, segment := range plan {
		path := filepath.Join(tempDir, fmt.Sprintf("part-%03d.wav", segment.Index))
		cutStart := time.Now()
		emit(p.Observer, segmentEvent("audio.cut_segment.started", segment, observe.Event{Step: "cut", Message: "cutting audio segment"}))
		if err := cut(ctx, inputPath, path, segment.Start, segment.End-segment.Start); err != nil {
			emitFailed(p.Observer, "audio.cut_segment.failed", "cut", cutStart, err)
			_ = p.Cleanup()
			return nil, err
		}
		emit(p.Observer, segmentEvent("audio.cut_segment.completed", segment, observe.Event{Step: "cut", ElapsedMS: elapsedMS(cutStart), Message: "audio segment cut"}))
		prepared = append(prepared, PreparedSegment{
			Index: segment.Index,
			Total: segment.Total,
			Start: segment.Start,
			End:   segment.End,
			Path:  path,
		})
	}
	return prepared, nil
}

func emit(observer observe.Observer, event observe.Event) {
	if observer != nil {
		observer.Emit(event)
	}
}

func emitFailed(observer observe.Observer, name string, step string, start time.Time, err error) {
	emit(observer, observe.Event{
		Event:     name,
		Level:     "error",
		Step:      step,
		ElapsedMS: elapsedMS(start),
		Error:     err.Error(),
		ErrorType: fmt.Sprintf("%T", err),
	})
}

func segmentEvent(name string, segment Segment, event observe.Event) observe.Event {
	event.Event = name
	event.SegmentIndex = segment.Index
	event.SegmentTotal = segment.Total
	event.StartSeconds = segment.Start.Seconds()
	event.EndSeconds = segment.End.Seconds()
	return event
}

func elapsedMS(start time.Time) int64 {
	return time.Since(start).Milliseconds()
}

func (p *Processor) Cleanup() error {
	if p.tempDir == "" {
		return nil
	}
	err := os.RemoveAll(p.tempDir)
	p.tempDir = ""
	return err
}

func ProbeDuration(ctx context.Context, path string) (time.Duration, error) {
	out, err := runCommand(ctx, "ffprobe", "-v", "error", "-show_entries", "format=duration", "-of", "json", path)
	if err != nil {
		return 0, err
	}
	var payload struct {
		Format struct {
			Duration string `json:"duration"`
		} `json:"format"`
	}
	if err := json.Unmarshal(out, &payload); err != nil {
		return 0, fmt.Errorf("parse ffprobe duration: %w", err)
	}
	seconds, err := parseFloat(payload.Format.Duration)
	if err != nil {
		return 0, fmt.Errorf("parse ffprobe duration %q: %w", payload.Format.Duration, err)
	}
	return time.Duration(seconds * float64(time.Second)), nil
}

func NormalizeWAV(ctx context.Context, inputPath string, outputPath string) error {
	_, err := runCommand(ctx, "ffmpeg", "-hide_banner", "-loglevel", "error", "-y", "-i", inputPath, "-ac", "1", "-ar", "16000", "-c:a", "pcm_s16le", outputPath)
	return err
}

func CutWAV(ctx context.Context, inputPath string, outputPath string, start time.Duration, duration time.Duration) error {
	_, err := runCommand(ctx, "ffmpeg", "-hide_banner", "-loglevel", "error", "-y", "-ss", formatSeconds(start), "-t", formatSeconds(duration), "-i", inputPath, "-ac", "1", "-ar", "16000", "-c:a", "pcm_s16le", outputPath)
	return err
}

func runCommand(ctx context.Context, name string, args ...string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, name, args...)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		msg := strings.TrimSpace(stderr.String())
		if msg != "" {
			return nil, fmt.Errorf("%s failed: %w: %s", name, err, msg)
		}
		return nil, fmt.Errorf("%s failed: %w", name, err)
	}
	return out, nil
}

func bestBoundary(speech []SpeechSegment, windowStart, windowEnd, target time.Duration) (time.Duration, bool) {
	var best time.Duration
	bestDistance := time.Duration(math.MaxInt64)
	found := false
	for _, segment := range speech {
		candidate := segment.Start
		if candidate < windowStart || candidate > windowEnd {
			continue
		}
		distance := absDuration(candidate - target)
		if !found || distance < bestDistance || (distance == bestDistance && candidate > best) {
			best = candidate
			bestDistance = distance
			found = true
		}
	}
	return best, found
}

func fillSegmentOptions(options SegmentOptions) SegmentOptions {
	defaults := DefaultSegmentOptions()
	if options.TargetDuration <= 0 {
		options.TargetDuration = defaults.TargetDuration
	}
	if options.MinDuration <= 0 {
		options.MinDuration = defaults.MinDuration
	}
	if options.MaxDuration <= 0 {
		options.MaxDuration = defaults.MaxDuration
	}
	return options
}

func formatSeconds(value time.Duration) string {
	return fmt.Sprintf("%.3f", value.Seconds())
}

func parseFloat(value string) (float64, error) {
	var out float64
	_, err := fmt.Sscanf(value, "%f", &out)
	return out, err
}

func absDuration(value time.Duration) time.Duration {
	if value < 0 {
		return -value
	}
	return value
}

func minDuration(a, b time.Duration) time.Duration {
	if a < b {
		return a
	}
	return b
}
