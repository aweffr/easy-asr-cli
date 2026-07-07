package vad

import (
	"bytes"
	"context"
	"encoding/binary"
	"fmt"
	"io"
	"math"
	"os/exec"
	"sync"
	"time"

	ort "github.com/yalue/onnxruntime_go"

	"github.com/aweffr/easy-asr-cli/internal/audio"
)

const (
	sampleRate  = int64(16000)
	chunkSize   = 512
	contextSize = 64
	stateSize   = 2 * 1 * 128
)

var (
	initMu         sync.Mutex
	ortInitialized bool
)

type SileroOptions struct {
	ModelPath              string
	ONNXRuntimeLibraryPath string
	Threshold              float32
	MinSilence             time.Duration
	SpeechPad              time.Duration
}

type SileroDetector struct {
	modelPath  string
	threshold  float32
	minSilence time.Duration
	speechPad  time.Duration
}

func NewSileroDetector(options SileroOptions) (*SileroDetector, error) {
	if options.ModelPath == "" {
		return nil, fmt.Errorf("silero vad model path is required")
	}
	if options.Threshold == 0 {
		options.Threshold = 0.5
	}
	if options.MinSilence == 0 {
		options.MinSilence = 100 * time.Millisecond
	}
	if options.SpeechPad == 0 {
		options.SpeechPad = 30 * time.Millisecond
	}
	if err := initializeONNX(options.ONNXRuntimeLibraryPath); err != nil {
		return nil, err
	}
	return &SileroDetector{
		modelPath:  options.ModelPath,
		threshold:  options.Threshold,
		minSilence: options.MinSilence,
		speechPad:  options.SpeechPad,
	}, nil
}

func (d *SileroDetector) Detect(ctx context.Context, wavPath string) ([]audio.SpeechSegment, error) {
	inputData := make([]float32, contextSize+chunkSize)
	state := make([]float32, stateSize)
	srData := []int64{sampleRate}
	input, err := ort.NewTensor(ort.NewShape(1, int64(len(inputData))), inputData)
	if err != nil {
		return nil, err
	}
	defer input.Destroy()
	stateTensor, err := ort.NewTensor(ort.NewShape(2, 1, 128), state)
	if err != nil {
		return nil, err
	}
	defer stateTensor.Destroy()
	srTensor, err := ort.NewTensor(ort.NewShape(1), srData)
	if err != nil {
		return nil, err
	}
	defer srTensor.Destroy()
	output, err := ort.NewEmptyTensor[float32](ort.NewShape(1, 1))
	if err != nil {
		return nil, err
	}
	defer output.Destroy()
	stateOut, err := ort.NewEmptyTensor[float32](ort.NewShape(2, 1, 128))
	if err != nil {
		return nil, err
	}
	defer stateOut.Destroy()
	session, err := ort.NewAdvancedSession(
		d.modelPath,
		[]string{"input", "state", "sr"},
		[]string{"output", "stateN"},
		[]ort.Value{input, stateTensor, srTensor},
		[]ort.Value{output, stateOut},
		nil,
	)
	if err != nil {
		return nil, err
	}
	defer session.Destroy()

	contextSamples := make([]float32, contextSize)
	probs := make([]float32, 0, 1024)
	sampleCount := 0
	err = streamFloat32Chunks(ctx, wavPath, func(chunk []float32, validSamples int) error {
		sampleCount += validSamples
		prob, err := d.detectChunk(session, inputData, state, contextSamples, output, stateOut, chunk)
		if err != nil {
			return err
		}
		probs = append(probs, prob)
		return nil
	})
	if err != nil {
		return nil, err
	}
	if sampleCount == 0 {
		return nil, nil
	}
	return speechFromProbabilities(probs, sampleCount, d.threshold, d.minSilence, d.speechPad), nil
}

func (d *SileroDetector) detectChunk(session *ort.AdvancedSession, inputData []float32, state []float32, contextSamples []float32, output *ort.Tensor[float32], stateOut *ort.Tensor[float32], chunk []float32) (float32, error) {
	copy(inputData, contextSamples)
	copy(inputData[contextSize:], chunk)

	if err := session.Run(); err != nil {
		return 0, err
	}
	prob := output.GetData()[0]
	copy(state, stateOut.GetData())
	copy(contextSamples, chunk[chunkSize-contextSize:])
	return prob, nil
}

