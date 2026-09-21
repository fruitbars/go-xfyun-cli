# 实时语音转写使用指南

`xfyun_rtasr` 用于以实时节奏发送 PCM、Opus 或 Speex 音频。已经录制完成的 MP3、WAV、FLAC、OGG 或多小时录音通常应使用 IFASR；只有实时流式场景或用户明确指定实时转写时才使用 RTASR。

## 输入与默认值

必须提供 MCP Server 可访问的本地 `input_path`。默认输入为 16000 Hz、16 bit、单声道 PCM：

```json
{
  "input_path": "/absolute/path/speech.pcm"
}
```

支持的 `audio_encoding`：

| 值 | 说明 |
| --- | --- |
| `pcm_s16le` | 默认；8000 或 16000 Hz 单声道 PCM |
| `opus-wb` | Opus 宽带裸码流 |
| `speex-7` | Speex 质量等级 7 |
| `speex-10` | Speex 质量等级 10 |

单连接最长 8 小时。工具按照实时节奏发送，不适合希望快速处理已有长录音的场景。

## 语种和领域

- `language="autodialect"`：默认，中文、英文和方言识别。
- `language="autominor"`：多语种识别；已知目标语种时用 `recognized_language` 缩小范围，例如 `cn,en,ja`。
- `domain`：`court`、`finance`、`medical`、`tech`、`sport`、`edu`、`isp`、`gov`、`game`、`ecom`、`mil`、`com`、`life`、`ent`、`culture`、`car`。

`recognized_language` 只应与 `autominor` 一起使用，不要与 `autodialect` 混用。

## 发音人与识别控制

```json
{
  "input_path": "/absolute/path/meeting.pcm",
  "role_type": 2,
  "feature_ids": "voiceprint-1,voiceprint-2",
  "speaker_match": true,
  "keep_punctuation": true,
  "vad_mode": 1
}
```

- `role_type=2`：开启角色分离。
- `feature_ids`：已注册的声纹 ID，要求 `role_type=2`。
- `speaker_match=true`：只匹配指定声纹，要求同时设置前两项。
- `keep_punctuation=false`：去除标点；默认保留。
- `vad_mode=1`：远场或会议室；`2`：近场或耳麦。
- `extra`：透传其他已确认的业务参数，不能放入凭证或覆盖签名字段。

## 结果与诊断

成功结果包含文本、SID、服务分段数以及脱敏诊断：

```json
{
  "transcript": "识别文本",
  "sid": "...",
  "segments": 12,
  "diagnostics": {
    "service": "rtasr",
    "variant": "spark",
    "operation": "transcribe",
    "sid": "...",
    "elapsed_ms": 65000,
    "retries": 0,
    "parameters": {
      "language": "autodialect",
      "audioEncoding": "pcm_s16le",
      "sampleRate": "16000",
      "roleType": "0",
      "segments": "12"
    }
  }
}
```

完整诊断字段和脱敏规则见 [诊断信息与排障](diagnostics.md)。

## CLI 保底入口

```bash
xfyun rtasr --input speech.pcm
xfyun rtasr --input speech.opus --audio-encoding opus-wb
xfyun rtasr --input multilingual.pcm --language autominor --recognized-language cn,en,ja
xfyun rtasr --input meeting.pcm --role-type 2 --feature-ids id1,id2 --speaker-match
```

`--jsonl` 会输出服务事件流；默认只输出最终文本。CLI 业务输出写 stdout，错误写 stderr。
