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
- 官方基础音频属性：8 kHz 或 16 kHz、16 bit、单声道；使用 `trackMode=2` 时输入为双声道
- 单个讯飞任务：最长 5 小时且最大 500 MiB
- 上传方式：本地 `fileStream` 或 HTTP(S) 外链 `urlLink`

普通文件会直接上传。超过时长或大小任一限制时，工具使用随 npm 包提供的媒体引擎探测音频，以 4 小时/400 MiB 为目标进行 `-c copy` 无重编码切片，逐片提交，最后按源文件顺序合并文本。用户不需要提前切分。

裸 PCM 无容器元数据，自动探测按 16 kHz、16 bit、单声道解释；其他 PCM 参数应先转换，或提供正确的 `duration_ms` 并确保服务支持其实际属性。

外链模式由讯飞服务器下载音频，必须提供 `audio_url`、带受支持后缀的 `file_name` 和准确的 `file_size_bytes`。外链无法在本地自动切片，必须保持在 5 小时和 500 MiB 内；超限时下载到本地并改用 `input_path`。

## 上传参数完整对照

下表覆盖当前 Ifasr_llm `/v2/upload` 的公开参数，并补充服务实际支持但当前公开页面漏写的 `trackMode`。已知正式字段由一等参数或客户端协议层管理，不能通过 `extra/--param` 重复覆盖；`extra` 可用于服务端兼容字段和未来新增字段。

| 讯飞参数 | MCP / CLI | 必传 | 默认值 | 说明 |
| --- | --- | --- | --- | --- |
| `appId` | `XFYUN_APP_ID` | 是 | 无 | 当前讯飞应用 ID，客户端自动注入 |
| `accessKeyId` | `XFYUN_API_KEY` | 是 | 无 | 当前应用 APIKey，客户端自动注入 |
| `dateTime` | 自动生成 | 是 | 当前时间 | `yyyy-MM-dd'T'HH:mm:ss±HHmm`，每次请求重新生成 |
| `signatureRandom` | 自动生成；查询时保存并复用 | 是 | 无 | 16 位大小写字母和数字；查询必须与上传相同 |
| `fileSize` | 本地自动读取；`file_size_bytes` / `--file-size-bytes` | 是 | 无 | 音频实际字节数 |
| `fileName` | 本地自动读取；`file_name` / `--file-name` | 是 | 无 | 必须带受支持的文件后缀 |
| `durationCheckDisable` | 由 `duration_ms` 推导 | 否 | `false` | `duration_ms=0` 时发送 `true` 并省略 `duration`；提供时发送 `false` |
| `duration` | `duration_ms` / `--duration-ms` | 条件必传 | 无 | 毫秒；启用时长校验时必须与真实时长一致 |
| `language` | `language` / `--language` | 是 | `autodialect` | `autodialect` 中英及方言；`autominor` 37 语种，需相应能力 |
| `pd` | `domain` / `--domain` | 否 | 通用模型 | 领域优化，完整取值见下文 |
| `callbackUrl` | `callback_url` / `--callback-url` | 否 | 无 | 最长 512 字符的 HTTP(S) GET 回调地址 |
| `roleType` | `role_type` / `--role-type` | 否 | `0` | `0` 关闭；`1` 通用角色分离；`3` 声纹角色分离 |
| `roleNum` | `role_num` / `--role-num` | 否 | `0` | `0` 自动盲分；`1..10` 指定人数；要求开启角色分离 |
| `featureIds` | `feature_ids` / `--feature-ids` | 条件必传 | 无 | `roleType=3` 必须提供，逗号分隔，最多 64 个且不能有空项 |
| `audioMode` | 由输入类型推导 | 否 | `fileStream` | `input_path/--input` 使用 `fileStream`；`audio_url/--audio-url` 使用 `urlLink` |
| `audioUrl` | `audio_url` / `--audio-url` | 条件必传 | 无 | `urlLink` 必传，绝对 HTTP(S) URL，最长 512 字符 |
| `eng_smoothproc` | `smooth` / `--smooth` | 否 | `true` | 顺滑开关，组合语义见下表 |
| `eng_colloqproc` | `colloquial` / `--colloquial` | 否 | `false` | 口语规整开关，组合语义见下表 |
| `eng_vad_mdn` | `vad_mode` / `--vad-mode` | 否 | `1` | `1` 远场；`2` 近场 |
| `eng_rlang` | `cantonese_script` / `--cantonese-script` | 否 | `1` | 粤语输出：`0` 简体；`1` 繁体 |
| `analysis` | `language_analysis` / `--language-analysis` | 否 | `0` | `1` 开启语种识别；仅 `autominor`，双声道模式不支持 |
| `trackMode` | `track_mode` / `--track-mode` | 否 | `1` | 服务支持但当前公开参数表漏写；`1` 不分轨，`2` 双声道分轨；`2` 与 `roleType`、`analysis` 互斥 |

