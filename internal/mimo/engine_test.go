package mimo_test

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/aweffr/easy-asr-cli/internal/audio"
	"github.com/aweffr/easy-asr-cli/internal/config"
	"github.com/aweffr/easy-asr-cli/internal/engine"
	"github.com/aweffr/easy-asr-cli/internal/mimo"
)

func TestEngineTranscribesSegmentsWritesRawWrapperAndSRTLabels(t *testing.T) {
	dir := t.TempDir()
	audioPath := filepath.Join(dir, "input.mp3")
	if err := os.WriteFile(audioPath, []byte("audio"), 0o600); err != nil {
		t.Fatalf("write audio: %v", err)
	}
	outPath := filepath.Join(dir, "out.srt")
	rawPath := filepath.Join(dir, "raw.json")
	part1Path := filepath.Join(dir, "part1.wav")
	part2Path := filepath.Join(dir, "part2.wav")
	if err := os.WriteFile(part1Path, []byte("part-one"), 0o600); err != nil {
		t.Fatalf("write part1: %v", err)
	}
	if err := os.WriteFile(part2Path, []byte("part-two"), 0o600); err != nil {
		t.Fatalf("write part2: %v", err)
	}
	processor := &fakeProcessor{segments: []audio.PreparedSegment{
		{Index: 1, Total: 2, Start: 0, End: 180 * time.Second, Path: part1Path},
		{Index: 2, Total: 2, Start: 180 * time.Second, End: 360 * time.Second, Path: part2Path},
	}}
	client := &fakeMimoClient{responses: []mimo.TranscriptionResponse{
		{ID: "r1", Content: "第一段", FinishReason: "stop", UsageSeconds: 181, Raw: map[string]any{"id": "r1"}},
		{ID: "r2", Content: "第二段", FinishReason: "length", UsageSeconds: 179, Raw: map[string]any{"id": "r2"}},
	}}
	runner := mimo.NewEngine(mimo.EngineOptions{
		Config:    validMimoConfig(),
		Processor: processor,
		Client:    client,
	})

	result, err := runner.Transcribe(context.Background(), engine.Request{
		AudioPath:   audioPath,
		OutputPath:  outPath,
		RawJSONPath: rawPath,
		Language:    "zh",
	})
	if err != nil {
		t.Fatalf("Transcribe returned error: %v", err)
	}
	if result.Engine != config.EngineMimoV25ASR || result.UsageSeconds != 360 {
		t.Fatalf("result = %#v", result)
	}
	if len(result.Warnings) != 1 || !strings.Contains(result.Warnings[0], "finish_reason=length") {
		t.Fatalf("warnings = %#v", result.Warnings)
	}
	body, err := os.ReadFile(outPath)
	if err != nil {
		t.Fatalf("read srt: %v", err)
	}
	srtBody := string(body)
	for _, want := range []string{
		"[PART 1/2 00:00:00-00:03:00] 第一段",
		"[PART 2/2 00:03:00-00:06:00][TRUNCATED] 第二段",
		"00:03:00,000 --> 00:06:00,000",
	} {
		if !strings.Contains(srtBody, want) {
			t.Fatalf("SRT missing %q:\n%s", want, srtBody)
		}
	}
	var raw map[string]any
	if err := json.Unmarshal(mustRead(t, rawPath), &raw); err != nil {
		t.Fatalf("raw json invalid: %v", err)
	}
	if raw["engine"] != config.EngineMimoV25ASR || int(raw["usage_seconds"].(float64)) != 360 {
		t.Fatalf("raw wrapper = %#v", raw)
	}
	rawSegments := raw["segments"].([]any)
	if len(rawSegments) != 2 {
		t.Fatalf("segments = %#v", rawSegments)
	}
	if rawSegments[1].(map[string]any)["finish_reason"] != "length" {
		t.Fatalf("second segment = %#v", rawSegments[1])
	}
	if !processor.cleaned {
		t.Fatal("processor cleanup was not called")
	}
	if len(client.dataURLs) != 2 || client.languages[0] != "zh" {
		t.Fatalf("client calls = %#v languages=%#v", client.dataURLs, client.languages)
	}
}

