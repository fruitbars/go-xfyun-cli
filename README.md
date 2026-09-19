# go-xfyun-cli

面向 Claude Code、Codex、WorkBuddy 和其他 Agent 的讯飞大模型工具，提供：

- `ocr`：通用文档识别（OCR 大模型）
- `tts`：超拟人语音合成
- `rtasr`：实时语音转写大模型
- `ifasr`：录音文件转写大模型
- `xfyun-ai-mcp`：以上能力的 stdio MCP Server（5 个工具）

四项能力统一使用讯飞控制台三元组 `APPID + APIKey + APISecret`。CLI 的业务结果写 stdout，进度和错误写 stderr，适合 Agent 与自动化程序调用。

## 安装

### MCP：npx（推荐）

```bash
npx -y @fruitbars/xfyun-ai-mcp@0.4.0 --version
```

主 npm 包会自动选择 Windows、macOS、Linux 的 x64/arm64 原生包，不在运行时从 GitHub 下载二进制。

### Go 安装或源码构建

需要 Go 1.25 或更高版本：

```bash
go install github.com/fruitbars/go-xfyun-cli/cmd/xfyun@latest
go install github.com/fruitbars/go-xfyun-cli/cmd/xfyun-ai-mcp@latest
```

```bash
go build -o xfyun ./cmd/xfyun
go build -o xfyun-ai-mcp ./cmd/xfyun-ai-mcp
```

Windows 输出名可改为 `xfyun.exe` 和 `xfyun-ai-mcp.exe`。

## 鉴权

推荐通过宿主环境或产品的 Secret 表单注入，避免凭证出现在命令历史与配置文件中：

```text
XFYUN_APP_ID
XFYUN_API_KEY
XFYUN_API_SECRET
```

PowerShell 示例：

```powershell
$env:XFYUN_APP_ID = 'your-app-id'
$env:XFYUN_API_KEY = 'your-api-key'
$env:XFYUN_API_SECRET = 'your-api-secret'
```

三项值必须属于同一个讯飞应用。每项能力、发音人、多语种或声纹权限仍需在该应用中分别开通。

## CLI

### OCR

```bash
xfyun ocr --input document.png
xfyun ocr --input scan.jpg --result-format json
xfyun ocr --input form.png --result-format json,markdown,sed --result-option normal,char
xfyun ocr --input report.pdf --pages 1-5 --pdf-dpi 150
```

接口原生支持 `jpg/jpeg/png/bmp`；GIF、WebP、TIFF 等可解码位图会在纯 Go 内自动转为 JPEG/PNG。源图片最大 32 MiB；超过上传阈值时自动压缩/缩放，使实际图像载荷不超过 4 MiB、base64 不超过 10 MiB。需要透明通道时保留 PNG，否则优先转为高质量 JPEG。

PDF 使用内嵌的 PDFium WebAssembly（无 CGO、无系统依赖）按页栅格化，文本型、矢量型和扫描型 PDF 都支持；默认 150 DPI，可用 `--pdf-dpi 72..300` 调整。页面严格逐页渲染、压缩、OCR、输出结果和释放，页图与识别结果都不会在内存中整份堆积；多页 CLI 结果是一页一行的 NDJSON，可边运行边消费。单页仍保持原来的直接输出。临时渲染位图限制为 4000 万像素，超大页面会自动降低 DPI（最低 72）；PDFium WASM 内存硬上限为 512 MiB，同一进程内的 PDF 渲染会串行排队，避免并发文档叠加峰值。单次最多 200 页，PDF 最大 500 MiB。`--pages 1-3,5` 可限制页码。高级参数还包括：

- `--markdown-elements`、`--sed-elements`
- `--rotation-min-angle 0..180`
- `--exif 0|1`、`--alpha 0|1`
- `--raw` 输出完整 API 响应

从 stdin 读取时必须指定 `--encoding`。

### TTS

```bash
xfyun tts --text '你好，这是连接测试。' --output speech.mp3
xfyun tts --text-file article.txt --voice x5_lingxiaoxuan_flow --output article.mp3
xfyun tts --text '测试 PCM' --encoding raw --output speech.pcm
```

单次流式会话文本上限为 64 KiB；工具会按句子和 UTF-8 安全边界自动分段调用，并把音频按顺序写入同一个输出文件。输出路径必须显式指定，已有文件默认不覆盖；仅在明确需要时使用 `--force`。常用参数：

- `--speed/--volume/--pitch 0..100`
- `--oral-level low|mid|high`、`--spark-assist 0|1`
- `--remain`、`--stop-split`、`--background-sound`
- `--english-reading 0..2`、`--number-reading 0..3`
- `--return-pronounce 1`
- `--visible-watermark 0..2`、`--implicit-watermark`（仅 MP3/lame）

### RTASR

```bash
xfyun rtasr --input speech.pcm
xfyun rtasr --input speech.opus --audio-encoding opus-wb
xfyun rtasr --input multilingual.pcm --language autominor --recognized-language cn,en,ja
```

默认输入为 16 kHz、16 bit、单声道 PCM，并以实时节奏发送。支持 `pcm_s16le`、`opus-wb`、`speex-7`、`speex-10`，单连接最长 8 小时。高级参数包括领域优化、角色分离、声纹匹配、标点与远近场 VAD：

