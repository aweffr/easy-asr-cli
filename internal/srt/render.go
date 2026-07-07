package srt

import (
	"errors"
	"fmt"
	"sort"
	"strings"
	"unicode"
	"unicode/utf8"
)

type Transcription struct {
	Transcripts []Transcript `json:"transcripts"`
}

type Transcript struct {
	ChannelID int        `json:"channel_id"`
	Text      string     `json:"text"`
	Sentences []Sentence `json:"sentences"`
}

type Sentence struct {
	SentenceID int    `json:"sentence_id"`
	BeginTime  int64  `json:"begin_time"`
	EndTime    int64  `json:"end_time"`
	Language   string `json:"language,omitempty"`
	Emotion    string `json:"emotion,omitempty"`
	Text       string `json:"text"`
	SpeakerID  *int   `json:"speaker_id,omitempty"`
	Words      []Word `json:"words"`
}

type Word struct {
	BeginTime   int64  `json:"begin_time"`
	EndTime     int64  `json:"end_time"`
	Text        string `json:"text"`
	Punctuation string `json:"punctuation,omitempty"`
}

type Options struct {
	MaxCueDuration int64
	MaxCueRunes    int
	SpeakerLabels  bool
}

type cue struct {
	start     int64
	end       int64
	text      string
	speakerID *int
}

func Render(payload Transcription, options Options) (string, error) {
	if options.MaxCueDuration <= 0 {
		options.MaxCueDuration = 6_000
	}
	if options.MaxCueRunes <= 0 {
		options.MaxCueRunes = 42
	}
	var cues []cue
	for _, transcript := range payload.Transcripts {
		for _, sentence := range transcript.Sentences {
			if len(sentence.Words) > 0 {
				cues = append(cues, cuesFromWords(sentence, options)...)
				continue
			}
			text := strings.TrimSpace(sentence.Text)
			if text == "" || sentence.EndTime <= sentence.BeginTime {
				continue
			}
			cues = append(cues, cue{start: sentence.BeginTime, end: sentence.EndTime, text: text, speakerID: sentence.SpeakerID})
		}
	}
	if len(cues) == 0 {
		return "", errors.New("no timed transcription cues found")
	}
	sort.SliceStable(cues, func(i, j int) bool {
		if cues[i].start == cues[j].start {
			return cues[i].end < cues[j].end
		}
		return cues[i].start < cues[j].start
	})

	var builder strings.Builder
	for i, c := range cues {
		if i > 0 {
			builder.WriteString("\n")
		}
		text := c.text
		if options.SpeakerLabels && c.speakerID != nil {
			text = fmt.Sprintf("[SPEAKER_%d] %s", *c.speakerID, text)
		}
		fmt.Fprintf(
			&builder,
			"%d\n%s --> %s\n%s\n",
			i+1,
			formatTimestamp(c.start),
			formatTimestamp(c.end),
			text,
		)
	}
	return builder.String(), nil
}

func cuesFromWords(sentence Sentence, options Options) []cue {
	var out []cue
	var current cue
	flush := func() {
		if current.text == "" || current.end <= current.start {
			current = cue{}
			return
		}
		current.text = strings.TrimSpace(current.text)
		if current.text != "" {
			out = append(out, current)
		}
		current = cue{}
	}
	for _, word := range sentence.Words {
		text := strings.TrimSpace(word.Text)
		if word.Punctuation != "" {
			text += word.Punctuation
		}
		if strings.TrimSpace(text) == "" || word.EndTime <= word.BeginTime {
			continue
		}
		if current.text == "" {
			current = cue{start: word.BeginTime, end: word.EndTime, text: text, speakerID: sentence.SpeakerID}
			continue
		}
		candidateDuration := word.EndTime - current.start
		candidateRunes := utf8.RuneCountInString(current.text + text)
		if candidateDuration > options.MaxCueDuration || candidateRunes > options.MaxCueRunes {
			flush()
			current = cue{start: word.BeginTime, end: word.EndTime, text: text, speakerID: sentence.SpeakerID}
			continue
		}
		current.end = word.EndTime
		if needsASCIISpace(current.text, text) {
			current.text += " "
		}
		current.text += text
	}
	flush()
	return out
}

func needsASCIISpace(left string, right string) bool {
	if left == "" || right == "" || strings.HasSuffix(left, " ") || strings.HasPrefix(right, " ") {
		return false
	}
	var last rune
	for _, r := range left {
		last = r
	}
	first, _ := utf8.DecodeRuneInString(right)
	return isASCIIAlnum(last) && isASCIIAlnum(first)
}

func isASCIIAlnum(r rune) bool {
	return r <= unicode.MaxASCII && (unicode.IsLetter(r) || unicode.IsDigit(r))
}

func formatTimestamp(ms int64) string {
	if ms < 0 {
		ms = 0
	}
	hours := ms / 3_600_000
	ms %= 3_600_000
	minutes := ms / 60_000
	ms %= 60_000
	seconds := ms / 1_000
	millis := ms % 1_000
	return fmt.Sprintf("%02d:%02d:%02d,%03d", hours, minutes, seconds, millis)
}