`pd` 只接受以下 16 个正式值：

| 值 | 领域 | 值 | 领域 | 值 | 领域 | 值 | 领域 |
| --- | --- | --- | --- | --- | --- | --- | --- |
| `court` | 法律/法院 | `finance` | 金融 | `medical` | 医疗 | `tech` | 科技 |
| `sport` | 体育 | `edu` | 教育 | `isp` | 运营商 | `gov` | 政府 |
| `game` | 游戏 | `ecom` | 电商 | `mil` | 军事 | `com` | 企业 |
| `life` | 生活 | `ent` | 娱乐 | `culture` | 人文历史 | `car` | 汽车 |

顺滑与口语规整按官方组合规则解释：

| `smooth` | `colloquial` | 返回行为 |
| --- | --- | --- |
| `false` | `false` | 只返回原始转写结果 |
| `true` | `false` | 返回顺滑结果和原始结果 |
| `true` | `true` | 返回口语规整结果和原始结果 |
| `false` | `true` | 返回口语规整结果和原始结果；需要去除“嗯、啊、呃”和重复口癖时推荐此组合 |

当服务返回两套结果时，MCP 的 `transcript` 是处理后文本，`original_transcript` 是原始文本。是否需要额外开通顺滑、口语规整、角色、声纹或多语种能力，以当前应用控制台权限为准。

## 老 lfasr 参数兼容

以下参数最初属于老 lfasr，但部分 Ifasr_llm/底层引擎仍可能识别。客户端不会替用户拒绝，会通过 `extra/--param` 原样签名并透传；实际是否生效、取值范围和效果以当前讯飞服务端为准。新项目优先使用 Ifasr_llm 的 `roleType + roleNum`，但迁移旧系统时可以保留这些参数。

| 老参数 | 老接口含义 | Ifasr_llm | 迁移方式 |
| --- | --- | --- | --- |
| `eng_max_clusters` | 说话人聚类最大人数 | 兼容透传 | 新项目优先 `roleType + roleNum` |
| `eng_min_clusters` | 说话人聚类最小人数 | 兼容透传 | 新项目优先 `roleType + roleNum` |
| `eng_dtd_thre` | 聚类相似度阈值 | 兼容透传 | 效果由当前引擎策略决定 |
| `eng_control_spk` | 启用老聚类人数约束 | 兼容透传 | 新项目优先 `roleType + roleNum` |
| `eng_combine_max` | 同角色片段合并最大间隔 | 兼容透传 | 效果由当前引擎策略决定 |

`roleNum=0` 表示自动盲分，`1..10` 表示指定人数。`trackMode=2` 时按左右声道分轨，角色分离参数失效；客户端选择直接报互斥错误，避免参数被静默忽略。

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

外链提交示例：

```json
{
  "audio_url": "https://media.example/meeting.wav",
  "file_name": "meeting.wav",
  "file_size_bytes": 12345678,
  "duration_ms": 600000
}
```

