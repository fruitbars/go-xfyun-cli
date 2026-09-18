# WorkBuddy 接入参考

连接器要求 WorkBuddy 5.0.0 或更高版本，并通过 Node.js 20 运行：

```bash
npx -y @fruitbars/xfyun-ai-mcp@0.3.0
```

npm 启动器会按当前系统选择 Windows、macOS 或 Linux 的 x64/arm64 原生包。只有在主包和六个平台包发布到 npm 后，市场安装用户才能直接运行。

连接表单收集同一讯飞应用的三元组，并以环境变量注入本地 stdio MCP 进程：

```text
XFYUN_APP_ID
XFYUN_API_KEY
XFYUN_API_SECRET
```

不要把真实值写入 Skill、`mcp.json`、提示词或仓库。该讯飞应用必须分别开通所调用的 OCR、TTS、RTASR、IFASR 权限、发音人或多语种套餐。

连接成功后应出现五个工具：`xfyun_ocr`、`xfyun_tts`、`xfyun_rtasr`、`xfyun_ifasr_submit`、`xfyun_ifasr_result`。本地文件路径必须对运行 WorkBuddy 与 MCP Server 的同一台机器可见。

连接器工具超时设为 30 分钟，以允许多页 PDF 逐页 OCR。渲染、压缩、OCR、NDJSON 写出和释放均逐页执行；工具最终返回结果文件 `output_path`，支持时还会逐页报告 MCP progress。超长文档仍应通过 `pages` 分批处理，避免一次调用占用宿主过久。
