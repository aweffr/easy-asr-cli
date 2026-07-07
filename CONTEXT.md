# easy_asr

`easy_asr` turns recorded audio files into subtitle artifacts through configured ASR engines.

## Language

**ASR Engine**:
A selectable speech recognition provider integration that accepts one recorded file transcription request and returns transcription data for SRT rendering.
_Avoid_: model, backend, provider

**Recorded File Transcription**:
The conversion of a complete local audio file into timed text after the file is uploaded and made available to the ASR engine.
_Avoid_: realtime recognition, streaming recognition

**Async Transcription Task**:
A remote DashScope job created by submitting a recorded file transcription request and later queried until it reaches a terminal status.
_Avoid_: request, job when referring only to local CLI execution

**Transcription JSON**:
The structured ASR result that contains transcripts, sentences, timestamps, optional words, and optional speaker identifiers.
_Avoid_: raw result, response body

**SRT Rendering**:
The conversion of transcription JSON into SubRip subtitle cues with stable timestamps and readable cue text.
_Avoid_: export, formatting

**Diarization**:
Speaker separation that assigns a speaker identifier to recognized speech segments.
_Avoid_: speaker detection, voice labeling
