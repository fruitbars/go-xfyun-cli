import { execFileSync } from "node:child_process";
import { createHash } from "node:crypto";
import { chmodSync, copyFileSync, mkdirSync, readFileSync, writeFileSync } from "node:fs";
import path from "node:path";
import { fileURLToPath } from "node:url";
import { runNpm } from "./npm.mjs";

const root = path.resolve(path.dirname(fileURLToPath(import.meta.url)), "..");
const readJSON = (file) => JSON.parse(readFileSync(path.join(root, file), "utf8"));
const launcher = readJSON("npm/xfyun-ai-mcp/package.json");
const targets = ["darwin-arm64", "darwin-x64", "linux-arm64", "linux-x64", "win32-arm64", "win32-x64"];
const selected = process.argv.slice(2).map((target) => target === "--host" ? `${process.platform}-${process.arch}` : target);
if (selected.some((target) => !targets.includes(target))) throw new Error("Unknown release target");
const run = (command, args, options = {}) => execFileSync(command, args, { cwd: root, ...options });
const goVersion = run("go", ["run", "./cmd/xfyun", "--version"], { encoding: "utf8" }).trim();
if (goVersion !== launcher.version) throw new Error(`Go version ${goVersion} differs from npm ${launcher.version}`);
if (process.env.GITHUB_REF_TYPE === "tag" && process.env.GITHUB_REF_NAME !== `v${launcher.version}`) {
  throw new Error("Release tag differs from package version");
}

// Validate every manifest before generating or publishing any package.
for (const target of targets) {
  const manifest = readJSON(`npm/platforms/${target}/package.json`);
  if (manifest.version !== launcher.version || launcher.optionalDependencies[manifest.name] !== launcher.version) {
    throw new Error(`Version mismatch: ${target}`);
  }
  const [platform, arch] = target.split("-");
  if (manifest.os[0] !== platform || manifest.cpu[0] !== arch) throw new Error(`Platform mismatch: ${target}`);
}

const dist = path.join(root, "dist");
mkdirSync(dist, { recursive: true });
const checksums = [];
for (const target of selected.length ? selected : targets) {
  const [platform, arch] = target.split("-");
  const goos = platform === "win32" ? "windows" : platform;
  const goarch = arch === "x64" ? "amd64" : arch;
  const extension = platform === "win32" ? ".exe" : "";
  for (const command of ["xfyun", "xfyun-ai-mcp"]) {
    const name = `${command}-${target}${extension}`;
    const binary = path.join(dist, name);
    console.log(`Building ${name}`);
    run("go", ["build", "-trimpath", "-ldflags=-s -w", "-o", binary, `./cmd/${command}`], {
      env: { ...process.env, CGO_ENABLED: "0", GOOS: goos, GOARCH: goarch }, stdio: "inherit"
    });
    checksums.push(`${createHash("sha256").update(readFileSync(binary)).digest("hex")}  ${name}`);
    if (command === "xfyun-ai-mcp") {
      const packageDir = path.join(root, "npm", "platforms", target);
      mkdirSync(path.join(packageDir, "bin"), { recursive: true });
      const packagedBinary = path.join(packageDir, "bin", `${command}${extension}`);
      copyFileSync(binary, packagedBinary);
      chmodSync(packagedBinary, 0o755);
      const packed = JSON.parse(runNpm(["pack", "--dry-run", "--json"], { cwd: packageDir, encoding: "utf8" }));
      if (!packed[0].files.some((file) => file.path === `bin/${command}${extension}` && file.size > 0)) {
        throw new Error(`Native binary missing from npm tarball: ${target}`);
      }
    }
  }
}
const checksumName = selected.length ? `SHA256SUMS-${selected.join("_")}` : "SHA256SUMS";
writeFileSync(path.join(dist, checksumName), `${checksums.join("\n")}\n`);
console.log(`Release ${launcher.version} built and npm contents verified`);
