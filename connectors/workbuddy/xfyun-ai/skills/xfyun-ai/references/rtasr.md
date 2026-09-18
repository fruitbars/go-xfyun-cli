# RTASR 参数

`xfyun_rtasr` 按实时速度上传，单会话最长 8 小时。已完成的 MP3/WAV/FLAC/OGG 录音通常改用 IFASR。

- 编码：`pcm_s16le`（默认）、`opus-wb`、`speex-7`、`speex-10`。PCM 为单声道 16 bit，采样率 8000 或 16000。
- 中文方言：`language="autodialect"`；多语种：`autominor`。仅在后者使用 `recognized_language`，以逗号分隔平台语种代码，如 `cn,en,ja`。
- 领域 `domain`：`court/finance/medical/tech/sport/edu/isp/gov/game/ecom/mil/com/life/ent/culture/car`。
- 角色分离：`role_type=2`；声纹 `feature_ids` 需要角色分离；仅匹配指定声纹时再设置 `speaker_match=true`。
- 去标点：`keep_punctuation=false`。
- 远场/会议室：`vad_mode=1`；近场/耳麦：`vad_mode=2`。

`extra` 只用于其他已确认的官方参数，不得放入凭证或覆盖签名字段。
