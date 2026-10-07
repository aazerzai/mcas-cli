#!/usr/bin/env node
// Generates one npm package per platform from the GoReleaser output in ../dist,
// and pins the main package's optionalDependencies to the same version.
// Usage: node scripts/build-platform-packages.js <version> [outDir]
"use strict";

const fs = require("fs");
const path = require("path");

const version = process.argv[2];
if (!version) {
  console.error("usage: build-platform-packages.js <version> [outDir]");
  process.exit(1);
}
const outRoot = path.resolve(process.argv[3] || path.join(__dirname, "..", "platforms"));
const dist = path.resolve(__dirname, "..", "..", "dist");
const mainPath = path.join(__dirname, "..", "package.json");
const main = require(mainPath);

// npm platform/arch -> GoReleaser os/arch
const TARGETS = [
  { os: "darwin", cpu: "arm64", goos: "darwin", goarch: "arm64" },
  { os: "darwin", cpu: "x64", goos: "darwin", goarch: "amd64" },
  { os: "linux", cpu: "x64", goos: "linux", goarch: "amd64" },
  { os: "linux", cpu: "arm64", goos: "linux", goarch: "arm64" },
  { os: "win32", cpu: "x64", goos: "windows", goarch: "amd64" },
  { os: "win32", cpu: "arm64", goos: "windows", goarch: "arm64" },
];

function find(dir, name) {
  for (const e of fs.readdirSync(dir, { withFileTypes: true })) {
    const p = path.join(dir, e.name);
    if (e.isDirectory()) {
      const hit = find(p, name);
      if (hit) return hit;
    } else if (e.name === name) {
      return p;
    }
  }
  return null;
}

const optionalDependencies = {};
for (const t of TARGETS) {
  const ext = t.os === "win32" ? ".exe" : "";
  const src = find(dist, `mcas_${t.goos}_${t.goarch}${ext}`);
  if (!src) throw new Error(`binary for ${t.goos}/${t.goarch} not found in ${dist}`);

  const name = `${main.name}-${t.os}-${t.cpu}`;
  const dir = path.join(outRoot, `${t.os}-${t.cpu}`);
  const dest = path.join(dir, "bin", `mcas${ext}`);
  fs.rmSync(dir, { recursive: true, force: true });
  fs.mkdirSync(path.dirname(dest), { recursive: true });
  fs.copyFileSync(src, dest);
  fs.chmodSync(dest, 0o755);
  fs.copyFileSync(path.join(__dirname, "..", "..", "LICENSE"), path.join(dir, "LICENSE"));

  fs.writeFileSync(
    path.join(dir, "package.json"),
    JSON.stringify(
      {
        name,
        version,
        description: `${t.os}/${t.cpu} binary for ${main.name}`,
        license: main.license,
        repository: main.repository,
        os: [t.os],
        cpu: [t.cpu],
        files: ["bin", "LICENSE"],
        publishConfig: { access: "public" },
      },
      null,
      2,
    ) + "\n",
  );
  optionalDependencies[name] = version;
  console.log(`built ${name}@${version}`);
}

main.version = version;
main.optionalDependencies = optionalDependencies;
fs.writeFileSync(mainPath, JSON.stringify(main, null, 2) + "\n");
