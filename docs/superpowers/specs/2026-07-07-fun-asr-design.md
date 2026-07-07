# Fun-ASR Async File Transcription Design

## Summary

Add `fun-asr` as a second implemented `easy_asr` engine for DashScope asynchronous recorded file transcription. The engine follows the existing CLI path: upload local audio to object storage, pass a presigned URL to DashScope, poll the async task, download transcription JSON, render SRT, and clean up the temporary object.

The scope excludes Fun-ASR Flash synchronous/SSE APIs and realtime APIs.

## Decisions

- CLI exposes exactly two implemented engines: `qwen3-asr-flash-filetrans` and `fun-asr`.
- `fun-asr` inherits existing DashScope, object storage, and polling defaults from the Qwen3 configuration unless `engines.fun_asr` overrides them.
- `fun-asr` uses DashScope async HTTP shape with `input.file_urls` and `output.results[]`.
- `parameters` is always sent, even when empty, to stay compatible with newer DashScope workspace domains.
- `fun-asr` defaults diarization to enabled; users disable it with `--no-diarization`.
- When diarization is enabled, SRT cue text is prefixed with `[SPEAKER_0]` style labels.
- Qwen3-only CLI flags are rejected for `fun-asr` instead of being ignored.

## Public Interface Changes

- New CLI flags: `--vocabulary-id`, `--no-diarization`, `--speaker-count`.
- `--language` maps to Fun-ASR `language_hints`.
- `--channel` maps to Fun-ASR `channel_id`.
- `--hotwords`, `--hotwords-file`, `--enable-itn`, `--enable-words`, and `--no-enable-words` are unsupported with `fun-asr`.

## Validation

Implementation must pass unit tests, `go vet ./...`, `go build -o bin/easy_asr ./cmd/easy_asr`, and one real Fun-ASR CLI smoke test using a small downloaded audio file.
