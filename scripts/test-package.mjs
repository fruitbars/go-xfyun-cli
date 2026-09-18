import assert from "node:assert/strict";
import { execFileSync, spawn } from "node:child_process";
import { mkdtempSync, readFileSync, rmSync } from "node:fs";
import os from "node:os";
import path from "node:path";
import { fileURLToPath } from "node:url";
import { runNpm } from "./npm.mjs";

const root = path.resolve(path.dirname(fileURLToPath(import.meta.url)), "..");
const target = `${process.platform}-${process.arch}`;
const version = JSON.parse(readFileSync(path.join(root, "npm/xfyun-ai-mcp/package.json"), "utf8")).version;
const temporary = mkdtempSync(path.join(os.tmpdir(), "xfyun-package-test-"));
try {
  const tarballs = [path.join(root, "npm/platforms", target), path.join(root, "npm/xfyun-ai-mcp")].map((cwd) => {
    const packed = JSON.parse(runNpm(["pack", "--json", "--pack-destination", temporary], { cwd, encoding: "utf8" }));
    return path.join(temporary, packed[0].filename);
  });
  runNpm(["install", "--offline", "--omit=optional", "--ignore-scripts", "--no-audit", "--no-fund", ...tarballs], {
    cwd: temporary, stdio: "inherit"
  });
  const launcher = path.join(temporary, "node_modules/@fruitbars/xfyun-ai-mcp/bin/xfyun-ai-mcp.js");
  const env = { ...process.env };
  delete env.XFYUN_AI_MCP_BINARY;
  assert.equal(execFileSync(process.execPath, [launcher, "--version"], { encoding: "utf8", env }).trim(), version);
  await new Promise((resolve, reject) => {
    const child = spawn(process.execPath, [launcher], { env, stdio: ["pipe", "pipe", "pipe"] });
    let buffer = "";
    let stderr = "";
    let verified = false;
    const timeout = setTimeout(() => { child.kill(); reject(new Error(`MCP smoke test timed out: ${stderr}`)); }, 10000);
    const send = (message) => child.stdin.write(`${JSON.stringify(message)}\n`);
    child.on("error", reject);
    child.stderr.on("data", (data) => { stderr += data; });
    child.stdout.on("data", (data) => {
      buffer += data;
      let newline;
      while ((newline = buffer.indexOf("\n")) >= 0) {
        const line = buffer.slice(0, newline);
        buffer = buffer.slice(newline + 1);
        try {
          const response = JSON.parse(line);
          if (response.id === 1) {
            assert.ok(response.result.serverInfo);
            send({ jsonrpc: "2.0", method: "notifications/initialized" });
            send({ jsonrpc: "2.0", id: 2, method: "tools/list", params: {} });
          } else if (response.id === 2) {
            assert.deepEqual(response.result.tools.map((tool) => tool.name).sort(), ["xfyun_ifasr_result", "xfyun_ifasr_submit", "xfyun_ocr", "xfyun_rtasr", "xfyun_tts"]);
            verified = true;
            child.stdin.end();
          }
        } catch (error) { child.kill(); reject(error); }
      }
    });
    child.on("exit", (code) => {
      clearTimeout(timeout);
      if (verified && code === 0) resolve();
      else reject(new Error(`MCP exited before successful discovery (${code}): ${stderr}`));
    });
    send({ jsonrpc: "2.0", id: 1, method: "initialize", params: { protocolVersion: "2025-11-25", capabilities: {}, clientInfo: { name: "package-smoke-test", version: "1.0.0" } } });
  });
  console.log(`Installed npm package passed version and MCP discovery checks: ${target} ${version}`);
} finally {
  rmSync(temporary, { recursive: true, force: true });
}
