---
name: xfyun-ai
display_name: 讯飞云 AI
display_name_en: XFYun AI
description: 使用讯飞云 AI 完成文档 OCR、语音合成、实时音频转写和录音文件转写。
description_zh: 使用讯飞云 AI 完成文档 OCR、语音合成、实时音频转写和录音文件转写。
description_en: Use XFYun AI tools for document OCR, speech synthesis, and audio transcription.
version: 0.3.0
author: fruitbars
---

# 讯飞云 AI

优先调用本连接器提供的 MCP 工具。鉴权由 WorkBuddy 的本地连接表单注入；不要索取、显示或写入 `XFYUN_APP_ID`、`XFYUN_API_KEY`、`XFYUN_API_SECRET` 的值。

## 选择工具

- 文档图片或 PDF 使用 `xfyun_ocr`；不支持的位图格式自动转换，PDF 的渲染、压缩、OCR、结果写出和释放都按页流式执行。多页结果位于返回的 NDJSON `output_path`，不要一次性读入全部内容。
- 文字转语音使用 `xfyun_tts`。必须指定输出路径；仅当用户明确同意覆盖该文件时设置 `force=true`。
- PCM、Opus 或 Speex 实时风格音频使用 `xfyun_rtasr`。
- 已录制的普通或长音频使用 `xfyun_ifasr_submit`；保存返回的 `order_id` 和 `signature_random`，再用 `xfyun_ifasr_result` 查询。

仅按当前能力读取对应参数参考：

- OCR 版面、字符坐标、旋转、EXIF、透明通道：[references/ocr.md](references/ocr.md)
- TTS 语速、口语化、发音、读数、水印、编码：[references/tts.md](references/tts.md)
- 实时转写语种、领域、角色、声纹、标点、VAD：[references/rtasr.md](references/rtasr.md)
- 文件转写角色、顺滑、口语规整、回调、语种分析、轮询：[references/ifasr.md](references/ifasr.md)

普通录音优先使用 IFASR。长任务先以 `wait=false` 查询，状态 `4` 为完成、`-1` 为失败、`0` 或 `3` 为未完成。文件路径必须对运行本地 MCP Server 的机器可见。

单页 OCR 可直接返回文本；多页 OCR 应按行或按页消费返回的 NDJSON，并在用户需要完整结果时保留文件路径。TTS 完成后报告输出路径、格式和字节数，不把二进制音频读入对话。需要安装、鉴权或宿主配置时读取 [references/integration.md](references/integration.md)。
