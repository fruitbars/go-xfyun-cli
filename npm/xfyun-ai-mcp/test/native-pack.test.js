"use strict";

const test = require("node:test");
const assert = require("node:assert/strict");
const { execFileSync } = require("node:child_process");
const { mkdtempSync, readFileSync, rmSync, writeFileSync } = require("node:fs");
const os = require("node:os");
const path = require("node:path");
const { TARGETS } = require("../bin/xfyun-ai-mcp.js");

test("every native prepack guard rejects packages without binaries", () => {
  for (const target of Object.keys(TARGETS)) {
    const manifest = JSON.parse(readFileSync(path.resolve(__dirname, "../../platforms", target, "package.json"), "utf8"));
    const temporary = mkdtempSync(path.join(os.tmpdir(), "xfyun-empty-package-"));
    try {
      writeFileSync(path.join(temporary, "package.json"), JSON.stringify(manifest));
      const match = /^node -e "(.*)"$/.exec(manifest.scripts.prepack);
      assert.ok(match, `${target} must have a prepack guard`);
      assert.throws(() => execFileSync(process.execPath, ["-e", match[1]], { cwd: temporary, stdio: "pipe" }), `${target} accepted missing binary`);
    } finally {
      rmSync(temporary, { recursive: true, force: true });
    }
  }
});
