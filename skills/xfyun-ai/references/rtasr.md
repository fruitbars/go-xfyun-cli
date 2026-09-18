# RTASR parameter guide

Use `xfyun_rtasr` for live-style streaming or when the user explicitly asks for the realtime service. It uploads at realtime pacing and supports sessions up to 8 hours. Prefer IFASR for an already completed MP3/WAV/FLAC/OGG recording.

## Audio and language

- `audio_encoding`: `pcm_s16le` (default), `opus-wb`, `speex-7`, or `speex-10`.
- PCM is mono, 16-bit, at `sample_rate=8000` or `16000` (default 16000).
- `language="autodialect"` recognizes Chinese dialects; `language="autominor"` is the multilingual mode.
- Set `recognized_language` only with `autominor`, using comma-separated service language codes when the requested languages are known (for example `cn,en,ja`). Do not use this field with `autodialect`.

## Recognition controls

- Domain optimization: map legal/court, finance, medical, technology, sports, education, ISP, government, games, ecommerce, military, communications, daily life, entertainment, culture, or automotive intent to `domain` values `court`, `finance`, `medical`, `tech`, `sport`, `edu`, `isp`, `gov`, `game`, `ecom`, `mil`, `com`, `life`, `ent`, `culture`, or `car`.
- Speaker separation: `role_type=2`.
- Voiceprint IDs: `feature_ids` requires `role_type=2`.
- “Only match these registered speakers”: also set `speaker_match=true`; it requires both `role_type=2` and `feature_ids`.
- Remove punctuation: `keep_punctuation=false`.
- Far-field/meeting-room audio: `vad_mode=1`; near-field/headset audio: `vad_mode=2`.

`extra` is an escape hatch for other documented query parameters. Never put credentials or replace signing fields in it. File paths must be visible to the MCP server process.
