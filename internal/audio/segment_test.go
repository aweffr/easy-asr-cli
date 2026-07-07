package audio_test

import (
	"testing"
	"time"

	"github.com/aweffr/easy-asr-cli/internal/audio"
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
