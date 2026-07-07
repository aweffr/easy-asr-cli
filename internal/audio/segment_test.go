package audio_test

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/aweffr/easy-asr-cli/internal/audio"
	"github.com/aweffr/easy-asr-cli/internal/observe"
)

func TestPlanSegmentsChoosesVADBoundaryInWindow(t *testing.T) {
	segments := audio.PlanSegments(10*time.Minute, []audio.SpeechSegment{
		{Start: 0, End: 179 * time.Second},
		{Start: 183 * time.Second, End: 300 * time.Second},
		{Start: 300 * time.Second, End: 359 * time.Second},
		{Start: 362 * time.Second, End: 600 * time.Second},
	}, audio.SegmentOptions{
		TargetDuration: 180 * time.Second,
		MinDuration:    150 * time.Second,
		MaxDuration:    210 * time.Second,
	})

	if len(segments) != 4 {
		t.Fatalf("len(segments) = %d, want 4: %#v", len(segments), segments)
	}
	if segments[0].End != 183*time.Second {
		t.Fatalf("first cut = %s, want 183s", segments[0].End)
	}
	if segments[0].Index != 1 || segments[0].Total != 4 {
		t.Fatalf("part metadata = %#v", segments[0])
	}
}

func TestPlanSegmentsHardCutsAtMaxWhenNoBoundaryExists(t *testing.T) {
	segments := audio.PlanSegments(6*time.Minute, []audio.SpeechSegment{
		{Start: 0, End: 6 * time.Minute},
	}, audio.SegmentOptions{
		TargetDuration: 180 * time.Second,
		MinDuration:    150 * time.Second,
		MaxDuration:    210 * time.Second,
	})

	if len(segments) != 2 {
		t.Fatalf("len(segments) = %d, want 2: %#v", len(segments), segments)
	}
	if segments[0].End != 210*time.Second {
		t.Fatalf("first cut = %s, want 210s", segments[0].End)
	}
}

func TestPlanSegmentsKeepsShortAudioAsSinglePart(t *testing.T) {
	segments := audio.PlanSegments(90*time.Second, nil, audio.SegmentOptions{
		TargetDuration: 180 * time.Second,
		MinDuration:    150 * time.Second,
		MaxDuration:    210 * time.Second,
	})

	if len(segments) != 1 {
		t.Fatalf("len(segments) = %d, want 1", len(segments))
	}
	if segments[0].Index != 1 || segments[0].Total != 1 || segments[0].End != 90*time.Second {
		t.Fatalf("segment = %#v", segments[0])
	}
}

func TestProcessorEmitsPreprocessingStepEvents(t *testing.T) {
	dir := t.TempDir()
	recorder := &recordingObserver{}
	var cutCount int
	processor := &audio.Processor{
		Detector: fakeDetector{segments: []audio.SpeechSegment{{Start: 0, End: 6 * time.Minute}}},
		Options: audio.SegmentOptions{
			TargetDuration: 180 * time.Second,
			MinDuration:    150 * time.Second,
			MaxDuration:    210 * time.Second,
		},
		TempDir:  dir,
		Observer: recorder,
		Probe: func(ctx context.Context, path string) (time.Duration, error) {
			return 6 * time.Minute, nil
		},
		Normalize: func(ctx context.Context, inputPath string, outputPath string) error {
			return nil
		},
		Cut: func(ctx context.Context, inputPath string, outputPath string, start time.Duration, duration time.Duration) error {
			cutCount++
			return nil
		},
	}

	segments, err := processor.Prepare(context.Background(), filepath.Join(dir, "input.mp3"))
	if err != nil {
		t.Fatalf("Prepare returned error: %v", err)
	}
	if len(segments) != 2 || cutCount != 2 {
		t.Fatalf("segments=%#v cutCount=%d", segments, cutCount)
	}
	names := eventNames(recorder.events)
	for _, want := range []string{
		"audio.probe.started",
		"audio.probe.completed",
		"audio.normalize.started",
		"audio.normalize.completed",
		"vad.detect.started",
		"vad.detect.completed",
		"audio.segment_plan.completed",
		"audio.cut_segment.started",
		"audio.cut_segment.completed",
	} {
		if !contains(names, want) {
			t.Fatalf("events missing %q: %#v", want, names)
		}
	}
	plan := findEvent(recorder.events, "audio.segment_plan.completed")
	if plan.SegmentTotal != 2 {
		t.Fatalf("plan event = %#v", plan)
	}
	cut := findEvent(recorder.events, "audio.cut_segment.completed")
	if cut.SegmentIndex != 1 || cut.SegmentTotal != 2 || cut.EndSeconds <= cut.StartSeconds {
		t.Fatalf("cut event = %#v", cut)
	}
}

type fakeDetector struct {
	segments []audio.SpeechSegment
}

func (f fakeDetector) Detect(ctx context.Context, wavPath string) ([]audio.SpeechSegment, error) {
	return f.segments, nil
}

type recordingObserver struct {
	events []observe.Event
}

func (r *recordingObserver) Emit(event observe.Event) {
	r.events = append(r.events, event)
}

func eventNames(events []observe.Event) []string {
	out := make([]string, 0, len(events))
	for _, event := range events {
		out = append(out, event.Event)
	}
	return out
}

func contains(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}

func findEvent(events []observe.Event, name string) observe.Event {
	for _, event := range events {
		if event.Event == name {
			return event
		}
	}
	return observe.Event{}
}
