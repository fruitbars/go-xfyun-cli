# IFASR parameter guide

Use `xfyun_ifasr_submit` for completed recordings, then preserve both `order_id` and `signature_random` for `xfyun_ifasr_result`. Supported file extensions are `mp3`, `wav`, `pcm`, `opus`, `flac`, `ogg`, and `speex`; limits are 5 hours and 500 MiB. This client currently implements `fileStream` upload only, not `urlLink`.

## Submission mapping

- Language: `autodialect` (default) or multilingual `autominor`.
- Known duration: set `duration_ms`; zero disables the server duration check.
- Domain optimization: use the same `domain` values as RTASR when the user names a supported field.
- Generic speaker separation: `role_type=1`; optionally set expected `role_num` from 0–10.
- Voiceprint separation: `role_type=3` plus comma-separated `feature_ids` (maximum 64).
- Completion webhook: `callback_url` must be an absolute HTTP(S) URL of at most 512 characters; the service calls it with GET.
- Smoothed text: `smooth=true` (service default). Colloquial normalization: `colloquial=true`.
- Far-field/meeting-room audio: `vad_mode=1`; near-field/headset audio: `vad_mode=2`.
- Cantonese simplified characters: `cantonese_script=0`; traditional: `1` (service default).
- Spoken-language analysis: `language_analysis=true`; this requires the platform's multilingual entitlement.

## Results

Use `wait=false` for one status check when host timeouts are short. Status `0` means created, `3` processing, `4` complete, and `-1` failed.

- Ordinary transcript: `result_type="transfer"`.
- Language analysis only: `result_type="analysis"` and `include_raw=true`.
- Both: `result_type="transfer,analysis"` and `include_raw=true`.

When smoothing or colloquial processing is enabled, `transcript` is the processed `lattice` text and `original_transcript` is parsed from `lattice2` when returned. `raw_result` contains the service `orderResult` JSON string only when `include_raw=true`; request it for language-analysis details or fields not represented by the parsed outputs.