本地路径与外链只能二选一。完整业务参数见上面的上传参数对照表。

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

`/v2/getResult` 查询参数完整对照：

| 讯飞参数 | 来源 | 说明 |
| --- | --- | --- |
| `accessKeyId` | `XFYUN_API_KEY` | 客户端自动注入 |
| `dateTime` | 每次查询自动生成 | 与上传时间不同，使用当前本地时间和时区 |
| `signatureRandom` | 上传结果 | 必须原样复用上传时的随机串 |
| `orderId` | 上传结果 | 当前查询任务 ID |
| `resultType` | `result_type` | `transfer`、`analysis` 或逗号组合 |

查询使用 HTTP POST、`Content-Type: application/json`，请求体固定为 `{}`。以上五项全部参与签名，查询接口不需要 `appId`。

状态含义：

| 状态 | 含义 | 建议 |
| --- | --- | --- |
| `0` | 已创建 | 稍后继续查询 |
| `3` | 处理中 | 稍后继续查询 |
| `4` | 已完成 | 使用 `transcript` |
| `-1` | 失败 | 检查 `fail_type`，不要无限重试 |

普通转写使用 `result_type="transfer"`。语种分析使用 `analysis`，两者都需要时使用 `transfer,analysis` 并设置 `include_raw=true`。

MCP 结构化结果还返回 `fail_type`、`language`、`original_duration_ms`、`expire_time`、`task_estimate_time_ms` 和 `speakers`。`speakers` 按 `st.rl` 聚合处理后文本，每项包含 `speaker`、`transcript`；双声道 `trackMode=2` 时还包含 `track`（`L` 或 `R`）。`transcript` 来自服务的 `lattice`，通常是顺滑或口语规整后的文本；服务返回 `lattice2` 时，`original_transcript` 保留原始识别文本。讯飞实际响应中的 `json_1best` 可能是 JSON 字符串，也可能直接是对象，客户端会自动兼容两种形式。

服务响应字段完整对照：

| 层级 | 字段 | 说明 |
| --- | --- | --- |
| 顶层 | `code` | `000000` 表示请求成功；任务状态仍需看 `orderInfo.status` |
| 顶层 | `descInfo` | 成功为 `success`，失败为错误描述 |
| 顶层 | `content` | 业务数据对象 |
| `content` | `orderResult` | 转写 JSON 字符串，包含 `lattice/lattice2/label` |
| `content` | `orderInfo` | 订单状态、失败类型、时长、过期时间和语种 |
| `content` | `taskEstimateTime` | 预估处理耗时，毫秒 |
| `content` | `transResult` | 翻译结果列表，官方标记即将开放 |
| `content` | `predictResult` | 质检结果，官方标记即将开放 |
| `orderInfo` | `orderId` | 订单 ID |
| `orderInfo` | `status` | `0/3/4/-1`，见上表 |
| `orderInfo` | `failType` | 失败分类，见下文 |
| `orderInfo` | `originalDuration` | 原始音频时长，毫秒 |
| `orderInfo` | `expireTime` | 结果过期时间，毫秒 |
| `orderInfo` | `language` | 开启语种分析并查询 `analysis` 时返回 |
| `orderResult` | `lattice` | 处理后识别结果 |
| `orderResult` | `lattice2` | 原始识别结果；开启顺滑/口语规整并具备权限时返回 |
| `orderResult` | `label.rl_track` | 双声道模式下角色与左右声道映射 |

设置 `include_raw=true` 可取得仅含 `orderResult` 的 `raw_result`，以及保留整个服务响应的 `raw_response`。角色编号、词级时间、词属性和双声道映射仍会保留在原始结构中；句段级 `st.bg/ed` 也会映射到 `speakers[].segments`：

- `st.bg/ed`：句段起止毫秒；`st.rl`：角色编号
- `ws.wb/we`：相对 `st.bg` 的词帧位置，每帧 10 ms
- `cw.w`：文本；`cw.wp`：`n` 正常、`s` 顺滑、`p` 标点、`g` 分段
- `label.rl_track[].rl/track`：双声道角色与 `L/R` 声道映射

