# @fruitbars/xfyun-ai-mcp

Cross-platform `npx` launcher for the `xfyun-ai-mcp` stdio MCP server.

```bash
npx -y @fruitbars/xfyun-ai-mcp@latest --version
```

The launcher selects an npm optional dependency for Windows, macOS, or Linux on x64/arm64 and starts the native Go server with inherited stdio and environment variables. It exposes six tools: `xfyun_ocr`, `xfyun_tts` (XFYun super-smart large-model synthesis), `xfyun_rtasr`, `xfyun_ifasr_submit`, `xfyun_ifasr_result`, and `xfyun_media`.

For a task-oriented tool selection guide covering document OCR, annotated layouts, TTS, live transcription, long recordings, speaker output, and media preparation, see the repository's [功能与使用场景](https://github.com/fruitbars/go-xfyun-cli/blob/main/docs/scenarios.md).

For CLI installations, run `xfyun doctor --json` before registering an MCP host. It reports only boolean credential presence, runtime/platform details, temporary-directory writability, and FFmpeg availability; it never prints credential values. Use `--strict` in CI.

Configure `XFYUN_APP_ID`, `XFYUN_API_KEY`, and `XFYUN_API_SECRET` from the same XFYun application in the MCP host. There is no separate fourth IFASR credential. Restart the host after changing its environment; an already-running MCP process cannot inherit later shell changes.

To configure Codex without editing TOML by hand, run:

```bash
npx -y @fruitbars/xfyun-ai-mcp@latest setup codex
```

This writes the MCP command and environment-variable names only; set the three credential values in the host environment and restart Codex.

Claude Code can register the server directly:

```bash
claude mcp add --scope user --transport stdio xfyun-ai -- npx -y @fruitbars/xfyun-ai-mcp@latest
```

The optional media helper is passed to the native server automatically. TTS text over the 64 KiB service-session limit is split at safe text boundaries and written to one audio output. IFASR audio over 5 hours or 500 MiB is probed, losslessly split, submitted as multiple orders, and merged by the result tool. Users do not need to prepare chunks themselves.

## Recording transcription (IFASR)

Use IFASR for completed recordings. The default `variant="llm"` selects XFYun's Spark large-model recording transcription; set `variant="standard"` for the standard recording transcription API. The large-model variant supports `mp3`, `wav`, `pcm`, `opus`, `flac`, `ogg`, and `speex`; standard also accepts the official `aac`, `m4a`, `amr`, `ac3`, `ape`, `m4r`, `mp4`, `acc`, and `wma` formats. Submit a local path with `xfyun_ifasr_submit`. Large-model orders return `order_id` plus `signature_random`; standard orders return only `order_id`. A split submission returns `parts`; preserve every part and pass the ordered references as `orders` so the result tool can merge them.

The submit tool accepts either local `input_path`, or `audio_url` together with `file_name` and `file_size_bytes` for XFYun `urlLink` mode. Oversized local files are split automatically; remote URLs must already fit one 5-hour/500-MiB order. `track_mode=2` enables stereo channel separation and cannot be combined with speaker-role separation or language analysis. Standard uses `appId + ts + signa` and maps `XFYUN_API_SECRET` to the service's `secretkey`; it does not use `signature_random`.

For local audio, omitted `track_mode` is detected automatically: mono inputs omit `trackMode`, while stereo inputs use `trackMode=2` unless `role_type` or language analysis was requested. An explicit `track_mode` always wins. Remote URLs cannot be probed locally and use only an explicit value or the service default.

Status `0` means created, `3` processing, `4` complete, and `-1` failed. Use `wait=false` for one status check when the MCP host has a short timeout. For batches, save each order reference immediately after submission so an interrupted run can resume without a duplicate upload.

Every IFASR submit/query result includes `requests`, pairing the order with effective non-secret provider parameters such as language, role, detected `trackMode`, and per-part file metadata. Authentication fields are excluded; URL query strings are redacted. Split results aggregate the traces and retain them on each part.

For long jobs, pass `task_file_path` to `xfyun_ifasr_submit` to save a local mode-0600 continuation file containing all order references and request traces. Pass the same `task_file_path` to `xfyun_ifasr_result` to resume and merge every part without manually copying `orders`. Existing files are protected by default; set `task_file_force=true` only when replacement is intentional.

Hosts that provide an MCP progress token receive progress notifications for TTS segments and IFASR part submission/polling. Hosts without progress support should use the returned output paths, `wait=false`, and saved order references to resume long work safely.

