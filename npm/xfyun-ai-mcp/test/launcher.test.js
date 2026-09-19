"use strict";

const test = require("node:test");
const assert = require("node:assert/strict");
const path = require("node:path");
const { targetFor, resolveBinary, resolveFFmpeg } = require("../bin/xfyun-ai-mcp.js");

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
