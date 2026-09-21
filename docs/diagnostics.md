# 诊断信息与排障

工具返回可复现、可提交工单的非敏感诊断信息。排障时应保留 SID 或订单号、实际参数、错误码和发生时间，但不要复制环境变量值、签名或包含临时令牌的完整 URL。

## 启动前自检

CLI 安装后可以运行：

```bash
xfyun doctor
xfyun doctor --json
xfyun doctor --strict
```

`doctor` 检查当前版本、操作系统、CPU 架构、三个环境变量是否存在、系统临时目录是否可写以及 FFmpeg 是否可用。它只输出布尔状态，不读取或显示凭证值。`--strict` 在发现问题时返回非零退出码，适合 CI 或安装验收。

MCP 进程只继承宿主启动时的环境变量。`doctor` 显示凭证就绪，但 MCP 仍提示缺少变量时，应完全退出并重新启动 Codex、Claude Code 或其他宿主。

## OCR、TTS 和 RTASR

这三个 MCP 工具在成功结果中返回 `diagnostics`：

```json
{
  "service": "tts",
  "variant": "spark",
  "operation": "synthesize",
  "sid": "...",
  "started_at": "2026-09-21T10:00:00Z",
  "elapsed_ms": 1830,
  "retries": 0,
  "parameters": {
    "voice": "x5_lingxiaoxuan_flow",
    "encoding": "lame",
    "sampleRate": "24000"
  },
  "output": {
    "path": "/absolute/path/output.mp3",
    "bytes": "123456"
  }
}
```

- OCR 顶层 `diagnostics[]` 汇总整次任务；多页 NDJSON 的每一页也包含该页完成时的诊断快照。
- TTS 的 `diagnostics` 包含最终生效的音色、编码、采样率、语速、音量、音调、分段数和输出文件。
- RTASR 的 `diagnostics` 包含语种、音频编码、采样率、角色配置、领域和最终分段数。
- `elapsed_ms` 是客户端从开始处理到生成结果的墙钟耗时。OCR 页级值为任务开始后的累计耗时。
- `retries` 当前为 `0`；鉴权、额度、权限和参数错误不会自动重试。

## IFASR

录音文件转写是异步订单接口，因此使用 `order_id`、可选的 `signature_random` 和 `requests[]`，而不是通用 `diagnostics`：

```json
{
  "order_id": "...",
  "signature_random": "...",
  "requests": [
    {
      "operation": "upload",
      "variant": "llm",
      "parameters": {
        "fileName": "meeting.wav",
        "fileSize": "123456",
        "language": "autodialect",
        "trackMode": "2"
      }
    }
  ]
}
```

同步等待结果会保留上传和查询快照；单独查询只记录本次查询。自动切片任务在顶层汇总全部快照，每个 `parts[]` 也保留自己的订单和参数。

长任务建议在提交时设置 `task_file_path`。结果查询再次传入同一路径即可恢复全部分片，不需要手工复制订单。任务文件权限为 `0600`，其中包含续查必需的 `signature_random`，应像本地会话状态一样妥善保管，不要提交到仓库。

## 永不返回的字段

诊断信息不会包含：

- `XFYUN_APP_ID`、`XFYUN_API_KEY`、`XFYUN_API_SECRET` 的值
- `appId`、`accessKeyId`、`dateTime`、`ts`、`signa` 和 HTTP 签名头
- `signatureRandom` 请求签名字段
- `audioUrl` 或 `callbackUrl` 查询字符串中的令牌

`signature_random` 是 IFASR 大模型订单续查凭证，会作为独立结果字段和任务文件内容保留，但不会混入参数快照。向第三方提供排障材料前应移除它；提交讯飞工单时按对方安全渠道要求提供。

## 推荐排障材料

向维护者或讯飞工单提供：工具版本、服务名、错误码和消息、SID 或订单号、发生时间、脱敏后的 `diagnostics`/`requests[]`、输入格式和大小。不要提供三项环境变量值、完整 `.env`、签名请求头或带令牌的 URL。
