# Agent 产品接入指南

`xfyun-ai-mcp` 是 Agent 产品的统一入口；`skills/xfyun-ai` 负责把自然语言需求映射到四个接口的详细参数；`xfyun` CLI 是不支持 MCP 时的保底入口。

## 共同前提

推荐所有支持本地 stdio MCP 的产品使用固定版本：

```bash
npx -y @fruitbars/xfyun-ai-mcp@0.3.0 --version
```

需要 Node.js 18 或更高版本（WorkBuddy 连接器声明 Node.js 20）。npm 主包按平台安装原生可选依赖，支持 Windows、macOS、Linux 的 x64/arm64。包尚未发布时，可先从源码构建 `xfyun-ai-mcp`，或设置 `XFYUN_AI_MCP_BINARY` 指向本地二进制测试启动器。

除 WorkBuddy 的本地凭证表单外，启动 Agent 前设置：

```text
XFYUN_APP_ID
XFYUN_API_KEY
XFYUN_API_SECRET
```

不要把真实值提交到仓库、Skill、`.mcp.json` 或提示词。

## Codex

把 [examples/codex-config.toml](../examples/codex-config.toml) 合并到用户级 `~/.codex/config.toml`，或可信项目的 `.codex/config.toml`：

```toml
[mcp_servers.xfyun-ai]
command = "npx"
args = ["-y", "@fruitbars/xfyun-ai-mcp@0.3.0"]
env_vars = ["XFYUN_APP_ID", "XFYUN_API_KEY", "XFYUN_API_SECRET"]
startup_timeout_sec = 10
tool_timeout_sec = 30000
default_tools_approval_mode = "writes"
```

`env_vars` 只列出从宿主转发的变量名，不保存值。用 `codex mcp list` 或 TUI 的 `/mcp` 检查连接。

把整个 `skills/xfyun-ai` 复制到：

- 用户级：`~/.agents/skills/xfyun-ai/`
- 项目级：`.agents/skills/xfyun-ai/`

然后可直接请求：

```text
使用 $xfyun-ai 识别 D:\docs\page.png，保留表格并整理成 Markdown。
使用 $xfyun-ai 把回答合成为 D:\audio\answer.mp3，语速稍慢，不覆盖已有文件。
```

## Claude Code

可用 CLI 注册用户级 MCP：

```bash
claude mcp add --scope user --transport stdio xfyun-ai -- npx -y @fruitbars/xfyun-ai-mcp@0.3.0
claude mcp list
```

团队项目也可把 [examples/claude-code.mcp.json](../examples/claude-code.mcp.json) 的 `mcpServers` 合并到仓库根目录 `.mcp.json`。启动后用 `/mcp` 检查，项目级 MCP 首次使用需在可信工作区批准。

把 `skills/xfyun-ai` 复制到：

- 用户级：`~/.claude/skills/xfyun-ai/`
- 项目级：`.claude/skills/xfyun-ai/`

## WorkBuddy

完整连接器位于 [connectors/workbuddy/xfyun-ai](../connectors/workbuddy/xfyun-ai)，要求 WorkBuddy 5.0.0+，包含：

```text
connector-meta.json
mcp.json
token-schema.json
icon.svg
skills/xfyun-ai/SKILL.md
skills/xfyun-ai/references/*.md
```

连接器采用 `auth_mode: "token"`。用户连接时在本地表单填写同一讯飞应用的 APPID、APIKey、APISecret；`mcp.json` 通过 `${XFYUN_*}` 占位符注入本地 MCP 进程。它以 Node.js 20 运行：

```json
{
  "type": "stdio",
  "command": "npx",
  "args": ["-y", "@fruitbars/xfyun-ai-mcp@0.3.0"]
}
```

跨平台能力来自 npm 的六个原生可选依赖，不依赖用户预装 Go，也不需要手动把二进制加入 PATH。前提是主包和所有平台包已发布。提交市场前还需确认 `source: "xfyun-ai"` 的全局唯一性，并按 WorkBuddy 开放平台流程打包审核。

连接器为多页 PDF 设置 30 分钟工具超时。PDF 会逐页渲染、压缩、OCR、写出 NDJSON 并释放页面；多页调用返回 `output_path`，不会把所有页面内容积压在 MCP 响应内存中。支持 progress token 时宿主可显示逐页进度。超长文档建议通过 `pages` 分批调用。

## 其他 Agent 产品

Cursor、Cline、Windsurf、Continue、Zed 等只要支持本地 stdio MCP，就可使用 [examples/mcp.json](../examples/mcp.json)：

```json
{
  "mcpServers": {
    "xfyun-ai": {
      "type": "stdio",
      "command": "npx",
      "args": ["-y", "@fruitbars/xfyun-ai-mcp@0.3.0"]
    }
  }
}
```

不同宿主的环境变量展开语法可能不同。让宿主继承三元组，或使用产品自己的本地 Secret/Environment 设置。支持开放 Agent Skills 标准的产品还可加载 `skills/xfyun-ai`；不支持 Skill 也不影响 MCP 工具本身。

## 验收

先只验证发现与 schema，不调用真实接口：

```text
列出 xfyun-ai 提供的工具，并解释每个工具的参数，不要调用它们。
```

配置凭证后再做最小真实验收：

```text
使用讯飞 OCR 识别 <绝对图片路径>，保留表格并返回 Markdown。
把“你好，这是连接测试”合成为 <绝对输出路径>，不要覆盖已有文件。
提交 <绝对音频路径> 做录音文件转写；若尚未完成，只查询一次状态并保留两个任务标识。
```

应看到五个工具：`xfyun_ocr`、`xfyun_tts`、`xfyun_rtasr`、`xfyun_ifasr_submit`、`xfyun_ifasr_result`。文件路径必须对 MCP Server 所在机器可见。

## 发布前检查

1. 确认 npm scope `@fruitbars` 可用且发布账号有权限。
2. 在 GitHub Actions 配置 `NPM_TOKEN`。
3. 决定并添加仓库许可证；当前包不声明许可证。
4. 按 [发布指南](releasing.md) 构建验收，先发布六个平台 npm 包，再发布启动器；GitHub Release 不是 npm 发布的前置依赖。
5. 在至少 Windows x64、macOS arm64、Linux x64 上运行 `npx ... --version`。
6. 正式桌面分发建议完成 macOS 签名/公证与 Windows 代码签名。

## 官方资料

- [Codex MCP](https://developers.openai.com/codex/mcp)
- [Codex Skills](https://developers.openai.com/codex/skills)
- [Claude Code MCP](https://code.claude.com/docs/en/mcp)
- [Claude Code Skills](https://code.claude.com/docs/en/skills)
- [WorkBuddy 连接器](https://open.workbuddy.cn/docs/connector)
