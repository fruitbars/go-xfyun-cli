# TTS parameter guide

Use `xfyun_tts` with exactly one of `text` or `text_path`, and always set `output_path`. Each service session accepts at most 64 KiB of UTF-8 text; the tool automatically splits longer input at sentence and UTF-8-safe boundaries and writes all audio to one output. Clean tabs, emoji, invisible characters, and HTML/Markdown control syntax when they are accidental; ask before changing meaningful content.

## Map user intent to arguments

- MP3: `encoding="lame"` (default). PCM: `encoding="raw"`. These are the provider-recommended formats.
- Higher speech quality: `sample_rate=24000` (recommended/default). Valid rates: 8000, 16000, 24000.
- Faster/slower, louder/quieter, higher/lower pitch: map to `speed`, `volume`, `pitch`, each 0–100 with default 50.
- More or less conversational: `oral_level="low"|"mid"|"high"`; `spark_assist=1` enables large-model oralization. Official documentation associates the oral controls with x4 voices, so if an enabled voice rejects them, choose a compatible voice rather than silently discarding the request.
- Preserve written form: `remain=1`. Disable server sentence splitting: `stop_split=1`.
- Background sound: `background_sound=1`.
- English pronunciation: `english_reading=0|1|2` where 0 is automatic with word fallback, 1 spells letters, and 2 is automatic with letter fallback.
- Number pronunciation: `number_reading=0|1|2|3` where 0 is automatic, 1 reads a number, 2 reads a digit string, and 3 prefers string reading.
- Return phoneme/timing annotation: `return_pronounce=1`; read `pronunciation` from the result.
- Audible watermark: `visible_watermark=1` at sentence start or `2` at sentence end. Invisible watermark: `implicit_watermark=true`, supported only with `lame`/MP3.

Other accepted encodings are `speex`, `opus`, `opus-wb`, `opus-swb`, and `speex-wb`. XFYun returns these as raw codec streams, not Ogg containers, so do not claim the resulting `.opus` or `.spx` path is directly playable. Prefer MP3 for user-facing playback and PCM for further processing. Output is fixed to mono, 16-bit audio. The server writes through a temporary file and rejects an existing path unless the user explicitly authorizes `force=true`.