OCR, TTS, and RTASR structured responses include redacted `diagnostics` with effective parameters, SID, elapsed time, segment/page counts, and output metadata. IFASR uses its asynchronous `requests[]` traces plus `order_id`/`signature_random` for continuation. Credentials, signatures, and URL query tokens are never included. See the repository's [diagnostics guide](https://github.com/fruitbars/go-xfyun-cli/blob/main/docs/diagnostics.md).

Set `include_raw=true` when speaker IDs, word timing, stereo `label.rl_track`, or other detailed fields are needed. `raw_result` contains `orderResult`; `raw_response` preserves the complete service response, including reserved future result fields.

The result also includes `speakers`, grouping processed text by the service-returned speaker ID. Each entry includes `transcript` and, when the service reports `st.bg/ed`, timestamped `segments` with `start_ms`, `end_ms`, and `transcript`. With `track_mode=2`, each entry includes `track` (`L` or `R`), so agents can return each channel separately without parsing raw JSON. Do not assume speaker IDs start at 1; use the returned `speaker` value together with `track`.

The default formatted result is an interleaved speaker dialogue. Set `transcript_format` to `text`, `timeline`, `speaker_grouped`, `srt`, or `vtt` when another presentation is needed. The structured `utterances` list preserves the original service order.

Use `xfyun_media` with `operation=info` to inspect sample rate, channels, codec, bitrate, and duration, or with `operation=convert` to change mono/stereo channels, sample rate, and bitrate. Conversion requires `output_path` and does not replace an existing file unless `force=true`. `ffmpeg` is provided by the launcher; `ffprobe` is used when available and the server falls back to parsing FFmpeg metadata. Set `XFYUN_FFPROBE_PATH` when a dedicated probe binary is available.

The large-model default language mode is `autodialect` (Chinese, English, and dialects); `autominor` enables multilingual recognition when that capability is intended. Standard defaults to `cn` and exposes standard-only controls such as `hot_word`, `sys_dicts`, `candidate`, `language_type`, translation, and segment limits. Domain values are validated against XFYun's complete list. Legacy lfasr parameters such as `eng_max_clusters` and `eng_min_clusters` are accepted through `extra` for engine compatibility and forwarded unchanged; new integrations should prefer the variant's documented first-class controls. The client accepts XFYun's observed `json_1best` variants whether the nested JSON is returned as a string or an object.

These standard-only controls and advanced entitlements are optional. Ordinary transcription uses `transfer` and does not require translation, quality inspection, system dictionaries, or any of these fields unless an integration explicitly supplies them.

Common setup errors:

- Missing `XFYUN_*`: restart the MCP host after setting the three variables.
- `000002`: verify APIKey, APPID, and APISecret belong to the same application.
- `100020`: verify recording-transcription access and the requested language mode have propagated; do not retry indefinitely or switch modes without user intent.

See the repository's [IFASR guide](https://github.com/fruitbars/go-xfyun-cli/blob/main/docs/ifasr.md) for MCP and CLI examples, batch recovery, speaker separation, smoothing, language analysis, limits, output fields, and troubleshooting.

For TTS, prefer `lame` (playable MP3, the default) or `raw` (headerless PCM). XFYun's Opus/Speex variants are raw codec streams rather than Ogg files. See the repository's [TTS guide](https://github.com/fruitbars/go-xfyun-cli/blob/main/docs/tts.md) for formats, voice controls, long-text behavior, watermarks, and troubleshooting.

PDF OCR uses embedded PDFium WebAssembly: no CGO, Poppler, or other system renderer is required. Text, vector, and scanned PDFs render at 150 DPI by default. Rendering, compression, OCR, NDJSON result writing, and cleanup happen one page at a time, so memory use does not grow with the full document. The PDFium runtime has a 512 MiB hard limit and concurrent PDFs are serialized within one server process. Multi-page MCP calls return an `output_path` instead of accumulating every page in one response.

OCR responses expose readable `markdown`/`sed` fields extracted from the service `document` section. The default `text` is the readable result; pass `include_raw=true` when coordinate and layout details from the full decoded JSON are needed, then read `raw`. There is no client-side PDF page-count hard limit; selections over 1,000 pages require `confirm_large_pdf=true` because each page creates a separate OCR request. A single PDF file may be at most 500 MiB.

Set `annotate=true` to draw OCR layout element types over the source image. Select types with comma-separated `annotation_types` or use `all`. A single page returns an inline PNG (up to 8 MiB) plus `annotation_path`; multi-page calls return `annotation_paths` without embedding every image.

See the repository's [OCR guide](https://github.com/fruitbars/go-xfyun-cli/blob/main/docs/ocr.md) for PDF paging, Markdown/SED output, coordinates, all annotation types, limits, and troubleshooting.

For local launcher development, `XFYUN_AI_MCP_BINARY` can point to a locally built `xfyun-ai-mcp` executable.
