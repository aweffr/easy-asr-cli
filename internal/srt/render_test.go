package srt_test

import (
	"strings"
	"testing"

	"github.com/aweffr/easy-asr-cli/internal/srt"
)

func TestRenderPrefersWordTimestampsAndSplitsReadableCues(t *testing.T) {
	payload := srt.Transcription{
		Transcripts: []srt.Transcript{{
			Sentences: []srt.Sentence{{
				BeginTime: 0,
				EndTime:   8000,
				Text:      "第一句比较长第二句继续",
				Words: []srt.Word{
					{BeginTime: 0, EndTime: 1000, Text: "第一句"},
					{BeginTime: 1000, EndTime: 2500, Text: "比较长"},
					{BeginTime: 6200, EndTime: 8000, Text: "第二句继续"},
				},
			}},
		}},
	}

	got, err := srt.Render(payload, srt.Options{MaxCueDuration: 5_000, MaxCueRunes: 12})
	if err != nil {
		t.Fatalf("Render returned error: %v", err)
	}

	wantParts := []string{
		"1\n00:00:00,000 --> 00:00:02,500\n第一句比较长",
		"2\n00:00:06,200 --> 00:00:08,000\n第二句继续",
	}
	for _, part := range wantParts {
		if !strings.Contains(got, part) {
			t.Fatalf("rendered SRT missing %q:\n%s", part, got)
		}
	}
}

func TestRenderFallsBackToSentenceTimestamps(t *testing.T) {
	payload := srt.Transcription{
		Transcripts: []srt.Transcript{{
			Sentences: []srt.Sentence{{
				BeginTime: 1234,
				EndTime:   4567,
				Text:      "只有句级时间戳",
			}},
		}},
	}

	got, err := srt.Render(payload, srt.Options{})
	if err != nil {
		t.Fatalf("Render returned error: %v", err)
	}
	if !strings.Contains(got, "00:00:01,234 --> 00:00:04,567") {
		t.Fatalf("sentence timestamp missing:\n%s", got)
	}
}

func TestRenderPreservesSpacesBetweenEnglishWords(t *testing.T) {
	payload := srt.Transcription{
		Transcripts: []srt.Transcript{{
			Sentences: []srt.Sentence{{
				BeginTime: 0,
				EndTime:   2000,
				Text:      "Hello world.",
				Words: []srt.Word{
					{BeginTime: 0, EndTime: 700, Text: "Hello"},
					{BeginTime: 800, EndTime: 1400, Text: "world", Punctuation: "."},
				},
			}},
		}},
	}

	got, err := srt.Render(payload, srt.Options{})
	if err != nil {
		t.Fatalf("Render returned error: %v", err)
	}
	if !strings.Contains(got, "Hello world.") {
		t.Fatalf("English words were not spaced correctly:\n%s", got)
	}
}

func TestRenderSortsCuesAcrossTranscripts(t *testing.T) {
	payload := srt.Transcription{
		Transcripts: []srt.Transcript{
			{Sentences: []srt.Sentence{{BeginTime: 5000, EndTime: 6000, Text: "later"}}},
			{Sentences: []srt.Sentence{{BeginTime: 1000, EndTime: 2000, Text: "earlier"}}},
		},
	}

	got, err := srt.Render(payload, srt.Options{})
	if err != nil {
		t.Fatalf("Render returned error: %v", err)
	}
	if strings.Index(got, "earlier") > strings.Index(got, "later") {
		t.Fatalf("cues not sorted by time:\n%s", got)
	}
}

func TestRenderFailsWhenNoTimedCueExists(t *testing.T) {
	_, err := srt.Render(srt.Transcription{
		Transcripts: []srt.Transcript{{Text: "plain text only"}},
	}, srt.Options{})
	if err == nil {
		t.Fatal("Render returned nil error for untimed transcription")
	}
}

func TestRenderPrefixesEveryCueWithSpeakerLabel(t *testing.T) {
	speakerID := 2
	got, err := srt.Render(srt.Transcription{Transcripts: []srt.Transcript{{
		Sentences: []srt.Sentence{{
			BeginTime: 0,
			EndTime:   7000,
			Text:      "alpha beta gamma delta epsilon zeta eta",
			SpeakerID: &speakerID,
			Words: []srt.Word{
				{BeginTime: 0, EndTime: 1000, Text: "alpha"},
				{BeginTime: 1000, EndTime: 2000, Text: "beta"},
				{BeginTime: 2000, EndTime: 3000, Text: "gamma"},
				{BeginTime: 3000, EndTime: 4000, Text: "delta"},
				{BeginTime: 4000, EndTime: 5000, Text: "epsilon"},
				{BeginTime: 5000, EndTime: 6000, Text: "zeta"},
				{BeginTime: 6000, EndTime: 7000, Text: "eta"},
			},
		}},
	}}}, srt.Options{SpeakerLabels: true, MaxCueDuration: 2500})
	if err != nil {
		t.Fatalf("Render returned error: %v", err)
	}
	if strings.Count(got, "[SPEAKER_2]") != 4 {
		t.Fatalf("speaker label should prefix every cue, got:\n%s", got)
	}
}