func initializeONNX(libraryPath string) error {
	initMu.Lock()
	defer initMu.Unlock()
	if ortInitialized {
		return nil
	}
	if libraryPath != "" {
		ort.SetSharedLibraryPath(libraryPath)
	}
	if err := ort.InitializeEnvironment(ort.WithLogLevelWarning()); err != nil {
		return err
	}
	ortInitialized = true
	return nil
}

func streamFloat32Chunks(ctx context.Context, path string, yield func(chunk []float32, validSamples int) error) error {
	cmd := exec.CommandContext(ctx, "ffmpeg", "-hide_banner", "-loglevel", "error", "-i", path, "-ac", "1", "-ar", "16000", "-f", "f32le", "pipe:1")
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return err
	}
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("start ffmpeg decode for vad: %w", err)
	}
	buffer := make([]byte, chunkSize*4)
	for {
		n, readErr := io.ReadFull(stdout, buffer)
		if readErr == io.EOF {
			break
		}
		if readErr == io.ErrUnexpectedEOF && n == 0 {
			break
		}
		if readErr != nil && readErr != io.ErrUnexpectedEOF {
			_ = cmd.Wait()
			return fmt.Errorf("read ffmpeg decode for vad: %w", readErr)
		}
		if n%4 != 0 {
			_ = cmd.Wait()
			return fmt.Errorf("ffmpeg produced invalid f32le byte count: %d", n)
		}
		chunk := make([]float32, chunkSize)
		validSamples := n / 4
		for i := 0; i < validSamples; i++ {
			bits := binary.LittleEndian.Uint32(buffer[i*4 : i*4+4])
			chunk[i] = math.Float32frombits(bits)
		}
		if err := yield(chunk, validSamples); err != nil {
			_ = cmd.Process.Kill()
			_ = cmd.Wait()
			return err
		}
		if readErr == io.ErrUnexpectedEOF {
			break
		}
	}
	if err := cmd.Wait(); err != nil {
		return fmt.Errorf("ffmpeg decode for vad failed: %w: %s", err, stderr.String())
	}
	return nil
}

func speechFromProbabilities(probs []float32, sampleCount int, threshold float32, minSilence time.Duration, speechPad time.Duration) []audio.SpeechSegment {
	negThreshold := threshold - 0.15
	if negThreshold < 0.01 {
		negThreshold = 0.01
	}
	var out []audio.SpeechSegment
	triggered := false
	start := time.Duration(0)
	possibleEnd := time.Duration(0)
	minSilenceSamples := durationToSamples(minSilence)
	pad := samplesToDuration(durationToSamples(speechPad))
	for i, prob := range probs {
		chunkStartSamples := i * chunkSize
		chunkEndSamples := minInt(chunkStartSamples+chunkSize, sampleCount)
		chunkStart := samplesToDuration(chunkStartSamples)
		chunkEnd := samplesToDuration(chunkEndSamples)
		if prob >= threshold {
			if !triggered {
				start = chunkStart - pad
				if start < 0 {
					start = 0
				}
				triggered = true
			}
			possibleEnd = 0
			continue
		}
		if triggered && prob < negThreshold {
			if possibleEnd == 0 {
				possibleEnd = chunkEnd
			}
			if durationToSamples(chunkEnd-possibleEnd) >= minSilenceSamples {
				end := possibleEnd + pad
				maxEnd := samplesToDuration(sampleCount)
				if end > maxEnd {
					end = maxEnd
				}
				if end > start {
					out = append(out, audio.SpeechSegment{Start: start, End: end})
				}
				triggered = false
				possibleEnd = 0
			}
		}
	}
	if triggered {
		end := samplesToDuration(sampleCount)
		if end > start {
			out = append(out, audio.SpeechSegment{Start: start, End: end})
		}
	}
	return out
}

func samplesToDuration(samples int) time.Duration {
	return time.Duration(int64(samples) * int64(time.Second) / sampleRate)
}

func durationToSamples(duration time.Duration) int {
	return int(int64(duration) * sampleRate / int64(time.Second))
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}
