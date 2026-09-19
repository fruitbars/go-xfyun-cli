# IFASR 录音文件转写指南

IFASR 适合已经录制完成的会议、访谈、客服录音和长音频文件。它采用“提交任务—异步查询结果”的流程。需要边录边出字时使用 RTASR；已有 `mp3/wav/flac/ogg` 等文件时优先使用 IFASR。

## 凭证与服务

录音文件转写大模型与项目中的其他讯飞能力共用三项环境变量：

```text
XFYUN_APP_ID
XFYUN_API_KEY
XFYUN_API_SECRET
```

接口字段映射为：

| 本项目 | 讯飞上传接口 |
| --- | --- |
| `XFYUN_APP_ID` | `appId` |
| `XFYUN_API_KEY` | `accessKeyId` |
| `XFYUN_API_SECRET` | HMAC-SHA1 签名密钥 |

不需要第四项 IFASR 专用凭证。三项值必须属于同一个讯飞应用，并在该应用下开通“录音文件转写大模型”。MCP 只继承宿主启动时的环境变量；修改变量后需要完全重启 Codex、Claude Code 或其他宿主。

## 输入要求

- 格式：`mp3`、`wav`、`pcm`、`opus`、`flac`、`ogg`、`speex`
- 官方音频属性：8 kHz 或 16 kHz、16 bit、单声道
- 单个讯飞任务：最长 5 小时且最大 500 MiB
- 当前仅实现 `fileStream` 本地文件上传，不实现 `urlLink`

普通文件会直接上传。超过时长或大小任一限制时，工具使用随 npm 包提供的媒体引擎探测音频，以 4 小时/400 MiB 为目标进行 `-c copy` 无重编码切片，逐片提交，最后按源文件顺序合并文本。用户不需要提前切分。

裸 PCM 无容器元数据，自动探测按 16 kHz、16 bit、单声道解释；其他 PCM 参数应先转换，或提供正确的 `duration_ms` 并确保服务支持其实际属性。

## MCP 使用

对 Agent 直接说明绝对路径即可：

```text
使用讯飞提交 /absolute/path/meeting.wav 做录音文件转写，保存任务标识；等待完成后返回文本。
```

### 1. 提交

调用 `xfyun_ifasr_submit`，最小输入为：

```json
{
  "input_path": "/absolute/path/meeting.wav"
}
```

常用提交参数：

- `language`：`autodialect`（默认，中英及方言）或 `autominor`（多语种）
- `duration_ms`：已知时长；为 0 时自动探测
- `role_type=1`：通用角色分离，可配 `role_num=0..10`
- `role_type=3`：声纹分离，必须配最多 64 个 `feature_ids`
- `smooth`：顺滑文本，服务默认开启
- `colloquial`：口语规整，服务默认关闭
- `vad_mode=1`：远场；`2`：近场
- `callback_url`：最长 512 字符的绝对 HTTP(S) 回调地址
- `language_analysis=true`：语种分析，需要相应权限

普通响应必须同时保存 `order_id` 和 `signature_random`。自动切片响应会返回 `split=true` 和 `parts`；必须完整保存每个 part 的两个标识，不能只保留第一个任务。

### 2. 查询

普通任务调用 `xfyun_ifasr_result`：

```json
{
  "order_id": "提交返回的 order_id",
  "signature_random": "提交返回的 signature_random",
  "wait": true,
  "poll_seconds": 2,
  "max_wait_seconds": 1800
}
```

宿主工具超时较短时使用 `wait=false` 只查询一次，并在后续回合继续使用原来的两个标识。自动切片任务把提交结果中的 `parts` 转成同顺序的 `orders`：

```json
{
  "orders": [
    {"order_id": "part-1", "signature_random": "random-1"},
    {"order_id": "part-2", "signature_random": "random-2"}
  ],
  "wait": true
}
```

状态含义：

| 状态 | 含义 | 建议 |
| --- | --- | --- |
| `0` | 已创建 | 稍后继续查询 |
| `3` | 处理中 | 稍后继续查询 |
| `4` | 已完成 | 使用 `transcript` |
| `-1` | 失败 | 检查 `fail_type`，不要无限重试 |

普通转写使用 `result_type="transfer"`。语种分析使用 `analysis`，两者都需要时使用 `transfer,analysis` 并设置 `include_raw=true`。

`transcript` 来自服务的 `lattice`，通常是顺滑或口语规整后的文本；服务返回 `lattice2` 时，`original_transcript` 保留原始识别文本。讯飞实际响应中的 `json_1best` 可能是 JSON 字符串，也可能直接是对象，客户端会自动兼容两种形式。

## CLI 使用

直接等待结果：

```bash
xfyun ifasr --input meeting.wav
```

异步提交并保存标识：

```bash
xfyun ifasr --input meeting.wav --no-wait > order.json
```

查询一次状态：

```bash
xfyun ifasr \
  --order-id 'ORDER_ID' \
  --signature-random 'SIGNATURE_RANDOM' \
  --no-wait
```

等待已有任务完成：

```bash
xfyun ifasr \
  --order-id 'ORDER_ID' \
  --signature-random 'SIGNATURE_RANDOM' \
  --max-wait 30m
```

角色分离示例：

```bash
xfyun ifasr --input meeting.wav --role-type 1 --role-num 2
```

## 批量处理建议

批量文件不要一次提交后才保存标识。推荐对每个文件执行以下流程：

1. 提交成功后立刻持久化 `order_id` 和 `signature_random`。
2. 再查询或等待任务完成。
3. 保存文本、状态、`fail_type`、原始时长和任务标识。
4. 中断恢复时复用已有任务，避免重复上传和重复计费。
5. 验收成功数、失败数、空文本、累计时长和输出文件数量。

任务标识用于后续查询，不是 APPID/APIKey/APISecret；仍应按业务数据妥善保存。不要把三项真实凭证写进状态文件、日志或仓库。

## 常见错误

### 缺少 `XFYUN_*`

环境变量可能是在 MCP 服务启动后才设置。完全退出并重新启动 Agent 宿主；只在另一个终端执行 `export` 不会更新已运行的 MCP 子进程。

### `000002 accessKeyId不存在`

确认 `XFYUN_API_KEY` 来自当前 APPID 对应的同一个应用，并且使用的是“录音文件转写大模型”服务页提供的应用凭证。不要另外寻找第四种 AccessKey。

### `100020 语言验证失败`

确认服务已在当前应用开通、额度可用，并且请求的 `autodialect` 或 `autominor` 能力已生效。新开通服务可能需要短暂同步。该错误属于权限/能力校验，不应自动无限重试，也不应未经用户确认擅自切换语言模式。

### `100008`、`100009`、`100012`

- `100008`：请求时间超过限制，检查系统时间与时区。
- `100009`：签名校验失败，检查三项凭证是否来自同一应用。
- `100012`：请求超过频率限制，降低提交/查询频率。

### 音频失败或空文本

检查文件是否能被媒体工具正常解码、属性是否符合接口要求，以及 `fail_type`：时长超限、转码失败、识别失败、时长校验失败或静音文件都会导致任务失败。批量运行时应保留失败文件和状态，不要只输出成功文本。

## 实测基线

项目已用 8 kHz、16 bit、单声道 WAV 批量验证：14 个文件、累计约 30 分钟，全部返回状态 `4`，无失败和空文本。自动化测试同时覆盖上传文件生命周期、字符串/对象两种 `json_1best`、原始 `lattice2`、空片段以及损坏片段报错。

官方接口文档：[录音文件转写大模型](https://www.xfyun.cn/doc/spark/asr_llm/Ifasr_llm.html)
