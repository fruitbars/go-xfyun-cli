import { execFileSync } from "node:child_process";
import path from "node:path";

export function runNpm(args, options = {}) {
  if (process.platform !== "win32") return execFileSync("npm", args, options);
  // Launch the CLI through Node, not npm.cmd, so paths need no shell escaping.
  const command = execFileSync("where.exe", ["npm.cmd"], { encoding: "utf8" }).trim().split(/\r?\n/)[0];
  const cli = path.join(path.dirname(command), "node_modules", "npm", "bin", "npm-cli.js");
  return execFileSync(process.execPath, [cli, ...args], options);
}
