# IFASR 参数

`xfyun_ifasr_submit` 支持 `mp3/wav/pcm/opus/flac/ogg/speex`。讯飞单任务最长 5 小时、最大 500 MiB；工具会自动探测并无损切分超限音频。普通提交保存 `order_id` 和 `signature_random`；自动切分时把返回的 `parts` 引用作为 `orders` 交给 `xfyun_ifasr_result`，工具会查询全部任务并按原顺序合并文本。本客户端只实现文件流上传，尚未实现 `urlLink`。

- 语种：`autodialect`（默认）或多语种 `autominor`。
- 已知时长可填 `duration_ms`；0 表示由随包媒体引擎自动探测。
- 通用角色分离：`role_type=1`，可填 0–10 的 `role_num`；声纹分离：`role_type=3` 并提供最多 64 个 `feature_ids`。
- 完成回调：`callback_url` 必须是最长 512 字符的绝对 HTTP(S) URL，服务端以 GET 调用。
- 顺滑文本：`smooth=true`（服务默认）；口语规整：`colloquial=true`。
- 远场：`vad_mode=1`；近场：`2`。粤语简体：`cantonese_script=0`；繁体：`1`（默认）。
- 语种分析：`language_analysis=true`，要求应用已开通多语种能力。

结果状态：`0` 已创建、`3` 处理中、`4` 完成、`-1` 失败。普通转写用 `result_type="transfer"`；语种分析用 `analysis`；两者都要用 `transfer,analysis`。分析结果需同时设置 `include_raw=true`。

`transcript` 是 `lattice` 的处理后文本；服务返回 `lattice2` 时，`original_transcript` 是原始文本。`raw_result` 仅在 `include_raw=true` 时返回，用于语种分析或其他未结构化字段。