func TestEngineRunsMimoRequestsWithBoundedParallelismAndOrderedOutput(t *testing.T) {
	dir := t.TempDir()
	audioPath := filepath.Join(dir, "input.mp3")
	if err := os.WriteFile(audioPath, []byte("audio"), 0o600); err != nil {
		t.Fatalf("write audio: %v", err)
	}
	var segments []audio.PreparedSegment
	for i := 1; i <= 6; i++ {
		path := filepath.Join(dir, "part-"+time.Duration(i).String()+".wav")
		if err := os.WriteFile(path, []byte{byte(i)}, 0o600); err != nil {
			t.Fatalf("write segment: %v", err)
		}
		segments = append(segments, audio.PreparedSegment{
			Index: i,
			Total: 6,
			Start: time.Duration(i-1) * time.Second,
			End:   time.Duration(i) * time.Second,
			Path:  path,
		})
	}
	client := &parallelMimoClient{
		sleep:     30 * time.Millisecond,
		responses: make(map[string]mimo.TranscriptionResponse),
	}
	for i := 1; i <= 6; i++ {
		client.responses[string([]byte{byte(i)})] = mimo.TranscriptionResponse{
			ID:           string(rune('a' + i)),
			Content:      "part " + string(rune('0'+i)),
			FinishReason: "stop",
			UsageSeconds: int64(i),
			Raw:          map[string]any{"index": i},
		}
	}
	runner := mimo.NewEngine(mimo.EngineOptions{
		Config:            validMimoConfig(),
		Processor:         &fakeProcessor{segments: segments},
		Client:            client,
		MaxConcurrency:    2,
		RequestStartDelay: 5 * time.Millisecond,
	})
	outPath := filepath.Join(dir, "out.srt")
	rawPath := filepath.Join(dir, "raw.json")

	result, err := runner.Transcribe(context.Background(), engine.Request{
		AudioPath:   audioPath,
		OutputPath:  outPath,
		RawJSONPath: rawPath,
	})
	if err != nil {
		t.Fatalf("Transcribe returned error: %v", err)
	}
	if client.maxActive > 2 {
		t.Fatalf("max active requests = %d, want <= 2", client.maxActive)
	}
	if len(client.starts) != 6 {
		t.Fatalf("starts = %d, want 6", len(client.starts))
	}
	sortedStarts := append([]time.Time(nil), client.starts...)
	sort.Slice(sortedStarts, func(i, j int) bool { return sortedStarts[i].Before(sortedStarts[j]) })
	for i := 1; i < len(sortedStarts); i++ {
		if sortedStarts[i].Sub(sortedStarts[i-1]) < 4*time.Millisecond {
			t.Fatalf("request starts were not rate-limited: %v then %v", sortedStarts[i-1], sortedStarts[i])
		}
	}
	if result.UsageSeconds != 21 {
		t.Fatalf("UsageSeconds = %d, want 21", result.UsageSeconds)
	}
	body := string(mustRead(t, outPath))
	if strings.Index(body, "part 1") > strings.Index(body, "part 6") {
		t.Fatalf("SRT output is not ordered by segment:\n%s", body)
	}
	var raw map[string]any
	if err := json.Unmarshal(mustRead(t, rawPath), &raw); err != nil {
		t.Fatalf("raw json invalid: %v", err)
	}
	rawSegments := raw["segments"].([]any)
	if int(rawSegments[0].(map[string]any)["index"].(float64)) != 1 || int(rawSegments[5].(map[string]any)["index"].(float64)) != 6 {
		t.Fatalf("raw segments not ordered: %#v", rawSegments)
	}
}

type fakeProcessor struct {
	segments []audio.PreparedSegment
	cleaned  bool
}

func (f *fakeProcessor) Prepare(ctx context.Context, path string) ([]audio.PreparedSegment, error) {
	return f.segments, nil
}

func (f *fakeProcessor) Cleanup() error {
	f.cleaned = true
	return nil
}

type fakeMimoClient struct {
	responses []mimo.TranscriptionResponse
	dataURLs  []string
	languages []string
}

func (f *fakeMimoClient) TranscribeDataURL(ctx context.Context, dataURL string, language string) (mimo.TranscriptionResponse, error) {
	f.dataURLs = append(f.dataURLs, dataURL)
	f.languages = append(f.languages, language)
	response := f.responses[0]
	f.responses = f.responses[1:]
	return response, nil
}

type parallelMimoClient struct {
	mu        sync.Mutex
	sleep     time.Duration
	active    int
	maxActive int
	starts    []time.Time
	responses map[string]mimo.TranscriptionResponse
}

func (p *parallelMimoClient) TranscribeDataURL(ctx context.Context, dataURL string, language string) (mimo.TranscriptionResponse, error) {
	payload := strings.TrimPrefix(dataURL, "data:audio/wav;base64,")
	p.mu.Lock()
	p.active++
	if p.active > p.maxActive {
		p.maxActive = p.active
	}
	p.starts = append(p.starts, time.Now())
	p.mu.Unlock()

	select {
	case <-time.After(p.sleep):
	case <-ctx.Done():
		return mimo.TranscriptionResponse{}, ctx.Err()
	}

	p.mu.Lock()
	p.active--
	response := p.responses[mustBase64Decode(payload)]
	p.mu.Unlock()
	return response, nil
}

func validMimoConfig() *config.MiMoConfig {
	cfg := config.Default().MiMoV25ASR()
	cfg.MiMo.APIKey = "mimo-key"
	return cfg
}

func mustRead(t *testing.T, path string) []byte {
	t.Helper()
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return body
}

func mustBase64Decode(value string) string {
	body, err := base64.StdEncoding.DecodeString(value)
	if err != nil {
		panic(err)
	}
	return string(body)
}
