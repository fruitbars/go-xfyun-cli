#!/usr/bin/env node
"use strict";

const path = require("node:path");
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

function run() {
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

module.exports = { TARGETS, targetFor, resolveBinary, resolveFFmpeg };
