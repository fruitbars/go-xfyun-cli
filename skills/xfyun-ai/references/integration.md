# XFYun integration reference

## Install and launch

The portable MCP entry point is:

```bash
npx -y @fruitbars/xfyun-ai-mcp@0.7.5
```

It selects the matching Windows, macOS, or Linux x64/arm64 native package. The npm packages must have been published before this command can work. For source development, build `xfyun-ai-mcp` and set `XFYUN_AI_MCP_BINARY` to its absolute path.

## Credentials

The CLI and MCP server both inherit these environment variables:

```text
XFYUN_APP_ID
XFYUN_API_KEY
XFYUN_API_SECRET
```

Do not put credential values in a skill, repository, prompt, or MCP arguments. Configure them in the host environment or its secret store.

## MCP tools

### `xfyun_ocr`

Required: `input_path`. Images are content-detected and converted when necessary. PDFs run through rendering, OCR, result writing, and release one page at a time at `pdf_dpi=150` by default; use `pages` to limit a large document. Multi-page results are NDJSON at the returned `output_path`; set `output_path` for a stable destination, otherwise it is an auto-generated temporary file.

For detailed options read [ocr.md](ocr.md).

### `xfyun_tts`

Provide exactly one of `text` or `text_path`, plus `output_path`. Input over the 64 KiB per-session limit is split automatically and still produces one output file. Useful options: `voice`, `encoding` (`lame` for MP3, `raw` for PCM), `sample_rate`, `speed`, `volume`, `pitch`, and `oral_level`.

The server synthesizes into a temporary file and commits it atomically. Existing files are rejected unless `force=true`. For detailed options read [tts.md](tts.md).

### `xfyun_rtasr`

Required: `input_path`. Defaults: `audio_encoding=pcm_s16le`, `sample_rate=16000`, `language=autodialect`. For `language=autominor`, narrow recognition with `recognized_language` when known.

For language codes and advanced recognition controls read [rtasr.md](rtasr.md).

### `xfyun_ifasr_submit`

Required: `input_path`. Defaults to the Spark large-model recording transcription; set `variant="standard"` for the standard recording transcription API. LLM supports `mp3`, `wav`, `pcm`, `opus`, `flac`, `ogg`, and `speex`; standard also accepts the official `aac`, `m4a`, `amr`, `ac3`, `ape`, `m4r`, `mp4`, `acc`, and `wma` formats. Audio over 5 hours or 500 MiB is automatically probed and losslessly split into multiple service orders. A zero `duration_ms` lets the tool probe duration when its bundled media helper is available.

For ordinary input, persist both returned fields:

```json
{
  "order_id": "DKHJQ...",
  "signature_random": "AbCd1234EfGh5678"
}
```

For automatically split input, preserve every `order_id` and `signature_random` in the returned `parts` array. Pass those references to `xfyun_ifasr_result` as `orders`; it merges completed transcripts in source order.

For `variant="standard"`, persist only `order_id`; the standard API signs each upload/query with `appId + ts + signa` and does not return or require `signature_random`. Its default language is `cn` and it supports standard-only fields such as `hot_word`, `sys_dicts`, `candidate`, `language_type`, translation, and segment controls.

For submission, post-processing, speaker, and analysis options read [ifasr.md](ifasr.md).

### `xfyun_ifasr_result`

Pass the two submit identifiers, or pass all split references in `orders`. Use `wait=false` for one status check per order or `wait=true` with `poll_seconds` and `max_wait_seconds` to poll and merge all parts.

### `xfyun_media`

Use `operation=info` with `input_path` to inspect local audio sample rate, channels, codec, bitrate, and duration. Use `operation=convert` with `output_path` and one or more of `channels` (`1` mono or `2` stereo), `sample_rate`, and `bitrate` (for example `128k`). Existing outputs are protected unless `force=true`. The server uses FFmpeg for conversion and prefers FFprobe for inspection, with an FFmpeg metadata fallback; set `XFYUN_FFPROBE_PATH` when a dedicated probe binary is available.

## CLI fallback

```bash
xfyun ocr --input document.png
xfyun ocr --input report.pdf --pages 1-20 > report.ocr.ndjson
xfyun tts --text-file answer.txt --output answer.mp3
xfyun rtasr --input speech.pcm
xfyun ifasr --input meeting.mp3
```

For an asynchronous recording workflow:

```bash
xfyun ifasr --input meeting.mp3 --no-wait
xfyun ifasr --order-id 'DKHJQ...' --signature-random 'AbCd1234EfGh5678'
xfyun media info --input meeting.wav
xfyun media convert --input stereo.wav --output mono-16k.wav --channels 1 --sample-rate 16000
```

Use `xfyun <command> --help` for advanced CLI options.
