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
	Detector Detector
	Options  SegmentOptions
	TempDir  string

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
	duration, err := ProbeDuration(ctx, inputPath)
	if err != nil {
		return nil, err
	}
	tempDir, err := os.MkdirTemp(p.TempDir, "easy_asr-mimo-*")
	if err != nil {
		return nil, err
	}
	p.tempDir = tempDir
	normalized := filepath.Join(tempDir, "normalized.wav")
	if err := NormalizeWAV(ctx, inputPath, normalized); err != nil {
		_ = p.Cleanup()
		return nil, err
	}
	speech, err := p.Detector.Detect(ctx, normalized)
	if err != nil {
		_ = p.Cleanup()
		return nil, err
	}
	plan := PlanSegments(duration, speech, p.Options)
	prepared := make([]PreparedSegment, 0, len(plan))
	for _, segment := range plan {
		path := filepath.Join(tempDir, fmt.Sprintf("part-%03d.wav", segment.Index))
		if err := CutWAV(ctx, inputPath, path, segment.Start, segment.End-segment.Start); err != nil {
			_ = p.Cleanup()
			return nil, err
		}
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
