#!/usr/bin/env node
"use strict";

const path = require("node:path");
const fs = require("node:fs");
const os = require("node:os");
const { spawn } = require("node:child_process");

const TARGETS = Object.freeze({
  "darwin-arm64": ["@fruitbars/xfyun-ai-mcp-darwin-arm64", "xfyun-ai-mcp"],
  "darwin-x64": ["@fruitbars/xfyun-ai-mcp-darwin-x64", "xfyun-ai-mcp"],
  "linux-arm64": ["@fruitbars/xfyun-ai-mcp-linux-arm64", "xfyun-ai-mcp"],
  "linux-x64": ["@fruitbars/xfyun-ai-mcp-linux-x64", "xfyun-ai-mcp"],
  "win32-arm64": ["@fruitbars/xfyun-ai-mcp-win32-arm64", "xfyun-ai-mcp.exe"],
  "win32-x64": ["@fruitbars/xfyun-ai-mcp-win32-x64", "xfyun-ai-mcp.exe"]
});

function targetFor(platform = process.platform, arch = process.arch) {
  const target = TARGETS[`${platform}-${arch}`];
  if (!target) {
    throw new Error(`unsupported platform: ${platform}/${arch}; supported: Windows, macOS, and Linux on x64 or arm64`);
  }
  return { packageName: target[0], binaryName: target[1] };
}

function resolveBinary(options = {}) {
  const override = options.override || process.env.XFYUN_AI_MCP_BINARY;
  if (override) {
    return path.resolve(override);
  }
  const target = targetFor(options.platform, options.arch);
  let manifest;
  try {
    manifest = require.resolve(`${target.packageName}/package.json`);
  } catch (error) {
    throw new Error(
      `native package ${target.packageName} is not installed; reinstall @fruitbars/xfyun-ai-mcp with optional dependencies enabled`,
      { cause: error }
    );
  }
  return path.join(path.dirname(manifest), "bin", target.binaryName);
}

function resolveFFmpeg(options = {}) {
  const override = options.override || process.env.XFYUN_FFMPEG_PATH;
  if (override) {
    return path.resolve(override);
  }
  try {
    return require("@ffmpeg-installer/ffmpeg").path;
  } catch {
    return null;
  }
}

const CODEX_MCP_SECTION = [
  "[mcp_servers.xfyun-ai]",
  'command = "npx"',
  'args = ["-y", "@fruitbars/xfyun-ai-mcp@latest"]',
  'env_vars = ["XFYUN_APP_ID", "XFYUN_API_KEY", "XFYUN_API_SECRET"]',
  "startup_timeout_sec = 10",
  "tool_timeout_sec = 30000",
  'default_tools_approval_mode = "writes"'
].join("\n");

function replaceTomlSection(text, sectionName, replacement) {
  const lines = text.replace(/\r\n/g, "\n").split("\n");
  const sectionPattern = new RegExp(`^\\[${sectionName.replaceAll(".", "\\.")}\\]\\s*$`);
  const start = lines.findIndex((line) => sectionPattern.test(line.trim()));
  if (start < 0) {
    const prefix = text.trimEnd();
    return `${prefix}${prefix ? "\n\n" : ""}${replacement}\n`;
  }
  let end = start + 1;
  while (end < lines.length) {
    const match = /^\[([^\]]+)\]\s*$/.exec(lines[end].trim());
    if (match && match[1] !== sectionName && !match[1].startsWith(`${sectionName}.`)) break;
    end += 1;
  }
  lines.splice(start, end - start, ...replacement.split("\n"));
  return `${lines.join("\n").replace(/\n+$/, "")}\n`;
}

function setupCodex(options = {}) {
  const home = options.homeDir || os.homedir();
  const configDir = path.join(home, ".codex");
  const configPath = path.join(configDir, "config.toml");
  fs.mkdirSync(configDir, { recursive: true });
  const original = fs.existsSync(configPath) ? fs.readFileSync(configPath, "utf8") : "";
  const updated = replaceTomlSection(original, "mcp_servers.xfyun-ai", CODEX_MCP_SECTION);
  if (updated !== original && original) {
    const backup = `${configPath}.bak-${new Date().toISOString().replaceAll(/[:.]/g, "-")}`;
    fs.copyFileSync(configPath, backup);
  }
  if (updated !== original) {
    const temporary = `${configPath}.tmp-${process.pid}`;
    fs.writeFileSync(temporary, updated, { mode: 0o600 });
    fs.renameSync(temporary, configPath);
  }
  process.stdout.write(`Configured Codex MCP at ${configPath}\n`);
  process.stdout.write("Set XFYUN_APP_ID, XFYUN_API_KEY, and XFYUN_API_SECRET, then restart Codex.\n");
}

function run() {
  if (process.argv[2] === "setup") {
    if (process.argv[3] !== "codex") {
      process.stderr.write("xfyun-ai launcher: usage: setup codex\n");
      process.exitCode = 2;
      return;
    }
    try {
      setupCodex();
    } catch (error) {
      process.stderr.write(`xfyun-ai launcher: unable to configure Codex: ${error.message}\n`);
      process.exitCode = 1;
    }
    return;
  }
  let binary;
  try {
    binary = resolveBinary();
  } catch (error) {
    process.stderr.write(`xfyun-ai launcher: ${error.message}\n`);
    process.exitCode = 1;
    return;
  }
  const env = { ...process.env };
  const ffmpeg = resolveFFmpeg();
  if (ffmpeg && !env.XFYUN_FFMPEG_PATH) {
    env.XFYUN_FFMPEG_PATH = ffmpeg;
  }
  const child = spawn(binary, process.argv.slice(2), {
    env,
    stdio: "inherit",
    windowsHide: true
  });
  child.on("error", (error) => {
    process.stderr.write(`xfyun-ai launcher: unable to start ${binary}: ${error.message}\n`);
    process.exitCode = 1;
  });
  child.on("exit", (code, signal) => {
    if (signal) {
      process.kill(process.pid, signal);
      return;
    }
    process.exitCode = code == null ? 1 : code;
  });
}

if (require.main === module) {
  run();
}

module.exports = { TARGETS, targetFor, resolveBinary, resolveFFmpeg, replaceTomlSection, setupCodex };
