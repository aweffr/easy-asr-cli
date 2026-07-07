package observe

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"sync"
	"time"
)

type Observer interface {
	Emit(Event)
}

type Event struct {
	Timestamp      string  `json:"ts,omitempty"`
	Level          string  `json:"level,omitempty"`
	Event          string  `json:"event"`
	Engine         string  `json:"engine,omitempty"`
	RunID          string  `json:"run_id,omitempty"`
	Step           string  `json:"step,omitempty"`
	Message        string  `json:"message,omitempty"`
	ElapsedMS      int64   `json:"elapsed_ms,omitempty"`
	SegmentIndex   int     `json:"segment_index,omitempty"`
	SegmentTotal   int     `json:"segment_total,omitempty"`
	StartSeconds   float64 `json:"start_seconds,omitempty"`
	EndSeconds     float64 `json:"end_seconds,omitempty"`
	Attempt        int     `json:"attempt,omitempty"`
	BackoffSeconds float64 `json:"backoff_seconds,omitempty"`
	UsageSeconds   int64   `json:"usage_seconds,omitempty"`
	ErrorType      string  `json:"error_type,omitempty"`
	Error          string  `json:"error,omitempty"`
}

type Format string

const (
	FormatHuman Format = "human"
	FormatJSONL Format = "jsonl"
)

type Logger struct {
	writer io.Writer
	format Format
	runID  string
	start  time.Time
	mu     sync.Mutex
}

func New(writer io.Writer, format Format) *Logger {
	return &Logger{
		writer: writer,
		format: format,
		runID:  NewRunID(),
		start:  time.Now(),
	}
}

func NewRunID() string {
	return "run-" + time.Now().UTC().Format("20060102T150405.000000000Z")
}

func (l *Logger) Emit(event Event) {
	if l == nil || l.writer == nil || strings.TrimSpace(event.Event) == "" {
		return
	}
	if event.Timestamp == "" {
		event.Timestamp = time.Now().UTC().Format(time.RFC3339Nano)
	}
	if event.Level == "" {
		event.Level = "info"
	}
	if event.RunID == "" {
		event.RunID = l.runID
	}
	if event.ElapsedMS == 0 {
		event.ElapsedMS = time.Since(l.start).Milliseconds()
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.format == FormatJSONL {
		_ = json.NewEncoder(l.writer).Encode(event)
		return
	}
	fmt.Fprintf(l.writer, "[%s] %-5s %s", formatElapsed(event.ElapsedMS), strings.ToUpper(event.Level), event.Event)
	if event.Step != "" {
		fmt.Fprintf(l.writer, " step=%s", event.Step)
	}
	if event.SegmentIndex > 0 {
		fmt.Fprintf(l.writer, " segment=%d/%d", event.SegmentIndex, event.SegmentTotal)
	}
	if event.Attempt > 0 {
		fmt.Fprintf(l.writer, " attempt=%d", event.Attempt)
	}
	if event.BackoffSeconds > 0 {
		fmt.Fprintf(l.writer, " backoff=%.0fs", event.BackoffSeconds)
	}
	if event.Message != "" {
		fmt.Fprintf(l.writer, " %s", event.Message)
	}
	if event.Error != "" {
		fmt.Fprintf(l.writer, " error=%s", event.Error)
	}
	fmt.Fprintln(l.writer)
}

func formatElapsed(ms int64) string {
	if ms < 0 {
		ms = 0
	}
	return fmt.Sprintf("%02d:%02d.%03d", ms/60000, (ms/1000)%60, ms%1000)
}