因此 Agent 不需要自行解析 `raw_response` 才能分别使用发音人文本：直接读取 `speakers` 即可。`transcript` 仍保留按时间顺序合并的完整文本。

`fail_type`：`0` 正常，`1` 上传失败，`2` 转码失败，`3` 识别失败，`4` 时长超限，`5` 时长校验失败，`6` 静音，`7` 翻译失败，`8` 无翻译权限，`9` 质检失败，`10` 质检关键词未匹配，`11` 未开通请求的翻译/质检能力，`12` 语种分析失败，`99` 其他。

上传时配置 `callback_url` 后，服务会以 GET 回调并附加 `orderId`、`status`（`1` 成功、`-1` 失败）以及可选 `resultType`；成功回调只是完成通知，仍需调用结果查询接口取回文本。

官方响应结构还预留 `transResult`（翻译）和 `predictResult`（质检）：

- `transResult[]`：`segId` 段号、`dst` 译文、`bg/ed` 起止时间、`tags` 标签、`roles` 角色。
- `predictResult.keywords[]`：`word` 关键词、`label` 词库标签、`timeStamp[].bg/ed` 命中起止时间。

当前官方页面标记这两项“即将开放”，所以客户端在 `--raw/raw_response` 中无损保留字段，但不会把尚未正式开放的 `translate/predict` 宣称为可用的 `result_type`。

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

双声道分轨：

```bash
xfyun ifasr --input stereo.wav --track-mode 2 --raw
```

CLI 的 `--raw` 结果也包含 `speakers`。若只需要某一发音人，可按 `speaker` 或 `track` 从该数组取出对应文本。

开启角色或声道分离时，可以让 CLI 额外写出便于直接交付的文本文件：

```bash
xfyun ifasr --input meeting.wav \
  --role-type 1 --role-num 2 \
  --speaker-output-dir ./meeting-speakers \
  --speaker-timestamps
```

该目录包含 `speakers.txt` 汇总文件，以及 `speaker-<speaker>[-<track>].txt` 独立文件。`--speaker-timestamps` 使用服务 lattice 的 `bg/ed` 毫秒偏移，输出 `[HH:MM:SS.mmm --> HH:MM:SS.mmm]`；不设置时只写纯文本。原始完整时序文本仍输出到 stdout 或 `transcript`，已有派生文件默认不覆盖，可用 `--speaker-force` 明确允许覆盖。

外链提交：

```bash
xfyun ifasr \
  --audio-url 'https://media.example/meeting.wav' \
  --file-name meeting.wav \
  --file-size-bytes 12345678 \
  --duration-ms 600000
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

项目已用 8 kHz、16 bit、单声道 WAV 批量验证 `role_type=1, role_num=2`：14 个文件、累计约 30 分钟，全部返回状态 `4`，每个结果均出现角色 `1` 和 `2`。另用 20 个双声道 WAV（累计约 1368 秒）验证 `trackMode=2`，全部返回状态 `4`，每个结果均包含非空的 `L/R` 两组 `speakers` 文本；双声道时不要假设 `speaker` 从 1 开始，应以服务返回的 `speaker + track` 为准。自动化测试同时覆盖文件流生命周期、URL 外链参数与空请求体、`trackMode`、正式领域值、老参数兼容透传、角色文本聚合、字符串/对象两种 `json_1best`、原始 `lattice2`、空片段以及损坏片段报错。

媒体预处理可用 CLI 的 `xfyun media info` 查询采样率、声道、编码、码率和时长，用 `xfyun media convert` 在单/双声道、采样率和码率之间转换；MCP 对应 `xfyun_media`。

官方接口文档：[录音文件转写大模型](https://www.xfyun.cn/doc/spark/asr_llm/Ifasr_llm.html)
