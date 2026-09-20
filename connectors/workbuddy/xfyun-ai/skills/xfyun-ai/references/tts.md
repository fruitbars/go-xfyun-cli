# TTS 参数

长文本会按 64 KiB 限制自动分段。WorkBuddy 若提供 MCP progress token，会在每个合成分段完成后收到进度；最终以输出文件和返回元数据为准。

`xfyun_tts` 必须设置 `output_path`，并且在 `text` 与 `text_path` 中二选一。讯飞单会话 UTF-8 文本不超过 64 KiB；工具会按句子和 UTF-8 安全边界自动分段，并把所有音频写入同一输出文件。意外出现的制表符、emoji、不可见字符、HTML/Markdown 控制符宜先清理。

- MP3：`encoding="lame"`（默认）；PCM：`raw`。这是讯飞推荐的两种格式。接口还接受 `speex/opus/opus-wb/opus-swb/speex-wb`，但返回的是裸编码码流而非 Ogg 容器，不应声称 `.opus` 或 `.spx` 可由普通播放器直接播放；用户交付优先 MP3，后处理优先 PCM。
- 采样率为 8000/16000/24000，推荐且默认 24000；输出固定单声道、16 bit。
- `speed/volume/pitch` 都是 0–100，默认 50。
- 口语程度：`oral_level="low"|"mid"|"high"`；`spark_assist=1` 开启大模型口语化。官方说明口语控制适用于 x4 发音人，若当前发音人拒绝参数，应改用已开通的兼容发音人。
- 保留书面形式：`remain=1`；禁止服务端自动断句：`stop_split=1`；背景音：`background_sound=1`。
- 英文朗读：`english_reading=0|1|2`；数字朗读：`number_reading=0|1|2|3`。
- 返回音素/时序：`return_pronounce=1`，读取结果的 `pronunciation`。
- 可听水印：`visible_watermark=1`（句首）或 `2`（句尾）；隐式水印 `implicit_watermark=true` 仅支持 lame/MP3。

仅在用户明确授权覆盖目标文件时设置 `force=true`。
