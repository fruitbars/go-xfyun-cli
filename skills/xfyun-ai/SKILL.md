---
name: xfyun-ai
description: Use XFYun AI tools for document-image OCR, speech synthesis, real-time audio transcription, or asynchronous recording transcription. Apply when the user requests XFYun AI processing or when these configured tools are the intended provider; do not use for generic local media conversion.
---

# XFYun AI

Use the `xfyun-ai` MCP tools when available. Fall back to the `xfyun` CLI only when MCP is unavailable. Never print, persist, or request the values of `XFYUN_APP_ID`, `XFYUN_API_KEY`, or `XFYUN_API_SECRET`; report which variable is missing instead.

## Choose the capability

- Use `xfyun_ocr` for a local document image or PDF. It converts decodable raster formats and streams the full PDF pipeline one page at a time. A multi-page call returns an NDJSON `output_path`; read it incrementally and do not load the entire file into context.
- Use `xfyun_tts` for speech synthesis. Require an explicit output path. Set `force=true` only when the user clearly authorized replacing that exact file.
- Use `xfyun_rtasr` for PCM, Opus, or Speex input that should be streamed at real-time pacing. Its default is 16 kHz, 16-bit, mono PCM.
- Use `xfyun_ifasr_submit` for ordinary or long recording files. Preserve `order_id` and `signature_random` for a normal response. When `split=true`, preserve every reference in `parts` and pass them as `orders` to `xfyun_ifasr_result`, which merges the transcripts.

Load only the parameter reference for the selected capability:

- OCR layout, Markdown/SED, character boxes, rotation, EXIF, or transparency: [references/ocr.md](references/ocr.md)
- TTS voice style, pronunciation, number/English reading, formats, or watermarks: [references/tts.md](references/tts.md)
- Live transcription language, domain, speaker separation, voiceprints, punctuation, or VAD: [references/rtasr.md](references/rtasr.md)
- Recording transcription language, speaker separation, smoothing, callbacks, analysis, or result polling: [references/ifasr.md](references/ifasr.md)

Prefer IFASR over RTASR for completed `mp3`, `wav`, `flac`, `ogg`, or multi-hour recordings. Prefer RTASR when the input is a live-style PCM/Opus/Speex stream or the user explicitly requests the realtime service.

## Handle results

- Return single-page OCR and transcription text directly when that satisfies the request; retain SIDs and order identifiers for diagnostics. For multi-page OCR, consume the returned NDJSON incrementally or in page ranges and preserve its path when the user needs the complete result.
- Treat IFASR status `4` as complete, `-1` as failed, and `0` or `3` as unfinished.
- When IFASR smoothing or colloquial processing is enabled, `transcript` is processed text and `original_transcript` is the retained original when the service returns `lattice2`. Request `include_raw=true` for language-analysis details.
- For TTS, report the resolved output path, encoding, byte count, and SID. Do not ingest generated binary audio into conversation context.
- On an API error, include the service, error code, message, and SID when present. Do not retry authentication, quota, permission, or invalid-input errors automatically.

Read [references/integration.md](references/integration.md) only when MCP setup, CLI fallback syntax, format limits, or advanced parameters are needed.
