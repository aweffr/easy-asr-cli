# MiMo V2.5 ASR Implementation Plan

## Summary

Implement `mimo-v2.5-asr` with Silero VAD based local segmentation and data URL submission to MiMo chat completions. Keep existing DashScope engines unchanged except for registry/help/docs updates.

## Tasks

- Add failing tests for MiMo config, registry/help, CLI flag validation, MiMo client requests, segmentation planner, raw JSON wrapper, and SRT part labels.
- Implement config types/defaults/env/redaction/validation for `mimo_v2_5_asr`.
- Implement `internal/mimo` client and engine.
- Implement `internal/audio` helpers for `ffprobe`, `ffmpeg` normalization/cutting, deterministic segment planning, and cleanup.
- Implement `internal/vad` ONNX Runtime wrapper for Silero VAD v6, plus a safe fallback error path when runtime/model is missing.
- Add `assets install` and `doctor` commands.
- Update README, quick start, user manual, and `CONTEXT.md`.
- Run unit tests, vet, build, real one-hour MiMo E2E, then commit, tag `0.3.0`, push, rebuild, and update local binary.

## Verification Commands

- `go test ./...`
- `go vet ./...`
- `go build -o bin/easy_asr ./cmd/easy_asr`
- `./bin/easy_asr assets install`
- `./bin/easy_asr transcribe /Users/aweffr/Documents/management/daily-notes/archives/2026-07-06/2026-07-06_170014-180015.mp3 --engine mimo-v2.5-asr --json --raw-json /tmp/mimo-v2.5-asr.raw.json -o /tmp/mimo-v2.5-asr.srt`

## Execution Findings

- 180 second 16 kHz mono WAV data URL probe succeeded against MiMo.
- 210 second 16 kHz mono WAV data URL is about 8.96 MB and remains below the 10 MB input limit.
