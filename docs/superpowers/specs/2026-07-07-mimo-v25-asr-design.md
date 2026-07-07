# MiMo V2.5 ASR Design

## Goal

Add `mimo-v2.5-asr` as a third implemented engine in `easy_asr`, alongside `qwen3-asr-flash-filetrans` and `fun-asr`.

MiMo accepts audio through OpenAI-compatible chat completions, but real probes showed recorded-file ASR does not accept ordinary audio URLs. The engine therefore sends local audio segments as `data:audio/wav;base64,...`.

## Key Decisions

- Always preprocess MiMo input locally with segmentation.
- Normalize audio through `ffmpeg` to 16 kHz mono WAV.
- Use Silero VAD v6 with ONNX Runtime to find speech-aware segment boundaries.
- Target segment length is 180 seconds. The planner searches for a VAD boundary between 150 and 210 seconds; if none is found, it hard-cuts at 210 seconds.
- MiMo v1 submits segments serially to keep ordering, rate limit behavior, and cleanup simple.
- MiMo raw JSON is a wrapper containing input metadata, segment metadata, each raw MiMo response, and total usage.
- MiMo SRT uses one cue per segment because MiMo does not return word or sentence timestamps. Cue timestamps use the original audio timeline and cue text is prefixed with `[PART i/N HH:MM:SS-HH:MM:SS]`.

## Public Surface

- New engine name: `mimo-v2.5-asr`.
- Config path: `engines.mimo_v2_5_asr`.
- Environment variables:
  - `EASY_ASR_MIMO_API_KEY` or `MIMO_API_KEY`
  - `EASY_ASR_MIMO_BASE_URL`
  - `EASY_ASR_MIMO_MODEL`
  - `EASY_ASR_SILERO_VAD_MODEL`
  - `EASY_ASR_ONNXRUNTIME_LIBRARY`
- New commands:
  - `easy_asr assets install`
  - `easy_asr doctor`

## Validation

- Unit tests cover config, registry, CLI flag mapping, unsupported flags, MiMo payload/error handling, segmentation planning, raw JSON shape, and SRT part labels.
- Real E2E uses the provided one-hour MP3 and verifies full segmentation, MiMo API calls, raw JSON, SRT labels, and temp cleanup.