```bash
xfyun rtasr --input meeting.pcm --role-type 2 --feature-ids id1,id2 --speaker-match --vad-mode 1
xfyun rtasr --input speech.pcm --punctuation=false
```

需要完整事件流时使用 `--jsonl`。

### IFASR

```bash
xfyun ifasr --input meeting.mp3
xfyun ifasr --input meeting.mp3 --no-wait > order.json
xfyun ifasr --order-id 'DKHJQ...' --signature-random 'AbCd...' --no-wait
```

支持 `mp3/wav/pcm/opus/flac/ogg/speex`。单个讯飞任务最长 5 小时、最大 500 MiB；超过任一限制时，工具自动探测媒体、无损切片并提交多个任务，完成后按原顺序合并文本。npm 启动器会提供切片引擎，无需用户另行预处理。高级参数包括：

- `--role-type 1` 通用角色分离；`--role-type 3 --feature-ids ...` 声纹分离
- `--role-num 0..10`
- `--smooth=true|false`、`--colloquial=true|false`
- `--vad-mode 1|2`、`--cantonese-script 0|1`
- `--callback-url https://...`
- `--language-analysis --result-type transfer,analysis --raw`

普通提交后必须同时保存 `order_id` 与 `signature_random`；自动切片时保存返回的 `parts` 数组，并原样传给结果查询的 `orders`。状态 `0` 为已创建、`3` 为处理中、`4` 为完成、`-1` 为失败。客户端当前只实现文件流上传，没有实现文档中语义不够明确的 `urlLink` 模式。

## MCP Server

`xfyun-ai-mcp` 暴露：

- `xfyun_ocr`
- `xfyun_tts`
- `xfyun_rtasr`
- `xfyun_ifasr_submit`
- `xfyun_ifasr_result`

IFASR 拆成提交与查询，方便 Agent 跨回合保存任务标识。TTS 先写同目录临时文件再原子提交，默认拒绝覆盖。OCR 单图结果直接返回；多页结果逐页写入 NDJSON，并返回 `output_path`、`page_count`，不会把整份结果塞进一次 MCP 响应。可显式设置 `output_path`，不设置时使用临时文件；覆盖已有文件必须设置 `force=true`。支持 progress token 的 MCP 宿主还会收到逐页进度通知。详细参数与自然语言映射位于 [skills/xfyun-ai](skills/xfyun-ai)。

通用 stdio 配置：

```json
{
  "mcpServers": {
    "xfyun-ai": {
      "type": "stdio",
      "command": "npx",
      "args": ["-y", "@fruitbars/xfyun-ai-mcp@0.4.0"]
    }
  }
}
```

Codex、Claude Code、WorkBuddy 及其他宿主的安装方法见 [Agent 产品接入指南](docs/agent-integration.md)。

## WorkBuddy 连接器与 Skill

[connectors/workbuddy/xfyun-ai](connectors/workbuddy/xfyun-ai) 是 WorkBuddy 5.0.0+ 的 `MCP + Skill` 连接器目录，使用本地凭证表单注入三元组，并以 Node.js 20 执行固定版本的 npx 包。可复用 Skill 位于 [skills/xfyun-ai](skills/xfyun-ai)，按能力渐进加载四份参数参考。

## 发布

- `.github/workflows/release.yml`：在 `CGO_ENABLED=0` 下为六个平台构建 CLI/MCP 二进制并生成 SHA-256。
- `.github/workflows/npm-publish.yml`：先发布六个原生 npm 包，再发布 `@fruitbars/xfyun-ai-mcp` 启动器。

正式发布前需要确认 npm scope `@fruitbars` 的所有权、设置 `NPM_TOKEN`，并决定仓库许可证。macOS 正式分发还建议签名/公证，Windows 建议代码签名。

本地构建并检查六个平台的发布产物（需要 Node.js 18+）：

```bash
node scripts/build-release.mjs
node scripts/test-package.mjs
```

产物位于 `dist/`，原生 npm 二进制写入各平台包的 `bin/`；这些生成文件已被 `.gitignore` 排除。平台包的 `prepack` 会拒绝缺失、空白或不可执行的二进制，构建脚本还会检查版本一致性和 npm 打包内容。安装包测试在临时目录离线安装当前平台包与启动器，并验证版本和五个 MCP 工具的发现，不调用讯飞接口。

发布顺序与手动命令见 [发布指南](docs/releasing.md)。npm 工作流只手动执行，必须选中与包版本一致的 tag；GitHub Release 构建不会自动发布 npm。先完成 npm 发布和安装验收，再提交 WorkBuddy 连接器。

## 官方文档

- [实时语音转写大模型](https://www.xfyun.cn/doc/spark/asr_llm/rtasr_llm.html)
- [通用文档识别 OCR 大模型](https://www.xfyun.cn/doc/words/OCRforLLM/API.html)
- [超拟人语音合成](https://www.xfyun.cn/doc/spark/super%20smart-tts.html)
- [录音文件转写大模型](https://www.xfyun.cn/doc/spark/asr_llm/Ifasr_llm.html)
