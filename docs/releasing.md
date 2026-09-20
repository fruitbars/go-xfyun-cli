# 发布指南

## 准备

1. 初始化 Git 并确认 `.gitignore` 生效，不提交凭证、音频或生成的二进制。
2. 决定并添加许可证；当前不替项目选择许可证。
3. 确认 npm 发布账号拥有 `@fruitbars` scope 权限，完成 `npm login`。
4. 确认 GitHub 仓库地址与 `go.mod`、npm manifests 一致；若地址变化，需要同步更新。
5. 保持 Go、七个 npm 包、optionalDependencies、连接器及配置示例的版本一致，当前 npm 版本为 `0.6.0`；WorkBuddy 连接器按单独审核版本管理。

## 本地验收

```bash
go test -race ./...
go vet ./...
node --test npm/xfyun-ai-mcp/test/launcher.test.js npm/xfyun-ai-mcp/test/native-pack.test.js
node scripts/build-release.mjs
node scripts/test-package.mjs
```

构建生成十二个 CLI/MCP 二进制及 `dist/SHA256SUMS`，并把 MCP 二进制写入六个平台的 npm 包。`test-package.mjs` 验证当前主机的真实 npm 安装包，清除二进制路径 override，检查版本及 stdio MCP 初始化和工具发现。跨平台构建通过不等于对应操作系统运行验收通过。

## 先发布 npm

本地发布可以在上传 GitHub 之前完成，但请先完成上面的验收。先发布六个平台包，每条命令成功后再继续；最后发布启动器：

```bash
npm publish ./npm/platforms/darwin-arm64 --access public
npm publish ./npm/platforms/darwin-x64 --access public
npm publish ./npm/platforms/linux-arm64 --access public
npm publish ./npm/platforms/linux-x64 --access public
npm publish ./npm/platforms/win32-arm64 --access public
npm publish ./npm/platforms/win32-x64 --access public
npm publish ./npm/xfyun-ai-mcp --access public
```

npm 版本不能重复发布。部分成功时，先用 `npm view <包名>@0.6.0` 核实已发布的包，只继续尚未发布的包；不要直接重新运行完整发布流程。若发布内容错误，需要提升所有相关版本后重新发布。

推送 `v0.6.0` 这类 `v*` tag 后，`Build Release` 和 `Publish npm` 都会自动触发；`Publish npm` 仍保留手动入口用于补发。设置 `NPM_TOKEN` secret，按需配置 `npm` environment 的审批。工作流先在 Windows、macOS、Linux runner 上测试并验证各自当前架构的安装包，全部通过后再构建六个平台并依次发布原生包和启动器。`Build Release` 不是 npm 发布的前置依赖。

## npm 安装验收

至少在 Windows x64、macOS arm64 和 Linux x64 上运行：

```bash
npx -y @fruitbars/xfyun-ai-mcp@0.6.0 --version
```

然后在 MCP 宿主中确认六个工具可发现。配置讯飞凭证后，用小文件分别验收 OCR、TTS、RTASR 和 IFASR，并用本地音频验收 `xfyun_media`；大模型版 IFASR 保存 `order_id` 与 `signature_random`，标准版只保存 `order_id`，自动切片时保存全部 `parts`。这些真实接口调用可能收费，应手动执行。

## 再提交 WorkBuddy

npm 安装和接口验收通过后，提交 `connectors/workbuddy/xfyun-ai` 目录中的连接器材料，按照 WorkBuddy 开放平台要求打包审核。确认 `source: xfyun-ai` 全局唯一、版本匹配、凭证表单注入正常，并在 WorkBuddy 内复验 MCP 工具和 Skill。不要把真实凭证放进连接器包。

GitHub、npm 或 WorkBuddy 发布均不会由本地构建脚本自动执行。
