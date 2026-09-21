# WorkBuddy 接入参考

连接器要求 WorkBuddy 5.0.0 或更高版本，并通过 Node.js 20 运行：

```bash
npx -y @fruitbars/xfyun-ai-mcp@0.7.6
```

npm 启动器会按当前系统选择 Windows、macOS 或 Linux 的 x64/arm64 原生包。只有在主包和六个平台包发布到 npm 后，市场安装用户才能直接运行。

启动器还会提供媒体切分引擎。TTS 超过单会话 64 KiB 时自动分段并生成一个输出文件；IFASR 超过 5 小时或 500 MiB 时自动无损切片、提交多个任务，并由结果工具按顺序合并文本，无需用户预处理。

连接表单收集同一讯飞应用的三元组，并以环境变量注入本地 stdio MCP 进程：

```text
XFYUN_APP_ID
XFYUN_API_KEY
XFYUN_API_SECRET
```

不要把真实值写入 Skill、`mcp.json`、提示词或仓库。该讯飞应用必须分别开通所调用的 OCR、超拟人 TTS、RTASR、录音文件转写标准版或大模型版权限、发音人或多语种套餐。标准版使用 `appId + ts + signa`，`XFYUN_API_KEY` 不是其签名必需项。

连接成功后应出现六个工具：`xfyun_ocr`、`xfyun_tts`、`xfyun_rtasr`、`xfyun_ifasr_submit`、`xfyun_ifasr_result`、`xfyun_media`。本地文件路径必须对运行 WorkBuddy 与 MCP Server 的同一台机器可见；`xfyun_media` 可查询或转换本地音频的声道、采样率和码率。

连接器工具超时设为 30 分钟，以允许多页 PDF 逐页 OCR。渲染、压缩、OCR、NDJSON 写出和释放均逐页执行；工具最终返回结果文件 `output_path`，支持时还会逐页报告 MCP progress。超长文档仍应通过 `pages` 分批处理，避免一次调用占用宿主过久。

排障时保留 OCR、TTS、RTASR 返回的脱敏 `diagnostics`；IFASR 使用 `requests[]`、`order_id` 和 `signature_random` 追踪异步订单。IFASR 提交可设置 `task_file_path` 保存权限为 `0600` 的续跑文件，结果查询再次传入同一路径即可恢复全部分片；默认拒绝覆盖已有任务文件。完整字段和脱敏规则见仓库的诊断指南。
