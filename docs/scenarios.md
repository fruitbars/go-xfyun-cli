# 功能与使用场景

本项目把讯飞 OCR、超拟人合成、实时转写和录音文件转写统一成 CLI 与 MCP 工具。选择工具时，先看输入是否已经录制完成，以及是否需要实时返回结果。

## 快速选择

| 需求 | 工具 | 关键结果 |
| --- | --- | --- |
| 图片、扫描件或 PDF 提取文字 | `xfyun_ocr` / `xfyun ocr` | Markdown、HTML 表格、SED、原始版面 JSON |
| 在原图上查看段落、表格、公式等版面类型 | `xfyun_ocr` 的 `annotate=true` | 标注 PNG 和标注类型统计 |
| 把文章、公告或客服话术变成语音 | `xfyun_tts` / `xfyun tts` | MP3、PCM 或原始 Opus/Speex 音频 |
| 正在发生的会议、直播或语音交互 | `xfyun_rtasr` / `xfyun rtasr` | 实时节奏发送、最终转写文本、JSONL 事件 |
| 已经录好的 MP3、WAV、FLAC 或长录音 | `xfyun_ifasr_submit` + `xfyun_ifasr_result` | 普通文本、发音人交错文本、时间轴、SRT/VTT |
| 转写前检查或转换音频 | `xfyun_media` / `xfyun media` | 采样率、声道、码率、时长和转换文件 |

## 文档与版面

### 检测报告、合同和扫描归档

对图片或 PDF 使用 OCR。默认结果优先返回可读 Markdown；复杂表格默认使用 HTML 表格，保留合并单元格和版面结构。需要保留坐标、字体区域、公式或印章信息时增加 `include_raw=true`。

适合：检测报告、合同、发票、病历、表单、营业执照、纸质档案数字化。

### OCR 结果质量检查

使用 `annotate=true` 在原图上绘制 `paragraph`、`title`、`table`、`cell`、`formula`、`seal`、`barcode` 等类型。适合调试版面识别、人工复核表格边界、检查印章和二维码是否被识别。

多页 PDF 会逐页处理并写入 NDJSON；需要逐页消费或避免一次性加载全文时，读取返回的 `output_path`。

## 语音合成

### 文章朗读和无障碍阅读

使用 TTS 将文章、通知、课程或帮助文档生成 MP3。超过 64 KiB 的文本会自动按安全边界拆分，并合并到同一个输出文件，不需要用户手工切段。

### 客服、播客和内容生产

使用音色、语速、音量、音调、口语化、英文/数字读法和水印参数生成客服话术、播客片段或批量内容。默认 `lame` 输出可直接播放的 MP3；`raw` 是无文件头 PCM，Opus/Speex 是原始编码流，不是 Ogg 容器。

## 语音转写

### 实时字幕、直播和热线

输入实时风格的 PCM、Opus 或 Speex 音频时使用 RTASR。它按实时节奏发送音频，适合直播字幕、会议进行中记录、在线语音交互和坐席辅助。

需要声道或声纹角色能力时，使用 `role_type=2` 和已注册的 `feature_ids`；需要近场/远场 VAD 或多语种识别时，再显式设置对应参数。

### 会议、访谈和客服录音

已经录制完成的文件使用 IFASR，不要用 RTASR 模拟快速上传。IFASR 支持大模型版和标准版，默认使用大模型版。

- 单声道会议或访谈：可使用 `role_type=1` 做通用发音人分离。
- 双声道客服录音：省略 `track_mode` 时工具会自动探测并发送 `trackMode=2`，结果包含 `L/R` 声道。
- 已知发音人声纹：使用 `role_type=3`、`role_num` 和 `feature_ids`。
- 需要逐句时间：使用 `transcript_format=timeline`、`srt` 或 `vtt`。
- 需要每个发音人的独立文本：使用 `speaker_grouped`，或 CLI 的 `--speaker-output-dir`。

### 超长录音和中断续跑

本地音频超过 5 小时或 500 MiB 时，IFASR 会自动切片、逐片提交并按原顺序合并。提交时设置 `task_file_path`，即可保存订单号、续查凭证、分片时长和参数快照；后续把同一个路径交给结果工具即可恢复，不需要重新上传。

自动切片后的双声道 `L/R` 标签可以跨分片稳定合并。单声道通用发音人编号属于各自订单，跨分片不保证“发音人 1”始终是同一个真人；需要身份级一致性时，应使用声纹能力或保留分片结果分别复核。

## 音频预处理

在提交 IFASR 前使用 `xfyun_media`：

- `operation=info`：确认采样率、声道、编码、码率和时长。
- `operation=convert`：转换单声道/双声道、采样率和码率。
- 单声道普通识别通常保持服务默认，不要为了“保险”强制发送 `trackMode=2`。
- 双声道客服录音需要分轨时，确保输入确实是双声道，再使用 `track_mode=2`。

## Agent 自动化

### 批量文档流水线

Agent 可以遍历输入目录，逐个调用 OCR，把每页 NDJSON 写入任务目录，再根据 `markdown`、`sed` 或 `raw` 选择后续结构化处理。失败时保留输入文件名、页码和 SID，便于单页重试。

### 会议纪要流水线

先用 `xfyun_media info` 读取音频属性，再提交 IFASR。结果完成后使用 `dialogue` 生成交错稿，使用 `timeline`/`srt` 生成字幕，最后把 `speakers` 或 `speaker_grouped` 交给摘要、行动项和发言统计步骤。

### 工单和问题排查

保留 MCP 返回的 `diagnostics` 或 IFASR 的 `requests[]`、SID、订单号、状态、实际参数和输出路径。不要提交三项凭证、签名、完整带令牌 URL 或 `signature_random` 到公开工单。

## 选型边界

- RTASR 面向实时流；已录制的 MP3、WAV、FLAC、OGG 或多小时文件优先使用 IFASR。
- 外链音频无法由客户端本地切片，必须在服务端单订单限制内，并提供文件名和字节数。
- `trackMode=2` 与角色分离、语种分析互斥。
- 单个 PDF 文件最大 500 MiB，客户端不按页数硬拒绝；选中超过 1000 页时会先要求用户确认，确认后才逐页发起 OCR 请求。MCP 使用 `confirm_large_pdf=true`，CLI 使用 `--confirm-large-pdf`。PDF 会逐页流式处理并写入 NDJSON；大文档也可用 `pages="1-200"`、`pages="201-400"` 等范围分批，额度、频率和宿主超时由实际调用结果决定。
- 所有工具都默认不覆盖已有输出；只有明确授权时才设置 `force=true`。
