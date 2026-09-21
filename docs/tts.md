# TTS 使用指南

`xfyun_tts` 使用讯飞超拟人语音合成服务，把 UTF-8 文本合成为本地音频文件。工具负责长文本拆分、参数校验、结果合并、临时文件写入和原子提交。

## 开始使用

先按 [Agent 产品接入指南](agent-integration.md) 安装 MCP，并配置 `XFYUN_APP_ID`、`XFYUN_API_KEY`、`XFYUN_API_SECRET`。之后可以直接向 Agent 提出请求：

```text
把“你好，这是连接测试”合成为 /absolute/path/hello.mp3。
把 /absolute/path/article.txt 合成为 MP3，语速稍慢、音量稍高。
合成 16 kHz 单声道 PCM 到 /absolute/path/speech.pcm。
返回这段中文的音素和时序标注。
```

## 最小调用

`text` 与 `text_path` 必须二选一，并且必须指定 `output_path`：

```json
{
  "text": "你好，这是连接测试。",
  "output_path": "/absolute/path/hello.mp3"
}
```

MCP 成功结果还包含脱敏的 `diagnostics`，记录实际音色、编码、采样率、语速、音量、音调、分段数、首个 SID、耗时和最终输出文件信息。`sids` 仍保留每个文本分段的 SID；诊断字段不会包含凭证或签名。完整规则见 [诊断信息与排障](diagnostics.md)。

默认使用发音人 `x5_lingxiaoxuan_flow`、`lame`/MP3、24000 Hz、单声道、16 bit。语速、音量、音调均为 50，口语化等级为 `mid`，大模型口语化默认开启。

## 音频格式

| `encoding` | 输出 | 建议 |
| --- | --- | --- |
| `lame` | 可直接播放的 MP3 | 默认、推荐 |
| `raw` | 无文件头的 PCM S16LE | 推荐；调用方需知道采样率 |
| `opus` | 讯飞裸 Opus 帧 | 高级用途，不是 Ogg Opus 文件 |
| `opus-wb` | 讯飞裸 Opus 宽带帧 | 高级用途，不是 Ogg Opus 文件 |
| `opus-swb` | 讯飞裸 Opus 超宽带帧 | 高级用途，不是 Ogg Opus 文件 |
| `speex` | 讯飞裸 Speex 码流 | 高级用途，不是 Ogg Speex 文件 |
| `speex-wb` | 讯飞裸 Speex 宽带码流 | 高级用途，不是 Ogg Speex 文件 |

讯飞官方推荐使用 `lame` 或 `raw`。Opus/Speex 系列虽然能由接口正常生成，但返回的是裸编码数据，当前工具不会自动封装为 Ogg，常见播放器和 FFmpeg 不能仅凭 `.opus` 或 `.spx` 扩展名自动识别。面向最终用户交付音频时优先选择 MP3；需要后续音频处理时可选择 PCM。

采样率支持 8000、16000、24000 Hz。PCM 始终为单声道、16 bit、小端有符号整数，例如：

```bash
ffmpeg -f s16le -ar 16000 -ac 1 -i speech.pcm speech.wav
```

## 语音控制

```json
{
  "text": "这是一段语音参数测试。",
  "output_path": "/absolute/path/custom.mp3",
  "speed": 30,
  "volume": 70,
  "pitch": 40,
  "oral_level": "high",
  "spark_assist": 1
}
```

- `speed`：0–100，0 约为默认语速的一半，100 约为两倍。
- `volume`：0–100，0 为静音，100 约为默认音量两倍。
- `pitch`：0–100，控制音调高低。
- `oral_level`：`low`、`mid`、`high`。
- `spark_assist`：0 或 1，是否启用大模型口语化。
- `stop_split`：0 或 1，是否关闭服务端拆句。
- `remain`：0 或 1，是否保留原书面表达。
- `background_sound`：0 或 1，是否加入背景音。

讯飞官方说明 `oral_level`、`spark_assist`、`stop_split`、`remain` 属于 x4 系列发音人的口语化配置。其他已授权音色可能接受这些参数，但不能据此假设声学效果一定相同。

## 英文和数字读法

`english_reading` 支持：`0` 自动判断、不确定时按单词处理；`1` 全部按字母朗读；`2` 自动判断、不确定时按字母朗读。

`number_reading` 支持：`0` 自动判断；`1` 按完整数值；`2` 按数字字符串逐位朗读；`3` 优先按字符串朗读。

## 发音和时序标注

设置 `return_pronounce=1` 后，返回结果中的 `pronunciation` 包含音素和时长信息：

```json
{
  "text": "语音标注测试。",
  "output_path": "/absolute/path/pronunciation.mp3",
  "return_pronounce": 1
}
```

音素时长单位为 5 毫秒。英文音素目前可能不带声调信息。

## 音频水印

- `visible_watermark=1`：在句首加入可听水印。
- `visible_watermark=2`：在句尾加入可听水印。
- `implicit_watermark=true`：加入隐式水印，仅支持 `lame`/MP3。

隐式水印信息由讯飞写入 MP3 元数据。对非 MP3 编码设置隐式水印会在调用接口前直接报错。

## 长文本自动拆分

讯飞单个会话最多接收 64 KiB UTF-8 文本。超过限制时，工具会：

1. 优先在换行、句号、问号、感叹号或分号处分段。
2. 没有句子边界时退回逗号、顿号、空格等软边界。
3. 始终保持 UTF-8 字符完整，每段不超过 64 KiB。
4. 按原顺序调用多个合成会话。
5. 将所有音频写入同一个输出文件。

返回的 `segments` 是实际分段数，`sids` 保存每段 SID，`sid` 是第一段 SID。长文本优先使用 MP3 或 PCM；裸 Opus/Speex 的跨会话拼接不保证能被通用播放器直接消费。

支持 progress token 的 MCP 宿主会在每个分段完成后收到 `TTS segment N of M completed` 通知。CLI 将相同信息写到 stderr，合成音频和最终 JSON 结果仍写到原来的输出位置。

## 输出和覆盖安全

合成结果先写入目标目录中的临时文件，全部成功后再原子提交。任何分段失败都不会留下半成品目标文件。已有文件默认拒绝覆盖，只有用户明确授权替换该文件时才设置 `force=true`。

成功结果包含：

```json
{
  "output_path": "/absolute/path/hello.mp3",
  "sid": "...",
  "sids": ["..."],
  "segments": 1,
  "encoding": "lame",
  "sample_rate": 24000,
  "bytes": 25200,
  "pronunciation": ""
}
```

## CLI 保底入口

```bash
xfyun tts --text '你好，这是连接测试。' --output speech.mp3
xfyun tts --text-file article.txt --output article.mp3
xfyun tts --text '测试 PCM' --encoding raw --sample-rate 16000 --output speech.pcm
```

运行 `xfyun tts --help` 查看全部参数。

## 常见问题

### 文件已经存在

这是默认的覆盖保护。使用新的输出路径，或者在用户明确同意后设置 `force=true`。

### Opus/Speex 文件不能播放

接口返回的是裸编码帧，不是 Ogg 容器。普通使用请改为 `encoding="lame"`；需要无损后处理时使用 `encoding="raw"` 并明确采样率。

### 修改凭证后仍提示缺失

MCP 服务进程只继承宿主启动时的环境变量。完全退出并重新启动 Codex、Claude Code 或其他宿主。

### 指定音色失败

发音人授权与服务调用额度是独立权限。确认该 APPID 已开通对应发音人，并保留错误码和 SID 用于讯飞工单排查。
