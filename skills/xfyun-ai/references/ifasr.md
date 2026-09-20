# IFASR parameter guide

Use `xfyun_ifasr_submit` for completed recordings. Supported file extensions are `mp3`, `wav`, `pcm`, `opus`, `flac`, `ogg`, and `speex`. Each service order is limited to 5 hours and 500 MiB; the tool automatically probes and losslessly splits larger local input. Preserve `order_id` and `signature_random` for a normal submission. For a split submission, pass the returned `parts` references to `xfyun_ifasr_result` as `orders`; it queries every order and merges transcripts in source order. For `urlLink`, provide `audio_url`, `file_name`, and `file_size_bytes`; remote inputs cannot be split locally.

## Submission mapping

- Language: `autodialect` (default) or multilingual `autominor`.
- Known duration: set `duration_ms`; zero lets the bundled media helper probe it automatically.
- Domain optimization: `court`, `finance`, `medical`, `tech`, `sport`, `edu`, `isp`, `gov`, `game`, `ecom`, `mil`, `com`, `life`, `ent`, `culture`, or `car`.
- Channel mode: `track_mode=1` mixed or `2` stereo tracks. Mode 2 cannot be combined with `role_type` or `language_analysis`. The service supports this parameter although the current public request table omits it.
- Generic speaker separation: `role_type=1`; optionally set expected `role_num` from 0–10.
- Voiceprint separation: `role_type=3` plus comma-separated `feature_ids` (maximum 64).
- Completion webhook: `callback_url` must be an absolute HTTP(S) URL of at most 512 characters; the service calls it with GET.
- Smoothed text: `smooth=true` (service default). Colloquial normalization: `colloquial=true`.
- Far-field/meeting-room audio: `vad_mode=1`; near-field/headset audio: `vad_mode=2`.
- Cantonese simplified characters: `cantonese_script=0`; traditional: `1` (service default).
- Spoken-language analysis: `language="autominor"` plus `language_analysis=true`; this requires the platform's multilingual entitlement and is unavailable with `track_mode=2`.
- `eng_max_clusters`, `eng_min_clusters`, `eng_dtd_thre`, `eng_control_spk`, and `eng_combine_max` are legacy lfasr compatibility parameters. Pass them through `extra` when migrating an existing integration; the client signs and forwards them unchanged, while actual support and effect are determined by the current service engine. Prefer `role_type` and `role_num` for new integrations.

## Results

Use `wait=false` for one status check when host timeouts are short. Status `0` means created, `3` processing, `4` complete, and `-1` failed.

- Ordinary transcript: `result_type="transfer"`.
- Language analysis only: `result_type="analysis"` and `include_raw=true`.
- Both: `result_type="transfer,analysis"` and `include_raw=true`.

When smoothing or colloquial processing is enabled, `transcript` is the processed `lattice` text and `original_transcript` is parsed from `lattice2` when returned. Use `smooth=false` and `colloquial=true` when the user asks to remove fillers and repeated speech patterns. With `include_raw=true`, `raw_result` contains `orderResult` and `raw_response` preserves the full service response; request them for role IDs, word timing, dual-channel `label.rl_track`, language-analysis details, or other fields not represented by the parsed outputs.

Structured results also expose `speakers`: processed text grouped by `st.rl`, with `speaker`, `transcript`, and (for `track_mode=2`) `track` set to `L` or `R`. Use `speakers` directly when the user asks for separate speaker transcripts; `transcript` remains the complete time-ordered text.

The service may encode `json_1best` as either a JSON string or an embedded object; the client accepts both. For batches, persist each order reference immediately after submission so an interrupted run can resume without uploading and charging again.

## Troubleshooting

- Missing `XFYUN_*`: restart the MCP host after setting credentials; an already-running child process cannot see later shell changes.
- `000002`: `accessKeyId` is unknown. Verify that APIKey and the other two credentials belong to the same application; there is no fourth IFASR credential.
- `100020`: language verification failed. Verify the recording-transcription entitlement and quota for the current application and requested `autodialect` or `autominor` mode. Newly enabled access may take time to propagate. Do not retry indefinitely or change language mode without user intent.
- `100008`: request time is outside the allowed window; check system clock and timezone.
- `100009`: signature verification failed; check the credential triplet.
- `100012`: request frequency exceeded; reduce submission or polling rate.
