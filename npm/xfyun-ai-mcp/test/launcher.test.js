"use strict";

const test = require("node:test");
const assert = require("node:assert/strict");
const path = require("node:path");
const { mkdtempSync, readFileSync, readdirSync, rmSync, writeFileSync } = require("node:fs");
const os = require("node:os");
const { targetFor, resolveBinary, resolveFFmpeg, setupCodex } = require("../bin/xfyun-ai-mcp.js");

test("maps every supported Node platform to the native package", () => {
  assert.deepEqual(targetFor("win32", "x64"), {
    packageName: "@fruitbars/xfyun-ai-mcp-win32-x64",
    binaryName: "xfyun-ai-mcp.exe"
  });
  assert.deepEqual(targetFor("darwin", "arm64"), {
    packageName: "@fruitbars/xfyun-ai-mcp-darwin-arm64",
    binaryName: "xfyun-ai-mcp"
  });
  assert.deepEqual(targetFor("linux", "x64"), {
    packageName: "@fruitbars/xfyun-ai-mcp-linux-x64",
    binaryName: "xfyun-ai-mcp"
  });
});

test("rejects unsupported platforms", () => {
  assert.throws(() => targetFor("freebsd", "x64"), /unsupported platform/);
  assert.throws(() => targetFor("linux", "ia32"), /unsupported platform/);
});

test("accepts an explicit binary override for local development", () => {
  const expected = path.resolve("build", process.platform === "win32" ? "xfyun-ai-mcp.exe" : "xfyun-ai-mcp");
  assert.equal(resolveBinary({ override: expected }), expected);
});

test("accepts an explicit ffmpeg override", () => {
  const expected = path.resolve("tools", process.platform === "win32" ? "ffmpeg.exe" : "ffmpeg");
  assert.equal(resolveFFmpeg({ override: expected }), expected);
});

test("sets up Codex MCP env var forwarding without writing credentials", () => {
  const home = mkdtempSync(path.join(os.tmpdir(), "xfyun-codex-setup-"));
  try {
    const configDir = path.join(home, ".codex");
    const configPath = path.join(configDir, "config.toml");
    require("node:fs").mkdirSync(configDir, { recursive: true });
    writeFileSync(configPath, "model = \"test\"\n\n[mcp_servers.other]\ncommand = \"other\"\n");
    setupCodex({ homeDir: home });
    const config = readFileSync(configPath, "utf8");
    assert.match(config, /\[mcp_servers\.xfyun-ai\]/);
    assert.match(config, /env_vars = \[\"XFYUN_APP_ID\", \"XFYUN_API_KEY\", \"XFYUN_API_SECRET\"\]/);
    assert.match(config, /\[mcp_servers\.other\]/);
    assert.doesNotMatch(config, /secret-value/);
    assert.equal(readdirSync(configDir).filter((name) => name.startsWith("config.toml.bak-")).length, 1);
  } finally {
    rmSync(home, { recursive: true, force: true });
  }
});

test("package README documents resumable IFASR usage", () => {
  const readme = readFileSync(path.resolve(__dirname, "../README.md"), "utf8");
  for (const required of [
    "xfyun_ifasr_submit",
    "xfyun_ifasr_result",
    "order_id",
    "signature_random",
    "parts",
    "audio_url",
    "track_mode",
    "eng_max_clusters",
    "100020",
    "docs/ifasr.md"
  ]) {
    assert.ok(readme.includes(required), `README is missing ${required}`);
  }
});
