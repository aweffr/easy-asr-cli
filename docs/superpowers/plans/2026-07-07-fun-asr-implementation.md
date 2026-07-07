# Fun-ASR Async File Transcription Implementation Plan

> **For agentic workers:** implement with TDD for behavior changes, then verify through the real CLI path.

**Goal:** Add production-ready `fun-asr` support to `easy_asr`.

**Architecture:** Reuse the existing engine boundary and storage lifecycle. Add a Fun-ASR DashScope async client for the different request and task-result shapes, then add a Fun-ASR engine that mirrors the Qwen3 engine lifecycle while using Fun-ASR-specific parameters.

**Tech Stack:** Go, Cobra, DashScope HTTP API, existing OSS/S3-compatible storage clients.

---

## Tasks

- [ ] Add failing tests for Fun-ASR DashScope payloads, polling, failed subtasks, and URL redaction.
- [ ] Add failing tests for config inheritance/validation and registry engine listing.
- [ ] Add failing tests for CLI flag mapping and unsupported flag errors.
- [ ] Add failing tests for SRT speaker labels and Fun-ASR engine lifecycle.
- [ ] Implement config, registry, DashScope client, Fun-ASR engine, CLI flags, and SRT label rendering.
- [ ] Update README, quick start, user manual, and glossary.
- [ ] Run unit tests, vet, build, and real Fun-ASR CLI smoke test.
- [ ] Request code review, fix material findings, commit, push, and update the installed binary.

## Acceptance Criteria

- `easy_asr engines` lists `qwen3-asr-flash-filetrans` and `fun-asr` as implemented engines.
- `easy_asr transcribe <file> --engine fun-asr` produces SRT through the real async DashScope path.
- `--json` output includes task id, output path, object key, redacted transcription URL, and usage seconds.
- Fun-ASR diarization defaults to enabled and produces `[SPEAKER_n]` SRT labels when the result contains `speaker_id`.
- Multi-channel Fun-ASR requests with diarization enabled fail locally with a usage error.
