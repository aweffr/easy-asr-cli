# easy_asr

`easy_asr` is a Go/Cobra CLI for transcribing local audio files into SRT subtitles.

Implemented engines:

- `qwen3-asr-flash-filetrans` (~¥0.79/hour)
- `fun-asr` (~¥0.79/hour)
- `mimo-v2.5-asr` (~¥0.50/hour)

## Quick Start

```bash
go build -o bin/easy_asr ./cmd/easy_asr
./bin/easy_asr config validate
./bin/easy_asr transcribe /path/to/audio.mp3
```

DashScope engine behavior:

- Uploads the local audio file to configured object storage.
- Passes a presigned public URL to the selected DashScope ASR engine.
- Writes a sibling `.srt` file.
- Prints only the absolute SRT path to stdout.
- Writes progress/errors to stderr.
- Deletes the temporary object unless `--keep-object` is set.

MiMo behavior:

- Normalizes local audio to 16 kHz mono WAV segments.
- Uses Silero VAD v6 through ONNX Runtime to choose speech-aware cut points.
- Sends each segment to MiMo as a `data:audio/wav;base64,...` payload.
- Writes one SRT cue per segment with `[PART i/N HH:MM:SS-HH:MM:SS]` labels.

## Config

Default path:

```text
~/.config/easy_asr/config.yaml
```

This file should be mode `0600`; the parent directory should be mode `0700`.

The config supports Aliyun OSS and S3-compatible storage endpoints. On this machine it has been filled from the local StreamSparkAI object storage config, which uses a Tencent COS S3-compatible endpoint, plus the DashScope API key.

## Useful Commands

```bash
easy_asr transcribe input.mp3
easy_asr transcribe input.mp3 --engine fun-asr
easy_asr assets install
easy_asr doctor
easy_asr transcribe input.mp3 --engine mimo-v2.5-asr
easy_asr transcribe input.mp3 -o output.srt --raw-json output.raw.json
easy_asr transcribe input.mp3 --json
easy_asr transcribe input.mp3 --progress
easy_asr transcribe input.mp3 --json --progress-jsonl
easy_asr engines --json
easy_asr config path
easy_asr config validate
easy_asr schema run-result
easy_asr schema progress-event
```

## Agent-Friendly Output

- Default stdout: one line, the SRT path.
- `--json`: stable run result JSON.
- `--progress`: human-readable progress events on stderr.
- `--progress-jsonl`: machine-readable progress events on stderr, one JSON object per line.
- Signed result URL query strings are redacted in command output.
- Secret values are not printed by config commands.

Progress events include stable fields such as `ts`, `level`, `event`, `engine`, `run_id`, `step`, `elapsed_ms`, segment indexes, retry attempt, backoff seconds, usage seconds, and error details. Use `easy_asr schema progress-event` for the JSONL event contract.

MiMo emits detailed local preprocessing nodes for `ffprobe`, 16 kHz mono WAV normalization, Silero VAD detection, segment planning, per-segment cutting, MiMo request start/retry/completion, SRT rendering, and cleanup. DashScope engines emit upload, presign, submit, poll, download, render, and cleanup nodes.
