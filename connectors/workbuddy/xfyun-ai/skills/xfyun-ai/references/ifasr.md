# IFASR 参数

`xfyun_ifasr_submit` 支持 `mp3/wav/pcm/opus/flac/ogg/speex`。讯飞单任务最长 5 小时、最大 500 MiB；工具会自动探测并无损切分超限的本地音频。普通提交保存 `order_id` 和 `signature_random`；自动切分时把返回的 `parts` 引用作为 `orders` 交给 `xfyun_ifasr_result`，工具会查询全部任务并按原顺序合并文本。外链 `urlLink` 需提供 `audio_url`、`file_name`、`file_size_bytes`，远程文件不能在本地自动切片。

- 语种：`autodialect`（默认）或多语种 `autominor`。
- 已知时长可填 `duration_ms`；0 表示由随包媒体引擎自动探测。
- 领域：`court/finance/medical/tech/sport/edu/isp/gov/game/ecom/mil/com/life/ent/culture/car`。
- 声道：`track_mode=1` 不分轨，`2` 双声道分轨；`2` 与 `role_type`、`language_analysis` 互斥。服务支持该参数，但当前公开请求参数表漏写。
- 通用角色分离：`role_type=1`，可填 0–10 的 `role_num`；声纹分离：`role_type=3` 并提供最多 64 个 `feature_ids`。
- 完成回调：`callback_url` 必须是最长 512 字符的绝对 HTTP(S) URL，服务端以 GET 调用。
- 顺滑文本：`smooth=true`（服务默认）；口语规整：`colloquial=true`。
- 远场：`vad_mode=1`；近场：`2`。粤语简体：`cantonese_script=0`；繁体：`1`（默认）。
- 语种分析：`language="autominor"` 且 `language_analysis=true`，要求应用已开通多语种能力，不能与 `track_mode=2` 同用。
- `eng_max_clusters/eng_min_clusters/eng_dtd_thre/eng_control_spk/eng_combine_max` 是老 lfasr 兼容参数。迁移旧集成时可通过 `extra` 原样透传，由当前服务引擎决定是否生效；新集成优先使用 `role_type + role_num`。

结果状态：`0` 已创建、`3` 处理中、`4` 完成、`-1` 失败。普通转写用 `result_type="transfer"`；语种分析用 `analysis`；两者都要用 `transfer,analysis`。分析结果需同时设置 `include_raw=true`。

`transcript` 是 `lattice` 的处理后文本；服务返回 `lattice2` 时，`original_transcript` 是原始文本。需要去除“嗯、啊、呃”和重复口癖时使用 `smooth=false, colloquial=true`。设置 `include_raw=true` 后，`raw_result` 返回 `orderResult`，`raw_response` 保留完整服务响应，用于角色编号、词级时间、双声道 `label.rl_track`、语种分析或其他未结构化字段。

结构化结果还提供 `speakers`：按 `st.rl` 聚合的处理后文本，每项包含 `speaker`、`transcript`，以及可用时的 `segments[{start_ms,end_ms,transcript}]`；`track_mode=2` 时还包含 `track=L/R`。用户要求分别返回发音人时直接使用 `speakers`，完整时序文本仍在 `transcript`。CLI 后备模式可用 `--speaker-output-dir` 写出汇总和逐发音人文本，`--speaker-timestamps` 加入时间戳。

服务可能把 `json_1best` 返回为 JSON 字符串或直接嵌入的对象，客户端会兼容两种形式。批量处理时，每次提交成功后立即持久化任务标识，以便中断后续跑而不重复上传和计费。

## 排障

- 缺少 `XFYUN_*`：设置凭证后完全重启 MCP 宿主；已运行的子进程看不到后来修改的 shell 环境。
- `000002`：`accessKeyId` 不存在。确认 APIKey 与另外两项凭证属于同一个应用；IFASR 不需要第四种凭证。
- `100020`：语言验证失败。检查当前应用的录音文件转写权限、通用额度，以及请求的 `autodialect`/`autominor` 模式是否已生效。新开通权限可能需要短暂同步；不要无限重试或未经用户确认切换语言。
- `100008`：请求时间超限，检查系统时间与时区。
- `100009`：签名校验失败，检查三项凭证。
- `100012`：请求频率超限，降低提交或轮询频率。
